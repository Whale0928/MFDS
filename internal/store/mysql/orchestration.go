//go:build legacy

package mysql

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/bottle-note/mfds-crawler/internal/usecase/weblist"
	"github.com/bottle-note/mfds-crawler/secrets/project/mfds/sqlcgen"
)

const claimCandidateLimit = 64

var errClaimRace = errors.New("claim candidate unavailable")

func (s *Store) ClaimDiscovery(
	ctx context.Context,
	params weblist.ClaimParams,
) (task weblist.Task, found bool, err error) {
	candidates, err := sqlcgen.New(s.db).ListPartitionLeaseCandidates(
		ctx,
		sqlcgen.ListPartitionLeaseCandidatesParams{
			RunID: params.RunID, Limit: claimCandidateLimit,
		},
	)
	if err != nil {
		return task, false, fmt.Errorf("discovery partition candidate 조회 실패: %w", err)
	}
	for _, candidateID := range candidates {
		err = s.withTx(ctx, func(q *sqlcgen.Queries) error {
			partition, findErr := q.LockPartitionLeaseCandidate(
				ctx,
				sqlcgen.LockPartitionLeaseCandidateParams{
					ID: candidateID, RunID: params.RunID,
				},
			)
			if errors.Is(findErr, sql.ErrNoRows) {
				return errClaimRace
			}
			if findErr != nil {
				return fmt.Errorf("discovery partition claim 조회 실패: %w", findErr)
			}
			if _, ensureErr := q.EnsureCrawlRunListing(ctx, sqlcgen.EnsureCrawlRunListingParams{
				StartedAt: nullTime(params.Now), ID: params.RunID,
			}); ensureErr != nil {
				return fmt.Errorf("discovery run LISTING 전이 실패: %w", ensureErr)
			}
			affected, leaseErr := q.MarkPartitionLeased(ctx, sqlcgen.MarkPartitionLeasedParams{
				LeaseOwner: nullString(params.Owner), LeaseUntil: nullTime(params.LeaseUntil), ID: partition.ID,
			})
			if leaseErr != nil {
				return fmt.Errorf("discovery partition lease 실패: %w", leaseErr)
			}
			if affected != 1 {
				return errClaimRace
			}
			partitionToken := partition.Attempts + 1
			pageID, createErr := q.CreateCrawlPage(ctx, sqlcgen.CreateCrawlPageParams{
				PartitionID: partition.ID, PageNo: 1,
			})
			if createErr != nil || pageID <= 0 {
				return fmt.Errorf("discovery page 1 생성 실패: id=%d: %w", pageID, createErr)
			}
			pageBefore, getErr := q.GetCrawlPageForClaim(ctx, uint64(pageID))
			if getErr != nil {
				return fmt.Errorf("discovery page 1 token 조회 실패: %w", getErr)
			}
			affected, leaseErr = q.MarkPageLeased(ctx, sqlcgen.MarkPageLeasedParams{
				LeaseOwner: nullString(params.Owner), LeaseUntil: nullTime(params.LeaseUntil), ID: uint64(pageID),
			})
			if leaseErr != nil {
				return fmt.Errorf("discovery page 1 lease 실패: %w", leaseErr)
			}
			if affected != 1 {
				return errClaimRace
			}
			pageToken := pageBefore.Attempts + 1
			if eventErr := appendEvent(ctx, q, weblist.EventParams{
				RunID: params.RunID, PartitionID: partition.ID, PageID: uint64(pageID),
				WorkerID: params.Owner, Level: "INFO", Phase: "DISPATCHED",
				Message: "discovery page 할당",
				MetadataJSON: eventJSON(map[string]any{
					"status": "LEASED", "attempt": pageToken, "owner": params.Owner, "page_no": 1,
				}),
			}); eventErr != nil {
				return eventErr
			}
			if eventErr := appendEvent(ctx, q, weblist.EventParams{
				RunID: params.RunID, PartitionID: partition.ID, PageID: uint64(pageID),
				WorkerID: params.Owner, Level: "INFO", Phase: "REQUESTING",
				Message: "discovery page 요청 준비",
				MetadataJSON: eventJSON(map[string]any{
					"status": "LEASED", "attempt": pageToken, "owner": params.Owner, "page_no": 1,
				}),
			}); eventErr != nil {
				return eventErr
			}
			task = weblist.Task{
				RunID: params.RunID, PartitionID: partition.ID, PageID: uint64(pageID),
				PartitionToken: partitionToken, PageToken: pageToken,
				Owner: params.Owner, LeaseUntil: params.LeaseUntil,
				ItemName: partition.ItemName, ItemCode: partition.ItemCode,
				ProcessDate: partition.ProcessDate, Page: 1, PageSize: partition.PageSize,
				Discovery: true,
			}
			found = true
			return nil
		})
		if errors.Is(err, errClaimRace) {
			continue
		}
		return task, found, err
	}
	return task, false, nil
}

func (s *Store) ClaimPage(
	ctx context.Context,
	params weblist.ClaimParams,
) (task weblist.Task, found bool, err error) {
	candidates, err := sqlcgen.New(s.db).ListPageLeaseCandidates(
		ctx,
		sqlcgen.ListPageLeaseCandidatesParams{
			RunID: params.RunID, Limit: claimCandidateLimit,
		},
	)
	if err != nil {
		return task, false, fmt.Errorf("page candidate 조회 실패: %w", err)
	}
	for _, candidateID := range candidates {
		err = s.withTx(ctx, func(q *sqlcgen.Queries) error {
			pageBefore, findErr := q.LockPageLeaseCandidate(ctx, candidateID)
			if errors.Is(findErr, sql.ErrNoRows) {
				return errClaimRace
			}
			if findErr != nil {
				return fmt.Errorf("page claim 조회 실패: %w", findErr)
			}
			page, metadataErr := q.GetPageClaimMetadata(ctx, sqlcgen.GetPageClaimMetadataParams{
				ID: candidateID, RunID: params.RunID,
			})
			if errors.Is(metadataErr, sql.ErrNoRows) {
				return errClaimRace
			}
			if metadataErr != nil {
				return fmt.Errorf("page claim metadata 조회 실패: %w", metadataErr)
			}
			affected, leaseErr := q.MarkPageLeased(ctx, sqlcgen.MarkPageLeasedParams{
				LeaseOwner: nullString(params.Owner), LeaseUntil: nullTime(params.LeaseUntil), ID: page.ID,
			})
			if leaseErr != nil {
				return fmt.Errorf("page lease 실패: %w", leaseErr)
			}
			if affected != 1 {
				return errClaimRace
			}
			token := pageBefore.Attempts + 1
			totalSnapshot := (*int)(nil)
			if page.ExpectedTotal.Valid {
				value := int(page.ExpectedTotal.Int64)
				totalSnapshot = &value
			}
			if eventErr := appendEvent(ctx, q, weblist.EventParams{
				RunID: params.RunID, PartitionID: page.PartitionID, PageID: page.ID,
				WorkerID: params.Owner, Level: "INFO", Phase: "DISPATCHED",
				Message: fmt.Sprintf("page %d 할당", page.PageNo),
				MetadataJSON: eventJSON(map[string]any{
					"status": "LEASED", "attempt": token, "owner": params.Owner, "page_no": page.PageNo,
				}),
			}); eventErr != nil {
				return eventErr
			}
			if eventErr := appendEvent(ctx, q, weblist.EventParams{
				RunID: params.RunID, PartitionID: page.PartitionID, PageID: page.ID,
				WorkerID: params.Owner, Level: "INFO", Phase: "REQUESTING",
				Message: fmt.Sprintf("page %d 요청 준비", page.PageNo),
				MetadataJSON: eventJSON(map[string]any{
					"status": "LEASED", "attempt": token, "owner": params.Owner, "page_no": page.PageNo,
				}),
			}); eventErr != nil {
				return eventErr
			}
			task = weblist.Task{
				RunID: params.RunID, PartitionID: page.PartitionID, PageID: page.ID,
				PageToken: token, Owner: params.Owner, LeaseUntil: params.LeaseUntil,
				ItemName: page.ItemName, ItemCode: page.ItemCode, ProcessDate: page.ProcessDate,
				Page: page.PageNo, PageSize: page.PageSize, TotalSnapshot: totalSnapshot,
			}
			found = true
			return nil
		})
		if errors.Is(err, errClaimRace) {
			continue
		}
		return task, found, err
	}
	return task, false, nil
}

func (s *Store) ClaimReconcile(
	ctx context.Context,
	params weblist.ClaimParams,
) (task weblist.ReconcileTask, found bool, err error) {
	candidates, err := sqlcgen.New(s.db).ListReconciliationCandidates(
		ctx,
		sqlcgen.ListReconciliationCandidatesParams{
			RunID: params.RunID, Limit: claimCandidateLimit,
		},
	)
	if err != nil {
		return task, false, fmt.Errorf("reconciliation candidate 조회 실패: %w", err)
	}
	for _, candidateID := range candidates {
		err = s.withTx(ctx, func(q *sqlcgen.Queries) error {
			partition, findErr := q.LockReconciliationCandidate(
				ctx,
				sqlcgen.LockReconciliationCandidateParams{
					ID: candidateID, RunID: params.RunID,
				},
			)
			if errors.Is(findErr, sql.ErrNoRows) {
				return errClaimRace
			}
			if findErr != nil {
				return fmt.Errorf("reconciliation claim 조회 실패: %w", findErr)
			}
			affected, leaseErr := q.MarkPartitionReconciling(ctx, sqlcgen.MarkPartitionReconcilingParams{
				LeaseOwner: nullString(params.Owner), LeaseUntil: nullTime(params.LeaseUntil), ID: partition.ID,
			})
			if leaseErr != nil {
				return fmt.Errorf("reconciliation lease 실패: %w", leaseErr)
			}
			if affected != 1 {
				return errClaimRace
			}
			token := partition.Attempts + 1
			if eventErr := appendEvent(ctx, q, weblist.EventParams{
				RunID: params.RunID, PartitionID: partition.ID, WorkerID: params.Owner,
				Level: "INFO", Phase: "DISPATCHED", Message: "partition reconciliation 할당",
				MetadataJSON: eventJSON(map[string]any{
					"status": "RECONCILING", "attempt": token, "owner": params.Owner,
				}),
			}); eventErr != nil {
				return eventErr
			}
			task = weblist.ReconcileTask{
				RunID: params.RunID, PartitionID: partition.ID, PartitionToken: token,
				Owner: params.Owner, LeaseUntil: params.LeaseUntil,
			}
			found = true
			return nil
		})
		if errors.Is(err, errClaimRace) {
			continue
		}
		return task, found, err
	}
	return task, false, nil
}

func (s *Store) RenewPage(ctx context.Context, params weblist.RenewParams) error {
	return leaseOne("page lease 갱신", func() (int64, error) {
		return sqlcgen.New(s.db).RenewPageLease(ctx, sqlcgen.RenewPageLeaseParams{
			LeaseUntil: nullTime(params.LeaseUntil), ID: params.ID,
			LeaseOwner: nullString(params.Owner), Attempts: params.AttemptToken,
		})
	})
}

func (s *Store) RenewPartition(ctx context.Context, params weblist.RenewParams) error {
	return leaseOne("partition lease 갱신", func() (int64, error) {
		return sqlcgen.New(s.db).RenewPartitionLease(ctx, sqlcgen.RenewPartitionLeaseParams{
			LeaseUntil: nullTime(params.LeaseUntil), ID: params.ID,
			LeaseOwner: nullString(params.Owner), Attempts: params.AttemptToken,
		})
	})
}

func leaseOne(operation string, execute func() (int64, error)) error {
	affected, err := execute()
	if err != nil {
		return fmt.Errorf("%s 실패: %w", operation, err)
	}
	if affected != 1 {
		return fmt.Errorf("%w: %s affected=%d", weblist.ErrLeaseLost, operation, affected)
	}
	return nil
}
