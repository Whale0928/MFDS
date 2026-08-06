package normalization

import "strings"

type Country struct {
	NameKO string
	NameEN string
	Alpha2 string
	Alpha3 string
}

var countryCatalog = []Country{
	{NameKO: "가이아나", NameEN: "Guyana", Alpha2: "GY", Alpha3: "GUY"},
	{NameKO: "남아프리카 공화국", NameEN: "South Africa", Alpha2: "ZA", Alpha3: "ZAF"},
	{NameKO: "네덜란드", NameEN: "Netherlands", Alpha2: "NL", Alpha3: "NLD"},
	{NameKO: "네팔", NameEN: "Nepal", Alpha2: "NP", Alpha3: "NPL"},
	{NameKO: "노르웨이", NameEN: "Norway", Alpha2: "NO", Alpha3: "NOR"},
	{NameKO: "뉴질랜드", NameEN: "New Zealand", Alpha2: "NZ", Alpha3: "NZL"},
	{NameKO: "대만", NameEN: "Taiwan", Alpha2: "TW", Alpha3: "TWN"},
	{NameKO: "대한민국", NameEN: "South Korea", Alpha2: "KR", Alpha3: "KOR"},
	{NameKO: "덴마크", NameEN: "Denmark", Alpha2: "DK", Alpha3: "DNK"},
	{NameKO: "도미니카 공화국", NameEN: "Dominican Republic", Alpha2: "DO", Alpha3: "DOM"},
	{NameKO: "독일", NameEN: "Germany", Alpha2: "DE", Alpha3: "DEU"},
	{NameKO: "라트비아", NameEN: "Latvia", Alpha2: "LV", Alpha3: "LVA"},
	{NameKO: "러시아", NameEN: "Russia", Alpha2: "RU", Alpha3: "RUS"},
	{NameKO: "리투아니아", NameEN: "Lithuania", Alpha2: "LT", Alpha3: "LTU"},
	{NameKO: "멕시코", NameEN: "Mexico", Alpha2: "MX", Alpha3: "MEX"},
	{NameKO: "몬테네그로", NameEN: "Montenegro", Alpha2: "ME", Alpha3: "MNE"},
	{NameKO: "몽골", NameEN: "Mongolia", Alpha2: "MN", Alpha3: "MNG"},
	{NameKO: "미국", NameEN: "United States", Alpha2: "US", Alpha3: "USA"},
	{NameKO: "미얀마", NameEN: "Myanmar", Alpha2: "MM", Alpha3: "MMR"},
	{NameKO: "바베이도스", NameEN: "Barbados", Alpha2: "BB", Alpha3: "BRB"},
	{NameKO: "베트남", NameEN: "Vietnam", Alpha2: "VN", Alpha3: "VNM"},
	{NameKO: "벨기에", NameEN: "Belgium", Alpha2: "BE", Alpha3: "BEL"},
	{NameKO: "불가리아", NameEN: "Bulgaria", Alpha2: "BG", Alpha3: "BGR"},
	{NameKO: "브라질", NameEN: "Brazil", Alpha2: "BR", Alpha3: "BRA"},
	{NameKO: "스리랑카", NameEN: "Sri Lanka", Alpha2: "LK", Alpha3: "LKA"},
	{NameKO: "스웨덴", NameEN: "Sweden", Alpha2: "SE", Alpha3: "SWE"},
	{NameKO: "스페인", NameEN: "Spain", Alpha2: "ES", Alpha3: "ESP"},
	{NameKO: "싱가포르", NameEN: "Singapore", Alpha2: "SG", Alpha3: "SGP"},
	{NameKO: "아랍에미리트", NameEN: "United Arab Emirates", Alpha2: "AE", Alpha3: "ARE"},
	{NameKO: "아르메니아", NameEN: "Armenia", Alpha2: "AM", Alpha3: "ARM"},
	{NameKO: "아일랜드", NameEN: "Ireland", Alpha2: "IE", Alpha3: "IRL"},
	{NameKO: "에스토니아", NameEN: "Estonia", Alpha2: "EE", Alpha3: "EST"},
	{NameKO: "엘살바도르", NameEN: "El Salvador", Alpha2: "SV", Alpha3: "SLV"},
	{NameKO: "영국", NameEN: "United Kingdom", Alpha2: "GB", Alpha3: "GBR"},
	{NameKO: "오스트리아", NameEN: "Austria", Alpha2: "AT", Alpha3: "AUT"},
	{NameKO: "요르단", NameEN: "Jordan", Alpha2: "JO", Alpha3: "JOR"},
	{NameKO: "우즈베키스탄", NameEN: "Uzbekistan", Alpha2: "UZ", Alpha3: "UZB"},
	{NameKO: "우크라이나", NameEN: "Ukraine", Alpha2: "UA", Alpha3: "UKR"},
	{NameKO: "이탈리아", NameEN: "Italy", Alpha2: "IT", Alpha3: "ITA"},
	{NameKO: "인도", NameEN: "India", Alpha2: "IN", Alpha3: "IND"},
	{NameKO: "인도네시아", NameEN: "Indonesia", Alpha2: "ID", Alpha3: "IDN"},
	{NameKO: "일본", NameEN: "Japan", Alpha2: "JP", Alpha3: "JPN"},
	{NameKO: "자메이카", NameEN: "Jamaica", Alpha2: "JM", Alpha3: "JAM"},
	{NameKO: "조지아", NameEN: "Georgia", Alpha2: "GE", Alpha3: "GEO"},
	{NameKO: "중국", NameEN: "China", Alpha2: "CN", Alpha3: "CHN"},
	{NameKO: "체코", NameEN: "Czechia", Alpha2: "CZ", Alpha3: "CZE"},
	{NameKO: "칠레", NameEN: "Chile", Alpha2: "CL", Alpha3: "CHL"},
	{NameKO: "카자흐스탄", NameEN: "Kazakhstan", Alpha2: "KZ", Alpha3: "KAZ"},
	{NameKO: "캐나다", NameEN: "Canada", Alpha2: "CA", Alpha3: "CAN"},
	{NameKO: "쿠바", NameEN: "Cuba", Alpha2: "CU", Alpha3: "CUB"},
	{NameKO: "크로아티아", NameEN: "Croatia", Alpha2: "HR", Alpha3: "HRV"},
	{NameKO: "태국", NameEN: "Thailand", Alpha2: "TH", Alpha3: "THA"},
	{NameKO: "튀르키예", NameEN: "Türkiye", Alpha2: "TR", Alpha3: "TUR"},
	{NameKO: "트리니다드 토바고", NameEN: "Trinidad and Tobago", Alpha2: "TT", Alpha3: "TTO"},
	{NameKO: "파나마", NameEN: "Panama", Alpha2: "PA", Alpha3: "PAN"},
	{NameKO: "페루", NameEN: "Peru", Alpha2: "PE", Alpha3: "PER"},
	{NameKO: "폴란드", NameEN: "Poland", Alpha2: "PL", Alpha3: "POL"},
	{NameKO: "프랑스", NameEN: "France", Alpha2: "FR", Alpha3: "FRA"},
	{NameKO: "필리핀", NameEN: "Philippines", Alpha2: "PH", Alpha3: "PHL"},
	{NameKO: "호주", NameEN: "Australia", Alpha2: "AU", Alpha3: "AUS"},
	{NameKO: "홍콩", NameEN: "Hong Kong", Alpha2: "HK", Alpha3: "HKG"},
}

var countriesByKO = func() map[string]Country {
	result := make(map[string]Country, len(countryCatalog))
	for _, country := range countryCatalog {
		result[country.NameKO] = country
	}
	return result
}()

func lookupCountry(name string) (Country, bool) {
	country, ok := countriesByKO[strings.TrimSpace(name)]
	return country, ok
}
