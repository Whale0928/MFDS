package normalization

import (
	"math"
	"regexp"
	"strconv"
	"strings"
)

var (
	compositionPattern = regexp.MustCompile(`인삼|송이|향|과즙|농축|원액|함유`)
	strongABVPattern   = regexp.MustCompile(`(?i)(?:ALC\.?\s*|)(\d+(?:\.\d+)?)\s*%\s*(?:VOL\.?\b|$|\))`)
	koABVPattern       = regexp.MustCompile(`(?:주도\s*)?(\d+(?:\.\d+)?)\s*(?:%|도)\s*$|\((\d+(?:\.\d+)?)\s*%\)`)
	percentPattern     = regexp.MustCompile(`(\d+(?:\.\d+)?)\s*%`)
	proofPattern       = regexp.MustCompile(`(?i)(\d+(?:\.\d+)?)\s*(?:PROOF|프루프)`)
	strengthPattern    = regexp.MustCompile(`(?i)\b(?:(?:CASK|BARREL|FULL)\s+STRENGTH|OVERPROOF)\b`)
)

func parseABV(ko, en string, state *derivationState) {
	candidates := []float64{}
	for _, source := range []struct {
		value string
		ko    bool
	}{{ko, true}, {en, false}} {
		if source.value == "" {
			continue
		}
		if source.ko && compositionPattern.MatchString(source.value) {
			if percentPattern.MatchString(source.value) {
				state.add(ReasonABVCompositionContext)
			}
			continue
		}
		match := strongABVPattern.FindStringSubmatch(source.value)
		if source.ko {
			match = koABVPattern.FindStringSubmatch(source.value)
		}
		if len(match) > 0 {
			valueText := ""
			for _, part := range match[1:] {
				if part != "" {
					valueText = part
					break
				}
			}
			if value, err := strconv.ParseFloat(valueText, 64); err == nil {
				candidates = append(candidates, value)
				if state.result.ABVRaw == "" {
					state.result.ABVRaw = match[0]
				}
			}
			continue
		}
		if generic := percentPattern.FindString(source.value); generic != "" && !strings.Contains(strings.ToUpper(source.value), "PROOF") && !isIngredientPercent(source.value, generic) {
			state.review(ReasonABVAmbiguousPosition, generic)
		}
	}
	if len(candidates) == 0 {
		return
	}
	first := candidates[0]
	for _, candidate := range candidates[1:] {
		if math.Abs(candidate-first) > 0.0000001 {
			state.review(ReasonABVConflict, formatNumber(first)+"% / "+formatNumber(candidate)+"%")
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
func parseProofAndStrength(value string, state *derivationState) {
	if match := proofPattern.FindStringSubmatch(value); len(match) == 2 {
		if proof, err := strconv.ParseFloat(match[1], 64); err == nil {
			state.result.ProofRaw = match[0]
			state.result.ProofValue = floatPointer(proof)
			state.structured++
			state.add(ReasonProofPreserved)
			state.review(ReasonProofPreserved, match[0])
		}
	}
	upper := strings.ToUpper(value)
	for _, strength := range []string{"CASK STRENGTH", "BARREL STRENGTH", "OVERPROOF", "FULL STRENGTH"} {
		if strings.Contains(upper, strength) {
			state.result.StrengthType = strength
			state.structured++
			return
		}
	}
}
func isIngredientPercent(value, percent string) bool {
	upper := strings.ToUpper(value)
	return strings.Contains(upper, "100% RYE") || strings.Contains(upper, "100% ISLAY") || strings.Contains(upper, "100% POIRE") || strings.Contains(upper, percent+" RYE") || strings.Contains(upper, percent+" ISLAY") || strings.Contains(upper, percent+" POIRE")
}
