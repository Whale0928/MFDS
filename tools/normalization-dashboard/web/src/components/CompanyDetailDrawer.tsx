import { OfficialRecords } from './OfficialRecords'
import type { CompanyDetail } from '../types'

export function CompanyDetailDrawer({ company, onClose }: { company: CompanyDetail | null; onClose: () => void }) {
  if (!company) return null
  return <div className="drawer-layer" role="presentation" onMouseDown={onClose}>
    <aside className="drawer company-drawer" role="dialog" aria-modal="true" aria-label="수입업체 공식정보 상세" onMouseDown={(event) => event.stopPropagation()}>
      <div className="drawer__head">
        <button className="drawer__close" onClick={onClose} autoFocus>닫기</button>
        <p className="drawer__kicker">동기화된 공식정보</p>
        <h2>{company.business_name}</h2>
        <p className="drawer__source">인허가번호: {company.license_number || '기록 없음'}</p>
        <p className="company-drawer__notice">네 공식 데이터에서 업체명과 인허가번호로 그때그때 조회한 결과입니다. 별도 연결 테이블은 사용하지 않습니다.</p>
      </div>
      <div className="company-drawer__body"><OfficialRecords records={company.records} /></div>
    </aside>
  </div>
}
