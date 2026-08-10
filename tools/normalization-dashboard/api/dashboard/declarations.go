package dashboard

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

type declarationFilter struct {
	Search   string
	Status   string
	ItemName string
	Importer string
	Country  string
	Reason   string
}

func (s *Server) declarations(w http.ResponseWriter, r *http.Request) {
	filter, page, pageSize, err := parseListRequest(r.URL.Query())
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	where, args := filter.where()
	countRows, err := s.queryer.QueryContext(r.Context(), "SELECT COUNT(*) FROM declaration_details"+where, args...)
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
	rows, err := s.queryer.QueryContext(r.Context(), declarationListSQL+where+" ORDER BY source_processed_date DESC, source_item_id DESC LIMIT ? OFFSET ?", args...)
	if err != nil {
		writeDatabaseError(w, err)
		return
	}
	items, err := scanDeclarationList(rows)
	if err != nil {
		writeDatabaseError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, declarationListResponse{Declarations: items, Page: page, PageSize: pageSize, Total: total, TotalPages: pages(total, pageSize)})
}

const declarationV3SourceSQL = `declaration_details JOIN (SELECT id AS declaration_id, ingredient_percent_raw, ingredient_percent, variant_marker_raw, variant_marker_type, variant_marker_value FROM declarations) AS declaration_v3 ON declaration_v3.declaration_id = declaration_details.id`

const declarationListSQL = `SELECT rcno, COALESCE(source_product_name_ko, source_item_name, source_product_name_en, ''), COALESCE(sku_display_name_ko, sku_display_name_en, ''), COALESCE(base_product_name_ko, base_product_name_en, ''), COALESCE(CONCAT(unit_volume_ml, ' mL'), ''), CONCAT_WS(' · ', NULLIF(CONCAT(age_years, '년'), '년'), NULLIF(CAST(vintage_year AS CHAR), ''), NULLIF(CONCAT(abv_percent, '%'), '%'), NULLIF(CONCAT(proof_value, ' proof'), ' proof'), NULLIF(strength_type, ''), NULLIF(version_marker, ''), NULLIF(edition_name, ''), NULLIF(declaration_v3.variant_marker_raw, ''), NULLIF(declaration_v3.variant_marker_type, ''), NULLIF(material_code, ''), NULLIF(cask_number, ''), NULLIF(batch_number, '')), normalization_status, COALESCE(DATE_FORMAT(source_processed_date, '%Y-%m-%d'), ''), COALESCE(source_queried_item_name, source_product_division_name, ''), COALESCE(importer_base_name, source_importer_name, ''), COALESCE(source_manufacture_country_name, ''), CAST(normalization_reasons AS CHAR) FROM ` + declarationV3SourceSQL

func (f declarationFilter) where() (string, []any) {
	clauses, args := []string{}, []any{}
	if f.Search != "" {
		clauses = append(clauses, "(rcno LIKE ? OR COALESCE(source_product_name_ko, source_item_name, source_product_name_en, '') LIKE ? OR COALESCE(sku_display_name_ko, sku_display_name_en, base_product_name_ko, base_product_name_en, '') LIKE ?)")
		needle := "%" + f.Search + "%"
		args = append(args, needle, needle, needle)
	}
	for _, pair := range []struct{ value, column string }{{f.Status, "normalization_status"}, {f.ItemName, "source_queried_item_name"}, {f.Importer, "COALESCE(importer_base_name, source_importer_name)"}, {f.Country, "source_manufacture_country_name"}} {
		if pair.value != "" {
			clauses, args = append(clauses, pair.column+" = ?"), append(args, pair.value)
		}
	}
	if f.Reason != "" {
		clauses, args = append(clauses, "JSON_CONTAINS(normalization_reasons, JSON_QUOTE(?))"), append(args, f.Reason)
	}
	if len(clauses) == 0 {
		return "", args
	}
	return " WHERE " + strings.Join(clauses, " AND "), args
}
func parseListRequest(values url.Values) (declarationFilter, int, int, error) {
	search := strings.TrimSpace(values.Get("q"))
	if search == "" {
		search = strings.TrimSpace(values.Get("search"))
	}
	filter := declarationFilter{Search: search, Status: strings.TrimSpace(values.Get("status")), ItemName: strings.TrimSpace(values.Get("item_name")), Importer: strings.TrimSpace(values.Get("importer")), Country: strings.TrimSpace(values.Get("country")), Reason: strings.TrimSpace(values.Get("reason"))}
	if len([]rune(filter.Search)) > 200 {
		return declarationFilter{}, 0, 0, errors.New("search must be 200 characters or fewer")
	}
	page, err := positiveInt(values.Get("page"), 1, 100000)
	if err != nil {
		return declarationFilter{}, 0, 0, errors.New("invalid page")
	}
	size, err := positiveInt(values.Get("page_size"), defaultPageSize, maxPageSize)
	if err != nil {
		return declarationFilter{}, 0, 0, fmt.Errorf("page_size must be between 1 and %d", maxPageSize)
	}
	return filter, page, size, nil
}

func (s *Server) declaration(w http.ResponseWriter, r *http.Request) {
	rcno := r.PathValue("rcno")
	if rcno == "" || len(rcno) > 32 {
		writeError(w, http.StatusBadRequest, "invalid rcno")
		return
	}
	rows, err := s.queryer.QueryContext(r.Context(), declarationDetailSQL, rcno)
	if err != nil {
		writeDatabaseError(w, err)
		return
	}
	defer rows.Close()
	if !rows.Next() {
		if rows.Err() != nil {
			writeDatabaseError(w, rows.Err())
		} else {
			writeError(w, http.StatusNotFound, "declaration not found")
		}
		return
	}
	var detail declarationDetail
	var reasons, fragments string
	var volumeRaw, volume, abvRaw, abv, ingredientRaw, ingredient, ageRaw, age, vintageRaw, vintage, edition, variantRaw, variantType, variantValue, lot, batch, importer, establishment string
	if err := rows.Scan(&detail.RCNO, &detail.SourceName, &detail.SourceNameEnglish, &detail.NormalizedName, &detail.NormalizedNameEnglish, &detail.Status, &reasons, &fragments, &detail.ProcessedAt, &detail.ItemName, &detail.ImporterName, &detail.Country, &volumeRaw, &volume, &abvRaw, &abv, &ingredientRaw, &ingredient, &ageRaw, &age, &vintageRaw, &vintage, &edition, &variantRaw, &variantType, &variantValue, &lot, &batch, &importer, &establishment); err != nil {
		writeDatabaseError(w, err)
		return
	}
	detail.ReasonCodes = jsonList(reasons)
	detail.Evidence = normalizationEvidence(detail.ReasonCodes, jsonList(fragments), []evidenceItem{{Label: "용량", RawValue: volumeRaw, NormalizedValue: volume}, {Label: "도수", RawValue: abvRaw, NormalizedValue: abv}, {Label: "성분 비율", RawValue: ingredientRaw, NormalizedValue: ingredient}, {Label: "숙성", RawValue: ageRaw, NormalizedValue: age}, {Label: "빈티지", RawValue: vintageRaw, NormalizedValue: vintage}, {Label: "변형 마커", RawValue: variantRaw, NormalizedValue: strings.TrimSpace(variantType + " " + variantValue)}})
	detail.Fields = map[string]string{"volume": volume, "abv": abv, "ingredient_percent": ingredient, "age": age, "vintage": vintage, "edition": edition, "variant_marker_raw": variantRaw, "variant_marker_type": variantType, "variant_marker_value": variantValue, "lot": lot, "batch": batch, "importer_base_name": importer, "overseas_establishment": establishment}
	groups, err := s.detailGroups(r, rcno)
	if err != nil {
		writeDatabaseError(w, err)
		return
	}
	detail.Groups = groups
	writeJSON(w, http.StatusOK, detail)
}

// detailGroups는 detailColumns 정의대로 원장과 정제 값을 그룹별로 읽는다.
func (s *Server) detailGroups(r *http.Request, rcno string) ([]detailGroup, error) {
	expressions := make([]string, len(detailColumns))
	for index, column := range detailColumns {
		expressions[index] = column.Expr
	}
	rows, err := s.queryer.QueryContext(r.Context(), "SELECT "+strings.Join(expressions, ", ")+" FROM "+declarationV3SourceSQL+" WHERE rcno = ? LIMIT 1", rcno)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	if !rows.Next() {
		return nil, rows.Err()
	}
	values := make([]string, len(detailColumns))
	targets := make([]any, len(detailColumns))
	for index := range values {
		targets[index] = &values[index]
	}
	if err := rows.Scan(targets...); err != nil {
		return nil, err
	}
	groups := make([]detailGroup, 0, 8)
	index := map[string]int{}
	for position, column := range detailColumns {
		slot, seen := index[column.Group]
		if !seen {
			slot = len(groups)
			index[column.Group] = slot
			groups = append(groups, detailGroup{Title: column.Group, Side: groupSide(column.Group)})
		}
		groups[slot].Fields = append(groups[slot].Fields, detailField{Label: column.Label, Hint: column.Hint, Value: values[position]})
	}
	return groups, rows.Err()
}

const declarationDetailSQL = `SELECT rcno, COALESCE(source_product_name_ko, source_item_name, ''), COALESCE(source_product_name_en, ''), COALESCE(sku_display_name_ko, base_product_name_ko, ''), COALESCE(sku_display_name_en, base_product_name_en, ''), normalization_status, CAST(normalization_reasons AS CHAR), CAST(unparsed_fragments_json AS CHAR), COALESCE(DATE_FORMAT(source_processed_date, '%Y-%m-%d'), ''), COALESCE(source_queried_item_name, source_product_division_name, ''), COALESCE(importer_base_name, source_importer_name, ''), COALESCE(source_manufacture_country_name, ''), COALESCE(volume_raw, ''), COALESCE(CONCAT(volume_ml, ' mL'), ''), COALESCE(abv_raw, ''), COALESCE(CONCAT(abv_percent, '%'), ''), COALESCE(declaration_v3.ingredient_percent_raw, ''), COALESCE(CONCAT(declaration_v3.ingredient_percent, '%'), ''), COALESCE(age_raw, ''), COALESCE(CONCAT(age_years, ' years'), ''), COALESCE(vintage_raw, ''), COALESCE(CAST(vintage_year AS CHAR), ''), COALESCE(edition_name, version_marker, ''), COALESCE(declaration_v3.variant_marker_raw, ''), COALESCE(declaration_v3.variant_marker_type, ''), COALESCE(declaration_v3.variant_marker_value, ''), COALESCE(lot_number, ''), COALESCE(batch_number, ''), COALESCE(importer_base_name, ''), COALESCE(overseas_establishment_search_key, '') FROM ` + declarationV3SourceSQL + ` WHERE rcno = ? LIMIT 1`

func normalizationEvidence(reasons, fragments []string, fields []evidenceItem) []evidenceItem {
	evidence := make([]evidenceItem, 0, len(reasons)+len(fragments)+len(fields))
	for _, field := range fields {
		if field.RawValue != "" || field.NormalizedValue != "" {
			evidence = append(evidence, field)
		}
	}
	for _, fragment := range fragments {
		evidence = append(evidence, evidenceItem{Label: "미해석 값", RawValue: fragment})
	}
	for _, reason := range reasons {
		evidence = append(evidence, evidenceItem{Label: "검토 사유", ReasonCode: reason})
	}
	return evidence
}
