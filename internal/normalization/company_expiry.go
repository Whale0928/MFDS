package normalization

import (
	"regexp"
	"strings"
	"time"
)

var expiryDatePattern = regexp.MustCompile(`^\s*(?:(\d{4}-\d{2}-\d{2}))?\s*~\s*(?:(\d{4}-\d{2}-\d{2}))?\s*$`)

func parseExpiry(raw string, state *derivationState) {
	if strings.TrimSpace(raw) == "" || strings.TrimSpace(raw) == "-" {
		return
	}
	match := expiryDatePattern.FindStringSubmatch(raw)
	if len(match) != 3 {
		state.result.UnparsedFragments = append(state.result.UnparsedFragments, raw)
		return
	}
	valid := true
	if match[1] != "" {
		if value, err := time.Parse(time.DateOnly, match[1]); err == nil {
			state.result.ExpiryStart = &value
		} else {
			valid = false
		}
	}
	if match[2] != "" {
		if value, err := time.Parse(time.DateOnly, match[2]); err == nil {
			state.result.ExpiryEnd = &value
		} else {
			valid = false
		}
	}
	if !valid || match[1] == "" && match[2] == "" {
		state.result.UnparsedFragments = append(state.result.UnparsedFragments, raw)
		return
	}
	state.structured++
}
func parseCompany(importer, overseas string, state *derivationState) {
	trimmed := strings.TrimSpace(importer)
	if trimmed != "" {
		for _, legal := range []string{"주식회사", "유한회사", "(주)", "㈜"} {
			if strings.HasPrefix(trimmed, legal) {
				state.result.LegalEntityType = legal
				trimmed = strings.TrimSpace(strings.TrimPrefix(trimmed, legal))
				break
			}
		}
		state.result.ImporterBaseName = trimmed
		state.result.ImporterSearchKey = searchKey(trimmed)
	}
	state.result.ManufacturerName = overseas
	state.result.OverseasEstablishmentSearchKey = searchKey(overseas)
}
