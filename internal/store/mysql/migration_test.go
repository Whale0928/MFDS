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
		"MFDS_JOBS", "MFDS_TASKS", "MFDS_FETCHES", "MFDS_ITEMS", "MFDS_DECLARATIONS",
		"MFDS_MATCHING_RUNS", "MFDS_ALCOHOL_MATCH_RECORDS", "MFDS_REFERENCE_MATCH_RECORDS",
		"MFDS_MATCHING_CANDIDATES", "MFDS_MATCHING_EVIDENCE", "MFDS_MATCHING_SELECTIONS",
		"MFDS_REFERENCE_ALIASES",
	} {
		if !strings.Contains(sql, "CREATE TABLE "+table) {
			t.Fatalf("V11 must create %s", table)
		}
	}
	for _, required := range []string{
		"CREATE VIEW MFDS_DECLARATION_DETAILS", "MANUFACTURER_NAME", "ALCOHOL_CANDIDATE_1_ID",
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

func TestFlywayV12_업체공식정보원장을추가한다(t *testing.T) {
	path := filepath.Join(
		"..", "..", "..", "git.environment-variables", "storage", "db", "migration",
		"V12__add_mfds_company_registry.sql",
	)
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	sql := strings.ToUpper(string(contents))

	for _, table := range []string{
		"MFDS_COMPANY_REGISTRY_RUNS", "MFDS_COMPANY_REGISTRY_FETCHES",
		"MFDS_C001_IMPORTER_LICENSES_RAW", "MFDS_I2821_IMPORTER_CLOSURES_RAW",
		"MFDS_I0250_EXCELLENT_IMPORTERS_RAW", "MFDS_I0470_ADMINISTRATIVE_DISPOSITIONS_RAW",
	} {
		if !strings.Contains(sql, "CREATE TABLE "+table) {
			t.Fatalf("V12 must create %s", table)
		}
	}
	if strings.Contains(sql, "+GOOSE") {
		t.Fatal("V12 must be a Flyway migration without Goose directives")
	}
}
