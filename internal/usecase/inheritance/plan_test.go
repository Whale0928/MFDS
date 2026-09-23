package inheritance

import (
	"testing"
)

const testKey = "k1"

func seedRow(id, alcoholID int64) Row {
	return Row{
		DeclarationID: id, RCNO: "SEED", IdentityKey: testKey, NormalizationStatus: "NORMALIZED",
		AlcoholCategoryEN: "Whisky", ManufactureCountryAlpha2: "GB", Decision: "CANDIDATE", SelectedAlcoholID: alcoholID,
	}
}

func targetRow(id int64) Row {
	return Row{
		DeclarationID: id, RCNO: "TARGET", IdentityKey: testKey, NormalizationStatus: "NORMALIZED",
		AlcoholCategoryEN: "Whisky", ManufactureCountryAlpha2: "GB", Decision: "REVIEW",
	}
}

func testAlcohols() map[int64]Alcohol {
	return map[int64]Alcohol{
		140:  {ID: 140, DistilleryID: 150, RegionID: 19, EngName: "Johnnie Walker Black Label 12yo", ABV: "40", Age: "12"},
		141:  {ID: 141, DistilleryID: 0, RegionID: 0, EngName: "Other"},
		7082: {ID: 7082, DistilleryID: 1, RegionID: 2, EngName: "Ballantine's 17", ABV: "40", Age: "17", Volume: "700"},
		7083: {ID: 7083, DistilleryID: 1, RegionID: 2, EngName: "Ballantine's 17", ABV: "40", Age: "17", Volume: "700"},
		9000: {ID: 9000, Deleted: true, EngName: "Deleted"},
	}
}

func TestBuildPlan_시드알코올을채우고증류소리전은알코올값을전파한다(t *testing.T) {
	// Given
	rows := []Row{seedRow(1, 140), targetRow(2)}

	// When
	plan := BuildPlan(rows, testAlcohols())

	// Then
	if plan.SeedGroups != 1 || len(plan.Groups) != 1 || plan.Fills() != 1 {
		t.Fatalf("plan = %+v", plan)
	}
	want := Selection{SeedDeclarationID: 1, AlcoholID: 140, DistilleryID: 150, DistillerySource: SourceAlcoholPropagated, RegionID: 19, RegionSource: SourceAlcoholPropagated}
	if got := plan.Groups[0].Fills[0].Desired; got != want {
		t.Fatalf("selection = %+v, want %+v", got, want)
	}
}

func TestBuildPlan_시드의증류소리전값이있으면INHERITED로복사하고0은채우지않는다(t *testing.T) {
	// Given
	seed := seedRow(1, 141)
	seed.SelectedDistilleryID = 77
	rows := []Row{seed, targetRow(2)}

	// When
	plan := BuildPlan(rows, testAlcohols())

	// Then
	want := Selection{SeedDeclarationID: 1, AlcoholID: 141, DistilleryID: 77, DistillerySource: SourceInherited}
	if got := plan.Groups[0].Fills[0].Desired; got != want {
		t.Fatalf("selection = %+v, want %+v", got, want)
	}
}

func TestBuildPlan_대상제외사유를집계한다(t *testing.T) {
	// Given
	review := targetRow(10)
	review.NormalizationStatus = "REVIEW_REQUIRED"
	generic := targetRow(11)
	generic.NormalizationStatus = "REVIEW_REQUIRED"
	generic.Reasons = []string{"GENERIC_PRODUCT_NAME_REVIEW_REQUIRED"}
	liqueur := targetRow(12)
	liqueur.AlcoholCategoryEN = "Liqueur"
	country := targetRow(13)
	country.ManufactureCountryAlpha2 = "US"
	stale := targetRow(14)
	stale.NormalizationStatus = "STALE"
	released := targetRow(15)
	released.AdminReleased = true
	rows := []Row{seedRow(1, 140), review, generic, liqueur, country, stale, released}

	// When
	plan := BuildPlan(rows, testAlcohols())

	// Then
	want := map[string]int{
		ExcludeReviewRequired: 1, ExcludeGenericProductName: 1, ExcludeNotWhisky: 1, ExcludeCountryMismatch: 1,
		ExcludeNotNormalized: 1, ExcludeAdminReleased: 1,
	}
	if plan.Fills() != 0 || len(plan.Exclusions) != len(want) {
		t.Fatalf("fills = %d exclusions = %v", plan.Fills(), plan.Exclusions)
	}
	for reason, count := range want {
		if plan.Exclusions[reason] != count {
			t.Fatalf("exclusions = %v, want %v", plan.Exclusions, want)
		}
	}
}

func TestBuildPlan_서로다른알코올시드는충돌로보고하고채우지않는다(t *testing.T) {
	// Given
	rows := []Row{seedRow(1, 140), seedRow(2, 141), targetRow(3)}

	// When
	plan := BuildPlan(rows, testAlcohols())

	// Then
	if plan.Fills() != 0 || len(plan.Conflicts) != 1 || plan.Conflicts[0].Reason != "ALCOHOL_MISMATCH" || plan.Exclusions[ExcludeSeedConflict] != 1 {
		t.Fatalf("plan = %+v", plan)
	}
	if len(plan.Conflicts[0].Seeds) != 2 {
		t.Fatalf("conflict seeds = %+v", plan.Conflicts[0].Seeds)
	}
}

func TestBuildPlan_같은알코올시드가여럿이면가장작은ID를기록한다(t *testing.T) {
	// Given
	rows := []Row{seedRow(9, 140), seedRow(4, 140), targetRow(20)}

	// When
	plan := BuildPlan(rows, testAlcohols())

	// Then
	if plan.Fills() != 1 || plan.Groups[0].Fills[0].Desired.SeedDeclarationID != 4 || len(plan.Groups[0].Seeds) != 2 {
		t.Fatalf("plan = %+v", plan)
	}
}

func TestBuildPlan_무효시드는사용하지않는다(t *testing.T) {
	// Given
	deleted := seedRow(1, 9000)
	review := seedRow(2, 140)
	review.IdentityKey = "k2"
	review.NormalizationStatus = "REVIEW_REQUIRED"
	stale := seedRow(3, 140)
	stale.IdentityKey = "k3"
	stale.NormalizationStatus = "STALE"
	target := targetRow(4)
	rows := []Row{deleted, review, stale, target}

	// When
	plan := BuildPlan(rows, testAlcohols())

	// Then
	if plan.Fills() != 0 || plan.InvalidSeeds["ALCOHOL_DELETED"] != 1 || plan.InvalidSeeds[ExcludeReviewRequired] != 1 || plan.InvalidSeeds[ExcludeNotNormalized] != 1 {
		t.Fatalf("plan = %+v", plan)
	}
}

func TestBuildPlan_참조중복묶음은대상1순위후보가같은묶음일때만채운다(t *testing.T) {
	// Given
	inBundle := targetRow(2)
	inBundle.TopAlcoholCandidateID = 7083
	outside := targetRow(3)
	outside.TopAlcoholCandidateID = 140
	rows := []Row{seedRow(1, 7082), inBundle, outside}

	// When
	plan := BuildPlan(rows, testAlcohols())

	// Then
	if plan.Fills() != 1 || plan.Groups[0].Fills[0].Current.DeclarationID != 2 || plan.Exclusions[ExcludeReferenceDuplicate] != 1 {
		t.Fatalf("plan = %+v", plan)
	}
}

func TestBuildPlan_기존상속행은같으면유지하고시드가바뀌면갱신하며시드가없으면해제한다(t *testing.T) {
	// Given
	current := targetRow(2)
	current.Decision, current.SelectedAlcoholID, current.InheritedFromDeclarationID = "INHERITED", 140, 1
	current.SelectedDistilleryID, current.DistillerySource = 150, SourceAlcoholPropagated
	current.SelectedRegionID, current.RegionSource = 19, SourceAlcoholPropagated
	moved := current
	moved.DeclarationID = 3
	orphan := current
	orphan.DeclarationID, orphan.IdentityKey = 4, "k-orphan"
	keyless := current
	keyless.DeclarationID, keyless.IdentityKey = 5, ""

	// When
	unchanged := BuildPlan([]Row{seedRow(1, 140), current}, testAlcohols())
	reassigned := BuildPlan([]Row{seedRow(1, 141), moved}, testAlcohols())
	released := BuildPlan([]Row{orphan, keyless}, testAlcohols())

	// Then
	if unchanged.Unchanged != 1 || unchanged.Updates() != 0 || len(unchanged.Releases) != 0 {
		t.Fatalf("unchanged plan = %+v", unchanged)
	}
	if reassigned.Updates() != 1 || reassigned.Groups[0].Updates[0].Desired.AlcoholID != 141 {
		t.Fatalf("reassigned plan = %+v", reassigned)
	}
	if len(released.Releases) != 2 || released.Releases[0].Reason != ReleaseSeedInvalid || released.Releases[1].Reason != ReleaseKeyMissing {
		t.Fatalf("released plan = %+v", released.Releases)
	}
}

func TestBuildPlan_제외조건이생긴상속행은해제한다(t *testing.T) {
	// Given
	current := targetRow(2)
	current.Decision, current.SelectedAlcoholID, current.InheritedFromDeclarationID = "INHERITED", 140, 1
	current.NormalizationStatus = "REVIEW_REQUIRED"

	// When
	plan := BuildPlan([]Row{seedRow(1, 140), current}, testAlcohols())

	// Then
	if len(plan.Releases) != 1 || plan.Releases[0].Reason != ReleaseTargetExcludedFront+ExcludeReviewRequired {
		t.Fatalf("releases = %+v", plan.Releases)
	}
}

func TestBuildPlan_관리자확정이자동확정보다우선한다(t *testing.T) {
	// Given: 같은 키에서 다른 알코올로 자동 확정된 행
	auto := targetRow(2)
	auto.Decision, auto.SelectedAlcoholID = "AUTO_SELECTED", 141

	// When
	plan := BuildPlan([]Row{seedRow(1, 140), auto}, testAlcohols())

	// Then
	if plan.Fills() != 1 || plan.Groups[0].Fills[0].Desired.AlcoholID != 140 {
		t.Fatalf("plan = %+v", plan)
	}
}
