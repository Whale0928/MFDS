package normalization

import (
	"regexp"
	"strings"
)

var (
	editionKOPattern = regexp.MustCompile(`[^()\[\],]+에디션`)
	editionENPattern = regexp.MustCompile(`(?i)\b[^()\[\],]*\bEDITION\b`)
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
		if koEdition == "" || enEdition == "" {
			state.review(ReasonEditionLanguageMismatch, state.result.EditionName)
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
