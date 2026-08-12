package matching

import (
	"errors"
	"sort"
)

const defaultRuleVersion = "mfds-reference-matching-v4"

// NewReferenceSnapshot builds a reusable immutable reference index.
func NewReferenceSnapshot(
	alcohols []AlcoholReference,
	distilleries []DistilleryReference,
	regions []RegionReference,
	version MatchingVersion,
) (*ReferenceSnapshot, error) {
	return NewReferenceSnapshotWithAliases(alcohols, distilleries, regions, nil, version)
}

// NewReferenceSnapshotWithAliases builds the immutable index including administrator aliases.
func NewReferenceSnapshotWithAliases(
	alcohols []AlcoholReference,
	distilleries []DistilleryReference,
	regions []RegionReference,
	aliases []ReferenceAlias,
	version MatchingVersion,
) (*ReferenceSnapshot, error) {
	if version.RuleVersion == "" {
		return nil, errors.New("matching rule version is required")
	}
	if version.ReferenceHash == "" {
		return nil, errors.New("reference hash is required")
	}

	snapshot := &ReferenceSnapshot{
		version:                  version,
		alcoholByID:              make(map[int64]indexedAlcohol, len(alcohols)),
		distilleryByID:           make(map[int64]indexedDistillery, len(distilleries)),
		distilleryIDs:            make(map[int64]struct{}, len(distilleries)),
		regionByID:               make(map[int64]indexedRegion, len(regions)),
		regionCountsByDistillery: make(map[int64]map[int64]int),
		alcoholIDsByBaseKey:      make(map[string][]int64),
		distilleryIDsByBaseKey:   make(map[string][]int64),
		regionIDsByBaseKey:       make(map[string][]int64),
		alcoholIDsByToken:        make(map[string][]int64),
		distilleryIDsByToken:     make(map[string][]int64),
		regionIDsByToken:         make(map[string][]int64),
	}

	for _, reference := range alcohols {
		if reference.ID <= 0 || reference.DeletedAt != nil {
			continue
		}
		ageYears, conflicts := referenceAge(reference)
		indexed := indexedAlcohol{
			reference: copyAlcohol(reference), ageYears: ageYears,
			volumeML: parseVolumeML(reference.Volume), conflicts: conflicts,
		}
		indexed.names = appendName(indexed.names, "alcohol_ko", reference.KorName)
		indexed.names = appendName(indexed.names, "alcohol_en", reference.EngName)
		snapshot.alcohols = append(snapshot.alcohols, indexed)
		snapshot.alcoholByID[reference.ID] = indexed
	}
	for _, reference := range distilleries {
		if reference.ID <= 0 {
			continue
		}
		indexed := indexedDistillery{reference: reference}
		indexed.names = appendName(indexed.names, "distillery_ko", reference.KorName)
		indexed.names = appendName(indexed.names, "distillery_en", reference.EngName)
		snapshot.distilleries = append(snapshot.distilleries, indexed)
		snapshot.distilleryByID[reference.ID] = indexed
		snapshot.distilleryIDs[reference.ID] = struct{}{}
	}
	for _, reference := range regions {
		if reference.ID <= 0 {
			continue
		}
		indexed := indexedRegion{reference: reference}
		indexed.names = appendName(indexed.names, "region_ko", reference.KorName)
		indexed.names = appendName(indexed.names, "region_en", reference.EngName)
		snapshot.regions = append(snapshot.regions, indexed)
		snapshot.regionByID[reference.ID] = indexed
	}
	for _, alias := range aliases {
		if alias.EntityID <= 0 || alias.Alias == "" {
			continue
		}
		label := "alias_" + alias.Language + "_" + alias.Source
		switch alias.EntityType {
		case "ALCOHOL":
			indexed, ok := snapshot.alcoholByID[alias.EntityID]
			if !ok {
				continue
			}
			indexed.names = appendName(indexed.names, label, alias.Alias)
			snapshot.alcoholByID[alias.EntityID] = indexed
		case "DISTILLERY":
			indexed, ok := snapshot.distilleryByID[alias.EntityID]
			if !ok {
				continue
			}
			indexed.names = appendName(indexed.names, label, alias.Alias)
			snapshot.distilleryByID[alias.EntityID] = indexed
		case "REGION":
			indexed, ok := snapshot.regionByID[alias.EntityID]
			if !ok {
				continue
			}
			indexed.names = appendName(indexed.names, label, alias.Alias)
			snapshot.regionByID[alias.EntityID] = indexed
		}
	}
	for index, alcohol := range snapshot.alcohols {
		if aliased, ok := snapshot.alcoholByID[alcohol.reference.ID]; ok {
			snapshot.alcohols[index] = aliased
		}
	}
	for index, distillery := range snapshot.distilleries {
		if aliased, ok := snapshot.distilleryByID[distillery.reference.ID]; ok {
			snapshot.distilleries[index] = aliased
		}
	}
	for index, region := range snapshot.regions {
		if aliased, ok := snapshot.regionByID[region.reference.ID]; ok {
			snapshot.regions[index] = aliased
		}
	}
	for _, alcohol := range snapshot.alcohols {
		reference := alcohol.reference
		if !snapshot.hasDistillery(reference.DistilleryID) || !snapshot.hasRegion(reference.RegionID) {
			continue
		}
		counts := snapshot.regionCountsByDistillery[reference.DistilleryID]
		if counts == nil {
			counts = make(map[int64]int)
			snapshot.regionCountsByDistillery[reference.DistilleryID] = counts
		}
		counts[reference.RegionID]++
	}

	sort.Slice(snapshot.alcohols, func(i, j int) bool {
		return snapshot.alcohols[i].reference.ID < snapshot.alcohols[j].reference.ID
	})
	sort.Slice(snapshot.distilleries, func(i, j int) bool {
		return snapshot.distilleries[i].reference.ID < snapshot.distilleries[j].reference.ID
	})
	sort.Slice(snapshot.regions, func(i, j int) bool {
		return snapshot.regions[i].reference.ID < snapshot.regions[j].reference.ID
	})
	for _, reference := range snapshot.alcohols {
		indexNames(snapshot.alcoholIDsByBaseKey, snapshot.alcoholIDsByToken, reference.reference.ID, reference.names)
	}
	for _, reference := range snapshot.distilleries {
		indexNames(snapshot.distilleryIDsByBaseKey, snapshot.distilleryIDsByToken, reference.reference.ID, reference.names)
	}
	for _, reference := range snapshot.regions {
		indexNames(snapshot.regionIDsByBaseKey, snapshot.regionIDsByToken, reference.reference.ID, reference.names)
	}

	return snapshot, nil
}

// DefaultMatchingVersion returns the current rule version with a caller-supplied reference hash.
func DefaultMatchingVersion(referenceHash string) MatchingVersion {
	return MatchingVersion{RuleVersion: defaultRuleVersion, ReferenceHash: referenceHash}
}

// Version returns the version captured when the snapshot was built.
func (s *ReferenceSnapshot) Version() MatchingVersion {
	if s == nil {
		return MatchingVersion{}
	}
	return s.version
}

func appendName(names []indexedName, label, value string) []indexedName {
	features := extractNameFeatures(value)
	tokens := features.tokens
	if len(tokens) == 0 {
		return names
	}
	return append(names, indexedName{label: label, tokens: tokens, baseKey: features.baseKey})
}

func indexNames(byBaseKey, byToken map[string][]int64, id int64, names []indexedName) {
	for _, name := range names {
		byBaseKey[name.baseKey] = appendUniqueID(byBaseKey[name.baseKey], id)
		for _, token := range name.tokens {
			if significantToken(token) {
				byToken[token] = appendUniqueID(byToken[token], id)
			}
		}
	}
}

func appendUniqueID(values []int64, value int64) []int64 {
	for _, existing := range values {
		if existing == value {
			return values
		}
	}
	return append(values, value)
}

func copyAlcohol(reference AlcoholReference) AlcoholReference {
	copy := reference
	if reference.ABVPercent != nil {
		value := *reference.ABVPercent
		copy.ABVPercent = &value
	}
	if reference.AgeYears != nil {
		value := *reference.AgeYears
		copy.AgeYears = &value
	}
	if reference.DeletedAt != nil {
		value := *reference.DeletedAt
		copy.DeletedAt = &value
	}
	return copy
}
