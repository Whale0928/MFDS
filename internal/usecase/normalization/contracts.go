// Package normalization coordinates non-destructive declaration normalization.
package normalization

import (
	"context"
	"time"

	matchdomain "github.com/bottle-note/mfds-crawler/internal/matching"
)

// Status is the current normalization decision for one declaration.
type Status string

const (
	StatusPending        Status = "PENDING"
	StatusStale          Status = "STALE"
	StatusNormalized     Status = "NORMALIZED"
	StatusPartial        Status = "PARTIAL"
	StatusReviewRequired Status = "REVIEW_REQUIRED"
	StatusUnparsed       Status = "UNPARSED"
)

func (s Status) IsTerminalDataStatus() bool {
	return s == StatusNormalized || s == StatusPartial || s == StatusReviewRequired || s == StatusUnparsed
}

// Source is the latest immutable ledger observation selected for one RCNO.
type Source struct {
	DeclarationID             int64
	RCNO                      string
	SourceItemID              int64
	SemanticHash              string
	ProductNameKO             string
	ProductNameEN             string
	ItemName                  string
	ImporterName              string
	OverseasEstablishmentName string
	ManufactureCountryName    string
	ExportCountryName         string
	ExpiryText                string
	ProcessedDate             time.Time
	ClaimOwner                string
	ClaimAttempt              int
}

// Fields contains only derived values. It never contains ledger HTML or raw hashes.
type Fields struct {
	BaseProductNameKO              string
	BaseProductNameEN              string
	SKUDisplayNameKO               string
	SKUDisplayNameEN               string
	NameSearchKeyKO                string
	NameSearchKeyEN                string
	SKUCandidateKeySHA256          string
	VolumeRaw                      string
	VolumeML                       *int
	UnitVolumeML                   *int
	PackageCount                   *int
	ABVRaw                         string
	ABVPercent                     *float64
	IngredientPercentRaw           string
	IngredientPercent              *float64
	ProofRaw                       string
	ProofValue                     *float64
	StrengthType                   string
	AgeRaw                         string
	AgeYears                       *int
	VintageRaw                     string
	VintageYear                    *int
	VersionMarker                  string
	EditionName                    string
	VariantMarkerRaw               string
	VariantMarkerType              string
	VariantMarkerValue             string
	MaterialCode                   string
	CaskNumber                     string
	BatchNumber                    string
	LotNumber                      string
	ManufactureNumber              string
	ExpiryRaw                      string
	ExpiryStart                    *time.Time
	ExpiryEnd                      *time.Time
	ImporterBaseName               string
	ImporterSearchKey              string
	LegalEntityType                string
	ManufacturerName               string
	OverseasEstablishmentSearchKey string
	AlcoholNameKO                  string
	AlcoholNameEN                  string
	AlcoholCategoryKO              string
	AlcoholCategoryEN              string
	AlcoholRegionKO                string
	AlcoholRegionEN                string
	AlcoholABV                     string
	CaskCandidate                  string
	DistilleryNameKOCandidate      string
	DistilleryNameENCandidate      string
	ManufactureCountryNameKO       string
	ManufactureCountryNameEN       string
	ManufactureCountryAlpha2       string
	ManufactureCountryAlpha3       string
	ExportCountryNameKO            string
	ExportCountryNameEN            string
	ExportCountryAlpha2            string
	ExportCountryAlpha3            string
	AlcoholCandidates              []ReferenceCandidate
	DistilleryCandidates           []ReferenceCandidate
	RegionCandidates               []ReferenceCandidate
	MatchingVersion                string
	MatchingRunID                  int64
	MatchingResult                 matchdomain.MatchResult
}

// ReferenceCandidate is one ranked BottleNote reference. Selection remains an administrator decision.
type ReferenceCandidate struct {
	ID    int64
	Score float64
}

// Result is the parser's deterministic, non-destructive decision.
type Result struct {
	Status            Status
	Fields            Fields
	Reasons           []string
	UnparsedFragments []string
}

// Parser must be pure: it has no database or clock dependency.
type Parser interface {
	Normalize(Source) (Result, error)
}

type ClaimRequest struct {
	Limit         int
	RCNO          string
	Owner         string
	LeaseDuration time.Duration
	MaxAttempts   int
	RetryDelay    time.Duration
}

type Completion struct {
	Source               Source
	Result               Result
	NormalizationVersion string
	NormalizedAt         time.Time
}

type Failure struct {
	Source Source
	// RetryDelay는 시각이 아니라 간격이다. 실제 재시도 시각은 store가 DB 시계로 계산한다.
	RetryDelay time.Duration
	LastError  string
}

type Remaining struct {
	Pending int
	Stale   int
}

// Store owns synchronization, claim leases, and all database mutations.
type Store interface {
	// SyncDeclarations creates PENDING declarations and refreshes latest source references.
	SyncDeclarations(context.Context) error
	// ForceRequeue marks existing terminal declarations stale without changing source or derived values.
	ForceRequeue(context.Context) error
	// Claim returns PENDING/STALE work, or force-selects exactly one RCNO.
	Claim(context.Context, ClaimRequest) ([]Source, error)
	// Preview is read-only and must not acquire a lease or modify attempts.
	Preview(context.Context, ClaimRequest) ([]Source, error)
	Complete(context.Context, Completion) error
	Fail(context.Context, Failure) error
	Remaining(context.Context) (Remaining, error)
}
