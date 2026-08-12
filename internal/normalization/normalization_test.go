package normalization

import (
	"strings"
	"testing"
	"time"
)

func TestNormalize_필수명칭회귀사례를보존하며구조화한다(t *testing.T) {
	tests := []struct {
		name  string
		input Input
		check func(*testing.T, Result)
	}{
		{
			name:  "천단위 용량",
			input: Input{ProductNameKO: "대용량 위스키 4,000ml"},
			check: func(t *testing.T, result Result) {
				mustInt(t, result.UnitVolumeML, 4000)
				if result.SKUDisplayNameKO != "대용량 위스키 4000ml" || !hasReason(result, ReasonVolumeThousandsSeparator) {
					t.Fatalf("result = %+v", result)
				}
			},
		},
		{
			name:  "묶음 용량",
			input: Input{ProductNameKO: "고량주 250ML*2"},
			check: func(t *testing.T, result Result) {
				mustInt(t, result.UnitVolumeML, 250)
				mustInt(t, result.PackageCount, 2)
				if result.SKUDisplayNameKO != "고량주 250ml × 2" {
					t.Fatalf("display name = %q", result.SKUDisplayNameKO)
				}
				if !hasReason(result, ReasonPackageCountExcludedFromBottleSKU) {
					t.Fatalf("reasons = %v", result.Reasons)
				}
			},
		},
		{
			name:  "성분 퍼센트는 ABV가 아니다",
			input: Input{ProductNameKO: "설원 인삼송이주(인삼0.45%) 150ml"},
			check: func(t *testing.T, result Result) {
				if result.ABVPercent != nil || !hasReason(result, ReasonABVCompositionContext) {
					t.Fatalf("result = %+v", result)
				}
			},
		},
		{
			name:  "영문 끝 도수",
			input: Input{ProductNameEN: "LOW LIQUOR 42%"},
			check: func(t *testing.T, result Result) { mustFloat(t, result.ABVPercent, 42) },
		},
		{
			name:  "100 percent islay는 ABV가 아니다",
			input: Input{ProductNameEN: "KILCHOMAN 100% ISLAY"},
			check: func(t *testing.T, result Result) {
				if result.ABVPercent != nil || hasReason(result, ReasonABVAmbiguousPosition) {
					t.Fatalf("result = %+v", result)
				}
			},
		},
		{
			name:  "용량 숫자는 vintage가 아니다",
			input: Input{ProductNameKO: "가쿠빈 1920ML"},
			check: func(t *testing.T, result Result) {
				mustInt(t, result.UnitVolumeML, 1920)
				if result.VintageRaw != "" || result.VintageYear != nil {
					t.Fatalf("result = %+v", result)
				}
			},
		},
		{
			name:  "명시 LOT은 SKU에서 제외",
			input: Input{ProductNameKO: "몽키숄더-700ML(LOT NO. L 0111613 2605)"},
			check: func(t *testing.T, result Result) {
				if result.LotNumber != "L 0111613 2605" || !hasReason(result, ReasonLotLabeled) {
					t.Fatalf("result = %+v", result)
				}
			},
		},
		{
			name:  "제조번호 내부 연도는 vintage가 아니다",
			input: Input{ProductNameKO: "발베니 12년-700ML(제조번호 : L 0113953 2006)"},
			check: func(t *testing.T, result Result) {
				if result.ManufactureNumber != "L 0113953 2006" || result.VintageRaw != "" {
					t.Fatalf("result = %+v", result)
				}
			},
		},
		{
			name:  "한글 구형은 별도 버전 주석",
			input: Input{ProductNameKO: "글렌알라키 15년 구형", ProductNameEN: "GLENALLACHIE 15 YEAR OLD"},
			check: func(t *testing.T, result Result) {
				if result.VersionMarker != "구형" || result.BaseProductNameEN != "GLENALLACHIE" {
					t.Fatalf("result = %+v", result)
				}
			},
		},
		{
			name:  "한글 숙성연수 경계",
			input: Input{ProductNameKO: "글렌알라키 15년 구형"},
			check: func(t *testing.T, result Result) { mustInt(t, result.AgeYears, 15) },
		},
		{
			name:  "OLD 브랜드는 보존",
			input: Input{ProductNameEN: "OLD FORESTER"},
			check: func(t *testing.T, result Result) {
				if result.BaseProductNameEN != "OLD FORESTER" || result.SKUDisplayNameEN != "OLD FORESTER" {
					t.Fatalf("result = %+v", result)
				}
			},
		},
		{
			name:  "NEW 브랜드는 보존",
			input: Input{ProductNameEN: "NEW RIFF"},
			check: func(t *testing.T, result Result) {
				if result.BaseProductNameEN != "NEW RIFF" {
					t.Fatalf("result = %+v", result)
				}
			},
		},
		{
			name:  "슬래시 자재 코드는 SKU 축으로 유지",
			input: Input{ProductNameKO: "싱글 몰트 스카치 위스키 / 135066GX0700615"},
			check: func(t *testing.T, result Result) {
				if result.MaterialCode != "135066GX0700615" || !hasReason(result, ReasonMaterialCodePreservedForSKU) {
					t.Fatalf("result = %+v", result)
				}
			},
		},
		{
			name:  "괄호 자재 코드와 L 접미 LOT 분리",
			input: Input{ProductNameKO: "조니워커 700ml (778061) L5293CA009"},
			check: func(t *testing.T, result Result) {
				if result.MaterialCode != "778061" || result.LotNumber != "L5293CA009" {
					t.Fatalf("result = %+v", result)
				}
			},
		},
		{
			name:  "single cask 번호는 SKU 축",
			input: Input{ProductNameKO: "글렌알라키 싱글캐스크 (59.1%) (#9485)"},
			check: func(t *testing.T, result Result) {
				if result.CaskNumber != "9485" || !hasReason(result, ReasonCaskNumberPreservedForSKU) {
					t.Fatalf("result = %+v", result)
				}
			},
		},
		{
			name:  "도수가 다른 카발란은 다른 후보 키",
			input: Input{ProductNameKO: "카발란 솔리스트 올로로소 쉐리캐스크 (51.6%)"},
			check: func(t *testing.T, result Result) { mustFloat(t, result.ABVPercent, 51.6) },
		},
		{
			name:  "병 수량은 후보 키에서 제외",
			input: Input{ProductNameKO: "고량주 250ml × 20"},
			check: func(t *testing.T, result Result) { mustInt(t, result.PackageCount, 20) },
		},
		{
			name:  "음차는 영문 일치 없이 통일하지 않음",
			input: Input{ProductNameKO: "스트렝스 쉐리 피니시"},
			check: func(t *testing.T, result Result) {
				if result.NameSearchKeyKO != "스트렝스 쉐리 피니시" {
					t.Fatalf("result = %+v", result)
				}
			},
		},
		{
			name:  "semantic 괄호 에디션은 보존",
			input: Input{ProductNameKO: "위스키 (서울에디션)"},
			check: func(t *testing.T, result Result) {
				if result.SKUDisplayNameKO != "위스키 (서울에디션)" || result.EditionName == "" {
					t.Fatalf("result = %+v", result)
				}
			},
		},
		{
			name:  "open ended expiry start",
			input: Input{ExpiryText: "2025-06-03 ~"},
			check: func(t *testing.T, result Result) {
				mustDate(t, result.ExpiryStart, "2025-06-03")
				if result.ExpiryEnd != nil {
					t.Fatalf("result = %+v", result)
				}
			},
		},
		{
			name:  "open ended expiry end",
			input: Input{ExpiryText: "~ 2026-08-26"},
			check: func(t *testing.T, result Result) {
				mustDate(t, result.ExpiryEnd, "2026-08-26")
				if result.ExpiryStart != nil {
					t.Fatalf("result = %+v", result)
				}
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			beforeKO, beforeEN := test.input.ProductNameKO, test.input.ProductNameEN
			result := Normalize(test.input)
			if test.input.ProductNameKO != beforeKO || test.input.ProductNameEN != beforeEN {
				t.Fatalf("input was mutated: %+v", test.input)
			}
			test.check(t, result)
		})
	}
}

func TestNormalize_PROOF는검토대상으로보존한다(t *testing.T) {
	result := Normalize(Input{ProductNameEN: "JACK DANIEL'S SINGLE BARREL 100 PROOF"})
	mustFloat(t, result.ProofValue, 100)
	if result.Status != StatusReviewRequired || !hasReason(result, ReasonProofPreserved) {
		t.Fatalf("result = %+v", result)
	}
	if result.BaseProductNameEN != "JACK DANIEL'S SINGLE BARREL" {
		t.Fatalf("base product name = %q", result.BaseProductNameEN)
	}
}

func TestNormalize_변형토큰_BaseName에서분리하고Display에보존한다(t *testing.T) {
	result := Normalize(Input{
		ProductNameKO: "글렌알라키 싱글캐스크 배치 3 (#9485)",
		ProductNameEN: "GLENALLACHIE SINGLE CASK BATCH 3 CASK STRENGTH (#9485)",
	})
	if strings.Contains(result.BaseProductNameKO, "9485") || strings.Contains(result.BaseProductNameKO, "배치 3") ||
		strings.Contains(result.BaseProductNameEN, "9485") || strings.Contains(result.BaseProductNameEN, "BATCH 3") ||
		strings.Contains(result.BaseProductNameEN, "CASK STRENGTH") {
		t.Fatalf("base names retain variant tokens: ko=%q en=%q", result.BaseProductNameKO, result.BaseProductNameEN)
	}
	if !strings.Contains(result.SKUDisplayNameKO, "#9485") || !strings.Contains(result.SKUDisplayNameEN, "CASK STRENGTH") {
		t.Fatalf("display names lost variant evidence: ko=%q en=%q", result.SKUDisplayNameKO, result.SKUDisplayNameEN)
	}
}

func TestNormalize_이름과업체명은원문보존형으로파생한다(t *testing.T) {
	result := Normalize(Input{
		ProductNameKO:             "일반 위스키-700ML",
		ProductNameEN:             "PLAIN WHISKY",
		ImporterName:              "주식회사 보틀노트",
		OverseasEstablishmentName: "O’Connor Distillery",
	})
	if result.Status != StatusNormalized || result.SKUDisplayNameKO != "일반 위스키-700ml" ||
		result.ImporterBaseName != "보틀노트" || result.LegalEntityType != "주식회사" ||
		result.ManufacturerName != "O’Connor Distillery" ||
		result.OverseasEstablishmentSearchKey != "o'connor distillery" {
		t.Fatalf("result = %+v", result)
	}
}

func TestNormalize_명시LOT은다른언어이름을삼키지않는다(t *testing.T) {
	result := Normalize(Input{
		ProductNameKO: "몽키숄더(LOT NO. L 0111613 2605)",
		ProductNameEN: "MONKEY SHOULDER",
	})
	if result.LotNumber != "L 0111613 2605" || result.BaseProductNameEN != "MONKEY SHOULDER" {
		t.Fatalf("result = %+v", result)
	}
}

func TestNormalize_품목총칭은후보키를만들지않는다(t *testing.T) {
	for _, input := range []Input{
		{ProductNameKO: "위스키"},
		{ProductNameEN: "WHISKY"},
		{ProductNameKO: "일반 증류주"},
	} {
		result := Normalize(input)
		if result.Status != StatusReviewRequired || result.SKUCandidateKeySHA256 != "" || !hasReason(result, ReasonGenericProductName) {
			t.Fatalf("input = %+v, result = %+v", input, result)
		}
	}
	if result := Normalize(Input{}); result.SKUCandidateKeySHA256 != "" || result.Status != StatusUnparsed {
		t.Fatalf("empty result = %+v", result)
	}
}

func TestNormalize_한글음차검색키는영문명이있을때만통일한다(t *testing.T) {
	first := Normalize(Input{ProductNameKO: "셰리 스트랭스 피니쉬 바렐", ProductNameEN: "SHERRY STRENGTH FINISH BARREL"})
	second := Normalize(Input{ProductNameKO: "쉐리 스트렝스 피니시 배럴", ProductNameEN: "SHERRY STRENGTH FINISH BARREL"})
	withoutEnglish := Normalize(Input{ProductNameKO: "셰리 스트랭스 피니쉬 바렐"})
	if first.NameSearchKeyKO != "쉐리 스트렝스 피니시 배럴" || first.SKUCandidateKeySHA256 != second.SKUCandidateKeySHA256 {
		t.Fatalf("with English: first=%+v second=%+v", first, second)
	}
	if withoutEnglish.NameSearchKeyKO != "셰리 스트랭스 피니쉬 바렐" {
		t.Fatalf("without English = %+v", withoutEnglish)
	}
}

func TestNormalize_SKU후보키는확정값만포함한다(t *testing.T) {
	first := Normalize(Input{ProductNameKO: "고량주 250ml × 20 (LOT NO. L 0111613 2605)"})
	second := Normalize(Input{ProductNameKO: "고량주 250ml × 36 (LOT NO. L 0113949 1606)"})
	differentABV := Normalize(Input{ProductNameKO: "고량주 250ml 58%"})

	if first.SKUCandidateKeySHA256 == "" || first.SKUCandidateKeySHA256 != second.SKUCandidateKeySHA256 {
		t.Fatalf("lot/package changed candidate key: %q != %q", first.SKUCandidateKeySHA256, second.SKUCandidateKeySHA256)
	}
	if first.SKUCandidateKeySHA256 == differentABV.SKUCandidateKeySHA256 {
		t.Fatalf("ABV must change candidate key: %q", first.SKUCandidateKeySHA256)
	}
}

// 용량 미확인은 원장 80%가 해당하는 정상 상태이므로 검토 사유가 아니다.
// 후보키는 여전히 만들지 않아 용량이 확인된 SKU와 자동 병합되지 않는다.
func TestNormalize_용량미확인_후보키없이사유만남긴다(t *testing.T) {
	result := Normalize(Input{ProductNameKO: "OLD FORESTER", ProductNameEN: "OLD FORESTER"})
	if result.Status != StatusNormalized || result.SKUCandidateKeySHA256 != "" || !hasReason(result, ReasonVolumeMissing) {
		t.Fatalf("result = %+v", result)
	}
}

func TestNormalize_유효하지않은소비기한_PARTIAL과원문근거를남긴다(t *testing.T) {
	result := Normalize(Input{ProductNameKO: "테스트 위스키 700ml", ExpiryText: "2026-02-30 ~"})
	if result.Status != StatusPartial || result.ExpiryStart != nil || len(result.UnparsedFragments) != 1 ||
		result.UnparsedFragments[0] != "2026-02-30 ~" {
		t.Fatalf("result = %+v", result)
	}
}

func mustInt(t *testing.T, actual *int, expected int) {
	t.Helper()
	if actual == nil || *actual != expected {
		t.Fatalf("actual = %v, expected %d", actual, expected)
	}
}

func mustFloat(t *testing.T, actual *float64, expected float64) {
	t.Helper()
	if actual == nil || *actual != expected {
		t.Fatalf("actual = %v, expected %v", actual, expected)
	}
}

func mustDate(t *testing.T, actual *time.Time, expected string) {
	t.Helper()
	if actual == nil || actual.Format(time.DateOnly) != expected {
		t.Fatalf("actual = %v, expected %s", actual, expected)
	}
}

func hasReason(result Result, expected Reason) bool {
	for _, reason := range result.Reasons {
		if reason == expected {
			return true
		}
	}
	return false
}
