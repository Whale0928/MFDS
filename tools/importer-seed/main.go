package main

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/bottle-note/mfds-crawler/internal/source/mfdscompany"
)

func main() {
	if err := runCLI(context.Background(), os.Args[1:], os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func runCLI(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	flags := flag.NewFlagSet("importer-seed", flag.ContinueOnError)
	flags.SetOutput(stderr)
	inputPath := flags.String("input", "importer-business-names.txt", "입력 수입사명 파일")
	manifestInputPath := flags.String("manifest-input", "", "기존 JSON manifest에서 SQL을 재생성하고 스크래핑을 건너뛰다")
	baseURLValue := flags.String("base-url", "https://impfood.mfds.go.kr", "식품안전나라 국내영업자 화면 주소")
	manifestPath := flags.String("manifest-output", "-", "JSON manifest 출력 경로 또는 -")
	sqlPath := flags.String("sql-output", "", "V12 seed SQL 출력 경로 또는 -")
	migrationPath := flags.String("migration-output", "", "생성 SQL을 고정 seed 구간에 삽입할 V12 migration 경로")
	delay := flags.Duration("delay", 1500*time.Millisecond, "요청 사이의 정중한 대기 시간")
	pageSize := flags.Int("page-size", 50, "목록 페이지 크기")
	maxAttempts := flags.Int("max-attempts", defaultMaxAttempts, "회사별 scrape 최대 시도 횟수")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if *sqlPath == "" {
		return errors.New("--sql-output을 지정해야 합니다")
	}
	if *manifestPath == "-" && *sqlPath == "-" {
		return errors.New("manifest와 SQL을 동시에 stdout으로 출력할 수 없습니다")
	}
	var manifest Manifest
	var err error
	if *manifestInputPath != "" {
		manifest, err = LoadManifest(*manifestInputPath)
	} else {
		baseURL, parseErr := url.Parse(*baseURLValue)
		if parseErr != nil {
			return fmt.Errorf("base URL 해석 실패: %w", parseErr)
		}
		manifest, err = Run(ctx, Config{
			InputPath:      *inputPath,
			BaseURL:        baseURL,
			HTTPClient:     &http.Client{Timeout: 30 * time.Second},
			Delay:          *delay,
			PageSize:       *pageSize,
			MaxAttempts:    *maxAttempts,
			IndustryCode:   defaultIndustryCode,
			OperatingState: defaultOperatingState,
		})
	}
	if err != nil {
		return err
	}
	manifestJSON, err := MarshalManifest(manifest)
	if err != nil {
		return err
	}
	seedSQL, err := BuildSQL(manifest)
	if err != nil {
		return err
	}
	if err := writeOutput(*manifestPath, manifestJSON, stdout); err != nil {
		return fmt.Errorf("manifest 출력 실패: %w", err)
	}
	if err := writeOutput(*sqlPath, []byte(seedSQL), stdout); err != nil {
		return fmt.Errorf("SQL 출력 실패: %w", err)
	}
	if *migrationPath != "" {
		if err := EmbedSeedSQL(*migrationPath, seedSQL); err != nil {
			return fmt.Errorf("V12 seed 삽입 실패: %w", err)
		}
	}
	return nil
}

func writeOutput(path string, data []byte, stdout io.Writer) error {
	if path == "-" {
		_, err := stdout.Write(data)
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

const (
	expectedImporterNameCount = 397
	defaultIndustryCode       = "141"
	defaultOperatingState     = "ING"
	defaultMaxAttempts        = 3
	migrationSeedAnchor       = "-- 아래에는 397개 최신 원장 상호의 공식 화면 조회 결과를 고정 seed로 추가한다."
	migrationSeedBegin        = "-- BEGIN GENERATED MFDS IMPORTER SEED"
	migrationSeedEnd          = "-- END GENERATED MFDS IMPORTER SEED"
)

type Config struct {
	InputPath      string
	BaseURL        *url.URL
	HTTPClient     *http.Client
	Delay          time.Duration
	PageSize       int
	MaxAttempts    int
	IndustryCode   string
	OperatingState string
	Now            func() time.Time
}

type Manifest struct {
	SchemaVersion  string       `json:"schema_version"`
	IndustryCode   string       `json:"industry_code"`
	OperatingState string       `json:"operating_state"`
	PageSize       int          `json:"page_size"`
	ImporterCount  int          `json:"importer_count"`
	Results        []SeedResult `json:"results"`
}

type SeedResult struct {
	SourceImporterName string                       `json:"source_importer_name"`
	MatchStatus        string                       `json:"match_status"`
	CandidateCount     int                          `json:"candidate_count"`
	Candidates         []mfdscompany.SearchResult   `json:"candidates"`
	ListSource         mfdscompany.SourceMetadata   `json:"list_source"`
	SearchSources      []mfdscompany.SourceMetadata `json:"search_sources"`
	Importer           *mfdscompany.BusinessDetail  `json:"importer,omitempty"`
	DetailSource       *mfdscompany.SourceMetadata  `json:"detail_source,omitempty"`
	DetailError        string                       `json:"detail_error,omitempty"`
}

func LoadManifest(path string) (Manifest, error) {
	file, err := os.Open(path)
	if err != nil {
		return Manifest{}, fmt.Errorf("manifest 열기 실패: %w", err)
	}
	defer file.Close()
	decoder := json.NewDecoder(file)
	decoder.DisallowUnknownFields()
	var manifest Manifest
	if err := decoder.Decode(&manifest); err != nil {
		return Manifest{}, fmt.Errorf("manifest JSON 해석 실패: %w", err)
	}
	if manifest.ImporterCount != expectedImporterNameCount || len(manifest.Results) != expectedImporterNameCount {
		return Manifest{}, fmt.Errorf("manifest는 수입사 %d개를 포함해야 합니다: count=%d results=%d", expectedImporterNameCount, manifest.ImporterCount, len(manifest.Results))
	}
	return manifest, nil
}

func EmbedSeedSQL(path, seedSQL string) error {
	contents, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	text := string(contents)
	if strings.Count(text, migrationSeedAnchor) != 1 {
		return errors.New("V12 seed anchor가 정확히 1개여야 합니다")
	}
	if begin := strings.Index(text, migrationSeedBegin); begin >= 0 {
		end := strings.Index(text[begin:], migrationSeedEnd)
		if end < 0 {
			return errors.New("V12 generated seed 종료 marker가 없습니다")
		}
		end += begin + len(migrationSeedEnd)
		text = strings.TrimRight(text[:begin], "\n") + text[end:]
	}
	anchorEnd := strings.Index(text, migrationSeedAnchor) + len(migrationSeedAnchor)
	generated := "\n" + migrationSeedBegin + "\n" + strings.TrimSpace(seedSQL) + "\n" + migrationSeedEnd
	updated := text[:anchorEnd] + generated + text[anchorEnd:]
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	temp, err := os.CreateTemp(filepath.Dir(path), ".v12-importer-seed-*")
	if err != nil {
		return err
	}
	tempPath := temp.Name()
	defer os.Remove(tempPath)
	if err := temp.Chmod(info.Mode().Perm()); err != nil {
		_ = temp.Close()
		return err
	}
	if _, err := temp.WriteString(updated); err != nil {
		_ = temp.Close()
		return err
	}
	if err := temp.Sync(); err != nil {
		_ = temp.Close()
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	return os.Rename(tempPath, path)
}

func LoadNames(path string) ([]string, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("수입사명 파일 열기 실패: %w", err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	var names []string
	seen := make(map[string]struct{})
	for scanner.Scan() {
		line := scanner.Text()
		if !utf8.ValidString(line) {
			return nil, errors.New("수입사명 파일이 유효한 UTF-8이 아닙니다")
		}
		name := strings.TrimSpace(line)
		if name == "" {
			continue
		}
		if _, exists := seen[name]; exists {
			return nil, fmt.Errorf("수입사명이 중복됩니다: %q", name)
		}
		seen[name] = struct{}{}
		names = append(names, name)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("수입사명 파일 읽기 실패: %w", err)
	}
	if len(names) != expectedImporterNameCount {
		return nil, fmt.Errorf("수입사명은 공백 제외 %d개여야 합니다: actual=%d", expectedImporterNameCount, len(names))
	}
	return names, nil
}

func Run(ctx context.Context, config Config) (Manifest, error) {
	if err := validateConfig(config); err != nil {
		return Manifest{}, err
	}
	names, err := LoadNames(config.InputPath)
	if err != nil {
		return Manifest{}, err
	}
	scraper, err := mfdscompany.NewScraper(mfdscompany.Options{
		BaseURL:    config.BaseURL,
		HTTPClient: config.HTTPClient,
		UserAgent:  mfdscompany.DefaultUserAgent,
		Now:        config.Now,
	})
	if err != nil {
		return Manifest{}, fmt.Errorf("업체 scraper 생성 실패: %w", err)
	}
	manifest := Manifest{
		SchemaVersion:  "mfds-importer-seed/v1",
		IndustryCode:   config.IndustryCode,
		OperatingState: config.OperatingState,
		PageSize:       config.PageSize,
		ImporterCount:  len(names),
		Results:        make([]SeedResult, 0, len(names)),
	}
	for index, name := range names {
		if index > 0 {
			if err := wait(ctx, config.Delay); err != nil {
				return Manifest{}, err
			}
		}
		result, err := scrapeNameWithRetry(ctx, scraper, config, name)
		if err != nil {
			return Manifest{}, fmt.Errorf("수입사 %q 처리 실패: %w", name, err)
		}
		manifest.Results = append(manifest.Results, result)
	}
	return manifest, nil
}

func scrapeNameWithRetry(ctx context.Context, scraper *mfdscompany.Scraper, config Config, name string) (SeedResult, error) {
	failures := make([]error, 0, config.MaxAttempts)
	for attempt := 1; attempt <= config.MaxAttempts; attempt++ {
		result, err := scrapeName(ctx, scraper, config, name)
		if err == nil {
			return result, nil
		}
		failures = append(failures, fmt.Errorf("attempt %d/%d: %w", attempt, config.MaxAttempts, err))
		if attempt < config.MaxAttempts {
			if waitErr := wait(ctx, config.Delay); waitErr != nil {
				return SeedResult{}, errors.Join(errors.Join(failures...), fmt.Errorf("retry wait after attempt %d: %w", attempt, waitErr))
			}
		}
	}
	return SeedResult{}, fmt.Errorf("company scrape failed after %d attempts: %w", config.MaxAttempts, errors.Join(failures...))
}

func validateConfig(config Config) error {
	if strings.TrimSpace(config.InputPath) == "" {
		return errors.New("입력 수입사명 파일이 필요합니다")
	}
	if config.BaseURL == nil {
		return errors.New("base URL이 필요합니다")
	}
	if config.PageSize < 1 || config.PageSize > mfdscompany.MaximumPageSize {
		return fmt.Errorf("page size는 1 이상 %d 이하여야 합니다", mfdscompany.MaximumPageSize)
	}
	if config.Delay < 0 {
		return errors.New("요청 대기 시간은 음수가 될 수 없습니다")
	}
	if config.MaxAttempts <= 0 {
		return errors.New("최대 시도 횟수는 1 이상이어야 합니다")
	}
	if strings.TrimSpace(config.IndustryCode) == "" {
		return errors.New("영업종류 코드가 필요합니다")
	}
	if strings.TrimSpace(config.OperatingState) == "" {
		return errors.New("영업상태 코드가 필요합니다")
	}
	return nil
}

func scrapeName(ctx context.Context, scraper *mfdscompany.Scraper, config Config, name string) (SeedResult, error) {
	result := SeedResult{
		SourceImporterName: name,
		Candidates:         make([]mfdscompany.SearchResult, 0),
		SearchSources:      make([]mfdscompany.SourceMetadata, 0),
	}
	var exactCandidates []mfdscompany.SearchResult
	var exactSource *mfdscompany.SourceMetadata
	for pageNumber := 1; ; pageNumber++ {
		if pageNumber > 1 {
			if err := wait(ctx, config.Delay); err != nil {
				return SeedResult{}, err
			}
		}
		page, err := scraper.Search(ctx, mfdscompany.SearchRequest{
			Page:           pageNumber,
			Limit:          config.PageSize,
			IndustryCode:   config.IndustryCode,
			BusinessName:   name,
			OperatingState: config.OperatingState,
		})
		if err != nil {
			return SeedResult{}, fmt.Errorf("목록 page=%d: %w", pageNumber, err)
		}
		result.SearchSources = append(result.SearchSources, page.Source)
		for _, candidate := range page.Rows {
			if strings.TrimSpace(candidate.BusinessName) != name {
				continue
			}
			exactCandidates = append(exactCandidates, candidate)
			if exactSource == nil {
				source := page.Source
				exactSource = &source
			}
		}
		if page.TotalPages == 0 || pageNumber >= page.TotalPages {
			break
		}
	}
	result.Candidates = exactCandidates
	result.CandidateCount = len(exactCandidates)
	if exactSource != nil {
		result.ListSource = *exactSource
	} else if len(result.SearchSources) > 0 {
		result.ListSource = result.SearchSources[0]
	}
	switch len(exactCandidates) {
	case 0:
		result.MatchStatus = "MISSING"
	case 1:
		if err := wait(ctx, config.Delay); err != nil {
			return SeedResult{}, err
		}
		detail, err := scraper.Detail(ctx, exactCandidates[0].InternalBusinessCode)
		if err != nil {
			if !isOfficialDetailUnavailableRedirect(err) {
				return SeedResult{}, fmt.Errorf("상세 조회: %w", err)
			}
			candidate := exactCandidates[0]
			detail = mfdscompany.BusinessDetail{
				InternalBusinessCode: candidate.InternalBusinessCode,
				LicenseNumber:        candidate.LicenseNumber,
				BusinessName:         candidate.BusinessName,
				Representative:       candidate.Representative,
				Institution:          candidate.Institution,
				Address:              candidate.Address,
				Industry:             candidate.Industry,
				OperatingStatus:      "UNKNOWN",
			}
			result.DetailError = "공식 상세 접근 제한 (HTTP 302)"
		}
		if strings.TrimSpace(detail.BusinessName) != name {
			return SeedResult{}, fmt.Errorf("상세 업소명이 exact 목록 결과와 다릅니다: %q", detail.BusinessName)
		}
		candidate := exactCandidates[0]
		if strings.TrimSpace(detail.Representative) == "" {
			detail.Representative = candidate.Representative
		}
		if strings.TrimSpace(detail.Institution) == "" {
			detail.Institution = candidate.Institution
		}
		if strings.TrimSpace(detail.Address) == "" {
			detail.Address = candidate.Address
		}
		if strings.TrimSpace(detail.Industry) == "" {
			detail.Industry = candidate.Industry
		}
		result.MatchStatus = "EXACT"
		result.Importer = &detail
		if !detail.Source.RetrievedAt.IsZero() {
			result.DetailSource = &detail.Source
		}
	default:
		result.MatchStatus = "AMBIGUOUS"
	}
	return result, nil
}

func isOfficialDetailUnavailableRedirect(err error) bool {
	var statusError *mfdscompany.HTTPStatusError
	if !errors.As(err, &statusError) || statusError.StatusCode != http.StatusFound {
		return false
	}
	location, parseErr := url.Parse(statusError.Location)
	if parseErr != nil {
		return false
	}
	return location.Path == "/error/noAuth" || location.Path == "/error/error"
}

func wait(ctx context.Context, delay time.Duration) error {
	if delay == 0 {
		return nil
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func MarshalManifest(manifest Manifest) ([]byte, error) {
	var buffer bytes.Buffer
	encoder := json.NewEncoder(&buffer)
	encoder.SetEscapeHTML(false)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(manifest); err != nil {
		return nil, fmt.Errorf("manifest JSON 직렬화 실패: %w", err)
	}
	return buffer.Bytes(), nil
}

func BuildSQL(manifest Manifest) (string, error) {
	if manifest.ImporterCount != len(manifest.Results) {
		return "", fmt.Errorf("manifest importer count가 결과 수와 다릅니다: count=%d results=%d", manifest.ImporterCount, len(manifest.Results))
	}
	var buffer bytes.Buffer
	buffer.WriteString("-- MFDS V12 importer seed; generated from official domestic-company pages.\n")
	for index, result := range manifest.Results {
		statement, err := seedStatement(result)
		if err != nil {
			return "", fmt.Errorf("manifest result %d: %w", index+1, err)
		}
		buffer.WriteString(statement)
		buffer.WriteString("\n")
	}
	return buffer.String(), nil
}

func seedStatement(result SeedResult) (string, error) {
	if result.ListSource.RequestURL == "" || result.ListSource.BodySHA256 == "" || result.ListSource.RetrievedAt.IsZero() {
		return "", errors.New("목록 source metadata가 없습니다")
	}
	if result.MatchStatus == "EXACT" {
		if result.Importer == nil {
			return "", errors.New("EXACT 결과에 수입사 정보가 없습니다")
		}
		return importerInsert(result)
	}
	if result.MatchStatus != "MISSING" && result.MatchStatus != "AMBIGUOUS" {
		return "", fmt.Errorf("알 수 없는 match status: %q", result.MatchStatus)
	}
	return missingImporterInsert(result)
}

func importerInsert(result SeedResult) (string, error) {
	detail := result.Importer
	permitDate, err := dateLiteral(detail.PermitDate)
	if err != nil {
		return "", err
	}
	name := strings.TrimSpace(detail.BusinessName)
	if name == "" || detail.InternalBusinessCode == "" || detail.LicenseNumber == "" {
		return "", errors.New("상세 필수 seed 값이 비어 있습니다")
	}
	nameHash := sha256.Sum256([]byte(name))
	listHash, err := digestLiteral(result.ListSource.BodySHA256)
	if err != nil {
		return "", fmt.Errorf("목록 SHA-256: %w", err)
	}
	detailURL := "NULL"
	detailHash := "NULL"
	if result.DetailSource != nil {
		if detail.Source.RequestURL == "" || detail.Source.BodySHA256 == "" || detail.Source.RetrievedAt.IsZero() {
			return "", errors.New("상세 source metadata가 불완전합니다")
		}
		detailURL = utf8Literal(detail.Source.RequestURL)
		detailHash, err = digestLiteral(detail.Source.BodySHA256)
		if err != nil {
			return "", fmt.Errorf("상세 SHA-256: %w", err)
		}
	}
	columns := "official_business_code, license_no, business_name, business_name_key_sha256, representative_name, permit_date, institution_name, primary_address, telephone_no, industry_name, operating_status, source_list_url, source_detail_url, source_list_sha256, source_detail_sha256, source_observed_at"
	values := strings.Join([]string{
		utf8Literal(detail.InternalBusinessCode),
		utf8Literal(detail.LicenseNumber),
		utf8Literal(name),
		binaryLiteral(nameHash[:]),
		nullableUTF8(detail.Representative),
		permitDate,
		nullableUTF8(detail.Institution),
		nullableUTF8(detail.Address),
		nullableUTF8(detail.Phone),
		nullableUTF8(detail.Industry),
		nullableUTF8(detail.OperatingStatus),
		utf8Literal(result.ListSource.RequestURL),
		detailURL,
		listHash,
		detailHash,
		datetimeLiteral(result.ListSource.RetrievedAt),
	}, ", ")
	return fmt.Sprintf("INSERT INTO mfds_importers (%s) VALUES (%s) ON DUPLICATE KEY UPDATE official_business_code=VALUES(official_business_code), license_no=VALUES(license_no), business_name=VALUES(business_name), business_name_key_sha256=VALUES(business_name_key_sha256), representative_name=VALUES(representative_name), permit_date=VALUES(permit_date), institution_name=VALUES(institution_name), primary_address=VALUES(primary_address), telephone_no=VALUES(telephone_no), industry_name=VALUES(industry_name), operating_status=VALUES(operating_status), source_list_url=VALUES(source_list_url), source_detail_url=VALUES(source_detail_url), source_list_sha256=VALUES(source_list_sha256), source_detail_sha256=VALUES(source_detail_sha256), source_observed_at=VALUES(source_observed_at);\n", columns, values), nil
}

func missingImporterInsert(result SeedResult) (string, error) {
	candidatesJSON := mustJSON(result.Candidates)
	nameHash := sha256.Sum256([]byte(result.SourceImporterName))
	listHash, err := digestLiteral(result.ListSource.BodySHA256)
	if err != nil {
		return "", fmt.Errorf("목록 SHA-256: %w", err)
	}
	columns := "source_importer_name, source_name_key_sha256, match_status, candidate_count, candidates_json, source_list_url, source_list_sha256, source_observed_at"
	values := strings.Join([]string{
		utf8Literal(result.SourceImporterName),
		binaryLiteral(nameHash[:]),
		utf8Literal(result.MatchStatus),
		strconv.Itoa(result.CandidateCount),
		utf8Literal(candidatesJSON),
		utf8Literal(result.ListSource.RequestURL),
		listHash,
		datetimeLiteral(result.ListSource.RetrievedAt),
	}, ", ")
	return fmt.Sprintf("INSERT INTO mfds_missing_importers (%s) VALUES (%s) ON DUPLICATE KEY UPDATE source_importer_name=VALUES(source_importer_name), source_name_key_sha256=VALUES(source_name_key_sha256), match_status=VALUES(match_status), candidate_count=VALUES(candidate_count), candidates_json=VALUES(candidates_json), source_list_url=VALUES(source_list_url), source_list_sha256=VALUES(source_list_sha256), source_observed_at=VALUES(source_observed_at);\n", columns, values), nil
}

func mustJSON(value any) string {
	var buffer bytes.Buffer
	encoder := json.NewEncoder(&buffer)
	encoder.SetEscapeHTML(false)
	_ = encoder.Encode(value)
	return strings.TrimSuffix(buffer.String(), "\n")
}

func utf8Literal(value string) string {
	return "CONVERT(X'" + hex.EncodeToString([]byte(value)) + "' USING utf8mb4)"
}

func nullableUTF8(value string) string {
	if strings.TrimSpace(value) == "" {
		return "NULL"
	}
	return utf8Literal(value)
}

func binaryLiteral(value []byte) string {
	return "X'" + hex.EncodeToString(value) + "'"
}

func digestLiteral(value string) (string, error) {
	digest, err := hex.DecodeString(strings.TrimSpace(value))
	if err != nil || len(digest) != sha256.Size {
		return "", fmt.Errorf("64자리 hex digest가 아닙니다: %q", value)
	}
	return binaryLiteral(digest), nil
}

func dateLiteral(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "NULL", nil
	}
	var parsed time.Time
	var err error
	for _, layout := range []string{"2006-01-02", "2006.01.02", "2006/01/02", "20060102"} {
		parsed, err = time.Parse(layout, value)
		if err == nil {
			return "CAST(" + utf8Literal(parsed.Format("2006-01-02")) + " AS DATE)", nil
		}
	}
	return "", fmt.Errorf("허가일자를 날짜로 해석할 수 없습니다: %q", value)
}

func datetimeLiteral(value time.Time) string {
	location := time.FixedZone("Asia/Seoul", 9*60*60)
	return "CAST(" + utf8Literal(value.In(location).Format("2006-01-02 15:04:05.999999")) + " AS DATETIME(6))"
}
