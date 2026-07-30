//go:build legacy

package mysql

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/bottle-note/mfds-crawler/internal/source/mfdsweb"
	"github.com/bottle-note/mfds-crawler/internal/usecase/weblist"
	"github.com/bottle-note/mfds-crawler/secrets/project/mfds/sqlcgen"
)

func (s *Store) JournalFetch(ctx context.Context, params weblist.JournalFetchParams) (fetchID uint64, err error) {
	if params.ErrorKind != "" {
		return 0, fmt.Errorf("성공 fetch journal에는 error kind를 사용할 수 없습니다")
	}
	err = s.withTx(ctx, func(q *sqlcgen.Queries) error {
		var storeErr error
		fetchID, storeErr = storeFetch(ctx, q, params, string(CrawlFetchStatusRawStored))
		if storeErr != nil {
			return storeErr
		}
		if err := leaseOne("page raw 저장", func() (int64, error) {
			return q.MarkPageRawStored(ctx, sqlcgen.MarkPageRawStoredParams{
				ID: params.PageID, LeaseOwner: nullString(params.Owner), Attempts: params.AttemptNo,
			})
		}); err != nil {
			return err
		}
		if err := appendEvent(ctx, q, weblist.EventParams{
			RunID: params.RunID, PartitionID: params.PartitionID, PageID: params.PageID,
			WorkerID: params.Owner, Level: "INFO", Phase: "RAW_STORED",
			Message: "page 원문 저장",
			MetadataJSON: eventJSON(map[string]any{
				"status": "RAW_STORED", "attempt": params.AttemptNo, "owner": params.Owner,
			}),
		}); err != nil {
			return err
		}
		return appendEvent(ctx, q, weblist.EventParams{
			RunID: params.RunID, PartitionID: params.PartitionID, PageID: params.PageID,
			WorkerID: params.Owner, Level: "INFO", Phase: "PARSING",
			Message: "page 파싱 준비",
			MetadataJSON: eventJSON(map[string]any{
				"status": "RAW_STORED", "attempt": params.AttemptNo, "owner": params.Owner,
			}),
		})
	})
	return
}

func storeFetch(
	ctx context.Context,
	q *sqlcgen.Queries,
	params weblist.JournalFetchParams,
	status string,
) (uint64, error) {
	id, err := q.CreateCrawlFetch(ctx, sqlcgen.CreateCrawlFetchParams{
		RunID: params.RunID, PartitionID: nullInt64(params.PartitionID), PageID: nullInt64(params.PageID),
		SourceKind: string(CrawlFetchSourceKindWebList), RequestKeySha256: params.Fetch.RequestKeySHA256[:],
		RequestMethod: params.Fetch.Method, RequestUrl: params.Fetch.URL,
		RequestQueryJson: params.Fetch.QueryJSON, RequestHeadersJson: params.Fetch.RequestHeadersJSON,
		AttemptNo: params.AttemptNo, StartedAt: params.Fetch.StartedAt,
	})
	if err != nil || id <= 0 {
		return 0, fmt.Errorf("crawl fetch 생성 실패: id=%d: %w", id, err)
	}
	responseBody, responseSize, responseHash := sql.NullString{}, sql.NullInt64{}, sql.NullString{}
	if params.Fetch.BodyCaptured {
		responseBody, responseSize = nullString(string(params.Fetch.BodyGZIP)), nullInt64Signed(params.Fetch.BodySize)
		responseHash = nullString(string(params.Fetch.BodySHA256[:]))
	}
	if err := one("fetch response 저장", func() (int64, error) {
		return q.StoreCrawlFetchResponse(ctx, sqlcgen.StoreCrawlFetchResponseParams{
			FinishedAt: nullTime(params.Fetch.FinishedAt), DurationMs: durationMillis(params.Fetch.Duration),
			HttpStatus: nullInt16(params.Fetch.HTTPStatus), ResponseHeadersJson: params.Fetch.ResponseHeadersJSON,
			ContentType: nullString(params.Fetch.ContentType), ResponseBodyGzip: responseBody,
			ResponseSizeBytes: responseSize, ResponseSha256: responseHash, Status: status,
			ErrorKind: nullString(params.ErrorKind), ErrorMessage: nullString(params.ErrorMessage), ID: uint64(id),
		})
	}); err != nil {
		return 0, err
	}
	return uint64(id), nil
}

func (s *Store) CommitPage(ctx context.Context, params weblist.CommitPageParams) error {
	return s.withTx(ctx, func(q *sqlcgen.Queries) error {
		if params.Request.Page == 1 {
			if err := leaseOne("partition snapshot 저장", func() (int64, error) {
				return q.UpdatePartitionListingSnapshot(ctx, sqlcgen.UpdatePartitionListingSnapshotParams{
					ExpectedTotal: nullInt64Signed(int64(params.Page.Total)),
					ExpectedPages: nullInt32(int64(params.Page.TotalPages)),
					ID:            params.PartitionID, LeaseOwner: nullString(params.Owner),
					Attempts: params.PartitionAttempt,
				})
			}); err != nil {
				return err
			}
			for pageNo := 2; pageNo <= params.Page.TotalPages; pageNo++ {
				id, err := q.CreateCrawlPage(ctx, sqlcgen.CreateCrawlPageParams{
					PartitionID: params.PartitionID, PageNo: uint32(pageNo),
				})
				if err != nil || id <= 0 {
					return fmt.Errorf("후속 page %d 생성 실패: id=%d: %w", pageNo, id, err)
				}
			}
		}
		unique := make(map[string]struct{}, len(params.Page.Rows))
		for _, row := range params.Page.Rows {
			rawID, err := q.CreateWebListRaw(ctx, sqlcgen.CreateWebListRawParams{
				RunID: params.RunID, PartitionID: params.PartitionID, PageID: params.PageID,
				FetchID: params.FetchID, RowNo: uint32(row.RowNo), Rcno: row.RCNO,
				QueriedItemCode: params.Request.ItemCode, QueriedItemName: params.Request.ItemName,
				ProductDivisionName: nullString(row.ProductDivisionName), ImporterName: nullString(row.ImporterName),
				ProductNameKo: nullString(row.ProductNameKO), ProductNameEn: nullString(row.ProductNameEN),
				ItemName: nullString(row.ItemName), OverseasEstablishmentName: nullString(row.OverseasEstablishmentName),
				ProcessedDateRaw: nullString(row.ProcessedDateRaw), ProcessedDate: nullableDate(row.ProcessedDate),
				ExpiryText: nullString(row.ExpiryText), ManufactureCountryName: nullString(row.ManufactureCountryName),
				ExportCountryName: nullString(row.ExportCountryName), DetailHref: nullString(row.DetailHref),
				CanonicalValuesJson: row.CanonicalValuesJSON, RawRowHtml: string(row.RawRowHTML),
				RawRowSha256: row.RawRowSHA256[:], ListSemanticSha256: row.SemanticSHA256[:],
				ParserVersion: mfdsweb.ParserVersion, ObservedAt: params.ObservedAt,
			})
			if err != nil || rawID <= 0 {
				return fmt.Errorf("목록 raw row %d 저장 실패: id=%d: %w", row.RowNo, rawID, err)
			}
			if _, err := q.UpsertWebRcnoRegistry(ctx, sqlcgen.UpsertWebRcnoRegistryParams{
				Rcno: row.RCNO, ObservedAt: params.ObservedAt, ListRawID: uint64(rawID),
				ListSemanticSha256: row.SemanticSHA256[:],
			}); err != nil {
				return fmt.Errorf("rcno %s registry upsert 실패: %w", row.RCNO, err)
			}
			unique[row.RCNO] = struct{}{}
		}
		if err := one("fetch parsed 전이", func() (int64, error) {
			return q.MarkCrawlFetchParsed(ctx, sqlcgen.MarkCrawlFetchParsedParams{
				ParserVersion: nullString(mfdsweb.ParserVersion), ParsedItemCount: nullInt32(int64(len(params.Page.Rows))),
				ID: params.FetchID,
			})
		}); err != nil {
			return err
		}
		if err := leaseOne("page parsed commit", func() (int64, error) {
			return q.MarkPageParsedCommitted(ctx, sqlcgen.MarkPageParsedCommittedParams{
				TotalSnapshot: nullInt64Signed(int64(params.Page.Total)), RowCount: nullInt32(int64(len(params.Page.Rows))),
				UniqueRcnoCount: nullInt32(int64(len(unique))), ID: params.PageID, LeaseOwner: nullString(params.Owner),
				Attempts: params.PageAttempt,
			})
		}); err != nil {
			return err
		}
		if err := leaseOne("page 완료", func() (int64, error) {
			return q.CompletePage(ctx, sqlcgen.CompletePageParams{
				LastError: nullString(weblist.SanitizeErrorMessage(strings.Join(params.Page.Warnings, "; "))),
				ID:        params.PageID, LeaseOwner: nullString(params.Owner), Attempts: params.PageAttempt,
			})
		}); err != nil {
			return err
		}
		if err := appendEvent(ctx, q, weblist.EventParams{
			RunID: params.RunID, PartitionID: params.PartitionID, PageID: params.PageID,
			WorkerID: params.Owner, Level: "INFO", Phase: "COMMITTED",
			Message: fmt.Sprintf("page %d 원장 %d행 반영", params.Request.Page, len(params.Page.Rows)),
			MetadataJSON: eventJSON(map[string]any{
				"status": "DONE", "attempt": params.PageAttempt, "owner": params.Owner,
				"page_no": params.Request.Page, "row_count": len(params.Page.Rows),
			}),
		}); err != nil {
			return err
		}
		if params.Request.Page == 1 {
			return appendEvent(ctx, q, weblist.EventParams{
				RunID: params.RunID, PartitionID: params.PartitionID, PageID: params.PageID,
				WorkerID: params.Owner, Level: "INFO", Phase: "PAGES_DISCOVERED",
				Message: fmt.Sprintf("후속 page %d개 발견", max(0, params.Page.TotalPages-1)),
				MetadataJSON: eventJSON(map[string]any{
					"status": "PAGING", "attempt": params.PageAttempt, "owner": params.Owner,
					"page_no": 1, "expected_pages": params.Page.TotalPages,
				}),
			})
		}
		return nil
	})
}

func (s *Store) FinalizeJob(
	ctx context.Context,
	params weblist.FinalizeJobParams,
) (status string, changed bool, err error) {
	err = s.withTx(ctx, func(q *sqlcgen.Queries) error {
		run, lockErr := q.GetCrawlRunForUpdate(ctx, params.RunID)
		if lockErr != nil {
			return fmt.Errorf("run finalization lock 실패: %w", lockErr)
		}
		status = run.Status
		switch CrawlRunStatus(run.Status) {
		case CrawlRunStatusCompleted, CrawlRunStatusPartialFailed, CrawlRunStatusCancelled:
			return nil
		}
		if params.Cancelled && !run.CancelRequestedAt.Valid {
			if _, requestErr := q.RequestCrawlRunCancellation(ctx, params.RunID); requestErr != nil {
				return fmt.Errorf("run 취소 요청 실패: %w", requestErr)
			}
			run.CancelRequestedAt = nullTime(params.FinishedAt)
		}
		if run.CancelRequestedAt.Valid {
			if _, cancelErr := q.CancelPendingPagesByRun(ctx, params.RunID); cancelErr != nil {
				return fmt.Errorf("취소 대상 page terminal 전이 실패: %w", cancelErr)
			}
			if _, cancelErr := q.CancelPendingPartitionsByRun(ctx, params.RunID); cancelErr != nil {
				return fmt.Errorf("취소 대상 partition terminal 전이 실패: %w", cancelErr)
			}
			if _, cancelErr := q.CancelReadyPartitionsByRun(ctx, params.RunID); cancelErr != nil {
				return fmt.Errorf("취소 준비 partition terminal 전이 실패: %w", cancelErr)
			}
		}
		if err := refreshRunCounters(ctx, q, params.RunID); err != nil {
			return err
		}
		summary, err := q.GetCrawlRunCompletion(ctx, sqlcgen.GetCrawlRunCompletionParams{
			RunID: params.RunID,
		})
		if err != nil {
			return fmt.Errorf("run 완료 상태 조회 실패: %w", err)
		}
		if summary.TotalPartitions == 0 {
			return fmt.Errorf("run %d에 partition이 없습니다", params.RunID)
		}
		if summary.ActivePartitions > 0 {
			return nil
		}
		status = string(CrawlRunStatusCompleted)
		if summary.FailedPartitions > 0 {
			status = string(CrawlRunStatusPartialFailed)
		}
		if run.CancelRequestedAt.Valid {
			status = string(CrawlRunStatusCancelled)
		}
		if err := one("run 완료", func() (int64, error) {
			return q.FinalizeCrawlRun(ctx, sqlcgen.FinalizeCrawlRunParams{
				Status: status, FinishedAt: nullTime(params.FinishedAt),
				LastError: nullString(databaseText(summary.LastError)), ID: params.RunID,
			})
		}); err != nil {
			return err
		}
		changed = true
		level, phase := "INFO", "JOB_COMPLETED"
		if status == string(CrawlRunStatusPartialFailed) {
			level, phase = "WARN", "JOB_PARTIAL_FAILED"
		}
		if status == string(CrawlRunStatusCancelled) {
			level, phase = "WARN", "JOB_CANCELLED"
		}
		return appendEvent(ctx, q, weblist.EventParams{
			RunID: params.RunID, Level: level, Phase: phase, Message: status,
			MetadataJSON: eventJSON(map[string]any{"status": status}),
		})
	})
	return
}

func (s *Store) withTx(ctx context.Context, operation func(*sqlcgen.Queries) error) (err error) {
	const maximumTransactionAttempts = 3
	for attempt := 1; attempt <= maximumTransactionAttempts; attempt++ {
		err = s.withTxOnce(ctx, operation)
		if err == nil || !isRetryableTransactionError(err) || attempt == maximumTransactionAttempts {
			return err
		}
		timer := time.NewTimer(time.Duration(attempt*10) * time.Millisecond)
		select {
		case <-ctx.Done():
			if !timer.Stop() {
				<-timer.C
			}
			return errors.Join(err, ctx.Err())
		case <-timer.C:
		}
	}
	return err
}

func (s *Store) withTxOnce(ctx context.Context, operation func(*sqlcgen.Queries) error) (err error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("transaction 시작 실패: %w", err)
	}
	defer func() {
		if err == nil {
			return
		}
		if rollbackErr := tx.Rollback(); rollbackErr != nil && !errors.Is(rollbackErr, sql.ErrTxDone) {
			err = errors.Join(err, fmt.Errorf("transaction rollback 실패: %w", rollbackErr))
		}
	}()
	if err = operation(sqlcgen.New(s.db).WithTx(tx)); err != nil {
		return err
	}
	if err = tx.Commit(); err != nil {
		return fmt.Errorf("transaction commit 실패: %w", err)
	}
	return nil
}

func one(operation string, execute func() (int64, error)) error {
	affected, err := execute()
	if err != nil {
		return fmt.Errorf("%s 실패: %w", operation, err)
	}
	if affected != 1 {
		return fmt.Errorf("%s 행 수가 유효하지 않습니다: %d", operation, affected)
	}
	return nil
}

func refreshRunCounters(ctx context.Context, q *sqlcgen.Queries, runID uint64) error {
	affected, err := q.RefreshCrawlRunCounters(ctx, runID)
	if err != nil {
		return fmt.Errorf("run counter 갱신 실패: %w", err)
	}
	if affected > 1 {
		return fmt.Errorf("run counter 갱신 행 수가 유효하지 않습니다: %d", affected)
	}
	if affected == 0 {
		if _, err := q.GetCrawlRun(ctx, runID); err != nil {
			return fmt.Errorf("run counter 갱신 대상 확인 실패: %w", err)
		}
	}
	return nil
}

func nullString(value string) sql.NullString {
	return sql.NullString{String: value, Valid: value != ""}
}
func nullTime(value time.Time) sql.NullTime { return sql.NullTime{Time: value, Valid: !value.IsZero()} }
func nullInt64(value uint64) sql.NullInt64 {
	return sql.NullInt64{Int64: int64(value), Valid: value != 0}
}
func nullInt64Signed(value int64) sql.NullInt64 { return sql.NullInt64{Int64: value, Valid: true} }
func nullInt32(value int64) sql.NullInt32       { return sql.NullInt32{Int32: int32(value), Valid: true} }
func nullInt16(value int) sql.NullInt16         { return sql.NullInt16{Int16: int16(value), Valid: value != 0} }
func nullableDate(value time.Time) sql.NullTime { return nullTime(value) }
func databaseText(value any) string {
	switch typed := value.(type) {
	case string:
		return typed
	case []byte:
		return string(typed)
	default:
		return ""
	}
}
func durationMillis(value time.Duration) sql.NullInt32 {
	if value < 0 {
		value = 0
	}
	return sql.NullInt32{Int32: int32(value / time.Millisecond), Valid: true}
}

var _ weblist.Store = (*Store)(nil)
