package normalization

import (
	"regexp"
	"strconv"
)

var (
	ageKOPattern   = regexp.MustCompile(`(\d{1,3})\s*년(?:\s|$|[)\],-])`)
	ageENPattern   = regexp.MustCompile(`(?i)(\d{1,3})\s*(?:YO\b|YEARS?(?:\s+OLD)?\b)|\bAGED\s+(\d{1,3})\b`)
	vintagePattern = regexp.MustCompile(`\b((?:19|20)\d{2})\b`)
)

func parseAge(ko, en string, state *derivationState) {
	values := []int{}
	raw := ""
	for _, source := range []struct {
		value string
		ko    bool
	}{{ko, true}, {en, false}} {
		var match []string
		if source.ko {
			match = ageKOPattern.FindStringSubmatch(source.value)
		} else {
			match = ageENPattern.FindStringSubmatch(source.value)
		}
		if len(match) == 0 {
			continue
		}
		for _, part := range match[1:] {
			if part == "" {
				continue
			}
			if parsed, err := strconv.Atoi(part); err == nil {
				values = append(values, parsed)
				if raw == "" {
					raw = match[0]
				}
			}
			break
		}
	}
	if len(values) == 0 {
		return
	}
	for _, value := range values[1:] {
		if value != values[0] {
			state.review(ReasonAgeConflict, "age "+strconv.Itoa(values[0])+" / "+strconv.Itoa(value))
			return
		}
	}
	state.result.AgeRaw = raw
	state.result.AgeYears = intPointer(values[0])
	state.structured++
	state.add(ReasonAgeExtracted)
}
func parseVintage(ko, en string, state *derivationState) {
	value := volumePattern.ReplaceAllString(ko+" "+en, "$1")
	value = manufacturePattern.ReplaceAllString(value, "")
	value = lotPattern.ReplaceAllString(value, "")
	for _, match := range vintagePattern.FindAllStringSubmatch(value, -1) {
		if len(match) != 2 {
			continue
		}
		state.result.VintageRaw = match[1]
		state.review(ReasonVintageReviewRequired, match[1])
		return
	}
}
