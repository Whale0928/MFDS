package normalization

import (
	"math"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"
)

type abvPattern struct {
	expression              *regexp.Regexp
	valueGroup              int
	overridesIngredientHint bool
}

type percentOccurrence struct {
	raw        string
	value      float64
	start, end int
	explicit   bool
}

var (
	compositionPattern = regexp.MustCompile(`(?i)인삼|송이|향료?|과즙|농축(?:액)?|원액|함유|추출물|침출액|주스|시럽|꿀|설탕|보리|호밀|몰트|곡물|아가베|RYE|ISLAY|POIRE|PEAR|APPLE|FRUIT|JUICE|EXTRACT|FLAVOU?R|CONCENTRATE|HONEY|SUGAR|MALT|GRAIN|AGAVE`)
	ingredientPrefix   = regexp.MustCompile(`(?i)(?:인삼|송이|향료?|과즙|농축(?:액)?|원액|추출물|침출액|주스|시럽|꿀|설탕|보리|호밀|몰트|곡물|아가베|RYE|ISLAY|POIRE|PEAR|APPLE|FRUIT|JUICE|EXTRACT|FLAVOU?R|CONCENTRATE|HONEY|SUGAR|MALT|GRAIN|AGAVE)\s*[:=]?\s*$`)
	percentPattern     = regexp.MustCompile(`(\d+(?:\.\d+)?)\s*%`)
	proofPattern       = regexp.MustCompile(`(?i)(\d+(?:\.\d+)?)\s*(?:PROOF|프루프)`)

	strongABVPattern = regexp.MustCompile(`(?i)(?:주도\s*|ALC\.?\s*|ABV\s*[:.]?\s*)\d+(?:\.\d+)?\s*(?:%|도)|\(\s*\d+(?:\.\d+)?\s*%\s*(?:,\s*\d+(?:\.\d+)?\s*(?:ML|L))?\s*\)|(?:^|[\s(])\d+(?:\.\d+)?\s*%\s*(?:VOL\.?\b|,?\s*\d+(?:\.\d+)?\s*(?:ML|L)\b|$)|(?:^|\s)\d+(?:\.\d+)?\s*도\s*$`)
	koABVPattern     = regexp.MustCompile(`(?:주도\s*)\d+(?:\.\d+)?\s*(?:%|도)|(?:^|\s)\d+(?:\.\d+)?\s*도\s*$`)

	strengthPattern  = regexp.MustCompile(`(?i)\b(?:CASK|BARREL)\s+(?:STRENGTH|STRENGHT|STRENGH|STRENCH)\b|\bOVERPROOF\b|캐스크\s*(?:스트렝스|스트랭스)|(?:배럴|바렐)\s*(?:스트렝스|스트랭스)`)
	englishStrength  = regexp.MustCompile(`(?i)\b(CASK|BARREL)\s+(?:STRENGTH|STRENGHT|STRENGH|STRENCH)\b`)
	koCaskStrength   = regexp.MustCompile(`캐스크\s*(?:스트렝스|스트랭스)`)
	koBarrelStrength = regexp.MustCompile(`(?:배럴|바렐)\s*(?:스트렝스|스트랭스)`)
	standaloneCS     = regexp.MustCompile(`(?i)(?:^|[^A-Z0-9])CS(?:$|[^A-Z0-9])`)

	abvPatterns = []abvPattern{
		{regexp.MustCompile(`(?i)(?:주도\s*|ALC\.?\s*|ABV\s*[:.]?\s*)(\d+(?:\.\d+)?)\s*(?:%|도)`), 1, true},
		{regexp.MustCompile(`(?i)\(\s*(\d+(?:\.\d+)?)\s*%\s*,\s*\d+(?:\.\d+)?\s*(?:ML|L)\s*\)`), 1, true},
		{regexp.MustCompile(`(?i)(?:^|[\s(])(\d+(?:\.\d+)?)\s*%\s*,?\s*\d+(?:\.\d+)?\s*(?:ML|L)\b`), 1, false},
		{regexp.MustCompile(`(?i)^\s*(\d+(?:\.\d+)?)\s*%\s*VOL\.?\b`), 1, true},
		{regexp.MustCompile(`(?i)(?:^|[\s(])(\d+(?:\.\d+)?)\s*%\s*VOL\.?\b`), 1, true},
		{regexp.MustCompile(`\(\s*(\d+(?:\.\d+)?)\s*%\s*\)`), 1, false},
		{regexp.MustCompile(`(?:^|\s)(\d+(?:\.\d+)?)\s*%\s*$`), 1, false},
		{regexp.MustCompile(`(?:^|\s)(\d+(?:\.\d+)?)\s*도\s*$`), 1, false},
	}
)

func parseIngredientPercent(ko, en string, state *derivationState) {
	seen := map[string]percentOccurrence{}
	order := []string{}
	for _, source := range []string{ko, en} {
		for _, occurrence := range ingredientPercentOccurrences(source) {
			key := formatNumber(occurrence.value)
			if _, exists := seen[key]; exists {
				continue
			}
			seen[key] = occurrence
			order = append(order, key)
		}
	}
	if len(order) == 0 {
		return
	}
	raw := make([]string, 0, len(order))
	for _, key := range order {
		raw = append(raw, seen[key].raw)
	}
	state.result.IngredientPercentRaw = strings.Join(raw, ", ")
	state.structured++
	state.add(ReasonABVCompositionContext)
	if len(order) == 1 {
		state.result.IngredientPercent = floatPointer(seen[order[0]].value)
		return
	}
	state.review(ReasonIngredientPercentMultiple, state.result.IngredientPercentRaw)
}

func ingredientPercentOccurrences(value string) []percentOccurrence {
	result := []percentOccurrence{}
	for _, indexes := range percentPattern.FindAllStringSubmatchIndex(value, -1) {
		if len(indexes) < 4 || explicitABVAnchorContains(value, indexes[0], indexes[1]) || !isIngredientPercentAt(value, indexes[0], indexes[1]) {
			continue
		}
		parsed, err := strconv.ParseFloat(value[indexes[2]:indexes[3]], 64)
		if err != nil {
			continue
		}
		result = append(result, percentOccurrence{raw: strings.TrimSpace(value[indexes[0]:indexes[1]]), value: parsed, start: indexes[0], end: indexes[1]})
	}
	return result
}

func explicitABVAnchorContains(value string, start, end int) bool {
	for _, candidatePattern := range abvPatterns {
		if !candidatePattern.overridesIngredientHint {
			continue
		}
		for _, indexes := range candidatePattern.expression.FindAllStringSubmatchIndex(value, -1) {
			if len(indexes) >= 2 && start >= indexes[0] && end <= indexes[1] {
				return true
			}
		}
	}
	return false
}

func isIngredientPercentAt(value string, start, end int) bool {
	runes := []rune(value)
	startRune := utf8.RuneCountInString(value[:start])
	endRune := startRune + utf8.RuneCountInString(value[start:end])
	windowStart, windowEnd := startRune-16, endRune+16
	if windowStart < 0 {
		windowStart = 0
	}
	if windowEnd > len(runes) {
		windowEnd = len(runes)
	}
	return compositionPattern.MatchString(string(runes[windowStart:windowEnd]))
}

func parseABV(ko, en string, state *derivationState) {
	candidates := []percentOccurrence{}
	ingredientValues := map[string]struct{}{}
	for _, source := range []string{ko, en} {
		for _, occurrence := range ingredientPercentOccurrences(source) {
			if isExplicitIngredientPercentAt(source, occurrence.start) {
				ingredientValues[formatNumber(occurrence.value)] = struct{}{}
			}
		}
	}
	for _, source := range []string{ko, en} {
		candidate, ok := anchoredABV(source)
		if ok && !candidate.explicit {
			_, ok = ingredientValues[formatNumber(candidate.value)]
			ok = !ok
		}
		if ok {
			candidates = append(candidates, candidate)
			if state.result.ABVRaw == "" {
				state.result.ABVRaw = candidate.raw
			}
		}
		for _, indexes := range percentPattern.FindAllStringSubmatchIndex(source, -1) {
			if len(indexes) < 4 || isIngredientPercentAt(source, indexes[0], indexes[1]) || ok && indexes[0] >= candidate.start && indexes[1] <= candidate.end {
				continue
			}
			state.review(ReasonABVAmbiguousPosition, strings.TrimSpace(source[indexes[0]:indexes[1]]))
			break
		}
	}
	if len(candidates) == 0 {
		return
	}
	first := candidates[0].value
	for _, candidate := range candidates[1:] {
		if math.Abs(candidate.value-first) > 0.0000001 {
			state.review(ReasonABVConflict, formatNumber(first)+"% / "+formatNumber(candidate.value)+"%")
			return
		}
	}
	if first <= 0 || first > 70 {
		state.review(ReasonABVOutOfAutomaticRange, formatNumber(first)+"%")
		return
	}
	state.result.ABVPercent = floatPointer(first)
	state.structured++
}

func isExplicitIngredientPercentAt(value string, start int) bool {
	runes := []rune(value[:start])
	windowStart := len(runes) - 16
	if windowStart < 0 {
		windowStart = 0
	}
	return ingredientPrefix.MatchString(string(runes[windowStart:]))
}

func anchoredABV(value string) (percentOccurrence, bool) {
	for _, candidatePattern := range abvPatterns {
		indexes := candidatePattern.expression.FindStringSubmatchIndex(value)
		group := candidatePattern.valueGroup * 2
		if len(indexes) <= group+1 || indexes[group] < 0 || !candidatePattern.overridesIngredientHint && isIngredientPercentAt(value, indexes[group], indexes[group+1]) {
			continue
		}
		parsed, err := strconv.ParseFloat(value[indexes[group]:indexes[group+1]], 64)
		if err != nil {
			continue
		}
		return percentOccurrence{raw: strings.TrimSpace(value[indexes[0]:indexes[1]]), value: parsed, start: indexes[0], end: indexes[1], explicit: candidatePattern.overridesIngredientHint}, true
	}
	return percentOccurrence{}, false
}

func parseProofAndStrength(ko, en string, state *derivationState) {
	combined := strings.TrimSpace(ko + " " + en)
	if match := proofPattern.FindStringSubmatch(combined); len(match) == 2 {
		if proof, err := strconv.ParseFloat(match[1], 64); err == nil {
			state.result.ProofRaw = match[0]
			state.result.ProofValue = floatPointer(proof)
			state.structured++
			state.add(ReasonProofPreserved)
			state.review(ReasonProofPreserved, match[0])
		}
	}
	strength := confirmedStrength(ko)
	if strength == "" {
		strength = confirmedStrength(en)
	}
	koCS, enCS := standaloneCS.MatchString(ko), standaloneCS.MatchString(en)
	if strength == "" {
		switch {
		case koCS && confirmedStrength(en) != "":
			strength = confirmedStrength(en)
		case enCS && confirmedStrength(ko) != "":
			strength = confirmedStrength(ko)
		}
	}
	if strength != "" {
		state.result.StrengthType = strength
		state.structured++
		return
	}
	if koCS || enCS {
		setVariantMarker(state, variantMarker{raw: "CS", markerType: VariantMarkerTypeStrengthAbbreviation, value: "CS"})
		state.review(ReasonStrengthAbbreviationAmbiguous, "CS")
	}
}

func confirmedStrength(value string) string {
	if match := englishStrength.FindStringSubmatch(value); len(match) == 2 {
		return strings.ToUpper(match[1]) + " STRENGTH"
	}
	switch {
	case koCaskStrength.MatchString(value):
		return "CASK STRENGTH"
	case koBarrelStrength.MatchString(value):
		return "BARREL STRENGTH"
	case strings.Contains(strings.ToUpper(value), "OVERPROOF"):
		return "OVERPROOF"
	default:
		return ""
	}
}

func isIngredientPercent(value, percent string) bool {
	for _, occurrence := range ingredientPercentOccurrences(value) {
		if occurrence.raw == strings.TrimSpace(percent) {
			return true
		}
	}
	return false
}
