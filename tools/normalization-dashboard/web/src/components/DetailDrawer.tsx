import { useState } from 'react'
import { reasonLabel } from '../format'
import { Hint, Term } from './Hint'
import { StatusChip } from './StatusChip'
import type { Declaration, DetailGroup } from '../types'

export function DetailDrawer({ declaration, onClose }: { declaration: Declaration | null; onClose: () => void }) {
  const [emptyShown, setEmptyShown] = useState(false)
  if (!declaration) return null
  const groups = declaration.groups ?? []
  const reasons = declaration.reason_codes ?? []
  const fragments = (declaration.evidence ?? []).filter((row) => typeof row !== 'string' && row.label === '미해석 값')
  const ledger = groups.filter((group) => group.side === 'ledger')
  const normalized = groups.filter((group) => group.side !== 'ledger')
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
        <DetailColumn title="원장" caption="식약처에서 수집한 그대로" groups={ledger} emptyShown={emptyShown} />
        <DetailColumn title="정제 결과" caption="원장에서 갈라내 정리한 값" groups={normalized} emptyShown={emptyShown} />
      </div> : <p className="empty-inline">표시할 값을 불러오지 못했습니다.</p>}
    </aside>
  </div>
}

// 원장과 정제를 나란히 두되 서로 다른 위치를 볼 수 있도록 열마다 따로 스크롤한다.
function DetailColumn({ title, caption, groups, emptyShown }: { title: string; caption: string; groups: DetailGroup[]; emptyShown: boolean }) {
  return <section className="drawer__column" aria-label={title}>
    <header className="drawer__column-head"><h3>{title}</h3><p>{caption}</p></header>
    {groups.map((group) => {
      const fields = emptyShown ? group.fields : group.fields.filter((field) => field.value !== '')
      if (!fields.length) return null
      return <div className="field-group" key={group.title}>
        <h4>{group.title}</h4>
        <dl className="field-grid">
          {fields.map((field) => <div key={field.label}>
            <dt><Hint text={field.hint}>{field.label}</Hint></dt>
            <dd>{field.value || '값 없음'}</dd>
          </div>)}
        </dl>
      </div>
    })}
  </section>
}
