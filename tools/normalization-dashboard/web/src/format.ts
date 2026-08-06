export function number(value?: number) { return new Intl.NumberFormat('ko-KR').format(value ?? 0) }
export function percent(value?: number) { return `${Math.round((value ?? 0) * 10) / 10}%` }
export function date(value?: string) {
  if (!value) return '—'
  const parsed = new Date(value)
  return Number.isNaN(parsed.valueOf()) ? value : new Intl.DateTimeFormat('ko-KR', { dateStyle: 'medium' }).format(parsed)
}
export function statusLabel(status: string) {
  return ({ NORMALIZED: '정제 완료', PARTIAL: '일부 항목만 추출', REVIEW_REQUIRED: '추가 확인 필요', UNPARSED: '해석하지 못함', PENDING: '정제 대기', STALE: '재정제 필요' } as Record<string, string>)[status] ?? status
}

const reasonLabels: Record<string, string> = {
  ABV_AMBIGUOUS_POSITION: '도수 표기 위치가 모호함',
  ABV_COMPOSITION_CONTEXT: '성분 설명 안의 도수 표기',
  AGE_CONFLICT_BETWEEN_LANGUAGES: '한글과 영문의 숙성연수가 다름',
  AGE_EXTRACTED: '숙성연수를 추출함',
  BATCH_LANGUAGE_MISMATCH: '한글과 영문의 배치 번호가 다름',
  CASK_NUMBER_PRESERVED_FOR_SKU: '캐스크 번호를 제품 구분값으로 보존함',
  EDITION_TOKEN_LANGUAGE_MISMATCH: '한글과 영문의 에디션 표기가 다름',
  GENERIC_PRODUCT_NAME_REVIEW_REQUIRED: '제품명이 너무 일반적이라 추가 확인 필요',
  HASH_NUMBER_AMBIGUOUS: '# 뒤 숫자의 의미가 모호함',
  KO_VERSION_MARKER_WITHOUT_ENGLISH_MAPPING: '한글 버전 표기에 대응하는 영문 표기가 없음',
  LOT_LABELED_EXCLUDED_FROM_SKU: 'LOT 표기는 제품 구분값에서 제외함',
  LOT_SUFFIX_CODE_EXCLUDED_FROM_SKU: '품명 끝 LOT 코드는 제품 구분값에서 제외함',
  LOT_UNLABELED_CODE: '라벨 없는 LOT 코드를 제품 구분값에서 제외함',
  MANUFACTURE_NUMBER_LABELED_EXCLUDED_FROM_SKU: '제조번호는 제품 구분값에서 제외함',
  MATERIAL_CODE_PRESERVED_FOR_SKU: '원액·재료 코드를 제품 구분값으로 보존함',
  PACKAGE_COUNT_EXCLUDED_FROM_BOTTLE_SKU: '포장 수량은 병 단위 제품 구분값에서 제외함',
  PARENTHESIS_SEMANTIC_TEXT: '괄호 안 의미 있는 표기를 보존함',
  PROOF_PRESERVED_FOR_SKU: 'Proof 값을 제품 구분값으로 보존함',
  VINTAGE_YEAR_REVIEW_REQUIRED: '빈티지 연도인지 추가 확인 필요',
  VOLUME_MISSING_REVIEW_REQUIRED: '용량 표기가 없어 자동 추정하지 않음',
  VOLUME_MISSING: '용량 표기가 없어 자동 추정하지 않음',
  VOLUME_THOUSANDS_SEPARATOR: '용량의 천 단위 구분기호를 정규화함',
  VOLUME_UNIT_CASE_NORMALIZED: '용량 단위 표기를 통일함',
  ALCOHOL_CATEGORY_NOT_MAPPED: '보틀노트 카테고리로 연결하지 못함',
  MANUFACTURE_COUNTRY_NOT_MAPPED: '제조 국가를 ISO 국가 정보로 연결하지 못함',
  EXPORT_COUNTRY_NOT_MAPPED: '수출 국가를 ISO 국가 정보로 연결하지 못함',
  CASK_TYPE_CANDIDATE_EXTRACTED: '제품명에서 캐스크 종류 후보를 추출함',
  OVERSEAS_ESTABLISHMENT_DISTILLERY_CANDIDATE: '해외 제조업소명을 증류소 후보로 보존함',
}

export function reasonLabel(code: string) {
  return reasonLabels[code] ?? code
}
