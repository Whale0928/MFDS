package app

import (
	"testing"

	matchdomain "github.com/bottle-note/mfds-crawler/internal/matching"
	parser "github.com/bottle-note/mfds-crawler/internal/normalization"
	"github.com/bottle-note/mfds-crawler/internal/usecase/inheritance"
	usecase "github.com/bottle-note/mfds-crawler/internal/usecase/normalization"
)

func emptySnapshot(t *testing.T) *matchdomain.ReferenceSnapshot {
	t.Helper()
	snapshot, err := matchdomain.NewReferenceSnapshot(nil, nil, nil, matchdomain.MatchingVersion{RuleVersion: "test", ReferenceHash: "0"})
	if err != nil {
		t.Fatal(err)
	}
	return snapshot
}

func glenmorangieSource(declarationID int64) usecase.Source {
	return usecase.Source{
		DeclarationID: declarationID, RCNO: "R", ProductNameKO: "글렌모렌지 프라이빗에디션 스피오스",
		ProductNameEN: "GLENMORANGIE PRIVATE EDITION SPIOS", ItemName: "위스키",
		ManufactureCountryName: "영국", ExportCountryName: "영국",
	}
}

func TestParserAdapter_같은키의관리자매칭이있으면매처없이바로쓴다(t *testing.T) {
	// Given: 같은 원문으로 정제된 관리자 확정 신고 하나
	source := glenmorangieSource(99)
	normalized := parser.Normalize(parser.Input{
		ProductNameKO: source.ProductNameKO, ProductNameEN: source.ProductNameEN, ItemName: source.ItemName,
		ManufactureCountryName: source.ManufactureCountryName, ExportCountryName: source.ExportCountryName,
	})
	seed := inheritance.Row{
		DeclarationID: 1, IdentityKey: normalized.ProductIdentityKeySHA256, NormalizationStatus: "NORMALIZED",
		AlcoholCategoryEN: normalized.AlcoholCategoryEN, ManufactureCountryAlpha2: normalized.ManufactureCountry.Alpha2,
		Decision: "MANUAL", SelectedAlcoholID: 5582,
	}
	seeds := inheritance.BuildSeedIndex([]inheritance.Row{seed}, map[int64]inheritance.Alcohol{5582: {ID: 5582, DistilleryID: 7, RegionID: 8}})
	adapter := parserAdapter{matcher: emptySnapshot(t), seeds: seeds}

	// When
	result, err := adapter.Normalize(source)

	// Then
	if err != nil {
		t.Fatal(err)
	}
	fields := result.Fields
	if fields.InheritedFromDeclarationID != 1 || fields.MatchingResult.AlcoholDecision.Status != matchdomain.DecisionInherited ||
		fields.MatchingResult.AlcoholDecision.SelectedID != 5582 || fields.MatchingResult.DistilleryDecision.SelectedID != 7 ||
		fields.MatchingResult.RegionDecision.SelectedID != 8 || len(fields.AlcoholCandidates) != 0 {
		t.Fatalf("fields = %+v", fields.MatchingResult)
	}
}

func TestParserAdapter_관리자매칭이없으면매처를돌린다(t *testing.T) {
	// Given
	adapter := parserAdapter{matcher: emptySnapshot(t), seeds: inheritance.BuildSeedIndex(nil, nil)}

	// When
	result, err := adapter.Normalize(glenmorangieSource(99))

	// Then
	if err != nil {
		t.Fatal(err)
	}
	if result.Fields.InheritedFromDeclarationID != 0 || result.Fields.MatchingResult.AlcoholDecision.Status == matchdomain.DecisionInherited {
		t.Fatalf("matcher was skipped: %+v", result.Fields.MatchingResult)
	}
}
