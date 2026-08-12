package normalization

import (
	"regexp"
	"strconv"
	"strings"
)

var (
	lotPattern         = regexp.MustCompile(`(?i)(?:LOT\s*NO\.?|LOTE)\s*[:.]?\s*([A-Z0-9][A-Z0-9\s-]*)`)
	manufacturePattern = regexp.MustCompile(`(?i)제조번호\s*(?:NO\.?\s*)?[:.]?\s*([A-Z0-9][A-Z0-9\s-]*)`)
	lotSuffixPattern   = regexp.MustCompile(`(?i)(?:^|[\s)])(L[0-9][A-Z0-9]{4,})\s*$`)
	parenMaterial      = regexp.MustCompile(`\((\d{6})\)`)
	// A code behind a slash identifies the importer material or the single cask, so it is kept whole as an SKU axis.
	slashMaterial         = regexp.MustCompile(`(?i)/\s*([A-Z0-9]{5,})\b`)
	caskPattern           = regexp.MustCompile(`#\s*(\d+)\b`)
	batchENPattern        = regexp.MustCompile(`(?i)\bBATCH\s*(?:(?:#|NO\.?)\s*)?(\d+)\b`)
	batchKOPattern        = regexp.MustCompile(`배치\s*(?:(?:#|No\.?)\s*)?(\d+)\b`)
	variantMarkerPattern  = regexp.MustCompile(`(?i)(?:#|@|NO\.)\s*(\d+)`)
	seriesNumberPattern   = regexp.MustCompile(`(?i)(?:\bSERIES|시리즈)\s+(\d+)\b`)
	labeledMarkerPrefix   = regexp.MustCompile(`(?i)(?:LOT|제조번호)\s*$`)
	batchContextPattern   = regexp.MustCompile(`(?i)(?:\bBATCH|배치)[^#@]{0,48}$`)
	smallBatchLinePattern = regexp.MustCompile(`(?i)\bSMALL\s+BATCH\b|스몰\s*배치`)
	editionContextPattern = regexp.MustCompile(
		`(?i)(?:\bEDITION|에디션)[^#@]{0,48}$`,
	)
	caskContextPattern = regexp.MustCompile(
		`(?i)(?:\b(?:SINGLE\s+)?CASK|\bBARREL|싱글\s*캐스크|캐스크|배럴|바렐)[^#@]{0,48}$`,
	)
	seriesContextPattern = regexp.MustCompile(`(?i)(?:\bSERIES|시리즈)[^#@]{0,48}$`)
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
	parseVariantMarker(ko, en, state)
	parseBatch(ko, en, state)
}

type variantMarker struct {
	raw, value, markerType string
}

func parseVariantMarker(ko, en string, state *derivationState) {
	markers := []variantMarker{}
	for _, source := range []string{ko, en} {
		for _, indexes := range seriesNumberPattern.FindAllStringSubmatchIndex(source, -1) {
			if len(indexes) < 4 {
				continue
			}
			markers = append(markers, variantMarker{
				raw:        strings.TrimSpace(source[indexes[0]:indexes[1]]),
				value:      source[indexes[2]:indexes[3]],
				markerType: VariantMarkerTypeSeriesNumber,
			})
		}
		for _, indexes := range variantMarkerPattern.FindAllStringSubmatchIndex(source, -1) {
			if len(indexes) < 4 || labeledMarkerPrefix.MatchString(source[:indexes[0]]) {
				continue
			}
			raw := strings.TrimSpace(source[indexes[0]:indexes[1]])
			value := source[indexes[2]:indexes[3]]
			markers = append(markers, variantMarker{raw: raw, value: value, markerType: classifyVariantMarker(source, indexes[0], raw)})
		}
	}
	if len(markers) == 0 {
		return
	}
	selected := markers[0]
	setVariantMarker(state, selected)

	canonical := canonicalNumeric(selected.value)
	switch selected.markerType {
	case VariantMarkerTypeCaskNumber:
		state.result.CaskNumber = canonical
		state.add(ReasonCaskNumberPreservedForSKU)
	case VariantMarkerTypeBatchNumber:
		state.result.BatchNumber = canonical
	case VariantMarkerTypeEditionNumber:
		// parseEditionAndVersion preserves the bounded source phrase in edition_name.
	case VariantMarkerTypeSeriesNumber:
	case VariantMarkerTypeUnknown:
		state.add(ReasonHashNumberAmbiguous)
		state.review(ReasonVariantMarkerAmbiguous, selected.raw)
	}
	for _, marker := range markers[1:] {
		if canonicalNumeric(marker.value) != canonical || marker.markerType != selected.markerType {
			state.review(ReasonVariantMarkerAmbiguous, selected.raw+" / "+marker.raw)
			break
		}
	}
}

func setVariantMarker(state *derivationState, marker variantMarker) bool {
	if state.result.VariantMarkerRaw != "" {
		return false
	}
	state.result.VariantMarkerRaw = marker.raw
	state.result.VariantMarkerType = marker.markerType
	state.result.VariantMarkerValue = marker.value
	state.structured++
	return true
}

func classifyVariantMarker(source string, markerStart int, raw string) string {
	prefix := source[:markerStart]
	switch {
	case editionContextPattern.MatchString(prefix):
		return VariantMarkerTypeEditionNumber
	case strings.HasPrefix(strings.TrimSpace(raw), "@") || seriesContextPattern.MatchString(prefix):
		return VariantMarkerTypeSeriesNumber
	case batchContextPattern.MatchString(smallBatchLinePattern.ReplaceAllString(prefix, "")):
		return VariantMarkerTypeBatchNumber
	case caskContextPattern.MatchString(prefix):
		return VariantMarkerTypeCaskNumber
	default:
		return VariantMarkerTypeUnknown
	}
}

func canonicalNumeric(value string) string {
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return value
	}
	return strconv.Itoa(parsed)
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
	if len(koMatch) == 2 && len(enMatch) == 2 && canonicalNumeric(koMatch[1]) == canonicalNumeric(enMatch[1]) {
		state.result.BatchNumber = canonicalNumeric(koMatch[1])
		state.structured++
		return
	}
	if len(koMatch) == 2 {
		state.result.BatchNumber = canonicalNumeric(koMatch[1])
	} else {
		state.result.BatchNumber = canonicalNumeric(enMatch[1])
	}
	state.structured++
	if len(koMatch) == 2 && len(enMatch) == 2 {
		state.review(ReasonBatchLanguageMismatch, canonicalNumeric(koMatch[1])+" / "+canonicalNumeric(enMatch[1]))
	}
}
