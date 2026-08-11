package mysql

import (
	"strings"
	"testing"

	"github.com/bottle-note/mfds-crawler/git.secrets/project/mfds/migrations"
)

func TestMigration00006_기존DefinerView를변경하지않는다(t *testing.T) {
	contents, err := migrations.FS.ReadFile("00006_add_declaration_variants.sql")
	if err != nil {
		t.Fatal(err)
	}
	sql := strings.ToUpper(string(contents))

	for _, forbidden := range []string{"DROP VIEW", "CREATE VIEW", "ALTER VIEW", "CREATE OR REPLACE VIEW"} {
		if strings.Contains(sql, forbidden) {
			t.Fatalf("00006 must not mutate declaration_details: found %q", forbidden)
		}
	}
	if strings.Count(sql, "ALTER TABLE DECLARATIONS") != 2 {
		t.Fatalf("00006 must contain only the Up/Down declarations ALTER statements: %s", sql)
	}
}

func TestMigration00007_Down이원상복구가아님을명시한다(t *testing.T) {
	contents, err := migrations.FS.ReadFile("00007_backfill_v3_normalization_state.sql")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(contents), "Down is intentionally irreversible") {
		t.Fatal("00007 must document that a successful Down does not restore overwritten data")
	}
}

func TestMigration00008_기준테이블과고정후보컬럼을한번에준비한다(t *testing.T) {
	contents, err := migrations.FS.ReadFile("00008_add_matching_references.sql")
	if err != nil {
		t.Fatal(err)
	}
	sql := strings.ToUpper(string(contents))

	for _, required := range []string{
		"CREATE TABLE DISTILLERIES", "CREATE TABLE REGIONS", "IDX_REGIONS_PARENT_ID",
		"MANUFACTURER_NAME",
		"DISTILLERY_CANDIDATE_1_ID", "DISTILLERY_CANDIDATE_2_ID", "DISTILLERY_CANDIDATE_3_ID",
		"SELECTED_DISTILLERY_ID",
		"REGION_CANDIDATE_1_ID", "REGION_CANDIDATE_2_ID", "REGION_CANDIDATE_3_ID",
		"SELECTED_REGION_ID", "MATCHING_VERSION", "MATCHED_AT",
		"UPDATE DECLARATIONS AS D",
		"JOIN ITEMS AS I ON I.ID = D.SOURCE_ITEM_ID",
		"SET D.MANUFACTURER_NAME = I.OVERSEAS_ESTABLISHMENT_NAME",
		"DROP TABLE IF EXISTS REGIONS", "DROP TABLE IF EXISTS DISTILLERIES",
	} {
		if !strings.Contains(sql, required) {
			t.Fatalf("00008 must contain %q", required)
		}
	}
	for _, forbidden := range []string{
		"FOREIGN KEY", "INSERT INTO DECLARATIONS", "CREATE TABLE DECLARATION_DISTILLERY_CANDIDATES",
		"CREATE TABLE DECLARATION_REGION_CANDIDATES",
		"DISTILLERY_NAME_KO ", "DISTILLERY_NAME_EN ", "DISTILLERY_DESCRIPTION",
		"REGION_NAME_KO ", "REGION_NAME_EN ", "REGION_DESCRIPTION",
	} {
		if strings.Contains(sql, forbidden) {
			t.Fatalf("00008 must not create candidate tables or name snapshots: found %q", forbidden)
		}
	}
	if strings.Count(sql, "ALTER TABLE DECLARATIONS") != 2 {
		t.Fatalf("00008 must contain one Up and one Down declarations ALTER: %s", sql)
	}
}

func TestMigration00009_원본Alcohol스키마를FK없이복제한다(t *testing.T) {
	contents, err := migrations.FS.ReadFile("00009_add_alcohol_references.sql")
	if err != nil {
		t.Fatal(err)
	}
	sql := strings.ToUpper(string(contents))

	for _, required := range []string{
		"CREATE TABLE ALCOHOLS", "ID                          BIGINT NOT NULL AUTO_INCREMENT",
		"REGION_ID", "DISTILLERY_ID", "DELETED_AT", "IDX_ALCOHOLS_REGION_ID", "IDX_ALCOHOLS_DISTILLERY_ID",
	} {
		if !strings.Contains(sql, required) {
			t.Fatalf("00009 must contain %q", required)
		}
	}
	for _, forbidden := range []string{"FOREIGN KEY", "SORT_ORDER"} {
		if strings.Contains(sql, forbidden) {
			t.Fatalf("00009 must preserve source orphan rows and columns: found %q", forbidden)
		}
	}
}

func TestMigration00010_알코올상위3개후보컬럼을추가한다(t *testing.T) {
	contents, err := migrations.FS.ReadFile("00010_add_alcohol_matching_candidates.sql")
	if err != nil {
		t.Fatal(err)
	}
	sql := strings.ToUpper(string(contents))
	for _, required := range []string{"ALCOHOL_CANDIDATE_1_ID", "ALCOHOL_CANDIDATE_2_ID", "ALCOHOL_CANDIDATE_3_ID", "SELECTED_ALCOHOL_ID"} {
		if !strings.Contains(sql, required) {
			t.Fatalf("00010 must contain %q", required)
		}
	}
}

func TestMigration00011_실행후보근거선택이력을분리해저장한다(t *testing.T) {
	contents, err := migrations.FS.ReadFile("00011_add_explainable_matching_records.sql")
	if err != nil {
		t.Fatal(err)
	}
	sql := strings.ToUpper(string(contents))
	for _, required := range []string{
		"CREATE TABLE MATCHING_RUNS", "CREATE TABLE ALCOHOL_MATCH_RECORDS",
		"CREATE TABLE REFERENCE_MATCH_RECORDS", "CREATE TABLE MATCHING_CANDIDATES",
		"CREATE TABLE MATCHING_EVIDENCE", "CREATE TABLE MATCHING_SELECTIONS",
		"CREATE TABLE REFERENCE_ALIASES", "UPSTREAM_TARGET_ID", "MATCHING_RUN_ID",
	} {
		if !strings.Contains(sql, required) {
			t.Fatalf("00011 must contain %q", required)
		}
	}
	if strings.Contains(sql, "DROP TABLE DECLARATIONS") {
		t.Fatal("00011 must not drop the declaration ledger")
	}
}
