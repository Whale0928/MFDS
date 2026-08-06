// Package normalization derives reviewable product attributes without changing source values.
package normalization

import "time"

// Version identifies this deterministic rule set in the derived declaration row.
const Version = "mfds-normalization-v2"

const (
	StatusNormalized     Status = "NORMALIZED"
	StatusPartial        Status = "PARTIAL"
	StatusReviewRequired Status = "REVIEW_REQUIRED"
	StatusUnparsed       Status = "UNPARSED"
)

// Status is the outcome of a single source-value derivation. PENDING and STALE belong to the ledger workflow.
type Status string

// Reason records either an applied rule or why a human decision is needed.
type Reason string

const (
	ReasonVolumeUnitCaseNormalized                 Reason = "VOLUME_UNIT_CASE_NORMALIZED"
	ReasonVolumeThousandsSeparator                 Reason = "VOLUME_THOUSANDS_SEPARATOR"
	ReasonPackageCountExcludedFromBottleSKU        Reason = "PACKAGE_COUNT_EXCLUDED_FROM_BOTTLE_SKU"
	ReasonVolumeAmbiguous                          Reason = "VOLUME_AMBIGUOUS"
	ReasonVolumeMissing                            Reason = "VOLUME_MISSING"
	ReasonABVConflict                              Reason = "ABV_CONFLICT_BETWEEN_LANGUAGES"
	ReasonABVCompositionContext                    Reason = "ABV_COMPOSITION_CONTEXT"
	ReasonABVAmbiguousPosition                     Reason = "ABV_AMBIGUOUS_POSITION"
	ReasonABVOutOfAutomaticRange                   Reason = "ABV_OUT_OF_AUTOMATIC_RANGE"
	ReasonAgeConflict                              Reason = "AGE_CONFLICT_BETWEEN_LANGUAGES"
	ReasonProofPreserved                           Reason = "PROOF_PRESERVED_FOR_SKU"
	ReasonAgeExtracted                             Reason = "AGE_EXTRACTED"
	ReasonVintageReviewRequired                    Reason = "VINTAGE_YEAR_REVIEW_REQUIRED"
	ReasonLotLabeled                               Reason = "LOT_LABELED_EXCLUDED_FROM_SKU"
	ReasonManufactureNumberLabeled                 Reason = "MANUFACTURE_NUMBER_LABELED_EXCLUDED_FROM_SKU"
	ReasonLotSuffixCodeExcludedFromSKU             Reason = "LOT_SUFFIX_CODE_EXCLUDED_FROM_SKU"
	ReasonLotUnlabeledCode                         Reason = "LOT_UNLABELED_CODE"
	ReasonMaterialCodePreservedForSKU              Reason = "MATERIAL_CODE_PRESERVED_FOR_SKU"
	ReasonCaskNumberPreservedForSKU                Reason = "CASK_NUMBER_PRESERVED_FOR_SKU"
	ReasonHashNumberAmbiguous                      Reason = "HASH_NUMBER_AMBIGUOUS"
	ReasonBatchLanguageMismatch                    Reason = "BATCH_LANGUAGE_MISMATCH"
	ReasonEditionLanguageMismatch                  Reason = "EDITION_TOKEN_LANGUAGE_MISMATCH"
	ReasonKOVersionMarker                          Reason = "KO_VERSION_MARKER_WITHOUT_ENGLISH_MAPPING"
	ReasonParenthesisSemanticText                  Reason = "PARENTHESIS_SEMANTIC_TEXT"
	ReasonGenericProductName                       Reason = "GENERIC_PRODUCT_NAME_REVIEW_REQUIRED"
	ReasonAlcoholCategoryNotMapped                 Reason = "ALCOHOL_CATEGORY_NOT_MAPPED"
	ReasonManufactureCountryNotMapped              Reason = "MANUFACTURE_COUNTRY_NOT_MAPPED"
	ReasonExportCountryNotMapped                   Reason = "EXPORT_COUNTRY_NOT_MAPPED"
	ReasonCaskTypeCandidateExtracted               Reason = "CASK_TYPE_CANDIDATE_EXTRACTED"
	ReasonOverseasEstablishmentDistilleryCandidate Reason = "OVERSEAS_ESTABLISHMENT_DISTILLERY_CANDIDATE"
)

// Input contains parsed source values. It intentionally excludes raw HTML and hashes because this package never rewrites them.
type Input struct {
	ProductNameKO             string
	ProductNameEN             string
	ImporterName              string
	OverseasEstablishmentName string
	ExpiryText                string
	ItemName                  string
	ManufactureCountryName    string
	ExportCountryName         string
}

// Result is a raw-preserving derivation. Nil numeric and date values mean unknown, not zero.
type Result struct {
	BaseProductNameKO string
	BaseProductNameEN string
	SKUDisplayNameKO  string
	SKUDisplayNameEN  string
	NameSearchKeyKO   string
	NameSearchKeyEN   string

	VolumeRaw, ABVRaw, ProofRaw, StrengthType string
	VolumeML, UnitVolumeML, PackageCount      *int
	ABVPercent, ProofValue                    *float64
	AgeRaw                                    string
	AgeYears                                  *int
	VintageRaw                                string
	VintageYear                               *int
	VersionMarker, EditionName                string
	MaterialCode, CaskNumber, BatchNumber     string
	LotNumber, ManufactureNumber              string

	ExpiryRaw              string
	ExpiryStart, ExpiryEnd *time.Time

	ImporterBaseName, ImporterSearchKey string
	LegalEntityType                     string
	OverseasEstablishmentSearchKey      string

	AlcoholNameKO, AlcoholNameEN         string
	AlcoholCategoryKO, AlcoholCategoryEN string
	AlcoholRegionKO, AlcoholRegionEN     string
	AlcoholABV                           string
	CaskCandidate                        string
	DistilleryNameKOCandidate            string
	DistilleryNameENCandidate            string
	ManufactureCountry                   Country
	ExportCountry                        Country
	SKUCandidateKeySHA256                string
	Status                               Status
	Reasons                              []Reason
	UnparsedFragments                    []string
}

type derivationState struct {
	result         *Result
	structured     int
	reviewRequired bool
}

func (s *derivationState) add(reason Reason) {
	for _, existing := range s.result.Reasons {
		if existing == reason {
			return
		}
	}
	s.result.Reasons = append(s.result.Reasons, reason)
}

func (s *derivationState) review(reason Reason, fragment string) {
	s.add(reason)
	s.reviewRequired = true
	if fragment != "" {
		s.result.UnparsedFragments = append(s.result.UnparsedFragments, fragment)
	}
}
