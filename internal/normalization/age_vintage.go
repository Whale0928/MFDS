package normalization

import (
	"regexp"
	"sort"
	"strconv"
	"strings"
)

var (
	ageKOPattern = regexp.MustCompile(`(\d{1,3})\s*년(?:\s|$|[)\],-])`)
	// AGED n 뒤의 YEARS까지 한 표기로 소비해야 베이스명에 YEARS가 남지 않는다.
	ageENPattern = regexp.MustCompile(`(?i)(\d{1,3})\s*(?:YO\b|YEARS?(?:\s+OLD)?\b)|\bAGED\s+(\d{1,3})(?:\s*YEARS?(?:\s+OLD)?)?\b`)
	// 기념 연수는 숙성연수가 아니다. 헤네시 260주년의 HENNESSY VS 260YEARS처럼 다른 언어에만 YEARS로 붙기도 한다.
	anniversaryKOPattern = regexp.MustCompile(`(\d{1,3})\s*주년`)
	anniversaryENPattern = regexp.MustCompile(`(?i)(\d{1,3})\s*(?:ST|ND|RD|TH)?\s*(?:YEARS?\s+)?ANNIVERSARY`)
	vintagePattern       = regexp.MustCompile(`\b((?:1[5-9]|20)\d{2})\b`)
)

const (
	minimumVintageYear = 1950
	maximumVintageYear = 2026
)

func parseAge(ko, en string, state *derivationState) {
	anniversaries := anniversaryNumbers(ko, en)
	values := []int{}
	raw := ""
	for _, source := range []struct {
		value string
		ko    bool
	}{{ko, true}, {en, false}} {
		pattern := ageENPattern
		if source.ko {
			pattern = ageKOPattern
		}
		for _, match := range pattern.FindAllStringSubmatch(source.value, -1) {
			parsed, ok := ageMatchValue(match)
			if !ok {
				continue
			}
			if _, anniversary := anniversaries[parsed]; anniversary {
				continue
			}
			values = append(values, parsed)
			if raw == "" {
				raw = match[0]
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

func ageMatchValue(match []string) (int, bool) {
	for _, part := range match[1:] {
		if part == "" {
			continue
		}
		parsed, err := strconv.Atoi(part)
		return parsed, err == nil
	}
	return 0, false
}

// anniversaryNumbers collects numbers written as 주년 or ANNIVERSARY in either language of the same name.
func anniversaryNumbers(ko, en string) map[int]struct{} {
	numbers := map[int]struct{}{}
	for _, found := range [][][]string{
		anniversaryKOPattern.FindAllStringSubmatch(ko, -1),
		anniversaryENPattern.FindAllStringSubmatch(en, -1),
	} {
		for _, match := range found {
			if parsed, err := strconv.Atoi(match[1]); err == nil {
				numbers[parsed] = struct{}{}
			}
		}
	}
	return numbers
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
