package normalization

import (
	"regexp"
	"strings"
)

var (
	caskCandidatePatterns = []*regexp.Regexp{
		regexp.MustCompile(`(?i)\bPEDRO\s+XIM[EÉ]NEZ(?:\s+SHERRY)?\s+CASKS?\b`),
		regexp.MustCompile(`(?i)\bOLOROSO(?:\s+SHERRY)?\s+CASKS?\b`),
		regexp.MustCompile(`(?i)\bPX(?:\s+SHERRY)?\s+CASKS?\b`),
		regexp.MustCompile(`(?i)\bSHERRY\s+CASKS?\b`),
		regexp.MustCompile(`(?i)\bBOURBON\s+CASKS?\b`),
		regexp.MustCompile(`(?i)\bPORT(?:\s+WINE)?\s+CASKS?\b`),
		regexp.MustCompile(`(?i)\bWINE\s+CASKS?\b`),
		regexp.MustCompile(`(?i)\bVIRGIN\s+OAK(?:\s+CASKS?)?\b`),
		regexp.MustCompile(`올로로소\s*(?:쉐리|셰리)?\s*캐스크`),
		regexp.MustCompile(`(?:PX|피엑스)\s*(?:쉐리|셰리)?\s*캐스크`),
		regexp.MustCompile(`(?:쉐리|셰리)\s*캐스크`),
		regexp.MustCompile(`버번\s*캐스크`),
		regexp.MustCompile(`포트(?:\s*와인)?\s*캐스크`),
		regexp.MustCompile(`와인\s*캐스크`),
		regexp.MustCompile(`버진\s*오크`),
	}
)

func deriveAlcoholCandidate(input Input, state *derivationState) {
	result := state.result
	result.AlcoholNameKO = alcoholName(input.ProductNameKO)
	result.AlcoholNameEN = alcoholName(input.ProductNameEN)
	result.AlcoholCategoryKO, result.AlcoholCategoryEN = alcoholCategory(input.ItemName)
	if input.ItemName != "" && result.AlcoholCategoryKO == "" {
		state.review(ReasonAlcoholCategoryNotMapped, input.ItemName)
	}

	manufacture, ok := lookupCountry(input.ManufactureCountryName)
	if ok {
		result.ManufactureCountry = manufacture
		result.AlcoholRegionKO = manufacture.NameKO
		result.AlcoholRegionEN = manufacture.NameEN
	} else if strings.TrimSpace(input.ManufactureCountryName) != "" {
		state.review(ReasonManufactureCountryNotMapped, input.ManufactureCountryName)
	}
	export, ok := lookupCountry(input.ExportCountryName)
	if ok {
		result.ExportCountry = export
	} else if strings.TrimSpace(input.ExportCountryName) != "" {
		state.review(ReasonExportCountryNotMapped, input.ExportCountryName)
	}

	if result.ABVPercent != nil {
		result.AlcoholABV = formatNumber(*result.ABVPercent) + "%"
	}
	result.CaskCandidate = findCaskCandidate(input.ProductNameKO, input.ProductNameEN)
	if result.CaskCandidate != "" {
		state.add(ReasonCaskTypeCandidateExtracted)
	}
	establishment := strings.TrimSpace(input.OverseasEstablishmentName)
	if establishment != "" {
		if containsHangul(establishment) {
			result.DistilleryNameKOCandidate = establishment
		} else {
			result.DistilleryNameENCandidate = establishment
		}
		state.add(ReasonOverseasEstablishmentDistilleryCandidate)
	}
}

func alcoholCategory(itemName string) (string, string) {
	switch strings.TrimSpace(itemName) {
	case "위스키":
		return "위스키", "Whisky"
	case "브랜디":
		return "브랜디", "Brandy"
	case "리큐르":
		return "리큐르", "Liqueur"
	case "일반증류주":
		return "일반증류주", "General Distilled Spirits"
	default:
		return "", ""
	}
}

// alcoholName keeps the BottleNote-facing name, so it shares baseName's segment engine but preserves age, cask and batch evidence.
func alcoholName(value string) string { return buildName(value, alcoholNameMode) }

func findCaskCandidate(values ...string) string {
	combined := strings.Join(values, " ")
	for _, pattern := range caskCandidatePatterns {
		if match := pattern.FindString(combined); match != "" {
			return strings.Join(strings.Fields(match), " ")
		}
	}
	return ""
}

func containsHangul(value string) bool {
	for _, char := range value {
		if char >= '가' && char <= '힣' {
			return true
		}
	}
	return false
}
