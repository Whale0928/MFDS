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
	slashMaterial      = regexp.MustCompile(`(?i)/\s*([A-Z0-9]*GX[A-Z0-9]+|\d{6,7})\b`)
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
	} else if match := slashMaterial.FindStringSubmatch(value); len(match) == 2 {
		state.result.MaterialCode = match[1]
		state.structured++
		state.add(ReasonMaterialCodePreservedForSKU)
	}
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
