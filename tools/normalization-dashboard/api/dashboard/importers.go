package dashboard

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
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
		AND NULLIF(TRIM(r.business_name), '') IS NOT NULL
		AND r.industry_name = '수입식품등 수입판매업'
), latest_closures AS (
	SELECT r.*,
		ROW_NUMBER() OVER (PARTITION BY r.license_no ORDER BY r.closure_date_raw DESC, r.observed_at DESC, r.id DESC) AS record_rank
	FROM mfds_i2821_importer_closures_raw AS r
	JOIN mfds_company_registry_fetches AS f ON f.id = r.fetch_id
	JOIN mfds_company_registry_runs AS run ON run.id = f.run_id AND run.status = 'COMPLETED'
	WHERE NULLIF(TRIM(r.license_no), '') IS NOT NULL
), current_importers AS (
	SELECT i.*,
		COALESCE(TRIM(c.closure_status_name), '') AS current_closure_status_name,
		CASE WHEN c.closure_date_raw REGEXP '^[0-9]{8}$' THEN DATE_FORMAT(STR_TO_DATE(c.closure_date_raw, '%Y%m%d'), '%Y-%m-%d') ELSE COALESCE(TRIM(c.closure_date_raw), '') END AS current_closure_date
	FROM latest_importers AS i
	LEFT JOIN latest_closures AS c ON c.license_no = i.license_no AND c.record_rank = 1
	WHERE i.record_rank = 1
), grouped_importers AS (
	SELECT CAST(TRIM(business_name) AS BINARY) AS business_key,
		MIN(TRIM(business_name)) AS business_name,
		COUNT(*) AS license_count,
		COUNT(DISTINCT NULLIF(TRIM(location_address), '')) AS address_count,
		COUNT(DISTINCT NULLIF(TRIM(institution_name), '')) AS institution_count,
		COALESCE(DATE_FORMAT(MIN(CASE WHEN permit_date_raw REGEXP '^[0-9]{8}$' THEN STR_TO_DATE(permit_date_raw, '%Y%m%d') END), '%Y-%m-%d'), '') AS first_permit_date,
		DATE_FORMAT(MAX(observed_at), '%Y-%m-%dT%H:%i:%s') AS observed_at
	FROM current_importers
	GROUP BY CAST(TRIM(business_name) AS BINARY)
), matched_importers AS (
	SELECT CAST(TRIM(source_importer_name) AS BINARY) AS business_key
	FROM mfds_declaration_details
	WHERE NULLIF(TRIM(source_importer_name), '') IS NOT NULL
	GROUP BY CAST(TRIM(source_importer_name) AS BINARY)
)`

const importerFromSQL = ` FROM current_importers AS i WHERE 1 = 1`

const importerListSQL = importerCTE + `
SELECT g.business_name, g.license_count, g.address_count, g.institution_count, g.first_permit_date, g.observed_at
FROM grouped_importers AS g WHERE 1 = 1`

const importerLicenseListSQL = importerCTE + `
SELECT TRIM(i.license_no),
	COALESCE(TRIM(i.business_name), ''),
	COALESCE(TRIM(i.president_name), ''),
	CASE WHEN i.permit_date_raw REGEXP '^[0-9]{8}$' THEN DATE_FORMAT(STR_TO_DATE(i.permit_date_raw, '%Y%m%d'), '%Y-%m-%d') ELSE COALESCE(TRIM(i.permit_date_raw), '') END,
	COALESCE(TRIM(i.institution_name), ''),
	COALESCE(TRIM(i.location_address), ''),
	COALESCE(TRIM(i.telephone_no), ''),
	COALESCE(TRIM(i.industry_name), ''),
	i.current_closure_status_name,
	i.current_closure_date,
	DATE_FORMAT(i.observed_at, '%Y-%m-%dT%H:%i:%s')` + importerFromSQL + `
	AND CAST(TRIM(i.business_name) AS BINARY) = CAST(? AS BINARY)
ORDER BY i.permit_date_raw, i.license_no`

const importerStatisticsSQL = `SELECT COUNT(*),
	COUNT(DISTINCT COALESCE(NULLIF(TRIM(base_product_name_ko), ''), NULLIF(TRIM(base_product_name_en), ''), NULLIF(TRIM(source_product_name_ko), ''), NULLIF(TRIM(source_product_name_en), ''), NULLIF(TRIM(source_item_name), ''))),
	COALESCE(DATE_FORMAT(MIN(source_processed_date), '%Y-%m-%d'), ''),
	COALESCE(DATE_FORMAT(MAX(source_processed_date), '%Y-%m-%d'), '')
FROM mfds_declaration_details
WHERE CAST(TRIM(source_importer_name) AS BINARY) = CAST(? AS BINARY)`

const importerProductNameSQL = `COALESCE(NULLIF(TRIM(base_product_name_ko), ''), NULLIF(TRIM(base_product_name_en), ''), NULLIF(TRIM(source_product_name_ko), ''), NULLIF(TRIM(source_product_name_en), ''), NULLIF(TRIM(source_item_name), ''), '이름 미정')`
const importerProductKeySQL = `CONCAT('name:', LOWER(SHA2(` + importerProductNameSQL + `, 256)))`

const importerProductGroupSQL = `SELECT ` + importerProductKeySQL + `,
	MIN(` + importerProductNameSQL + `),
	COUNT(*),
	COALESCE(DATE_FORMAT(MIN(source_processed_date), '%Y-%m-%d'), ''),
	COALESCE(DATE_FORMAT(MAX(source_processed_date), '%Y-%m-%d'), '')
FROM mfds_declaration_details
WHERE CAST(TRIM(source_importer_name) AS BINARY) = CAST(? AS BINARY)
GROUP BY ` + importerProductKeySQL

const importerLedgerListSQL = `SELECT rcno,
	COALESCE(TRIM(source_product_name_ko), TRIM(source_item_name), ''),
	COALESCE(TRIM(source_product_name_en), ''),
	COALESCE(DATE_FORMAT(source_processed_date, '%Y-%m-%d'), ''),
	COALESCE(TRIM(source_queried_item_name), TRIM(source_product_division_name), ''),
	COALESCE(TRIM(source_overseas_establishment_name), ''),
	COALESCE(TRIM(source_manufacture_country_name), ''),
	COALESCE(NULLIF(TRIM(volume_raw), ''), CASE WHEN unit_volume_ml IS NOT NULL THEN CONCAT(unit_volume_ml, ' mL') ELSE '' END),
	COALESCE(NULLIF(TRIM(abv_raw), ''), CASE WHEN abv_percent IS NOT NULL THEN CONCAT(abv_percent, '%') ELSE '' END)
FROM mfds_declaration_details
WHERE CAST(TRIM(source_importer_name) AS BINARY) = CAST(? AS BINARY)
	AND ` + importerProductKeySQL + ` = ?`

type importerFilter struct {
	Search      string
	MatchedOnly bool
}

func (s *Server) importers(w http.ResponseWriter, r *http.Request) {
	filter, page, pageSize, err := parseImporterListRequest(r.URL.Query())
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	where, args := filter.where()
	countRows, err := s.queryer.QueryContext(r.Context(), importerCTE+" SELECT COUNT(*) FROM grouped_importers AS g WHERE 1 = 1"+where, args...)
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
	rows, err := s.queryer.QueryContext(r.Context(), importerListSQL+where+" ORDER BY g.business_name LIMIT ? OFFSET ?", args...)
	if err != nil {
		writeDatabaseError(w, err)
		return
	}
	items, err := scanImporterGroups(rows)
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
	if raw := values.Get("matched_only"); raw != "" {
		matchedOnly, err := strconv.ParseBool(raw)
		if err != nil {
			return importerFilter{}, 0, 0, errors.New("matched_only must be true or false")
		}
		filter.MatchedOnly = matchedOnly
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
		clauses = append(clauses, "g.business_name LIKE ?")
		needle := "%" + f.Search + "%"
		args = append(args, needle)
	}
	if f.MatchedOnly {
		clauses = append(clauses, "EXISTS (SELECT 1 FROM matched_importers AS m WHERE m.business_key = g.business_key)")
	}
	if len(clauses) == 0 {
		return "", args
	}
	return " AND " + strings.Join(clauses, " AND "), args
}

func (s *Server) importerDetail(w http.ResponseWriter, r *http.Request) {
	businessName := strings.TrimSpace(r.URL.Query().Get("business_name"))
	if businessName == "" || len([]rune(businessName)) > 512 {
		writeError(w, http.StatusBadRequest, "business_name is required and must be 512 characters or fewer")
		return
	}
	rows, err := s.queryer.QueryContext(r.Context(), importerLicenseListSQL, businessName)
	if err != nil {
		writeDatabaseError(w, err)
		return
	}
	licenses, err := scanImporterList(rows)
	if err != nil {
		writeDatabaseError(w, err)
		return
	}
	if len(licenses) == 0 {
		writeError(w, http.StatusNotFound, "importer was not found")
		return
	}
	statistics, err := s.importerStatistics(r.Context(), businessName)
	if err != nil {
		writeDatabaseError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, importerDetailResponse{BusinessName: businessName, Licenses: licenses, Statistics: statistics})
}

func (s *Server) importerStatistics(ctx context.Context, businessName string) (importerStatistics, error) {
	rows, err := s.queryer.QueryContext(ctx, importerStatisticsSQL, businessName)
	if err != nil {
		return importerStatistics{}, err
	}
	var result importerStatistics
	if err := scanSingle(rows, &result.DeclarationCount, &result.ProductCount, &result.FirstImportDate, &result.LastImportDate); err != nil {
		return importerStatistics{}, err
	}
	return result, nil
}

func (s *Server) importerProducts(w http.ResponseWriter, r *http.Request) {
	businessName, page, pageSize, err := parseImporterLedgerRequest(r.URL.Query(), false)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	countRows, err := s.queryer.QueryContext(r.Context(), `SELECT COUNT(DISTINCT `+importerProductKeySQL+`) FROM mfds_declaration_details WHERE CAST(TRIM(source_importer_name) AS BINARY) = CAST(? AS BINARY)`, businessName)
	if err != nil {
		writeDatabaseError(w, err)
		return
	}
	var total int64
	if err := scanSingle(countRows, &total); err != nil {
		writeDatabaseError(w, err)
		return
	}
	rows, err := s.queryer.QueryContext(r.Context(), importerProductGroupSQL+` ORDER BY MAX(source_processed_date) DESC, MIN(`+importerProductNameSQL+`) LIMIT ? OFFSET ?`, businessName, pageSize, (page-1)*pageSize)
	if err != nil {
		writeDatabaseError(w, err)
		return
	}
	products, err := scanImporterProductGroups(rows)
	if err != nil {
		writeDatabaseError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, importerProductPage{Products: products, Page: page, PageSize: pageSize, Total: total, TotalPages: pages(total, pageSize)})
}

func (s *Server) importerProductDeclarations(w http.ResponseWriter, r *http.Request) {
	businessName, page, pageSize, err := parseImporterLedgerRequest(r.URL.Query(), true)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	productKey := strings.TrimSpace(r.URL.Query().Get("product_key"))
	if productKey == "" || len(productKey) > 80 || !strings.HasPrefix(productKey, "name:") {
		writeError(w, http.StatusBadRequest, "invalid product_key")
		return
	}
	countRows, err := s.queryer.QueryContext(r.Context(), `SELECT COUNT(*) FROM mfds_declaration_details WHERE CAST(TRIM(source_importer_name) AS BINARY) = CAST(? AS BINARY) AND `+importerProductKeySQL+` = ?`, businessName, productKey)
	if err != nil {
		writeDatabaseError(w, err)
		return
	}
	var total int64
	if err := scanSingle(countRows, &total); err != nil {
		writeDatabaseError(w, err)
		return
	}
	rows, err := s.queryer.QueryContext(r.Context(), importerLedgerListSQL+` ORDER BY source_processed_date DESC, source_item_id DESC LIMIT ? OFFSET ?`, businessName, productKey, pageSize, (page-1)*pageSize)
	if err != nil {
		writeDatabaseError(w, err)
		return
	}
	items, err := scanImporterLedgerItems(rows)
	if err != nil {
		writeDatabaseError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, importerLedgerPage{Declarations: items, Page: page, PageSize: pageSize, Total: total, TotalPages: pages(total, pageSize)})
}

func parseImporterLedgerRequest(values url.Values, declarationPage bool) (string, int, int, error) {
	businessName := strings.TrimSpace(values.Get("business_name"))
	if businessName == "" || len([]rune(businessName)) > 512 {
		return "", 0, 0, errors.New("business_name is required and must be 512 characters or fewer")
	}
	page, err := positiveInt(values.Get("page"), 1, 100000)
	if err != nil {
		return "", 0, 0, errors.New("invalid page")
	}
	defaultSize := 20
	if declarationPage {
		defaultSize = 10
	}
	pageSize, err := positiveInt(values.Get("page_size"), defaultSize, maxPageSize)
	if err != nil {
		return "", 0, 0, fmt.Errorf("page_size must be between 1 and %d", maxPageSize)
	}
	return businessName, page, pageSize, nil
}

func scanImporterProductGroups(rows RowIterator) ([]importerProductGroup, error) {
	defer rows.Close()
	result := []importerProductGroup{}
	for rows.Next() {
		var value importerProductGroup
		if err := rows.Scan(&value.ProductKey, &value.ProductName, &value.DeclarationCount, &value.FirstImportDate, &value.LastImportDate); err != nil {
			return nil, err
		}
		result = append(result, value)
	}
	return result, rows.Err()
}

func scanImporterLedgerItems(rows RowIterator) ([]importerLedgerItem, error) {
	defer rows.Close()
	result := []importerLedgerItem{}
	for rows.Next() {
		var value importerLedgerItem
		if err := rows.Scan(&value.RCNO, &value.SourceName, &value.SourceNameEnglish, &value.ProcessedAt, &value.ItemName, &value.ManufacturerName, &value.Country, &value.Volume, &value.ABV); err != nil {
			return nil, err
		}
		result = append(result, value)
	}
	return result, rows.Err()
}

func scanImporterGroups(rows RowIterator) ([]importerGroupListItem, error) {
	defer rows.Close()
	result := []importerGroupListItem{}
	for rows.Next() {
		var value importerGroupListItem
		if err := rows.Scan(&value.BusinessName, &value.LicenseCount, &value.AddressCount, &value.InstitutionCount, &value.FirstPermitDate, &value.ObservedAt); err != nil {
			return nil, err
		}
		value.Source = importerSourceName
		result = append(result, value)
	}
	return result, rows.Err()
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
