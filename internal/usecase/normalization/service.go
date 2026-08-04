package normalization

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

type Options struct {
	RunLimit             int
	LeaseDuration        time.Duration
	MaxAttempts          int
	RetryDelay           time.Duration
	NormalizationVersion string
	Now                  func() time.Time
	Owner                string
}

type Command struct {
	Limit  int
	RCNO   string
	DryRun bool
}

type Summary struct {
	Processed        map[Status]int
	SystemFailures   int
	RemainingPending int
	RemainingStale   int
}

type Service struct {
	store  Store
	parser Parser
	opts   Options
}

func NewService(store Store, parser Parser, opts Options) (*Service, error) {
	if store == nil || parser == nil {
		return nil, errors.New("normalization store와 parser가 필요합니다")
	}
	if opts.RunLimit <= 0 || opts.LeaseDuration <= 0 || opts.MaxAttempts <= 0 || opts.RetryDelay <= 0 {
		return nil, errors.New("normalization 운영 설정은 모두 0보다 커야 합니다")
	}
	if strings.TrimSpace(opts.NormalizationVersion) == "" {
		opts.NormalizationVersion = Version
	}
	if opts.Now == nil {
		opts.Now = time.Now
	}
	if strings.TrimSpace(opts.Owner) == "" {
		return nil, errors.New("normalization claim owner가 필요합니다")
	}
	return &Service{store: store, parser: parser, opts: opts}, nil
}

func (s *Service) Execute(ctx context.Context, command Command) (Summary, error) {
	request, err := s.claimRequest(command)
	if err != nil {
		return Summary{}, err
	}

	summary := Summary{Processed: make(map[Status]int)}
	var sources []Source
	if command.DryRun {
		sources, err = s.store.Preview(ctx, request)
	} else {
		if err = s.store.SyncDeclarations(ctx); err == nil {
			sources, err = s.store.Claim(ctx, request)
		}
	}
	if err != nil {
		summary.SystemFailures++
		return summary, fmt.Errorf("normalization 작업 조회 실패: %w", err)
	}

	var systemErrors []error
	for _, source := range sources {
		parsed, parseErr := s.parser.Normalize(source)
		if parseErr != nil {
			summary.SystemFailures++
			systemErrors = append(systemErrors, fmt.Errorf("rcno=%s parser 실패: %w", source.RCNO, parseErr))
			if !command.DryRun {
				if failErr := s.store.Fail(ctx, Failure{
					Source: source, RetryAt: s.opts.Now().Add(s.opts.RetryDelay), LastError: parseErr.Error(),
				}); failErr != nil {
					summary.SystemFailures++
					systemErrors = append(systemErrors, fmt.Errorf("rcno=%s 실패 상태 저장 실패: %w", source.RCNO, failErr))
				}
			}
			continue
		}
		if !parsed.Status.IsTerminalDataStatus() {
			summary.SystemFailures++
			statusErr := fmt.Errorf("rcno=%s parser가 terminal data status를 반환하지 않았습니다: %s", source.RCNO, parsed.Status)
			systemErrors = append(systemErrors, statusErr)
			if !command.DryRun {
				if failErr := s.store.Fail(ctx, Failure{
					Source: source, RetryAt: s.opts.Now().Add(s.opts.RetryDelay), LastError: statusErr.Error(),
				}); failErr != nil {
					summary.SystemFailures++
					systemErrors = append(systemErrors, fmt.Errorf("rcno=%s 실패 상태 저장 실패: %w", source.RCNO, failErr))
				}
			}
			continue
		}

		if !command.DryRun {
			if completeErr := s.store.Complete(ctx, Completion{
				Source: source, Result: parsed, NormalizationVersion: s.opts.NormalizationVersion, NormalizedAt: s.opts.Now(),
			}); completeErr != nil {
				summary.SystemFailures++
				systemErrors = append(systemErrors, fmt.Errorf("rcno=%s 정제 결과 저장 실패: %w", source.RCNO, completeErr))
				continue
			}
		}
		summary.Processed[parsed.Status]++
	}

	remaining, remainingErr := s.store.Remaining(ctx)
	if remainingErr != nil {
		summary.SystemFailures++
		systemErrors = append(systemErrors, fmt.Errorf("남은 normalization 작업 조회 실패: %w", remainingErr))
	} else {
		summary.RemainingPending = remaining.Pending
		summary.RemainingStale = remaining.Stale
	}
	return summary, errors.Join(systemErrors...)
}

func (s *Service) claimRequest(command Command) (ClaimRequest, error) {
	if command.Limit < 0 {
		return ClaimRequest{}, errors.New("limit은 0보다 크거나 같아야 합니다")
	}
	if command.Limit > 0 && strings.TrimSpace(command.RCNO) != "" {
		return ClaimRequest{}, errors.New("limit과 rcno는 함께 사용할 수 없습니다")
	}
	limit := command.Limit
	if strings.TrimSpace(command.RCNO) != "" {
		limit = 1
	} else if limit == 0 {
		limit = s.opts.RunLimit
	}
	return ClaimRequest{
		Limit: limit, RCNO: strings.TrimSpace(command.RCNO), Owner: s.opts.Owner,
		LeaseDuration: s.opts.LeaseDuration, MaxAttempts: s.opts.MaxAttempts, RetryDelay: s.opts.RetryDelay,
	}, nil
}
