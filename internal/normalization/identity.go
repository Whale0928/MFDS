package normalization

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"math"
	"strconv"
	"strings"
)

// ProductIdentityKeyVersion names the component list hashed into product_identity_key_sha256.
const ProductIdentityKeyVersion = "mfds-product-identity-v2"

// ProductIdentity holds only stored derived columns, so missing keys can be filled without renormalizing.
type ProductIdentity struct {
	NameSearchKeyKO string
	NameSearchKeyEN string
	ABVPercent      *float64
	AgeYears        *int
	StrengthType    string
}

// IdentityFromResult selects the product identity components of one normalization result.
func IdentityFromResult(result Result) ProductIdentity {
	return ProductIdentity{
		NameSearchKeyKO: result.NameSearchKeyKO, NameSearchKeyEN: result.NameSearchKeyEN,
		ABVPercent: result.ABVPercent, AgeYears: result.AgeYears, StrengthType: result.StrengthType,
	}
}

// ProductIdentityKey hashes both language search keys, ABV, age, and strength type. Volume, importer, and lot-like codes
// are left out so bottle sizes and parallel imports of one product share a key. Strength type stays because many
// declarations lack an ABV, and without it a cask strength release would merge with the regular bottling. A missing
// component is a fixed JSON null. Without both search keys the name alone cannot identify a product, so the key is empty.
func ProductIdentityKey(identity ProductIdentity) string {
	ko, en := strings.TrimSpace(identity.NameSearchKeyKO), strings.TrimSpace(identity.NameSearchKeyEN)
	if ko == "" || en == "" {
		return ""
	}
	canonical := []any{
		ProductIdentityKeyVersion, ko, en,
		identityDecimal(identity.ABVPercent), identityInt(identity.AgeYears), identityText(identity.StrengthType),
	}
	encoded, err := json.Marshal(canonical)
	if err != nil {
		return ""
	}
	hash := sha256.Sum256(encoded)
	return hex.EncodeToString(hash[:])
}

func identityInt(value *int) any {
	if value == nil {
		return nil
	}
	return strconv.Itoa(*value)
}

// identityDecimal rounds to the three decimals the DECIMAL columns keep, so a stored value hashes like a fresh one.
func identityDecimal(value *float64) any {
	if value == nil {
		return nil
	}
	return formatNumber(math.Round(*value*1000) / 1000)
}

// identityText treats an empty string as missing because the store writes empty derived text as NULL.
func identityText(value string) any {
	if value == "" {
		return nil
	}
	return value
}
