package mysql

import (
	"database/sql/driver"
	"testing"
)

type rowsAffectedResult int64

func (r rowsAffectedResult) LastInsertId() (int64, error) { return 0, nil }
func (r rowsAffectedResult) RowsAffected() (int64, error) { return int64(r), nil }

var _ driver.Result = rowsAffectedResult(0)

func TestRequireOne_영향행이1개_오류가없다(t *testing.T) {
	if err := requireOne(rowsAffectedResult(1), "test"); err != nil {
		t.Fatalf("requireOne() error = %v", err)
	}
}

func TestRequireLease_영향행이0개_LeaseLost오류를반환한다(t *testing.T) {
	if err := requireLease(rowsAffectedResult(0), "test"); err == nil {
		t.Fatal("requireLease() error = nil, want error")
	}
}
