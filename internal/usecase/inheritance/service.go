package inheritance

import (
	"context"
	"errors"
	"fmt"
)

// Store reads one snapshot and applies fenced writes. Every write re-checks the state the plan was built from.
type Store interface {
	LoadInheritanceRows(context.Context) ([]Row, error)
	LoadInheritanceAlcohols(context.Context) (map[int64]Alcohol, error)
	// ApplyInheritanceGroup writes one group in a transaction after confirming every seed still holds the group's alcohol.
	ApplyInheritanceGroup(context.Context, Group) (GroupResult, error)
	// ReleaseInheritance restores the latest matcher decision of one INHERITED row that no longer has a valid seed.
	ReleaseInheritance(context.Context, Action) (bool, error)
}

// GroupResult counts rows the store actually changed. Skipped rows, or a whole group whose seeds changed, were changed
// by someone else after the read.
type GroupResult struct {
	Filled  int
	Updated int
	Skipped int
}

// Summary counts rows inherited or released, or in a dry run the rows that would be.
// Conflicts are identity groups whose seeds chose different alcohols and were left alone.
type Summary struct {
	Inherited int
	Released  int
	Conflicts int
	Skipped   int
}

type Service struct {
	store Store
}

func NewService(store Store) (*Service, error) {
	if store == nil {
		return nil, errors.New("inheritance store가 필요합니다")
	}
	return &Service{store: store}, nil
}

// Execute plans from one snapshot and, unless dryRun, applies the plan with fenced writes.
func (s *Service) Execute(ctx context.Context, dryRun bool) (Summary, error) {
	rows, err := s.store.LoadInheritanceRows(ctx)
	if err != nil {
		return Summary{}, fmt.Errorf("상속 대상 조회 실패: %w", err)
	}
	alcohols, err := s.store.LoadInheritanceAlcohols(ctx)
	if err != nil {
		return Summary{}, fmt.Errorf("상속 기준 알코올 조회 실패: %w", err)
	}
	plan := BuildPlan(rows, alcohols)
	summary := Summary{Conflicts: len(plan.Conflicts)}
	if dryRun {
		summary.Inherited = plan.Fills() + plan.Updates()
		summary.Released = len(plan.Releases)
		return summary, nil
	}
	for _, group := range plan.Groups {
		result, err := s.store.ApplyInheritanceGroup(ctx, group)
		if err != nil {
			return summary, fmt.Errorf("상속 그룹 저장 실패: %w", err)
		}
		summary.Inherited += result.Filled + result.Updated
		summary.Skipped += result.Skipped
	}
	for _, release := range plan.Releases {
		released, err := s.store.ReleaseInheritance(ctx, release)
		if err != nil {
			return summary, fmt.Errorf("rcno=%s 상속 해제 실패: %w", release.Current.RCNO, err)
		}
		if released {
			summary.Released++
		} else {
			summary.Skipped++
		}
	}
	return summary, nil
}
