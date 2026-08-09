package dashboard

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
)

const officialSourceRowsSQL = `WITH official_source_rows AS (
    SELECT 'BUSINESS_LICENSE' AS source_type, r.id AS source_row_id,
           COALESCE(r.business_name, '') AS business_name,
           COALESCE(r.license_no, '') AS license_number,
           COALESCE(r.industry_name, '') AS industry_name,
           COALESCE(r.location_address, '') AS address,
           r.observed_at
    FROM c001_importer_licenses_raw AS r
    JOIN company_registry_fetches AS f ON f.id = r.fetch_id
    JOIN company_registry_runs AS run ON run.id = f.run_id AND run.status = 'COMPLETED'
    UNION ALL
    SELECT 'CLOSURE', r.id, COALESCE(r.business_name, ''), COALESCE(r.license_no, ''),
           COALESCE(r.industry_name, ''), COALESCE(r.location_address, ''), r.observed_at
    FROM i2821_importer_closures_raw AS r
    JOIN company_registry_fetches AS f ON f.id = r.fetch_id
    JOIN company_registry_runs AS run ON run.id = f.run_id AND run.status = 'COMPLETED'
    UNION ALL
    SELECT 'EXCELLENT_IMPORTER', r.id, COALESCE(r.business_name, ''), COALESCE(r.license_no, ''),
           '', COALESCE(r.address, ''), r.observed_at
    FROM i0250_excellent_importers_raw AS r
    JOIN company_registry_fetches AS f ON f.id = r.fetch_id
    JOIN company_registry_runs AS run ON run.id = f.run_id AND run.status = 'COMPLETED'
    UNION ALL
    SELECT 'DISPOSITION', r.id, COALESCE(r.business_name, ''), COALESCE(r.license_no, ''),
           COALESCE(r.industry_name, ''), COALESCE(r.address, ''), r.observed_at
    FROM i0470_administrative_dispositions_raw AS r
    JOIN company_registry_fetches AS f ON f.id = r.fetch_id
    JOIN company_registry_runs AS run ON run.id = f.run_id AND run.status = 'COMPLETED'
), ranked_source_rows AS (
    SELECT source_type, source_row_id, business_name, license_number,
           CAST(business_name AS BINARY) AS business_name_key,
           CAST(license_number AS BINARY) AS license_number_key,
           industry_name, address, observed_at,
           ROW_NUMBER() OVER (
               PARTITION BY source_type, BINARY business_name, BINARY license_number
               ORDER BY observed_at DESC, source_row_id DESC
           ) AS source_rank
    FROM official_source_rows
    WHERE business_name <> ''
), companies AS (
    SELECT ANY_VALUE(business_name) AS business_name,
           ANY_VALUE(license_number) AS license_number,
           COALESCE(MAX(CASE WHEN source_type = 'BUSINESS_LICENSE' THEN industry_name END), MAX(industry_name), '') AS industry_name,
           COALESCE(MAX(CASE WHEN source_type = 'BUSINESS_LICENSE' THEN address END), MAX(address), '') AS address,
           DATE_FORMAT(MAX(observed_at), '%Y-%m-%d %H:%i:%s') AS latest_observed_at,
           MAX(source_type = 'BUSINESS_LICENSE') AS has_business_license,
           MAX(source_type = 'CLOSURE') AS has_closure,
           MAX(source_type = 'EXCELLENT_IMPORTER') AS has_excellent_importer,
           MAX(source_type = 'DISPOSITION') AS has_disposition
    FROM ranked_source_rows
    WHERE source_rank = 1
    GROUP BY business_name_key, license_number_key
) `

const officialDetailSQL = `WITH official_detail_rows AS (
    SELECT 'BUSINESS_LICENSE' AS source_type, r.id AS source_row_id,
           COALESCE(r.business_name, '') AS business_name, COALESCE(r.license_no, '') AS license_number,
           r.observed_at, CAST(r.raw_payload_json AS CHAR) AS payload_json, HEX(r.raw_payload_sha256) AS payload_hash
    FROM c001_importer_licenses_raw AS r
    JOIN company_registry_fetches AS f ON f.id = r.fetch_id
    JOIN company_registry_runs AS run ON run.id = f.run_id AND run.status = 'COMPLETED'
    UNION ALL
    SELECT 'CLOSURE', r.id, COALESCE(r.business_name, ''), COALESCE(r.license_no, ''),
           r.observed_at, CAST(r.raw_payload_json AS CHAR), HEX(r.raw_payload_sha256)
    FROM i2821_importer_closures_raw AS r
    JOIN company_registry_fetches AS f ON f.id = r.fetch_id
    JOIN company_registry_runs AS run ON run.id = f.run_id AND run.status = 'COMPLETED'
    UNION ALL
    SELECT 'EXCELLENT_IMPORTER', r.id, COALESCE(r.business_name, ''), COALESCE(r.license_no, ''),
           r.observed_at, CAST(r.raw_payload_json AS CHAR), HEX(r.raw_payload_sha256)
    FROM i0250_excellent_importers_raw AS r
    JOIN company_registry_fetches AS f ON f.id = r.fetch_id
    JOIN company_registry_runs AS run ON run.id = f.run_id AND run.status = 'COMPLETED'
    UNION ALL
    SELECT 'DISPOSITION', r.id, COALESCE(r.business_name, ''), COALESCE(r.license_no, ''),
           r.observed_at, CAST(r.raw_payload_json AS CHAR), HEX(r.raw_payload_sha256)
    FROM i0470_administrative_dispositions_raw AS r
    JOIN company_registry_fetches AS f ON f.id = r.fetch_id
    JOIN company_registry_runs AS run ON run.id = f.run_id AND run.status = 'COMPLETED'
), ranked_detail_rows AS (
    SELECT source_type, source_row_id, business_name, license_number, observed_at, payload_json, payload_hash,
           ROW_NUMBER() OVER (
               PARTITION BY source_type, payload_hash
               ORDER BY observed_at DESC, source_row_id DESC
           ) AS payload_rank
    FROM official_detail_rows
    WHERE BINARY business_name = BINARY ?
      AND (? = '' OR BINARY license_number = BINARY ?)
)
SELECT source_type, DATE_FORMAT(observed_at, '%Y-%m-%d %H:%i:%s'), payload_json
FROM ranked_detail_rows
WHERE payload_rank = 1
ORDER BY observed_at DESC, source_row_id DESC`

func (s *Server) companyRecords(w http.ResponseWriter, r *http.Request) {
	search := strings.TrimSpace(r.URL.Query().Get("q"))
	if len([]rune(search)) > 200 {
		writeError(w, http.StatusBadRequest, "search must be 200 characters or fewer")
		return
	}
	page, err := positiveInt(r.URL.Query().Get("page"), 1, 100000)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid page")
		return
	}
	pageSize, err := positiveInt(r.URL.Query().Get("page_size"), defaultPageSize, maxPageSize)
	if err != nil {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("page_size must be between 1 and %d", maxPageSize))
		return
	}
	where, args := companySearchWhere(search)
	countRows, err := s.queryer.QueryContext(r.Context(), officialSourceRowsSQL+"SELECT COUNT(*) FROM companies"+where, args...)
	if err != nil {
		writeDatabaseError(w, err)
		return
	}
	var total int64
	if err := scanSingle(countRows, &total); err != nil {
		writeDatabaseError(w, err)
		return
	}
	listArgs := append(append([]any{}, args...), pageSize, (page-1)*pageSize)
	rows, err := s.queryer.QueryContext(r.Context(), officialSourceRowsSQL+`SELECT business_name, license_number, industry_name, address, latest_observed_at,
        has_business_license, has_closure, has_excellent_importer, has_disposition
        FROM companies`+where+" ORDER BY latest_observed_at DESC, business_name, license_number LIMIT ? OFFSET ?", listArgs...)
	if err != nil {
		writeDatabaseError(w, err)
		return
	}
	companies, err := scanCompanyList(rows)
	if err != nil {
		writeDatabaseError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, companyListResponse{Companies: companies, Page: page, PageSize: pageSize, Total: total, TotalPages: pages(total, pageSize)})
}

func companySearchWhere(search string) (string, []any) {
	if search == "" {
		return "", nil
	}
	needle := "%" + search + "%"
	return " WHERE business_name LIKE ? OR license_number LIKE ?", []any{needle, needle}
}

func scanCompanyList(rows RowIterator) ([]companyListItem, error) {
	defer rows.Close()
	result := []companyListItem{}
	for rows.Next() {
		var value companyListItem
		var license, closure, excellent, disposition int64
		if err := rows.Scan(&value.BusinessName, &value.LicenseNumber, &value.IndustryName, &value.Address,
			&value.LatestObservedAt, &license, &closure, &excellent, &disposition); err != nil {
			return nil, err
		}
		value.HasBusinessLicense = license > 0
		value.HasClosure = closure > 0
		value.HasExcellent = excellent > 0
		value.HasDisposition = disposition > 0
		result = append(result, value)
	}
	return result, rows.Err()
}

func (s *Server) companyRecordDetail(w http.ResponseWriter, r *http.Request) {
	businessName := strings.TrimSpace(r.URL.Query().Get("business_name"))
	licenseNumber := strings.TrimSpace(r.URL.Query().Get("license_number"))
	if businessName == "" || len([]rune(businessName)) > 512 || len(licenseNumber) > 64 {
		writeError(w, http.StatusBadRequest, "invalid company key")
		return
	}
	records, err := s.companyRecordsByName(r.Context(), businessName, licenseNumber)
	if err != nil {
		writeDatabaseError(w, err)
		return
	}
	if len(records) == 0 {
		writeError(w, http.StatusNotFound, "company record not found")
		return
	}
	writeJSON(w, http.StatusOK, companyDetailResponse{BusinessName: businessName, LicenseNumber: licenseNumber, Records: records})
}

func (s *Server) companyRecordsByName(ctx context.Context, businessName, licenseNumber string) ([]officialRecord, error) {
	if businessName == "" {
		return []officialRecord{}, nil
	}
	rows, err := s.queryer.QueryContext(ctx, officialDetailSQL, businessName, licenseNumber, licenseNumber)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []officialRecord{}
	for rows.Next() {
		var sourceType, observedAt, payload string
		if err := rows.Scan(&sourceType, &observedAt, &payload); err != nil {
			return nil, err
		}
		fields, err := officialFields(sourceType, payload)
		if err != nil {
			return nil, err
		}
		result = append(result, officialRecord{SourceType: sourceType, SourceName: officialSourceName(sourceType), ObservedAt: observedAt, Fields: fields})
	}
	return result, rows.Err()
}

type officialFieldSpec struct{ key, label, hint string }

var officialFieldSpecs = map[string][]officialFieldSpec{
	"BUSINESS_LICENSE": {
		{"LCNS_NO", "인허가번호", "LCNS_NO"}, {"BSSH_NM", "업소명", "BSSH_NM"}, {"PRSDNT_NM", "대표자명", "PRSDNT_NM"},
		{"INDUTY_NM", "업종", "INDUTY_NM"}, {"PRMS_DT", "허가일자", "PRMS_DT"}, {"LOCP_ADDR", "주소", "LOCP_ADDR"},
		{"INSTT_NM", "관할 기관", "INSTT_NM"}, {"TELNO", "전화번호", "TELNO"},
	},
	"CLOSURE": {
		{"LCNS_NO", "인허가번호", "LCNS_NO"}, {"BSSH_NM", "업소명", "BSSH_NM"}, {"CLSBIZ_DT", "폐업일자", "CLSBIZ_DT"},
		{"CLSBIZ_DVS_CD_NM", "폐업상태", "CLSBIZ_DVS_CD_NM"}, {"INDUTY_NM", "업종", "INDUTY_NM"},
		{"PRSDNT_NM", "대표자명", "PRSDNT_NM"}, {"PRMS_DT", "허가일자", "PRMS_DT"}, {"LOCP_ADDR", "주소", "LOCP_ADDR"},
		{"INSTT_NM", "관할 기관", "INSTT_NM"},
	},
	"EXCELLENT_IMPORTER": {
		{"EXCLNC_INCM_BSSH_REGNO", "우수수입업소 등록번호", "EXCLNC_INCM_BSSH_REGNO"}, {"LCNS_NO", "인허가번호", "LCNS_NO"},
		{"BSSH_NM", "업소명", "BSSH_NM"}, {"PRMS_DT", "허가일자", "PRMS_DT"}, {"ADDR", "소재지", "ADDR"},
		{"EXCOURY_NATN_CD_NM", "수출국가", "EXCOURY_NATN_CD_NM"}, {"INCM_PRDT_XPORT_MC_NM", "수입제품 제조회사", "INCM_PRDT_XPORT_MC_NM"},
		{"PRDLST_CNT", "품목수", "PRDLST_CNT"}, {"PRDLST_NM", "품목명", "PRDLST_NM"},
	},
	"DISPOSITION": {
		{"DSPSDTLS_SEQ", "행정처분 전산키", "DSPSDTLS_SEQ"}, {"LCNS_NO", "인허가번호", "LCNS_NO"}, {"PRCSCITYPOINT_BSSHNM", "업소명", "PRCSCITYPOINT_BSSHNM"},
		{"INDUTY_CD_NM", "업종", "INDUTY_CD_NM"}, {"DSPS_DCSNDT", "처분확정일자", "DSPS_DCSNDT"}, {"DSPS_TYPECD_NM", "처분유형", "DSPS_TYPECD_NM"},
		{"VILTCN", "위반일자 및 내용", "VILTCN"}, {"DSPSCN", "처분내용", "DSPSCN"}, {"LAWORD_CD_NM", "위반법령", "LAWORD_CD_NM"},
		{"DSPS_BGNDT", "처분시작일", "DSPS_BGNDT"}, {"DSPS_ENDDT", "처분종료일", "DSPS_ENDDT"}, {"PUBLIC_DT", "공개기한", "PUBLIC_DT"},
		{"ADDR", "주소", "ADDR"}, {"TELNO", "전화번호", "TELNO"}, {"PRSDNT_NM", "대표자명", "PRSDNT_NM"},
		{"LAST_UPDT_DTM", "최종수정일", "LAST_UPDT_DTM"}, {"DSPS_INSTTCD_NM", "처분기관", "DSPS_INSTTCD_NM"},
	},
}

func officialFields(sourceType, payload string) ([]detailField, error) {
	var values map[string]any
	if err := json.Unmarshal([]byte(payload), &values); err != nil {
		return nil, fmt.Errorf("%s 원문 JSON 해석 실패: %w", sourceType, err)
	}
	specs, ok := officialFieldSpecs[sourceType]
	if !ok {
		return nil, errors.New("지원하지 않는 업체 공식정보 유형")
	}
	result := make([]detailField, 0, len(specs))
	for _, spec := range specs {
		value := strings.TrimSpace(fmt.Sprint(values[spec.key]))
		if value == "" || value == "<nil>" {
			continue
		}
		result = append(result, detailField{Label: spec.label, Hint: spec.hint, Value: value})
	}
	return result, nil
}

func officialSourceName(sourceType string) string {
	switch sourceType {
	case "BUSINESS_LICENSE":
		return "수입식품 영업신고 정보(C001)"
	case "CLOSURE":
		return "수입식품업 폐업정보(I2821)"
	case "EXCELLENT_IMPORTER":
		return "우수수입업소 현황(I0250)"
	case "DISPOSITION":
		return "행정처분 결과(I0470)"
	default:
		return sourceType
	}
}
