package dashboard

import "net/http"

const overviewSQL = `SELECT COUNT(*), COALESCE(SUM(normalization_status IN ('REVIEW_REQUIRED', 'PARTIAL', 'UNPARSED')), 0), COALESCE(SUM(source_item_id IS NOT NULL), 0), COALESCE(DATE_FORMAT(MAX(source_processed_date), '%Y-%m-%d'), '') FROM mfds_declaration_details`
const statusSQL = `SELECT normalization_status, COUNT(*) FROM mfds_declaration_details GROUP BY normalization_status ORDER BY normalization_status`
const coverageSQL = `SELECT field_name, populated, total FROM (SELECT 'normalized_name' AS field_name, SUM(COALESCE(sku_display_name_ko, sku_display_name_en, '') <> '') AS populated, COUNT(*) AS total FROM mfds_declaration_details UNION ALL SELECT 'volume', SUM(volume_ml IS NOT NULL), COUNT(*) FROM mfds_declaration_details UNION ALL SELECT 'abv', SUM(abv_percent IS NOT NULL), COUNT(*) FROM mfds_declaration_details UNION ALL SELECT 'ingredient_percent', SUM(declaration_v3.ingredient_percent_raw IS NOT NULL), COUNT(*) FROM ` + declarationV3SourceSQL + ` UNION ALL SELECT 'variant_marker', SUM(declaration_v3.variant_marker_raw IS NOT NULL), COUNT(*) FROM ` + declarationV3SourceSQL + ` UNION ALL SELECT 'age', SUM(age_years IS NOT NULL), COUNT(*) FROM mfds_declaration_details UNION ALL SELECT 'vintage', SUM(vintage_year IS NOT NULL), COUNT(*) FROM mfds_declaration_details UNION ALL SELECT 'importer', SUM(COALESCE(importer_base_name, '') <> ''), COUNT(*) FROM mfds_declaration_details) coverage ORDER BY field_name`

func (s *Server) overview(w http.ResponseWriter, r *http.Request) {
	rows, err := s.queryer.QueryContext(r.Context(), overviewSQL)
	if err != nil {
		writeDatabaseError(w, err)
		return
	}
	var response overviewResponse
	var reviewPopulation int64
	if err := scanSingle(rows, &response.TotalRCNO, &reviewPopulation, &response.PreservedRCNO, &response.LatestProcessedAt); err != nil {
		writeDatabaseError(w, err)
		return
	}
	response.LedgerPreservationRate, response.ReviewRequiredRatio = percent(response.PreservedRCNO, response.TotalRCNO), percent(reviewPopulation, response.TotalRCNO)
	statusRows, err := s.queryer.QueryContext(r.Context(), statusSQL)
	if err != nil {
		writeDatabaseError(w, err)
		return
	}
	statuses, err := scanStatusCounts(statusRows)
	if err != nil {
		writeDatabaseError(w, err)
		return
	}
	coverageRows, err := s.queryer.QueryContext(r.Context(), coverageSQL)
	if err != nil {
		writeDatabaseError(w, err)
		return
	}
	coverage, err := scanCoverage(coverageRows)
	if err != nil {
		writeDatabaseError(w, err)
		return
	}
	response.StatusCounts, response.FieldCoverage = map[string]int64{}, map[string]float64{}
	for _, status := range statuses {
		response.StatusCounts[status.Status] = status.Count
	}
	for _, field := range coverage {
		response.FieldCoverage[field.Field] = field.Percentage
	}
	response.NormalizedCount = response.StatusCounts["NORMALIZED"] + response.StatusCounts["PARTIAL"] + response.StatusCounts["REVIEW_REQUIRED"] + response.StatusCounts["UNPARSED"]
	response.ReviewRequiredCount = response.StatusCounts["REVIEW_REQUIRED"]
	writeJSON(w, http.StatusOK, response)
}

func (s *Server) filters(w http.ResponseWriter, r *http.Request) {
	queries := map[string]string{
		"statuses": `SELECT DISTINCT normalization_status FROM mfds_declaration_details ORDER BY normalization_status`, "item_names": `SELECT DISTINCT source_queried_item_name FROM mfds_declaration_details WHERE source_queried_item_name IS NOT NULL AND source_queried_item_name <> '' ORDER BY source_queried_item_name`, "importers": `SELECT DISTINCT COALESCE(importer_base_name, source_importer_name) FROM mfds_declaration_details WHERE COALESCE(importer_base_name, source_importer_name) IS NOT NULL AND COALESCE(importer_base_name, source_importer_name) <> '' ORDER BY 1`, "countries": `SELECT DISTINCT source_manufacture_country_name FROM mfds_declaration_details WHERE source_manufacture_country_name IS NOT NULL AND source_manufacture_country_name <> '' ORDER BY source_manufacture_country_name`, "reason_codes": `SELECT DISTINCT reasons.reason_code FROM mfds_declaration_details d CROSS JOIN JSON_TABLE(CAST(d.normalization_reasons AS JSON), '$[*]' COLUMNS(reason_code VARCHAR(128) PATH '$')) AS reasons ORDER BY reasons.reason_code`,
	}
	result := map[string][]string{}
	for key, query := range queries {
		rows, err := s.queryer.QueryContext(r.Context(), query)
		if err != nil {
			writeDatabaseError(w, err)
			return
		}
		values, err := scanStrings(rows)
		if err != nil {
			writeDatabaseError(w, err)
			return
		}
		result[key] = values
	}
	writeJSON(w, http.StatusOK, result)
}
