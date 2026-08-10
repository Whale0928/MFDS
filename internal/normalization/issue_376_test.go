package normalization

import (
	"strings"
	"testing"
)

func TestNormalize_RCNO202600444849_스몰배치에디션번호를배치로오분류하지않는다(t *testing.T) {
	// Given
	input := Input{
		ProductNameKO: "벤로막 2003 스몰배치 에디션 #1 (700mL) (56%) (02/12/2025)",
		ProductNameEN: "BENROMACH 2023 SMALL BATCH EDITION #1",
	}

	// When
	result := Normalize(input)

	// Then
	if result.BatchNumber != "" {
		t.Fatalf("batch number = %q, want empty", result.BatchNumber)
	}
	if result.VariantMarkerType != VariantMarkerTypeEditionNumber || result.VariantMarkerRaw != "#1" || result.VariantMarkerValue != "1" {
		t.Fatalf("variant = %q/%q/%q, want %q/#1/1", result.VariantMarkerType, result.VariantMarkerRaw, result.VariantMarkerValue, VariantMarkerTypeEditionNumber)
	}
	if result.EditionName != "에디션 #1" {
		t.Fatalf("edition name = %q, want %q", result.EditionName, "에디션 #1")
	}
	if !strings.Contains(result.BaseProductNameKO, "스몰배치") || !strings.Contains(result.BaseProductNameEN, "SMALL BATCH") {
		t.Fatalf("small batch line was removed: ko=%q en=%q", result.BaseProductNameKO, result.BaseProductNameEN)
	}
}

func TestNormalize_변형마커_문맥별필드와원문을보존한다(t *testing.T) {
	// Given
	tests := []struct {
		name        string
		input       Input
		markerType  string
		markerRaw   string
		markerValue string
		cask        string
		batch       string
		review      bool
		keepInName  string
	}{
		{"캐스크 번호", Input{ProductNameKO: "싱글 캐스크 #9485 700ml"}, VariantMarkerTypeCaskNumber, "#9485", "9485", "9485", "", false, ""},
		{"배치 번호", Input{ProductNameKO: "브랜드 위스키 배치 #03 700ml", ProductNameEN: "BRAND WHISKY BATCH 3 700ml"}, VariantMarkerTypeBatchNumber, "#03", "03", "", "3", false, ""},
		{"에디션 번호", Input{ProductNameKO: "위스키 서울 에디션 No. 2 700ml"}, VariantMarkerTypeEditionNumber, "No. 2", "2", "", "", false, ""},
		{"시리즈 번호", Input{ProductNameKO: "위스키 @001 700ml"}, VariantMarkerTypeSeriesNumber, "@001", "001", "", "", false, "@001"},
		{"미확정 번호", Input{ProductNameKO: "위스키 #77 700ml"}, VariantMarkerTypeUnknown, "#77", "77", "", "", true, "#77"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// When
			result := Normalize(test.input)

			// Then
			if result.VariantMarkerType != test.markerType || result.VariantMarkerRaw != test.markerRaw || result.VariantMarkerValue != test.markerValue {
				t.Fatalf("variant = %q/%q/%q, want %q/%q/%q", result.VariantMarkerType, result.VariantMarkerRaw, result.VariantMarkerValue, test.markerType, test.markerRaw, test.markerValue)
			}
			if result.CaskNumber != test.cask || result.BatchNumber != test.batch {
				t.Fatalf("cask/batch = %q/%q, want %q/%q", result.CaskNumber, result.BatchNumber, test.cask, test.batch)
			}
			if (result.Status == StatusReviewRequired) != test.review {
				t.Fatalf("status = %s, review want %t, reasons=%v", result.Status, test.review, result.Reasons)
			}
			if test.keepInName != "" && (!strings.Contains(result.BaseProductNameKO, test.keepInName) || !strings.Contains(result.AlcoholNameKO, test.keepInName)) {
				t.Fatalf("unconfirmed marker was removed: base=%q alcohol=%q", result.BaseProductNameKO, result.AlcoholNameKO)
			}
		})
	}
}

func TestNormalize_미확정변형마커_SKU후보키를구분한다(t *testing.T) {
	// Given
	first := Input{ProductNameKO: "위스키 #77 700ml"}
	second := Input{ProductNameKO: "위스키 #78 700ml"}

	// When
	firstResult := Normalize(first)
	secondResult := Normalize(second)

	// Then
	if firstResult.SKUCandidateKeySHA256 == "" || secondResult.SKUCandidateKeySHA256 == "" || firstResult.SKUCandidateKeySHA256 == secondResult.SKUCandidateKeySHA256 {
		t.Fatalf("candidate keys must retain unknown marker: %q / %q", firstResult.SKUCandidateKeySHA256, secondResult.SKUCandidateKeySHA256)
	}
}

func TestNormalize_확정배치마커_표기차이만으로SKU후보키를나누지않는다(t *testing.T) {
	// Given
	marked := Input{ProductNameEN: "BRAND WHISKY BATCH #03 700ml"}
	plain := Input{ProductNameEN: "BRAND WHISKY BATCH 3 700ml"}

	// When
	markedResult := Normalize(marked)
	plainResult := Normalize(plain)

	// Then
	if markedResult.SKUCandidateKeySHA256 == "" || markedResult.SKUCandidateKeySHA256 != plainResult.SKUCandidateKeySHA256 {
		t.Fatalf("confirmed batch notation split candidate key: marked=%+v plain=%+v", markedResult, plainResult)
	}
}

func TestNormalize_CS_반대언어근거가있을때만도수유형으로확정한다(t *testing.T) {
	// Given
	confirmedInput := Input{ProductNameKO: "위스키 CS 700ml", ProductNameEN: "WHISKY CASK STRENGTH 700ml"}
	ambiguousInput := Input{ProductNameEN: "WHISKY CS 700ml"}
	typoInput := Input{ProductNameEN: "WHISKY CASK STRENGHT 700ml"}

	// When
	confirmed := Normalize(confirmedInput)
	ambiguous := Normalize(ambiguousInput)
	typo := Normalize(typoInput)

	// Then
	if confirmed.StrengthType != "CASK STRENGTH" || confirmed.VariantMarkerType != "" {
		t.Fatalf("confirmed = %+v", confirmed)
	}
	if ambiguous.StrengthType != "" || ambiguous.VariantMarkerType != VariantMarkerTypeStrengthAbbreviation || ambiguous.Status != StatusReviewRequired || !strings.Contains(ambiguous.BaseProductNameEN, "CS") {
		t.Fatalf("ambiguous = %+v", ambiguous)
	}
	if typo.StrengthType != "CASK STRENGTH" {
		t.Fatalf("typo alias = %+v", typo)
	}
}

func TestNormalize_미관측도수표기_새규칙으로해석하지않는다(t *testing.T) {
	// Given
	inputs := []Input{
		{ProductNameEN: "WHISKY FULL STRENGTH 700ml"},
		{ProductNameEN: "WHISKY N° 7 700ml"},
		{ProductNameEN: "WHISKY 33° 700ml"},
	}

	// When / Then
	for _, input := range inputs {
		result := Normalize(input)
		if result.StrengthType != "" || result.VariantMarkerType != "" {
			t.Fatalf("unsupported marker was structured: input=%q result=%+v", input.ProductNameEN, result)
		}
	}
}

func TestNormalize_Batch_숫자만추출하고실제값충돌만검토한다(t *testing.T) {
	// Given
	oneLanguage := Input{ProductNameEN: "BRAND WHISKY BATCH 12 700ml"}
	smallBatch := Input{ProductNameEN: "BRAND WHISKY SMALL BATCH 700ml"}
	conflict := Input{ProductNameKO: "브랜드 위스키 배치 2 700ml", ProductNameEN: "BRAND WHISKY BATCH 3 700ml"}

	// When
	one := Normalize(oneLanguage)
	small := Normalize(smallBatch)
	different := Normalize(conflict)

	// Then
	if one.BatchNumber != "12" || one.Status == StatusReviewRequired {
		t.Fatalf("one language = %+v", one)
	}
	if small.BatchNumber != "" || !strings.Contains(small.BaseProductNameEN, "SMALL BATCH") {
		t.Fatalf("small batch = %+v", small)
	}
	if different.BatchNumber != "2" || different.Status != StatusReviewRequired || !hasReason(different, ReasonBatchLanguageMismatch) {
		t.Fatalf("conflict = %+v", different)
	}
}

func TestNormalize_Edition_한언어값을보존하고숫자충돌만검토한다(t *testing.T) {
	// Given
	oneLanguage := Input{ProductNameEN: "MACALLAN LIMITED EDITION 700ml"}
	conflict := Input{ProductNameKO: "위스키 에디션 2 700ml", ProductNameEN: "WHISKY EDITION 3 700ml"}

	// When
	one := Normalize(oneLanguage)
	different := Normalize(conflict)

	// Then
	if one.EditionName != "LIMITED EDITION" || one.Status == StatusReviewRequired {
		t.Fatalf("one language = %+v", one)
	}
	if different.Status != StatusReviewRequired || !hasReason(different, ReasonEditionLanguageMismatch) {
		t.Fatalf("conflict = %+v", different)
	}
}

func TestNormalize_Vintage_브랜드숫자를제외하고가장오래된연도를고른다(t *testing.T) {
	// Given
	blocked := []Input{
		{ProductNameEN: "DON JULIO 1942 700ml"},
		{ProductNameEN: "OLD FORESTER 1920 700ml"},
		{ProductNameEN: "BALVENIE TUN 1509 700ml"},
	}
	multiple := Input{ProductNameEN: "VINTAGE 2008 RELEASE 2012 700ml"}

	// When / Then
	for _, input := range blocked {
		result := Normalize(input)
		if result.VintageYear != nil || result.VintageRaw != "" {
			t.Fatalf("brand number became vintage: input=%q result=%+v", input.ProductNameEN, result)
		}
	}
	result := Normalize(multiple)
	if result.VintageYear == nil || *result.VintageYear != 2008 || result.Status != StatusReviewRequired || !hasReason(result, ReasonVintageReviewRequired) {
		t.Fatalf("multiple vintage = %+v", result)
	}
}

func TestNormalize_성분퍼센트와도수를분리한다(t *testing.T) {
	// Given
	single := Input{ProductNameKO: "인삼향 리큐르 인삼0.45% 주도38% 700ml"}
	multiple := Input{ProductNameKO: "리큐르 천연향료1.03%, 과즙0.02% 700ml"}
	volumeAnchored := []Input{
		{ProductNameKO: "리큐르 (40%,700ml)"},
		{ProductNameEN: "LIQUEUR 40% 700ml"},
		{ProductNameEN: "40%VOL LIQUEUR 700ml"},
		{ProductNameKO: "브랜드 고량주 56도"},
	}

	// When
	singleResult := Normalize(single)
	multipleResult := Normalize(multiple)

	// Then
	mustFloat(t, singleResult.IngredientPercent, 0.45)
	mustFloat(t, singleResult.ABVPercent, 38)
	if singleResult.IngredientPercentRaw != "0.45%" {
		t.Fatalf("ingredient raw = %q", singleResult.IngredientPercentRaw)
	}
	if multipleResult.IngredientPercent != nil || multipleResult.IngredientPercentRaw != "1.03%, 0.02%" || multipleResult.Status != StatusReviewRequired {
		t.Fatalf("multiple ingredients = %+v", multipleResult)
	}
	for index, input := range volumeAnchored {
		expected := 40.0
		if index == len(volumeAnchored)-1 {
			expected = 56
		}
		mustFloat(t, Normalize(input).ABVPercent, expected)
	}
}

func TestNormalize_AlcoholName_용량과도수만제외하고표시명과컬럼은유지한다(t *testing.T) {
	// Given
	input := Input{ProductNameKO: "싱글몰트 위스키 700ml (40%)", ProductNameEN: "SINGLE MALT WHISKY 700ML 40%"}

	// When
	result := Normalize(input)

	// Then
	if strings.Contains(strings.ToLower(result.AlcoholNameKO), "700") || strings.Contains(result.AlcoholNameKO, "40%") || strings.Contains(strings.ToLower(result.AlcoholNameEN), "700") || strings.Contains(result.AlcoholNameEN, "40%") {
		t.Fatalf("alcohol names retained volume/abv: %q / %q", result.AlcoholNameKO, result.AlcoholNameEN)
	}
	if !strings.Contains(result.SKUDisplayNameKO, "700ml") || !strings.Contains(result.SKUDisplayNameKO, "40%") {
		t.Fatalf("display name lost volume/abv: %q", result.SKUDisplayNameKO)
	}
	mustInt(t, result.VolumeML, 700)
	mustFloat(t, result.ABVPercent, 40)
}

func TestNormalize_LOT와제조번호No숫자를변형마커와후보키에서제외한다(t *testing.T) {
	// Given
	tests := []struct {
		first, second Input
	}{
		{Input{ProductNameEN: "BRAND WHISKY LOT NO. 123 700ml"}, Input{ProductNameEN: "BRAND WHISKY LOT NO. 456 700ml"}},
		{Input{ProductNameKO: "브랜드 위스키 제조번호 No. 123 700ml"}, Input{ProductNameKO: "브랜드 위스키 제조번호 No. 456 700ml"}},
	}

	for _, test := range tests {
		// When
		first := Normalize(test.first)
		second := Normalize(test.second)

		// Then
		if first.VariantMarkerRaw != "" || second.VariantMarkerRaw != "" {
			t.Fatalf("labeled code became variant: first=%+v second=%+v", first, second)
		}
		if first.SKUCandidateKeySHA256 == "" || first.SKUCandidateKeySHA256 != second.SKUCandidateKeySHA256 {
			t.Fatalf("labeled number changed candidate hash: %q / %q", first.SKUCandidateKeySHA256, second.SKUCandidateKeySHA256)
		}
	}
}

func TestNormalize_Edition_뒤용량을번호로흡수하지않고실제번호는보존한다(t *testing.T) {
	// Given
	tests := []struct {
		input Input
		want  string
	}{
		{Input{ProductNameEN: "WHISKY EDITION 700ml"}, "EDITION"},
		{Input{ProductNameEN: "WHISKY EDITION 750ML"}, "EDITION"},
		{Input{ProductNameKO: "위스키 에디션 2 700ml"}, "에디션 2"},
		{Input{ProductNameKO: "위스키 에디션 #1 700ml"}, "에디션 #1"},
	}

	for _, test := range tests {
		// When
		result := Normalize(test.input)

		// Then
		if result.EditionName != test.want {
			t.Fatalf("edition = %q, want %q for %+v", result.EditionName, test.want, test.input)
		}
	}
}

func TestNormalize_Vintage_결정적범위밖브랜드숫자를차단하고실제연도는구조화한다(t *testing.T) {
	// Given
	blocked := []string{"1500", "1792", "1800", "1907", "1920", "1942", "2099"}

	// When / Then
	for _, number := range blocked {
		result := Normalize(Input{ProductNameEN: "BRAND " + number + " WHISKY 700ml"})
		if result.VintageYear != nil || result.VintageRaw != "" {
			t.Fatalf("blocked number became vintage: number=%s result=%+v", number, result)
		}
	}
	actual := Normalize(Input{ProductNameEN: "ISLAY VINTAGE 2010 WHISKY 700ml"})
	if actual.VintageYear == nil || *actual.VintageYear != 2010 || actual.VintageRaw != "2010" {
		t.Fatalf("actual vintage was not structured: %+v", actual)
	}
}

func TestNormalize_성분문맥이용량인접과타언어약한도수앵커보다우선한다(t *testing.T) {
	// Given
	nearVolume := Input{ProductNameKO: "인삼 리큐르 인삼0.45% 700ml"}
	crossLanguage := Input{ProductNameKO: "인삼 리큐르 인삼0.45%", ProductNameEN: "GINSENG LIQUEUR 0.45% 700ml"}

	// When / Then
	for _, input := range []Input{nearVolume, crossLanguage} {
		result := Normalize(input)
		mustFloat(t, result.IngredientPercent, 0.45)
		if result.ABVPercent != nil || result.ABVRaw != "" {
			t.Fatalf("ingredient was reconfirmed as ABV: input=%+v result=%+v", input, result)
		}
	}
}

func TestNormalize_확정도수만이름에서제거하고성분퍼센트는보존한다(t *testing.T) {
	// Given
	input := Input{ProductNameKO: "인삼 리큐르 인삼0.45% 700ml 주도38%"}
	volumeOrder := Input{ProductNameEN: "WHISKY 40% 700ml"}

	// When
	result := Normalize(input)
	ordered := Normalize(volumeOrder)

	// Then
	for _, name := range []string{result.BaseProductNameKO, result.AlcoholNameKO} {
		if !strings.Contains(name, "인삼0.45%") || strings.Contains(name, "38%") || strings.Contains(strings.ToLower(name), "700ml") {
			t.Fatalf("ingredient/ABV name boundary failed: %q", name)
		}
	}
	if ordered.BaseProductNameEN != "WHISKY" || ordered.AlcoholNameEN != "WHISKY" {
		t.Fatalf("NN%% volume order was not removed: base=%q alcohol=%q", ordered.BaseProductNameEN, ordered.AlcoholNameEN)
	}
}

func TestNormalize_CS와숫자마커가같이있을때숫자슬롯과각근거를보존한다(t *testing.T) {
	// Given
	input := Input{ProductNameEN: "WHISKY CS #77 700ml"}

	// When
	result := Normalize(input)

	// Then
	if result.VariantMarkerType != VariantMarkerTypeUnknown || result.VariantMarkerRaw != "#77" || result.VariantMarkerValue != "77" {
		t.Fatalf("variant slot was overwritten: %+v", result)
	}
	if !hasReason(result, ReasonVariantMarkerAmbiguous) || !hasReason(result, ReasonStrengthAbbreviationAmbiguous) {
		t.Fatalf("variant evidence was lost: reasons=%v fragments=%v", result.Reasons, result.UnparsedFragments)
	}
}

func TestNormalize_Series단어뒤숫자를시리즈마커로구조화한다(t *testing.T) {
	// Given / When / Then
	for _, input := range []Input{{ProductNameEN: "WHISKY SERIES 3 700ml"}, {ProductNameKO: "위스키 시리즈 3 700ml"}} {
		result := Normalize(input)
		if result.VariantMarkerType != VariantMarkerTypeSeriesNumber || result.VariantMarkerValue != "3" {
			t.Fatalf("series number was not structured: input=%+v result=%+v", input, result)
		}
	}
}

func TestNormalize_도수제거후의미괄호와기존구분자를보존한다(t *testing.T) {
	// Given
	input := Input{ProductNameEN: "WHISKY, RESERVE (SHERRY) / 40% 700ml"}

	// When
	result := Normalize(input)

	// Then
	if result.BaseProductNameEN != "WHISKY, RESERVE (SHERRY)" || result.AlcoholNameEN != "WHISKY, RESERVE (SHERRY)" {
		t.Fatalf("parentheses or separator was damaged: base=%q alcohol=%q", result.BaseProductNameEN, result.AlcoholNameEN)
	}
}
