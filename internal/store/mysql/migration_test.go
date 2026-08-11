package mysql

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFlywayV11_MFDS신규스키마만추가한다(t *testing.T) {
	path := filepath.Join(
		"..", "..", "..", "git.environment-variables", "storage", "db", "migration",
		"V11__add_mfds_import_ledger_and_matching.sql",
	)
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	sql := strings.ToUpper(string(contents))

	for _, table := range []string{
		"JOBS", "TASKS", "FETCHES", "ITEMS", "DECLARATIONS", "MATCHING_RUNS",
		"ALCOHOL_MATCH_RECORDS", "REFERENCE_MATCH_RECORDS", "MATCHING_CANDIDATES",
		"MATCHING_EVIDENCE", "MATCHING_SELECTIONS", "REFERENCE_ALIASES",
	} {
		if !strings.Contains(sql, "CREATE TABLE "+table) {
			t.Fatalf("V11 must create %s", table)
		}
	}
	for _, required := range []string{
		"CREATE VIEW DECLARATION_DETAILS", "MANUFACTURER_NAME", "ALCOHOL_CANDIDATE_1_ID",
		"DISTILLERY_CANDIDATE_1_ID", "REGION_CANDIDATE_1_ID", "MATCHING_RUN_ID",
		"UPSTREAM_TARGET_ID",
	} {
		if !strings.Contains(sql, required) {
			t.Fatalf("V11 must contain %q", required)
		}
	}
	for _, canonical := range []string{"ALCOHOLS", "DISTILLERIES", "REGIONS"} {
		if strings.Contains(sql, "CREATE TABLE "+canonical) {
			t.Fatalf("V11 must use the existing BottleNote table %s", canonical)
		}
	}
	if strings.Contains(sql, "+GOOSE") {
		t.Fatal("V11 must be a Flyway migration without Goose directives")
	}
}
