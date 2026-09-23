package inheritance

// SeedIndex answers, while a row is being normalized, whether its identity key already has an administrator match.
// It applies the same seed validity, conflict and target exclusion rules as BuildPlan, so a row taken here ends in the
// same state the post-normalization pass would give it.
type SeedIndex struct {
	groups   map[string]*seedGroup
	seedIDs  map[int64]bool
	released map[int64]bool
	bundles  map[int64]string
}

// Target is the freshly normalized state of one declaration.
type Target struct {
	DeclarationID            int64
	IdentityKey              string
	NormalizationStatus      string
	Reasons                  []string
	AlcoholCategoryEN        string
	ManufactureCountryAlpha2 string
}

// BuildSeedIndex reads the seeds once per normalize run.
func BuildSeedIndex(rows []Row, alcohols map[int64]Alcohol) SeedIndex {
	index := SeedIndex{
		groups:  collectSeedGroups(rows, alcohols, &Plan{InvalidSeeds: map[string]int{}}),
		seedIDs: map[int64]bool{}, released: map[int64]bool{}, bundles: duplicateBundles(alcohols),
	}
	for _, row := range rows {
		if isSeedDecision(row.Decision) {
			index.seedIDs[row.DeclarationID] = true
		}
		if row.AdminReleased {
			index.released[row.DeclarationID] = true
		}
	}
	return index
}

// Lookup returns the administrator selection for the target, or false when the matcher must run. A seed that picked
// one of several indistinguishable reference alcohols needs the matcher's top candidate to decide, so it is left to
// the matcher and the post-normalization pass.
func (i SeedIndex) Lookup(target Target) (Selection, bool) {
	group := i.groups[target.IdentityKey]
	if target.IdentityKey == "" || group == nil || group.conflict != "" || i.seedIDs[target.DeclarationID] {
		return Selection{}, false
	}
	if i.bundles[group.selection.AlcoholID] != "" {
		return Selection{}, false
	}
	row := Row{
		DeclarationID: target.DeclarationID, IdentityKey: target.IdentityKey,
		NormalizationStatus: target.NormalizationStatus, Reasons: target.Reasons,
		AlcoholCategoryEN: target.AlcoholCategoryEN, ManufactureCountryAlpha2: target.ManufactureCountryAlpha2,
		AdminReleased: i.released[target.DeclarationID],
	}
	if targetExclusion(row, group, nil) != "" {
		return Selection{}, false
	}
	return group.selection, true
}
