import { number, percent } from '../format'
import type { Overview } from '../types'

export function ProvenanceRail({ overview }: { overview: Overview }) {
  const preserved = overview.total_rcno ? (overview.preserved_rcno / overview.total_rcno) * 100 : 0
  const complete = overview.total_rcno ? (overview.normalized_count / overview.total_rcno) * 100 : 0
  return <section className="provenance" aria-label="원장 보존과 정제 흐름">
    <div className="provenance__cap">MFDS IMPORT DECLARATION LEDGER</div>
    <div className="provenance__flow">
      <div><span>원장</span><strong>{number(overview.total_rcno)}</strong><small>수입신고 RCNO</small></div>
      <div className="provenance__bridge" aria-hidden="true"><i /><b>보존 {percent(preserved)}</b><i /></div>
      <div><span>정제</span><strong>{number(overview.normalized_count)}</strong><small>해석 가능 선언</small></div>
    </div>
    <p>원본 선언을 바꾸지 않고, 정제 결과와 검토 근거를 별도 라벨로 연결합니다. 현재 정제 완료율 {percent(complete)}.</p>
  </section>
}
