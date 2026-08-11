package matching

import "testing"

func TestParseAgeValue_지원표기_동일한숙성연수를반환한다(t *testing.T) {
	for _, input := range []string{"10y", "10yo", "10 year", "10 years old", "aged 10 years", "10년", "10년산", "10 年"} {
		t.Run(input, func(t *testing.T) {
			got := parseAgeValue(input)
			if got == nil || *got != 10 {
				t.Fatalf("parseAgeValue(%q) = %v, want 10", input, got)
			}
		})
	}
}

func TestParseAgeValue_용량과빈티지_숙성연수로해석하지않는다(t *testing.T) {
	for _, input := range []string{"1920ml", "2008", "L 0134739 2104"} {
		t.Run(input, func(t *testing.T) {
			if got := parseAgeValue(input); got != nil {
				t.Fatalf("parseAgeValue(%q) = %d, want nil", input, *got)
			}
		})
	}
}
