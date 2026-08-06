package normalization

import (
	"math"
	"regexp"
	"strconv"
	"strings"
)

var (
	volumePattern = regexp.MustCompile(`(?i)(^|[^A-Za-z0-9])((?:\d{1,3}(?:,\d{3})+|\d+(?:\.\d+)?)\s*(?:ml|l))\b`)
	// packagePattern reads the count that directly follows a matched volume token, so its multiplier is already in volume context.
	packagePattern = regexp.MustCompile(`^\s*[*x×X]\s*(\d+)`)
	// packageTokenPattern requires the volume + multiplier + count context. Case folding is off so an X inside a code is never consumed.
	packageTokenPattern   = regexp.MustCompile(`((?:\d{1,3}(?:,\d{3})+|\d+(?:\.\d+)?)\s*(?:[mM][lL]|[lL]))\s*[*x×X]\s*\d+\s*(?:BTLS?|BOTTLES?|EA|PCS|병)?`)
	packageDisplayPattern = regexp.MustCompile(`(?i)(\d+ml)\s*[*x×]\s*\d+`)
)

type volumeMatch struct {
	raw, display string
	ml           int
	packageCount *int
}

func parseVolume(ko, en string, state *derivationState) *volumeMatch {
	matches := []volumeMatch{}
	for _, source := range []string{ko, en} {
		for _, matched := range volumePattern.FindAllStringSubmatchIndex(source, -1) {
			raw := source[matched[4]:matched[5]]
			value, ok := volumeML(raw)
			if !ok {
				continue
			}
			candidate := volumeMatch{raw: raw, ml: value, display: strconv.Itoa(value) + "ml"}
			tail := source[matched[1]:]
			if packageMatch := packagePattern.FindStringSubmatch(tail); len(packageMatch) == 2 {
				if count, err := strconv.Atoi(packageMatch[1]); err == nil && count > 0 {
					candidate.packageCount = intPointer(count)
				}
			}
			matches = append(matches, candidate)
		}
	}
	if len(matches) == 0 {
		return nil
	}
	first := matches[0]
	for _, candidate := range matches[1:] {
		if candidate.ml != first.ml {
			state.review(ReasonVolumeAmbiguous, first.raw+" / "+candidate.raw)
			return nil
		}
	}
	state.result.VolumeRaw = first.raw
	state.result.VolumeML = intPointer(first.ml)
	state.result.UnitVolumeML = intPointer(first.ml)
	state.structured++
	if strings.Contains(first.raw, ",") {
		state.add(ReasonVolumeThousandsSeparator)
	}
	if strings.ContainsAny(first.raw, "ML") || strings.Contains(first.raw, "mL") || strings.HasSuffix(first.raw, "L") {
		state.add(ReasonVolumeUnitCaseNormalized)
	}
	if first.packageCount != nil {
		state.result.PackageCount = first.packageCount
		state.add(ReasonPackageCountExcludedFromBottleSKU)
	}
	return &first
}
func volumeML(raw string) (int, bool) {
	tokens := strings.Fields(strings.ToLower(raw))
	if len(tokens) == 0 {
		return 0, false
	}
	valueToken := strings.TrimSpace(strings.TrimSuffix(strings.TrimSuffix(tokens[0], "ml"), "l"))
	if len(tokens) == 1 {
		valueToken = strings.TrimSuffix(strings.TrimSuffix(strings.ToLower(raw), "ml"), "l")
	}
	value, err := strconv.ParseFloat(strings.ReplaceAll(strings.TrimSpace(valueToken), ",", ""), 64)
	if err != nil || value <= 0 {
		return 0, false
	}
	if strings.HasSuffix(strings.ToLower(strings.TrimSpace(raw)), "l") && !strings.HasSuffix(strings.ToLower(strings.TrimSpace(raw)), "ml") {
		value *= 1000
	}
	if math.Abs(value-math.Round(value)) > 0.0000001 || value > math.MaxInt {
		return 0, false
	}
	return int(math.Round(value)), true
}
