// Package matching provides immutable, deterministic reference matching.
package matching

import "time"

// MatchingVersion identifies the rules and reference snapshot used for a match.
type MatchingVersion struct {
	RuleVersion   string
	ReferenceHash string
}

// String fits the rule and a collision-resistant reference hash prefix into declarations.matching_version.
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

// ReferenceSnapshot is an immutable index that can be reused for many inputs.
type ReferenceSnapshot struct {
	version       MatchingVersion
	alcohols      []indexedAlcohol
	distilleries  []indexedDistillery
	regions       []indexedRegion
	distilleryIDs map[int64]struct{}
	regionByID    map[int64]indexedRegion
}

// Candidate is a ranked distillery or region result. No candidate is selected automatically.
type Candidate struct {
	ID               int64
	Score            int
	EvidenceStrength int
	Evidence         []Evidence
}

// Evidence explains one score contribution.
type Evidence struct {
	Kind   string
	Source string
	Weight int
}

// MatchResult contains independent top-three lists for distilleries and regions.
type MatchResult struct {
	Version      MatchingVersion
	Distilleries []Candidate
	Regions      []Candidate
	Exact        bool
}

type indexedName struct {
	label  string
	tokens []string
}

type indexedAlcohol struct {
	reference AlcoholReference
	names     []indexedName
}

type indexedDistillery struct {
	reference DistilleryReference
	names     []indexedName
}

type indexedRegion struct {
	reference RegionReference
	names     []indexedName
}
