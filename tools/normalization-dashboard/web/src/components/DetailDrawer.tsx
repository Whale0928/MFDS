import { useId, useState } from 'react'
import { reasonLabel } from '../format'
import { Hint, Term } from './Hint'
import { StatusChip } from './StatusChip'
import type { Declaration, DetailGroup, MatchingCandidate } from '../types'

const matchingGroupTitles = new Set(['알코올 매칭', '증류소 매칭', '리전 매칭', '매칭 이력'])

export function DetailDrawer({ declaration, onClose }: { declaration: Declaration | null; onClose: () => void }) {
  const [emptyShown, setEmptyShown] = useState(false)
  if (!declaration) return null
  const groups = declaration.groups ?? []
  const reasons = declaration.reason_codes ?? []
  const fragments = (declaration.evidence ?? []).filter((row) => typeof row !== 'string' && row.label === '미해석 값')
  const ledger = groups.filter((group) => group.side === 'ledger')
  const matching = groups.filter((group) => matchingGroupTitles.has(group.title))
  const normalized = groups.filter((group) => group.side !== 'ledger' && !matchingGroupTitles.has(group.title))
  return <div className="drawer-layer" role="presentation" onMouseDown={onClose}>
    <aside className="drawer" role="dialog" aria-modal="true" aria-label="신고 건 상세" onMouseDown={(event) => event.stopPropagation()}>
      <div className="drawer__head">
        <button className="drawer__close" onClick={onClose} autoFocus>닫기</button>
        <p className="drawer__kicker">신고 건 하나의 모든 값</p>
        <h2>{declaration.normalized_name || declaration.source_name}</h2>
        <p className="drawer__source">식약처 원본 표기: {declaration.source_name}</p>
        <div className="drawer__meta">
          <StatusChip status={declaration.status} />
          <label className="drawer__toggle"><input type="checkbox" checked={emptyShown} onChange={(event) => setEmptyShown(event.target.checked)} />값이 없는 항목도 보기</label>
        </div>

        {reasons.length > 0 && <details className="drawer__reasons">
          <summary><Term>확인 사유</Term> {reasons.length}건</summary>
          {reasons.map((code) => <div className="evidence__row evidence__row--reason" key={code}>
            <span>{reasonLabel(code)}</span><i>{code}</i>
          </div>)}
        </details>}

        {fragments.length > 0 && <details className="drawer__reasons">
          <summary><Term>미해석 값</Term> {fragments.length}건</summary>
          {fragments.map((row, index) => <div className="evidence__row evidence__row--reason" key={index}>
            <span>{typeof row === 'string' ? row : row.raw_value || '—'}</span><i>원문 그대로 보관</i>
          </div>)}
        </details>}
      </div>

      {groups.length ? <div className="drawer__columns">
        <DetailColumn kind="ledger" title="원장" caption="식약처에서 수집한 그대로" groups={ledger} emptyShown={emptyShown} />
        <DetailColumn kind="normalized" title="정제 결과" caption="원장에서 갈라내 정리한 값" groups={normalized} emptyShown={emptyShown} />
        <DetailColumn kind="matching" title="매칭 후보" caption="BottleNote 기준 DB와 비교한 결과" groups={matching} matchingCandidates={declaration.matching_candidates ?? []} emptyShown={emptyShown} />
      </div> : <p className="empty-inline">표시할 값을 불러오지 못했습니다.</p>}
    </aside>
  </div>
}

// 원장, 정제, 기준 DB 비교 결과를 나란히 두고 열마다 독립적으로 탐색한다.
function DetailColumn({ kind, title, caption, groups, matchingCandidates = [], emptyShown }: { kind: 'ledger' | 'normalized' | 'matching'; title: string; caption: string; groups: DetailGroup[]; matchingCandidates?: MatchingCandidate[]; emptyShown: boolean }) {
  const headingID = useId()
  const isMatchingColumn = kind === 'matching'
  return <section className={`drawer__column drawer__column--${kind}`} aria-labelledby={headingID} tabIndex={0}>
    <header className="drawer__column-head"><h3 id={headingID}>{title}</h3><p>{caption}</p></header>
    {groups.map((group) => {
      const isHistory = group.title === '매칭 이력'
      const isEntityMatch = isMatchingColumn && !isHistory
      const candidateFields = isEntityMatch ? group.fields.filter((field) => field.label.endsWith('후보')) : []
      const visibleCandidates = candidateFields.filter((field) => field.value !== '')
      const missingCandidates = candidateFields.filter((field) => field.value === '')
      const selectionFields = isEntityMatch ? group.fields.filter((field) => field.label.startsWith('선택한')) : []
      const otherFields = isEntityMatch
        ? group.fields.filter((field) => !field.label.endsWith('후보') && !field.label.startsWith('선택한') && (emptyShown || field.value !== ''))
        : []
      const fields = isEntityMatch
        ? [...visibleCandidates, ...otherFields, ...selectionFields]
        : emptyShown ? group.fields : group.fields.filter((field) => field.value !== '')
      if (!fields.length && !isEntityMatch) return null
      return <div className={`field-group${isHistory ? ' field-group--matching-history' : ''}`} key={group.title}>
        <h4>{group.title}</h4>
        <dl className="field-grid">
          {fields.map((field, index) => {
            const isSelection = isMatchingColumn && field.label.startsWith('선택한')
            const emptyLabel = isSelection ? '선택 안 됨' : isMatchingColumn ? '후보 없음' : '값 없음'
            const isPrimaryCandidate = isEntityMatch && index === 0 && visibleCandidates.length > 0
            const rank = field.label.endsWith('순위 후보') ? Number.parseInt(field.label, 10) : 0
            const targetType = matchingTargetType(group.title)
            const candidate = rank > 0 ? matchingCandidates.find((item) => item.target_type === targetType && item.rank === rank) : undefined
            return <div className={`${isMatchingColumn ? 'field-item--candidate' : ''}${isPrimaryCandidate ? ' field-item--candidate-primary' : ''}${isSelection ? ' field-item--selection' : ''}${field.value ? '' : ' field-item--empty'}`} key={field.label}>
              <dt><Hint text={field.hint}>{field.label}</Hint></dt>
              <dd>{field.value || emptyLabel}</dd>
              {candidate && candidate.evidence.length > 0 && <ul className="matching-evidence" aria-label={`${field.label} 매칭 근거`}>
                {candidate.evidence.map((evidence) => <li key={evidence.feature_code}>
                  <span>{matchingEvidenceLabel(evidence.feature_code)}</span>
                  <strong>{evidence.weight >= 0 ? '+' : ''}{evidence.weight.toFixed(2)}</strong>
                </li>)}
              </ul>}
            </div>
          })}
        </dl>
        {isEntityMatch && visibleCandidates.length === 0 && <p className="matching-empty-summary">일치하는 기준 데이터를 찾지 못했습니다.</p>}
        {isEntityMatch && visibleCandidates.length > 0 && missingCandidates.length > 0 && <p className="matching-empty-summary">
          {missingCandidates.map((field) => field.label.replace('순위 후보', '')).join('·')}순위 후보 없음
        </p>}
      </div>
    })}
  </section>
}

function matchingTargetType(groupTitle: string) {
  switch (groupTitle) {
  case '알코올 매칭': return 'ALCOHOL'
  case '증류소 매칭': return 'DISTILLERY'
  case '리전 매칭': return 'REGION'
  default: return ''
  }
}

function matchingEvidenceLabel(code: string) {
  const labels: Record<string, string> = {
    alcohol_name_exact: '제품명 완전 일치', alcohol_alias_exact: '제품명 별칭 일치',
    alcohol_name_contains: '제품명 포함 일치', alcohol_name_token_fuzzy: '제품명 단어 유사 일치', alcohol_name_fuzzy: '제품명 철자 유사 일치',
    distillery_name_exact: '증류소명 완전 일치', distillery_alias_exact: '증류소명 별칭 일치',
    distillery_name_contains: '증류소명 포함 일치', distillery_name_token_fuzzy: '증류소명 단어 유사 일치', distillery_name_fuzzy: '증류소명 철자 유사 일치',
    region_name_exact: '리전명 완전 일치', region_alias_exact: '리전명 별칭 일치',
    region_name_contains: '리전명 포함 일치', region_name_token_fuzzy: '리전명 단어 유사 일치', region_name_fuzzy: '리전명 철자 유사 일치',
    age_exact: '숙성 연수 일치', age_conflict: '숙성 연수 충돌',
    reference_age_name_conflict: '기준 제품명의 숙성 연수 충돌', reference_age_attribute_conflict: '기준 숙성 정보 충돌',
    abv_exact: '도수 일치', abv_near: '도수 근접', abv_conflict: '도수 충돌',
    cask_exact: '캐스크 일치', edition_exact: '에디션 일치',
    volume_exact: '용량 일치', volume_conflict: '용량 충돌', category_conflict: '주종 분류 충돌',
    alcohol_propagated: '확정 알코올에서 전파', alcohol_consensus: '알코올 후보 공통값',
    region_from_distillery_prior: '증류소 기준 리전 분포', region_from_country_root: '제조국 루트 후보',
    region_parent_compatible: '상위 리전 연관',
  }
  return labels[code] ?? code
}
