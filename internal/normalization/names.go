package normalization

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"regexp"
	"strconv"
	"strings"
	"unicode"
)

// removalMark records where a token was removed so reassembly can drop the separator that joined it.
const removalMark = "\x00"

var (
	markCleanupPattern       = regexp.MustCompile(`\s*[-/,]?\s*\x00+\s*`)
	repeatedSeparatorPattern = regexp.MustCompile(`\s*([,/])(?:\s*[,/])+\s*`)

	volumeTokenPattern   = regexp.MustCompile(`^(?:\d{1,3}(?:,\d{3})+|\d+(?:\.\d+)?)\s*(?:[mM][lL]|[lL])(?:\s*[*x×X]\s*\d+\s*(?:BTLS?|BOTTLES?|EA|PCS|병)?)?$`)
	packageTokenOnly     = regexp.MustCompile(`^[*x×X]\s*\d+\s*(?:BTLS?|BOTTLES?|EA|PCS|병)?$`)
	abvTokenPattern      = regexp.MustCompile(`(?i)^(?:주도\s*|ALC\.?\s*)?(\d+(?:\.\d+)?)\s*(?:%|도)(?:\s*VOL\.?)?$`)
	proofTokenPattern    = regexp.MustCompile(`(?i)^\d+(?:\.\d+)?\s*(?:PROOF|프루프)$`)
	ageTokenPattern      = regexp.MustCompile(`(?i)^(?:\d{1,3}\s*(?:년|YO|YEARS?(?:\s+OLD)?)|AGED\s+\d{1,3})$`)
	caskTokenPattern     = regexp.MustCompile(`^#\s*\d+$`)
	batchTokenPattern    = regexp.MustCompile(`(?i)^(?:BATCH|배치)\s*#?\s*[A-Z0-9-]+$`)
	strengthTokenPattern = regexp.MustCompile(`(?i)^(?:(?:CASK|BARREL|FULL)\s+STRENGTH|OVERPROOF)$`)
	labeledLotToken      = regexp.MustCompile(`(?i)^(?:LOT\s*NO\.?|LOTE|제조번호)\s*[:.]?\s*[A-Z0-9]`)
	dateCodeToken        = regexp.MustCompile(`^\d{1,4}[./-]\d{1,2}(?:[./-]\d{1,4})?$`)
	materialCodeToken    = regexp.MustCompile(`^(?:\d{6,}|[A-Z0-9]*GX[A-Z0-9]+)$`)
	bareLotToken         = regexp.MustCompile(`^L[\s.-]?[A-Z0-9]+(?:[\s.-][A-Z0-9]+)*$`)
	opaqueCodeToken      = regexp.MustCompile(`^[A-Z0-9]{1,4}[\s.-]?[A-Z0-9]{4,}$`)
)

// nameMode selects which confirmed variant tokens a derived name drops. Volume, ABV, LOT and code tokens always drop.
type nameMode struct{ age, cask, batch, strength, proof, version bool }

var (
	baseNameMode    = nameMode{age: true, cask: true, batch: true, strength: true, proof: true, version: true}
	alcoholNameMode = nameMode{}
)

// nameSegment is either a bracket group kept or dropped as a whole, or a run of plain text.
type nameSegment struct {
	text, inner string
	group       bool
}

// displayName keeps the source wording and only rewrites what was confirmed: bracket groups holding nothing but a
// confirmed volume or ABV are unwrapped to their standard form, and LOT or code groups excluded from the SKU are dropped.
func displayName(value string, volume *volumeMatch, abv *float64) string {
	if strings.TrimSpace(value) == "" {
		return value
	}
	rebuilt := strings.Builder{}
	for _, segment := range splitNameSegments(value) {
		if !segment.group {
			rebuilt.WriteString(rebuildDisplayTokens(segment.text, volume))
			continue
		}
		unwrapped, ok := unwrapDisplayGroup(segment.inner, volume, abv)
		switch {
		case !ok:
			rebuilt.WriteString(segment.text)
		case strings.TrimSpace(unwrapped) == "":
			rebuilt.WriteString(removalMark)
		default:
			rebuilt.WriteString(" ")
			rebuilt.WriteString(unwrapped)
			rebuilt.WriteString(" ")
		}
	}
	cleaned := markCleanupPattern.ReplaceAllString(rebuilt.String(), " ")
	cleaned = strings.Trim(strings.Join(strings.Fields(cleaned), " "), " -/,")
	if cleaned == "" {
		return strings.Join(strings.Fields(value), " ")
	}
	return cleaned
}

// unwrapDisplayGroup reports whether every token in the group is a confirmed value or an excluded code, and returns the
// standard form of the confirmed ones. A group holding anything else stays untouched, brackets included.
func unwrapDisplayGroup(inner string, volume *volumeMatch, abv *float64) (string, bool) {
	kept := []string{}
	for _, token := range groupTokens(inner) {
		trimmed := strings.TrimSpace(token)
		switch {
		case trimmed == "" || isExcludedCode(trimmed):
		case volume != nil && volumeTokenPattern.MatchString(trimmed):
			kept = append(kept, standardVolume(trimmed, volume))
		case abv != nil && isConfirmedABVToken(trimmed, *abv):
			kept = append(kept, formatNumber(*abv)+"%")
		default:
			return "", false
		}
	}
	return strings.Join(kept, " "), true
}
func rebuildDisplayTokens(value string, volume *volumeMatch) string {
	tokens, joins := splitTokens(value, ",/")
	rebuilt := strings.Builder{}
	lastKept := -1
	for index, token := range tokens {
		if strings.TrimSpace(token) == "" || isExcludedCode(strings.TrimSpace(token)) {
			continue
		}
		// A separator in front of a technical annotation joined nothing in the name, so the volume keeps a plain space.
		if volumeTokenPattern.MatchString(strings.TrimSpace(token)) {
			rebuilt.WriteString(" ")
		} else {
			rebuilt.WriteString(separatorFor(joins, lastKept, index))
		}
		rebuilt.WriteString(standardVolume(token, volume))
		lastKept = index
	}
	if lastKept < 0 {
		return removalMark
	}
	return rebuilt.String()
}

// separatorFor keeps the source separator only between two kept neighbours. A separator whose other side was dropped
// would be an orphan, so it becomes a space.
func separatorFor(joins []string, lastKept, index int) string {
	switch {
	case lastKept < 0:
		return ""
	case lastKept == index-1:
		return joins[index-1]
	default:
		return " "
	}
}

// isExcludedCode marks LOT, manufacture, material and date-code tokens, which section 6.1 keeps out of the display name.
// A value token such as 500ML is checked first: it is uppercase and mixed like a code but it is the bottle volume.
func isExcludedCode(token string) bool {
	if volumeTokenPattern.MatchString(token) || abvTokenPattern.MatchString(token) ||
		packageTokenOnly.MatchString(token) || ageTokenPattern.MatchString(token) {
		return false
	}
	return labeledLotToken.MatchString(token) || dateCodeToken.MatchString(token) ||
		materialCodeToken.MatchString(token) || isCodeToken(token)
}
func isConfirmedABVToken(token string, abv float64) bool {
	match := abvTokenPattern.FindStringSubmatch(token)
	if len(match) == 0 || !strings.Contains(token, "%") {
		return false
	}
	value, err := strconv.ParseFloat(strings.TrimSpace(match[1]), 64)
	return err == nil && value == abv
}
func standardVolume(token string, volume *volumeMatch) string {
	if volume == nil {
		return token
	}
	display := volumePattern.ReplaceAllString(token, "${1}"+volume.display)
	if volume.packageCount != nil {
		display = packageDisplayPattern.ReplaceAllString(display, "${1} × "+strconv.Itoa(*volume.packageCount))
	}
	return display
}
func baseName(value string) string { return buildName(value, baseNameMode) }

// buildName parses the name into bracket groups and separator-delimited tokens, classifies each one,
// then reassembles. A bracket group leaves or stays with both brackets, so a pair is never broken.
func buildName(value string, mode nameMode) string {
	rebuilt := strings.Builder{}
	for _, segment := range splitNameSegments(value) {
		switch {
		case segment.group && droppableGroup(segment.inner, mode):
			rebuilt.WriteString(removalMark)
		case segment.group:
			rebuilt.WriteString(segment.text)
		default:
			rebuilt.WriteString(rebuildTokens(segment.text, mode))
		}
	}
	cleaned := markCleanupPattern.ReplaceAllString(rebuilt.String(), " ")
	cleaned = repeatedSeparatorPattern.ReplaceAllString(cleaned, "$1 ")
	cleaned = strings.Trim(strings.Join(strings.Fields(cleaned), " "), " -/,")
	if cleaned == "" {
		return strings.Join(strings.Fields(value), " ")
	}
	return cleaned
}
func splitNameSegments(value string) []nameSegment {
	segments := []nameSegment{}
	start, groupStart, depth := 0, 0, 0
	var opener byte
	for index := 0; index < len(value); index++ {
		char := value[index]
		switch {
		case depth == 0 && (char == '(' || char == '['):
			opener, groupStart, depth = char, index, 1
		case depth > 0 && char == opener:
			depth++
		case depth > 0 && (opener == '(' && char == ')' || opener == '[' && char == ']'):
			depth--
			if depth > 0 {
				continue
			}
			if groupStart > start {
				segments = append(segments, nameSegment{text: value[start:groupStart]})
			}
			segments = append(segments, nameSegment{text: value[groupStart : index+1], inner: value[groupStart+1 : index], group: true})
			start = index + 1
		}
	}
	if start < len(value) {
		segments = append(segments, nameSegment{text: value[start:]})
	}
	return segments
}

// splitTokens cuts at top-level separators only. A comma between digits stays inside the token so 4,000ml survives.
func splitTokens(value, separators string) ([]string, []string) {
	tokens, joins := []string{}, []string{}
	start, depth := 0, 0
	for index := 0; index < len(value); index++ {
		char := value[index]
		switch {
		case char == '(' || char == '[':
			depth++
		case char == ')' || char == ']':
			if depth > 0 {
				depth--
			}
		case depth == 0 && strings.IndexByte(separators, char) >= 0 && !thousandsComma(value, index):
			tokens = append(tokens, value[start:index])
			joins = append(joins, string(char))
			start = index + 1
		}
	}
	return append(tokens, value[start:]), joins
}
func thousandsComma(value string, index int) bool {
	if value[index] != ',' || index == 0 || index+1 >= len(value) {
		return false
	}
	return value[index-1] >= '0' && value[index-1] <= '9' && value[index+1] >= '0' && value[index+1] <= '9'
}

// nameTokens lists every top-level token of a name: bracket groups contribute their inner tokens.
func nameTokens(value string) []string {
	tokens := []string{}
	for _, segment := range splitNameSegments(value) {
		found := groupTokens(segment.inner)
		if !segment.group {
			found, _ = splitTokens(segment.text, ",/")
		}
		for _, token := range found {
			if trimmed := strings.TrimSpace(token); trimmed != "" {
				tokens = append(tokens, trimmed)
			}
		}
	}
	return tokens
}

// groupTokens splits a bracket group by comma and slash. A group that is nothing but a date code keeps its slashes.
func groupTokens(inner string) []string {
	if dateCodeToken.MatchString(strings.TrimSpace(inner)) {
		return []string{inner}
	}
	tokens, _ := splitTokens(inner, ",/")
	return tokens
}
func droppableGroup(inner string, mode nameMode) bool {
	for _, token := range groupTokens(inner) {
		if !droppableToken(token, mode) {
			return false
		}
	}
	return true
}
func rebuildTokens(value string, mode nameMode) string {
	tokens, joins := splitTokens(value, ",/")
	rebuilt := strings.Builder{}
	lastKept := -1
	for index, token := range tokens {
		if droppableToken(token, mode) {
			continue
		}
		rebuilt.WriteString(separatorFor(joins, lastKept, index))
		rebuilt.WriteString(stripTokens(token, mode))
		lastKept = index
	}
	if lastKept < 0 {
		return removalMark
	}
	return rebuilt.String()
}
func droppableToken(value string, mode nameMode) bool {
	token := strings.TrimSpace(value)
	switch {
	case token == "":
		return true
	case volumeTokenPattern.MatchString(token), packageTokenOnly.MatchString(token), abvTokenPattern.MatchString(token):
		return true
	case labeledLotToken.MatchString(token), dateCodeToken.MatchString(token), materialCodeToken.MatchString(token):
		return true
	case isCodeToken(token):
		return true
	case mode.proof && proofTokenPattern.MatchString(token):
		return true
	case mode.age && ageTokenPattern.MatchString(token):
		return true
	case mode.cask && caskTokenPattern.MatchString(token):
		return true
	case mode.batch && batchTokenPattern.MatchString(token):
		return true
	case mode.strength && strengthTokenPattern.MatchString(token):
		return true
	}
	return false
}

// isCodeToken accepts unlabeled LOT and material codes only. A word without digits stays in the name.
func isCodeToken(value string) bool {
	bareLot := bareLotToken.MatchString(value)
	if !bareLot && !opaqueCodeToken.MatchString(value) {
		return false
	}
	letters, digits := false, 0
	for _, char := range value {
		switch {
		case char >= 'A' && char <= 'Z':
			letters = true
		case char >= '0' && char <= '9':
			digits++
		}
	}
	if !letters || digits == 0 {
		return false
	}
	return bareLot || digits >= 2
}

// stripTokens removes confirmed tokens inside a kept token and marks each removal for separator cleanup.
func stripTokens(value string, mode nameMode) string {
	value = packageTokenPattern.ReplaceAllString(value, "${1}")
	value = volumePattern.ReplaceAllString(value, "${1}"+removalMark)
	value = strongABVPattern.ReplaceAllString(value, "${2}"+removalMark)
	value = koABVPattern.ReplaceAllString(value, removalMark)
	value = lotPattern.ReplaceAllString(value, removalMark)
	value = manufacturePattern.ReplaceAllString(value, removalMark)
	value = lotSuffixPattern.ReplaceAllString(value, removalMark)
	value = slashMaterial.ReplaceAllString(value, removalMark)
	if mode.proof {
		value = proofPattern.ReplaceAllString(value, removalMark)
	}
	if mode.strength {
		value = strengthPattern.ReplaceAllString(value, removalMark)
	}
	if mode.age {
		value = ageKOPattern.ReplaceAllString(value, removalMark)
		value = ageENPattern.ReplaceAllString(value, removalMark)
	}
	if mode.cask {
		value = caskPattern.ReplaceAllString(value, removalMark)
	}
	if mode.batch {
		value = batchENPattern.ReplaceAllString(value, removalMark)
		value = batchKOPattern.ReplaceAllString(value, removalMark)
	}
	if mode.version {
		value = strings.ReplaceAll(value, "구형", removalMark)
	}
	return value
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
