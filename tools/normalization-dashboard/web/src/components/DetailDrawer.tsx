import { useState } from 'react'
import { reasonLabel } from '../format'
import { Hint, Term } from './Hint'
import { StatusChip } from './StatusChip'
import type { Declaration } from '../types'

export function DetailDrawer({ declaration, onClose }: { declaration: Declaration | null; onClose: () => void }) {
  const [emptyShown, setEmptyShown] = useState(false)
  if (!declaration) return null
  const groups = declaration.groups ?? []
  const reasons = declaration.reason_codes ?? []
  const fragments = (declaration.evidence ?? []).filter((row) => typeof row !== 'string' && row.label === '미해석 값')
  return <div className="drawer-layer" role="presentation" onMouseDown={onClose}>
    <aside className="drawer" role="dialog" aria-modal="true" aria-label="신고 건 상세" onMouseDown={(event) => event.stopPropagation()}>
      <button className="drawer__close" onClick={onClose} autoFocus>닫기</button>
      <p className="drawer__kicker">신고 건 하나의 모든 값</p>
      <h2>{declaration.normalized_name || declaration.source_name}</h2>
      <p className="drawer__source">식약처 원본 표기: {declaration.source_name}</p>
      <StatusChip status={declaration.status} />

      {reasons.length > 0 && <section className="evidence">
        <h3><Term>확인 사유</Term></h3>
        {reasons.map((code) => <div className="evidence__row evidence__row--reason" key={code}>
          <span>{reasonLabel(code)}</span><i>{code}</i>
        </div>)}
      </section>}

      {fragments.length > 0 && <section className="evidence">
        <h3><Term>미해석 값</Term></h3>
        {fragments.map((row, index) => <div className="evidence__row" key={index}>
          <span>읽어내지 못한 글자</span><b>{typeof row === 'string' ? row : row.raw_value || '—'}</b><i>원문 그대로 보관</i>
        </div>)}
      </section>}

      <div className="drawer__toggle">
        <label><input type="checkbox" checked={emptyShown} onChange={(event) => setEmptyShown(event.target.checked)} />값이 없는 항목도 보기</label>
      </div>

      {groups.map((group) => {
        const fields = emptyShown ? group.fields : group.fields.filter((field) => field.value !== '')
        if (!fields.length) return null
        return <section className="field-group" key={group.title}>
          <h3>{group.title}</h3>
          <dl className="field-grid">
            {fields.map((field) => <div key={field.label}>
              <dt><Hint text={field.hint}>{field.label}</Hint></dt>
              <dd>{field.value || '값 없음'}</dd>
            </div>)}
          </dl>
        </section>
      })}
      {!groups.length && <p className="empty-inline">표시할 값을 불러오지 못했습니다.</p>}
    </aside>
  </div>
}
