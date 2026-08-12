package dashboard

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

const importerSourceName = "C001(수입식품 영업신고 정보)"

const importerCTE = `WITH latest_importers AS (
	SELECT r.*,
		ROW_NUMBER() OVER (PARTITION BY r.license_no ORDER BY r.observed_at DESC, r.id DESC) AS record_rank
	FROM mfds_c001_importer_licenses_raw AS r
	JOIN mfds_company_registry_fetches AS f ON f.id = r.fetch_id
	JOIN mfds_company_registry_runs AS run ON run.id = f.run_id AND run.status = 'COMPLETED'
	WHERE NULLIF(TRIM(r.license_no), '') IS NOT NULL
		AND r.industry_name = '수입식품등 수입판매업'
), latest_closures AS (
	SELECT r.*,
		ROW_NUMBER() OVER (PARTITION BY r.license_no ORDER BY r.closure_date_raw DESC, r.observed_at DESC, r.id DESC) AS record_rank
	FROM mfds_i2821_importer_closures_raw AS r
	JOIN mfds_company_registry_fetches AS f ON f.id = r.fetch_id
	JOIN mfds_company_registry_runs AS run ON run.id = f.run_id AND run.status = 'COMPLETED'
	WHERE NULLIF(TRIM(r.license_no), '') IS NOT NULL
)`

const importerFromSQL = ` FROM latest_importers AS i
LEFT JOIN latest_closures AS c ON c.license_no = i.license_no AND c.record_rank = 1
WHERE i.record_rank = 1`

const importerListSQL = importerCTE + `
SELECT TRIM(i.license_no),
	COALESCE(TRIM(i.business_name), ''),
	COALESCE(TRIM(i.president_name), ''),
	CASE WHEN i.permit_date_raw REGEXP '^[0-9]{8}$' THEN DATE_FORMAT(STR_TO_DATE(i.permit_date_raw, '%Y%m%d'), '%Y-%m-%d') ELSE COALESCE(TRIM(i.permit_date_raw), '') END,
	COALESCE(TRIM(i.institution_name), ''),
	COALESCE(TRIM(i.location_address), ''),
	COALESCE(TRIM(i.telephone_no), ''),
	COALESCE(TRIM(i.industry_name), ''),
	COALESCE(TRIM(c.closure_status_name), ''),
	CASE WHEN c.closure_date_raw REGEXP '^[0-9]{8}$' THEN DATE_FORMAT(STR_TO_DATE(c.closure_date_raw, '%Y%m%d'), '%Y-%m-%d') ELSE COALESCE(TRIM(c.closure_date_raw), '') END,
	DATE_FORMAT(i.observed_at, '%Y-%m-%dT%H:%i:%s')` + importerFromSQL

type importerFilter struct {
	Search string
}

func (s *Server) importers(w http.ResponseWriter, r *http.Request) {
	filter, page, pageSize, err := parseImporterListRequest(r.URL.Query())
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	where, args := filter.where()
	countRows, err := s.queryer.QueryContext(r.Context(), importerCTE+" SELECT COUNT(*)"+importerFromSQL+where, args...)
	if err != nil {
		writeDatabaseError(w, err)
		return
	}
	var total int64
	if err := scanSingle(countRows, &total); err != nil {
		writeDatabaseError(w, err)
		return
	}
	args = append(args, pageSize, (page-1)*pageSize)
	rows, err := s.queryer.QueryContext(r.Context(), importerListSQL+where+" ORDER BY i.business_name, i.license_no LIMIT ? OFFSET ?", args...)
	if err != nil {
		writeDatabaseError(w, err)
		return
	}
	items, err := scanImporterList(rows)
	if err != nil {
		writeDatabaseError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, importerListResponse{Importers: items, Page: page, PageSize: pageSize, Total: total, TotalPages: pages(total, pageSize)})
}

func parseImporterListRequest(values url.Values) (importerFilter, int, int, error) {
	filter := importerFilter{
		Search: strings.TrimSpace(values.Get("q")),
	}
	if len([]rune(filter.Search)) > 200 {
		return importerFilter{}, 0, 0, errors.New("q must be 200 characters or fewer")
	}
	page, err := positiveInt(values.Get("page"), 1, 100000)
	if err != nil {
		return importerFilter{}, 0, 0, errors.New("invalid page")
	}
	pageSize, err := positiveInt(values.Get("page_size"), defaultPageSize, maxPageSize)
	if err != nil {
		return importerFilter{}, 0, 0, fmt.Errorf("page_size must be between 1 and %d", maxPageSize)
	}
	return filter, page, pageSize, nil
}

func (f importerFilter) where() (string, []any) {
	clauses, args := []string{}, []any{}
	if f.Search != "" {
		clauses = append(clauses, "(i.business_name LIKE ? OR i.license_no LIKE ? OR i.location_address LIKE ?)")
		needle := "%" + f.Search + "%"
		args = append(args, needle, needle, needle)
	}
	if len(clauses) == 0 {
		return "", args
	}
	return " AND " + strings.Join(clauses, " AND "), args
}

func scanImporterList(rows RowIterator) ([]importerListItem, error) {
	defer rows.Close()
	result := []importerListItem{}
	for rows.Next() {
		var value importerListItem
		if err := rows.Scan(&value.LicenseNo, &value.BusinessName, &value.Representative, &value.PermitDate, &value.InstitutionName, &value.Address, &value.Telephone, &value.IndustryName, &value.ClosureStatusName, &value.ClosureDate, &value.ObservedAt); err != nil {
			return nil, err
		}
		value.Source = importerSourceName
		result = append(result, value)
	}
	return result, rows.Err()
}
