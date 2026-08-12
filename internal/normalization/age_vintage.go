package normalization

import (
	"regexp"
	"sort"
	"strconv"
	"strings"
)

var (
	ageKOPattern   = regexp.MustCompile(`(\d{1,3})\s*년(?:\s|$|[)\],-])`)
	ageENPattern   = regexp.MustCompile(`(?i)(\d{1,3})\s*(?:YO\b|YEARS?(?:\s+OLD)?\b)|\bAGED\s+(\d{1,3})\b`)
	vintagePattern = regexp.MustCompile(`\b((?:1[5-9]|20)\d{2})\b`)
)

const (
	minimumVintageYear = 1950
	maximumVintageYear = 2026
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
	// Section 5.4 requires LOT, manufacture number and unlabeled code sections to be separated before a vintage is searched.
	value := buildName(ko, baseNameMode, nil) + " " + buildName(en, baseNameMode, nil)
	years := map[int]string{}
	for _, match := range vintagePattern.FindAllStringSubmatch(value, -1) {
		if len(match) != 2 {
			continue
		}
		year, err := strconv.Atoi(match[1])
		if err != nil || year < minimumVintageYear || year > maximumVintageYear {
			continue
		}
		years[year] = match[1]
	}
	if len(years) == 0 {
		return
	}
	ordered := make([]int, 0, len(years))
	for year := range years {
		ordered = append(ordered, year)
	}
	sort.Ints(ordered)
	oldest := ordered[0]
	state.result.VintageRaw = years[oldest]
	state.result.VintageYear = intPointer(oldest)
	state.structured++
	fragments := make([]string, len(ordered))
	for index, year := range ordered {
		fragments[index] = strconv.Itoa(year)
	}
	state.review(ReasonVintageReviewRequired, strings.Join(fragments, " / "))
}
