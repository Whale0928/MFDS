import { Hint } from './Hint'
import type { OfficialRecord } from '../types'

export function OfficialRecords({ records, emptyMessage = '이름이 같은 수입업체 공식정보가 없습니다.' }: { records: OfficialRecord[]; emptyMessage?: string }) {
  if (!records.length) return <p className="official-records__empty">{emptyMessage}</p>
  return <div className="official-records">
    {records.map((record, index) => <article className="official-record" key={`${record.source_type}-${record.observed_at}-${index}`}>
      <header><div><span>{record.source_name}</span><small>관측 {record.observed_at || '—'}</small></div></header>
      <dl>
        {record.fields.map((field) => <div key={`${field.label}-${field.value}`}><dt><Hint text={`공식 응답 필드 ${field.hint}`}>{field.label}</Hint></dt><dd>{field.value}</dd></div>)}
      </dl>
    </article>)}
  </div>
}
