package weblist

import (
	"strings"
	"testing"
)

func TestReconcileTaskAttempt_단일페이지4개품목_RCNO가일치하면완료한다(t *testing.T) {
	evidence := []TaskAttemptEvidence{
		attemptEvidence(1, singlePageItems("100000000001")),
	}

	result := ReconcileTaskAttempt(evidence, 1, fixedTargets, 50)

	if !result.Complete || result.Reason != "" {
		t.Fatalf("result = %+v", result)
	}
}

func TestReconcileTaskAttempt_다중페이지첫유효시도_검증재시도를요청한다(t *testing.T) {
	evidence := []TaskAttemptEvidence{
		attemptEvidence(1, withWhiskyPages(singlePageItems("100000000001"),
			[]string{"100000000011", "100000000012", "100000000013"})),
	}

	result := ReconcileTaskAttempt(evidence, 1, fixedTargets, 2)

	if result.Complete || !strings.Contains(result.Reason, "RCNO_VERIFICATION_REQUIRED") {
		t.Fatalf("result = %+v", result)
	}
}

func TestReconcileTaskAttempt_다중페이지연속시도_RCNO집합이같으면완료한다(t *testing.T) {
	first := withWhiskyPages(singlePageItems("100000000001"),
		[]string{"100000000011", "100000000012", "100000000013"})
	second := withWhiskyPages(singlePageItems("200000000001"),
		[]string{"100000000013", "100000000011", "100000000012"})
	evidence := []TaskAttemptEvidence{
		attemptEvidence(1, first),
		attemptEvidence(2, second),
	}

	result := ReconcileTaskAttempt(evidence, 2, fixedTargets, 2)

	if !result.Complete || result.Reason != "" {
		t.Fatalf("result = %+v", result)
	}
}

func TestReconcileTaskAttempt_페이지경계RCNO중복_고유수가부족하면실패한다(t *testing.T) {
	items := withWhiskyPages(singlePageItems("100000000001"),
		[]string{"100000000011", "100000000012", "100000000012"})
	evidence := []TaskAttemptEvidence{attemptEvidence(1, items)}

	result := ReconcileTaskAttempt(evidence, 1, fixedTargets, 2)

	if result.Complete ||
		!strings.Contains(result.Reason, "RCNO_RECONCILIATION_FAILED") ||
		!strings.Contains(result.Reason, "unique_rcno=2") {
		t.Fatalf("result = %+v", result)
	}
}

func TestReconcileTaskAttempt_고유수는같지만RCNO집합이다르면재시도한다(t *testing.T) {
	first := withWhiskyPages(singlePageItems("100000000001"),
		[]string{"100000000011", "100000000012", "100000000013"})
	second := withWhiskyPages(singlePageItems("200000000001"),
		[]string{"100000000011", "100000000012", "100000000014"})
	evidence := []TaskAttemptEvidence{
		attemptEvidence(1, first),
		attemptEvidence(2, second),
	}

	result := ReconcileTaskAttempt(evidence, 2, fixedTargets, 2)

	if result.Complete ||
		!strings.Contains(result.Reason, "RCNO_SET_CHANGED") ||
		!strings.Contains(result.Reason, "missing=1") ||
		!strings.Contains(result.Reason, "unexpected=1") {
		t.Fatalf("result = %+v", result)
	}
}

func TestReconcileTaskAttempt_세번째시도가직전유효시도와같으면완료한다(t *testing.T) {
	first := withWhiskyPages(singlePageItems("100000000001"),
		[]string{"100000000011", "100000000012", "100000000013"})
	second := withWhiskyPages(singlePageItems("200000000001"),
		[]string{"100000000011", "100000000012", "100000000014"})
	third := withWhiskyPages(singlePageItems("300000000001"),
		[]string{"100000000014", "100000000012", "100000000011"})
	evidence := []TaskAttemptEvidence{
		attemptEvidence(1, first),
		attemptEvidence(2, second),
		attemptEvidence(3, third),
	}

	result := ReconcileTaskAttempt(evidence, 3, fixedTargets, 2)

	if !result.Complete || result.Reason != "" {
		t.Fatalf("result = %+v", result)
	}
}

func TestReconcileTaskAttempt_빈결과4개품목_한번에완료한다(t *testing.T) {
	items := make([]AttemptItemEvidence, 0, len(fixedTargets))
	for _, target := range fixedTargets {
		items = append(items, AttemptItemEvidence{
			ItemName: target.Name,
			ItemCode: target.Code,
			Pages: []AttemptPageEvidence{{
				PageNo: 1,
				Status: "PARSED",
			}},
		})
	}

	result := ReconcileTaskAttempt(
		[]TaskAttemptEvidence{attemptEvidence(1, items)},
		1,
		fixedTargets,
		50,
	)

	if !result.Complete || result.Reason != "" {
		t.Fatalf("result = %+v", result)
	}
}

func attemptEvidence(attempt uint32, items []AttemptItemEvidence) TaskAttemptEvidence {
	return TaskAttemptEvidence{Attempt: attempt, Items: items}
}

func singlePageItems(prefix string) []AttemptItemEvidence {
	items := make([]AttemptItemEvidence, 0, len(fixedTargets))
	for index, target := range fixedTargets {
		rcno := prefix[:9] + string(rune('1'+index)) + "01"
		items = append(items, AttemptItemEvidence{
			ItemName: target.Name,
			ItemCode: target.Code,
			Pages: []AttemptPageEvidence{{
				PageNo:     1,
				Status:     "PARSED",
				Total:      1,
				ParsedRows: 1,
			}},
			RCNOs: []string{rcno},
		})
	}
	return items
}

func withWhiskyPages(
	items []AttemptItemEvidence,
	rcnos []string,
) []AttemptItemEvidence {
	result := append([]AttemptItemEvidence(nil), items...)
	result[0] = AttemptItemEvidence{
		ItemName: fixedTargets[0].Name,
		ItemCode: fixedTargets[0].Code,
		Pages: []AttemptPageEvidence{
			{PageNo: 1, Status: "PARSED", Total: 3, ParsedRows: 2},
			{PageNo: 2, Status: "PARSED", Total: 3, ParsedRows: 1},
		},
		RCNOs: append([]string(nil), rcnos...),
	}
	return result
}
