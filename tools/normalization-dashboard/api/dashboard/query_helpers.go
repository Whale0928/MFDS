package dashboard

import (
	"encoding/json"
	"errors"
	"math"
	"strconv"
	"strings"
)

func scanStatusCounts(rows RowIterator) ([]statusCount, error) {
	defer rows.Close()
	result := []statusCount{}
	for rows.Next() {
		var value statusCount
		if err := rows.Scan(&value.Status, &value.Count); err != nil {
			return nil, err
		}
		result = append(result, value)
	}
	return result, rows.Err()
}
func scanCoverage(rows RowIterator) ([]fieldCoverage, error) {
	defer rows.Close()
	result := []fieldCoverage{}
	for rows.Next() {
		var value fieldCoverage
		if err := rows.Scan(&value.Field, &value.Populated, &value.Total); err != nil {
			return nil, err
		}
		value.Percentage = percent(value.Populated, value.Total)
		result = append(result, value)
	}
	return result, rows.Err()
}
func scanStrings(rows RowIterator) ([]string, error) {
	defer rows.Close()
	result := []string{}
	for rows.Next() {
		var value string
		if err := rows.Scan(&value); err != nil {
			return nil, err
		}
		result = append(result, value)
	}
	return result, rows.Err()
}
func scanSingle(rows RowIterator, dest ...any) error {
	defer rows.Close()
	if !rows.Next() {
		if rows.Err() != nil {
			return rows.Err()
		}
		return errors.New("query returned no row")
	}
	if err := rows.Scan(dest...); err != nil {
		return err
	}
	return rows.Err()
}
func scanDeclarationList(rows RowIterator) ([]declarationListItem, error) {
	defer rows.Close()
	result := []declarationListItem{}
	for rows.Next() {
		var value declarationListItem
		var reasons string
		if err := rows.Scan(&value.RCNO, &value.SourceName, &value.NormalizedName, &value.Key1, &value.Key2, &value.Key3, &value.Status, &value.ProcessedAt, &value.ItemName, &value.ImporterName, &value.SourceImporterName, &value.ImporterID, &value.ImporterBusinessName, &value.ImporterLinked, &value.ImporterLinkSource, &value.Country, &value.AlcoholMatched, &value.DistilleryMatched, &value.RegionMatched, &reasons); err != nil {
			return nil, err
		}
		value.ReasonCodes = jsonList(reasons)
		result = append(result, value)
	}
	return result, rows.Err()
}

func containsLikePattern(value string) string {
	escaped := strings.NewReplacer("=", "==", "%", "=%", "_", "=_").Replace(value)
	return "%" + escaped + "%"
}
func jsonList(value string) []string {
	var result []string
	if json.Unmarshal([]byte(value), &result) != nil {
		return []string{}
	}
	return result
}
func positiveInt(value string, fallback, maximum int) (int, error) {
	if value == "" {
		return fallback, nil
	}
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed < 1 || parsed > maximum {
		return 0, errors.New("invalid positive integer")
	}
	return parsed, nil
}
func percent(part, total int64) float64 {
	if total == 0 {
		return 0
	}
	return math.Round(float64(part)*10000/float64(total)) / 100
}
func pages(total int64, size int) int {
	if total == 0 {
		return 0
	}
	return int((total + int64(size) - 1) / int64(size))
}
