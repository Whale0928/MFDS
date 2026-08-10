package dashboard

// detailColumn은 상세 화면에 노출하는 값 하나를 정의한다.
// Expr은 원장 View와 declarations의 v3 컬럼에서 읽을 SQL 표현식이고 Hint는 화면 각주로 그대로 쓴다.
type detailColumn struct {
	Group string
	Expr  string
	Label string
	Hint  string
}

// 상세 화면은 원장과 정제 결과를 좌우로 나눠 따로 스크롤한다.
// 식약처가 준 값은 ledger, 우리가 만들어낸 값은 normalized 쪽에 놓는다.
func groupSide(group string) string {
	switch group {
	case "신고 정보", "원장 - 수집한 그대로":
		return "ledger"
	default:
		return "normalized"
	}
}

// 노출 대상은 사람이 읽는 업무 값으로 한정한다.
// 원본 HTML, 행 해시, 수집 job/task/fetch 식별자, 수집 URL, 점유(claim) 상태는 반환하지 않는다.
var detailColumns = []detailColumn{
	{"신고 정보", "rcno", "수입신고번호", "식약처가 수입신고 건마다 부여하는 번호입니다. 이 화면의 한 행이 이 번호 하나에 해당합니다."},
	{"신고 정보", "COALESCE(DATE_FORMAT(source_processed_date, '%Y-%m-%d'), '')", "처리일", "식약처가 이 신고 건을 처리한 날짜입니다."},
	{"신고 정보", "COALESCE(source_processed_date_raw, '')", "처리일 원문", "식약처 화면에 적혀 있던 날짜 문구 그대로입니다. 형식이 일정하지 않을 수 있습니다."},
	{"신고 정보", "COALESCE(source_queried_item_name, '')", "조회 품목", "이 건을 수집할 때 사용한 품목 분류입니다. 위스키, 브랜디처럼 검색 조건으로 쓴 값입니다."},
	{"신고 정보", "COALESCE(source_product_division_name, '')", "식품 구분", "식약처가 분류한 식품 유형입니다."},
	{"신고 정보", "COALESCE(source_item_name, '')", "식약처 품목명", "식약처가 기록한 품목 이름입니다. 제품명과 다를 수 있습니다."},

	{"원장 - 수집한 그대로", "COALESCE(source_product_name_ko, '')", "제품명 (한글) 원문", "식약처 원장에 적힌 한글 제품명입니다. 손대지 않은 원본이며 용량, 도수, 관리번호가 섞여 있습니다."},
	{"원장 - 수집한 그대로", "COALESCE(source_product_name_en, '')", "제품명 (영문) 원문", "식약처 원장에 적힌 영문 제품명입니다. 손대지 않은 원본입니다."},
	{"원장 - 수집한 그대로", "COALESCE(source_importer_name, '')", "수입사 원문", "식약처 원장에 적힌 수입사 이름입니다. 주식회사 표기 등이 제각각입니다."},
	{"원장 - 수집한 그대로", "COALESCE(source_overseas_establishment_name, '')", "해외 제조업소 원문", "해외에서 이 제품을 만든 시설의 이름입니다. 증류소 이름인 경우가 많습니다."},
	{"원장 - 수집한 그대로", "COALESCE(source_manufacture_country_name, '')", "제조 국가 원문", "제품을 만든 나라입니다. 식약처 표기 그대로입니다."},
	{"원장 - 수집한 그대로", "COALESCE(source_export_country_name, '')", "수출 국가 원문", "제품을 실어 보낸 나라입니다. 제조 국가와 다를 수 있습니다."},
	{"원장 - 수집한 그대로", "COALESCE(source_expiry_text, '')", "소비기한 원문", "식약처 원장에 적힌 소비기한 문구 그대로입니다."},

	{"제품명 정제 결과", "COALESCE(base_product_name_ko, '')", "제품 이름 (한글)", "원문에서 용량, 도수, 관리번호를 떼어내고 남긴 순수한 제품 이름입니다."},
	{"제품명 정제 결과", "COALESCE(base_product_name_en, '')", "제품 이름 (영문)", "영문 원문에서 같은 방식으로 뽑아낸 제품 이름입니다."},
	{"제품명 정제 결과", "COALESCE(sku_display_name_ko, '')", "표시용 전체 이름 (한글)", "제품 이름에 용량과 도수를 다시 붙여 사람이 읽기 좋게 만든 이름입니다."},
	{"제품명 정제 결과", "COALESCE(sku_display_name_en, '')", "표시용 전체 이름 (영문)", "영문 기준으로 만든 표시용 이름입니다."},
	{"제품명 정제 결과", "COALESCE(name_search_key_ko, '')", "검색용 이름 (한글)", "띄어쓰기와 기호를 없애 검색·비교에만 쓰는 형태입니다. 사람에게 보여줄 이름이 아닙니다."},
	{"제품명 정제 결과", "COALESCE(name_search_key_en, '')", "검색용 이름 (영문)", "영문 기준 검색용 형태입니다."},

	{"용량과 도수", "COALESCE(volume_raw, '')", "용량 원문", "제품명 안에서 용량으로 판단한 글자 그대로입니다."},
	{"용량과 도수", "COALESCE(CAST(volume_ml AS CHAR), '')", "총 용량 (mL)", "포장 전체 용량을 mL로 바꾼 값입니다. 6병 세트라면 6병을 합한 양입니다."},
	{"용량과 도수", "COALESCE(CAST(unit_volume_ml AS CHAR), '')", "병 하나 용량 (mL)", "병 한 개의 용량입니다. 같은 제품인지 비교할 때 이 값을 씁니다."},
	{"용량과 도수", "COALESCE(CAST(package_count AS CHAR), '')", "포장 수량", "한 신고 건에 묶인 병 개수입니다. 6병 세트면 6입니다."},
	{"용량과 도수", "COALESCE(abv_raw, '')", "도수 원문", "제품명 안에서 알코올 도수로 판단한 글자 그대로입니다."},
	{"용량과 도수", "COALESCE(CAST(abv_percent AS CHAR), '')", "알코올 도수 (%)", "전체 부피 대비 알코올 비율입니다. 40이면 40도입니다."},
	{"용량과 도수", "COALESCE(declaration_v3.ingredient_percent_raw, '')", "성분 비율 원문", "성분 함량으로 판단한 퍼센트 표기를 원문 그대로 보존한 값입니다."},
	{"용량과 도수", "COALESCE(CAST(declaration_v3.ingredient_percent AS CHAR), '')", "성분 비율 (%)", "한 가지 성분 비율만 명확할 때 구조화한 값입니다. 여러 값이면 원문만 보존합니다."},
	{"용량과 도수", "COALESCE(proof_raw, '')", "프루프 원문", "미국식 도수 표기(proof)를 발견한 그대로입니다."},
	{"용량과 도수", "COALESCE(CAST(proof_value AS CHAR), '')", "프루프 값", "미국식 도수 단위입니다. 도수의 두 배가 프루프입니다. 80 프루프는 40도입니다."},
	{"용량과 도수", "COALESCE(strength_type, '')", "도수 유형", "캐스크 스트렝스처럼 도수와 관련된 제품 특성 표기입니다."},

	{"제품 특성", "COALESCE(age_raw, '')", "숙성 원문", "제품명 안에서 숙성 기간으로 판단한 글자 그대로입니다."},
	{"제품 특성", "COALESCE(CAST(age_years AS CHAR), '')", "숙성 연수", "통에서 숙성한 햇수입니다. 12면 12년산입니다."},
	{"제품 특성", "COALESCE(vintage_raw, '')", "빈티지 원문", "제품명 안에서 생산 연도로 판단한 글자 그대로입니다."},
	{"제품 특성", "COALESCE(CAST(vintage_year AS CHAR), '')", "빈티지 연도", "원액을 만든 해입니다. 숙성 연수와 다른 개념입니다."},
	{"제품 특성", "COALESCE(edition_name, '')", "에디션", "한정판이나 특별판 이름입니다."},
	{"제품 특성", "COALESCE(version_marker, '')", "버전 표기", "같은 제품의 몇 번째 출시인지 나타내는 표기입니다."},
	{"제품 특성", "COALESCE(declaration_v3.variant_marker_raw, '')", "변형 마커 원문", "#, @, No. 또는 CS처럼 제품 변형을 나타낼 수 있는 표기를 원문 그대로 보존합니다."},
	{"제품 특성", "COALESCE(declaration_v3.variant_marker_type, '')", "변형 마커 유형", "캐스크, 배치, 에디션, 시리즈 또는 미확정 문맥으로 분류한 값입니다."},
	{"제품 특성", "COALESCE(declaration_v3.variant_marker_value, '')", "변형 마커 값", "변형 마커에서 분리한 번호나 약어입니다."},
	{"제품 특성", "COALESCE(cask_number, '')", "캐스크 번호", "숙성에 쓴 통의 개별 번호입니다. 이 번호가 다르면 다른 병입입니다."},
	{"제품 특성", "COALESCE(cask_candidate, '')", "캐스크 종류 후보", "셰리, 버번처럼 숙성통 종류로 보이는 표기입니다. 확정이 아니라 후보입니다."},

	{"관리 번호", "COALESCE(lot_number, '')", "LOT 번호", "제조사가 생산 묶음마다 붙이는 번호입니다. 같은 제품이라도 생산 시점이 다르면 달라집니다."},
	{"관리 번호", "COALESCE(manufacture_number, '')", "제조번호", "제조사의 생산 관리 번호입니다."},
	{"관리 번호", "COALESCE(batch_number, '')", "배치 번호", "한 번에 만든 생산 단위의 번호입니다."},
	{"관리 번호", "COALESCE(material_code, '')", "자재 코드", "수입사나 제조사의 내부 품목 코드입니다. 제품 이름이 아닙니다."},
	{"관리 번호", "COALESCE(expiry_raw, '')", "소비기한 정제 원문", "소비기한으로 판단한 글자 그대로입니다."},
	{"관리 번호", "COALESCE(CAST(expiry_start AS CHAR), '')", "소비기한 시작", "소비기한 범위의 시작일입니다."},
	{"관리 번호", "COALESCE(CAST(expiry_end AS CHAR), '')", "소비기한 종료", "소비기한 범위의 종료일입니다."},

	{"수입사와 제조사", "COALESCE(importer_base_name, '')", "수입사 정제명", "주식회사, (주) 같은 표기를 정리한 수입사 이름입니다."},
	{"수입사와 제조사", "COALESCE(importer_search_key, '')", "수입사 검색용 이름", "같은 수입사를 찾기 위한 비교 전용 형태입니다."},
	{"수입사와 제조사", "COALESCE(legal_entity_type, '')", "법인 형태", "주식회사, 유한회사처럼 원문에서 떼어낸 법인 표기입니다."},
	{"수입사와 제조사", "COALESCE(overseas_establishment_search_key, '')", "해외 제조업소 검색용 이름", "해외 제조업소를 비교하기 위한 전용 형태입니다."},
	{"수입사와 제조사", "COALESCE(distillery_name_ko_candidate, '')", "증류소 후보 (한글)", "제품명과 향후 증류소 사전을 대조해 확인할 후보입니다. 현재는 자동 생성하지 않습니다."},
	{"수입사와 제조사", "COALESCE(distillery_name_en_candidate, '')", "증류소 후보 (영문)", "제품명과 향후 증류소 사전을 대조해 확인할 영문 후보입니다. 현재는 자동 생성하지 않습니다."},

	{"주류 분류", "COALESCE(alcohol_name_ko, '')", "주종 이름 (한글)", "위스키, 브랜디처럼 술의 종류를 가리키는 이름입니다."},
	{"주류 분류", "COALESCE(alcohol_name_en, '')", "주종 이름 (영문)", "영문 기준 주종 이름입니다."},
	{"주류 분류", "COALESCE(alcohol_category_ko, '')", "카테고리 (한글)", "보틀노트 서비스에서 쓰는 분류 체계로 연결한 값입니다."},
	{"주류 분류", "COALESCE(alcohol_category_en, '')", "카테고리 (영문)", "영문 기준 카테고리입니다."},
	{"주류 분류", "COALESCE(alcohol_region_ko, '')", "생산 지역 (한글)", "스페이사이드, 아일라처럼 술의 산지입니다."},
	{"주류 분류", "COALESCE(alcohol_region_en, '')", "생산 지역 (영문)", "영문 기준 생산 지역입니다."},
	{"주류 분류", "COALESCE(alcohol_abv, '')", "주종 기준 도수", "주종 분류에서 참고하는 도수 표기입니다."},

	{"국가 정보", "COALESCE(manufacture_country_name_ko, '')", "제조 국가 (한글)", "표준 국가명으로 연결한 제조국입니다."},
	{"국가 정보", "COALESCE(manufacture_country_name_en, '')", "제조 국가 (영문)", "표준 영문 국가명입니다."},
	{"국가 정보", "COALESCE(manufacture_country_alpha2, '')", "제조 국가 2자리 코드", "국제 표준(ISO 3166) 두 글자 국가 코드입니다. 예: KR, GB."},
	{"국가 정보", "COALESCE(manufacture_country_alpha3, '')", "제조 국가 3자리 코드", "국제 표준 세 글자 국가 코드입니다. 예: KOR, GBR."},
	{"국가 정보", "COALESCE(export_country_name_ko, '')", "수출 국가 (한글)", "표준 국가명으로 연결한 수출국입니다."},
	{"국가 정보", "COALESCE(export_country_name_en, '')", "수출 국가 (영문)", "표준 영문 수출국명입니다."},
	{"국가 정보", "COALESCE(export_country_alpha2, '')", "수출 국가 2자리 코드", "국제 표준 두 글자 국가 코드입니다."},
	{"국가 정보", "COALESCE(export_country_alpha3, '')", "수출 국가 3자리 코드", "국제 표준 세 글자 국가 코드입니다."},

	{"정제 이력", "COALESCE(normalization_status, '')", "정제 상태", "이 건을 자동으로 어디까지 해석했는지 나타냅니다."},
	{"정제 이력", "COALESCE(normalization_version, '')", "정제 규칙 버전", "이 결과를 만든 규칙의 버전입니다. 규칙이 바뀌면 값이 올라갑니다."},
	{"정제 이력", "COALESCE(DATE_FORMAT(normalized_at, '%Y-%m-%d %H:%i'), '')", "정제 실행 시각", "이 건을 마지막으로 정제한 시각입니다."},
	{"정제 이력", "COALESCE(LEFT(HEX(sku_candidate_key_sha256), 12), '')", "같은 제품 묶음 코드", "제품 이름과 병 용량 등이 같은 건끼리 붙는 코드입니다. 같은 코드끼리는 같은 제품일 가능성이 있다는 뜻이지 확정은 아닙니다."},
	{"정제 이력", "COALESCE(review_status, '')", "확인 상태", "사람이 이 건을 확인했는지 여부입니다."},
	{"정제 이력", "COALESCE(reviewed_by, '')", "확인한 사람", "이 건을 확인한 담당자입니다."},
	{"정제 이력", "COALESCE(DATE_FORMAT(reviewed_at, '%Y-%m-%d %H:%i'), '')", "확인 시각", "사람이 이 건을 확인한 시각입니다."},
	{"정제 이력", "COALESCE(review_note, '')", "확인 메모", "확인하면서 남긴 메모입니다."},
}
