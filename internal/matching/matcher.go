package matching

import (
	"math"
	"sort"
	"strconv"
	"strings"
)

const (
	weightNameExact            = 8.0
	weightAliasExact           = 7.0
	weightNameContains         = 5.0
	weightNameFuzzy            = 2.5
	weightAgeExact             = 3.0
	weightAgeConflict          = -6.0
	weightABVExact             = 2.5
	weightABVNear              = 1.0
	weightABVConflict          = -4.0
	weightEditionExact         = 2.0
	weightCaskExact            = 1.5
	weightVolumeExact          = 1.0
	weightVolumeConflict       = -0.5
	weightReferencePropagation = 7.0
	weightDistilleryPriorMax   = 4.0
	weightCountryRoot          = 1.0
	weightParentCompatible     = 0.5

	alcoholAutoThreshold   = 10.0
	alcoholAutoMargin      = 3.0
	referenceAutoThreshold = 7.0
	referenceAutoMargin    = 2.0
	weightDirectRegion     = weightNameExact
)

type scoredCandidate struct {
	candidate Candidate
}

// RuleWeights returns an immutable snapshot suitable for run audit records.
func RuleWeights() map[string]float64 {
	return map[string]float64{
		"name_exact": weightNameExact, "alias_exact": weightAliasExact,
		"name_contains": weightNameContains, "name_fuzzy": weightNameFuzzy,
		"age_exact": weightAgeExact, "age_conflict": weightAgeConflict,
		"abv_exact": weightABVExact, "abv_near": weightABVNear, "abv_conflict": weightABVConflict,
		"edition_exact": weightEditionExact, "cask_exact": weightCaskExact,
		"volume_exact": weightVolumeExact, "volume_conflict": weightVolumeConflict,
		"reference_propagation": weightReferencePropagation,
		"distillery_prior_max":  weightDistilleryPriorMax, "country_root": weightCountryRoot,
	}
}

// Match executes alcohol-first matching and then independent distillery and region fallback.
func (s *ReferenceSnapshot) Match(input Input) MatchResult {
	if s == nil {
		return MatchResult{}
	}
	inputNames := inputIndexedNames(input)
	alcoholScores := s.scoreAlcohols(input, inputNames)
	allAlcohols := rankCandidates(alcoholScores)
	alcoholDecision := decideAlcohol(allAlcohols)
	consensus := s.alcoholConsensus(allAlcohols)

	distilleryScores := s.scoreDistilleries(inputNames)
	propagatedDistilleryID, distillerySource := s.propagatedReference(alcoholDecision, consensus, true)
	if propagatedDistilleryID > 0 {
		addReferencePropagation(distilleryScores, s.distilleryCandidate(propagatedDistilleryID), distillerySource, alcoholDecision.SelectedID)
	}
	allDistilleries := rankCandidates(distilleryScores)
	distilleryDecision := decideReference(allDistilleries, propagatedDistilleryID, distillerySource)

	regionScores := s.scoreRegions(inputNames)
	propagatedRegionID, regionSource := s.propagatedReference(alcoholDecision, consensus, false)
	if propagatedRegionID > 0 {
		addReferencePropagation(regionScores, s.regionCandidate(propagatedRegionID), regionSource, alcoholDecision.SelectedID)
	}
	s.addRegionsFromDistilleryPrior(regionScores, distilleryDecision)
	s.addCountryRootRegion(regionScores, input.ManufactureCountry)
	allRegions := rankCandidates(regionScores)
	regionDecision := decideReference(allRegions, propagatedRegionID, regionSource)

	return MatchResult{
		Version:            s.version,
		Alcohols:           topCandidates(allAlcohols),
		Distilleries:       topCandidates(allDistilleries),
		Regions:            topCandidates(allRegions),
		AlcoholDecision:    alcoholDecision,
		DistilleryDecision: distilleryDecision,
		RegionDecision:     regionDecision,
		AlcoholConsensus:   consensus,
		Exact:              containsExactEvidence(allAlcohols) || containsExactEvidence(allDistilleries) || containsExactEvidence(allRegions),
	}
}

func (s *ReferenceSnapshot) scoreAlcohols(input Input, inputNames []indexedName) map[int64]*scoredCandidate {
	scores := make(map[int64]*scoredCandidate)
	ids := blockedCandidateIDs(inputNames, s.alcoholIDsByBaseKey, s.alcoholIDsByToken)
	if len(ids) == 0 {
		for _, reference := range s.alcohols {
			ids = append(ids, reference.reference.ID)
		}
	}
	inputAge := bestInputAge(input)
	for _, id := range ids {
		reference, ok := s.alcoholByID[id]
		if !ok {
			continue
		}
		nameEvidence, matched := bestNameEvidence(inputNames, reference.names, "alcohol")
		if !matched {
			continue
		}
		candidate := Candidate{ID: id, NameKO: reference.reference.KorName, NameEN: reference.reference.EngName}
		addCandidateEvidence(&candidate, nameEvidence)
		for _, evidence := range attributeEvidence(input, inputAge, reference) {
			addCandidateEvidence(&candidate, evidence)
		}
		for _, conflict := range reference.conflicts {
			addCandidateEvidence(&candidate, Evidence{Kind: conflict, Source: "reference", RuleCode: "REFERENCE_ATTRIBUTE_CONFLICT"})
		}
		scores[id] = &scoredCandidate{candidate: candidate}
	}
	return scores
}

func (s *ReferenceSnapshot) scoreDistilleries(inputNames []indexedName) map[int64]*scoredCandidate {
	scores := make(map[int64]*scoredCandidate)
	ids := blockedCandidateIDs(inputNames, s.distilleryIDsByBaseKey, s.distilleryIDsByToken)
	if len(ids) == 0 {
		for _, reference := range s.distilleries {
			ids = append(ids, reference.reference.ID)
		}
	}
	for _, id := range ids {
		reference, ok := s.distilleryByID[id]
		if !ok {
			continue
		}
		evidence, matched := bestNameEvidence(inputNames, reference.names, "distillery")
		if !matched {
			continue
		}
		candidate := Candidate{ID: id, NameKO: reference.reference.KorName, NameEN: reference.reference.EngName}
		addCandidateEvidence(&candidate, evidence)
		scores[id] = &scoredCandidate{candidate: candidate}
	}
	return scores
}

func (s *ReferenceSnapshot) scoreRegions(inputNames []indexedName) map[int64]*scoredCandidate {
	scores := make(map[int64]*scoredCandidate)
	ids := blockedCandidateIDs(inputNames, s.regionIDsByBaseKey, s.regionIDsByToken)
	if len(ids) == 0 {
		for _, reference := range s.regions {
			ids = append(ids, reference.reference.ID)
		}
	}
	for _, id := range ids {
		reference, ok := s.regionByID[id]
		if !ok {
			continue
		}
		evidence, matched := bestNameEvidence(inputNames, reference.names, "region")
		if !matched {
			continue
		}
		candidate := Candidate{ID: id, NameKO: reference.reference.KorName, NameEN: reference.reference.EngName}
		addCandidateEvidence(&candidate, evidence)
		scores[id] = &scoredCandidate{candidate: candidate}
	}
	return scores
}

func blockedCandidateIDs(inputs []indexedName, byBaseKey, byToken map[string][]int64) []int64 {
	seen := make(map[int64]struct{})
	for _, input := range inputs {
		for _, id := range byBaseKey[input.baseKey] {
			seen[id] = struct{}{}
		}
		for _, token := range input.tokens {
			if !significantToken(token) {
				continue
			}
			for _, id := range byToken[token] {
				seen[id] = struct{}{}
			}
		}
	}
	ids := make([]int64, 0, len(seen))
	for id := range seen {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	return ids
}

func bestNameEvidence(inputs, references []indexedName, entity string) (Evidence, bool) {
	best := Evidence{}
	for _, input := range inputs {
		for _, reference := range references {
			evidence, matched := compareNames(input, reference, entity)
			if matched && evidence.Weight > best.Weight {
				best = evidence
			}
		}
	}
	return best, best.Kind != ""
}

func compareNames(input, reference indexedName, entity string) (Evidence, bool) {
	source := reference.label
	base := Evidence{
		Source: source, InputValue: input.baseKey, ReferenceValue: reference.baseKey,
		RuleCode: "NAME_COMPARISON_V4",
	}
	if input.baseKey != "" && input.baseKey == reference.baseKey {
		base.Kind = entity + "_name_exact"
		base.Weight = weightNameExact
		if strings.HasPrefix(reference.label, "alias_") {
			base.Kind = entity + "_alias_exact"
			base.Weight = weightAliasExact
		}
		return base, true
	}
	if bidirectionalSequenceMatch(input.tokens, reference.tokens) {
		base.Kind = entity + "_name_contains"
		base.Weight = weightNameContains
		return base, true
	}
	if tokenJaccard(input.tokens, reference.tokens) >= 0.6 {
		base.Kind = entity + "_name_token_fuzzy"
		base.Weight = weightNameFuzzy
		return base, true
	}
	if len(input.tokens) == 1 && len(reference.tokens) == 1 &&
		len([]rune(input.tokens[0])) >= 9 && len([]rune(reference.tokens[0])) >= 9 &&
		levenshteinDistance(input.tokens[0], reference.tokens[0]) <= 1 {
		base.Kind = entity + "_name_fuzzy"
		base.Weight = weightNameFuzzy
		return base, true
	}
	return Evidence{}, false
}

func attributeEvidence(input Input, inputAge *int, reference indexedAlcohol) []Evidence {
	evidence := make([]Evidence, 0, 6)
	if inputAge != nil && reference.ageYears != nil {
		item := Evidence{Kind: "age_exact", Source: "age", InputValue: strconv.Itoa(*inputAge), ReferenceValue: strconv.Itoa(*reference.ageYears), RuleCode: "AGE_CANONICAL_V4", Weight: weightAgeExact}
		if *inputAge != *reference.ageYears {
			item.Kind, item.Weight = "age_conflict", weightAgeConflict
		}
		evidence = append(evidence, item)
	}
	if input.ABVPercent != nil && reference.reference.ABVPercent != nil {
		delta := math.Abs(*input.ABVPercent - *reference.reference.ABVPercent)
		item := Evidence{Source: "abv", InputValue: strconv.FormatFloat(*input.ABVPercent, 'f', 3, 64), ReferenceValue: strconv.FormatFloat(*reference.reference.ABVPercent, 'f', 3, 64), RuleCode: "ABV_TOLERANCE_V4"}
		switch {
		case delta <= 0.05:
			item.Kind, item.Weight = "abv_exact", weightABVExact
		case delta <= 0.5:
			item.Kind, item.Weight = "abv_near", weightABVNear
		default:
			item.Kind, item.Weight = "abv_conflict", weightABVConflict
		}
		evidence = append(evidence, item)
	}
	if input.UnitVolumeML != nil && reference.volumeML != nil {
		item := Evidence{Kind: "volume_exact", Source: "volume", InputValue: strconv.Itoa(*input.UnitVolumeML), ReferenceValue: strconv.Itoa(*reference.volumeML), RuleCode: "VOLUME_ML_V4", Weight: weightVolumeExact}
		if *input.UnitVolumeML != *reference.volumeML {
			item.Kind, item.Weight = "volume_conflict", weightVolumeConflict
		}
		evidence = append(evidence, item)
	}
	if input.Cask != "" && reference.reference.Cask != "" && bidirectionalSequenceMatch(tokenize(input.Cask), tokenize(reference.reference.Cask)) {
		evidence = append(evidence, Evidence{Kind: "cask_exact", Source: "cask", InputValue: input.Cask, ReferenceValue: reference.reference.Cask, RuleCode: "CASK_TOKEN_V4", Weight: weightCaskExact})
	}
	if input.Edition != "" && referenceNameContains(reference.names, input.Edition) {
		evidence = append(evidence, Evidence{Kind: "edition_exact", Source: "edition", InputValue: input.Edition, ReferenceValue: input.Edition, RuleCode: "EDITION_TOKEN_V4", Weight: weightEditionExact})
	}
	if incompatibleCategory(input.Category, reference.reference.Type+" "+reference.reference.Category) {
		evidence = append(evidence, Evidence{Kind: "category_conflict", Source: "category", InputValue: input.Category, ReferenceValue: reference.reference.Category, RuleCode: "CATEGORY_GUARD_V4", Weight: -100})
	}
	return evidence
}

func inputIndexedNames(input Input) []indexedName {
	values := []struct{ label, value string }{
		{"input_base_ko", input.BaseNameKO}, {"input_base_en", input.BaseNameEN},
		{"input_search_ko", input.SearchNameKO}, {"input_search_en", input.SearchNameEN},
	}
	result := make([]indexedName, 0, len(values))
	for _, value := range values {
		features := extractNameFeatures(value.value)
		if len(features.tokens) > 0 {
			result = append(result, indexedName{label: value.label, tokens: features.tokens, baseKey: features.baseKey})
		}
	}
	return result
}

func referenceNameContains(names []indexedName, value string) bool {
	tokens := extractNameFeatures(value).tokens
	for _, name := range names {
		if bidirectionalSequenceMatch(name.tokens, tokens) {
			return true
		}
	}
	return false
}

func addCandidateEvidence(candidate *Candidate, evidence Evidence) {
	if candidate == nil || evidence.Kind == "" {
		return
	}
	for _, existing := range candidate.Evidence {
		if existing.Kind == evidence.Kind {
			return
		}
	}
	candidate.Score += evidence.Weight
	candidate.EvidenceStrength += evidenceStrength(evidence.Kind)
	candidate.Evidence = append(candidate.Evidence, evidence)
}

func rankCandidates(scores map[int64]*scoredCandidate) []Candidate {
	result := make([]Candidate, 0, len(scores))
	for _, entry := range scores {
		candidate := entry.candidate
		if hasEvidence(candidate, "category_conflict") {
			continue
		}
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
	return result
}

func topCandidates(candidates []Candidate) []Candidate {
	if len(candidates) <= maxStoredCandidates {
		return append([]Candidate(nil), candidates...)
	}
	return append([]Candidate(nil), candidates[:maxStoredCandidates]...)
}

func decideAlcohol(candidates []Candidate) MatchDecision {
	if len(candidates) == 0 {
		return MatchDecision{Status: DecisionNoMatch, Source: "STAGE_A", StopReason: "A_NONE"}
	}
	top := candidates[0]
	margin := scoreMargin(candidates)
	competitive := competitiveCandidates(candidates, alcoholAutoThreshold, alcoholAutoMargin)
	decision := MatchDecision{Status: DecisionReview, Source: "STAGE_A", StopReason: "A_WEAK", TopScore: top.Score, Margin: margin, CompetitiveCount: len(competitive)}
	if hasAutoConflict(top) {
		decision.Status, decision.StopReason = DecisionConflictReview, "A_ATTRIBUTE_CONFLICT"
		return decision
	}
	if top.Score >= alcoholAutoThreshold && len(competitive) > 1 {
		decision.Status, decision.StopReason = DecisionAmbiguous, "A_AMBIGUOUS"
		return decision
	}
	if top.Score >= alcoholAutoThreshold && margin >= alcoholAutoMargin {
		decision.Status, decision.StopReason, decision.SelectedID = DecisionAutoSelected, "A_UNIQUE_STRONG", top.ID
	}
	return decision
}

func decideReference(candidates []Candidate, propagatedID int64, source string) MatchDecision {
	if len(candidates) == 0 {
		return MatchDecision{Status: DecisionNoMatch, Source: "STAGE_B", StopReason: "B_NONE"}
	}
	top := candidates[0]
	decision := MatchDecision{Status: DecisionReview, Source: "STAGE_B", StopReason: "B_WEAK", TopScore: top.Score, Margin: scoreMargin(candidates), CompetitiveCount: len(competitiveCandidates(candidates, referenceAutoThreshold, referenceAutoMargin))}
	if propagatedID > 0 {
		if directConflict(candidates, propagatedID) {
			decision.Status, decision.StopReason = DecisionConflictReview, "B_CONFLICTS_WITH_A"
			return decision
		}
		decision.Status, decision.Source, decision.StopReason, decision.SelectedID = DecisionAutoSelected, source, "B_FROM_ALCOHOL", propagatedID
		return decision
	}
	if top.Score >= referenceAutoThreshold && decision.Margin >= referenceAutoMargin && hasStrongDirectName(top) {
		decision.Status, decision.Source, decision.StopReason, decision.SelectedID = DecisionAutoSelected, "DIRECT", "B_UNIQUE_DIRECT", top.ID
	}
	return decision
}

func (s *ReferenceSnapshot) alcoholConsensus(candidates []Candidate) AlcoholConsensus {
	competitive := competitiveCandidates(candidates, alcoholAutoThreshold, alcoholAutoMargin)
	if len(competitive) < 2 {
		return AlcoholConsensus{}
	}
	consensus := AlcoholConsensus{}
	for index, candidate := range competitive {
		reference, ok := s.alcoholByID[candidate.ID]
		if !ok {
			return AlcoholConsensus{}
		}
		if index == 0 {
			consensus.DistilleryID = reference.reference.DistilleryID
			consensus.RegionID = reference.reference.RegionID
			continue
		}
		if consensus.DistilleryID != reference.reference.DistilleryID {
			consensus.DistilleryID = 0
		}
		if consensus.RegionID != reference.reference.RegionID {
			consensus.RegionID = 0
		}
	}
	return consensus
}

func (s *ReferenceSnapshot) propagatedReference(decision MatchDecision, consensus AlcoholConsensus, distillery bool) (int64, string) {
	if decision.SelectedID > 0 {
		if reference, ok := s.alcoholByID[decision.SelectedID]; ok {
			if distillery {
				return reference.reference.DistilleryID, "ALCOHOL_PROPAGATED"
			}
			return reference.reference.RegionID, "ALCOHOL_PROPAGATED"
		}
	}
	if distillery && consensus.DistilleryID > 0 {
		return consensus.DistilleryID, "ALCOHOL_CONSENSUS"
	}
	if !distillery && consensus.RegionID > 0 {
		return consensus.RegionID, "ALCOHOL_CONSENSUS"
	}
	return 0, ""
}

func addReferencePropagation(scores map[int64]*scoredCandidate, candidate Candidate, source string, upstreamID int64) {
	if candidate.ID <= 0 {
		return
	}
	entry := scores[candidate.ID]
	if entry == nil {
		entry = &scoredCandidate{candidate: candidate}
		scores[candidate.ID] = entry
	}
	addCandidateEvidence(&entry.candidate, Evidence{Kind: strings.ToLower(source), Source: source, RuleCode: "REFERENCE_PATH_V4", Weight: weightReferencePropagation, UpstreamCandidateID: upstreamID})
}

func (s *ReferenceSnapshot) addRegionsFromDistilleryPrior(scores map[int64]*scoredCandidate, decision MatchDecision) {
	var distilleryID int64
	if decision.SelectedID > 0 {
		distilleryID = decision.SelectedID
	}
	counts := s.regionCountsByDistillery[distilleryID]
	if len(counts) == 0 {
		return
	}
	total := 0
	for _, count := range counts {
		total += count
	}
	for regionID, count := range counts {
		candidate := s.regionCandidate(regionID)
		entry := scores[regionID]
		if entry == nil {
			entry = &scoredCandidate{candidate: candidate}
			scores[regionID] = entry
		}
		weight := weightDistilleryPriorMax * float64(count) / float64(total)
		addCandidateEvidence(&entry.candidate, Evidence{Kind: "region_from_distillery_prior", Source: strconv.FormatInt(distilleryID, 10), InputValue: strconv.Itoa(count), ReferenceValue: strconv.Itoa(total), RuleCode: "DISTILLERY_REGION_PRIOR_V4", Weight: weight, UpstreamCandidateID: distilleryID})
		for parentID := s.parentID(regionID); parentID > 0; parentID = s.parentID(parentID) {
			parent := scores[parentID]
			if parent == nil {
				parent = &scoredCandidate{candidate: s.regionCandidate(parentID)}
				scores[parentID] = parent
			}
			addCandidateEvidence(&parent.candidate, Evidence{Kind: "region_parent_compatible", Source: strconv.FormatInt(regionID, 10), RuleCode: "REGION_PARENT_V4", Weight: weightParentCompatible, UpstreamCandidateID: regionID})
		}
	}
}

func (s *ReferenceSnapshot) addCountryRootRegion(scores map[int64]*scoredCandidate, country string) {
	features := extractNameFeatures(country)
	if len(features.tokens) == 0 {
		return
	}
	input := indexedName{label: "manufacture_country", tokens: features.tokens, baseKey: features.baseKey}
	for _, region := range s.regions {
		if region.reference.ParentID > 0 {
			continue
		}
		for _, name := range region.names {
			if input.baseKey != name.baseKey {
				continue
			}
			entry := scores[region.reference.ID]
			if entry == nil {
				entry = &scoredCandidate{candidate: s.regionCandidate(region.reference.ID)}
				scores[region.reference.ID] = entry
			}
			addCandidateEvidence(&entry.candidate, Evidence{Kind: "region_from_country_root", Source: "manufacture_country", InputValue: country, ReferenceValue: name.baseKey, RuleCode: "COUNTRY_ROOT_V4", Weight: weightCountryRoot})
			break
		}
	}
}

func (s *ReferenceSnapshot) distilleryCandidate(id int64) Candidate {
	reference, ok := s.distilleryByID[id]
	if !ok {
		return Candidate{}
	}
	return Candidate{ID: id, NameKO: reference.reference.KorName, NameEN: reference.reference.EngName}
}

func (s *ReferenceSnapshot) regionCandidate(id int64) Candidate {
	reference, ok := s.regionByID[id]
	if !ok {
		return Candidate{}
	}
	return Candidate{ID: id, NameKO: reference.reference.KorName, NameEN: reference.reference.EngName}
}

func (s *ReferenceSnapshot) hasDistillery(id int64) bool {
	_, ok := s.distilleryByID[id]
	return ok
}

func (s *ReferenceSnapshot) hasRegion(id int64) bool {
	_, ok := s.regionByID[id]
	return ok
}

func (s *ReferenceSnapshot) parentID(id int64) int64 {
	region, ok := s.regionByID[id]
	if !ok || region.reference.ParentID <= 0 || region.reference.ParentID == id {
		return 0
	}
	return region.reference.ParentID
}

func competitiveCandidates(candidates []Candidate, threshold, margin float64) []Candidate {
	if len(candidates) == 0 || candidates[0].Score < threshold {
		return nil
	}
	topScore := candidates[0].Score
	result := make([]Candidate, 0, len(candidates))
	for _, candidate := range candidates {
		if candidate.Score < threshold || topScore-candidate.Score >= margin {
			break
		}
		result = append(result, candidate)
	}
	return result
}

func scoreMargin(candidates []Candidate) float64 {
	if len(candidates) < 2 {
		return math.Inf(1)
	}
	return candidates[0].Score - candidates[1].Score
}

func hasAutoConflict(candidate Candidate) bool {
	for _, kind := range []string{"age_conflict", "abv_conflict", "reference_age_name_conflict", "reference_age_attribute_conflict", "category_conflict"} {
		if hasEvidence(candidate, kind) {
			return true
		}
	}
	return false
}

func directConflict(candidates []Candidate, propagatedID int64) bool {
	for _, candidate := range candidates {
		if candidate.ID != propagatedID && candidate.Score >= referenceAutoThreshold && hasStrongDirectName(candidate) {
			return true
		}
	}
	return false
}

func hasStrongDirectName(candidate Candidate) bool {
	for _, evidence := range candidate.Evidence {
		if strings.HasSuffix(evidence.Kind, "_name_exact") || strings.HasSuffix(evidence.Kind, "_alias_exact") {
			return true
		}
	}
	return false
}

func hasEvidence(candidate Candidate, kind string) bool {
	for _, evidence := range candidate.Evidence {
		if evidence.Kind == kind {
			return true
		}
	}
	return false
}

func containsExactEvidence(candidates []Candidate) bool {
	for _, candidate := range candidates {
		if hasStrongDirectName(candidate) {
			return true
		}
	}
	return false
}

func evidenceStrength(kind string) int {
	switch {
	case strings.HasSuffix(kind, "_name_exact"):
		return 4
	case strings.HasSuffix(kind, "_alias_exact"):
		return 3
	case kind == "age_exact" || kind == "abv_exact" || strings.HasPrefix(kind, "alcohol_"):
		return 2
	default:
		return 1
	}
}

func incompatibleCategory(input, reference string) bool {
	left, right := categoryGroup(input), categoryGroup(reference)
	return left != "" && right != "" && left != right
}

func categoryGroup(value string) string {
	lower := strings.ToLower(value)
	groups := []struct {
		name  string
		words []string
	}{
		{name: "WHISKY", words: []string{"whisky", "whiskey", "위스키"}},
		{name: "BRANDY", words: []string{"brandy", "브랜디"}},
		{name: "LIQUEUR", words: []string{"liqueur", "리큐르"}},
		{name: "SAKE", words: []string{"sake", "청주", "사케"}},
		{name: "BEER", words: []string{"beer", "맥주"}},
		{name: "WINE", words: []string{"wine", "와인"}},
		{name: "SOJU", words: []string{"soju", "소주"}},
	}
	for _, group := range groups {
		for _, word := range group.words {
			if strings.Contains(lower, word) {
				return group.name
			}
		}
	}
	return ""
}
