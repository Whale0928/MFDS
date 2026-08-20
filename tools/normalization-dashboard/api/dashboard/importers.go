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

const importerBaseCTE = `WITH declaration_stats AS (
	SELECT importer_id,
		COUNT(*) AS declaration_count,
		COUNT(DISTINCT ` + importerProductNameSQL + `) AS product_count,
		COALESCE(DATE_FORMAT(MIN(source_processed_date), '%Y-%m-%d'), '') AS first_import_date,
		COALESCE(DATE_FORMAT(MAX(source_processed_date), '%Y-%m-%d'), '') AS last_import_date
	FROM mfds_declaration_details
	WHERE importer_id IS NOT NULL
	GROUP BY importer_id
), grouped_importers AS (
	SELECT id AS importer_id,
		official_business_code, license_no, business_name,
		COALESCE(TRIM(representative_name), '') AS representative_name,
		COALESCE(DATE_FORMAT(permit_date, '%Y-%m-%d'), '') AS permit_date,
		COALESCE(TRIM(institution_name), '') AS institution_name,
		COALESCE(TRIM(primary_address), '') AS primary_address,
		COALESCE(TRIM(telephone_no), '') AS telephone_no,
		COALESCE(TRIM(industry_name), '') AS industry_name,
		operating_status, COALESCE(source_detail_url, '') AS source_detail_url,
		COALESCE(description, '') AS description,
		COALESCE(admin_status, '') AS admin_status,
		DATE_FORMAT(source_observed_at, '%Y-%m-%dT%H:%i:%s') AS source_observed_at,
		DATE_FORMAT(updated_at, '%Y-%m-%dT%H:%i:%s') AS updated_at
	FROM mfds_importers
)`

const importerListSQL = importerBaseCTE + `
	SELECT g.importer_id, g.official_business_code, g.license_no, g.business_name,
		g.representative_name, g.permit_date, g.institution_name, g.primary_address,
		g.telephone_no, g.industry_name, g.operating_status, g.source_detail_url,
		g.description, g.admin_status, g.source_observed_at, g.updated_at,
		COALESCE(stats.declaration_count, 0), COALESCE(stats.product_count, 0),
		COALESCE(stats.first_import_date, ''), COALESCE(stats.last_import_date, '')
	FROM grouped_importers AS g
	LEFT JOIN declaration_stats AS stats ON stats.importer_id = g.importer_id
	WHERE 1 = 1`

const importerProfileSQL = `SELECT id, official_business_code, license_no, business_name,
	COALESCE(HEX(business_name_key_sha256), ''),
	COALESCE(TRIM(representative_name), ''), COALESCE(DATE_FORMAT(permit_date, '%Y-%m-%d'), ''),
	COALESCE(TRIM(institution_name), ''), COALESCE(TRIM(primary_address), ''),
	COALESCE(TRIM(telephone_no), ''), COALESCE(TRIM(industry_name), ''), operating_status,
	COALESCE(source_list_url, ''), COALESCE(source_detail_url, ''),
	COALESCE(HEX(source_list_sha256), ''), COALESCE(HEX(source_detail_sha256), ''),
	COALESCE(DATE_FORMAT(source_observed_at, '%Y-%m-%dT%H:%i:%s'), ''),
	COALESCE(description, ''), COALESCE(admin_note, ''), COALESCE(admin_status, ''),
	COALESCE(reviewed_by, ''), COALESCE(DATE_FORMAT(reviewed_at, '%Y-%m-%dT%H:%i:%s'), ''),
	COALESCE(DATE_FORMAT(created_at, '%Y-%m-%dT%H:%i:%s'), ''),
	COALESCE(DATE_FORMAT(updated_at, '%Y-%m-%dT%H:%i:%s'), '')
FROM mfds_importers
WHERE id = ?
LIMIT 1`

const importerStatisticsSQL = `SELECT COUNT(*),
	COUNT(DISTINCT ` + importerProductNameSQL + `),
	COALESCE(DATE_FORMAT(MIN(source_processed_date), '%Y-%m-%d'), ''),
	COALESCE(DATE_FORMAT(MAX(source_processed_date), '%Y-%m-%d'), '')
FROM mfds_declaration_details
WHERE importer_id = ?`

const importerProductNameSQL = `COALESCE(NULLIF(TRIM(base_product_name_ko), ''), NULLIF(TRIM(base_product_name_en), ''), NULLIF(TRIM(source_product_name_ko), ''), NULLIF(TRIM(source_product_name_en), ''), NULLIF(TRIM(source_item_name), ''), '이름 미정')`
const importerProductKeySQL = `CONCAT('name:', LOWER(SHA2(` + importerProductNameSQL + `, 256)))`

const importerProductGroupSQL = `SELECT ` + importerProductKeySQL + `,
	MIN(` + importerProductNameSQL + `), COUNT(*),
	COALESCE(DATE_FORMAT(MIN(source_processed_date), '%Y-%m-%d'), ''),
	COALESCE(DATE_FORMAT(MAX(source_processed_date), '%Y-%m-%d'), '')
FROM mfds_declaration_details
WHERE importer_id = ?
GROUP BY ` + importerProductKeySQL

const importerLedgerListSQL = `SELECT rcno,
	COALESCE(TRIM(source_product_name_ko), TRIM(source_item_name), ''),
	COALESCE(TRIM(source_product_name_en), ''),
	COALESCE(DATE_FORMAT(source_processed_date, '%Y-%m-%d'), ''),
	COALESCE(TRIM(source_queried_item_name), TRIM(source_product_division_name), ''),
	COALESCE(TRIM(source_overseas_establishment_name), ''),
	COALESCE(TRIM(source_manufacture_country_name), ''),
	COALESCE(NULLIF(TRIM(volume_raw), ''), CASE WHEN unit_volume_ml IS NOT NULL THEN CONCAT(unit_volume_ml, ' mL') ELSE '' END),
	COALESCE(NULLIF(TRIM(abv_raw), ''), CASE WHEN abv_percent IS NOT NULL THEN CONCAT(abv_percent, '%') ELSE '' END),
	COALESCE(importer_link_source, '')
FROM mfds_declaration_details
WHERE importer_id = ?
	AND ` + importerProductKeySQL + ` = ?`

type importerFilter struct {
	Search          string
	MatchedOnly     bool
	OperatingStatus string
	IndustryName    string
	Sort            string
	Order           string
}

func (s *Server) importers(w http.ResponseWriter, r *http.Request) {
	filter, page, pageSize, err := parseImporterListRequest(r.URL.Query())
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	countWhere, countArgs := filter.where(false)
	countRows, err := s.queryer.QueryContext(r.Context(), importerBaseCTE+" SELECT COUNT(*) FROM grouped_importers AS g WHERE 1 = 1"+countWhere, countArgs...)
	if err != nil {
		writeDatabaseError(w, err)
		return
	}
	var total int64
	if err := scanSingle(countRows, &total); err != nil {
		writeDatabaseError(w, err)
		return
	}
	listWhere, listArgs := filter.where(true)
	listArgs = append(listArgs, pageSize, (page-1)*pageSize)
	rows, err := s.queryer.QueryContext(r.Context(), importerListSQL+listWhere+filter.orderBy()+" LIMIT ? OFFSET ?", listArgs...)
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
		Search:          strings.TrimSpace(values.Get("q")),
		OperatingStatus: strings.TrimSpace(values.Get("operating_status")),
		IndustryName:    strings.TrimSpace(values.Get("industry_name")),
		Sort:            strings.TrimSpace(values.Get("sort")),
		Order:           strings.TrimSpace(values.Get("order")),
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
	if len([]rune(filter.OperatingStatus)) > 32 || len([]rune(filter.IndustryName)) > 255 {
		return importerFilter{}, 0, 0, errors.New("filter is too long")
	}
	if filter.Sort == "" {
		filter.Sort = "business_name"
	}
	if filter.Order == "" {
		filter.Order = "asc"
	}
	if filter.Sort != "business_name" && filter.Sort != "declarations" && filter.Sort != "last_import" && filter.Sort != "observed_at" {
		return importerFilter{}, 0, 0, errors.New("invalid sort")
	}
	if filter.Order != "asc" && filter.Order != "desc" {
		return importerFilter{}, 0, 0, errors.New("invalid order")
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

func (f importerFilter) where(withDeclarationStats bool) (string, []any) {
	clauses, args := []string{}, []any{}
	if f.Search != "" {
		clauses = append(clauses, "(g.business_name LIKE ? ESCAPE '=' OR g.official_business_code LIKE ? ESCAPE '=' OR g.license_no LIKE ? ESCAPE '=' OR g.representative_name LIKE ? ESCAPE '=' OR g.primary_address LIKE ? ESCAPE '=')")
		needle := containsLikePattern(f.Search)
		args = append(args, needle, needle, needle, needle, needle)
	}
	if f.MatchedOnly {
		if withDeclarationStats {
			clauses = append(clauses, "stats.importer_id IS NOT NULL")
		} else {
			clauses = append(clauses, "EXISTS (SELECT 1 FROM mfds_declaration_details AS matched_declaration WHERE matched_declaration.importer_id = g.importer_id)")
		}
	}
	if f.OperatingStatus != "" {
		clauses, args = append(clauses, "g.operating_status = ?"), append(args, f.OperatingStatus)
	}
	if f.IndustryName != "" {
		clauses, args = append(clauses, "g.industry_name = ?"), append(args, f.IndustryName)
	}
	if len(clauses) == 0 {
		return "", args
	}
	return " AND " + strings.Join(clauses, " AND "), args
}

func (f importerFilter) orderBy() string {
	column := map[string]string{
		"business_name": "g.business_name",
		"declarations":  "COALESCE(stats.declaration_count, 0)",
		"last_import":   "COALESCE(stats.last_import_date, '')",
		"observed_at":   "g.source_observed_at",
	}[f.Sort]
	return " ORDER BY " + column + " " + strings.ToUpper(f.Order) + ", g.importer_id ASC"
}

func (s *Server) importerDetail(w http.ResponseWriter, r *http.Request) {
	importerID, err := parseImporterID(r.URL.Query())
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	rows, err := s.queryer.QueryContext(r.Context(), importerProfileSQL, importerID)
	if err != nil {
		writeDatabaseError(w, err)
		return
	}
	defer rows.Close()
	var profile importerProfile
	if !rows.Next() {
		if err := rows.Err(); err != nil {
			writeDatabaseError(w, err)
			return
		}
		writeError(w, http.StatusNotFound, "importer was not found")
		return
	}
	if err := rows.Scan(&profile.ImporterID, &profile.OfficialBusinessCode, &profile.LicenseNo, &profile.BusinessName, &profile.BusinessNameKeySHA256, &profile.Representative, &profile.PermitDate, &profile.InstitutionName, &profile.PrimaryAddress, &profile.Telephone, &profile.IndustryName, &profile.OperatingStatus, &profile.SourceListURL, &profile.SourceDetailURL, &profile.SourceListSHA256, &profile.SourceDetailSHA256, &profile.SourceObservedAt, &profile.Description, &profile.AdminNote, &profile.AdminStatus, &profile.ReviewedBy, &profile.ReviewedAt, &profile.CreatedAt, &profile.UpdatedAt); err != nil {
		writeDatabaseError(w, err)
		return
	}
	statistics, err := s.importerStatistics(r.Context(), importerID)
	if err != nil {
		writeDatabaseError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, importerDetailResponse{ImporterID: profile.ImporterID, BusinessName: profile.BusinessName, Profile: profile, Statistics: statistics})
}

func (s *Server) importerStatistics(ctx context.Context, importerID int64) (importerStatistics, error) {
	rows, err := s.queryer.QueryContext(ctx, importerStatisticsSQL, importerID)
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
	importerID, page, pageSize, err := parseImporterLedgerRequest(r.URL.Query(), false)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	countRows, err := s.queryer.QueryContext(r.Context(), `SELECT EXISTS(SELECT 1 FROM mfds_importers WHERE id = ?), COUNT(DISTINCT `+importerProductKeySQL+`) FROM mfds_declaration_details WHERE importer_id = ?`, importerID, importerID)
	if err != nil {
		writeDatabaseError(w, err)
		return
	}
	var exists bool
	var total int64
	if err := scanSingle(countRows, &exists, &total); err != nil {
		writeDatabaseError(w, err)
		return
	}
	if !exists {
		writeError(w, http.StatusNotFound, "importer was not found")
		return
	}
	rows, err := s.queryer.QueryContext(r.Context(), importerProductGroupSQL+` ORDER BY MAX(source_processed_date) DESC, MIN(`+importerProductNameSQL+`), `+importerProductKeySQL+` LIMIT ? OFFSET ?`, importerID, pageSize, (page-1)*pageSize)
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
	importerID, page, pageSize, err := parseImporterLedgerRequest(r.URL.Query(), true)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	productKey := strings.TrimSpace(r.URL.Query().Get("product_key"))
	if productKey == "" || len(productKey) > 80 || !strings.HasPrefix(productKey, "name:") {
		writeError(w, http.StatusBadRequest, "invalid product_key")
		return
	}
	countRows, err := s.queryer.QueryContext(r.Context(), `SELECT EXISTS(SELECT 1 FROM mfds_importers WHERE id = ?), COUNT(*) FROM mfds_declaration_details WHERE importer_id = ? AND `+importerProductKeySQL+` = ?`, importerID, importerID, productKey)
	if err != nil {
		writeDatabaseError(w, err)
		return
	}
	var exists bool
	var total int64
	if err := scanSingle(countRows, &exists, &total); err != nil {
		writeDatabaseError(w, err)
		return
	}
	if !exists {
		writeError(w, http.StatusNotFound, "importer was not found")
		return
	}
	rows, err := s.queryer.QueryContext(r.Context(), importerLedgerListSQL+` ORDER BY source_processed_date DESC, source_item_id DESC LIMIT ? OFFSET ?`, importerID, productKey, pageSize, (page-1)*pageSize)
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

func parseImporterLedgerRequest(values url.Values, declarationPage bool) (int64, int, int, error) {
	importerID, err := parseImporterID(values)
	if err != nil {
		return 0, 0, 0, err
	}
	page, err := positiveInt(values.Get("page"), 1, 100000)
	if err != nil {
		return 0, 0, 0, errors.New("invalid page")
	}
	defaultSize := 20
	if declarationPage {
		defaultSize = 10
	}
	pageSize, err := positiveInt(values.Get("page_size"), defaultSize, maxPageSize)
	if err != nil {
		return 0, 0, 0, fmt.Errorf("page_size must be between 1 and %d", maxPageSize)
	}
	return importerID, page, pageSize, nil
}

func parseImporterID(values url.Values) (int64, error) {
	importerID, err := strconv.ParseInt(strings.TrimSpace(values.Get("importer_id")), 10, 64)
	if err != nil || importerID <= 0 {
		return 0, errors.New("importer_id must be a positive integer")
	}
	return importerID, nil
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
		if err := rows.Scan(&value.RCNO, &value.SourceName, &value.SourceNameEnglish, &value.ProcessedAt, &value.ItemName, &value.ManufacturerName, &value.Country, &value.Volume, &value.ABV, &value.LinkSource); err != nil {
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
		if err := rows.Scan(&value.ImporterID, &value.OfficialBusinessCode, &value.LicenseNo, &value.BusinessName, &value.Representative, &value.PermitDate, &value.InstitutionName, &value.PrimaryAddress, &value.Telephone, &value.IndustryName, &value.OperatingStatus, &value.SourceDetailURL, &value.Description, &value.AdminStatus, &value.SourceObservedAt, &value.UpdatedAt, &value.DeclarationCount, &value.ProductCount, &value.FirstImportDate, &value.LastImportDate); err != nil {
			return nil, err
		}
		result = append(result, value)
	}
	return result, rows.Err()
}
