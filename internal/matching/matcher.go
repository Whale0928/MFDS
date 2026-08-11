package matching

import (
	"math"
	"sort"
	"strconv"
)

const (
	weightAlcoholNameExact = 100
	weightAlcoholNameFuzzy = 25
	weightDistilleryLink   = 60
	weightRegionLink       = 50
	weightDirectDistillery = 80
	weightDirectRegion     = 70
	weightParentCompatible = 10
	weightABV              = 8
	weightAge              = 6
	weightCask             = 5
)

type scoredCandidate struct {
	candidate Candidate
}

// Match ranks independent distillery and region top-three candidates.
func (s *ReferenceSnapshot) Match(input Input) MatchResult {
	if s == nil {
		return MatchResult{}
	}
	nameTokens := inputNameTokens(input)
	distilleryScores := make(map[int64]*scoredCandidate)
	regionScores := make(map[int64]*scoredCandidate)

	for _, alcohol := range s.alcohols {
		matchLabel, matched := exactNameLabel(nameTokens, alcohol.names)
		if !matched {
			continue
		}
		nameEvidence := Evidence{Kind: "alcohol_exact", Source: matchLabel, Weight: weightAlcoholNameExact}
		attributeEvidence := alcoholAttributeEvidence(input, alcohol.reference)
		if s.hasDistillery(alcohol.reference.DistilleryID) {
			addScore(distilleryScores, alcohol.reference.DistilleryID, weightAlcoholNameExact, nameEvidence, attributeEvidence...)
			addEvidence(distilleryScores, alcohol.reference.DistilleryID, Evidence{Kind: "alcohol_distillery_id", Source: strconv.FormatInt(alcohol.reference.ID, 10), Weight: weightDistilleryLink})
		}
		if s.hasRegion(alcohol.reference.RegionID) {
			addScore(regionScores, alcohol.reference.RegionID, weightAlcoholNameExact, nameEvidence, attributeEvidence...)
			addEvidence(regionScores, alcohol.reference.RegionID, Evidence{Kind: "alcohol_region_id", Source: strconv.FormatInt(alcohol.reference.ID, 10), Weight: weightRegionLink})
			for parentID := s.parentID(alcohol.reference.RegionID); parentID > 0; parentID = s.parentID(parentID) {
				if _, ok := s.regionByID[parentID]; !ok {
					break
				}
				addEvidence(regionScores, parentID, Evidence{Kind: "region_parent_compatible", Source: strconv.FormatInt(alcohol.reference.RegionID, 10), Weight: weightParentCompatible})
			}
		}
	}

	if len(distilleryScores) == 0 {
		for _, distillery := range s.distilleries {
			for _, name := range distillery.names {
				if sequenceNameMatch(nameTokens, name.tokens) {
					addEvidence(distilleryScores, distillery.reference.ID, Evidence{Kind: "distillery_name_exact", Source: name.label, Weight: weightDirectDistillery})
				}
			}
		}
	}
	if len(regionScores) == 0 {
		for _, region := range s.regions {
			for _, name := range region.names {
				if sequenceNameMatch(nameTokens, name.tokens) {
					addEvidence(regionScores, region.reference.ID, Evidence{Kind: "region_name_exact", Source: name.label, Weight: weightDirectRegion})
				}
			}
		}
	}

	if len(distilleryScores) == 0 {
		for _, alcohol := range s.alcohols {
			if hasFuzzyNameMatch(nameTokens, alcohol.names) && s.hasDistillery(alcohol.reference.DistilleryID) {
				addEvidence(distilleryScores, alcohol.reference.DistilleryID, Evidence{Kind: "alcohol_name_fuzzy", Source: strconv.FormatInt(alcohol.reference.ID, 10), Weight: weightAlcoholNameFuzzy})
			}
		}
		for _, distillery := range s.distilleries {
			if hasFuzzyNameMatch(nameTokens, distillery.names) {
				addEvidence(distilleryScores, distillery.reference.ID, Evidence{Kind: "distillery_name_fuzzy", Source: strconv.FormatInt(distillery.reference.ID, 10), Weight: weightAlcoholNameFuzzy})
			}
		}
	}
	if len(regionScores) == 0 {
		for _, alcohol := range s.alcohols {
			if hasFuzzyNameMatch(nameTokens, alcohol.names) && s.hasRegion(alcohol.reference.RegionID) {
				addEvidence(regionScores, alcohol.reference.RegionID, Evidence{Kind: "alcohol_name_fuzzy", Source: strconv.FormatInt(alcohol.reference.ID, 10), Weight: weightAlcoholNameFuzzy})
			}
		}
		for _, region := range s.regions {
			if hasFuzzyNameMatch(nameTokens, region.names) {
				addEvidence(regionScores, region.reference.ID, Evidence{Kind: "region_name_fuzzy", Source: strconv.FormatInt(region.reference.ID, 10), Weight: weightAlcoholNameFuzzy})
			}
		}
	}

	return MatchResult{
		Version:      s.version,
		Distilleries: topThree(distilleryScores),
		Regions:      topThree(regionScores),
		Exact:        hasExactCandidate(distilleryScores) || hasExactCandidate(regionScores),
	}
}

func exactNameLabel(inputs [][]string, names []indexedName) (string, bool) {
	for _, input := range inputs {
		for _, name := range names {
			if sequenceMatch(input, name.tokens) {
				return name.label, true
			}
		}
	}
	return "", false
}

func sequenceNameMatch(inputs [][]string, candidate []string) bool {
	for _, input := range inputs {
		if sequenceMatch(input, candidate) {
			return true
		}
	}
	return false
}

func hasFuzzyNameMatch(inputs [][]string, names []indexedName) bool {
	for _, input := range inputs {
		for _, name := range names {
			if fuzzySequenceMatch(input, name.tokens) {
				return true
			}
		}
	}
	return false
}

func alcoholAttributeEvidence(input Input, reference AlcoholReference) []Evidence {
	var evidence []Evidence
	if inputValue, referenceValue := input.ABVPercent, reference.ABVPercent; inputValue != nil && referenceValue != nil && math.Abs(*inputValue-*referenceValue) <= 0.1 {
		evidence = append(evidence, Evidence{Kind: "abv_exact", Source: "abv", Weight: weightABV})
	}
	if input.AgeYears != nil && reference.AgeYears != nil && *input.AgeYears == *reference.AgeYears {
		evidence = append(evidence, Evidence{Kind: "age_exact", Source: "age", Weight: weightAge})
	} else if input.Age != "" && sequenceMatch(tokenize(input.Age), tokenize(reference.Age)) {
		evidence = append(evidence, Evidence{Kind: "age_exact", Source: "age", Weight: weightAge})
	}
	if input.Cask != "" && sequenceMatch(tokenize(input.Cask), tokenize(reference.Cask)) {
		evidence = append(evidence, Evidence{Kind: "cask_exact", Source: "cask", Weight: weightCask})
	}
	return evidence
}

func addScore(scores map[int64]*scoredCandidate, id int64, base int, evidence Evidence, attributes ...Evidence) {
	if id <= 0 {
		return
	}
	entry := scores[id]
	if entry == nil {
		entry = &scoredCandidate{candidate: Candidate{ID: id}}
		scores[id] = entry
	}
	entry.candidate.Score += base
	entry.candidate.EvidenceStrength += evidenceStrength(evidence.Kind)
	entry.candidate.Evidence = append(entry.candidate.Evidence, evidence)
	for _, attribute := range attributes {
		entry.candidate.Score += attribute.Weight
		entry.candidate.EvidenceStrength += evidenceStrength(attribute.Kind)
		entry.candidate.Evidence = append(entry.candidate.Evidence, attribute)
	}
}

func addEvidence(scores map[int64]*scoredCandidate, id int64, evidence Evidence) {
	if id <= 0 {
		return
	}
	entry := scores[id]
	if entry == nil {
		entry = &scoredCandidate{candidate: Candidate{ID: id}}
		scores[id] = entry
	}
	entry.candidate.Score += evidence.Weight
	entry.candidate.EvidenceStrength += evidenceStrength(evidence.Kind)
	entry.candidate.Evidence = append(entry.candidate.Evidence, evidence)
}

func topThree(scores map[int64]*scoredCandidate) []Candidate {
	result := make([]Candidate, 0, len(scores))
	for _, entry := range scores {
		candidate := entry.candidate
		candidate.Evidence = append([]Evidence(nil), candidate.Evidence...)
		result = append(result, candidate)
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Score != result[j].Score {
			return result[i].Score > result[j].Score
		}
		if result[i].EvidenceStrength != result[j].EvidenceStrength {
			return result[i].EvidenceStrength > result[j].EvidenceStrength
		}
		return result[i].ID < result[j].ID
	})
	if len(result) > 3 {
		result = result[:3]
	}
	return result
}

func (s *ReferenceSnapshot) parentID(id int64) int64 {
	region, ok := s.regionByID[id]
	if !ok || region.reference.ParentID <= 0 || region.reference.ParentID == id {
		return 0
	}
	return region.reference.ParentID
}

func (s *ReferenceSnapshot) hasDistillery(id int64) bool {
	_, ok := s.distilleryIDs[id]
	return ok
}

func (s *ReferenceSnapshot) hasRegion(id int64) bool {
	_, ok := s.regionByID[id]
	return ok
}

func hasExactCandidate(scores map[int64]*scoredCandidate) bool {
	for _, score := range scores {
		for _, evidence := range score.candidate.Evidence {
			if evidence.Kind == "alcohol_exact" || evidence.Kind == "distillery_name_exact" || evidence.Kind == "region_name_exact" {
				return true
			}
		}
	}
	return false
}

func evidenceStrength(kind string) int {
	switch kind {
	case "alcohol_exact", "distillery_name_exact", "region_name_exact":
		return 3
	case "alcohol_distillery_id", "alcohol_region_id":
		return 2
	case "region_parent_compatible":
		return 1
	default:
		return 1
	}
}
