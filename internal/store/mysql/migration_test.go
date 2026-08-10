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
