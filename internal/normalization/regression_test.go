package normalization

import (
	"strings"
	"testing"
)

func TestNormalize_결함1_괄호짝불일치_원본이름을보존한다(t *testing.T) {
	// 규칙문서 5.1절: 괄호 전체를 일괄 제거하지 않는다 (line 112)
	// 규칙문서 4절: (, )는 제품 구분 정보를 포함할 수 있으므로 제거하지 않는다 (line 86-88)
	tests := []struct {
		name     string
		input    Input
		wantKO   string // expected SKUDisplayNameKO
		wantBase string // expected BaseProductNameKO
	}{
		{
			name:     "주도38% 괄호 보존",
			input:    Input{ProductNameKO: "동북삼보주(주도38%)"},
			wantKO:   "동북삼보주(주도38%)",
			wantBase: "동북삼보주",
		},
		{
			name:     "영문 괄호 짝 보존",
			input:    Input{ProductNameKO: "보트야드 더블 진 (BOATYARD DOUBLE GIN)"},
			wantKO:   "보트야드 더블 진 (BOATYARD DOUBLE GIN)",
			wantBase: "보트야드 더블 진 (BOATYARD DOUBLE GIN)",
		},
		{
			name:     "의미 괄호 가정용 보존",
			input:    Input{ProductNameKO: "골든블루 쿼츠 (가정용)"},
			wantKO:   "골든블루 쿼츠 (가정용)",
			wantBase: "골든블루 쿼츠 (가정용)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := Normalize(tt.input)
			if result.SKUDisplayNameKO != tt.wantKO {
				t.Errorf("SKUDisplayNameKO = %q, want %q", result.SKUDisplayNameKO, tt.wantKO)
			}
			if result.BaseProductNameKO != tt.wantBase {
				t.Errorf("BaseProductNameKO = %q, want %q", result.BaseProductNameKO, tt.wantBase)
			}
			assertBracketBalanced(t, result.BaseProductNameKO)
			assertBracketBalanced(t, result.SKUDisplayNameKO)
		})
	}
}

func TestNormalize_결함2_다중괄호_각괄호를개별처리한다(t *testing.T) {
	// 규칙문서 5.1절: 한 괄호 안에 값이 섞여 있으므로 내부 토큰 단위로 파싱한다 (line 126-132)
	// 규칙문서 5.1절: 괄호 전체를 일괄 제거하지 않는다 (line 112)
	tests := []struct {
		name    string
		input   Input
		wantVol *int
		wantABV *float64
		wantLot string
		wantAge *int
		wantKO  string
	}{
		{
			name:    "용량_ABV_LOT 괄호",
			input:   Input{ProductNameKO: "라벨 5 블렌디드 스카치 위스키 (700mL)(40%)(L514755B)"},
			wantVol: intPtr(700),
			wantABV: floatPtr(40),
			wantLot: "L514755B",
			wantKO:  "라벨 5 블렌디드 스카치 위스키 700ml 40%",
		},
		{
			name:    "용량_ABV_날짜 괄호",
			input:   Input{ProductNameKO: "벤로막 15년 (700mL) (43%) (07/11/2024)"},
			wantVol: intPtr(700),
			wantABV: floatPtr(43),
			wantAge: intPtr(15),
			wantKO:  "벤로막 15년 700ml 43%",
		},
		{
			name:   "중첩괄호_성분설명_보존",
			input:  Input{ProductNameKO: "푸쉬킨 너츠 & 누가 (천연향료(누가향1.03%,헤일즐넛향0.02%)함유)"},
			wantKO: "푸쉬킨 너츠 & 누가 (천연향료(누가향1.03%,헤일즐넛향0.02%)함유)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := Normalize(tt.input)
			if tt.wantVol != nil {
				if result.UnitVolumeML == nil || *result.UnitVolumeML != *tt.wantVol {
					t.Errorf("UnitVolumeML = %v, want %d", result.UnitVolumeML, *tt.wantVol)
				}
			}
			if tt.wantABV != nil {
				if result.ABVPercent == nil || *result.ABVPercent != *tt.wantABV {
					t.Errorf("ABVPercent = %v, want %v", result.ABVPercent, *tt.wantABV)
				}
			}
			if tt.wantLot != "" {
				if result.LotNumber != tt.wantLot {
					t.Errorf("LotNumber = %q, want %q", result.LotNumber, tt.wantLot)
				}
			}
			if tt.wantAge != nil {
				if result.AgeYears == nil || *result.AgeYears != *tt.wantAge {
					t.Errorf("AgeYears = %v, want %d", result.AgeYears, *tt.wantAge)
				}
			}
			if tt.wantKO != "" && result.SKUDisplayNameKO != tt.wantKO {
				t.Errorf("SKUDisplayNameKO = %q, want %q", result.SKUDisplayNameKO, tt.wantKO)
			}
			assertBracketBalanced(t, result.BaseProductNameKO)
			assertBracketBalanced(t, result.SKUDisplayNameKO)
			assertNoEmptyBrackets(t, result.BaseProductNameKO)
		})
	}
}

func TestNormalize_결함3_쉼표슬래시잔재_원본이름보존한다(t *testing.T) {
	// 규칙문서 5.5절: 슬래시 뒤 6~7자리 숫자는 자재코드 후보로 SKU 사양에 유지 (line 281)
	// 규칙문서 5.1절: 구조화한 토큰도 소스 이름에서는 제거하지 않는다 (line 110)
	// 규칙문서 5.5절: 괄호 안 6자리 숫자는 자재코드 후보로 유지 (line 280)
	tests := []struct {
		name       string
		input      Input
		wantVol    *int
		wantMat    string
		wantLot    string
		wantStatus Status
		wantKO     string
		wantBase   string
	}{
		{
			name:       "슬래시_쉼표_복합잔재",
			input:      Input{ProductNameKO: "리차드 헤네시 / 1107891, 1750mL, L5184"},
			wantVol:    intPtr(1750),
			wantMat:    "1107891",
			wantLot:    "L5184",
			wantStatus: StatusNormalized,
		},
		{
			name:       "복합괄호_용량_LOT_분리",
			input:      Input{ProductNameKO: "가쿠빈(1920ml, L A5B)"},
			wantVol:    intPtr(1920),
			wantLot:    "L A5B",
			wantStatus: StatusNormalized,
		},
		{
			name:       "용량_NC_잔재_검토대상",
			input:      Input{ProductNameKO: "라오왕 연태구냥(500ML) NC"},
			wantVol:    intPtr(500),
			wantStatus: StatusNormalized,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := Normalize(tt.input)
			if tt.wantVol != nil {
				if result.UnitVolumeML == nil || *result.UnitVolumeML != *tt.wantVol {
					t.Errorf("UnitVolumeML = %v, want %d", result.UnitVolumeML, *tt.wantVol)
				}
			}
			if tt.wantMat != "" {
				if result.MaterialCode != tt.wantMat {
					t.Errorf("MaterialCode = %q, want %q", result.MaterialCode, tt.wantMat)
				}
			}
			if tt.wantLot != "" {
				if result.LotNumber != tt.wantLot {
					t.Errorf("LotNumber = %q, want %q", result.LotNumber, tt.wantLot)
				}
			}
			if tt.wantStatus != "" && result.Status != tt.wantStatus {
				t.Errorf("Status = %s, want %s", result.Status, tt.wantStatus)
			}
			if tt.wantKO != "" && result.SKUDisplayNameKO != tt.wantKO {
				t.Errorf("SKUDisplayNameKO = %q, want %q", result.SKUDisplayNameKO, tt.wantKO)
			}
			if tt.wantBase != "" && result.BaseProductNameKO != tt.wantBase {
				t.Errorf("BaseProductNameKO = %q, want %q", result.BaseProductNameKO, tt.wantBase)
			}
			assertNoConsecutiveCommas(t, result.BaseProductNameKO)
			assertNoOrphanSeparators(t, result.BaseProductNameKO)
			assertNoOrphanSeparators(t, result.SKUDisplayNameKO)
		})
	}
}

func TestNormalize_결함4_LOT자재코드절단_전체코드를보존한다(t *testing.T) {
	// 규칙문서 5.5절: 슬래시 뒤 6~7자리 숫자는 자재코드 후보, SKU 사양에 유지 (line 281)
	// 규칙문서 5.5절: SMWS NNNNNNGX... 는 단일 캐스크 제품 식별 코드로 유지 (line 282)
	tests := []struct {
		name       string
		input      Input
		wantMat    string
		wantStatus Status
		wantKO     string
		wantBase   string
	}{
		{
			name:       "자재코드_GX_전체보존",
			input:      Input{ProductNameKO: "싱글 몰트 스카치 위스키 / 135066GX0700615"},
			wantMat:    "135066GX0700615",
			wantStatus: StatusNormalized,
			wantBase:   "싱글 몰트 스카치 위스키",
		},
		{
			name:       "자재코드_쉼표_용량_전체보존",
			input:      Input{ProductNameKO: "제임슨 스탠다드 / LFOX625200, 500mL"},
			wantMat:    "LFOX625200",
			wantStatus: StatusNormalized,
			wantBase:   "제임슨 스탠다드",
		},
		{
			name:       "자재코드_GX_전체보존2",
			input:      Input{ProductNameKO: "파이어 위드아웃 스모크 / 010277GX0700611"},
			wantMat:    "010277GX0700611",
			wantStatus: StatusNormalized,
			wantBase:   "파이어 위드아웃 스모크",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := Normalize(tt.input)
			if tt.wantMat != "" && result.MaterialCode != tt.wantMat {
				t.Errorf("MaterialCode = %q, want %q", result.MaterialCode, tt.wantMat)
			}
			if tt.wantStatus != "" && result.Status != tt.wantStatus {
				t.Errorf("Status = %s, want %s", result.Status, tt.wantStatus)
			}
			if tt.wantKO != "" && result.SKUDisplayNameKO != tt.wantKO {
				t.Errorf("SKUDisplayNameKO = %q, want %q", result.SKUDisplayNameKO, tt.wantKO)
			}
			if tt.wantBase != "" && result.BaseProductNameKO != tt.wantBase {
				t.Errorf("BaseProductNameKO = %q, want %q", result.BaseProductNameKO, tt.wantBase)
			}
			assertNoTruncatedCode(t, tt.input.ProductNameKO, result.BaseProductNameKO)
			assertNoTruncatedCode(t, tt.input.ProductNameKO, result.SKUDisplayNameKO)
		})
	}
}

func TestNormalize_필수회귀확장_추출값과이름을함께검증한다(t *testing.T) {
	// 규칙문서 10장 필수 회귀 사례 (line 737-758)
	// 기존 테스트가 추출값만 보고 이름 보존을 검증하지 않은 구멍을 메운다
	tests := []struct {
		name  string
		input Input
		check func(*testing.T, Result)
	}{
		{
			name:  "천단위_용량_BaseName검증",
			input: Input{ProductNameKO: "대용량 위스키 4,000ml"},
			check: func(t *testing.T, result Result) {
				mustInt(t, result.UnitVolumeML, 4000)
				if result.SKUDisplayNameKO != "대용량 위스키 4000ml" {
					t.Errorf("SKUDisplayNameKO = %q, want %q", result.SKUDisplayNameKO, "대용량 위스키 4000ml")
				}
				if result.BaseProductNameKO != "대용량 위스키" {
					t.Errorf("BaseProductNameKO = %q, want %q", result.BaseProductNameKO, "대용량 위스키")
				}
				if !hasReason(result, ReasonVolumeThousandsSeparator) {
					t.Errorf("missing ReasonVolumeThousandsSeparator in %v", result.Reasons)
				}
				assertBracketBalanced(t, result.BaseProductNameKO)
			},
		},
		{
			name:  "묶음_용량_BaseName검증",
			input: Input{ProductNameKO: "고량주 250ML*2"},
			check: func(t *testing.T, result Result) {
				mustInt(t, result.UnitVolumeML, 250)
				mustInt(t, result.PackageCount, 2)
				if result.SKUDisplayNameKO != "고량주 250ml × 2" {
					t.Errorf("SKUDisplayNameKO = %q", result.SKUDisplayNameKO)
				}
				if result.BaseProductNameKO != "고량주" {
					t.Errorf("BaseProductNameKO = %q, want '고량주'", result.BaseProductNameKO)
				}
			},
		},
		{
			name:  "성분_퍼센트_ABV아님_이름보존",
			input: Input{ProductNameKO: "설원 인삼송이주(인삼0.45%) 150ml"},
			check: func(t *testing.T, result Result) {
				if result.ABVPercent != nil {
					t.Errorf("ABVPercent must be nil, got %v", result.ABVPercent)
				}
				if !strings.Contains(result.SKUDisplayNameKO, "인삼송이주") {
					t.Errorf("SKUDisplayNameKO lost name: %q", result.SKUDisplayNameKO)
				}
				assertBracketBalanced(t, result.SKUDisplayNameKO)
			},
		},
		{
			name:  "100_percent_islay_ABV아님_이름보존",
			input: Input{ProductNameEN: "KILCHOMAN 100% ISLAY"},
			check: func(t *testing.T, result Result) {
				if result.ABVPercent != nil {
					t.Errorf("ABVPercent must be nil, got %v", result.ABVPercent)
				}
				if result.BaseProductNameEN != "KILCHOMAN 100% ISLAY" {
					t.Errorf("BaseProductNameEN = %q, want 'KILCHOMAN 100%% ISLAY'", result.BaseProductNameEN)
				}
			},
		},
		{
			name:  "슬래시_자재코드_BaseName_코드제거확인",
			input: Input{ProductNameKO: "싱글 몰트 스카치 위스키 / 135066GX0700615"},
			check: func(t *testing.T, result Result) {
				if result.MaterialCode != "135066GX0700615" {
					t.Errorf("MaterialCode = %q, want '135066GX0700615'", result.MaterialCode)
				}
				if !strings.Contains(result.BaseProductNameKO, "싱글 몰트") {
					t.Errorf("BaseProductNameKO lost base name: %q", result.BaseProductNameKO)
				}
				if strings.Contains(result.BaseProductNameKO, "135066G") {
					t.Errorf("BaseProductNameKO contains partial code: %q", result.BaseProductNameKO)
				}
			},
		},
		{
			name:  "괄호_자재코드_LOT_분리_이름보존",
			input: Input{ProductNameKO: "조니워커 700ml (778061) L5293CA009"},
			check: func(t *testing.T, result Result) {
				if result.MaterialCode != "778061" {
					t.Errorf("MaterialCode = %q, want '778061'", result.MaterialCode)
				}
				if result.LotNumber != "L5293CA009" {
					t.Errorf("LotNumber = %q, want 'L5293CA009'", result.LotNumber)
				}
				if result.BaseProductNameKO != "조니워커" {
					t.Errorf("BaseProductNameKO = %q, want '조니워커'", result.BaseProductNameKO)
				}
			},
		},
		{
			name:  "서울에디션_제거금지",
			input: Input{ProductNameKO: "위스키 (서울에디션)"},
			check: func(t *testing.T, result Result) {
				if !strings.Contains(result.SKUDisplayNameKO, "서울에디션") {
					t.Errorf("SKUDisplayNameKO lost edition: %q", result.SKUDisplayNameKO)
				}
				if result.EditionName == "" {
					t.Errorf("EditionName must be set")
				}
				assertBracketBalanced(t, result.SKUDisplayNameKO)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := Normalize(tt.input)
			tt.check(t, result)
		})
	}
}

func TestNormalize_결함5_추출률_대표케이스를검증한다(t *testing.T) {
	// 규칙문서 5.3절: 도수가 다른 후보는 이름과 용량이 같아도 자동 병합하지 않는다 (line 192)
	// 규칙문서 6.1절: 키1~3은 후보 탐색용이며 unique key가 아니다 (line 385)
	// 추출률 현황: 용량 19.5% / ABV 11.7% / 숙성 28.9% / vintage 0% / base 원본 무변화 47.8%
	tests := []struct {
		name    string
		input   Input
		wantVol bool
		wantABV bool
		wantAge bool
	}{
		{
			name:    "용량_미확인_일반명",
			input:   Input{ProductNameKO: "맥캘란 12년 쉐리 캐스크"},
			wantAge: true,
		},
		{
			name:    "기본명만_있음_용량미확인_검토대상",
			input:   Input{ProductNameKO: "글렌피딕"},
			wantVol: false,
			wantABV: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := Normalize(tt.input)
			if tt.wantVol {
				if result.UnitVolumeML == nil {
					t.Errorf("expected volume extraction, got nil")
				}
			}
			if tt.wantABV {
				if result.ABVPercent == nil {
					t.Errorf("expected ABV extraction, got nil")
				}
			}
			if tt.wantAge {
				if result.AgeYears == nil {
					t.Errorf("expected age extraction, got nil")
				}
			}
			if !tt.wantVol && result.UnitVolumeML != nil {
				t.Errorf("unexpected volume extraction: %d", *result.UnitVolumeML)
			}
			if !tt.wantABV && result.ABVPercent != nil {
				t.Errorf("unexpected ABV extraction: %v", *result.ABVPercent)
			}
		})
	}
}

func TestNormalize_불변식_괄호대괄호짝이항상맞는다(t *testing.T) {
	// 규칙문서 4절: (, )는 제품 구분 정보를 포함할 수 있으므로 제거하지 않는다 (line 86-88)
	inputs := []Input{
		{ProductNameKO: "동북삼보주(주도38%)"},
		{ProductNameKO: "보트야드 더블 진 (BOATYARD DOUBLE GIN)"},
		{ProductNameKO: "골든블루 쿼츠 (가정용)"},
		{ProductNameKO: "라벨 5 블렌디드 스카치 위스키 (700mL)(40%)(L514755B)"},
		{ProductNameKO: "벤로막 15년 (700mL) (43%) (07/11/2024)"},
		{ProductNameKO: "푸쉬킨 너츠 & 누가 (천연향료(누가향1.03%,헤일즐넛향0.02%)함유)"},
		{ProductNameKO: "위스키 (서울에디션)"},
		{ProductNameKO: "카발란 솔리스트 올로로소 쉐리캐스크 (51.6%)"},
		{ProductNameKO: "글렌알라키 싱글캐스크 (59.1%) (#9485)"},
		{ProductNameKO: "가쿠빈(1920ml, L A5B)"},
		{ProductNameKO: "조니워커 700ml (778061) L5293CA009"},
	}

	for _, input := range inputs {
		t.Run(input.ProductNameKO[:min(len(input.ProductNameKO), 40)], func(t *testing.T) {
			result := Normalize(input)
			assertBracketBalanced(t, result.BaseProductNameKO)
			assertBracketBalanced(t, result.SKUDisplayNameKO)
		})
	}
}

func TestNormalize_불변식_빈괄호_연속쉼표_고아구분자가남지않는다(t *testing.T) {
	// 규칙문서 5.1절: SKU 표시명에서는 확정된 기술 주석만 분리할 수 있다 (line 111)
	inputs := []Input{
		{ProductNameKO: "라벨 5 블렌디드 스카치 위스키 (700mL)(40%)(L514755B)"},
		{ProductNameKO: "리차드 헤네시 / 1107891, 1750mL, L5184"},
		{ProductNameKO: "가쿠빈(1920ml, L A5B)"},
		{ProductNameKO: "라오왕 연태구냥(500ML) NC"},
		{ProductNameKO: "동북삼보주(주도38%)"},
		{ProductNameKO: "푸쉬킨 너츠 & 누가 (천연향료(누가향1.03%,헤일즐넛향0.02%)함유)"},
	}

	for _, input := range inputs {
		t.Run(input.ProductNameKO[:min(len(input.ProductNameKO), 40)], func(t *testing.T) {
			result := Normalize(input)
			assertNoEmptyBrackets(t, result.BaseProductNameKO)
			assertNoEmptyBrackets(t, result.SKUDisplayNameKO)
			assertNoConsecutiveCommas(t, result.BaseProductNameKO)
			assertNoConsecutiveCommas(t, result.SKUDisplayNameKO)
			assertNoOrphanSeparators(t, result.BaseProductNameKO)
			assertNoOrphanSeparators(t, result.SKUDisplayNameKO)
		})
	}
}

func TestNormalize_불변식_영숫자코드토큰이중간에서잘리지않는다(t *testing.T) {
	// 규칙문서 5.5절: SMWS NNNNNNGX... 는 단일 캐스크 제품 식별 코드로 유지 (line 282)
	// 규칙문서 4절: #는 캐스크 번호, 배치 번호로 구분 정보 포함 (line 84)
	type codeCase struct {
		input     Input
		fullCodes []string // code fragments that must appear fully or not at all
	}
	cases := []codeCase{
		{
			input:     Input{ProductNameKO: "싱글 몰트 스카치 위스키 / 135066GX0700615"},
			fullCodes: []string{"135066GX0700615"},
		},
		{
			input:     Input{ProductNameKO: "제임슨 스탠다드 / LFOX625200, 500mL"},
			fullCodes: []string{"LFOX625200"},
		},
		{
			input:     Input{ProductNameKO: "파이어 위드아웃 스모크 / 010277GX0700611"},
			fullCodes: []string{"010277GX0700611"},
		},
		{
			input:     Input{ProductNameKO: "글렌알라키 싱글캐스크 (59.1%) (#9485)"},
			fullCodes: []string{"9485"},
		},
	}

	for _, c := range cases {
		t.Run(c.input.ProductNameKO[:min(len(c.input.ProductNameKO), 40)], func(t *testing.T) {
			result := Normalize(c.input)
			assertNoTruncatedCodeInNames(t, c.fullCodes, result.BaseProductNameKO)
			assertNoTruncatedCodeInNames(t, c.fullCodes, result.SKUDisplayNameKO)
		})
	}
}

func TestNormalize_불변식_NORMALIZED상태에잔재가남지않는다(t *testing.T) {
	// 규칙문서 7절: normalized는 안정된 규칙으로 구조화 완료된 상태 (line 480-481)
	// 규칙문서 5.5절: 괄호 안 6자리 숫자는 수입사 자재코드 후보로 SKU 사양에 유지 (line 280)
	inputs := []Input{
		{ProductNameKO: "싱글 몰트 스카치 위스키 / 135066GX0700615"},
		{ProductNameKO: "조니워커 700ml (778061) L5293CA009"},
		{ProductNameKO: "라오왕 연태구냥(500ML) NC"},
	}

	for _, input := range inputs {
		t.Run(input.ProductNameKO[:min(len(input.ProductNameKO), 40)], func(t *testing.T) {
			result := Normalize(input)
			if result.Status == StatusNormalized {
				// NORMALIZED 상태라면 SKU 표시명에 잔재 토큰이 없어야 함
				name := result.SKUDisplayNameKO
				if strings.Contains(name, "()") || strings.Contains(name, "[]") {
					t.Errorf("NORMALIZED but has empty brackets: %q", name)
				}
				if strings.Contains(name, ",,") {
					t.Errorf("NORMALIZED but has consecutive commas: %q", name)
				}
			}
		})
	}
}

// --- invariant helpers ---

func assertBracketBalanced(t *testing.T, value string) {
	t.Helper()
	if value == "" {
		return
	}
	depth := 0
	for _, ch := range value {
		switch ch {
		case '(':
			depth++
		case ')':
			depth--
		case '[':
			depth++
		case ']':
			depth--
		}
		if depth < 0 {
			t.Errorf("unmatched closing bracket in %q", value)
			return
		}
	}
	if depth != 0 {
		t.Errorf("unmatched opening bracket (depth=%d) in %q", depth, value)
	}
}

func assertNoEmptyBrackets(t *testing.T, value string) {
	t.Helper()
	if strings.Contains(value, "()") {
		t.Errorf("empty parentheses in %q", value)
	}
	if strings.Contains(value, "[]") {
		t.Errorf("empty square brackets in %q", value)
	}
}

func assertNoConsecutiveCommas(t *testing.T, value string) {
	t.Helper()
	if strings.Contains(value, ",,") {
		t.Errorf("consecutive commas in %q", value)
	}
}

func assertNoOrphanSeparators(t *testing.T, value string) {
	t.Helper()
	if strings.Contains(value, " / ") || strings.Contains(value, " , ") {
		t.Errorf("orphan separators in %q", value)
	}
}

// assertNoTruncatedCode checks that material codes in the original are either fully present or fully absent
func assertNoTruncatedCode(t *testing.T, original string, name string) {
	t.Helper()
	if name == "" || original == "" {
		return
	}
	for _, code := range findAlnumTokens(original) {
		if len(code) < 5 {
			continue
		}
		if strings.Contains(name, code) {
			continue // full code present - OK
		}
		for i := 5; i < len(code); i++ {
			partial := code[:i]
			if strings.Contains(name, partial) && !strings.HasPrefix(name+",", partial+",") {
				remaining := code[i:]
				if !strings.Contains(name, remaining) {
					t.Errorf("truncated code in %q: %q is partial of %q (missing %q)", name, partial, code, remaining)
					return
				}
			}
		}
	}
}

func assertNoTruncatedCodeInNames(t *testing.T, fullCodes []string, name string) {
	t.Helper()
	for _, code := range fullCodes {
		if strings.Contains(name, code) {
			continue
		}
		for i := 5; i < len(code); i++ {
			if strings.Contains(name, code[:i]) {
				t.Errorf("truncated code in %q: %q (prefix of %q)", name, code[:i], code)
				return
			}
		}
	}
}

func findAlnumTokens(value string) []string {
	var tokens []string
	current := ""
	for _, ch := range value {
		if (ch >= '0' && ch <= '9') || (ch >= 'A' && ch <= 'Z') || (ch >= 'a' && ch <= 'z') {
			current += string(ch)
		} else {
			if len(current) > 0 {
				tokens = append(tokens, current)
				current = ""
			}
		}
	}
	if len(current) > 0 {
		tokens = append(tokens, current)
	}
	return tokens
}

func intPtr(v int) *int           { return &v }
func floatPtr(v float64) *float64 { return &v }
