package normalization

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strconv"
	"strings"
	"unicode"
)

func displayName(value string, volume *volumeMatch) string {
	if value == "" || volume == nil {
		return value
	}
	display := volumePattern.ReplaceAllString(value, "${1}"+volume.display)
	if volume.packageCount != nil {
		display = packageDisplayPattern.ReplaceAllString(display, "${1} × "+strconv.Itoa(*volume.packageCount))
	}
	return display
}
func baseName(value string) string {
	value = volumePattern.ReplaceAllString(value, "$1")
	value = packageTokenPattern.ReplaceAllString(value, "")
	value = strongABVPattern.ReplaceAllString(value, "")
	value = koABVPattern.ReplaceAllString(value, "")
	value = proofPattern.ReplaceAllString(value, "")
	value = strengthPattern.ReplaceAllString(value, "")
	value = ageKOPattern.ReplaceAllString(value, "")
	value = ageENPattern.ReplaceAllString(value, "")
	value = lotPattern.ReplaceAllString(value, "")
	value = manufacturePattern.ReplaceAllString(value, "")
	value = lotSuffixPattern.ReplaceAllString(value, "")
	value = parenMaterial.ReplaceAllString(value, "")
	value = slashMaterial.ReplaceAllString(value, "")
	value = caskPattern.ReplaceAllString(value, "")
	value = batchENPattern.ReplaceAllString(value, "")
	value = batchKOPattern.ReplaceAllString(value, "")
	value = strings.ReplaceAll(value, "구형", "")
	value = strings.Trim(value, " -/,()[]")
	return strings.Join(strings.Fields(value), " ")
}
func searchKey(value string) string {
	value = strings.NewReplacer("’", "'", "‘", "'", "–", "-", "—", "-", "－", "-").Replace(value)
	value = strings.ToLower(strings.TrimSpace(value))
	return strings.Join(strings.FieldsFunc(value, unicode.IsSpace), " ")
}
func canonicalizeKoreanSearchKey(value string) string {
	return strings.NewReplacer(
		"스트랭스", "스트렝스",
		"셰리", "쉐리",
		"피니쉬", "피니시",
		"바렐", "배럴",
	).Replace(value)
}
func isGenericProductName(result Result) bool {
	ko := strings.ReplaceAll(result.NameSearchKeyKO, " ", "")
	if _, ok := map[string]struct{}{
		"위스키": {}, "보드카": {}, "브랜디": {}, "리큐르": {}, "일반증류주": {},
	}[ko]; ok {
		return true
	}
	if result.NameSearchKeyKO != "" {
		return false
	}
	_, ok := map[string]struct{}{
		"whisky": {}, "whiskey": {}, "vodka": {}, "brandy": {}, "liqueur": {},
		"general distilled spirits": {}, "distilled spirits": {},
	}[result.NameSearchKeyEN]
	return ok
}
func candidateHash(result Result) string {
	canonical := []string{result.NameSearchKeyKO, result.NameSearchKeyEN, intString(result.UnitVolumeML), intString(result.AgeYears), floatString(result.ABVPercent), floatString(result.ProofValue), result.StrengthType, result.VersionMarker, result.EditionName, result.MaterialCode, result.CaskNumber, result.BatchNumber}
	encoded, err := json.Marshal(canonical)
	if err != nil {
		return ""
	}
	hash := sha256.Sum256(encoded)
	return hex.EncodeToString(hash[:])
}
func cleanCode(value string) string {
	value = strings.TrimSpace(value)
	value = strings.Trim(value, ")]")
	return strings.Join(strings.Fields(value), " ")
}
func formatNumber(value float64) string   { return strconv.FormatFloat(value, 'f', -1, 64) }
func intPointer(value int) *int           { return &value }
func floatPointer(value float64) *float64 { return &value }
func intString(value *int) string {
	if value == nil {
		return ""
	}
	return strconv.Itoa(*value)
}
func floatString(value *float64) string {
	if value == nil {
		return ""
	}
	return formatNumber(*value)
}
