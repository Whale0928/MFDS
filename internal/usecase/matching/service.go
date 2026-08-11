// Package matching coordinates candidate backfill and dry-run validation.
package matching

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	domain "github.com/bottle-note/mfds-crawler/internal/matching"
)

// Source is one already-normalized declaration used for matching.
type Source struct {
	DeclarationID          int64
	RCNO                   string
	SourceItemID           int64
	NormalizationVersion   string
	NormalizedAt           time.Time
	MatchingVersion        string
	BaseProductNameKO      string
	BaseProductNameEN      string
	NameSearchKeyKO        string
	NameSearchKeyEN        string
	ABVPercent             *float64
	AgeRaw                 string
	AgeYears               *int
	CaskCandidate          string
	ManufactureCountryName string
}

// Query controls a fenced store read.
type Query struct {
	Version string
	Limit   int
	RCNO    string
	Force   bool
}

// Completion contains only machine-generated candidates. It has no selected IDs.
type Completion struct {
	Source    Source
	Result    domain.MatchResult
	Version   string
	MatchedAt time.Time
}

// Store persists candidate slots while preserving administrator selections.
type Store interface {
	ListMatchingSources(context.Context, Query) ([]Source, error)
	SaveMatchingResult(context.Context, Completion) error
	MatchingRemaining(context.Context, string) (int, error)
}

// Command selects a bounded backfill or a read-only validation pass.
type Command struct {
	Limit  int
	RCNO   string
	All    bool
	DryRun bool
	Force  bool
}

// Summary reports exact processed and candidate coverage counts.
type Summary struct {
	Processed         int
	DistilleryMatched int
	RegionMatched     int
	NoMatch           int
	Remaining         int
	MatchingVersion   string
}

type Options struct {
	DefaultLimit int
	Now          func() time.Time
}

type Service struct {
	store   Store
	matcher *domain.ReferenceSnapshot
	opts    Options
}

func NewService(store Store, matcher *domain.ReferenceSnapshot, opts Options) (*Service, error) {
	if store == nil || matcher == nil {
		return nil, errors.New("matching store와 reference snapshot이 필요합니다")
	}
	if opts.DefaultLimit <= 0 {
		return nil, errors.New("matching 기본 limit은 0보다 커야 합니다")
	}
	if opts.Now == nil {
		opts.Now = time.Now
	}
	if matcher.Version().String() == "" {
		return nil, errors.New("matching version이 필요합니다")
	}
	return &Service{store: store, matcher: matcher, opts: opts}, nil
}

func (s *Service) Execute(ctx context.Context, command Command) (Summary, error) {
	query, err := s.query(command)
	if err != nil {
		return Summary{}, err
	}
	summary := Summary{MatchingVersion: query.Version}
	sources, err := s.store.ListMatchingSources(ctx, query)
	if err != nil {
		return summary, fmt.Errorf("matching 대상 조회 실패: %w", err)
	}
	for _, source := range sources {
		result := s.matcher.Match(domain.Input{
			BaseNameKO: source.BaseProductNameKO, BaseNameEN: source.BaseProductNameEN,
			SearchNameKO: source.NameSearchKeyKO, SearchNameEN: source.NameSearchKeyEN,
			ABVPercent: source.ABVPercent, Age: source.AgeRaw, AgeYears: source.AgeYears,
			Cask: source.CaskCandidate, ManufactureCountry: source.ManufactureCountryName,
		})
		if !command.DryRun {
			if err := s.store.SaveMatchingResult(ctx, Completion{
				Source: source, Result: result, Version: query.Version, MatchedAt: s.opts.Now(),
			}); err != nil {
				return summary, fmt.Errorf("rcno=%s matching 저장 실패: %w", source.RCNO, err)
			}
		}
		summary.Processed++
		if len(result.Distilleries) > 0 {
			summary.DistilleryMatched++
		}
		if len(result.Regions) > 0 {
			summary.RegionMatched++
		}
		if len(result.Distilleries) == 0 && len(result.Regions) == 0 {
			summary.NoMatch++
		}
	}
	summary.Remaining, err = s.store.MatchingRemaining(ctx, query.Version)
	if err != nil {
		return summary, fmt.Errorf("남은 matching 대상 조회 실패: %w", err)
	}
	return summary, nil
}

func (s *Service) query(command Command) (Query, error) {
	if command.Limit < 0 {
		return Query{}, errors.New("limit은 0보다 크거나 같아야 합니다")
	}
	rcno := strings.TrimSpace(command.RCNO)
	if rcno != "" && (command.All || command.Limit > 0) {
		return Query{}, errors.New("rcno는 all 또는 limit과 함께 사용할 수 없습니다")
	}
	if command.All && command.Limit > 0 {
		return Query{}, errors.New("all과 limit은 함께 사용할 수 없습니다")
	}
	limit := command.Limit
	if rcno != "" {
		limit = 1
	} else if command.All {
		limit = 0
	} else if limit == 0 {
		limit = s.opts.DefaultLimit
	}
	return Query{
		Version: s.matcher.Version().String(), Limit: limit, RCNO: rcno,
		Force: command.Force || command.DryRun || rcno != "",
	}, nil
}
