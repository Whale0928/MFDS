import { describe, expect, it } from 'vitest'
import { date, fieldLabel, percent, reasonLabel, statusLabel } from './format'
import { glossary, hintFor } from './glossary'

describe('presentation formatters', () => {
  it('formats status and safe fallback values', () => {
    expect(statusLabel('REVIEW_REQUIRED')).toBe('추가 확인 필요')
    expect(statusLabel('NORMALIZED')).toBe('정제 완료')
    expect(statusLabel('UNKNOWN')).toBe('UNKNOWN')
    expect(percent(12.34)).toBe('12.3%')
    expect(date()).toBe('—')
  })

  // 화면에 영문 약어나 내부 컬럼명을 그대로 노출하지 않는다.
  it('통계 항목 이름을 한글 라벨로 바꾼다', () => {
    expect(fieldLabel('abv')).toBe('알코올 도수')
    expect(fieldLabel('normalized_name')).toBe('정제된 제품 이름')
    expect(fieldLabel('ingredient_percent')).toBe('성분 비율')
    expect(fieldLabel('variant_marker')).toBe('제품 변형 마커')
    expect(fieldLabel('unknown_field')).toBe('unknown_field')
  })

  it('확인 사유 코드를 사람이 읽는 문장으로 바꾼다', () => {
    expect(reasonLabel('VOLUME_MISSING')).toBe('용량 표기가 없어 자동 추정하지 않음')
    expect(reasonLabel('VINTAGE_YEAR_REVIEW_REQUIRED')).toBe('빈티지 연도인지 추가 확인 필요')
  })
})

describe('용어 각주', () => {
  it('용어사전의 모든 설명이 한 문장 이상으로 채워져 있다', () => {
    const entries = Object.entries(glossary)
    expect(entries.length).toBeGreaterThan(20)
    for (const [term, hint] of entries) {
      expect(hint.length, `${term} 각주가 너무 짧다`).toBeGreaterThan(15)
      expect(hint, `${term} 각주가 문장으로 끝나지 않는다`).toMatch(/\.$/)
    }
  })

  it('띄어쓰기와 무관하게 각주를 찾는다', () => {
    expect(hintFor('확인 사유')).toBe(glossary.확인사유)
    expect(hintFor('LOT 번호')).toBe(glossary.LOT번호)
    expect(hintFor('없는용어')).toBe('')
  })

  // SKU 같은 줄임말은 화면에 남기지 않기로 했다.
  it('각주 문장에 설명 없는 줄임말을 쓰지 않는다', () => {
    for (const [term, hint] of Object.entries(glossary)) {
      expect(`${term} ${hint}`, `${term} 에 SKU 가 남아 있다`).not.toContain('SKU')
    }
  })
})
