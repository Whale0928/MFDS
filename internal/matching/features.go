package matching

import (
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"
)

var (
	ageWithUnitPattern = regexp.MustCompile(`(?i)(?:\baged\s*)?([1-9][0-9]?)\s*(?:yo|y\.o\.?|y(?:ears?)?(?:\s*old)?)\b|([1-9][0-9]?)\s*(?:년(?:산|\s*숙성)?|年)`)
	abvPattern         = regexp.MustCompile(`(?i)\b[0-9]{1,3}(?:\.[0-9]+)?\s*(?:%|abv)\b`)
	volumePattern      = regexp.MustCompile(`(?i)\b([0-9]{1,4}(?:\.[0-9]+)?)\s*(ml|cl|l)\b`)
	plainAgePattern    = regexp.MustCompile(`^[1-9][0-9]?$`)
)

type nameFeatures struct {
	tokens  []string
	baseKey string
	age     *int
}

func extractNameFeatures(value string) nameFeatures {
	var age *int
	withoutAge := ageWithUnitPattern.ReplaceAllStringFunc(value, func(match string) string {
		if age == nil {
			age = parseAgeValue(match)
		}
		return " "
	})
	withoutAttributes := abvPattern.ReplaceAllString(withoutAge, " ")
	withoutAttributes = volumePattern.ReplaceAllString(withoutAttributes, " ")
	tokens := tokenize(withoutAttributes)
	return nameFeatures{tokens: tokens, baseKey: strings.Join(tokens, " "), age: age}
}

func parseStructuredAge(value string) *int {
	trimmed := strings.TrimSpace(value)
	if plainAgePattern.MatchString(trimmed) {
		parsed, _ := strconv.Atoi(trimmed)
		return &parsed
	}
	return parseAgeValue(trimmed)
}

func parseAgeValue(value string) *int {
	match := ageWithUnitPattern.FindStringSubmatch(value)
	if len(match) != 3 {
		return nil
	}
	age := match[1]
	if age == "" {
		age = match[2]
	}
	parsed, err := strconv.Atoi(age)
	if err != nil || parsed <= 0 || parsed > 99 {
		return nil
	}
	return &parsed
}

func parseVolumeML(value string) *int {
	match := volumePattern.FindStringSubmatch(strings.TrimSpace(value))
	if len(match) != 3 {
		return nil
	}
	amount, err := strconv.ParseFloat(match[1], 64)
	if err != nil || amount <= 0 {
		return nil
	}
	switch strings.ToLower(match[2]) {
	case "l":
		amount *= 1000
	case "cl":
		amount *= 10
	}
	result := int(amount + 0.5)
	return &result
}

func bestInputAge(input Input) *int {
	if input.AgeYears != nil {
		value := *input.AgeYears
		return &value
	}
	if value := parseStructuredAge(input.Age); value != nil {
		return value
	}
	for _, name := range []string{input.BaseNameKO, input.BaseNameEN, input.SearchNameKO, input.SearchNameEN} {
		if value := extractNameFeatures(name).age; value != nil {
			return value
		}
	}
	return nil
}

func referenceAge(reference AlcoholReference) (*int, []string) {
	var structured *int
	if reference.AgeYears != nil {
		value := *reference.AgeYears
		structured = &value
	} else {
		structured = parseStructuredAge(reference.Age)
	}
	var fromName *int
	for _, name := range []string{reference.KorName, reference.EngName} {
		value := extractNameFeatures(name).age
		if value == nil {
			continue
		}
		if fromName != nil && *fromName != *value {
			return structured, []string{"reference_age_name_conflict"}
		}
		fromName = value
	}
	if structured != nil && fromName != nil && *structured != *fromName {
		return structured, []string{"reference_age_attribute_conflict"}
	}
	if structured != nil {
		return structured, nil
	}
	return fromName, nil
}

func bidirectionalSequenceMatch(left, right []string) bool {
	return sequenceMatch(left, right) || sequenceMatch(right, left)
}

func tokenJaccard(left, right []string) float64 {
	if len(left) == 0 || len(right) == 0 {
		return 0
	}
	leftSet := make(map[string]struct{}, len(left))
	for _, token := range left {
		leftSet[token] = struct{}{}
	}
	intersection := 0
	union := len(leftSet)
	seenRight := make(map[string]struct{}, len(right))
	for _, token := range right {
		if _, seen := seenRight[token]; seen {
			continue
		}
		seenRight[token] = struct{}{}
		if _, exists := leftSet[token]; exists {
			intersection++
		} else {
			union++
		}
	}
	return float64(intersection) / float64(union)
}

func significantToken(token string) bool {
	if utf8.RuneCountInString(token) < 2 {
		return false
	}
	switch token {
	case "whisky", "whiskey", "scotch", "single", "malt", "blend", "blended", "위스키", "싱글몰트":
		return false
	default:
		return true
	}
}
