package mysql

import (
	"sort"

	"github.com/bottle-note/mfds-crawler/internal/usecase/weblist"
)

type taskAttemptEvidenceBuilder struct {
	attempts    map[uint32]*weblist.TaskAttemptEvidence
	itemIndexes map[uint32]map[string]int
}

func newTaskAttemptEvidenceBuilder() *taskAttemptEvidenceBuilder {
	return &taskAttemptEvidenceBuilder{
		attempts:    make(map[uint32]*weblist.TaskAttemptEvidence),
		itemIndexes: make(map[uint32]map[string]int),
	}
}

func (b *taskAttemptEvidenceBuilder) ensureItem(
	attempt uint32,
	itemCode string,
	itemName string,
) *weblist.AttemptItemEvidence {
	evidence, exists := b.attempts[attempt]
	if !exists {
		evidence = &weblist.TaskAttemptEvidence{Attempt: attempt}
		b.attempts[attempt] = evidence
		b.itemIndexes[attempt] = make(map[string]int)
	}
	index, exists := b.itemIndexes[attempt][itemCode]
	if !exists {
		index = len(evidence.Items)
		evidence.Items = append(evidence.Items, weblist.AttemptItemEvidence{
			ItemCode: itemCode,
			ItemName: itemName,
		})
		b.itemIndexes[attempt][itemCode] = index
	}
	return &evidence.Items[index]
}

func (b *taskAttemptEvidenceBuilder) result() []weblist.TaskAttemptEvidence {
	result := make([]weblist.TaskAttemptEvidence, 0, len(b.attempts))
	for _, evidence := range b.attempts {
		sort.Slice(evidence.Items, func(left, right int) bool {
			return evidence.Items[left].ItemCode < evidence.Items[right].ItemCode
		})
		result = append(result, *evidence)
	}
	sort.Slice(result, func(left, right int) bool {
		return result[left].Attempt < result[right].Attempt
	})
	return result
}
