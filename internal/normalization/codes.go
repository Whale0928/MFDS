package normalization

import (
	"regexp"
	"strings"
)

var (
	lotPattern         = regexp.MustCompile(`(?i)(?:LOT\s*NO\.?|LOTE)\s*[:.]?\s*([A-Z0-9][A-Z0-9\s-]*)`)
	manufacturePattern = regexp.MustCompile(`제조번호\s*[:.]?\s*([A-Z0-9][A-Z0-9\s-]*)`)
	lotSuffixPattern   = regexp.MustCompile(`(?i)(?:^|[\s)])(L[0-9][A-Z0-9]{4,})\s*$`)
	parenMaterial      = regexp.MustCompile(`\((\d{6})\)`)
	// A code behind a slash identifies the importer material or the single cask, so it is kept whole as an SKU axis.
	slashMaterial = regexp.MustCompile(`(?i)/\s*([A-Z0-9]{5,})\b`)
	caskPattern        = regexp.MustCompile(`#\s*(\d+)\b`)
	batchENPattern     = regexp.MustCompile(`(?i)\bBATCH\s*#?\s*([A-Z0-9-]+)\b`)
	batchKOPattern     = regexp.MustCompile(`배치\s*#?\s*([A-Z0-9-]+)\b`)
)

func parseCodes(ko, en string, state *derivationState) {
	value := strings.TrimSpace(ko + " " + en)
	parseLabeledCode(ko, state)
	if state.result.LotNumber == "" && state.result.ManufactureNumber == "" {
		parseLabeledCode(en, state)
	}
	if match := parenMaterial.FindStringSubmatch(value); len(match) == 2 {
		state.result.MaterialCode = match[1]
		state.structured++
		state.add(ReasonMaterialCodePreservedForSKU)
	} else if match := slashMaterial.FindStringSubmatch(value); len(match) == 2 && strings.ContainsAny(match[1], "0123456789") {
		state.result.MaterialCode = match[1]
		state.structured++
		state.add(ReasonMaterialCodePreservedForSKU)
	}
	parseUnlabeledLot(ko, en, state)
	if match := caskPattern.FindStringSubmatch(value); len(match) == 2 {
		if strings.Contains(strings.ToUpper(value), "SINGLE CASK") || strings.Contains(value, "싱글캐스크") || strings.Contains(value, "싱글 캐스크") {
			state.result.CaskNumber = match[1]
			state.structured++
			state.add(ReasonCaskNumberPreservedForSKU)
		} else {
			state.review(ReasonHashNumberAmbiguous, match[0])
		}
	}
	parseBatch(ko, en, state)
}

func parseLabeledCode(source string, state *derivationState) {
	if match := manufacturePattern.FindStringSubmatch(source); len(match) == 2 {
		state.result.ManufactureNumber = cleanCode(match[1])
		state.structured++
		state.add(ReasonManufactureNumberLabeled)
		return
	}
	if match := lotPattern.FindStringSubmatch(source); len(match) == 2 {
		state.result.LotNumber = cleanCode(match[1])
		state.structured++
		state.add(ReasonLotLabeled)
		return
	}
	if match := lotSuffixPattern.FindStringSubmatch(source); len(match) == 2 {
		state.result.LotNumber = match[1]
		state.structured++
		state.add(ReasonLotSuffixCodeExcludedFromSKU)
	}
}
// parseUnlabeledLot separates an unlabeled L code that sits in a bracket or behind a separator. Section 5.5 keeps such a
// code out of the SKU key, and it must never be split, so the whole token is taken or nothing is.
func parseUnlabeledLot(ko, en string, state *derivationState) {
	if state.result.LotNumber != "" || state.result.ManufactureNumber != "" {
		return
	}
	for _, source := range []string{ko, en} {
		for _, token := range nameTokens(source) {
			if token == state.result.MaterialCode || !isCodeToken(token) || !strings.HasPrefix(token, "L") {
				continue
			}
			state.result.LotNumber = token
			state.structured++
			state.add(ReasonLotUnlabeledCode)
			return
		}
	}
}
func parseBatch(ko, en string, state *derivationState) {
	koMatch := batchKOPattern.FindStringSubmatch(ko)
	enMatch := batchENPattern.FindStringSubmatch(en)
	if len(koMatch) == 0 && len(enMatch) == 0 {
		return
	}
	if len(koMatch) == 2 && len(enMatch) == 2 && koMatch[1] == enMatch[1] {
		state.result.BatchNumber = koMatch[1]
		state.structured++
		return
	}
	if len(koMatch) == 2 {
		state.result.BatchNumber = koMatch[1]
	} else {
		state.result.BatchNumber = enMatch[1]
	}
	state.review(ReasonBatchLanguageMismatch, state.result.BatchNumber)
}
