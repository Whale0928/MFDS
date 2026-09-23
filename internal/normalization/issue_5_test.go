package normalization

import (
	"strings"
	"testing"
)

// 이 파일의 원문은 2026.09.23 운영 원장 분석(B_normalization 2.1~2.4)에서 확인한 실제 RCNO 값이다.

func TestNormalize_B1_괄호와끝퍼센트는성분단서가있어도병도수로본다(t *testing.T) {
	tests := []struct {
		name string
		rcno string
		ko   string
		en   string
		abv  float64
	}{
		{"한글 괄호 도수와 싱글몰트", "202600183477", "카발란 솔리스트 비노 바리끄 싱글몰트 위스키 (54.8%)", "KAVALAN SOLIST VINHO BARRIQUE SINGLE MALT WHISKY", 54.8},
		{"영문 괄호 도수와 MALT", "202500683426", "탐두 시가몰트 IV", "TAMDHU CIGAR MALT (53.8%)", 53.8},
		{"영문 괄호 도수와 GRAIN", "202500857509", "틸링 싱글 그레인 / L24014", "TEELING SINGLE GRAIN (46%)", 46},
		{"한글 괄호 도수와 인삼", "202600427350", "인삼송이주(33%)", "GINSENG MATSUTAKE LIQUOR", 33},
		{"한글 끝 도수와 인삼", "202600360196", "통화인삼주 43.8%", "TONGHUARENSHENJIU", 43.8},
		{"리큐르 괄호 도수와 FRUIT", "202500730266", "디카이퍼 그레이프후르트 / L32825", "GRAPEFRUIT LIQUEUR(15%)", 15},
		{"영문 끝 도수와 MALT", "202500785216", "버니빌 싱글몰트 NO 19 40%", "BUNNYVILLE SINGLE MALT WHISKY NO 19 40% 670ML", 40},
	}
	for _, test := range tests {
		t.Run(test.rcno+" "+test.name, func(t *testing.T) {
			// When
			result := Normalize(Input{ProductNameKO: test.ko, ProductNameEN: test.en, ItemName: "위스키"})

			// Then
			if result.ABVPercent == nil || *result.ABVPercent != test.abv {
				t.Fatalf("abv = %v, want %v (reasons=%v)", result.ABVPercent, test.abv, result.Reasons)
			}
			if result.IngredientPercent != nil || result.IngredientPercentRaw != "" {
				t.Fatalf("ingredient = %v/%q, want empty", result.IngredientPercent, result.IngredientPercentRaw)
			}
			if strings.Contains(result.BaseProductNameKO, "%") || strings.Contains(result.BaseProductNameEN, "%") {
				t.Fatalf("confirmed ABV left in base name: %q / %q", result.BaseProductNameKO, result.BaseProductNameEN)
			}
		})
	}
}

func TestNormalize_B1_함량서술문맥은성분으로유지한다(t *testing.T) {
	tests := []struct {
		name       string
		rcno       string
		ko         string
		en         string
		ingredient float64
		status     Status
	}{
		{"함유 서술", "202600070154", "유연고량주(수수43%함유) (500ml)", "YOUYUAN", 43, StatusNormalized},
		{"100% 원재료", "202600328936", "소비에스키 100% 호밀 보드카", "SOBIESKI 100% RYE VODKA", 100, StatusNormalized},
		{"향료 함유", "202500784566", "오렌지 도르(향료(천연오렌지향) 10.99% 함유)", "ORANGE D'OR", 10.99, StatusNormalized},
		{"향 바로 뒤 함량", "202600144937", "클라이너 파이글링 레드베리 사우어 [천연크랜베리향 0.1%] [20ML]", "KLEINER FEIGLING RED BERRY SOUR", 0.1, StatusNormalized},
		{"추출물과 고형분함량", "202500557732", "페로네 리몬첼로(레몬추출물2%(고형분함량2%))", "FERONE LIMONCELLO", 2, StatusNormalized},
	}
	for _, test := range tests {
		t.Run(test.rcno+" "+test.name, func(t *testing.T) {
			// When
			result := Normalize(Input{ProductNameKO: test.ko, ProductNameEN: test.en, ItemName: "리큐르"})

			// Then
			if result.ABVPercent != nil {
				t.Fatalf("abv = %v, want nil", *result.ABVPercent)
			}
			if result.IngredientPercent == nil || *result.IngredientPercent != test.ingredient {
				t.Fatalf("ingredient = %v, want %v", result.IngredientPercent, test.ingredient)
			}
			if result.Status != test.status {
				t.Fatalf("status = %s, want %s (reasons=%v)", result.Status, test.status, result.Reasons)
			}
		})
	}
}

func TestNormalize_B1_함량서술이아닌20퍼센트초과성분값은검토사유를붙인다(t *testing.T) {
	// When: 품목명 바로 뒤 퍼센트는 성분이지만 함유·100% 원재료 서술이 아니어서 병 도수일 수 있다.
	result := Normalize(Input{ProductNameKO: "인삼주 인삼 25%", ProductNameEN: "GINSENG LIQUOR", ItemName: "일반증류주"})

	// Then
	if result.IngredientPercent == nil || *result.IngredientPercent != 25 {
		t.Fatalf("ingredient = %v, want 25", result.IngredientPercent)
	}
	if result.Status != StatusReviewRequired || !hasReason(result, ReasonIngredientPercentHigh) {
		t.Fatalf("status = %s, reasons = %v", result.Status, result.Reasons)
	}
	statement := Normalize(Input{ProductNameKO: "유연고량주(수수43%함유) (500ml)", ProductNameEN: "YOUYUAN", ItemName: "일반증류주"})
	if hasReason(statement, ReasonIngredientPercentHigh) {
		t.Fatalf("함유 statement got high reason: %v", statement.Reasons)
	}
	low := Normalize(Input{ProductNameKO: "클라이너 파이글링 레드베리 사우어 [천연크랜베리향 0.1%] [20ML]", ProductNameEN: "KLEINER FEIGLING RED BERRY SOUR", ItemName: "리큐르"})
	if hasReason(low, ReasonIngredientPercentHigh) {
		t.Fatalf("low ingredient got high reason: %v", low.Reasons)
	}
}

func TestNormalize_B1_설원인삼송이주_복수성분과영문도수를분리한다(t *testing.T) {
	// When
	result := Normalize(Input{ProductNameKO: "설원 인삼송이주(인삼0.45%, 송이0.1%) 150ml", ProductNameEN: "LOW LIQUOR 42%"})

	// Then
	if result.ABVPercent == nil || *result.ABVPercent != 42 {
		t.Fatalf("abv = %v, want 42", result.ABVPercent)
	}
	if result.IngredientPercent != nil || result.IngredientPercentRaw != "0.45%, 0.1%" || !hasReason(result, ReasonIngredientPercentMultiple) {
		t.Fatalf("ingredient = %v/%q reasons=%v", result.IngredientPercent, result.IngredientPercentRaw, result.Reasons)
	}
}

func TestNormalize_B2_문장가운데한글도표기를도수로인정한다(t *testing.T) {
	tests := []struct {
		rcno string
		ko   string
		en   string
		abv  float64
	}{
		{"202500662781", "56도 프리미엄금문고량주(750ML)", "PREMIUM KINMEN KAOLIANG LIQUOR", 56},
		{"202500805587", "북경이과도주(56도)", "북경이과두주", 56},
		{"202500559138", "우란산천량 36도 500ML", "NIU LAN SHAN CHEN NIANG JIU", 36},
		{"202500666573", "촉한삼국 35도 농향형 고체법 백주", "NONGXIANGXING CHINESE BAIJIU", 35},
		{"202500542372", "북경이과두주 56도(5000ML)", "BEIJING ERGUOTOU JIU 56%", 56},
	}
	for _, test := range tests {
		t.Run(test.rcno, func(t *testing.T) {
			// When
			result := Normalize(Input{ProductNameKO: test.ko, ProductNameEN: test.en, ItemName: "일반증류주"})

			// Then
			if result.ABVPercent == nil || *result.ABVPercent != test.abv {
				t.Fatalf("abv = %v, want %v (reasons=%v)", result.ABVPercent, test.abv, result.Reasons)
			}
			if result.Status != StatusNormalized {
				t.Fatalf("status = %s, reasons = %v", result.Status, result.Reasons)
			}
			if strings.Contains(result.BaseProductNameKO, "도 ") || strings.HasSuffix(result.BaseProductNameKO, "도") {
				t.Fatalf("degree token left in base name: %q", result.BaseProductNameKO)
			}
		})
	}
}

func TestNormalize_B2_도뒤에글자가이어지면도수로보지않는다(t *testing.T) {
	// When
	result := Normalize(Input{ProductNameKO: "56도라지 술", ProductNameEN: "DORAJI"})

	// Then
	if result.ABVPercent != nil {
		t.Fatalf("abv = %v, want nil", *result.ABVPercent)
	}
}

func TestNormalize_B3_AGED_n_YEARS는YEARS까지이름에서제거한다(t *testing.T) {
	tests := []struct {
		rcno   string
		ko, en string
		base   string
		age    int
	}{
		{"202500850665", "잉글리쉬 위스키 올로로소 쉐리케스크 머츄어드 15년", "THE ENGLISH OLOROSO CASK MATURED AGED 15 YEARS", "THE ENGLISH OLOROSO CASK MATURED", 15},
		{"202600564111", "더글렌터렛 12년 2024", "THE GLENTURRET AGED 12 YEARS 2024", "THE GLENTURRET 2024", 12},
	}
	for _, test := range tests {
		t.Run(test.rcno, func(t *testing.T) {
			// When
			result := Normalize(Input{ProductNameKO: test.ko, ProductNameEN: test.en})

			// Then
			if result.AgeYears == nil || *result.AgeYears != test.age {
				t.Fatalf("age = %v, want %d", result.AgeYears, test.age)
			}
			if result.BaseProductNameEN != test.base {
				t.Fatalf("base en = %q, want %q", result.BaseProductNameEN, test.base)
			}
		})
	}
}

func TestNormalize_B4_주년숫자는숙성연수로쓰지않는다(t *testing.T) {
	// When
	result := Normalize(Input{ProductNameKO: "헤네시 브이 에스 260주년 에디션 / 750mL, 1116586, L5260", ProductNameEN: "HENNESSY VS 260YEARS"})

	// Then
	if result.AgeYears != nil {
		t.Fatalf("age = %d, want nil", *result.AgeYears)
	}
	if !strings.Contains(result.BaseProductNameEN, "260YEARS") {
		t.Fatalf("anniversary token removed from base en: %q", result.BaseProductNameEN)
	}
}

func TestNormalize_B4_주년과숙성이함께있으면숙성만추출한다(t *testing.T) {
	tests := []struct {
		rcno   string
		ko, en string
		age    int
	}{
		{"202500652793", "글렌리벳 200주년", "GLENLIVET 12YEAR OLD FIRST FILL AMERICAN OAK 200YEAR ANNIVERSARY EDITION", 12},
		{"202600382829", "더 글로버 바이 아델피 7년 10주년 기념 한정판", "THE GLOVER BY ADELPHI 7 YEARS OLD 10TH ANNIVERSARY EDITION", 7},
	}
	for _, test := range tests {
		t.Run(test.rcno, func(t *testing.T) {
			// When
			result := Normalize(Input{ProductNameKO: test.ko, ProductNameEN: test.en})

			// Then
			if result.AgeYears == nil || *result.AgeYears != test.age {
				t.Fatalf("age = %v, want %d (reasons=%v)", result.AgeYears, test.age, result.Reasons)
			}
		})
	}
}

func TestProductIdentityKey_수입사와용량은무시하고연수는구분한다(t *testing.T) {
	// Given
	first := Normalize(Input{ProductNameKO: "조니워커 블랙 12년", ProductNameEN: "JOHNNIE WALKER BLACK LABEL 12YO", ImporterName: "수입사 A"})
	second := Normalize(Input{ProductNameKO: "조니워커 블랙 12년", ProductNameEN: "JOHNNIE WALKER BLACK LABEL 12YO", ImporterName: "수입사 B"})
	bottled := Normalize(Input{ProductNameKO: "조니워커 블랙 12년 500ml", ProductNameEN: "JOHNNIE WALKER BLACK LABEL 12YO"})
	larger := Normalize(Input{ProductNameKO: "조니워커 블랙 12년 750ml", ProductNameEN: "JOHNNIE WALKER BLACK LABEL 12YO"})
	older := Normalize(Input{ProductNameKO: "조니워커 블랙 18년", ProductNameEN: "JOHNNIE WALKER BLACK LABEL 18YO"})

	// Then
	if first.ProductIdentityKeySHA256 == "" || first.ProductIdentityKeySHA256 != second.ProductIdentityKeySHA256 {
		t.Fatalf("importers must not split identity: %q / %q", first.ProductIdentityKeySHA256, second.ProductIdentityKeySHA256)
	}
	if bottled.ProductIdentityKeySHA256 != first.ProductIdentityKeySHA256 || larger.ProductIdentityKeySHA256 != first.ProductIdentityKeySHA256 {
		t.Fatalf("volume must not split identity: %q / %q / %q", first.ProductIdentityKeySHA256, bottled.ProductIdentityKeySHA256, larger.ProductIdentityKeySHA256)
	}
	if first.ProductIdentityKeySHA256 == older.ProductIdentityKeySHA256 {
		t.Fatal("age difference produced the same identity key")
	}
}

func TestProductIdentityKey_도수와캐스크스트렝스는구분한다(t *testing.T) {
	// Given
	regular := Normalize(Input{ProductNameKO: "부나하벤 12년", ProductNameEN: "BUNNAHABHAIN 12YO"})
	cask := Normalize(Input{ProductNameKO: "부나하벤 12년 캐스크 스트렝스", ProductNameEN: "BUNNAHABHAIN 12YO CASK STRENGTH"})
	withABV := Normalize(Input{ProductNameKO: "부나하벤 12년 (46.3%)", ProductNameEN: "BUNNAHABHAIN 12YO"})

	// Then
	if cask.StrengthType == "" {
		t.Fatalf("strength type not parsed: %+v", cask)
	}
	if regular.ProductIdentityKeySHA256 == "" || regular.ProductIdentityKeySHA256 == cask.ProductIdentityKeySHA256 {
		t.Fatalf("cask strength merged with regular bottling: %q / %q", regular.ProductIdentityKeySHA256, cask.ProductIdentityKeySHA256)
	}
	if regular.ProductIdentityKeySHA256 == withABV.ProductIdentityKeySHA256 {
		t.Fatal("ABV difference produced the same identity key")
	}
}

func TestProductIdentityKey_한쪽검색키가없으면키를만들지않는다(t *testing.T) {
	// When
	result := Normalize(Input{ProductNameKO: "조니워커 블랙 12년"})

	// Then
	if result.ProductIdentityKeySHA256 != "" {
		t.Fatalf("identity key = %q, want empty", result.ProductIdentityKeySHA256)
	}
}

func TestProductIdentityKey_저장값으로다시계산해도같다(t *testing.T) {
	// Given: DECIMAL(6,3) 컬럼은 소수 셋째 자리까지만 돌려준다.
	result := Normalize(Input{ProductNameKO: "카발란 솔리스트 비노 바리끄 싱글몰트 위스키 (54.8%)", ProductNameEN: "KAVALAN SOLIST VINHO BARRIQUE SINGLE MALT WHISKY"})
	stored := IdentityFromResult(result)
	abv := 54.8000000001
	stored.ABVPercent = &abv

	// When
	key := ProductIdentityKey(stored)

	// Then
	if key != result.ProductIdentityKeySHA256 {
		t.Fatalf("stored key = %q, normalized key = %q", key, result.ProductIdentityKeySHA256)
	}
}
