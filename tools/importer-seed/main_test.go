package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/bottle-note/mfds-crawler/internal/source/mfdscompany"
)

func TestRun_OfficialSearchBuildsDeterministicManifestAndV12SQL(t *testing.T) {
	const exactName = "주식회사 정확수입사"
	const missingName = "없는 수입사"
	const ambiguousName = "동명이인 수입사"
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path == "/CFCCC01F02/getList" {
			if request.URL.Query().Get("indutyCd") != defaultIndustryCode || request.URL.Query().Get("selBsshSttus") != defaultOperatingState {
				t.Fatalf("unexpected list query: %s", request.URL.RawQuery)
			}
			writer.Header().Set("Content-Type", "text/html; charset=utf-8")
			switch request.URL.Query().Get("bsnOfcName") {
			case exactName:
				_, _ = writer.Write([]byte(listFixture(exactName, 1)))
			case ambiguousName:
				_, _ = writer.Write([]byte(listFixture(ambiguousName, 2)))
			default:
				_, _ = writer.Write([]byte(emptyListFixture))
			}
			return
		}
		if request.URL.Path == "/CFCCC01P01" {
			writer.Header().Set("Content-Type", "text/html; charset=utf-8")
			if request.URL.Query().Get("srchBsshCode") != "2024001000000001" {
				t.Fatalf("unexpected detail query: %s", request.URL.RawQuery)
			}
			_, _ = writer.Write([]byte(detailFixture(exactName)))
			return
		}
		http.NotFound(writer, request)
	}))
	defer server.Close()

	inputPath := writeNamesFile(t, exactName, missingName, ambiguousName)
	baseURL, err := url.Parse(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	wantObservedAt := time.Date(2026, time.August, 15, 9, 10, 11, 123456000, time.FixedZone("Asia/Seoul", 9*60*60))
	manifest, err := Run(context.Background(), Config{
		InputPath:      inputPath,
		BaseURL:        baseURL,
		Delay:          0,
		PageSize:       50,
		MaxAttempts:    3,
		IndustryCode:   defaultIndustryCode,
		OperatingState: defaultOperatingState,
		Now:            func() time.Time { return wantObservedAt },
	})
	if err != nil {
		t.Fatal(err)
	}
	if manifest.ImporterCount != expectedImporterNameCount || len(manifest.Results) != expectedImporterNameCount {
		t.Fatalf("manifest count = %d/%d", manifest.ImporterCount, len(manifest.Results))
	}
	if manifest.Results[0].MatchStatus != "EXACT" || manifest.Results[0].CandidateCount != 1 {
		t.Fatalf("exact result = %+v", manifest.Results[0])
	}
	if manifest.Results[0].Importer.Representative != "목록 대표자" || manifest.Results[0].Importer.Institution != "목록 기관" ||
		manifest.Results[0].Importer.Address != "목록 주소" || manifest.Results[0].Importer.Industry != "목록 업종" {
		t.Fatalf("detail fields were not merged from list candidate: %+v", manifest.Results[0].Importer)
	}
	if manifest.Results[1].MatchStatus != "MISSING" || manifest.Results[1].CandidateCount != 0 {
		t.Fatalf("missing result = %+v", manifest.Results[1])
	}
	if manifest.Results[2].MatchStatus != "AMBIGUOUS" || manifest.Results[2].CandidateCount != 2 {
		t.Fatalf("ambiguous result = %+v", manifest.Results[2])
	}

	manifestJSON, err := MarshalManifest(manifest)
	if err != nil {
		t.Fatal(err)
	}
	manifestJSONAgain, err := MarshalManifest(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if string(manifestJSON) != string(manifestJSONAgain) || !strings.Contains(string(manifestJSON), "주식회사 정확수입사") {
		t.Fatal("manifest is not deterministic UTF-8 JSON")
	}

	seedSQL, err := BuildSQL(manifest)
	if err != nil {
		t.Fatal(err)
	}
	seedSQLAgain, err := BuildSQL(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if seedSQL != seedSQLAgain {
		t.Fatal("seed SQL is not deterministic")
	}
	if strings.Contains(seedSQL, "START TRANSACTION") || strings.Contains(seedSQL, "COMMIT;") {
		t.Fatal("Flyway seed must not wrap statements in a transaction")
	}
	for _, column := range []string{
		"official_business_code", "license_no", "business_name_key_sha256", "source_list_sha256", "source_detail_sha256", "source_observed_at",
		"source_importer_name", "source_name_key_sha256", "match_status", "candidate_count", "candidates_json",
	} {
		if !strings.Contains(seedSQL, column) {
			t.Fatalf("SQL missing V12 column %q", column)
		}
	}
	if strings.Contains(seedSQL, "description") || strings.Contains(seedSQL, "admin_note") || strings.Contains(seedSQL, "admin_status") {
		t.Fatal("seed SQL must not overwrite admin fields")
	}
	listDigest := sha256.Sum256([]byte(listFixture(exactName, 1)))
	if !strings.Contains(seedSQL, "X'"+hex.EncodeToString(listDigest[:])+"'") {
		t.Fatal("list SHA-256 was not emitted as BINARY(32)")
	}
	if !strings.Contains(seedSQL, "CAST(CONVERT(X'323032362d30382d31352030393a31303a31312e313233343536' USING utf8mb4) AS DATETIME(6))") {
		t.Fatal("source_observed_at was not formatted in Asia/Seoul")
	}
	if strings.Contains(seedSQL, "주식회사 정확수입사") || strings.Contains(seedSQL, "동명이인 수입사") {
		t.Fatal("SQL text values must use UTF-8 hex literals")
	}
}

func TestLoadNames_Requires397UniqueNonblankNames(t *testing.T) {
	path := filepath.Join(t.TempDir(), "names.txt")
	if err := os.WriteFile(path, []byte("한 곳\n\n두 곳\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadNames(path); err == nil || !strings.Contains(err.Error(), "397") {
		t.Fatalf("LoadNames() error = %v", err)
	}
}

func TestEmbedSeedSQL_ReplacesGeneratedSectionIdempotently(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "V12.sql")
	initial := "CREATE TABLE mfds_importers (id BIGINT);\n" + migrationSeedAnchor + "\n-- preserved tail\n"
	if err := os.WriteFile(path, []byte(initial), 0o640); err != nil {
		t.Fatal(err)
	}
	if err := EmbedSeedSQL(path, "INSERT INTO mfds_importers VALUES (1);\n"); err != nil {
		t.Fatal(err)
	}
	if err := EmbedSeedSQL(path, "INSERT INTO mfds_importers VALUES (2);\n"); err != nil {
		t.Fatal(err)
	}
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	actual := string(contents)
	if strings.Count(actual, migrationSeedBegin) != 1 || strings.Count(actual, migrationSeedEnd) != 1 {
		t.Fatalf("generated markers are not unique: %s", actual)
	}
	if strings.Contains(actual, "VALUES (1)") || !strings.Contains(actual, "VALUES (2)") || !strings.Contains(actual, "-- preserved tail") {
		t.Fatalf("generated section was not replaced safely: %s", actual)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o640 {
		t.Fatalf("migration mode = %o, want 640", info.Mode().Perm())
	}
}

func TestValidateConfig_RejectsNonPositiveMaxAttempts(t *testing.T) {
	baseURL, err := url.Parse("https://example.test")
	if err != nil {
		t.Fatal(err)
	}
	for _, attempts := range []int{0, -1} {
		err := validateConfig(Config{
			InputPath:      "names.txt",
			BaseURL:        baseURL,
			PageSize:       50,
			MaxAttempts:    attempts,
			IndustryCode:   defaultIndustryCode,
			OperatingState: defaultOperatingState,
		})
		if err == nil || !strings.Contains(err.Error(), "1 이상") {
			t.Fatalf("MaxAttempts=%d error = %v", attempts, err)
		}
	}
}

func TestRun_NoAuth상세는목록정보로보존한다(t *testing.T) {
	const importerName = "주식회사 상세제한수입사"
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/CFCCC01F02/getList":
			writer.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = writer.Write([]byte(listFixture(request.URL.Query().Get("bsnOfcName"), 1)))
		case "/CFCCC01P01":
			writer.Header().Set("Location", server.URL+"/error/noAuth")
			writer.WriteHeader(http.StatusFound)
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()
	baseURL, err := url.Parse(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := Run(context.Background(), Config{
		InputPath:      writeNamesFile(t, importerName, "두번째", "세번째"),
		BaseURL:        baseURL,
		HTTPClient:     server.Client(),
		Delay:          0,
		PageSize:       50,
		MaxAttempts:    1,
		IndustryCode:   defaultIndustryCode,
		OperatingState: defaultOperatingState,
		Now:            func() time.Time { return time.Date(2026, 8, 15, 9, 0, 0, 0, time.FixedZone("KST", 9*60*60)) },
	})
	if err != nil {
		t.Fatal(err)
	}
	result := manifest.Results[0]
	if result.MatchStatus != "EXACT" || result.Importer == nil || result.Importer.BusinessName != importerName ||
		result.Importer.OperatingStatus != "UNKNOWN" || result.DetailSource != nil || result.DetailError == "" {
		t.Fatalf("noAuth fallback result = %+v", result)
	}
	seedSQL, err := BuildSQL(Manifest{ImporterCount: 1, Results: []SeedResult{result}})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(seedSQL, ", NULL, ") || !strings.Contains(seedSQL, "source_detail_sha256") {
		t.Fatalf("detail-unavailable SQL was not preserved as NULL: %s", seedSQL)
	}
}

func TestRun_Error상세는목록정보로보존한다(t *testing.T) {
	t.Parallel()

	err := &mfdscompany.HTTPStatusError{
		StatusCode: http.StatusFound,
		Location:   "https://impfood.mfds.go.kr/error/error",
	}
	if !isOfficialDetailUnavailableRedirect(err) {
		t.Fatal("현재 공식 사이트의 상세 접근 제한 redirect를 인식하지 못했습니다")
	}
}

func TestRun_RetriesCompanyScrapeAfterFirstHTTPFailure(t *testing.T) {
	const exactName = "재시도 수입사"
	listRequests := 0
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path == "/CFCCC01F02/getList" {
			listRequests++
			if listRequests == 1 {
				http.Error(writer, "temporary failure", http.StatusBadGateway)
				return
			}
			writer.Header().Set("Content-Type", "text/html; charset=utf-8")
			if request.URL.Query().Get("bsnOfcName") == exactName {
				_, _ = writer.Write([]byte(listFixture(exactName, 1)))
				return
			}
			_, _ = writer.Write([]byte(emptyListFixture))
			return
		}
		if request.URL.Path == "/CFCCC01P01" {
			writer.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = writer.Write([]byte(detailFixture(exactName)))
			return
		}
		http.NotFound(writer, request)
	}))
	defer server.Close()

	inputPath := writeNamesFile(t, exactName, "재시도 없는 수입사", "재시도 모호 수입사")
	baseURL, err := url.Parse(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := Run(context.Background(), Config{
		InputPath:      inputPath,
		BaseURL:        baseURL,
		Delay:          0,
		PageSize:       50,
		MaxAttempts:    3,
		IndustryCode:   defaultIndustryCode,
		OperatingState: defaultOperatingState,
	})
	if err != nil {
		t.Fatal(err)
	}
	if listRequests != expectedImporterNameCount+1 {
		t.Fatalf("list requests = %d, want one bounded retry", listRequests)
	}
	if manifest.Results[0].MatchStatus != "EXACT" {
		t.Fatalf("retried result = %+v", manifest.Results[0])
	}
}

func writeNamesFile(t *testing.T, first, second, third string) string {
	t.Helper()
	names := []string{first, second, third}
	for index := 0; len(names) < expectedImporterNameCount; index++ {
		names = append(names, fmt.Sprintf("테스트 수입사 %03d", index))
	}
	path := filepath.Join(t.TempDir(), "importer-business-names.txt")
	if err := os.WriteFile(path, []byte(strings.Join(names, "\n")+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func listFixture(name string, count int) string {
	rows := ""
	for index := 0; index < count; index++ {
		code := fmt.Sprintf("202400100000000%d", index+1)
		rows += fmt.Sprintf("<tr><td><a href=\"javascript:fnDetailPopup1(&#39;%s&#39;);\">%d</a></td><td>2024000123%d</td><td>%s</td><td>목록 업종</td><td>목록 대표자</td><td>목록 주소</td><td>목록 기관</td></tr>", code, index+1, index+4, name)
	}
	return fmt.Sprintf(`<!doctype html><html lang="ko"><body><ul><li>
<div class="board_count"><span>총 <strong>%d</strong>건</span></div>
<table><caption>국내영업자 검색 조회 테이블</caption><thead><tr><th>번호</th><th>인허가번호</th><th>업소명</th><th>업종</th><th>대표자</th><th>소재지</th><th>인허가기관</th></tr></thead>
<tbody>%s</tbody></table></li></ul></body></html>`, count, rows)
}

const emptyListFixture = `<!doctype html><html lang="ko"><body><ul><li>
<div class="board_count"><span>총 <strong>0</strong>건</span></div>
<table><caption>국내영업자 검색 조회 테이블</caption><thead><tr><th>번호</th><th>인허가번호</th><th>업소명</th><th>업종</th><th>대표자</th><th>소재지</th><th>인허가기관</th></tr></thead>
<tbody><tr><td colspan="7">조회 결과가 없습니다.</td></tr></tbody></table></li></ul></body></html>`

func detailFixture(name string) string {
	return fmt.Sprintf(`<!doctype html><html lang="ko"><body>
<table class="board_view"><caption>업소 기본정보</caption><tbody>
<tr><th>업소명</th><td>%s</td><th>인허가번호</th><td>20240001234</td></tr>
</tbody></table></body></html>`, name)
}
