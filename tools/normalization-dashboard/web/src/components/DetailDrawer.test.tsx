import { renderToStaticMarkup } from 'react-dom/server'
import { describe, expect, it } from 'vitest'
import { DetailDrawer } from './DetailDrawer'
import type { Declaration } from '../types'

const declaration: Declaration = {
  rcno: '202600000001',
  source_name: '원본 제품',
  normalized_name: '정제 제품',
  status: 'NORMALIZED',
  groups: [
    {
      title: '일반 정제',
      side: 'normalized',
      fields: [{ label: '빈 일반 필드', hint: '일반 값', value: '' }],
    },
    {
      title: '알코올 매칭',
      side: 'normalized',
      fields: [
        { label: '1순위 후보', hint: '알코올 후보', value: '라프로익 10년 · ID 600 · 점수 13.50' },
        { label: '2순위 후보', hint: '알코올 후보', value: '' },
        { label: '3순위 후보', hint: '알코올 후보', value: '' },
        { label: '선택한 알코올 ID', hint: '선택값', value: '' },
      ],
    },
    {
      title: '증류소 매칭',
      side: 'normalized',
      fields: [
        { label: '1순위 후보', hint: '증류소 후보', value: '' },
        { label: '2순위 후보', hint: '증류소 후보', value: '' },
        { label: '3순위 후보', hint: '증류소 후보', value: '' },
        { label: '선택한 증류소 ID', hint: '선택값', value: '' },
      ],
    },
    {
      title: '리전 매칭',
      side: 'normalized',
      fields: [{ label: '1순위 후보', hint: '리전 후보', value: '스코틀랜드 · ID 19 · 점수 4.00' }],
    },
    {
      title: '매칭 이력',
      side: 'normalized',
      fields: [{ label: '매칭 규칙 버전', hint: '규칙 버전', value: 'matching-v1' }],
    },
  ],
  matching_candidates: [
    {
      target_type: 'ALCOHOL',
      rank: 1,
      target_id: 600,
      name_ko: '라프로익 10년',
      name_en: 'Laphroaig 10 Year Old',
      score: 13.5,
      evidence: [
        { feature_code: 'age_exact', source: 'age', input_value: '10', reference_value: '10', rule_code: 'AGE_CANONICAL_V4', weight: 3, upstream_target_id: 0 },
        { feature_code: 'volume_conflict', source: 'volume', input_value: '700', reference_value: '750', rule_code: 'VOLUME_ML_V4', weight: -0.5, upstream_target_id: 0 },
      ],
    },
  ],
}

describe('DetailDrawer', () => {
  it('원장_정제결과_매칭후보를독립된세열로표시한다', () => {
    const html = renderToStaticMarkup(<DetailDrawer declaration={declaration} onClose={() => {}} />)

    expect(html).toContain('aria-labelledby=')
    expect(html.match(/class="drawer__column /g)).toHaveLength(3)
    expect(html).toContain('drawer__column--ledger')
    expect(html).toContain('drawer__column--normalized')
    expect(html).toContain('drawer__column--matching')
    expect(html).toContain('BottleNote 기준 DB와 비교한 결과')
    expect(html).toContain('증류소 매칭')
    expect(html).toContain('알코올 매칭')
    expect(html).toContain('2·3순위 후보 없음')
    expect(html).toContain('일치하는 기준 데이터를 찾지 못했습니다.')
    expect(html.match(/field-item--candidate-primary/g)).toHaveLength(2)
    expect(html).toContain('숙성 연수 일치')
    expect(html).toContain('+3.00')
    expect(html).toContain('용량 충돌')
    expect(html).toContain('-0.50')
    expect(html).toContain('선택 안 됨')
    expect(html).not.toContain('field-group--matching"')
  })

  it('일반그룹_값이없으면기존처럼숨긴다', () => {
    const html = renderToStaticMarkup(<DetailDrawer declaration={declaration} onClose={() => {}} />)

    expect(html).not.toContain('일반 정제')
    expect(html).not.toContain('빈 일반 필드')
  })

  it('연결된정제수입사와양방향탐색버튼을표시한다', () => {
    const linked: Declaration = { ...declaration, source_importer_name: '(주)수입사', importer_id: 7, importer_business_name: '(주)수입사', importer_linked: true }
    const html = renderToStaticMarkup(<DetailDrawer declaration={linked} onClose={() => {}} onOpenImporter={() => {}} />)

    expect(html).toContain('원장 수입사')
    expect(html).toContain('정제 수입사')
    expect(html).toContain('ID 7 · 상호 exact 자동 연결')
    expect(html).toContain('수입사 모든 정보 보기')
  })
})
