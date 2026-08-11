package matching

import (
	"strings"
	"unicode"

	"golang.org/x/text/unicode/norm"
)

func tokenize(value string) []string {
	value = strings.ToLower(norm.NFKC.String(value))
	var tokens []string
	var current []rune
	flush := func() {
		if len(current) == 0 {
			return
		}
		tokens = append(tokens, string(current))
		current = current[:0]
	}
	for _, r := range value {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			current = append(current, r)
			continue
		}
		flush()
	}
	flush()
	return tokens
}

func sequenceMatch(input, candidate []string) bool {
	if len(candidate) == 0 || len(input) < len(candidate) {
		return false
	}
	for start := 0; start <= len(input)-len(candidate); start++ {
		matched := true
		for offset := range candidate {
			if input[start+offset] != candidate[offset] {
				matched = false
				break
			}
		}
		if matched {
			return true
		}
	}
	return false
}

func fuzzySequenceMatch(input, candidate []string) bool {
	if len(candidate) == 0 || len(input) < len(candidate) {
		return false
	}
	for start := 0; start <= len(input)-len(candidate); start++ {
		matched := true
		for offset, candidateToken := range candidate {
			inputToken := input[start+offset]
			if len(candidateToken) < 5 || levenshteinDistance(inputToken, candidateToken) > fuzzyDistance(candidateToken) {
				matched = false
				break
			}
		}
		if matched {
			return true
		}
	}
	return false
}

func fuzzyDistance(value string) int {
	if len([]rune(value)) >= 9 {
		return 1
	}
	return 0
}

func levenshteinDistance(left, right string) int {
	a, b := []rune(left), []rune(right)
	if len(a) < len(b) {
		a, b = b, a
	}
	previous := make([]int, len(b)+1)
	for i := range previous {
		previous[i] = i
	}
	for i, leftRune := range a {
		current := make([]int, len(b)+1)
		current[0] = i + 1
		for j, rightRune := range b {
			cost := 0
			if leftRune != rightRune {
				cost = 1
			}
			current[j+1] = min(current[j]+1, previous[j+1]+1, previous[j]+cost)
		}
		previous = current
	}
	return previous[len(b)]
}

func inputNameTokens(input Input) [][]string {
	var values [][]string
	for _, value := range []string{
		input.BaseNameKO, input.BaseNameEN,
		input.SearchNameKO, input.SearchNameEN,
	} {
		if tokens := tokenize(value); len(tokens) > 0 {
			values = append(values, tokens)
		}
	}
	return values
}
