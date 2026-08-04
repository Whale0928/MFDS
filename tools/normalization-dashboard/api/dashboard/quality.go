package dashboard

import (
	"context"
	"net/http"
)

func (s *Server) quality(w http.ResponseWriter, r *http.Request) {
	monthly, err := s.countPoints(r.Context(), `SELECT DATE_FORMAT(source_processed_date, '%Y-%m'), COUNT(*) FROM declaration_details WHERE source_processed_date IS NOT NULL GROUP BY 1 ORDER BY 1`)
	if err != nil {
		writeDatabaseError(w, err)
		return
	}
	items, err := s.countPoints(r.Context(), `SELECT COALESCE(source_queried_item_name, '미분류'), COUNT(*) FROM declaration_details GROUP BY 1 ORDER BY 2 DESC, 1 LIMIT 20`)
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
	reasons, err := s.countPoints(r.Context(), `SELECT reasons.reason_code, COUNT(*) FROM declaration_details d CROSS JOIN JSON_TABLE(CAST(d.normalization_reasons AS JSON), '$[*]' COLUMNS(reason_code VARCHAR(128) PATH '$')) AS reasons WHERE d.normalization_status IN ('REVIEW_REQUIRED', 'PARTIAL', 'UNPARSED') GROUP BY 1 ORDER BY 2 DESC, 1 LIMIT 20`)
	if err != nil {
		writeDatabaseError(w, err)
		return
	}
	queries := []string{`SELECT COALESCE(source_queried_item_name, '미분류'), COUNT(*) FROM declaration_details WHERE normalization_status IN ('REVIEW_REQUIRED', 'PARTIAL', 'UNPARSED') GROUP BY 1 ORDER BY 2 DESC, 1 LIMIT 20`, `SELECT COALESCE(importer_base_name, source_importer_name, '미분류'), COUNT(*) FROM declaration_details WHERE normalization_status IN ('REVIEW_REQUIRED', 'PARTIAL', 'UNPARSED') GROUP BY 1 ORDER BY 2 DESC, 1 LIMIT 20`, `SELECT COALESCE(source_manufacture_country_name, '미분류'), COUNT(*) FROM declaration_details WHERE normalization_status IN ('REVIEW_REQUIRED', 'PARTIAL', 'UNPARSED') GROUP BY 1 ORDER BY 2 DESC, 1 LIMIT 20`, `SELECT normalization_status, COUNT(*) FROM declaration_details WHERE normalization_status IN ('REVIEW_REQUIRED', 'PARTIAL', 'UNPARSED') GROUP BY 1 ORDER BY 2 DESC, 1`}
	dists := make([][]countPoint, len(queries))
	for i, query := range queries {
		dists[i], err = s.countPoints(r.Context(), query)
		if err != nil {
			writeDatabaseError(w, err)
			return
		}
	}
	groups, err := s.candidateGroups(r.Context())
	if err != nil {
		writeDatabaseError(w, err)
		return
	}
	duplicates, err := s.duplicateObservations(r.Context())
	if err != nil {
		writeDatabaseError(w, err)
		return
	}
	coverageMap := map[string]float64{}
	for _, field := range coverage {
		coverageMap[field.Field] = field.Percentage
	}
	statusMap := map[string]int64{}
	for _, status := range statuses {
		statusMap[status.Status] = status.Count
	}
	months := make([]monthlyPoint, 0, len(monthly))
	for _, point := range monthly {
		months = append(months, monthlyPoint{Month: point.Name, Count: point.Count})
	}
	reasonPoints := make([]reasonPoint, 0, len(reasons))
	for _, point := range reasons {
		reasonPoints = append(reasonPoints, reasonPoint{Code: point.Name, Count: point.Count})
	}
	writeJSON(w, http.StatusOK, qualityResponse{MonthlyCollections: months, ItemDistribution: items, FieldCoverage: coverageMap, StatusDistribution: statusMap, ReviewReasons: reasonPoints, ReviewDistributions: reviewDistributions{Items: dists[0], Importers: dists[1], Countries: dists[2], Statuses: dists[3]}, SKUGroups: groups, DuplicateObservations: duplicates})
}
func (s *Server) countPoints(ctx context.Context, query string) ([]countPoint, error) {
	rows, err := s.queryer.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []countPoint{}
	for rows.Next() {
		var value countPoint
		if err := rows.Scan(&value.Name, &value.Count); err != nil {
			return nil, err
		}
		result = append(result, value)
	}
	return result, rows.Err()
}
func (s *Server) candidateGroups(ctx context.Context) ([]candidateGroup, error) {
	rows, err := s.queryer.QueryContext(ctx, `SELECT COALESCE(MIN(NULLIF(sku_display_name_ko, '')), MIN(NULLIF(base_product_name_ko, '')), '이름 미정'), COALESCE(unit_volume_ml, 0), COUNT(*), COUNT(DISTINCT COALESCE(importer_base_name, source_importer_name)) FROM declaration_details WHERE sku_candidate_key_sha256 IS NOT NULL GROUP BY sku_candidate_key_sha256, unit_volume_ml HAVING COUNT(*) > 1 ORDER BY 3 DESC, 1 LIMIT 20`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []candidateGroup{}
	for rows.Next() {
		var value candidateGroup
		if err := rows.Scan(&value.DisplayName, &value.UnitVolumeML, &value.Declarations, &value.Importers); err != nil {
			return nil, err
		}
		result = append(result, value)
	}
	return result, rows.Err()
}
func (s *Server) duplicateObservations(ctx context.Context) ([]duplicateObservation, error) {
	rows, err := s.queryer.QueryContext(ctx, `SELECT COALESCE(i.queried_item_name, '미분류'), COUNT(DISTINCT i.rcno), COUNT(*) FROM items i JOIN (SELECT rcno FROM items GROUP BY rcno HAVING COUNT(*) > 1) duplicated ON duplicated.rcno = i.rcno GROUP BY 1 ORDER BY 3 DESC, 1 LIMIT 20`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []duplicateObservation{}
	for rows.Next() {
		var value duplicateObservation
		if err := rows.Scan(&value.Name, &value.Declarations, &value.Observations); err != nil {
			return nil, err
		}
		result = append(result, value)
	}
	return result, rows.Err()
}
