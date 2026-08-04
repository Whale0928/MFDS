import { date } from '../format'
import { StatusChip } from './StatusChip'
import type { Declaration } from '../types'

export function DetailDrawer({ declaration, onClose }: { declaration: Declaration | null; onClose: () => void }) {
  if (!declaration) return null
  return <div className="drawer-layer" role="presentation" onMouseDown={onClose}>
    <aside className="drawer" role="dialog" aria-modal="true" aria-label="정제 근거 상세" onMouseDown={(event) => event.stopPropagation()}>
      <button className="drawer__close" onClick={onClose} autoFocus>닫기</button>
      <p className="eyebrow">DECLARATION EVIDENCE</p>
      <h2>{declaration.normalized_name || declaration.source_name}</h2>
      <p className="drawer__source">원본명: {declaration.source_name}</p>
      <StatusChip status={declaration.status} />
      <dl className="detail-grid">
        <div><dt>수입신고</dt><dd>{declaration.rcno}</dd></div>
        <div><dt>처리일</dt><dd>{date(declaration.processed_at)}</dd></div>
        <div><dt>품목</dt><dd>{declaration.item_name || '—'}</dd></div>
        <div><dt>수입사</dt><dd>{declaration.importer_name || '—'}</dd></div>
        <div><dt>국가</dt><dd>{declaration.country || '—'}</dd></div>
      </dl>
      <section className="evidence"><h3>공개 정제 근거</h3>{declaration.evidence?.length ? declaration.evidence.map((row, index) => <div className="evidence__row" key={`${typeof row === 'string' ? row : row.label}-${index}`}>
        {typeof row === 'string' ? <><span>미해석 조각</span><b>{row}</b><i>검토 필요</i></> : <><span>{row.label}</span><b>{row.raw_value || '—'}</b><i>{row.normalized_value || row.reason_code || '확인 필요'}</i></>}
      </div>) : <p className="empty-inline">표시 가능한 정제 근거가 없습니다.</p>}</section>
      {declaration.fields && <section className="evidence"><h3>정제된 공개 필드</h3>{Object.entries(declaration.fields).map(([label, value]) => <div className="evidence__row" key={label}><span>{label}</span><b>{value || '—'}</b><i>정제 결과</i></div>)}</section>}
    </aside>
  </div>
}
