//go:build legacy

package mysql

import (
	"context"
	"fmt"
	"strings"

	"github.com/bottle-note/mfds-crawler/internal/usecase/weblist"
	"github.com/bottle-note/mfds-crawler/secrets/project/mfds/sqlcgen"
)

func (s *Store) FailTask(
	ctx context.Context,
	params weblist.TaskFailureParams,
) (terminal bool, err error) {
	params.ErrorMessage = weblist.SanitizeErrorMessage(params.ErrorMessage)
	err = s.withTx(ctx, func(q *sqlcgen.Queries) error {
		terminal = false
		if _, fetchErr := executeFailureFetchAttempt(
			params.FetchID,
			params.PageWasRaw,
			func() (uint64, error) {
				return storeFetch(ctx, q, weblist.JournalFetchParams{
					RunID: params.RunID, PartitionID: params.PartitionID, PageID: params.PageID,
					AttemptNo: params.PageToken, PartitionAttempt: params.PartitionToken,
					Owner: params.Owner, Fetch: params.Fetch,
					ErrorKind: params.ErrorKind, ErrorMessage: params.ErrorMessage,
				}, string(CrawlFetchStatusFailed))
			},
			func(fetchID uint64) error {
				affected, failErr := q.FailCrawlFetch(ctx, sqlcgen.FailCrawlFetchParams{
					FinishedAt: nullTime(params.FinishedAt), DurationMs: durationMillis(params.Duration),
					ErrorKind: nullString(params.ErrorKind), ErrorMessage: nullString(params.ErrorMessage),
					ID: fetchID,
				})
				if failErr != nil {
					return fmt.Errorf("fetch 실패 전이 실패: %w", failErr)
				}
				if affected != 1 {
					return fmt.Errorf("fetch 실패 전이 행 수가 유효하지 않습니다: %d", affected)
				}
				return nil
			},
		); fetchErr != nil {
			return fetchErr
		}

		currentStatus := string(CrawlPageStatusLeased)
		if params.PageWasRaw {
			currentStatus = string(CrawlPageStatusRawStored)
		}
		if params.Retryable && int(params.PageToken) < params.MaxAttempts && !params.Cancelled {
			if retryErr := leaseOne("page retry 대기 전이", func() (int64, error) {
				return q.MarkPageForRetry(ctx, sqlcgen.MarkPageForRetryParams{
					NextAttemptAt: nullTime(params.NextAttemptAt), LastError: nullString(params.ErrorMessage),
					ID: params.PageID, CurrentStatus: currentStatus,
					LeaseOwner: nullString(params.Owner), Attempts: params.PageToken,
				})
			}); retryErr != nil {
				return retryErr
			}
			if params.Discovery {
				if retryErr := leaseOne("partition retry 대기 전이", func() (int64, error) {
					return q.MarkPartitionForRetry(ctx, sqlcgen.MarkPartitionForRetryParams{
						NextAttemptAt: nullTime(params.NextAttemptAt), LastError: nullString(params.ErrorMessage),
						ID: params.PartitionID, LeaseOwner: nullString(params.Owner),
						Attempts: params.PartitionToken,
					})
				}); retryErr != nil {
					return retryErr
				}
			}
			return appendEvent(ctx, q, weblist.EventParams{
				RunID: params.RunID, PartitionID: params.PartitionID, PageID: params.PageID,
				WorkerID: params.Owner, Level: "WARN", Phase: "PAGE_RETRY_WAIT",
				Message: params.ErrorMessage,
				MetadataJSON: eventJSON(map[string]any{
					"status": "RETRY_WAIT", "attempt": params.PageToken, "owner": params.Owner,
					"page_no": params.Page, "error_kind": params.ErrorKind,
				}),
			})
		}

		pageStatus := string(CrawlPageStatusFailed)
		if params.ParseFailure {
			pageStatus = string(CrawlPageStatusParseFailed)
		}
		if failErr := leaseOne("page terminal 실패 전이", func() (int64, error) {
			return q.FailCrawlPage(ctx, sqlcgen.FailCrawlPageParams{
				Status: pageStatus, LastError: nullString(params.ErrorMessage),
				ID: params.PageID, CurrentStatus: currentStatus,
				LeaseOwner: nullString(params.Owner), Attempts: params.PageToken,
			})
		}); failErr != nil {
			return failErr
		}
		if params.Discovery {
			if failErr := leaseOne("discovery partition 실패 전이", func() (int64, error) {
				return q.FailCrawlPartition(ctx, sqlcgen.FailCrawlPartitionParams{
					LastError: nullString(params.ErrorMessage), ID: params.PartitionID,
					LeaseOwner: nullString(params.Owner), Attempts: params.PartitionToken,
				})
			}); failErr != nil {
				return failErr
			}
		}
		terminal = true
		return appendEvent(ctx, q, weblist.EventParams{
			RunID: params.RunID, PartitionID: params.PartitionID, PageID: params.PageID,
			WorkerID: params.Owner, Level: "ERROR", Phase: "FAILED", Message: params.ErrorMessage,
			MetadataJSON: eventJSON(map[string]any{
				"status": pageStatus, "attempt": params.PageToken, "owner": params.Owner,
				"page_no": params.Page, "error_kind": params.ErrorKind,
			}),
		})
	})
	return
}

func (s *Store) CompleteReconcile(
	ctx context.Context,
	params weblist.ReconcileCompleteParams,
) (result weblist.Result, err error) {
	var reconciliationErr error
	err = s.withTx(ctx, func(q *sqlcgen.Queries) error {
		reconciliationErr = nil
		row, getErr := q.GetPartitionReconciliation(ctx, params.PartitionID)
		if getErr != nil {
			return fmt.Errorf("partition reconciliation 조회 실패: %w", getErr)
		}
		status := string(CrawlPartitionStatusDone)
		level, phase := "INFO", "PARTITION_DONE"
		var mismatches []string
		if row.FailedPages > 0 {
			status, level, phase = string(CrawlPartitionStatusFailed), "ERROR", "PARTITION_FAILED"
			mismatches = append(mismatches, fmt.Sprintf("failed_pages=%d", row.FailedPages))
		} else {
			if row.ExpectedTotal.Int64 != row.ParsedRows {
				mismatches = append(mismatches, fmt.Sprintf(
					"expected_total=%d parsed_rows=%d", row.ExpectedTotal.Int64, row.ParsedRows,
				))
			}
			if row.RequiredPages != row.CompletedPages {
				mismatches = append(mismatches, fmt.Sprintf(
					"required_pages=%d completed_pages=%d", row.RequiredPages, row.CompletedPages,
				))
			}
			if row.TotalSnapshotVersions != 1 {
				mismatches = append(mismatches, fmt.Sprintf(
					"total_snapshot_versions=%d", row.TotalSnapshotVersions,
				))
			}
			if parserWarnings := databaseText(row.ParserWarnings); parserWarnings != "" {
				mismatches = append(mismatches, parserWarnings)
			}
			mismatches = append(mismatches, params.Warnings...)
			if len(mismatches) > 0 {
				status, level, phase = string(CrawlPartitionStatusDirty), "WARN", "PARTITION_DIRTY"
			}
		}
		lastError := weblist.SanitizeErrorMessage(strings.Join(mismatches, "; "))
		if completeErr := leaseOne("partition terminal 전이", func() (int64, error) {
			return q.CompletePartition(ctx, sqlcgen.CompletePartitionParams{
				Status: status, ParsedRows: uint64(row.ParsedRows),
				UniqueRcnoCount: uint64(row.UniqueRcnoCount), LastError: nullString(lastError),
				ID: params.PartitionID, LeaseOwner: nullString(params.Owner),
				Attempts: params.PartitionToken,
			})
		}); completeErr != nil {
			return completeErr
		}
		if eventErr := appendEvent(ctx, q, weblist.EventParams{
			RunID: params.RunID, PartitionID: params.PartitionID, WorkerID: params.Owner,
			Level: level, Phase: phase, Message: lastError,
			MetadataJSON: eventJSON(map[string]any{
				"status": status, "attempt": params.PartitionToken, "owner": params.Owner,
				"row_count": row.ParsedRows,
			}),
		}); eventErr != nil {
			return eventErr
		}
		if status == string(CrawlPartitionStatusDone) {
			if eventErr := appendEvent(ctx, q, weblist.EventParams{
				RunID: params.RunID, PartitionID: params.PartitionID, WorkerID: params.Owner,
				Level: "INFO", Phase: "FINISHED", Message: "partition 완료",
				MetadataJSON: eventJSON(map[string]any{
					"status": status, "attempt": params.PartitionToken, "owner": params.Owner,
				}),
			}); eventErr != nil {
				return eventErr
			}
		}
		result = weblist.Result{
			RunID: params.RunID, PartitionID: params.PartitionID, PartitionStatus: status,
			ExpectedTotal: uint64(row.ExpectedTotal.Int64), ExpectedPages: uint32(row.ExpectedPages.Int32),
			FetchedPages: uint32(row.CompletedPages), ParsedRows: uint64(row.ParsedRows),
			UniqueRCNOCount: uint64(row.UniqueRcnoCount), NewRCNOCount: uint64(row.NewRcnoCount),
		}
		reconciliationErr = reconciliationResultError(status, lastError, result)
		return nil
	})
	if err == nil && reconciliationErr != nil {
		err = reconciliationErr
	}
	return
}

func executeFailureFetchAttempt(
	initialFetchID uint64,
	pageWasRaw bool,
	storeFailedFetch func() (uint64, error),
	failStoredFetch func(uint64) error,
) (uint64, error) {
	fetchID := initialFetchID
	if fetchID == 0 {
		return storeFailedFetch()
	}
	if pageWasRaw {
		if err := failStoredFetch(fetchID); err != nil {
			return 0, err
		}
	}
	return fetchID, nil
}

func reconciliationResultError(
	status string,
	message string,
	result weblist.Result,
) error {
	if status != string(CrawlPartitionStatusDirty) {
		return nil
	}
	return &weblist.ReconciliationError{Message: message, Result: result}
}

func (s *Store) RequestCancellation(ctx context.Context, runID uint64) error {
	affected, err := sqlcgen.New(s.db).RequestCrawlRunCancellation(ctx, runID)
	if err != nil {
		return fmt.Errorf("run 취소 요청 실패: %w", err)
	}
	if affected > 1 {
		return fmt.Errorf("run 취소 요청 행 수가 유효하지 않습니다: %d", affected)
	}
	return nil
}
