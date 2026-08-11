package matching

import (
	"testing"
	"time"
)

func TestSnapshotMatch_exactBilingualAndPunctuation(t *testing.T) {
	snapshot := mustSnapshot(t,
		[]AlcoholReference{{ID: 1, KorName: "글렌피딕 12년", EngName: "Glenfiddich 12", RegionID: 7, DistilleryID: 23}},
		[]DistilleryReference{{ID: 23, KorName: "글렌피딕", EngName: "Glenfiddich"}},
		[]RegionReference{{ID: 7, KorName: "스코틀랜드", EngName: "Scotland"}},
	)

	result := snapshot.Match(Input{BaseNameKO: "글렌피딕-12년", SearchNameEN: "GLENFIDDICH, 12"})
	if len(result.Distilleries) != 1 || result.Distilleries[0].ID != 23 {
		t.Fatalf("distillery candidates = %#v", result.Distilleries)
	}
	if len(result.Regions) != 1 || result.Regions[0].ID != 7 {
		t.Fatalf("region candidates = %#v", result.Regions)
	}
	if got := result.Distilleries[0].Evidence[0].Kind; got != "alcohol_exact" {
		t.Fatalf("evidence = %q", got)
	}
}

func TestSnapshotMatch_excludesDeletedAndPlaceholderReferences(t *testing.T) {
	deletedAt := time.Unix(0, 0)
	snapshot := mustSnapshot(t,
		[]AlcoholReference{{ID: 1, EngName: "Deleted Malt", DeletedAt: &deletedAt, RegionID: 1, DistilleryID: 10}},
		[]DistilleryReference{{ID: 0, EngName: "Placeholder"}}, nil,
	)
	result := snapshot.Match(Input{BaseNameEN: "Deleted Malt Placeholder"})
	if len(result.Distilleries) != 0 || len(result.Regions) != 0 {
		t.Fatalf("deleted or placeholder references leaked: %#v", result)
	}
}

func TestSnapshotMatch_excludesOrphanReferenceIDs(t *testing.T) {
	snapshot := mustSnapshot(t,
		[]AlcoholReference{{ID: 1, EngName: "Orphan Malt", DistilleryID: 999, RegionID: 998}},
		[]DistilleryReference{{ID: 10, EngName: "Known"}},
		[]RegionReference{{ID: 20, EngName: "Known Region"}},
	)

	result := snapshot.Match(Input{BaseNameEN: "Orphan Malt"})
	if len(result.Distilleries) != 0 || len(result.Regions) != 0 {
		t.Fatalf("orphan reference IDs leaked: %#v", result)
	}
}

func TestSnapshotMatch_calculatesDistilleryAndRegionIndependently(t *testing.T) {
	snapshot := mustSnapshot(t,
		[]AlcoholReference{{ID: 1, EngName: "Orphan Malt", DistilleryID: 10, RegionID: 998}},
		[]DistilleryReference{{ID: 10, EngName: "Known Distillery"}},
		[]RegionReference{{ID: 20, EngName: "Islay"}},
	)

	result := snapshot.Match(Input{BaseNameEN: "Orphan Malt Islay"})
	if len(result.Distilleries) != 1 || result.Distilleries[0].ID != 10 {
		t.Fatalf("distillery candidates = %#v", result.Distilleries)
	}
	if len(result.Regions) != 1 || result.Regions[0].ID != 20 {
		t.Fatalf("region candidates = %#v", result.Regions)
	}
}

func TestSnapshotMatch_keepsAliasCollisionAndRejectsSubstringFalsePositives(t *testing.T) {
	snapshot := mustSnapshot(t, nil, []DistilleryReference{
		{ID: 164, EngName: "Ryuka"}, {ID: 227, EngName: "Ryuka"},
		{ID: 14, EngName: "Glenlossie"}, {ID: 15, EngName: "Glen Rothes"},
		{ID: 16, EngName: "Tormore"},
	}, nil)

	result := snapshot.Match(Input{BaseNameEN: "Ryuka Glen Rothes"})
	if len(result.Distilleries) != 3 || result.Distilleries[0].ID != 15 || result.Distilleries[1].ID != 164 || result.Distilleries[2].ID != 227 {
		t.Fatalf("collision ordering = %#v", result.Distilleries)
	}
	for _, candidate := range result.Distilleries {
		if candidate.ID == 14 || candidate.ID == 16 {
			t.Fatalf("false positive candidate = %#v", candidate)
		}
	}
	if got := snapshot.Match(Input{BaseNameEN: "Octomore"}); len(got.Distilleries) != 0 {
		t.Fatalf("octomore matched tormore: %#v", got.Distilleries)
	}
}

func TestSnapshotMatch_returnsNoMatchAndDeterministicTopThree(t *testing.T) {
	snapshot := mustSnapshot(t, nil, []DistilleryReference{
		{ID: 9, EngName: "Alpha"}, {ID: 3, EngName: "Alpha"}, {ID: 5, EngName: "Alpha"}, {ID: 1, EngName: "Alpha"},
	}, nil)
	result := snapshot.Match(Input{BaseNameEN: "Alpha"})
	if len(result.Distilleries) != 3 {
		t.Fatalf("top three count = %d", len(result.Distilleries))
	}
	for index, want := range []int64{1, 3, 5} {
		if result.Distilleries[index].ID != want {
			t.Fatalf("candidate %d = %d, want %d", index, result.Distilleries[index].ID, want)
		}
	}
	if got := snapshot.Match(Input{BaseNameEN: "Unrelated Product"}); len(got.Distilleries) != 0 || len(got.Regions) != 0 {
		t.Fatalf("unexpected no-match result: %#v", got)
	}
}

func TestSnapshotMatch_doesNotUseGBForChildRegion(t *testing.T) {
	snapshot := mustSnapshot(t, nil, nil, []RegionReference{
		{ID: 1, EngName: "Scotland"},
		{ID: 2, EngName: "Islay", ParentID: 1},
	})
	result := snapshot.Match(Input{ManufactureCountry: "GB"})
	if len(result.Regions) != 0 {
		t.Fatalf("GB produced a child-region candidate: %#v", result.Regions)
	}
}

func TestSnapshot_versionIsStableAcrossReferenceOrder(t *testing.T) {
	first := mustSnapshot(t, nil, []DistilleryReference{{ID: 2, EngName: "Two"}, {ID: 1, EngName: "One"}}, nil)
	second := mustSnapshot(t, nil, []DistilleryReference{{ID: 1, EngName: "One"}, {ID: 2, EngName: "Two"}}, nil)
	if first.Version() != second.Version() {
		t.Fatalf("versions differ: %+v != %+v", first.Version(), second.Version())
	}
}

func mustSnapshot(t *testing.T, alcohols []AlcoholReference, distilleries []DistilleryReference, regions []RegionReference) *ReferenceSnapshot {
	t.Helper()
	snapshot, err := NewReferenceSnapshot(alcohols, distilleries, regions, DefaultMatchingVersion("test-reference-hash"))
	if err != nil {
		t.Fatal(err)
	}
	return snapshot
}
