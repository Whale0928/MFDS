package mysql

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/bottle-note/mfds-crawler/internal/source/mfdsweb"
	"github.com/bottle-note/mfds-crawler/internal/usecase/ledger"
	"github.com/bottle-note/mfds-crawler/internal/usecase/operator"
	"github.com/bottle-note/mfds-crawler/internal/usecase/overview"
	"github.com/bottle-note/mfds-crawler/internal/usecase/weblist"
)

func (s *Store) StartWebListJob(
	ctx context.Context,
	params weblist.JobStartParams,
) (record weblist.JobStartRecord, err error) {
	err = s.withTx(ctx, func(tx *sql.Tx) error {
		result, err := tx.ExecContext(ctx, `
			INSERT INTO jobs (
				job_type, requested_from_date, requested_to_date, status,
				config_json, page_size, total_tasks, started_at
			) VALUES ('BACKFILL', ?, ?, 'CREATED', ?, ?, DATEDIFF(?, ?) + 1, ?)
		`, params.FromDate, params.ToDate, params.ConfigJSON, params.PageSize,
			params.ToDate, params.FromDate, params.StartedAt)
		if err != nil {
			return fmt.Errorf("Job 생성 실패: %w", err)
		}
		id, err := result.LastInsertId()
		if err != nil || id <= 0 {
			return fmt.Errorf("Job ID 확인 실패: id=%d: %w", id, err)
		}
		record.RunID = uint64(id)
		for date := params.FromDate; !date.After(params.ToDate); date = date.AddDate(0, 0, 1) {
			if _, err := tx.ExecContext(ctx, `
				INSERT INTO tasks (job_id, process_date, status)
				VALUES (?, ?, 'PENDING')
			`, record.RunID, date); err != nil {
				return fmt.Errorf("날짜 Task %s 생성 실패: %w", date.Format(time.DateOnly), err)
			}
		}
		return nil
	})
	return record, err
}

func (s *Store) ClaimTask(
	ctx context.Context,
	params weblist.ClaimParams,
) (task weblist.DateTask, found bool, err error) {
	err = s.withTx(ctx, func(tx *sql.Tx) error {
		row := tx.QueryRowContext(ctx, `
			SELECT t.id, t.process_date, t.attempts
			FROM tasks AS t
			JOIN jobs AS j ON j.id = t.job_id
			WHERE t.job_id = ?
			  AND j.status IN ('CREATED', 'RUNNING')
			  AND j.cancel_requested_at IS NULL
			  AND (
			      t.status = 'PENDING'
			      OR (t.status = 'RETRY_WAIT' AND t.next_attempt_at <= ?)
			      OR (t.status = 'LEASED' AND t.lease_until < ?)
			  )
			ORDER BY t.process_date, t.id
			LIMIT 1
			FOR UPDATE SKIP LOCKED
		`, params.RunID, params.Now, params.Now)
		var taskID uint64
		var processDate time.Time
		var attempts uint32
		if scanErr := row.Scan(&taskID, &processDate, &attempts); errors.Is(scanErr, sql.ErrNoRows) {
			return nil
		} else if scanErr != nil {
			return fmt.Errorf("날짜 Task claim 조회 실패: %w", scanErr)
		}
		result, updateErr := tx.ExecContext(ctx, `
			UPDATE tasks
			SET status = 'LEASED', attempts = attempts + 1,
			    lease_owner = ?, lease_until = ?, next_attempt_at = NULL,
			    last_error = NULL, started_at = COALESCE(started_at, ?)
			WHERE id = ?
		`, params.Owner, params.LeaseUntil, params.Now, taskID)
		if updateErr != nil {
			return fmt.Errorf("날짜 Task lease 실패: %w", updateErr)
		}
		if err := requireOne(result, "날짜 Task lease"); err != nil {
			return err
		}
		if _, updateErr = tx.ExecContext(ctx, `
			UPDATE jobs SET status = 'RUNNING', started_at = COALESCE(started_at, ?)
			WHERE id = ? AND status = 'CREATED'
		`, params.Now, params.RunID); updateErr != nil {
			return fmt.Errorf("Job RUNNING 전이 실패: %w", updateErr)
		}
		task = weblist.DateTask{
			RunID: params.RunID, TaskID: taskID, Attempt: attempts + 1,
			Owner: params.Owner, ProcessDate: processDate,
		}
		found = true
		return nil
	})
	return task, found, err
}

func (s *Store) RenewTask(
	ctx context.Context,
	task weblist.DateTask,
	leaseUntil time.Time,
) error {
	result, err := s.db.ExecContext(ctx, `
		UPDATE tasks SET lease_until = ?
		WHERE id = ? AND job_id = ? AND status = 'LEASED'
		  AND lease_owner = ? AND attempts = ? AND lease_until >= NOW(6)
	`, leaseUntil, task.TaskID, task.RunID, task.Owner, task.Attempt)
	if err != nil {
		return fmt.Errorf("Task lease 갱신 실패: %w", err)
	}
	return requireLease(result, "Task lease 갱신")
}

func (s *Store) CommitFetch(ctx context.Context, params weblist.CommitFetchParams) error {
	return s.withTx(ctx, func(tx *sql.Tx) error {
		if err := lockTaskLease(ctx, tx, params.Task); err != nil {
			return err
		}
		fetchID, err := insertFetch(ctx, tx, params.Task, params.Request, params.Fetch,
			params.Page, "PARSED", "", "")
		if err != nil {
			return err
		}
		for _, row := range params.Page.Rows {
			if _, err := tx.ExecContext(ctx, `
				INSERT INTO items (
					job_id, task_id, fetch_id, row_no, rcno,
					queried_item_code, queried_item_name, product_division_name,
					importer_name, product_name_ko, product_name_en, item_name,
					overseas_establishment_name, processed_date_raw, processed_date,
					expiry_text, manufacture_country_name, export_country_name,
					detail_href, canonical_values_json, raw_row_html, raw_row_sha256,
					semantic_sha256, parser_version, parser_warning, observed_at
				) VALUES (
					?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, NULLIF(?, '0001-01-01'),
					?, ?, ?, ?, ?, ?, ?, ?, ?, NULL, ?
				)
				ON DUPLICATE KEY UPDATE id = id
			`, params.Task.RunID, params.Task.TaskID, fetchID, row.RowNo, row.RCNO,
				params.Request.ItemCode, params.Request.ItemName,
				nullString(row.ProductDivisionName), nullString(row.ImporterName),
				nullString(row.ProductNameKO), nullString(row.ProductNameEN),
				nullString(row.ItemName), nullString(row.OverseasEstablishmentName),
				nullString(row.ProcessedDateRaw), row.ProcessedDate.Format(time.DateOnly),
				nullString(row.ExpiryText), nullString(row.ManufactureCountryName),
				nullString(row.ExportCountryName), nullString(row.DetailHref),
				row.CanonicalValuesJSON, string(row.RawRowHTML), row.RawRowSHA256[:],
				row.SemanticSHA256[:], mfdsweb.ParserVersion, params.ObservedAt,
			); err != nil {
				return fmt.Errorf("Item row %d 저장 실패: %w", row.RowNo, err)
			}
		}
		return refreshTaskCounters(ctx, tx, params.Task.TaskID)
	})
}

func (s *Store) RecordFailedFetch(
	ctx context.Context,
	params weblist.FailedFetchParams,
) error {
	return s.withTx(ctx, func(tx *sql.Tx) error {
		if err := lockTaskLease(ctx, tx, params.Task); err != nil {
			return err
		}
		page := weblist.Page{}
		status := "FAILED"
		if params.ParserFailure {
			status = "PARSE_FAILED"
		}
		_, err := insertFetch(ctx, tx, params.Task, params.Request, params.Fetch,
			page, status, params.ErrorKind, params.ErrorMessage)
		return err
	})
}

func (s *Store) LoadTaskAttemptEvidence(
	ctx context.Context,
	task weblist.DateTask,
) ([]weblist.TaskAttemptEvidence, error) {
	attempts := make(map[uint32]*weblist.TaskAttemptEvidence)
	itemIndexes := make(map[uint32]map[string]int)
	ensureItem := func(attempt uint32, itemCode, itemName string) *weblist.AttemptItemEvidence {
		evidence, exists := attempts[attempt]
		if !exists {
			evidence = &weblist.TaskAttemptEvidence{Attempt: attempt}
			attempts[attempt] = evidence
			itemIndexes[attempt] = make(map[string]int)
		}
		index, exists := itemIndexes[attempt][itemCode]
		if !exists {
			index = len(evidence.Items)
			evidence.Items = append(evidence.Items, weblist.AttemptItemEvidence{
				ItemCode: itemCode,
				ItemName: itemName,
			})
			itemIndexes[attempt][itemCode] = index
		}
		return &evidence.Items[index]
	}

	pageRows, err := s.db.QueryContext(ctx, `
		SELECT attempt_no, item_code, item_name, page_no, status,
		       COALESCE(total_snapshot, 0), COALESCE(parsed_item_count, 0)
		FROM fetches
		WHERE task_id = ? AND job_id = ? AND attempt_no <= ?
		ORDER BY attempt_no, item_code, page_no, id
	`, task.TaskID, task.RunID, task.Attempt)
	if err != nil {
		return nil, fmt.Errorf("Task attempt page 증거 조회 실패: %w", err)
	}
	for pageRows.Next() {
		var attempt, pageNo, parsedRows uint32
		var itemCode, itemName, status string
		var total uint64
		if err := pageRows.Scan(
			&attempt,
			&itemCode,
			&itemName,
			&pageNo,
			&status,
			&total,
			&parsedRows,
		); err != nil {
			_ = pageRows.Close()
			return nil, fmt.Errorf("Task attempt page 증거 scan 실패: %w", err)
		}
		item := ensureItem(attempt, itemCode, itemName)
		item.Pages = append(item.Pages, weblist.AttemptPageEvidence{
			PageNo:     pageNo,
			Status:     status,
			Total:      total,
			ParsedRows: parsedRows,
		})
	}
	if err := pageRows.Close(); err != nil {
		return nil, fmt.Errorf("Task attempt page 증거 close 실패: %w", err)
	}
	if err := pageRows.Err(); err != nil {
		return nil, fmt.Errorf("Task attempt page 증거 순회 실패: %w", err)
	}

	itemRows, err := s.db.QueryContext(ctx, `
		SELECT f.attempt_no, f.item_code, f.item_name, i.rcno
		FROM items AS i
		JOIN fetches AS f ON f.id = i.fetch_id
		WHERE i.task_id = ? AND i.job_id = ? AND f.attempt_no <= ?
		ORDER BY f.attempt_no, f.item_code, i.rcno, i.id
	`, task.TaskID, task.RunID, task.Attempt)
	if err != nil {
		return nil, fmt.Errorf("Task attempt RCNO 증거 조회 실패: %w", err)
	}
	for itemRows.Next() {
		var attempt uint32
		var itemCode, itemName, rcno string
		if err := itemRows.Scan(&attempt, &itemCode, &itemName, &rcno); err != nil {
			_ = itemRows.Close()
			return nil, fmt.Errorf("Task attempt RCNO 증거 scan 실패: %w", err)
		}
		item := ensureItem(attempt, itemCode, itemName)
		item.RCNOs = append(item.RCNOs, rcno)
	}
	if err := itemRows.Close(); err != nil {
		return nil, fmt.Errorf("Task attempt RCNO 증거 close 실패: %w", err)
	}
	if err := itemRows.Err(); err != nil {
		return nil, fmt.Errorf("Task attempt RCNO 증거 순회 실패: %w", err)
	}

	result := make([]weblist.TaskAttemptEvidence, 0, len(attempts))
	for _, evidence := range attempts {
		sort.Slice(evidence.Items, func(left, right int) bool {
			return evidence.Items[left].ItemCode < evidence.Items[right].ItemCode
		})
		result = append(result, *evidence)
	}
	sort.Slice(result, func(left, right int) bool {
		return result[left].Attempt < result[right].Attempt
	})
	return result, nil
}

func insertFetch(
	ctx context.Context,
	tx *sql.Tx,
	task weblist.DateTask,
	request weblist.ListRequest,
	fetch weblist.Fetch,
	page weblist.Page,
	status, errorKind, errorMessage string,
) (uint64, error) {
	startedAt := fetch.StartedAt
	if startedAt.IsZero() {
		startedAt = time.Now()
	}
	result, err := tx.ExecContext(ctx, `
		INSERT INTO fetches (
			job_id, task_id, item_code, item_name, page_no, source_kind,
			request_key_sha256, request_method, request_url, request_query_json,
			request_headers_json, attempt_no, started_at, finished_at, duration_ms,
			http_status, response_headers_json, content_type, response_body_encoding,
			response_body_gzip, response_size_bytes, response_sha256, parser_version,
			parsed_item_count, total_snapshot, status, error_kind, error_message
		) VALUES (
			?, ?, ?, ?, ?, 'WEB_LIST', ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?,
			'gzip', ?, ?, ?, ?, ?, ?, ?, ?, ?
		)
		ON DUPLICATE KEY UPDATE id = LAST_INSERT_ID(id)
	`, task.RunID, task.TaskID, request.ItemCode, request.ItemName, request.Page,
		fetch.RequestKeySHA256[:], fallback(fetch.Method, "GET"),
		fetch.URL, validJSON(fetch.QueryJSON, `{}`), nullableJSON(fetch.RequestHeadersJSON),
		task.Attempt, startedAt, nullTime(fetch.FinishedAt), durationMillis(fetch.Duration),
		nullInt16(fetch.HTTPStatus), nullableJSON(fetch.ResponseHeadersJSON),
		nullString(fetch.ContentType), nullBytes(fetch.BodyGZIP), nullInt64(fetch.BodySize),
		nullHash(fetch.BodySHA256, fetch.BodyCaptured), nullString(mfdsweb.ParserVersion),
		nullInt32(int64(len(page.Rows))), nullInt64(int64(page.Total)), status,
		nullString(errorKind), nullString(weblist.SanitizeErrorMessage(errorMessage)),
	)
	if err != nil {
		return 0, fmt.Errorf("Fetch 저장 실패: %w", err)
	}
	id, err := result.LastInsertId()
	if err != nil || id <= 0 {
		return 0, fmt.Errorf("Fetch ID 확인 실패: id=%d: %w", id, err)
	}
	return uint64(id), nil
}

func (s *Store) CompleteTask(ctx context.Context, task weblist.DateTask) error {
	return s.withTx(ctx, func(tx *sql.Tx) error {
		if err := lockTaskLease(ctx, tx, task); err != nil {
			return err
		}
		if err := refreshTaskCounters(ctx, tx, task.TaskID); err != nil {
			return err
		}
		result, err := tx.ExecContext(ctx, `
			UPDATE tasks
			SET status = 'DONE', lease_owner = NULL, lease_until = NULL,
			    next_attempt_at = NULL, finished_at = NOW(6), last_error = NULL
			WHERE id = ? AND job_id = ? AND status = 'LEASED'
			  AND lease_owner = ? AND attempts = ?
		`, task.TaskID, task.RunID, task.Owner, task.Attempt)
		if err != nil {
			return fmt.Errorf("Task 완료 실패: %w", err)
		}
		return requireLease(result, "Task 완료")
	})
}

func (s *Store) FailTask(ctx context.Context, params weblist.FailTaskParams) error {
	status := "FAILED"
	var nextAttempt any
	if params.Retryable && int(params.Task.Attempt) < params.MaxAttempts {
		status = "RETRY_WAIT"
		nextAttempt = params.NextAttemptAt
	}
	result, err := s.db.ExecContext(ctx, `
		UPDATE tasks
		SET status = ?, lease_owner = NULL, lease_until = NULL,
		    next_attempt_at = ?, last_error = ?,
		    finished_at = CASE WHEN ? = 'FAILED' THEN NOW(6) ELSE NULL END
		WHERE id = ? AND job_id = ? AND status = 'LEASED'
		  AND lease_owner = ? AND attempts = ? AND lease_until >= NOW(6)
	`, status, nextAttempt, params.ErrorMessage, status,
		params.Task.TaskID, params.Task.RunID, params.Task.Owner, params.Task.Attempt)
	if err != nil {
		return fmt.Errorf("Task 실패 전이 실패: %w", err)
	}
	return requireLease(result, "Task 실패 전이")
}

func (s *Store) RequestCancellation(ctx context.Context, jobID uint64) error {
	_, err := s.db.ExecContext(ctx, `
		UPDATE jobs SET cancel_requested_at = COALESCE(cancel_requested_at, NOW(6))
		WHERE id = ? AND status IN ('CREATED', 'RUNNING')
	`, jobID)
	if err != nil {
		return fmt.Errorf("Job 취소 요청 실패: %w", err)
	}
	return nil
}

func (s *Store) FinalizeJob(
	ctx context.Context,
	jobID uint64,
	finishedAt time.Time,
) (status string, changed bool, err error) {
	err = s.withTx(ctx, func(tx *sql.Tx) error {
		var cancelRequested sql.NullTime
		if err := tx.QueryRowContext(ctx, `
			SELECT status, cancel_requested_at FROM jobs WHERE id = ? FOR UPDATE
		`, jobID).Scan(&status, &cancelRequested); err != nil {
			return fmt.Errorf("Job finalization lock 실패: %w", err)
		}
		if status == weblist.RunStatusCompleted || status == weblist.RunStatusPartialFailed ||
			status == weblist.RunStatusCancelled {
			return nil
		}
		if cancelRequested.Valid {
			if _, err := tx.ExecContext(ctx, `
				UPDATE tasks SET status = 'FAILED', last_error = 'CANCELLED',
				    next_attempt_at = NULL, lease_owner = NULL, lease_until = NULL,
				    finished_at = NOW(6)
				WHERE job_id = ? AND status IN ('PENDING', 'RETRY_WAIT')
			`, jobID); err != nil {
				return fmt.Errorf("대기 Task 취소 실패: %w", err)
			}
		}
		var total, done, failed, active uint32
		var lastError sql.NullString
		if err := tx.QueryRowContext(ctx, `
			SELECT COUNT(*), SUM(status = 'DONE'), SUM(status = 'FAILED'),
			       SUM(status IN ('PENDING', 'LEASED', 'RETRY_WAIT')), MAX(last_error)
			FROM tasks WHERE job_id = ?
		`, jobID).Scan(&total, &done, &failed, &active, &lastError); err != nil {
			return fmt.Errorf("Job Task 집계 실패: %w", err)
		}
		if active > 0 {
			return refreshJobCounters(ctx, tx, jobID)
		}
		status = weblist.RunStatusCompleted
		if failed > 0 {
			status = weblist.RunStatusPartialFailed
		}
		if cancelRequested.Valid {
			status = weblist.RunStatusCancelled
		}
		if err := refreshJobCounters(ctx, tx, jobID); err != nil {
			return err
		}
		result, err := tx.ExecContext(ctx, `
			UPDATE jobs SET status = ?, total_tasks = ?, completed_tasks = ?,
			    last_error = ?, finished_at = ?
			WHERE id = ? AND status IN ('CREATED', 'RUNNING')
		`, status, total, done, lastError, finishedAt, jobID)
		if err != nil {
			return fmt.Errorf("Job 완료 실패: %w", err)
		}
		if err := requireOne(result, "Job 완료"); err != nil {
			return err
		}
		changed = true
		return nil
	})
	return status, changed, err
}

func (s *Store) LoadJobResult(
	ctx context.Context,
	jobID uint64,
) (result weblist.JobResult, err error) {
	var failed uint32
	err = s.db.QueryRowContext(ctx, `
		SELECT j.id, j.status, j.total_tasks, j.completed_tasks,
		       COUNT(DISTINCT CASE WHEN t.status = 'FAILED' THEN t.id END),
		       j.fetched_requests, j.parsed_rows,
		       (
		           SELECT COUNT(DISTINCT i.rcno)
		           FROM items AS i
		           JOIN fetches AS f ON f.id = i.fetch_id
		           JOIN tasks AS accepted_task ON accepted_task.id = i.task_id
		           WHERE i.job_id = j.id
		             AND accepted_task.status = 'DONE'
		             AND f.attempt_no = accepted_task.attempts
		       ),
		       j.new_rcno_count
		FROM jobs AS j
		LEFT JOIN tasks AS t ON t.job_id = j.id
		WHERE j.id = ?
		GROUP BY j.id
	`, jobID).Scan(&result.RunID, &result.Status, &result.TotalPartitions,
		&result.CompletedPartitions, &failed, &result.FetchedPages, &result.ParsedRows,
		&result.UniqueRCNOCount, &result.NewRCNOCount)
	if err != nil {
		return result, fmt.Errorf("Job 결과 조회 실패: %w", err)
	}
	result.FailedPartitions = failed
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, process_date, status, fetched_requests, parsed_rows,
		       unique_rcno_count, new_rcno_count
		FROM tasks WHERE job_id = ? ORDER BY process_date, id
	`, jobID)
	if err != nil {
		return result, fmt.Errorf("Job Task 결과 조회 실패: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var unit weblist.Result
		if err := rows.Scan(&unit.PartitionID, &unit.ProcessDate, &unit.PartitionStatus,
			&unit.FetchedPages, &unit.ParsedRows, &unit.UniqueRCNOCount,
			&unit.NewRCNOCount); err != nil {
			return result, err
		}
		unit.RunID, unit.RunStatus = jobID, result.Status
		result.Units = append(result.Units, unit)
	}
	return result, rows.Err()
}

func lockTaskLease(ctx context.Context, tx *sql.Tx, task weblist.DateTask) error {
	var exists uint8
	err := tx.QueryRowContext(ctx, `
		SELECT 1 FROM tasks
		WHERE id = ? AND job_id = ? AND status = 'LEASED'
		  AND lease_owner = ? AND attempts = ? AND lease_until >= NOW(6)
		FOR UPDATE
	`, task.TaskID, task.RunID, task.Owner, task.Attempt).Scan(&exists)
	if errors.Is(err, sql.ErrNoRows) {
		return weblist.ErrLeaseLost
	}
	if err != nil {
		return fmt.Errorf("Task lease 확인 실패: %w", err)
	}
	return nil
}

func refreshTaskCounters(ctx context.Context, tx *sql.Tx, taskID uint64) error {
	_, err := tx.ExecContext(ctx, `
		UPDATE tasks AS t
		SET fetched_requests = (SELECT COUNT(*) FROM fetches f WHERE f.task_id = t.id),
		    parsed_rows = (
		        SELECT COUNT(*)
		        FROM items i
		        JOIN fetches f ON f.id = i.fetch_id
		        WHERE i.task_id = t.id AND f.attempt_no = t.attempts
		    ),
		    unique_rcno_count = (
		        SELECT COUNT(DISTINCT i.rcno)
		        FROM items i
		        JOIN fetches f ON f.id = i.fetch_id
		        WHERE i.task_id = t.id AND f.attempt_no = t.attempts
		    ),
		    new_rcno_count = (
		        SELECT COUNT(DISTINCT i.rcno)
		        FROM items i
		        JOIN fetches f ON f.id = i.fetch_id
		        WHERE i.task_id = t.id AND f.attempt_no = t.attempts
		          AND NOT EXISTS (
		              SELECT 1
		              FROM items earlier
		              WHERE earlier.rcno = i.rcno
		                AND earlier.id < i.id
		                AND earlier.task_id <> t.id
		          )
		    )
		WHERE t.id = ?
	`, taskID)
	if err != nil {
		return fmt.Errorf("Task counter 갱신 실패: %w", err)
	}
	return nil
}

func refreshJobCounters(ctx context.Context, tx *sql.Tx, jobID uint64) error {
	_, err := tx.ExecContext(ctx, `
		UPDATE jobs AS j
		SET total_tasks = (SELECT COUNT(*) FROM tasks t WHERE t.job_id = j.id),
		    completed_tasks = (SELECT COUNT(*) FROM tasks t WHERE t.job_id = j.id AND t.status = 'DONE'),
		    fetched_requests = (SELECT COUNT(*) FROM fetches f WHERE f.job_id = j.id),
		    parsed_rows = (
		        SELECT COUNT(*)
		        FROM items i
		        JOIN fetches f ON f.id = i.fetch_id
		        JOIN tasks t ON t.id = i.task_id
		        WHERE i.job_id = j.id
		          AND t.status = 'DONE'
		          AND f.attempt_no = t.attempts
		    ),
		    new_rcno_count = (
		        SELECT COUNT(DISTINCT i.rcno)
		        FROM items i
		        JOIN fetches f ON f.id = i.fetch_id
		        JOIN tasks t ON t.id = i.task_id
		        WHERE i.job_id = j.id
		          AND t.status = 'DONE'
		          AND f.attempt_no = t.attempts
		          AND NOT EXISTS (
		              SELECT 1
		              FROM items earlier
		              JOIN fetches earlier_fetch ON earlier_fetch.id = earlier.fetch_id
		              JOIN tasks earlier_task ON earlier_task.id = earlier.task_id
		              WHERE earlier.rcno = i.rcno
		                AND earlier.id < i.id
		                AND earlier_task.status = 'DONE'
		                AND earlier_fetch.attempt_no = earlier_task.attempts
		          )
		    )
		WHERE j.id = ?
	`, jobID)
	if err != nil {
		return fmt.Errorf("Job counter 갱신 실패: %w", err)
	}
	return nil
}

func (s *Store) withTx(ctx context.Context, operation func(*sql.Tx) error) (err error) {
	for attempt := 1; attempt <= 3; attempt++ {
		tx, beginErr := s.db.BeginTx(ctx, nil)
		if beginErr != nil {
			return fmt.Errorf("transaction 시작 실패: %w", beginErr)
		}
		err = operation(tx)
		if err == nil {
			err = tx.Commit()
		} else {
			err = errors.Join(err, tx.Rollback())
		}
		if err == nil || !isRetryableTransactionError(err) || attempt == 3 {
			return err
		}
	}
	return err
}

func requireOne(result sql.Result, operation string) error {
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("%s 행 수 확인 실패: %w", operation, err)
	}
	if affected != 1 {
		return fmt.Errorf("%s 행 수가 유효하지 않습니다: %d", operation, affected)
	}
	return nil
}

func requireLease(result sql.Result, operation string) error {
	if err := requireOne(result, operation); err != nil {
		return fmt.Errorf("%w: %v", weblist.ErrLeaseLost, err)
	}
	return nil
}

func fallback(value, defaultValue string) string {
	if value == "" {
		return defaultValue
	}
	return value
}

func validJSON(value []byte, defaultValue string) []byte {
	if json.Valid(value) {
		return value
	}
	return []byte(defaultValue)
}

func nullableJSON(value []byte) any {
	if json.Valid(value) {
		return value
	}
	return nil
}

func nullString(value string) any {
	if value == "" {
		return nil
	}
	return value
}

func nullTime(value time.Time) any {
	if value.IsZero() {
		return nil
	}
	return value
}

func nullBytes(value []byte) any {
	if len(value) == 0 {
		return nil
	}
	return value
}

func nullHash(value [32]byte, valid bool) any {
	if !valid {
		return nil
	}
	return value[:]
}

func nullInt16(value int) any {
	if value == 0 {
		return nil
	}
	return value
}

func nullInt32(value int64) any {
	if value == 0 {
		return nil
	}
	return value
}

func nullInt64(value int64) any {
	if value == 0 {
		return nil
	}
	return value
}

func durationMillis(value time.Duration) any {
	if value <= 0 {
		return nil
	}
	return value.Milliseconds()
}

// LoadOverview, LoadRunDetail, LoadEvents, LoadPageItems, LoadObservations는
// TUI가 jobs/tasks/fetches/items를 polling하기 위한 읽기 모델입니다.
func (s *Store) LoadOverview(ctx context.Context, limit int32) (snapshot overview.Snapshot, err error) {
	err = s.db.QueryRowContext(ctx, `
		SELECT COUNT(*), SUM(status IN ('CREATED','RUNNING')),
		       SUM(status = 'COMPLETED'), SUM(status IN ('PARTIAL_FAILED','CANCELLED')),
		       (SELECT COUNT(*) FROM tasks WHERE status = 'FAILED'),
		       (SELECT COUNT(*) FROM items), (SELECT COUNT(DISTINCT rcno) FROM items)
		FROM jobs
	`).Scan(&snapshot.TotalRuns, &snapshot.ActiveRuns, &snapshot.CompletedRuns,
		&snapshot.FailedRuns, &snapshot.DirtyPartitions, &snapshot.ListRawRows,
		&snapshot.UniqueRCNO)
	if err != nil {
		return snapshot, fmt.Errorf("overview 집계 실패: %w", err)
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, job_type, requested_from_date, requested_to_date, status,
		       total_tasks, completed_tasks, parsed_rows, new_rcno_count,
		       COALESCE(last_error,''), created_at, finished_at
		FROM jobs ORDER BY id DESC LIMIT ?
	`, limit)
	if err != nil {
		return snapshot, err
	}
	defer rows.Close()
	for rows.Next() {
		var run overview.Run
		var finished sql.NullTime
		if err := rows.Scan(&run.ID, &run.RunType, &run.RequestedFromDate,
			&run.RequestedToDate, &run.Status, &run.TotalPartitions,
			&run.CompletedPartitions, &run.ParsedRows, &run.NewRCNOCount,
			&run.LastError, &run.CreatedAt, &finished); err != nil {
			return snapshot, err
		}
		if finished.Valid {
			run.FinishedAt = &finished.Time
		}
		snapshot.RecentRuns = append(snapshot.RecentRuns, run)
	}
	return snapshot, rows.Err()
}

func (s *Store) LoadRunDetail(ctx context.Context, jobID uint64) (detail overview.RunDetail, err error) {
	err = s.db.QueryRowContext(ctx, `
		SELECT id, status, total_tasks, completed_tasks, fetched_requests,
		       parsed_rows, (SELECT COUNT(*) FROM tasks WHERE job_id = jobs.id AND status='FAILED'),
		       (SELECT COUNT(*) FROM tasks WHERE job_id = jobs.id AND status IN ('PENDING','LEASED','RETRY_WAIT'))
		FROM jobs WHERE id = ?
	`, jobID).Scan(&detail.RunID, &detail.Status, &detail.TotalPartitions,
		&detail.CompletedPartitions, &detail.FetchedRequests, &detail.ParsedRows,
		&detail.DirtyPartitions, &detail.PendingPages)
	if err != nil {
		return detail, err
	}
	taskRows, err := s.db.QueryContext(ctx, `
		SELECT id, process_date, status, parsed_rows, unique_rcno_count,
		       attempts, COALESCE(last_error,'')
		FROM tasks WHERE job_id = ? ORDER BY process_date, id
	`, jobID)
	if err != nil {
		return detail, err
	}
	for taskRows.Next() {
		var task overview.Partition
		task.ItemName = "위스키·브랜디·일반증류주·리큐르"
		if err := taskRows.Scan(&task.ID, &task.ProcessDate, &task.Status,
			&task.ParsedRows, &task.UniqueRCNOCount, &task.Attempts,
			&task.LastError); err != nil {
			taskRows.Close()
			return detail, err
		}
		detail.Partitions = append(detail.Partitions, task)
	}
	if err := taskRows.Close(); err != nil {
		return detail, err
	}
	fetchRows, err := s.db.QueryContext(ctx, `
		SELECT f.id, f.task_id, f.item_name, f.item_code, t.process_date,
		       f.page_no, f.status, '', f.total_snapshot, f.parsed_item_count,
		       f.parsed_item_count, f.attempt_no, COALESCE(f.error_message,'')
		FROM fetches f JOIN tasks t ON t.id=f.task_id
		WHERE f.job_id=? ORDER BY t.process_date, f.item_code, f.page_no, f.attempt_no
	`, jobID)
	if err != nil {
		return detail, err
	}
	defer fetchRows.Close()
	for fetchRows.Next() {
		var page overview.Page
		if err := fetchRows.Scan(&page.ID, &page.PartitionID, &page.ItemName,
			&page.ItemCode, &page.ProcessDate, &page.PageNo, &page.Status,
			&page.WorkerID, &page.TotalSnapshot, &page.RowCount,
			&page.UniqueRCNOCount, &page.Attempts, &page.LastError); err != nil {
			return detail, err
		}
		detail.Pages = append(detail.Pages, page)
	}
	return detail, fetchRows.Err()
}

func (s *Store) LoadEvents(
	ctx context.Context,
	jobID uint64,
	_ string,
	afterID uint64,
	limit int32,
) (page operator.EventPage, err error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, job_id, task_id, item_name, page_no, status,
		       COALESCE(error_message,''), created_at
		FROM fetches WHERE job_id=? AND id>? ORDER BY id LIMIT ?
	`, jobID, afterID, limit)
	if err != nil {
		return page, err
	}
	defer rows.Close()
	for rows.Next() {
		var event operator.Event
		var itemName, status string
		var pageNo uint32
		if err := rows.Scan(&event.ID, &event.RunID, &event.PartitionID,
			&itemName, &pageNo, &status, &event.Message, &event.CreatedAt); err != nil {
			return page, err
		}
		event.PageID = event.ID
		event.Level = "INFO"
		if status == "FAILED" || status == "PARSE_FAILED" {
			event.Level = "ERROR"
		}
		event.Phase = status
		if event.Message == "" {
			event.Message = fmt.Sprintf("%s page %d %s", itemName, pageNo, status)
		}
		page.Events = append(page.Events, event)
	}
	return page, rows.Err()
}

func (s *Store) LoadPageItems(ctx context.Context, fetchID uint64) ([]operator.Item, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, job_id, task_id, fetch_id, row_no, rcno,
		       queried_item_code, queried_item_name, COALESCE(product_name_ko,''),
		       COALESCE(product_name_en,''), COALESCE(importer_name,''),
		       COALESCE(manufacture_country_name,''), processed_date, observed_at
		FROM items WHERE fetch_id=? ORDER BY row_no, id
	`, fetchID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []operator.Item
	for rows.Next() {
		var item operator.Item
		var date sql.NullTime
		if err := rows.Scan(&item.ID, &item.RunID, &item.PartitionID, &item.FetchID,
			&item.RowNo, &item.RCNO, &item.ItemCode, &item.ItemName,
			&item.ProductNameKO, &item.ProductNameEN, &item.ImporterName,
			&item.CountryName, &date, &item.ObservedAt); err != nil {
			return nil, err
		}
		item.PageID = item.FetchID
		if date.Valid {
			item.ProcessedDate = &date.Time
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

func (s *Store) LoadObservations(
	ctx context.Context,
	filters ledger.Filters,
	beforeID uint64,
	limit int32,
) (page ledger.Page, err error) {
	query := `
		SELECT id, job_id, task_id, fetch_id, rcno, queried_item_code,
		       queried_item_name, COALESCE(product_name_ko,''),
		       COALESCE(product_name_en,''), COALESCE(importer_name,''),
		       COALESCE(manufacture_country_name,''), processed_date,
		       semantic_sha256, parser_version, COALESCE(parser_warning,''), observed_at
		FROM items
		WHERE id < ? AND (? = '' OR queried_item_code = ?)
		  AND (? = '' OR rcno = ?)
		  AND (? IS NULL OR processed_date >= ?)
		  AND (? IS NULL OR processed_date <= ?)
		ORDER BY id DESC LIMIT ?`
	rows, err := s.db.QueryContext(ctx, query, beforeID,
		filters.ItemCode, filters.ItemCode, filters.RCNO, filters.RCNO,
		filters.FromDate, filters.FromDate, filters.ToDate, filters.ToDate, limit)
	if err != nil {
		return page, err
	}
	defer rows.Close()
	for rows.Next() {
		var item ledger.Observation
		var date sql.NullTime
		if err := rows.Scan(&item.ID, &item.RunID, &item.PartitionID, &item.FetchID,
			&item.RCNO, &item.ItemCode, &item.ItemName, &item.ProductNameKO,
			&item.ProductNameEN, &item.ImporterName, &item.CountryName, &date,
			&item.SemanticSHA256, &item.ParserVersion, &item.ParserWarning,
			&item.ObservedAt); err != nil {
			return page, err
		}
		item.PageID = item.FetchID
		if date.Valid {
			item.ProcessedDate = &date.Time
		}
		page.Items = append(page.Items, item)
	}
	return page, rows.Err()
}

var (
	_ weblist.Store   = (*Store)(nil)
	_ overview.Reader = (*Store)(nil)
	_ operator.Reader = (*Store)(nil)
	_ ledger.Reader   = (*Store)(nil)
)
