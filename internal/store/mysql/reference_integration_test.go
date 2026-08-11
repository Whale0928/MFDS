package mysql

import (
	"context"
	"os"
	"testing"
)

func TestSyncReferences_sourceTargetHashMatches(t *testing.T) {
	sourceDSN := os.Getenv("REFERENCE_SOURCE_DSN")
	if sourceDSN == "" {
		t.Skip("REFERENCE_SOURCE_DSN이 설정되지 않았습니다")
	}
	store := integrationStore(t)
	result, err := store.SyncReferences(context.Background(), sourceDSN)
	if err != nil {
		t.Fatal(err)
	}
	if result.Regions.Count == 0 || result.Distilleries.Count == 0 || result.Alcohols.Count == 0 {
		t.Fatalf("reference sync returned an empty table: %+v", result)
	}
	if result.Regions.Hash == "" || result.Distilleries.Hash == "" || result.Alcohols.Hash == "" {
		t.Fatalf("reference sync returned an empty hash: %+v", result)
	}
}
