// Package matching provides immutable, deterministic reference matching.
package matching

import "time"

const maxStoredCandidates = 3

// MatchingVersion identifies the rules and reference snapshot used for a match.
type MatchingVersion struct {
	RuleVersion   string
	ReferenceHash string
}

// String fits the rule and a collision-resistant reference hash prefix into mfds_declarations.matching_version.
func (v MatchingVersion) String() string {
	const (
		columnLength = 64
		hashLength   = 32
	)
	hash := v.ReferenceHash
	if len(hash) > hashLength {
		hash = hash[:hashLength]
	}
	rule := v.RuleVersion
	if available := columnLength - len(hash) - 1; available > 0 && len(rule) > available {
		rule = rule[:available]
	}
	if rule == "" || hash == "" {
		return ""
	}
	return rule + ":" + hash
}

// Input contains only normalized values that may participate in matching.
type Input struct {
	BaseNameKO         string
	BaseNameEN         string
	SearchNameKO       string
	SearchNameEN       string
	ABVPercent         *float64
	Age                string
	AgeYears           *int
	Cask               string
	Edition            string
	Category           string
	UnitVolumeML       *int
	ManufactureCountry string
}

// AlcoholReference is one BottleNote alcohol reference row.
type AlcoholReference struct {
	ID           int64
	KorName      string
	EngName      string
	ABVPercent   *float64
	Type         string
	Category     string
	RegionID     int64
	DistilleryID int64
	Age          string
	AgeYears     *int
	Cask         string
	Volume       string
	DeletedAt    *time.Time
}

// DistilleryReference is one BottleNote distillery reference row.
type DistilleryReference struct {
	ID      int64
	KorName string
	EngName string
}

// RegionReference is one BottleNote region reference row.
type RegionReference struct {
	ID       int64
	KorName  string
	EngName  string
	ParentID int64
}

// ReferenceAlias is one administrator-managed lookup term for a reference entity.
type ReferenceAlias struct {
	EntityType string
	EntityID   int64
	Alias      string
	Language   string
	Source     string
}

// ReferenceSnapshot is an immutable index that can be reused for many inputs.
type ReferenceSnapshot struct {
	version                  MatchingVersion
	alcohols                 []indexedAlcohol
	distilleries             []indexedDistillery
	regions                  []indexedRegion
	alcoholByID              map[int64]indexedAlcohol
	distilleryByID           map[int64]indexedDistillery
	distilleryIDs            map[int64]struct{}
	regionByID               map[int64]indexedRegion
	regionCountsByDistillery map[int64]map[int64]int
	alcoholIDsByBaseKey      map[string][]int64
	distilleryIDsByBaseKey   map[string][]int64
	regionIDsByBaseKey       map[string][]int64
	alcoholIDsByToken        map[string][]int64
	distilleryIDsByToken     map[string][]int64
	regionIDsByToken         map[string][]int64
}

// Candidate is one ranked BottleNote reference with machine-readable evidence.
type Candidate struct {
	ID               int64
	NameKO           string
	NameEN           string
	Score            float64
	EvidenceStrength int
	Evidence         []Evidence
}

// Evidence explains one score contribution.
type Evidence struct {
	Kind                string
	Source              string
	InputValue          string
	ReferenceValue      string
	RuleCode            string
	Weight              float64
	UpstreamCandidateID int64
}

type DecisionStatus string

const (
	DecisionAutoSelected   DecisionStatus = "AUTO_SELECTED"
	DecisionAmbiguous      DecisionStatus = "AMBIGUOUS"
	DecisionReview         DecisionStatus = "REVIEW"
	DecisionNoMatch        DecisionStatus = "NO_MATCH"
	DecisionConflictReview DecisionStatus = "CONFLICT_REVIEW"
)

// MatchDecision records why one target was selected, deferred, or rejected.
type MatchDecision struct {
	Status           DecisionStatus
	Source           string
	StopReason       string
	SelectedID       int64
	TopScore         float64
	Margin           float64
	CompetitiveCount int
}

// AlcoholConsensus contains safe reference values shared by every competitive alcohol candidate.
type AlcoholConsensus struct {
	DistilleryID int64
	RegionID     int64
}

// MatchResult contains independent top-three lists for alcohols, distilleries, and regions.
type MatchResult struct {
	Version            MatchingVersion
	Alcohols           []Candidate
	Distilleries       []Candidate
	Regions            []Candidate
	AlcoholDecision    MatchDecision
	DistilleryDecision MatchDecision
	RegionDecision     MatchDecision
	AlcoholConsensus   AlcoholConsensus
	Exact              bool
}

type indexedName struct {
	label   string
	tokens  []string
	baseKey string
}

type indexedAlcohol struct {
	reference AlcoholReference
	names     []indexedName
	ageYears  *int
	volumeML  *int
	conflicts []string
}

type indexedDistillery struct {
	reference DistilleryReference
	names     []indexedName
}

type indexedRegion struct {
	reference RegionReference
	names     []indexedName
}
