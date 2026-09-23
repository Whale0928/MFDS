// Package inheritance spreads administrator alcohol selections to declarations of the same product.
package inheritance

import (
	"sort"
	"strings"

	domain "github.com/bottle-note/mfds-crawler/internal/matching"
)

// 판정·출처 문자열은 api-server 확정 로직과 공유하는 계약이다.
const (
	SourceInherited         = "INHERITED"
	SourceAlcoholPropagated = "ALCOHOL_PROPAGATED"

	whiskyCategory = "Whisky"
	genericReason  = "GENERIC_PRODUCT_NAME_REVIEW_REQUIRED"
)

// 대상 제외와 상속 해제 사유 코드. 해제 사유는 mfds_matching_selections.reason_code에도 기록한다.
const (
	ExcludeAdminReleased       = "ADMIN_RELEASED"
	ExcludeGenericProductName  = "GENERIC_PRODUCT_NAME"
	ExcludeReviewRequired      = "REVIEW_REQUIRED"
	ExcludeNotNormalized       = "NOT_NORMALIZED"
	ExcludeNotWhisky           = "NOT_WHISKY"
	ExcludeCountryMismatch     = "COUNTRY_MISMATCH"
	ExcludeReferenceDuplicate  = "REFERENCE_DUPLICATE_UNCONFIRMED"
	ExcludeSeedConflict        = "SEED_CONFLICT"
	ReleaseKeyMissing          = "INHERITANCE_KEY_MISSING"
	ReleaseSeedInvalid         = "INHERITANCE_SEED_INVALID"
	ReleaseSeedConflict        = "INHERITANCE_SEED_CONFLICT"
	ReleaseTargetExcludedFront = "INHERITANCE_TARGET_"
)

// Row is the current matching state of one declaration.
type Row struct {
	DeclarationID              int64
	RCNO                       string
	IdentityKey                string
	NormalizationStatus        string
	Reasons                    []string
	AlcoholCategoryEN          string
	ManufactureCountryAlpha2   string
	Decision                   string
	SelectedAlcoholID          int64
	SelectedDistilleryID       int64
	SelectedRegionID           int64
	DistillerySource           string
	RegionSource               string
	InheritedFromDeclarationID int64
	TopAlcoholCandidateID      int64
	// AdminReleased is true when the latest ADMIN ALCOHOL history row is a REVOKE: an administrator cleared it on purpose.
	AdminReleased bool
}

// Alcohol is the reference data an inheritance decision needs.
type Alcohol struct {
	ID           int64
	Deleted      bool
	DistilleryID int64
	RegionID     int64
	EngName      string
	ABV          string
	Age          string
	Volume       string
}

// Selection is the full alcohol, distillery and region choice written to one declaration.
type Selection struct {
	SeedDeclarationID int64
	AlcoholID         int64
	DistilleryID      int64
	DistillerySource  string
	RegionID          int64
	RegionSource      string
}

// Action changes one declaration. Current holds the state the store must still find before writing.
type Action struct {
	Current Row
	Desired Selection
	Reason  string
}

// SeedRef identifies one seed in a conflict report.
type SeedRef struct {
	DeclarationID int64
	RCNO          string
	AlcoholID     int64
}

// Conflict is a key group whose seeds disagree, so nothing in it is inherited.
type Conflict struct {
	IdentityKey string
	Reason      string
	Seeds       []SeedRef
}

// Group is one key whose seeds agree. Seeds lists every seed row so the store can confirm they are still unchanged.
type Group struct {
	IdentityKey string
	Selection   Selection
	Seeds       []SeedRef
	Fills       []Action
	Updates     []Action
}

// Plan is the complete, deterministic result of one inheritance pass.
type Plan struct {
	// SeedGroups counts keys whose valid seeds agree, whether or not anything is left to write.
	SeedGroups   int
	Groups       []Group
	Releases     []Action
	Conflicts    []Conflict
	Unchanged    int
	InvalidSeeds map[string]int
	Exclusions   map[string]int
}

// Fills, Updates count planned writes across groups.
func (p Plan) Fills() int {
	total := 0
	for _, group := range p.Groups {
		total += len(group.Fills)
	}
	return total
}

func (p Plan) Updates() int {
	total := 0
	for _, group := range p.Groups {
		total += len(group.Updates)
	}
	return total
}

type seedGroup struct {
	seeds     []Row
	selection Selection
	country   string
	conflict  string
}

// BuildPlan decides every fill, update and release from one consistent read. It never writes.
func BuildPlan(rows []Row, alcohols map[int64]Alcohol) Plan {
	plan := Plan{InvalidSeeds: map[string]int{}, Exclusions: map[string]int{}}
	bundles := duplicateBundles(alcohols)
	groups := collectSeedGroups(rows, alcohols, &plan)

	planned := map[string]*Group{}
	for _, row := range rows {
		if isSeedDecision(row.Decision) {
			continue
		}
		// 관리자 값이 자동 매칭보다 우선하므로 자동 확정된 행도 채움 대상이다.
		inherited := row.Decision == string(domain.DecisionInherited)
		group := groups[row.IdentityKey]
		if row.IdentityKey == "" || group == nil {
			if inherited {
				plan.Releases = append(plan.Releases, Action{Current: row, Reason: releaseReasonWithoutGroup(row)})
			}
			continue
		}
		if group.conflict != "" {
			if inherited {
				plan.Releases = append(plan.Releases, Action{Current: row, Reason: ReleaseSeedConflict})
			} else {
				plan.Exclusions[ExcludeSeedConflict]++
			}
			continue
		}
		if reason := targetExclusion(row, group, bundles); reason != "" {
			if inherited {
				plan.Releases = append(plan.Releases, Action{Current: row, Reason: ReleaseTargetExcludedFront + reason})
			} else {
				plan.Exclusions[reason]++
			}
			continue
		}
		entry := planned[row.IdentityKey]
		if entry == nil {
			entry = &Group{IdentityKey: row.IdentityKey, Selection: group.selection, Seeds: seedRefs(group.seeds)}
			planned[row.IdentityKey] = entry
		}
		action := Action{Current: row, Desired: group.selection}
		switch {
		case !inherited:
			entry.Fills = append(entry.Fills, action)
		case currentSelection(row) == group.selection:
			plan.Unchanged++
		default:
			entry.Updates = append(entry.Updates, action)
		}
	}
	for _, group := range planned {
		if len(group.Fills) > 0 || len(group.Updates) > 0 {
			plan.Groups = append(plan.Groups, *group)
		}
	}
	sort.Slice(plan.Groups, func(i, j int) bool { return plan.Groups[i].IdentityKey < plan.Groups[j].IdentityKey })
	sort.Slice(plan.Conflicts, func(i, j int) bool { return plan.Conflicts[i].IdentityKey < plan.Conflicts[j].IdentityKey })
	sort.Slice(plan.Releases, func(i, j int) bool {
		return plan.Releases[i].Current.DeclarationID < plan.Releases[j].Current.DeclarationID
	})
	return plan
}

// collectSeedGroups groups valid administrator seeds by identity key and marks groups whose seeds disagree.
func collectSeedGroups(rows []Row, alcohols map[int64]Alcohol, plan *Plan) map[string]*seedGroup {
	groups := map[string]*seedGroup{}
	for _, row := range rows {
		if !isSeedDecision(row.Decision) {
			continue
		}
		if reason := invalidSeedReason(row, alcohols); reason != "" {
			plan.InvalidSeeds[reason]++
			continue
		}
		group := groups[row.IdentityKey]
		if group == nil {
			group = &seedGroup{}
			groups[row.IdentityKey] = group
		}
		group.seeds = append(group.seeds, row)
	}
	for key, group := range groups {
		resolveGroup(key, group, alcohols, plan)
		if group.conflict == "" {
			plan.SeedGroups++
		}
	}
	return groups
}

func isSeedDecision(decision string) bool {
	return decision == string(domain.DecisionCandidate) || decision == string(domain.DecisionManual)
}

func isNormalized(status string) bool {
	return status == "NORMALIZED" || status == "PARTIAL"
}

// invalidSeedReason keeps a seed only while its choice is still trustworthy. A STALE row lost the source its selection
// was made for, and a REVIEW_REQUIRED row has an unsettled name, so neither may spread.
func invalidSeedReason(row Row, alcohols map[int64]Alcohol) string {
	alcohol, ok := alcohols[row.SelectedAlcoholID]
	switch {
	case row.SelectedAlcoholID <= 0 || !ok:
		return "ALCOHOL_MISSING"
	case alcohol.Deleted:
		return "ALCOHOL_DELETED"
	case row.NormalizationStatus == "REVIEW_REQUIRED":
		return ExcludeReviewRequired
	case !isNormalized(row.NormalizationStatus):
		return ExcludeNotNormalized
	case row.IdentityKey == "":
		return "KEY_MISSING"
	default:
		return ""
	}
}

// resolveGroup picks the lowest seed ID as the recorded source so repeated runs choose the same seed.
func resolveGroup(key string, group *seedGroup, alcohols map[int64]Alcohol, plan *Plan) {
	sort.Slice(group.seeds, func(i, j int) bool { return group.seeds[i].DeclarationID < group.seeds[j].DeclarationID })
	first := group.seeds[0]
	group.selection = seedSelection(first, alcohols[first.SelectedAlcoholID])
	group.country = first.ManufactureCountryAlpha2
	for _, seed := range group.seeds[1:] {
		selection := seedSelection(seed, alcohols[seed.SelectedAlcoholID])
		switch {
		case selection.AlcoholID != group.selection.AlcoholID:
			group.conflict = "ALCOHOL_MISMATCH"
		case group.conflict == "" && (selection.DistilleryID != group.selection.DistilleryID || selection.RegionID != group.selection.RegionID):
			group.conflict = "DISTILLERY_REGION_MISMATCH"
		}
	}
	if group.conflict != "" {
		plan.Conflicts = append(plan.Conflicts, Conflict{IdentityKey: key, Reason: group.conflict, Seeds: seedRefs(group.seeds)})
	}
}

// seedSelection copies the seed's own distillery and region first. When the seed has none, it propagates the selected
// alcohol's positive IDs, the same rule api-server applies on confirmation. A 0 placeholder or NULL leaves the pair empty.
func seedSelection(seed Row, alcohol Alcohol) Selection {
	selection := Selection{SeedDeclarationID: seed.DeclarationID, AlcoholID: seed.SelectedAlcoholID}
	switch {
	case seed.SelectedDistilleryID > 0:
		selection.DistilleryID, selection.DistillerySource = seed.SelectedDistilleryID, SourceInherited
	case alcohol.DistilleryID > 0:
		selection.DistilleryID, selection.DistillerySource = alcohol.DistilleryID, SourceAlcoholPropagated
	}
	switch {
	case seed.SelectedRegionID > 0:
		selection.RegionID, selection.RegionSource = seed.SelectedRegionID, SourceInherited
	case alcohol.RegionID > 0:
		selection.RegionID, selection.RegionSource = alcohol.RegionID, SourceAlcoholPropagated
	}
	return selection
}

func targetExclusion(row Row, group *seedGroup, bundles map[int64]string) string {
	switch {
	case row.AdminReleased:
		return ExcludeAdminReleased
	case containsReason(row.Reasons, genericReason):
		return ExcludeGenericProductName
	case row.NormalizationStatus == "REVIEW_REQUIRED":
		return ExcludeReviewRequired
	case !isNormalized(row.NormalizationStatus):
		return ExcludeNotNormalized
	case row.AlcoholCategoryEN != whiskyCategory:
		return ExcludeNotWhisky
	case row.ManufactureCountryAlpha2 != group.country:
		return ExcludeCountryMismatch
	}
	// 원장 속성으로 구분할 수 없는 참조 중복(예: 발렌타인 17년 신형·구형) 중 하나를 시드가 골랐다면, 대상의
	// 1순위 후보도 같은 묶음일 때만 이어받는다. 그렇지 않으면 관리자 선택 하나가 다른 제품 신고로 번진다.
	if bundle := bundles[group.selection.AlcoholID]; bundle != "" && bundles[row.TopAlcoholCandidateID] != bundle {
		return ExcludeReferenceDuplicate
	}
	return ""
}

func releaseReasonWithoutGroup(row Row) string {
	if row.IdentityKey == "" {
		return ReleaseKeyMissing
	}
	return ReleaseSeedInvalid
}

func currentSelection(row Row) Selection {
	return Selection{
		SeedDeclarationID: row.InheritedFromDeclarationID, AlcoholID: row.SelectedAlcoholID,
		DistilleryID: row.SelectedDistilleryID, DistillerySource: row.DistillerySource,
		RegionID: row.SelectedRegionID, RegionSource: row.RegionSource,
	}
}

func seedRefs(seeds []Row) []SeedRef {
	refs := make([]SeedRef, 0, len(seeds))
	for _, seed := range seeds {
		refs = append(refs, SeedRef{DeclarationID: seed.DeclarationID, RCNO: seed.RCNO, AlcoholID: seed.SelectedAlcoholID})
	}
	return refs
}

func containsReason(reasons []string, reason string) bool {
	for _, value := range reasons {
		if value == reason {
			return true
		}
	}
	return false
}

// duplicateBundles maps each live alcohol that shares English name, ABV, age and volume with another live alcohol to a
// bundle label. Alcohols outside any bundle are absent from the map.
func duplicateBundles(alcohols map[int64]Alcohol) map[int64]string {
	members := map[string][]int64{}
	for _, alcohol := range alcohols {
		if alcohol.Deleted || strings.TrimSpace(alcohol.EngName) == "" {
			continue
		}
		label := strings.Join([]string{
			strings.ToLower(strings.TrimSpace(alcohol.EngName)), strings.TrimSpace(alcohol.ABV),
			strings.TrimSpace(alcohol.Age), strings.TrimSpace(alcohol.Volume),
		}, "\x1f")
		members[label] = append(members[label], alcohol.ID)
	}
	bundles := map[int64]string{}
	for label, ids := range members {
		if len(ids) < 2 {
			continue
		}
		for _, id := range ids {
			bundles[id] = label
		}
	}
	return bundles
}
