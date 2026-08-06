package normalization

import "strings"

// Normalize derives only values with explicit source evidence. It never mutates Input values.
func Normalize(input Input) Result {
	result := Result{SKUDisplayNameKO: input.ProductNameKO, SKUDisplayNameEN: input.ProductNameEN, ExpiryRaw: input.ExpiryText, Reasons: []Reason{}, UnparsedFragments: []string{}}
	state := derivationState{result: &result}
	volume := parseVolume(input.ProductNameKO, input.ProductNameEN, &state)
	parseABV(input.ProductNameKO, input.ProductNameEN, &state)
	parseProofAndStrength(input.ProductNameKO+" "+input.ProductNameEN, &state)
	parseAge(input.ProductNameKO, input.ProductNameEN, &state)
	parseCodes(input.ProductNameKO, input.ProductNameEN, &state)
	parseEditionAndVersion(input.ProductNameKO, input.ProductNameEN, &state)
	parseVintage(input.ProductNameKO, input.ProductNameEN, &state)
	parseExpiry(input.ExpiryText, &state)
	parseCompany(input.ImporterName, input.OverseasEstablishmentName, &state)
	result.SKUDisplayNameKO = displayName(input.ProductNameKO, volume, result.ABVPercent)
	result.SKUDisplayNameEN = displayName(input.ProductNameEN, volume, result.ABVPercent)
	result.BaseProductNameKO = baseName(result.SKUDisplayNameKO)
	result.BaseProductNameEN = baseName(result.SKUDisplayNameEN)
	result.NameSearchKeyKO = searchKey(result.BaseProductNameKO)
	result.NameSearchKeyEN = searchKey(result.BaseProductNameEN)
	if strings.TrimSpace(input.ProductNameEN) != "" {
		result.NameSearchKeyKO = canonicalizeKoreanSearchKey(result.NameSearchKeyKO)
	}
	deriveAlcoholCandidate(input, &state)
	if isGenericProductName(result) {
		state.review(ReasonGenericProductName, result.BaseProductNameKO+" "+result.BaseProductNameEN)
	} else if hasSourceName(input) && result.UnitVolumeML == nil {
		// 80% of the ledger carries no volume token. A missing key component blocks the candidate key but is not a review signal.
		state.add(ReasonVolumeMissing)
	} else if hasSourceName(input) {
		result.SKUCandidateKeySHA256 = candidateHash(result)
	}
	hasName := strings.TrimSpace(input.ProductNameKO) != "" || strings.TrimSpace(input.ProductNameEN) != ""
	if state.reviewRequired {
		result.Status = StatusReviewRequired
	} else if !hasName && state.structured == 0 {
		result.Status = StatusUnparsed
	} else if len(result.UnparsedFragments) > 0 {
		result.Status = StatusPartial
	} else {
		result.Status = StatusNormalized
	}
	return result
}

func hasSourceName(input Input) bool {
	return strings.TrimSpace(input.ProductNameKO) != "" || strings.TrimSpace(input.ProductNameEN) != ""
}
