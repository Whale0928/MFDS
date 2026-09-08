package importerresolution

import (
	"context"
	"errors"
	"github.com/bottle-note/mfds-crawler/internal/source/mfdscompany"
	"time"
)

// Retry a whole group so incomplete candidate evidence is never saved.
func (s *Service) resolveGroupWithRetry(ctx context.Context, jobID uint64, group PendingGroup) ([]Resolution, error) {
	for attempt := 1; ; attempt++ {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		started := time.Now()
		result, err := s.resolveGroup(ctx, group)
		if err == nil {
			return result, nil
		}
		retry := ctx.Err() == nil && mfdscompany.IsRetryable(err) && attempt < s.maxAttempts
		var requestError *mfdscompany.RequestError
		endpoint := ""
		if errors.As(err, &requestError) {
			endpoint = requestError.Endpoint
		}
		s.logger.WarnContext(ctx, "importer_group_attempt_failed", "code", mfdscompany.ErrorCode(err), "job_id", jobID, "business_name", group.BusinessName, "rcno_count", len(group.Records), "endpoint", endpoint, "attempt", attempt, "max_attempts", s.maxAttempts, "duration_ms", time.Since(started).Milliseconds(), "retry", retry)
		if !retry {
			return nil, err
		}
		delay := s.retryDelays[min(attempt-1, len(s.retryDelays)-1)]
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil, ctx.Err()
		case <-timer.C:
		}
	}
}
