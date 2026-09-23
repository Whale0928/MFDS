package identity

import (
	"context"
	"testing"

	parser "github.com/bottle-note/mfds-crawler/internal/normalization"
)

type memoryStore struct {
	sources []Source
	saved   [][]Update
	reject  int64
}

func (m *memoryStore) ListMissingIdentitySources(_ context.Context, afterID int64, limit int) ([]Source, error) {
	result := []Source{}
	for _, source := range m.sources {
		if source.DeclarationID > afterID && len(result) < limit {
			result = append(result, source)
		}
	}
	return result, nil
}

func (m *memoryStore) SaveProductIdentityKeys(_ context.Context, updates []Update) (int, error) {
	m.saved = append(m.saved, updates)
	applied := 0
	for _, update := range updates {
		if update.DeclarationID != m.reject {
			applied++
		}
	}
	return applied, nil
}

func TestService_키없는행을채우고키를만들수없는행과펜스실패는제외한다(t *testing.T) {
	// Given
	identity := parser.ProductIdentity{NameSearchKeyKO: "글렌피딕", NameSearchKeyEN: "glenfiddich"}
	store := &memoryStore{reject: 2, sources: []Source{
		{DeclarationID: 1, Identity: identity},
		{DeclarationID: 2, Identity: identity},
		{DeclarationID: 3, Identity: parser.ProductIdentity{NameSearchKeyKO: "글렌피딕"}},
	}}
	service, err := NewService(store)
	if err != nil {
		t.Fatal(err)
	}

	// When
	summary, err := service.FillMissing(context.Background(), false)

	// Then
	if err != nil {
		t.Fatal(err)
	}
	if summary != (Summary{Filled: 1, Skipped: 1}) {
		t.Fatalf("summary = %+v", summary)
	}
	if len(store.saved) != 1 || len(store.saved[0]) != 2 || store.saved[0][0].Key != parser.ProductIdentityKey(identity) {
		t.Fatalf("saved = %+v", store.saved)
	}
}

func TestService_DryRun은저장하지않고채울수만센다(t *testing.T) {
	// Given
	identity := parser.ProductIdentity{NameSearchKeyKO: "글렌피딕", NameSearchKeyEN: "glenfiddich"}
	store := &memoryStore{sources: []Source{{DeclarationID: 1, Identity: identity}}}
	service, err := NewService(store)
	if err != nil {
		t.Fatal(err)
	}

	// When
	summary, err := service.FillMissing(context.Background(), true)

	// Then
	if err != nil || summary != (Summary{Filled: 1}) || len(store.saved) != 0 {
		t.Fatalf("summary = %+v, saved = %+v, error = %v", summary, store.saved, err)
	}
}
