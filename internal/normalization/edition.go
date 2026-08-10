package normalization

import (
	"regexp"
	"strings"
)

var (
	editionKOPattern       = regexp.MustCompile(`(?:(?:서울|코리아|한국|리미티드|한정|특별|기념|면세|파이널)\s*)?에디션(?:\s*(?:(?:#|No\.?)\s*)?\d+(?:\s|$))?`)
	editionENPattern       = regexp.MustCompile(`(?i)(?:(?:LIMITED|SPECIAL|COLLECTOR'?S|ANNIVERSARY|FINAL|TRAVEL|EXCLUSIVE|SEOUL|KOREA|\d{4})\s+)?EDITION(?:\s*(?:(?:#|NO\.?)\s*)?\d+(?:\s|$))?`)
	editionNumberKOPattern = regexp.MustCompile(`에디션\s*(?:(?:#|No\.?)\s*)?(\d+)`)
	editionNumberENPattern = regexp.MustCompile(`(?i)\bEDITION\s*(?:(?:#|NO\.?)\s*)?(\d+)`)
)

func parseEditionAndVersion(ko, en string, state *derivationState) {
	koEdition := editionKOPattern.FindString(strings.TrimSpace(ko))
	enEdition := editionENPattern.FindString(strings.TrimSpace(en))
	if koEdition != "" || enEdition != "" {
		if koEdition != "" {
			state.result.EditionName = strings.TrimSpace(koEdition)
		} else {
			state.result.EditionName = strings.TrimSpace(enEdition)
		}
		state.structured++
		koValue, enValue := editionComparableValue(koEdition), editionComparableValue(enEdition)
		if koValue != "" && enValue != "" && koValue != enValue {
			state.review(ReasonEditionLanguageMismatch, koValue+" / "+enValue)
		}
	}
	if strings.Contains(ko, "구형") {
		state.result.VersionMarker = "구형"
		state.structured++
		state.add(ReasonKOVersionMarker)
	}
	if strings.Contains(ko, "(") && strings.Contains(ko, ")") {
		state.add(ReasonParenthesisSemanticText)
	}
}

func editionComparableValue(value string) string {
	if value == "" {
		return ""
	}
	for _, pattern := range []*regexp.Regexp{editionNumberKOPattern, editionNumberENPattern} {
		if match := pattern.FindStringSubmatch(value); len(match) == 2 {
			return canonicalNumeric(match[1])
		}
	}
	return ""
}
