// Package identity fills product identity keys on declarations normalized before the key existed.
package identity

import (
	"context"
	"errors"
	"fmt"
	"time"

	parser "github.com/bottle-note/mfds-crawler/internal/normalization"
)

// batchSize bounds one fill transaction.
const batchSize = 500

// Source is one normalized declaration without a key and the stored fields that make up its product identity.
type Source struct {
	DeclarationID int64
	NormalizedAt  time.Time
	Identity      parser.ProductIdentity
}

// Update sets one key only if the row is still keyless and was not renormalized since it was read.
type Update struct {
	DeclarationID int64
	NormalizedAt  time.Time
	Key           string
}

// Store pages keyless declarations by ID and writes fenced key updates.
type Store interface {
	ListMissingIdentitySources(ctx context.Context, afterID int64, limit int) ([]Source, error)
	// SaveProductIdentityKeys applies one batch atomically and returns how many rows passed the fence.
	SaveProductIdentityKeys(ctx context.Context, updates []Update) (int, error)
}

// Summary counts keys written, or in a dry run the keys that would be written.
type Summary struct {
	Filled  int
	Skipped int
}

type Service struct {
	store Store
}

func NewService(store Store) (*Service, error) {
	if store == nil {
		return nil, errors.New("identity store가 필요합니다")
	}
	return &Service{store: store}, nil
}

// FillMissing uses the same key function as normalization, so a filled key equals a freshly normalized one.
func (s *Service) FillMissing(ctx context.Context, dryRun bool) (Summary, error) {
	summary := Summary{}
	var afterID int64
	for {
		sources, err := s.store.ListMissingIdentitySources(ctx, afterID, batchSize)
		if err != nil {
			return summary, fmt.Errorf("제품 동일성 키 대상 조회 실패: %w", err)
		}
		if len(sources) == 0 {
			return summary, nil
		}
		updates := make([]Update, 0, len(sources))
		for _, source := range sources {
			afterID = source.DeclarationID
			if key := parser.ProductIdentityKey(source.Identity); key != "" {
				updates = append(updates, Update{DeclarationID: source.DeclarationID, NormalizedAt: source.NormalizedAt, Key: key})
			}
		}
		if dryRun || len(updates) == 0 {
			summary.Filled += len(updates)
			continue
		}
		applied, err := s.store.SaveProductIdentityKeys(ctx, updates)
		if err != nil {
			return summary, fmt.Errorf("제품 동일성 키 저장 실패: %w", err)
		}
		summary.Filled += applied
		summary.Skipped += len(updates) - applied
	}
}
