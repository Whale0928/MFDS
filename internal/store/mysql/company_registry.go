package mysql

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/bottle-note/mfds-crawler/internal/usecase/companyregistry"
)

func (s *Store) StartCollection(ctx context.Context, startedAt time.Time, configJSON json.RawMessage) (uint64, error) {
	runUUID, err := randomUUID()
	if err != nil {
		return 0, err
	}
	matcherVersion := "importer-license-match-v1"
	var snapshot struct {
		MatcherVersion string `json:"matcher_version"`
	}
	if json.Unmarshal(configJSON, &snapshot) == nil && snapshot.MatcherVersion != "" {
		matcherVersion = snapshot.MatcherVersion
	}
	servicesJSON, _ := json.Marshal(companyregistry.Services)
	result, err := s.db.ExecContext(ctx, `
		INSERT INTO company_registry_runs (
			run_uuid, status, config_json, matcher_version, requested_services_json, started_at
		) VALUES (?, 'RUNNING', ?, ?, ?, ?)
	`, runUUID, configJSON, matcherVersion, servicesJSON, startedAt)
	if err != nil {
		return 0, fmt.Errorf("업체 등록정보 수집 실행 생성 실패: %w", err)
	}
	id, err := result.LastInsertId()
	if err != nil || id <= 0 {
		return 0, fmt.Errorf("업체 등록정보 수집 실행 ID 확인 실패: id=%d: %w", id, err)
	}
	return uint64(id), nil
}

func (s *Store) SavePage(ctx context.Context, runID uint64, page companyregistry.Page, fetchErr error) error {
	return s.withTx(ctx, func(tx *sql.Tx) error {
		fetchID, err := insertCompanyRegistryFetch(ctx, tx, runID, page, fetchErr)
		if err != nil {
			return err
		}
		if fetchErr == nil && page.ResultCode == "INFO-000" {
			for index, raw := range page.Rows {
				if err := insertCompanyRegistryRawRow(ctx, tx, fetchID, index+1, page.Service, raw, page.FinishedAt); err != nil {
					return err
				}
			}
		}
		_, err = tx.ExecContext(ctx, `
			UPDATE company_registry_runs
			SET fetched_requests = fetched_requests + 1,
			    parsed_rows = parsed_rows + ?
			WHERE id = ?
		`, len(page.Rows), runID)
		if err != nil {
			return fmt.Errorf("업체 등록정보 수집 실행 집계 갱신 실패: %w", err)
		}
		return nil
	})
}

func insertCompanyRegistryFetch(
	ctx context.Context,
	tx *sql.Tx,
	runID uint64,
	page companyregistry.Page,
	fetchErr error,
) (uint64, error) {
	requestFingerprint := sha256.Sum256([]byte(fmt.Sprintf("%s/%d/%d", page.Service, page.StartIndex, page.EndIndex)))
	responseHash := sha256.Sum256(page.RawBody)
	compressed, err := gzipBytes(page.RawBody)
	if err != nil {
		return 0, err
	}
	headersJSON, err := json.Marshal(page.ResponseHeaders)
	if err != nil {
		return 0, fmt.Errorf("업체 등록정보 응답 header 직렬화 실패: %w", err)
	}
	filterJSON := page.RequestFilterJSON
	if len(filterJSON) == 0 {
		filterJSON = json.RawMessage(`{}`)
	}
	status := "PARSED"
	errorKind, errorMessage := "", ""
	if fetchErr != nil {
		status, errorKind, errorMessage = "FAILED", companyRegistryErrorKind(fetchErr), fetchErr.Error()
	}
	pageSize := page.EndIndex - page.StartIndex + 1
	pageNo := 1
	if pageSize > 0 && page.StartIndex > 0 {
		pageNo = (page.StartIndex-1)/pageSize + 1
	}
	redactedPath := page.RequestPathRedacted
	if redactedPath == "" {
		redactedPath = fmt.Sprintf("/api/<redacted>/%s/json/%d/%d", page.Service, page.StartIndex, page.EndIndex)
	}
	duration := page.FinishedAt.Sub(page.StartedAt)
	if duration < 0 {
		duration = 0
	}
	var totalCount any
	if page.ResultCode != "" {
		totalCount = page.TotalCount
	}
	result, err := tx.ExecContext(ctx, `
		INSERT INTO company_registry_fetches (
			run_id, service_id, page_no, start_idx, end_idx,
			request_key_sha256, request_method, request_path_redacted, request_filter_json,
			attempt_no, started_at, finished_at, duration_ms, http_status,
			response_headers_json, content_type, response_body_encoding, response_body_gzip,
			response_size_bytes, response_sha256, result_code, result_message, total_count,
			parser_version, parsed_row_count, status, error_kind, error_message
		) VALUES (?, ?, ?, ?, ?, ?, 'GET', ?, ?, ?, ?, ?, ?, ?, ?, ?, 'gzip', ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, runID, page.Service, pageNo, page.StartIndex, page.EndIndex, requestFingerprint[:], redactedPath,
		filterJSON, page.Attempt, page.StartedAt, nullableObservedTime(page.FinishedAt), duration.Milliseconds(),
		nullableUint(page.HTTPStatus), headersJSON, nullString(page.ContentType), compressed, len(page.RawBody),
		responseHash[:], nullString(page.ResultCode), nullString(page.ResultMessage), totalCount,
		"foodsafetykorea-json-v1", len(page.Rows), status, nullString(errorKind), nullString(errorMessage))
	if err != nil {
		return 0, fmt.Errorf("업체 등록정보 fetch 저장 실패: %w", err)
	}
	id, err := result.LastInsertId()
	if err != nil || id <= 0 {
		return 0, fmt.Errorf("업체 등록정보 fetch ID 확인 실패: id=%d: %w", id, err)
	}
	return uint64(id), nil
}

func insertCompanyRegistryRawRow(
	ctx context.Context,
	tx *sql.Tx,
	fetchID uint64,
	rowNo int,
	service companyregistry.ServiceID,
	raw json.RawMessage,
	observedAt time.Time,
) error {
	if !json.Valid(raw) {
		return fmt.Errorf("%s %d행 JSON이 유효하지 않습니다", service, rowNo)
	}
	hash := sha256.Sum256(raw)
	if observedAt.IsZero() {
		observedAt = time.Now()
	}
	switch service {
	case companyregistry.ServiceC001:
		var row c001RawRow
		if err := json.Unmarshal(raw, &row); err != nil {
			return err
		}
		_, err := tx.ExecContext(ctx, `
			INSERT INTO c001_importer_licenses_raw (
				fetch_id, row_no, president_name, permit_date_raw, license_no, institution_name,
				business_name, business_name_search_key, location_address, telephone_no, industry_name,
				raw_payload_json, raw_payload_sha256, observed_at
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		`, fetchID, rowNo, nullString(row.PresidentName), nullString(row.PermitDate), nullString(row.LicenseNo),
			nullString(row.InstitutionName), nullString(row.BusinessName), nullString(companyRegistryNameKey(row.BusinessName)),
			nullString(row.Address), nullString(row.TelephoneNo), nullString(row.IndustryName), raw, hash[:], observedAt)
		return wrapRawInsertError(service, rowNo, err)
	case companyregistry.ServiceI2821:
		var row i2821RawRow
		if err := json.Unmarshal(raw, &row); err != nil {
			return err
		}
		_, err := tx.ExecContext(ctx, `
			INSERT INTO i2821_importer_closures_raw (
				fetch_id, row_no, closure_date_raw, president_name, permit_date_raw, license_no,
				institution_name, business_name, closure_status_name, location_address, industry_name,
				raw_payload_json, raw_payload_sha256, observed_at
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		`, fetchID, rowNo, nullString(row.ClosureDate), nullString(row.PresidentName), nullString(row.PermitDate),
			nullString(row.LicenseNo), nullString(row.InstitutionName), nullString(row.BusinessName),
			nullString(row.ClosureStatusName), nullString(row.Address), nullString(row.IndustryName), raw, hash[:], observedAt)
		return wrapRawInsertError(service, rowNo, err)
	case companyregistry.ServiceI0250:
		var row i0250RawRow
		if err := json.Unmarshal(raw, &row); err != nil {
			return err
		}
		_, err := tx.ExecContext(ctx, `
			INSERT INTO i0250_excellent_importers_raw (
				fetch_id, row_no, export_country_name, import_product_manufacturer_name, permit_date_raw,
				product_count_raw, license_no, product_name, excellent_importer_registration_no,
				business_name, address, raw_payload_json, raw_payload_sha256, observed_at
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		`, fetchID, rowNo, nullString(row.ExportCountryName), nullString(row.ManufacturerName), nullString(row.PermitDate),
			nullString(row.ProductCount), nullString(row.LicenseNo), nullString(row.ProductName),
			nullString(row.RegistrationNo), nullString(row.BusinessName), nullString(row.Address), raw, hash[:], observedAt)
		return wrapRawInsertError(service, rowNo, err)
	case companyregistry.ServiceI0470:
		var row i0470RawRow
		if err := json.Unmarshal(raw, &row); err != nil {
			return err
		}
		_, err := tx.ExecContext(ctx, `
			INSERT INTO i0470_administrative_dispositions_raw (
				fetch_id, row_no, president_name, last_updated_raw, license_no,
				disposition_institution_name, violated_law_name, disposition_detail_sequence,
				violation_content, address, public_until_raw, industry_name, decision_date_raw,
				business_name, disposition_start_date_raw, disposition_type_name,
				disposition_end_date_raw, telephone_no, disposition_content,
				raw_payload_json, raw_payload_sha256, observed_at
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		`, fetchID, rowNo, nullString(row.PresidentName), nullString(row.LastUpdated), nullString(row.LicenseNo),
			nullString(row.InstitutionName), nullString(row.ViolatedLawName), nullString(row.Sequence),
			nullString(row.ViolationContent), nullString(row.Address), nullString(row.PublicUntil),
			nullString(row.IndustryName), nullString(row.DecisionDate), nullString(row.BusinessName),
			nullString(row.StartDate), nullString(row.TypeName), nullString(row.EndDate),
			nullString(row.TelephoneNo), nullString(row.DispositionContent), raw, hash[:], observedAt)
		return wrapRawInsertError(service, rowNo, err)
	default:
		return fmt.Errorf("지원하지 않는 업체 등록정보 서비스: %s", service)
	}
}

func (s *Store) ListLatestImporters(ctx context.Context) ([]companyregistry.Importer, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT latest.id, latest.rcno, latest.importer_name
		FROM (
			SELECT i.id, i.rcno, i.importer_name,
			       ROW_NUMBER() OVER (PARTITION BY i.rcno ORDER BY i.observed_at DESC, i.id DESC) AS rn
			FROM items AS i
			WHERE i.importer_name IS NOT NULL AND TRIM(i.importer_name) <> ''
		) AS latest
		WHERE latest.rn = 1
		ORDER BY latest.id
	`)
	if err != nil {
		return nil, fmt.Errorf("최신 수입업체 원장 조회 실패: %w", err)
	}
	defer rows.Close()
	var result []companyregistry.Importer
	for rows.Next() {
		var importer companyregistry.Importer
		if err := rows.Scan(&importer.SourceItemID, &importer.RCNO, &importer.Name); err != nil {
			return nil, fmt.Errorf("최신 수입업체 원장 scan 실패: %w", err)
		}
		result = append(result, importer)
	}
	return result, rows.Err()
}

func (s *Store) ListC001Candidates(ctx context.Context, runID uint64) ([]companyregistry.LicenseCandidate, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT r.id, COALESCE(r.license_no, ''), COALESCE(r.business_name, ''), COALESCE(r.location_address, '')
		FROM c001_importer_licenses_raw AS r
		JOIN company_registry_fetches AS f ON f.id = r.fetch_id
		WHERE f.run_id = ?
		ORDER BY r.id
	`, runID)
	if err != nil {
		return nil, fmt.Errorf("C001 업체 후보 조회 실패: %w", err)
	}
	defer rows.Close()
	var result []companyregistry.LicenseCandidate
	for rows.Next() {
		var candidate companyregistry.LicenseCandidate
		if err := rows.Scan(&candidate.RawID, &candidate.LicenseNo, &candidate.BusinessName, &candidate.Address); err != nil {
			return nil, fmt.Errorf("C001 업체 후보 scan 실패: %w", err)
		}
		result = append(result, candidate)
	}
	return result, rows.Err()
}

func (s *Store) SaveMatchEvidence(ctx context.Context, runID uint64, evidence []companyregistry.MatchEvidence) error {
	return s.withTx(ctx, func(tx *sql.Tx) error {
		stmt, err := tx.PrepareContext(ctx, `
			INSERT INTO importer_license_match_evidence (
				run_id, source_item_id, source_rcno, source_importer_name, source_importer_name_key,
				selected_c001_raw_id, selected_license_no, selected_business_name, selected_location_address,
				match_status, evidence_json, matcher_version, candidate_count, matched_at
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		`)
		if err != nil {
			return fmt.Errorf("수입업체 매칭 근거 statement 준비 실패: %w", err)
		}
		defer stmt.Close()
		for _, item := range evidence {
			if _, err := stmt.ExecContext(ctx, runID, item.SourceItemID, item.RCNO, item.ImporterName,
				item.ImporterMatchKey, item.C001RawID, nullString(item.LicenseNo), nullString(item.BusinessName),
				nullString(item.Address), item.Status, item.EvidenceJSON, item.MatcherVersion,
				item.CandidateCount, item.MatchedAt); err != nil {
				return fmt.Errorf("수입업체 매칭 근거 저장 실패: source_item_id=%d: %w", item.SourceItemID, err)
			}
		}
		return nil
	})
}

func (s *Store) CompleteCollection(ctx context.Context, runID uint64, summary companyregistry.Summary, finishedAt time.Time) error {
	matched := summary.Matches[companyregistry.MatchExactName] + summary.Matches[companyregistry.MatchNormalizedName] +
		summary.Matches[companyregistry.MatchNameAndAddress] + summary.Matches[companyregistry.MatchConfirmedAlias] +
		summary.Matches[companyregistry.MatchManual]
	result, err := s.db.ExecContext(ctx, `
		UPDATE company_registry_runs
		SET status = 'COMPLETED', completed_services = total_services,
		    matched_count = ?, ambiguous_count = ?, unresolved_count = ?, finished_at = ?
		WHERE id = ? AND status = 'RUNNING'
	`, matched, summary.Matches[companyregistry.MatchAmbiguous], summary.Matches[companyregistry.MatchUnresolved], finishedAt, runID)
	if err != nil {
		return fmt.Errorf("업체 등록정보 수집 완료 갱신 실패: %w", err)
	}
	return requireOne(result, "업체 등록정보 수집 완료 갱신")
}

func (s *Store) FailCollection(ctx context.Context, runID uint64, cause error, finishedAt time.Time) error {
	result, err := s.db.ExecContext(ctx, `
		UPDATE company_registry_runs
		SET status = 'FAILED', last_error = ?, finished_at = ?
		WHERE id = ? AND status = 'RUNNING'
	`, cause.Error(), finishedAt, runID)
	if err != nil {
		return fmt.Errorf("업체 등록정보 수집 실패 갱신 오류: %w", err)
	}
	return requireOne(result, "업체 등록정보 수집 실패 갱신")
}

type c001RawRow struct {
	PresidentName   string `json:"PRSDNT_NM"`
	PermitDate      string `json:"PRMS_DT"`
	LicenseNo       string `json:"LCNS_NO"`
	InstitutionName string `json:"INSTT_NM"`
	BusinessName    string `json:"BSSH_NM"`
	Address         string `json:"LOCP_ADDR"`
	TelephoneNo     string `json:"TELNO"`
	IndustryName    string `json:"INDUTY_NM"`
}

type i2821RawRow struct {
	ClosureDate       string `json:"CLSBIZ_DT"`
	PresidentName     string `json:"PRSDNT_NM"`
	PermitDate        string `json:"PRMS_DT"`
	LicenseNo         string `json:"LCNS_NO"`
	InstitutionName   string `json:"INSTT_NM"`
	BusinessName      string `json:"BSSH_NM"`
	ClosureStatusName string `json:"CLSBIZ_DVS_CD_NM"`
	Address           string `json:"LOCP_ADDR"`
	IndustryName      string `json:"INDUTY_NM"`
}

type i0250RawRow struct {
	ExportCountryName string `json:"EXCOURY_NATN_CD_NM"`
	ManufacturerName  string `json:"INCM_PRDT_XPORT_MC_NM"`
	PermitDate        string `json:"PRMS_DT"`
	ProductCount      string `json:"PRDLST_CNT"`
	LicenseNo         string `json:"LCNS_NO"`
	ProductName       string `json:"PRDLST_NM"`
	RegistrationNo    string `json:"EXCLNC_INCM_BSSH_REGNO"`
	BusinessName      string `json:"BSSH_NM"`
	Address           string `json:"ADDR"`
}

type i0470RawRow struct {
	PresidentName      string `json:"PRSDNT_NM"`
	LastUpdated        string `json:"LAST_UPDT_DTM"`
	LicenseNo          string `json:"LCNS_NO"`
	InstitutionName    string `json:"DSPS_INSTTCD_NM"`
	ViolatedLawName    string `json:"LAWORD_CD_NM"`
	Sequence           string `json:"DSPSDTLS_SEQ"`
	ViolationContent   string `json:"VILTCN"`
	Address            string `json:"ADDR"`
	PublicUntil        string `json:"PUBLIC_DT"`
	IndustryName       string `json:"INDUTY_CD_NM"`
	DecisionDate       string `json:"DSPS_DCSNDT"`
	BusinessName       string `json:"PRCSCITYPOINT_BSSHNM"`
	StartDate          string `json:"DSPS_BGNDT"`
	TypeName           string `json:"DSPS_TYPECD_NM"`
	EndDate            string `json:"DSPS_ENDDT"`
	TelephoneNo        string `json:"TELNO"`
	DispositionContent string `json:"DSPSCN"`
}

func gzipBytes(value []byte) ([]byte, error) {
	var buffer bytes.Buffer
	writer := gzip.NewWriter(&buffer)
	if _, err := writer.Write(value); err != nil {
		return nil, fmt.Errorf("업체 등록정보 응답 압축 실패: %w", err)
	}
	if err := writer.Close(); err != nil {
		return nil, fmt.Errorf("업체 등록정보 응답 압축 종료 실패: %w", err)
	}
	return buffer.Bytes(), nil
}

func randomUUID() (string, error) {
	value := make([]byte, 16)
	if _, err := rand.Read(value); err != nil {
		return "", fmt.Errorf("업체 등록정보 실행 UUID 생성 실패: %w", err)
	}
	value[6] = value[6]&0x0f | 0x40
	value[8] = value[8]&0x3f | 0x80
	encoded := hex.EncodeToString(value)
	return encoded[0:8] + "-" + encoded[8:12] + "-" + encoded[12:16] + "-" + encoded[16:20] + "-" + encoded[20:], nil
}

func companyRegistryNameKey(value string) string {
	value = strings.ToLower(strings.Join(strings.Fields(value), " "))
	return strings.NewReplacer("㈜", "", "(주)", "", "주식회사", "", "유한회사", "", " ", "", "(", "", ")", "").Replace(value)
}

func companyRegistryErrorKind(err error) string {
	var classified interface{ Kind() string }
	if errors.As(err, &classified) {
		return classified.Kind()
	}
	return "UNKNOWN"
}

func wrapRawInsertError(service companyregistry.ServiceID, rowNo int, err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%s 원문 %d행 저장 실패: %w", service, rowNo, err)
}

func nullableObservedTime(value time.Time) any {
	if value.IsZero() {
		return nil
	}
	return value
}

func nullableUint(value int) any {
	if value <= 0 {
		return nil
	}
	return value
}
