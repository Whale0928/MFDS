package weblist

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
)

type TaskReconciliation struct {
	Complete bool
	Reason   string
}

type itemAttemptResult struct {
	valid         bool
	multiPage     bool
	total         uint64
	parsedRows    uint64
	uniqueRCNOs   []string
	rcnoSetSHA256 string
	reason        string
}

func ReconcileTaskAttempt(
	evidence []TaskAttemptEvidence,
	currentAttempt uint32,
	targets []Target,
	pageSize int,
) TaskReconciliation {
	attempts := make(map[uint32]map[string]AttemptItemEvidence, len(evidence))
	for _, attempt := range evidence {
		items := make(map[string]AttemptItemEvidence, len(attempt.Items))
		for _, item := range attempt.Items {
			items[item.ItemCode] = item
		}
		attempts[attempt.Attempt] = items
	}

	currentItems := attempts[currentAttempt]
	currentResults := make(map[string]itemAttemptResult, len(targets))
	for _, target := range targets {
		result := reconcileItemAttempt(currentItems[target.Code], pageSize)
		if !result.valid {
			return TaskReconciliation{
				Reason: fmt.Sprintf(
					"RCNO_RECONCILIATION_FAILED item=%s item_code=%s attempt=%d %s",
					target.Name,
					target.Code,
					currentAttempt,
					result.reason,
				),
			}
		}
		currentResults[target.Code] = result
	}

	requiresStableScan := false
	for _, result := range currentResults {
		requiresStableScan = requiresStableScan || result.multiPage
	}
	if !requiresStableScan {
		return TaskReconciliation{Complete: true}
	}

	for _, target := range targets {
		current := currentResults[target.Code]
		if !current.multiPage {
			continue
		}
		previous, found := latestValidItemAttempt(
			attempts,
			currentAttempt,
			target.Code,
			pageSize,
		)
		if !found {
			return TaskReconciliation{
				Reason: fmt.Sprintf(
					"RCNO_VERIFICATION_REQUIRED item=%s item_code=%s attempt=%d total=%d unique_rcno=%d hash=%s",
					target.Name,
					target.Code,
					currentAttempt,
					current.total,
					len(current.uniqueRCNOs),
					current.rcnoSetSHA256,
				),
			}
		}
		if previous.total != current.total ||
			previous.rcnoSetSHA256 != current.rcnoSetSHA256 {
			missing, unexpected := rcnoSetDifference(previous.uniqueRCNOs, current.uniqueRCNOs)
			return TaskReconciliation{
				Reason: fmt.Sprintf(
					"RCNO_SET_CHANGED item=%s item_code=%s attempt=%d previous_total=%d current_total=%d missing=%d unexpected=%d previous_hash=%s current_hash=%s",
					target.Name,
					target.Code,
					currentAttempt,
					previous.total,
					current.total,
					len(missing),
					len(unexpected),
					previous.rcnoSetSHA256,
					current.rcnoSetSHA256,
				),
			}
		}
	}
	return TaskReconciliation{Complete: true}
}

func reconcileItemAttempt(item AttemptItemEvidence, pageSize int) itemAttemptResult {
	if pageSize < 1 {
		return itemAttemptResult{reason: "page_size가 1보다 작음"}
	}
	if len(item.Pages) == 0 {
		return itemAttemptResult{reason: "fetch page가 없음"}
	}

	pages := append([]AttemptPageEvidence(nil), item.Pages...)
	sort.Slice(pages, func(left, right int) bool {
		return pages[left].PageNo < pages[right].PageNo
	})
	total := pages[0].Total
	requiredPages := uint64(1)
	if total > 0 {
		requiredPages = (total + uint64(pageSize) - 1) / uint64(pageSize)
	}
	if uint64(len(pages)) != requiredPages {
		return itemAttemptResult{
			reason: fmt.Sprintf(
				"required_pages=%d fetched_pages=%d",
				requiredPages,
				len(pages),
			),
		}
	}

	var parsedRows uint64
	for index, page := range pages {
		expectedPage := uint32(index + 1)
		if page.PageNo != expectedPage {
			return itemAttemptResult{
				reason: fmt.Sprintf(
					"expected_page=%d actual_page=%d",
					expectedPage,
					page.PageNo,
				),
			}
		}
		if page.Status != "PARSED" {
			return itemAttemptResult{
				reason: fmt.Sprintf("page=%d status=%s", page.PageNo, page.Status),
			}
		}
		if page.Total != total {
			return itemAttemptResult{
				reason: fmt.Sprintf(
					"page=%d first_total=%d page_total=%d",
					page.PageNo,
					total,
					page.Total,
				),
			}
		}
		parsedRows += uint64(page.ParsedRows)
	}
	if parsedRows != total {
		return itemAttemptResult{
			reason: fmt.Sprintf("total=%d parsed_rows=%d", total, parsedRows),
		}
	}
	if uint64(len(item.RCNOs)) != parsedRows {
		return itemAttemptResult{
			reason: fmt.Sprintf(
				"parsed_rows=%d stored_rows=%d",
				parsedRows,
				len(item.RCNOs),
			),
		}
	}

	uniqueRCNOs := sortedUniqueStrings(item.RCNOs)
	if uint64(len(uniqueRCNOs)) != total {
		return itemAttemptResult{
			reason: fmt.Sprintf(
				"total=%d parsed_rows=%d unique_rcno=%d duplicate_rows=%d",
				total,
				parsedRows,
				len(uniqueRCNOs),
				len(item.RCNOs)-len(uniqueRCNOs),
			),
		}
	}
	hash := sha256.Sum256([]byte(strings.Join(uniqueRCNOs, "\n")))
	return itemAttemptResult{
		valid:         true,
		multiPage:     requiredPages > 1,
		total:         total,
		parsedRows:    parsedRows,
		uniqueRCNOs:   uniqueRCNOs,
		rcnoSetSHA256: hex.EncodeToString(hash[:]),
	}
}

func latestValidItemAttempt(
	attempts map[uint32]map[string]AttemptItemEvidence,
	currentAttempt uint32,
	itemCode string,
	pageSize int,
) (itemAttemptResult, bool) {
	for attempt := int(currentAttempt) - 1; attempt >= 1; attempt-- {
		item, exists := attempts[uint32(attempt)][itemCode]
		if !exists {
			continue
		}
		result := reconcileItemAttempt(item, pageSize)
		if result.valid {
			return result, true
		}
	}
	return itemAttemptResult{}, false
}

func sortedUniqueStrings(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		seen[value] = struct{}{}
	}
	result := make([]string, 0, len(seen))
	for value := range seen {
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}

func rcnoSetDifference(previous, current []string) (missing, unexpected []string) {
	previousSet := make(map[string]struct{}, len(previous))
	currentSet := make(map[string]struct{}, len(current))
	for _, value := range previous {
		previousSet[value] = struct{}{}
	}
	for _, value := range current {
		currentSet[value] = struct{}{}
	}
	for value := range previousSet {
		if _, exists := currentSet[value]; !exists {
			missing = append(missing, value)
		}
	}
	for value := range currentSet {
		if _, exists := previousSet[value]; !exists {
			unexpected = append(unexpected, value)
		}
	}
	sort.Strings(missing)
	sort.Strings(unexpected)
	return missing, unexpected
}
