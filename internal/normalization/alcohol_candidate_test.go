package normalization

import "testing"

func TestCountryCatalog_현재MFDS국가를ISO코드와함께제공한다(t *testing.T) {
	if len(countryCatalog) != 61 {
		t.Fatalf("country catalog size = %d", len(countryCatalog))
	}
	seenAlpha2 := map[string]struct{}{}
	seenAlpha3 := map[string]struct{}{}
	for _, country := range countryCatalog {
		if country.NameKO == "" || country.NameEN == "" || len(country.Alpha2) != 2 || len(country.Alpha3) != 3 {
			t.Fatalf("invalid country = %+v", country)
		}
		if _, exists := seenAlpha2[country.Alpha2]; exists {
			t.Fatalf("duplicate alpha2 = %s", country.Alpha2)
		}
		if _, exists := seenAlpha3[country.Alpha3]; exists {
			t.Fatalf("duplicate alpha3 = %s", country.Alpha3)
		}
		seenAlpha2[country.Alpha2] = struct{}{}
		seenAlpha3[country.Alpha3] = struct{}{}
	}

	unitedKingdom, ok := lookupCountry("영국")
	if !ok || unitedKingdom.NameEN != "United Kingdom" || unitedKingdom.Alpha2 != "GB" || unitedKingdom.Alpha3 != "GBR" {
		t.Fatalf("country = %+v, ok = %v", unitedKingdom, ok)
	}
}

func TestNormalize_BottleNoteAlcohol필드_근거값만정제한다(t *testing.T) {
	result := Normalize(Input{
		ProductNameKO:             "맥캘란 12년 쉐리 캐스크 700ml (40%)",
		ProductNameEN:             "MACALLAN 12 YEARS SHERRY CASK 700ML 40%",
		ItemName:                  "위스키",
		OverseasEstablishmentName: "THE MACALLAN DISTILLERS LTD",
		ManufactureCountryName:    "영국",
		ExportCountryName:         "네덜란드",
	})

	if result.AlcoholNameKO != "맥캘란 12년 쉐리 캐스크" || result.AlcoholNameEN != "MACALLAN 12 YEARS SHERRY CASK" {
		t.Fatalf("alcohol names = %q / %q", result.AlcoholNameKO, result.AlcoholNameEN)
	}
	if result.AlcoholCategoryKO != "위스키" || result.AlcoholCategoryEN != "Whisky" {
		t.Fatalf("categories = %q / %q", result.AlcoholCategoryKO, result.AlcoholCategoryEN)
	}
	if result.AlcoholRegionKO != "영국" || result.AlcoholRegionEN != "United Kingdom" || result.AlcoholABV != "40%" {
		t.Fatalf("region/abv = %q / %q / %q", result.AlcoholRegionKO, result.AlcoholRegionEN, result.AlcoholABV)
	}
	if result.ManufactureCountry.Alpha2 != "GB" || result.ManufactureCountry.Alpha3 != "GBR" ||
		result.ExportCountry.Alpha2 != "NL" || result.ExportCountry.Alpha3 != "NLD" {
		t.Fatalf("countries = %+v / %+v", result.ManufactureCountry, result.ExportCountry)
	}
	if result.CaskCandidate != "SHERRY CASK" || result.DistilleryNameENCandidate != "" || result.DistilleryNameKOCandidate != "" {
		t.Fatalf("candidates = %q / %q / %q", result.CaskCandidate, result.DistilleryNameKOCandidate, result.DistilleryNameENCandidate)
	}
	if !hasReason(result, ReasonCaskTypeCandidateExtracted) || hasReason(result, ReasonOverseasEstablishmentDistilleryCandidate) {
		t.Fatalf("reasons = %v", result.Reasons)
	}
}

func TestNormalize_미등록국가코드를추측하지않는다(t *testing.T) {
	result := Normalize(Input{
		ProductNameKO:          "테스트 위스키 700ml",
		ItemName:               "위스키",
		ManufactureCountryName: "알수없는나라",
		ExportCountryName:      "미확인국가",
	})

	if result.ManufactureCountry != (Country{}) || result.ExportCountry != (Country{}) ||
		result.AlcoholRegionKO != "" || result.AlcoholRegionEN != "" {
		t.Fatalf("countries must remain empty: %+v / %+v", result.ManufactureCountry, result.ExportCountry)
	}
	if result.Status != StatusReviewRequired || !hasReason(result, ReasonManufactureCountryNotMapped) ||
		!hasReason(result, ReasonExportCountryNotMapped) {
		t.Fatalf("status/reasons = %s / %v", result.Status, result.Reasons)
	}
}

func TestNormalize_캐스크번호를캐스크종류후보로사용하지않는다(t *testing.T) {
	result := Normalize(Input{
		ProductNameKO: "글렌알라키 싱글캐스크 700ml (#9485)",
		ItemName:      "위스키",
	})

	if result.CaskNumber != "9485" || result.CaskCandidate != "" {
		t.Fatalf("cask number/candidate = %q / %q", result.CaskNumber, result.CaskCandidate)
	}
}
