package normalization

import (
	"math"
	"regexp"
	"strconv"
	"strings"
)

// maximumAutomaticIngredientPercent is the largest ingredient share accepted without review.
const maximumAutomaticIngredientPercent = 20

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
	// statement는 함유나 100% 원재료처럼 함량을 직접 서술한 표기다.
	statement bool
}

var (
	// 성분 판정은 함량을 서술하는 문맥으로만 한정한다. 몰트·그레인·인삼 같은 품목 단어가 근처에 있다는 이유만으로
	// 병 도수를 성분으로 보내면 카발란 싱글몰트 (54.8%) 같은 도수가 abv_percent에서 빠진다.
	ingredientPrefix       = regexp.MustCompile(`(?i)(?:인삼|송이|향료?|과즙|농축(?:액)?|원액|추출물|증류액|침출액|주스|시럽|꿀|설탕|함량|고형분|JUICE|EXTRACT|FLAVOU?R|CONCENTRATE|HONEY|SUGAR)\s*[:=]?\s*$`)
	ingredientSuffix       = regexp.MustCompile(`^\s*함유`)
	wholeIngredientPattern = regexp.MustCompile(`(?i)^\s*(?:호밀|보리|몰트|곡물|아가베|아일라|포아르|RYE|ISLAY|MALT|GRAIN|AGAVE|BARLEY|WHEAT|POIRE|PEAR|APPLE)`)
	percentPattern         = regexp.MustCompile(`(\d+(?:\.\d+)?)\s*%`)
	percentNumberPattern   = regexp.MustCompile(`\d+(?:\.\d+)?`)
	proofPattern           = regexp.MustCompile(`(?i)(\d+(?:\.\d+)?)\s*(?:PROOF|프루프)`)

	strongABVPattern = regexp.MustCompile(`(?i)(?:주도\s*|ALC\.?\s*|ABV\s*[:.]?\s*)\d+(?:\.\d+)?\s*(?:%|도)|\(\s*\d+(?:\.\d+)?\s*%\s*(?:,\s*\d+(?:\.\d+)?\s*(?:ML|L))?\s*\)|(?:^|[\s(])\d+(?:\.\d+)?\s*%\s*(?:VOL\.?\b|,?\s*\d+(?:\.\d+)?\s*(?:ML|L)\b|$)|(?:^|[\s(\[])\d+(?:\.\d+)?\s*도(?:[\s()\[\]]|$)`)
	koABVPattern     = regexp.MustCompile(`(?:주도\s*)\d+(?:\.\d+)?\s*(?:%|도)|(?:^|[\s(\[])\d+(?:\.\d+)?\s*도(?:[\s()\[\]]|$)`)

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
		// 한글 도 표기는 뒤가 공백·괄호·끝일 때만 도수로 본다. 56도 프리미엄금문고량주(750ML), 북경이과두주(56도)가 이 경우다.
		{regexp.MustCompile(`(?:^|[\s(\[])(\d+(?:\.\d+)?)\s*도(?:[\s()\[\]]|$)`), 1, false},
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
		value := seen[order[0]].value
		state.result.IngredientPercent = floatPointer(value)
		// 20%를 넘는 성분 함량은 병 도수를 잘못 분류했을 가능성이 커서 자동 확정하지 않는다.
		// 함유나 100% 원재료처럼 함량을 직접 서술한 표기는 병 도수로 읽힐 여지가 없어 예외로 둔다.
		if value > maximumAutomaticIngredientPercent && !seen[order[0]].statement {
			state.review(ReasonIngredientPercentHigh, seen[order[0]].raw)
		}
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
		result = append(result, percentOccurrence{
			raw: strings.TrimSpace(value[indexes[0]:indexes[1]]), value: parsed, start: indexes[0], end: indexes[1],
			statement: isIngredientStatementAt(value, indexes[0], indexes[1]),
		})
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

// isIngredientPercentAt accepts only a content statement: an ingredient noun right before the percent, 함유 right after
// it, or 100% naming the whole raw material such as 100% 호밀. A grain or fruit word elsewhere in the name does not decide it.
func isIngredientPercentAt(value string, start, end int) bool {
	return isExplicitIngredientPercentAt(value, start) || isIngredientStatementAt(value, start, end)
}

// isIngredientStatementAt reports 함유 right after the percent or 100% naming the whole raw material.
func isIngredientStatementAt(value string, start, end int) bool {
	// 앵커 패턴은 숫자 부분만 넘기므로 뒤따르는 % 기호를 건너뛴 위치에서 함유 서술을 확인한다.
	rest := strings.TrimPrefix(strings.TrimLeft(value[end:], " "), "%")
	if ingredientSuffix.MatchString(rest) {
		return true
	}
	parsed, err := strconv.ParseFloat(percentNumberPattern.FindString(value[start:end]), 64)
	return err == nil && parsed == 100 && wholeIngredientPattern.MatchString(rest)
}

func parseABV(ko, en string, state *derivationState) {
	candidates := []percentOccurrence{}
	ingredientValues := map[string]struct{}{}
	for _, source := range []string{ko, en} {
		for _, occurrence := range ingredientPercentOccurrences(source) {
			ingredientValues[formatNumber(occurrence.value)] = struct{}{}
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
