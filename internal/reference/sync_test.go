package reference

import (
	"database/sql"
	"testing"
	"time"
)

func TestHashRows_orderIndependent(t *testing.T) {
	if hashRows([]string{"b", "a"}) != hashRows([]string{"a", "b"}) {
		t.Fatal("hash must not depend on row order")
	}
}

func TestAlcoholLines_distinguishesNullZeroEmptyAndTimestamp(t *testing.T) {
	at := time.Date(2026, 8, 10, 1, 2, 3, 4, time.UTC)
	base := alcohol{ID: 1, KorName: "A", EngName: "B", Type: "T", KorCategory: "K", EngCategory: "E", CategoryGroup: "G"}
	empty := base
	empty.ABV = sql.NullString{Valid: true, String: ""}
	zero := base
	zero.RegionID = sql.NullInt64{Valid: true, Int64: 0}
	withTime := base
	withTime.CreateAt = sql.NullTime{Valid: true, Time: at}
	for name, value := range map[string]string{
		"null":  alcoholLines([]alcohol{base})[0],
		"empty": alcoholLines([]alcohol{empty})[0],
		"zero":  alcoholLines([]alcohol{zero})[0],
		"time":  alcoholLines([]alcohol{withTime})[0],
	} {
		if value == alcoholLines([]alcohol{base})[0] && name != "null" {
			t.Fatalf("%s must change the canonical row", name)
		}
	}
}
