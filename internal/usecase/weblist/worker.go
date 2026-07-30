//go:build legacy

package weblist

import (
	"context"
	"errors"
	"fmt"
	"time"
)

func (s *Service) runWorker(
	ctx context.Context,
	runID uint64,
	workerIndex int,
	results chan<- Result,
) error {
	owner := fmt.Sprintf("web-list/%d/%s/%02d", runID, s.processID, workerIndex)
	var resultErr error
	cancellationRequested := false
	for {
		if ctx.Err() != nil && !cancellationRequested {
			requestCtx, cancel := detachedFailureContext(ctx)
			err := s.store.RequestCancellation(requestCtx, runID)
			cancel()
			if err != nil {
				return err
			}
			cancellationRequested = true
		}
		claimCtx, claimCancel := context.WithTimeout(context.WithoutCancel(ctx), failureCleanupTimeout)
		claim := ClaimParams{
			RunID: runID, Owner: owner, Now: s.now(), LeaseUntil: s.now().Add(defaultExecutionLease),
		}
		task, found, err := s.store.ClaimPage(claimCtx, claim)
		claimCancel()
		if err != nil {
			return err
		}
		if found {
			if err := s.processTask(ctx, task); err != nil && !errors.Is(err, ErrLeaseLost) {
				return err
			}
			continue
		}

		claimCtx, claimCancel = context.WithTimeout(context.WithoutCancel(ctx), failureCleanupTimeout)
		task, found, err = s.store.ClaimDiscovery(claimCtx, claim)
		claimCancel()
		if err != nil {
			return err
		}
		if found {
			if err := s.processTask(ctx, task); err != nil && !errors.Is(err, ErrLeaseLost) {
				return err
			}
			continue
		}

		claimCtx, claimCancel = context.WithTimeout(context.WithoutCancel(ctx), failureCleanupTimeout)
		reconcile, found, err := s.store.ClaimReconcile(claimCtx, claim)
		claimCancel()
		if err != nil {
			return err
		}
		if found {
			result, reconcileErr := s.completeReconcile(ctx, reconcile)
			if result.PartitionID != 0 {
				select {
				case results <- result:
				case <-ctx.Done():
					return ctx.Err()
				}
			}
			var mismatch *ReconciliationError
			if reconcileErr != nil && !errors.As(reconcileErr, &mismatch) && !errors.Is(reconcileErr, ErrLeaseLost) {
				return reconcileErr
			}
			if mismatch != nil {
				resultErr = errors.Join(resultErr, reconcileErr)
			}
			finalizeCtx, finalizeCancel := context.WithTimeout(context.WithoutCancel(ctx), failureCleanupTimeout)
			_, _, finalizeErr := s.store.FinalizeJob(finalizeCtx, FinalizeJobParams{
				RunID: runID, Cancelled: ctx.Err() != nil, FinishedAt: s.now(),
			})
			finalizeCancel()
			if finalizeErr != nil {
				return finalizeErr
			}
			continue
		}

		finalizeCtx, finalizeCancel := context.WithTimeout(context.WithoutCancel(ctx), failureCleanupTimeout)
		status, _, finalizeErr := s.store.FinalizeJob(finalizeCtx, FinalizeJobParams{
			RunID: runID, Cancelled: ctx.Err() != nil, FinishedAt: s.now(),
		})
		finalizeCancel()
		if finalizeErr != nil {
			return finalizeErr
		}
		if isTerminalRunStatus(status) {
			return resultErr
		}
		timer := time.NewTimer(workerIdleInterval)
		if cancellationRequested {
			<-timer.C
			continue
		}
		select {
		case <-ctx.Done():
			if !timer.Stop() {
				<-timer.C
			}
		case <-timer.C:
		}
	}
}

func (s *Service) processTask(parent context.Context, task Task) error {
	taskCtx, cancelTask := context.WithCancel(context.WithoutCancel(parent))
	renewErr := make(chan error, 1)
	stopRenew := make(chan struct{})
	go s.renewTaskLeases(taskCtx, task, cancelTask, stopRenew, renewErr)
	defer func() {
		close(stopRenew)
		cancelTask()
	}()

	if err := s.waitForFetchSlot(taskCtx); err != nil {
		return s.handleTaskContextError(task, renewErr, err)
	}
	request := ListRequest{
		ItemName: task.ItemName, ItemCode: task.ItemCode, ProcessDate: task.ProcessDate,
		Page: int(task.Page), Limit: int(task.PageSize), TotalSnapshot: task.TotalSnapshot,
	}
	fetch, fetchErr := s.source.FetchList(taskCtx, request)
	if leaseErr := receiveRenewError(renewErr); leaseErr != nil {
		return s.recordLeaseLost(task, leaseErr)
	}
	if fetchErr != nil {
		kind := s.source.ErrorKind(fetchErr)
		if kind == "" {
			kind = "NETWORK"
		}
		return s.failTask(task, fetch, 0, false, kind, fetchErr)
	}

	fetchID, err := s.store.JournalFetch(taskCtx, JournalFetchParams{
		RunID: task.RunID, PartitionID: task.PartitionID, PageID: task.PageID,
		AttemptNo: task.PageToken, PartitionAttempt: task.PartitionToken,
		Owner: task.Owner, Fetch: fetch,
	})
	if err != nil {
		if errors.Is(err, ErrLeaseLost) {
			return s.recordLeaseLost(task, err)
		}
		return err
	}
	page, parseErr := s.source.ParseList(fetch.Body, request)
	if parseErr != nil {
		return s.failTask(task, fetch, fetchID, true, "PARSE", parseErr)
	}
	if leaseErr := receiveRenewError(renewErr); leaseErr != nil {
		return s.recordLeaseLost(task, leaseErr)
	}
	if err := s.store.CommitPage(taskCtx, CommitPageParams{
		RunID: task.RunID, PartitionID: task.PartitionID, PageID: task.PageID, FetchID: fetchID,
		PartitionAttempt: task.PartitionToken, PageAttempt: task.PageToken,
		Owner: task.Owner, Request: request, Page: page, ObservedAt: fetch.FinishedAt,
	}); err != nil {
		if errors.Is(err, ErrLeaseLost) {
			return s.recordLeaseLost(task, err)
		}
		return s.failTask(task, fetch, fetchID, true, "COMMIT", err)
	}
	return nil
}

func (s *Service) failTask(
	task Task,
	fetch Fetch,
	fetchID uint64,
	raw bool,
	kind string,
	cause error,
) error {
	retryable := retryableFailure(kind, fetch.HTTPStatus)
	failureCtx, cancel := context.WithTimeout(context.Background(), failureCleanupTimeout)
	defer cancel()
	_, err := s.store.FailTask(failureCtx, TaskFailureParams{
		Task: task, FetchID: fetchID, Fetch: fetch, PageWasRaw: raw,
		Retryable: retryable, ParseFailure: kind == "PARSE",
		MaxAttempts: s.maxAttempts, NextAttemptAt: s.now().Add(s.retryDelay(task.PageToken)),
		FinishedAt: s.now(), ErrorKind: kind, ErrorMessage: errorText(cause),
	})
	if errors.Is(err, ErrLeaseLost) {
		return s.recordLeaseLost(task, err)
	}
	if err != nil {
		return errors.Join(cause, err)
	}
	return nil
}

func (s *Service) completeReconcile(
	parent context.Context,
	task ReconcileTask,
) (Result, error) {
	taskCtx, cancelTask := context.WithCancel(context.WithoutCancel(parent))
	defer cancelTask()
	renewErr := make(chan error, 1)
	stopRenew := make(chan struct{})
	go s.renewPartitionLease(taskCtx, task, cancelTask, stopRenew, renewErr)
	result, err := s.store.CompleteReconcile(taskCtx, ReconcileCompleteParams{ReconcileTask: task})
	close(stopRenew)
	if leaseErr := receiveRenewError(renewErr); leaseErr != nil {
		return Result{}, s.recordReconcileLeaseLost(task, leaseErr)
	}
	if errors.Is(err, ErrLeaseLost) {
		return Result{}, s.recordReconcileLeaseLost(task, err)
	}
	return result, err
}

func (s *Service) renewTaskLeases(
	ctx context.Context,
	task Task,
	cancel context.CancelFunc,
	stop <-chan struct{},
	errs chan<- error,
) {
	ticker := time.NewTicker(leaseRenewInterval)
	defer ticker.Stop()
	for {
		select {
		case <-stop:
			return
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := s.store.RenewPage(ctx, RenewParams{
				ID: task.PageID, RunID: task.RunID, PartitionID: task.PartitionID,
				PageID: task.PageID, Owner: task.Owner, AttemptToken: task.PageToken,
				LeaseUntil: s.now().Add(defaultExecutionLease),
			}); err != nil {
				errs <- err
				cancel()
				return
			}
			if task.Discovery {
				if err := s.store.RenewPartition(ctx, RenewParams{
					ID: task.PartitionID, RunID: task.RunID, PartitionID: task.PartitionID,
					Owner: task.Owner, AttemptToken: task.PartitionToken,
					LeaseUntil: s.now().Add(defaultExecutionLease),
				}); err != nil {
					errs <- err
					cancel()
					return
				}
			}
		}
	}
}

func (s *Service) renewPartitionLease(
	ctx context.Context,
	task ReconcileTask,
	cancel context.CancelFunc,
	stop <-chan struct{},
	errs chan<- error,
) {
	ticker := time.NewTicker(leaseRenewInterval)
	defer ticker.Stop()
	for {
		select {
		case <-stop:
			return
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := s.store.RenewPartition(ctx, RenewParams{
				ID: task.PartitionID, RunID: task.RunID, PartitionID: task.PartitionID,
				Owner: task.Owner, AttemptToken: task.PartitionToken,
				LeaseUntil: s.now().Add(defaultExecutionLease),
			}); err != nil {
				errs <- err
				cancel()
				return
			}
		}
	}
}

func (s *Service) recordLeaseLost(task Task, cause error) error {
	ctx, cancel := context.WithTimeout(context.Background(), failureCleanupTimeout)
	defer cancel()
	s.store.AppendLeaseLostEvent(ctx, EventParams{
		RunID: task.RunID, PartitionID: task.PartitionID, PageID: task.PageID,
		WorkerID: task.Owner, Level: "WARN", Phase: "LEASE_LOST", Message: errorText(cause),
	})
	return fmt.Errorf("%w: %v", ErrLeaseLost, cause)
}

func (s *Service) recordReconcileLeaseLost(task ReconcileTask, cause error) error {
	ctx, cancel := context.WithTimeout(context.Background(), failureCleanupTimeout)
	defer cancel()
	s.store.AppendLeaseLostEvent(ctx, EventParams{
		RunID: task.RunID, PartitionID: task.PartitionID, WorkerID: task.Owner,
		Level: "WARN", Phase: "LEASE_LOST", Message: errorText(cause),
	})
	return fmt.Errorf("%w: %v", ErrLeaseLost, cause)
}

func (s *Service) handleTaskContextError(task Task, renewErr <-chan error, cause error) error {
	if leaseErr := receiveRenewError(renewErr); leaseErr != nil {
		return s.recordLeaseLost(task, leaseErr)
	}
	return cause
}

func (s *Service) retryDelay(attempt uint32) time.Duration {
	index := int(attempt) - 1
	if index < 0 {
		index = 0
	}
	if index >= len(s.retryDelays) {
		index = len(s.retryDelays) - 1
	}
	return s.retryDelays[index]
}

func retryableFailure(kind string, status int) bool {
	switch kind {
	case "NETWORK", "COMMIT":
		return true
	case "HTTP":
		switch status {
		case 408, 425, 429, 500, 502, 503, 504:
			return true
		}
	}
	return false
}

func receiveRenewError(errs <-chan error) error {
	select {
	case err := <-errs:
		return err
	default:
		return nil
	}
}

func isTerminalRunStatus(status string) bool {
	return status == RunStatusCompleted || status == RunStatusPartialFailed || status == RunStatusCancelled
}
