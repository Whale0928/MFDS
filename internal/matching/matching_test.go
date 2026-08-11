package matching

import (
	"strconv"
	"testing"
	"time"
)

func TestReferenceSnapshotMatchTableDriven(t *testing.T) {
	deletedAt := time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC)
	abv40 := 40.0
	cases := []struct {
		name             string
		input            Input
		wantDistilleries []int64
		wantRegions      []int64
		wantExact        bool
	}{
		{
			name:             "exact bilingual names aggregate same IDs",
			input:            Input{BaseNameKO: "글렌피딕 12", SearchNameEN: "Glenfiddich 12", ABVPercent: &abv40, AgeYears: intPtr(12), Cask: "Sherry Cask"},
			wantDistilleries: []int64{10},
			wantRegions:      []int64{100},
			wantExact:        true,
		},
		{
			name:             "punctuation is a token boundary",
			input:            Input{BaseNameEN: "Glenfiddich, 12 Years"},
			wantDistilleries: []int64{10},
			wantRegions:      []int64{100},
			wantExact:        true,
		},
		{
			name:             "deleted alcohol is excluded",
			input:            Input{BaseNameEN: "Deleted Spirit"},
			wantDistilleries: nil,
			wantRegions:      nil,
		},
		{
			name:             "zero alcohol ID placeholder is excluded",
			input:            Input{BaseNameEN: "Placeholder Spirit"},
			wantDistilleries: nil,
			wantRegions:      nil,
		},
		{
			name:             "same alias keeps Ryuka collision ambiguous",
			input:            Input{BaseNameEN: "Ryuka"},
			wantDistilleries: []int64{20, 21},
			wantRegions:      []int64{200, 201},
			wantExact:        true,
		},
		{
			name:             "Octomore does not match Tormore",
			input:            Input{BaseNameEN: "Octomore"},
			wantDistilleries: []int64{30},
			wantRegions:      []int64{300},
			wantExact:        true,
		},
		{
			name:             "Glen Rothes does not match Glenlossie",
			input:            Input{BaseNameEN: "Glen Rothes"},
			wantDistilleries: []int64{40},
			wantRegions:      []int64{400},
			wantExact:        true,
		},
		{
			name:             "no match returns no candidates",
			input:            Input{BaseNameEN: "Completely Unknown Spirit", ManufactureCountry: "GB"},
			wantDistilleries: nil,
			wantRegions:      nil,
		},
	}

	snapshot := testSnapshot(t, deletedAt)
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			result := snapshot.Match(test.input)
			if got := candidateIDs(result.Distilleries); !equalIDs(got, test.wantDistilleries) {
				t.Fatalf("distilleries = %v, want %v", got, test.wantDistilleries)
			}
			if got := candidateIDs(result.Regions); !equalIDs(got, test.wantRegions) {
				t.Fatalf("regions = %v, want %v", got, test.wantRegions)
			}
			if result.Exact != test.wantExact {
				t.Fatalf("exact = %v, want %v", result.Exact, test.wantExact)
			}
		})
	}
}

func TestReferenceSnapshotMatchDeterministicTopThree(t *testing.T) {
	var alcohols []AlcoholReference
	for id := int64(1); id <= 5; id++ {
		alcohols = append(alcohols, AlcoholReference{ID: id, EngName: "Spirit " + strconv.FormatInt(id, 10), DistilleryID: id, RegionID: id})
	}
	snapshot, err := NewReferenceSnapshot(alcohols, []DistilleryReference{
		{ID: 1, EngName: "Spirit 1"}, {ID: 2, EngName: "Spirit 2"}, {ID: 3, EngName: "Spirit 3"}, {ID: 4, EngName: "Spirit 4"}, {ID: 5, EngName: "Spirit 5"},
	}, []RegionReference{{ID: 1}, {ID: 2}, {ID: 3}, {ID: 4}, {ID: 5}}, DefaultMatchingVersion("hash"))
	if err != nil {
		t.Fatal(err)
	}
	result := snapshot.Match(Input{BaseNameEN: "Spirit 1 Spirit 2 Spirit 3 Spirit 4 Spirit 5"})
	if got := candidateIDs(result.Distilleries); !equalIDs(got, []int64{1, 2, 3}) {
		t.Fatalf("distilleries = %v, want [1 2 3]", got)
	}
}

func TestReferenceSnapshotUsesParentAsWeakRegionEvidence(t *testing.T) {
	snapshot, err := NewReferenceSnapshot(
		[]AlcoholReference{{ID: 1, EngName: "Islay Spirit", RegionID: 11, DistilleryID: 10}},
		[]DistilleryReference{{ID: 10, EngName: "Islay Spirit"}},
		[]RegionReference{{ID: 1, EngName: "Scotland"}, {ID: 11, EngName: "Islay", ParentID: 1}},
		DefaultMatchingVersion("hash"),
	)
	if err != nil {
		t.Fatal(err)
	}
	result := snapshot.Match(Input{BaseNameEN: "Islay Spirit"})
	if got := candidateIDs(result.Regions); !equalIDs(got, []int64{11, 1}) {
		t.Fatalf("regions = %v, want [11 1]", got)
	}
	if result.Regions[1].Score >= result.Regions[0].Score {
		t.Fatalf("parent region score = %d, child score = %d", result.Regions[1].Score, result.Regions[0].Score)
	}
}

func TestReferenceSnapshotCopiesInputs(t *testing.T) {
	alcohols := []AlcoholReference{{ID: 1, EngName: "Original", DistilleryID: 10}}
	distilleries := []DistilleryReference{{ID: 10, EngName: "Original"}}
	regions := []RegionReference{{ID: 20, EngName: "Region"}}
	snapshot, err := NewReferenceSnapshot(alcohols, distilleries, regions, DefaultMatchingVersion("hash"))
	if err != nil {
		t.Fatal(err)
	}
	alcohols[0].EngName = "Changed"
	distilleries[0].EngName = "Changed"
	regions[0].EngName = "Changed"
	if got := candidateIDs(snapshot.Match(Input{BaseNameEN: "Original"}).Distilleries); !equalIDs(got, []int64{10}) {
		t.Fatalf("snapshot changed after source mutation: %v", got)
	}
}

func testSnapshot(t *testing.T, deletedAt time.Time) *ReferenceSnapshot {
	t.Helper()
	snapshot, err := NewReferenceSnapshot(
		[]AlcoholReference{
			{ID: 1, KorName: "글렌피딕 12", EngName: "Glenfiddich 12", ABVPercent: floatPtr(40), AgeYears: intPtr(12), Cask: "Sherry Cask", DistilleryID: 10, RegionID: 100},
			{ID: 2, EngName: "Deleted Spirit", DistilleryID: 99, RegionID: 999, DeletedAt: &deletedAt},
			{ID: 0, EngName: "Placeholder Spirit", DistilleryID: 98, RegionID: 998},
			{ID: 3, EngName: "Ryuka", DistilleryID: 20, RegionID: 200},
			{ID: 4, EngName: "Ryuka", DistilleryID: 21, RegionID: 201},
			{ID: 5, EngName: "Octomore", DistilleryID: 30, RegionID: 300},
			{ID: 6, EngName: "Tormore", DistilleryID: 31, RegionID: 301},
			{ID: 7, EngName: "Glen Rothes", DistilleryID: 40, RegionID: 400},
			{ID: 8, EngName: "Glenlossie", DistilleryID: 41, RegionID: 401},
		},
		[]DistilleryReference{
			{ID: 10, EngName: "Glenfiddich"}, {ID: 20, EngName: "Ryuka"}, {ID: 21, EngName: "Ryuka"},
			{ID: 30, EngName: "Octomore"}, {ID: 31, EngName: "Tormore"}, {ID: 40, EngName: "Glen Rothes"}, {ID: 41, EngName: "Glenlossie"},
		},
		[]RegionReference{
			{ID: 100, EngName: "Speyside"}, {ID: 200, EngName: "Okinawa"}, {ID: 201, EngName: "Okinawa"}, {ID: 300, EngName: "Islay"}, {ID: 301, EngName: "Speyside"}, {ID: 400, EngName: "Speyside"}, {ID: 401, EngName: "Speyside"},
		},
		DefaultMatchingVersion("hash"),
	)
	if err != nil {
		t.Fatal(err)
	}
	return snapshot
}

func candidateIDs(candidates []Candidate) []int64 {
	ids := make([]int64, 0, len(candidates))
	for _, candidate := range candidates {
		ids = append(ids, candidate.ID)
	}
	return ids
}

func equalIDs(left, right []int64) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}

func floatPtr(value float64) *float64 { return &value }

func intPtr(value int) *int { return &value }
