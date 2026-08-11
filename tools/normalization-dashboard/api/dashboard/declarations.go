package dashboard

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

type declarationFilter struct {
	Search          string
	Status          string
	ItemName        string
	Importer        string
	Country         string
	Reason          string
	AlcoholMatch    string
	DistilleryMatch string
	RegionMatch     string
	Sort            string
	Order           string
}

func (s *Server) declarations(w http.ResponseWriter, r *http.Request) {
	filter, page, pageSize, err := parseListRequest(r.URL.Query())
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	where, args := filter.where()
	countRows, err := s.queryer.QueryContext(r.Context(), "SELECT COUNT(*) FROM "+declarationV3SourceSQL+where, args...)
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
	rows, err := s.queryer.QueryContext(r.Context(), declarationListSQL+where+filter.orderBy()+" LIMIT ? OFFSET ?", args...)
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

const declarationV3SourceSQL = `mfds_declaration_details JOIN (SELECT id AS declaration_id, ingredient_percent_raw, ingredient_percent, variant_marker_raw, variant_marker_type, variant_marker_value, alcohol_candidate_1_id, alcohol_candidate_1_score, alcohol_candidate_2_id, alcohol_candidate_2_score, alcohol_candidate_3_id, alcohol_candidate_3_score, selected_alcohol_id, distillery_candidate_1_id, distillery_candidate_1_score, distillery_candidate_2_id, distillery_candidate_2_score, distillery_candidate_3_id, distillery_candidate_3_score, selected_distillery_id, region_candidate_1_id, region_candidate_1_score, region_candidate_2_id, region_candidate_2_score, region_candidate_3_id, region_candidate_3_score, selected_region_id, matching_version, matching_run_id, alcohol_match_decision, distillery_match_source, region_match_source, matched_at FROM mfds_declarations) AS declaration_v3 ON declaration_v3.declaration_id = mfds_declaration_details.id`

const declarationMatchingSourceSQL = declarationV3SourceSQL + `
LEFT JOIN alcohols AS alcohol_candidate_1 ON alcohol_candidate_1.id = declaration_v3.alcohol_candidate_1_id
LEFT JOIN alcohols AS alcohol_candidate_2 ON alcohol_candidate_2.id = declaration_v3.alcohol_candidate_2_id
LEFT JOIN alcohols AS alcohol_candidate_3 ON alcohol_candidate_3.id = declaration_v3.alcohol_candidate_3_id
LEFT JOIN distilleries AS distillery_candidate_1 ON distillery_candidate_1.id = declaration_v3.distillery_candidate_1_id
LEFT JOIN distilleries AS distillery_candidate_2 ON distillery_candidate_2.id = declaration_v3.distillery_candidate_2_id
LEFT JOIN distilleries AS distillery_candidate_3 ON distillery_candidate_3.id = declaration_v3.distillery_candidate_3_id
LEFT JOIN regions AS region_candidate_1 ON region_candidate_1.id = declaration_v3.region_candidate_1_id
LEFT JOIN regions AS region_candidate_2 ON region_candidate_2.id = declaration_v3.region_candidate_2_id
LEFT JOIN regions AS region_candidate_3 ON region_candidate_3.id = declaration_v3.region_candidate_3_id`

const declarationListSQL = `SELECT rcno, COALESCE(source_product_name_ko, source_item_name, source_product_name_en, ''), COALESCE(sku_display_name_ko, sku_display_name_en, ''), COALESCE(base_product_name_ko, base_product_name_en, ''), COALESCE(CONCAT(unit_volume_ml, ' mL'), ''), CONCAT_WS(' · ', NULLIF(CONCAT(age_years, '년'), '년'), NULLIF(CAST(vintage_year AS CHAR), ''), NULLIF(CONCAT(abv_percent, '%'), '%'), NULLIF(CONCAT(proof_value, ' proof'), ' proof'), NULLIF(strength_type, ''), NULLIF(version_marker, ''), NULLIF(edition_name, ''), NULLIF(declaration_v3.variant_marker_raw, ''), NULLIF(declaration_v3.variant_marker_type, ''), NULLIF(material_code, ''), NULLIF(cask_number, ''), NULLIF(batch_number, '')), normalization_status, COALESCE(DATE_FORMAT(source_processed_date, '%Y-%m-%d'), ''), COALESCE(source_queried_item_name, source_product_division_name, ''), COALESCE(importer_base_name, source_importer_name, ''), COALESCE(source_manufacture_country_name, ''), declaration_v3.alcohol_candidate_1_id IS NOT NULL, declaration_v3.distillery_candidate_1_id IS NOT NULL, declaration_v3.region_candidate_1_id IS NOT NULL, CAST(normalization_reasons AS CHAR) FROM ` + declarationV3SourceSQL

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
	for _, pair := range []struct{ value, column string }{
		{f.AlcoholMatch, "declaration_v3.alcohol_candidate_1_id"},
		{f.DistilleryMatch, "declaration_v3.distillery_candidate_1_id"},
		{f.RegionMatch, "declaration_v3.region_candidate_1_id"},
	} {
		switch pair.value {
		case "matched":
			clauses = append(clauses, pair.column+" IS NOT NULL")
		case "unmatched":
			clauses = append(clauses, pair.column+" IS NULL")
		}
	}
	if len(clauses) == 0 {
		return "", args
	}
	return " WHERE " + strings.Join(clauses, " AND "), args
}

func (f declarationFilter) orderBy() string {
	direction := strings.ToUpper(f.Order)
	if f.Sort == "processed_at" {
		return " ORDER BY source_processed_date " + direction + ", source_item_id DESC"
	}
	column := map[string]string{
		"alcohol":    "declaration_v3.alcohol_candidate_1_id IS NOT NULL",
		"distillery": "declaration_v3.distillery_candidate_1_id IS NOT NULL",
		"region":     "declaration_v3.region_candidate_1_id IS NOT NULL",
	}[f.Sort]
	return " ORDER BY (" + column + ") " + direction + ", source_processed_date DESC, source_item_id DESC"
}

func parseListRequest(values url.Values) (declarationFilter, int, int, error) {
	search := strings.TrimSpace(values.Get("q"))
	if search == "" {
		search = strings.TrimSpace(values.Get("search"))
	}
	filter := declarationFilter{
		Search:          search,
		Status:          strings.TrimSpace(values.Get("status")),
		ItemName:        strings.TrimSpace(values.Get("item_name")),
		Importer:        strings.TrimSpace(values.Get("importer")),
		Country:         strings.TrimSpace(values.Get("country")),
		Reason:          strings.TrimSpace(values.Get("reason")),
		AlcoholMatch:    strings.TrimSpace(values.Get("alcohol_match")),
		DistilleryMatch: strings.TrimSpace(values.Get("distillery_match")),
		RegionMatch:     strings.TrimSpace(values.Get("region_match")),
		Sort:            strings.TrimSpace(values.Get("sort")),
		Order:           strings.TrimSpace(values.Get("order")),
	}
	if len([]rune(filter.Search)) > 200 {
		return declarationFilter{}, 0, 0, errors.New("search must be 200 characters or fewer")
	}
	for _, value := range []string{filter.AlcoholMatch, filter.DistilleryMatch, filter.RegionMatch} {
		if value != "" && value != "matched" && value != "unmatched" {
			return declarationFilter{}, 0, 0, errors.New("match filter must be matched or unmatched")
		}
	}
	if filter.Sort == "" {
		filter.Sort = "processed_at"
	}
	if filter.Order == "" {
		filter.Order = "desc"
	}
	if filter.Sort != "processed_at" && filter.Sort != "alcohol" && filter.Sort != "distillery" && filter.Sort != "region" {
		return declarationFilter{}, 0, 0, errors.New("invalid sort")
	}
	if filter.Order != "asc" && filter.Order != "desc" {
		return declarationFilter{}, 0, 0, errors.New("invalid order")
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
	if runID := matchingRunID(groups); runID > 0 {
		detail.MatchingCandidates, err = s.matchingCandidateDetails(r, rcno, runID)
		if err != nil {
			writeDatabaseError(w, err)
			return
		}
	}
	writeJSON(w, http.StatusOK, detail)
}

func matchingRunID(groups []detailGroup) int64 {
	for _, group := range groups {
		if group.Title != "매칭 이력" {
			continue
		}
		for _, field := range group.Fields {
			if field.Label == "매칭 실행 ID" {
				value, _ := strconv.ParseInt(field.Value, 10, 64)
				return value
			}
		}
	}
	return 0
}

func (s *Server) matchingCandidateDetails(r *http.Request, rcno string, runID int64) ([]matchingCandidateDetail, error) {
	rows, err := s.queryer.QueryContext(r.Context(), `
		SELECT mc.id, mc.target_type, mc.target_id, mc.rank_no,
		       COALESCE(mc.target_name_ko, ''), COALESCE(mc.target_name_en, ''), mc.raw_score,
		       COALESCE(me.feature_code, ''), COALESCE(me.evidence_source, ''),
		       COALESCE(me.input_value, ''), COALESCE(me.reference_value, ''),
		       COALESCE(me.rule_code, ''), COALESCE(me.weight, 0),
		       COALESCE(me.upstream_target_id, 0)
		FROM mfds_matching_candidates AS mc
		JOIN mfds_declarations AS d ON d.id = mc.declaration_id
		LEFT JOIN mfds_matching_evidence AS me ON me.candidate_id = mc.id
		WHERE d.rcno = ? AND mc.run_id = ?
		ORDER BY FIELD(mc.target_type, 'ALCOHOL', 'DISTILLERY', 'REGION'), mc.rank_no, me.id
	`, rcno, runID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []matchingCandidateDetail
	index := make(map[int64]int)
	for rows.Next() {
		var candidateID int64
		var candidate matchingCandidateDetail
		var evidence matchingEvidenceDetail
		if err := rows.Scan(
			&candidateID, &candidate.TargetType, &candidate.TargetID, &candidate.Rank,
			&candidate.NameKO, &candidate.NameEN, &candidate.Score,
			&evidence.FeatureCode, &evidence.Source, &evidence.InputValue, &evidence.ReferenceValue,
			&evidence.RuleCode, &evidence.Weight, &evidence.UpstreamTargetID,
		); err != nil {
			return nil, err
		}
		position, exists := index[candidateID]
		if !exists {
			position = len(result)
			index[candidateID] = position
			candidate.Evidence = []matchingEvidenceDetail{}
			result = append(result, candidate)
		}
		if evidence.FeatureCode != "" {
			result[position].Evidence = append(result[position].Evidence, evidence)
		}
	}
	return result, rows.Err()
}

// detailGroups는 detailColumns 정의대로 원장과 정제 값을 그룹별로 읽는다.
func (s *Server) detailGroups(r *http.Request, rcno string) ([]detailGroup, error) {
	expressions := make([]string, len(detailColumns))
	for index, column := range detailColumns {
		expressions[index] = column.Expr
	}
	rows, err := s.queryer.QueryContext(r.Context(), "SELECT "+strings.Join(expressions, ", ")+" FROM "+declarationMatchingSourceSQL+" WHERE rcno = ? LIMIT 1", rcno)
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
