package matching

import (
	"errors"
	"sort"
)

const defaultRuleVersion = "mfds-reference-matching-v1"

// NewReferenceSnapshot builds a reusable immutable reference index.
func NewReferenceSnapshot(
	alcohols []AlcoholReference,
	distilleries []DistilleryReference,
	regions []RegionReference,
	version MatchingVersion,
) (*ReferenceSnapshot, error) {
	if version.RuleVersion == "" {
		return nil, errors.New("matching rule version is required")
	}
	if version.ReferenceHash == "" {
		return nil, errors.New("reference hash is required")
	}

	snapshot := &ReferenceSnapshot{
		version:       version,
		distilleryIDs: make(map[int64]struct{}, len(distilleries)),
		regionByID:    make(map[int64]indexedRegion, len(regions)),
	}

	for _, reference := range alcohols {
		if reference.ID <= 0 || reference.DeletedAt != nil {
			continue
		}
		indexed := indexedAlcohol{reference: copyAlcohol(reference)}
		indexed.names = appendName(indexed.names, "alcohol_ko", reference.KorName)
		indexed.names = appendName(indexed.names, "alcohol_en", reference.EngName)
		snapshot.alcohols = append(snapshot.alcohols, indexed)
	}
	for _, reference := range distilleries {
		if reference.ID <= 0 {
			continue
		}
		indexed := indexedDistillery{reference: reference}
		indexed.names = appendName(indexed.names, "distillery_ko", reference.KorName)
		indexed.names = appendName(indexed.names, "distillery_en", reference.EngName)
		snapshot.distilleries = append(snapshot.distilleries, indexed)
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

	sort.Slice(snapshot.alcohols, func(i, j int) bool {
		return snapshot.alcohols[i].reference.ID < snapshot.alcohols[j].reference.ID
	})
	sort.Slice(snapshot.distilleries, func(i, j int) bool {
		return snapshot.distilleries[i].reference.ID < snapshot.distilleries[j].reference.ID
	})
	sort.Slice(snapshot.regions, func(i, j int) bool {
		return snapshot.regions[i].reference.ID < snapshot.regions[j].reference.ID
	})

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
	tokens := tokenize(value)
	if len(tokens) == 0 {
		return names
	}
	return append(names, indexedName{label: label, tokens: tokens})
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
