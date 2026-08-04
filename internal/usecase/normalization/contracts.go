// Package normalization coordinates non-destructive declaration normalization.
package normalization

import (
	"context"
	"time"
)

// Status is the current normalization decision for one declaration.
type Status string

const (
	Version = "mfds-normalization-v1"

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
	ProofRaw                       string
	ProofValue                     *float64
	StrengthType                   string
	AgeRaw                         string
	AgeYears                       *int
	VintageRaw                     string
	VintageYear                    *int
	VersionMarker                  string
	EditionName                    string
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
	OverseasEstablishmentSearchKey string
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
	Source    Source
	RetryAt   time.Time
	LastError string
}

type Remaining struct {
	Pending int
	Stale   int
}

// Store owns synchronization, claim leases, and all database mutations.
type Store interface {
	// SyncDeclarations creates PENDING declarations and refreshes latest source references.
	SyncDeclarations(context.Context) error
	// Claim returns PENDING/STALE work, or force-selects exactly one RCNO.
	Claim(context.Context, ClaimRequest) ([]Source, error)
	// Preview is read-only and must not acquire a lease or modify attempts.
	Preview(context.Context, ClaimRequest) ([]Source, error)
	Complete(context.Context, Completion) error
	Fail(context.Context, Failure) error
	Remaining(context.Context) (Remaining, error)
}
