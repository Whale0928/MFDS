package mysql

import (
	"testing"

	"github.com/bottle-note/mfds-crawler/internal/usecase/weblist"
)

func TestTaskAttemptEvidenceBuilderEnsureItem_같은시도와품목_기존증거를반환한다(t *testing.T) {
	builder := newTaskAttemptEvidenceBuilder()
	first := builder.ensureItem(1, "ITEM-B", "브랜디")
	first.Pages = append(first.Pages, weblist.AttemptPageEvidence{PageNo: 1})

	second := builder.ensureItem(1, "ITEM-B", "브랜디")
	second.RCNOs = append(second.RCNOs, "202600000001")

	result := builder.result()
	if len(result) != 1 || len(result[0].Items) != 1 {
		t.Fatalf("evidence count = %d/%d, want 1/1", len(result), len(result[0].Items))
	}
	item := result[0].Items[0]
	if len(item.Pages) != 1 || len(item.RCNOs) != 1 {
		t.Fatalf("item evidence = %+v", item)
	}
}

func TestTaskAttemptEvidenceBuilderResult_순서가섞인증거_시도와품목코드순으로정렬한다(t *testing.T) {
	builder := newTaskAttemptEvidenceBuilder()
	builder.ensureItem(2, "ITEM-B", "브랜디")
	builder.ensureItem(2, "ITEM-A", "위스키")
	builder.ensureItem(1, "ITEM-C", "리큐르")

	result := builder.result()
	if len(result) != 2 || result[0].Attempt != 1 || result[1].Attempt != 2 {
		t.Fatalf("attempts = %+v", result)
	}
	items := result[1].Items
	if len(items) != 2 || items[0].ItemCode != "ITEM-A" || items[1].ItemCode != "ITEM-B" {
		t.Fatalf("items = %+v", items)
	}
}
