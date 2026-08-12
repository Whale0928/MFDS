package matching

import (
	"strconv"
	"testing"
	"time"
)

func TestSnapshotMatch_exactBilingualAndPunctuation(t *testing.T) {
	snapshot := mustSnapshot(t,
		[]AlcoholReference{{ID: 1, KorName: "글렌피딕 12년", EngName: "Glenfiddich 12", AgeYears: intPtr(12), RegionID: 7, DistilleryID: 23}},
		[]DistilleryReference{{ID: 23, KorName: "글렌피딕", EngName: "Glenfiddich"}},
		[]RegionReference{{ID: 7, KorName: "스코틀랜드", EngName: "Scotland"}},
	)

	result := snapshot.Match(Input{BaseNameKO: "글렌피딕-12년", SearchNameEN: "GLENFIDDICH, 12", AgeYears: intPtr(12)})
	if len(result.Alcohols) != 1 || result.Alcohols[0].ID != 1 {
		t.Fatalf("alcohol candidates = %#v", result.Alcohols)
	}
	if len(result.Distilleries) != 1 || result.Distilleries[0].ID != 23 {
		t.Fatalf("distillery candidates = %#v", result.Distilleries)
	}
	if len(result.Regions) != 1 || result.Regions[0].ID != 7 {
		t.Fatalf("region candidates = %#v", result.Regions)
	}
	if result.DistilleryDecision.SelectedID != 23 || result.RegionDecision.SelectedID != 7 {
		t.Fatalf("decisions = distillery:%+v region:%+v", result.DistilleryDecision, result.RegionDecision)
	}
}

func TestSnapshotMatch_fuzzy알코올후보를반환한다(t *testing.T) {
	snapshot := mustSnapshot(t,
		[]AlcoholReference{{ID: 1, EngName: "Laphroaig"}},
		nil,
		nil,
	)

	result := snapshot.Match(Input{BaseNameEN: "Laphroaigx"})
	if got := candidateIDs(result.Alcohols); !equalIDs(got, []int64{1}) {
		t.Fatalf("alcohols = %v, want [1]", got)
	}
	if result.Exact {
		t.Fatal("fuzzy match must not be exact")
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
	if len(result.Distilleries) != 0 {
		t.Fatalf("weak alcohol must not propagate distillery candidates: %#v", result.Distilleries)
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

func TestSnapshotMatch_exact증류소에서리전후보를건수순으로전파한다(t *testing.T) {
	snapshot := mustSnapshot(t,
		[]AlcoholReference{
			{ID: 1, EngName: "Laphroaig 15yo 200th Anniversary Limited Edition", DistilleryID: 36, RegionID: 15},
			{ID: 2, EngName: "Laphroaig Quarter Cask", DistilleryID: 36, RegionID: 15},
			{ID: 3, EngName: "Laphroaig Select", DistilleryID: 36, RegionID: 15},
			{ID: 4, EngName: "Samaroli Laphroaig", DistilleryID: 36, RegionID: 34},
		},
		[]DistilleryReference{{ID: 36, EngName: "Laphroaig"}},
		[]RegionReference{{ID: 19, EngName: "Scotland"}, {ID: 15, EngName: "Islay", ParentID: 19}, {ID: 34, EngName: "Italy"}},
	)

	result := snapshot.Match(Input{BaseNameEN: "Laphroaig"})
	if got := candidateIDs(result.Regions); !equalIDs(got, []int64{15, 34, 19}) {
		t.Fatalf("regions = %v, want [15 34 19]", got)
	}
	if result.Regions[0].Score != 3 || result.Regions[1].Score != 1 || result.Regions[2].Score != 0.5 {
		t.Fatalf("region scores = %#v", result.Regions)
	}
	if result.Regions[0].Evidence[0].Kind != "region_from_distillery_prior" {
		t.Fatalf("evidence = %#v", result.Regions[0].Evidence)
	}
}

func TestSnapshotMatch_기존리전후보가있으면증류소리전을전파하지않는다(t *testing.T) {
	snapshot := mustSnapshot(t,
		[]AlcoholReference{{ID: 1, EngName: "Unrelated Laphroaig Edition", DistilleryID: 36, RegionID: 15}},
		[]DistilleryReference{{ID: 36, EngName: "Laphroaig"}},
		[]RegionReference{{ID: 15, EngName: "Islay"}, {ID: 20, EngName: "Speyside"}},
	)

	result := snapshot.Match(Input{BaseNameEN: "Laphroaig Speyside Release"})
	if got := candidateIDs(result.Regions); !equalIDs(got, []int64{20}) {
		t.Fatalf("regions = %v, want [20]", got)
	}
	if result.Regions[0].Evidence[0].Kind != "region_name_contains" {
		t.Fatalf("evidence = %#v", result.Regions[0].Evidence)
	}
}

func TestSnapshotMatch_fuzzy증류소에서는리전을전파하지않는다(t *testing.T) {
	snapshot := mustSnapshot(t,
		[]AlcoholReference{{ID: 1, EngName: "Unrelated Product", DistilleryID: 36, RegionID: 15}},
		[]DistilleryReference{{ID: 36, EngName: "Laphroaig"}},
		[]RegionReference{{ID: 15, EngName: "Islay"}},
	)

	result := snapshot.Match(Input{BaseNameEN: "Laphroaix"})
	if len(result.Distilleries) != 1 || result.Distilleries[0].Evidence[0].Kind != "distillery_name_fuzzy" {
		t.Fatalf("distilleries = %#v", result.Distilleries)
	}
	if len(result.Regions) != 0 {
		t.Fatalf("fuzzy distillery propagated regions: %#v", result.Regions)
	}
}

func TestSnapshotMatch_삭제및고아alcohol은증류소리전집계에서제외한다(t *testing.T) {
	deletedAt := time.Unix(0, 0)
	snapshot := mustSnapshot(t,
		[]AlcoholReference{
			{ID: 1, EngName: "Known Product", DistilleryID: 36, RegionID: 15},
			{ID: 2, EngName: "Deleted Product", DistilleryID: 36, RegionID: 34, DeletedAt: &deletedAt},
			{ID: 3, EngName: "Orphan Product", DistilleryID: 36, RegionID: 999},
		},
		[]DistilleryReference{{ID: 36, EngName: "Laphroaig"}},
		[]RegionReference{{ID: 15, EngName: "Islay"}, {ID: 34, EngName: "Italy"}},
	)

	result := snapshot.Match(Input{BaseNameEN: "Laphroaig"})
	if got := candidateIDs(result.Regions); !equalIDs(got, []int64{15}) {
		t.Fatalf("regions = %v, want [15]", got)
	}
}

func TestSnapshotMatch_증류소리전점수는건수가많아도직접매칭보다낮다(t *testing.T) {
	alcohols := make([]AlcoholReference, 25)
	for index := range alcohols {
		alcohols[index] = AlcoholReference{ID: int64(index + 1), EngName: "Reference " + strconv.Itoa(index+1), DistilleryID: 36, RegionID: 15}
	}
	snapshot := mustSnapshot(t, alcohols, []DistilleryReference{{ID: 36, EngName: "Laphroaig"}}, []RegionReference{{ID: 15, EngName: "Islay"}})

	result := snapshot.Match(Input{BaseNameEN: "Laphroaig"})
	if len(result.Regions) != 1 || result.Regions[0].Score != weightDistilleryPriorMax || result.Regions[0].Score >= weightDirectRegion {
		t.Fatalf("regions = %#v", result.Regions)
	}
}

func TestSnapshotMatch_증류소리전건수동률은ID순으로정렬한다(t *testing.T) {
	snapshot := mustSnapshot(t,
		[]AlcoholReference{
			{ID: 1, EngName: "Reference One", DistilleryID: 36, RegionID: 20},
			{ID: 2, EngName: "Reference Two", DistilleryID: 36, RegionID: 10},
		},
		[]DistilleryReference{{ID: 36, EngName: "Laphroaig"}},
		[]RegionReference{{ID: 10, EngName: "Ten"}, {ID: 20, EngName: "Twenty"}},
	)

	result := snapshot.Match(Input{BaseNameEN: "Laphroaig"})
	if got := candidateIDs(result.Regions); !equalIDs(got, []int64{10, 20}) {
		t.Fatalf("regions = %v, want [10 20]", got)
	}
}

func TestSnapshotMatch_글렌피딕15년_알코올상위3개와공통참조를반환한다(t *testing.T) {
	snapshot := mustSnapshot(t,
		[]AlcoholReference{
			{ID: 245, EngName: "Glenfiddich 15y", ABVPercent: floatPtr(40), Age: "15", DistilleryID: 23, RegionID: 16},
			{ID: 5529, EngName: "Glenfiddich 15yo", ABVPercent: floatPtr(40), Age: "15", DistilleryID: 23, RegionID: 16},
			{ID: 5530, EngName: "Glenfiddich 15 year", ABVPercent: floatPtr(40), Age: "15", DistilleryID: 23, RegionID: 16},
		},
		[]DistilleryReference{{ID: 23, EngName: "Glenfiddich"}},
		[]RegionReference{{ID: 16, EngName: "Speyside"}},
	)

	result := snapshot.Match(Input{BaseNameEN: "GLENFIDDICH", AgeYears: intPtr(15), ABVPercent: floatPtr(40)})

	if got := candidateIDs(result.Alcohols); !equalIDs(got, []int64{245, 5529, 5530}) {
		t.Fatalf("alcohols = %v", got)
	}
	for _, candidate := range result.Alcohols {
		if candidate.Score != 13.5 {
			t.Fatalf("candidate %d score = %.2f", candidate.ID, candidate.Score)
		}
	}
	if result.AlcoholDecision.Status != DecisionAmbiguous || result.AlcoholDecision.SelectedID != 0 || result.AlcoholDecision.CompetitiveCount != 3 {
		t.Fatalf("alcohol decision = %+v", result.AlcoholDecision)
	}
	if result.AlcoholConsensus != (AlcoholConsensus{DistilleryID: 23, RegionID: 16}) {
		t.Fatalf("consensus = %+v", result.AlcoholConsensus)
	}
	if result.DistilleryDecision.SelectedID != 23 || result.RegionDecision.SelectedID != 16 {
		t.Fatalf("fallback decisions = distillery:%+v region:%+v", result.DistilleryDecision, result.RegionDecision)
	}
}

func TestSnapshotMatch_동률후보가4개면3개만표시하고전체후보로합의를판정한다(t *testing.T) {
	alcohols := []AlcoholReference{
		{ID: 1, EngName: "Example 10yo", ABVPercent: floatPtr(40), Age: "10", DistilleryID: 10, RegionID: 20},
		{ID: 2, EngName: "Example 10y", ABVPercent: floatPtr(40), Age: "10", DistilleryID: 10, RegionID: 20},
		{ID: 3, EngName: "Example 10 year", ABVPercent: floatPtr(40), Age: "10", DistilleryID: 10, RegionID: 20},
		{ID: 4, EngName: "Example 10 years old", ABVPercent: floatPtr(40), Age: "10", DistilleryID: 99, RegionID: 98},
	}
	snapshot := mustSnapshot(t, alcohols,
		[]DistilleryReference{{ID: 10, EngName: "Example"}, {ID: 99, EngName: "Other"}},
		[]RegionReference{{ID: 20, EngName: "One"}, {ID: 98, EngName: "Other"}},
	)

	result := snapshot.Match(Input{BaseNameEN: "Example", AgeYears: intPtr(10), ABVPercent: floatPtr(40)})

	if len(result.Alcohols) != 3 || result.AlcoholDecision.CompetitiveCount != 4 {
		t.Fatalf("visible=%d decision=%+v", len(result.Alcohols), result.AlcoholDecision)
	}
	if result.AlcoholConsensus != (AlcoholConsensus{}) {
		t.Fatalf("top three only produced unsafe consensus: %+v", result.AlcoholConsensus)
	}
}

func TestDecideAlcohol_명시적도수충돌_자동선택하지않는다(t *testing.T) {
	decision := decideAlcohol([]Candidate{{
		ID:    1,
		Score: 20,
		Evidence: []Evidence{
			{Kind: "alcohol_name_exact", Weight: weightNameExact},
			{Kind: "abv_conflict", Weight: weightABVConflict},
		},
	}})

	if decision.Status != DecisionConflictReview || decision.SelectedID != 0 || decision.StopReason != "A_ATTRIBUTE_CONFLICT" {
		t.Fatalf("decision = %+v", decision)
	}
}

func TestAddCandidateEvidence_동일한근거_한번만점수에반영한다(t *testing.T) {
	candidate := Candidate{ID: 1}
	evidence := Evidence{Kind: "age_exact", Weight: weightAgeExact}

	addCandidateEvidence(&candidate, evidence)
	addCandidateEvidence(&candidate, evidence)

	if candidate.Score != weightAgeExact || len(candidate.Evidence) != 1 {
		t.Fatalf("candidate = %+v", candidate)
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
