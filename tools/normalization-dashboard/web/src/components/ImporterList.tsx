import { type FormEvent, useEffect, useState } from 'react'
import { api } from '../api'
import { number } from '../format'
import type { ImporterDetail as ImporterDetailData, ImporterGroup, ImporterLedgerPage, ImporterPage, ImporterProductGroup, ImporterProductPage, ImporterProfile } from '../types'

const PAGE_SIZE = 20
const LEDGER_PAGE_SIZE = 10

export function ImporterList({ onOpenImporter }: { onOpenImporter: (importer: ImporterGroup) => void }) {
  const [input, setInput] = useState('')
  const [query, setQuery] = useState('')
  const [page, setPage] = useState(1)
  const [data, setData] = useState<ImporterPage | null>(null)
  const [loading, setLoading] = useState(true)
	const [error, setError] = useState<string | null>(null)
	const [matchedOnly, setMatchedOnly] = useState(true)
	const [operatingStatus, setOperatingStatus] = useState('')
	const [industryName, setIndustryName] = useState('수입식품등 수입판매업')
	const [sort, setSort] = useState<'business_name' | 'declarations' | 'last_import' | 'observed_at'>('business_name')

  useEffect(() => {
    let alive = true
    setLoading(true)
	api.importers({ page, page_size: PAGE_SIZE, q: query || undefined, matched_only: matchedOnly, operating_status: operatingStatus || undefined, industry_name: industryName || undefined, sort, order: sort === 'business_name' ? 'asc' : 'desc' })
      .then((result) => { if (alive) { setData(result); setError(null) } })
      .catch((reason: unknown) => { if (alive) setError(errorMessage(reason)) })
      .finally(() => { if (alive) setLoading(false) })
    return () => { alive = false }
	}, [page, query, matchedOnly, operatingStatus, industryName, sort])

  const search = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault()
    setPage(1)
    setQuery(input.trim())
  }
  const totalPages = Math.max(1, data?.total_pages ?? 0)
	const activeFilters = [query, operatingStatus, industryName !== '수입식품등 수입판매업' ? industryName : '', matchedOnly ? '' : 'all', sort !== 'business_name' ? sort : ''].filter(Boolean).length
  const resetFilters = () => {
    setInput('')
    setQuery('')
    setMatchedOnly(true)
    setOperatingStatus('')
		setIndustryName('수입식품등 수입판매업')
    setSort('business_name')
    setPage(1)
  }

  return <div className="page importer-page">
    <div className="page-title importer-page__title">
      <div><p className="eyebrow">OFFICIAL IMPORTER REGISTER</p><h1>수입사 목록</h1></div>
      <p>공식 국내업소 화면과 수입신고번호 근거로 확정된 수입사와 제품별 수입 원장을 확인합니다.</p>
    </div>
    <section className="importer-register-note" aria-label="수입사 목록 기준">
      <div><span>목록 기준</span><strong>공식 업소 식별코드 하나당 한 줄</strong></div>
		<p>원장 상호의 공식 후보가 한 건이면 바로 연결하고, 복수 후보는 수입신고번호(RCNO)가 확인되는 공식 제품 상세로 정확한 업소를 확정합니다.</p>
		<span className="importer-source">수입식품정보마루 국내업소 공식 화면</span>
    </section>
    <form className="importer-search" onSubmit={search}>
      <div className="importer-search__query"><label htmlFor="importer-search">통합 검색</label><div><input id="importer-search" value={input} onChange={(event) => setInput(event.target.value)} maxLength={200} placeholder="상호 · 대표자 · 주소 · 전화" /><button type="submit">검색</button></div></div>
		<label>영업 상태<select value={operatingStatus} onChange={(event) => { setOperatingStatus(event.target.value); setPage(1) }}><option value="">전체</option><option value="정상">정상</option><option value="UNKNOWN">공식 상세 미확인</option></select></label>
      <label>업종<select value={industryName} onChange={(event) => { setIndustryName(event.target.value); setPage(1) }}><option value="">업종 전체</option><option value="수입식품등 수입판매업">수입식품등 수입판매업</option></select></label>
		<label>정렬<select value={sort} onChange={(event) => { setSort(event.target.value as typeof sort); setPage(1) }}><option value="business_name">상호명순</option><option value="declarations">수입 기록 많은 순</option><option value="last_import">최근 수입순</option><option value="observed_at">최근 관측순</option></select></label>
      <label className="importer-match-filter"><input type="checkbox" checked={matchedOnly} onChange={(event) => { setMatchedOnly(event.target.checked); setPage(1) }} /><span><strong>수입 원장과 매칭되는 상호만</strong><small>상호명이 완전히 같은 수입 기록이 있는 경우</small></span></label>
      <button className="filter-reset" type="button" onClick={resetFilters}>기본 조건으로 초기화 ({activeFilters})</button>
      <span aria-live="polite">{number(data?.total)}개 상호</span>
    </form>
    {loading && <div className="state state--loading" role="status"><span />수입사 목록을 읽는 중</div>}
    {error && <div className="state state--error" role="alert"><b>수입사 목록을 조회할 수 없습니다.</b><span>{error}</span></div>}
    {!loading && !error && data && <>
      <ImporterRegister importers={data.importers} matchedOnly={matchedOnly} onShowAll={() => { setMatchedOnly(false); setPage(1) }} onOpen={onOpenImporter} />
      <div className="pagination"><button disabled={page <= 1} onClick={() => setPage(page - 1)}>이전</button><span>{page} / {totalPages}</span><button disabled={page >= totalPages} onClick={() => setPage(page + 1)}>다음</button></div>
    </>}
  </div>
}

function ImporterRegister({ importers, matchedOnly, onShowAll, onOpen }: { importers: ImporterGroup[]; matchedOnly: boolean; onShowAll: () => void; onOpen: (importer: ImporterGroup) => void }) {
  if (!importers.length) return <div className="empty-state"><h2>조건에 맞는 수입사가 없습니다.</h2><p>{matchedOnly ? '아직 원장 연결이 없거나 다른 필터 조건에 해당하지 않습니다.' : '검색어 또는 필터 조건을 바꿔 다시 확인하세요.'}</p>{matchedOnly && <button type="button" onClick={onShowAll}>원장 미연결 수입사도 보기</button>}</div>
  return <section className="importer-register" aria-label="상호별 수입사 목록">
    <div className="importer-register__header"><span>정제 수입사</span><span>공식 프로필</span><span>수입 원장</span><span>관리 상태</span><span>상세</span></div>
    {importers.map((importer) => <button type="button" className="importer-record" key={importer.importer_id} onClick={() => onOpen(importer)} aria-label={`${importer.business_name} 상호 상세 열기`}>
      <span className="importer-record__identity" data-label="정제 수입사"><strong>{importer.business_name}</strong><small>ID {importer.importer_id} · {statusLabel(importer.operating_status)}</small><em>{value(importer.industry_name)}</em></span>
      <span className="importer-record__profile" data-label="공식 프로필"><b>{value(importer.representative_name)}</b><small>{value(importer.primary_address)}</small><small>{value(importer.telephone)}</small><small>인허가 {value(importer.license_no)}</small></span>
      <span data-label="수입 원장"><b>{number(importer.declaration_count)}</b>건<small>제품 {number(importer.product_count)}종</small><small>{dateRange(importer.first_import_date, importer.last_import_date)}</small></span>
      <span data-label="관리 상태"><b>{value(importer.admin_status)}</b><small>{value(importer.description)}</small><small>관측 {displayObservedAt(importer.source_observed_at)}</small></span>
      <span className="importer-record__open" data-label="상세">정보 · 원장 보기</span>
    </button>)}
  </section>
}

export function ImporterDetailDialog({ importer, active = true, onClose, onOpenDeclaration }: { importer: Pick<ImporterGroup, 'importer_id' | 'business_name'>; active?: boolean; onClose: () => void; onOpenDeclaration: (rcno: string) => void }) {
  const [data, setData] = useState<ImporterDetailData | null>(null)
  const [error, setError] = useState<string | null>(null)
  useEffect(() => {
    let alive = true
    api.importer(importer.importer_id)
      .then((result) => { if (alive) { setData(result); setError(null) } })
      .catch((reason: unknown) => { if (alive) setError(errorMessage(reason)) })
    return () => { alive = false }
  }, [importer.importer_id])

  useEffect(() => {
    if (!active) return
    const closeOnEscape = (event: KeyboardEvent) => { if (event.key === 'Escape') onClose() }
    window.addEventListener('keydown', closeOnEscape)
    return () => window.removeEventListener('keydown', closeOnEscape)
  }, [active, onClose])

  return <div className="importer-detail-layer" role="presentation" onMouseDown={(event) => { if (event.target === event.currentTarget) onClose() }}>
    <aside className="importer-detail-modal" role="dialog" aria-modal="true" aria-labelledby="importer-detail-title">
      <header className="importer-detail-modal__head"><div><p>IMPORTER DOSSIER · EXACT TRADE NAME</p><h1 id="importer-detail-title">{importer.business_name}</h1><span>정제 수입사 ID {importer.importer_id} · 원장 상호 완전 일치 기준</span></div><button type="button" onClick={onClose} aria-label="수입사 상세 닫기">닫기</button></header>
      <div className="importer-profile">
        {!data && !error && <div className="state state--loading" role="status"><span />상호 정보와 수입 현황을 읽는 중</div>}
        {error && <div className="state state--error" role="alert"><b>수입사 상세를 조회할 수 없습니다.</b><span>{error}</span><button type="button" onClick={onClose}>닫기</button></div>}
        {data && <>
          <ImporterDetailAccessNote profile={data.profile} />
          <section className="importer-profile__section" aria-labelledby="importer-profile-title">
			<SectionHeading number="1" title="정제 수입사 프로필" description="공식 국내업소 화면과 수입신고번호 근거로 확정한 기준정보입니다." id="importer-profile-title" />
            <ImporterProfileSummary profile={data.profile} />
          </section>
          <section className="importer-profile__section" aria-labelledby="importer-official-title">
            <SectionHeading number="2" title="공식 필드와 관리 필드" description="최신 2테이블 계약이 제공하는 공식 값과 내부 관리 값을 모두 읽기 전용으로 표시합니다." id="importer-official-title" />
            <ImporterOfficialProfile profile={data.profile} />
          </section>
          <section className="importer-profile__section" aria-labelledby="importer-stat-title">
            <SectionHeading number="3" title="수입 현황" description="연결된 정제 원장의 규모와 확인 기간입니다." id="importer-stat-title" />
            <Statistics statistics={data.statistics} />
          </section>
          <section className="importer-profile__section" aria-labelledby="importer-ledger-title">
            <SectionHeading number="4" title="제품별 수입 원장" description="저장된 수입사 연결 ID가 같은 기록을 제품명별 폴더로 묶었습니다. 원장을 누르면 신고 상세를 확인할 수 있습니다." id="importer-ledger-title" />
            <ImporterProductFolders importerID={importer.importer_id} onOpenDeclaration={onOpenDeclaration} />
          </section>
        </>}
      </div>
    </aside>
  </div>
}

function SectionHeading({ number: sectionNumber, title, description, id }: { number: string; title: string; description: string; id: string }) {
  return <header className="importer-section-head"><span>{sectionNumber}</span><div><h2 id={id}>{title}</h2><p>{description}</p></div></header>
}

function ImporterDetailAccessNote({ profile }: { profile: ImporterProfile }) {
  const restricted = !profile.source_detail_url
  return <section className={`importer-detail-access-note${restricted ? '' : ' is-available'}`} aria-label="공식 상세 접근 상태">
    <strong>{restricted ? '공식 상세 접근 제한 · HTTP 302' : '공식 상세 연결 확인'}</strong>
    <p>{restricted ? '공식 상세 화면으로 확인할 수 없는 값은 추정하지 않고 미제공으로 표시합니다.' : '공식 상세 URL이 저장되어도 응답에 없는 값은 추정하지 않고 미제공으로 표시합니다.'} 사업자등록번호, 법인 식별자와 상세 HTML은 이 계약에서 제공되지 않습니다.</p>
  </section>
}

function ImporterProfileSummary({ profile }: { profile: ImporterProfile }) {
  const fields = [['정제 수입사 ID', String(profile.importer_id)], ['상호', profile.business_name], ['대표자', profile.representative_name], ['대표 주소', profile.primary_address], ['전화번호', profile.telephone], ['업종', profile.industry_name], ['영업 상태', statusLabel(profile.operating_status)], ['인허가번호', profile.license_no], ['인허가일', profile.permit_date], ['관할기관', profile.institution_name]]
	return <div className="importer-master-profile"><dl>{fields.map(([label, fieldValue]) => <div key={label}><dt>{label}</dt><dd>{officialValue(fieldValue)}</dd></div>)}</dl><p>영업 상태는 공식 상세 화면에서 확인된 경우에만 표시하며, 접근 제한 건은 추정하지 않습니다.</p></div>
}

function ImporterOfficialProfile({ profile }: { profile: ImporterProfile }) {
  const officialFields = [['내부 업소 코드', profile.official_business_code], ['상호 exact key SHA-256', profile.business_name_key_sha256], ['목록 조회 URL', profile.source_list_url], ['상세 조회 URL', profile.source_detail_url], ['목록 관측 시각', displayObservedAt(profile.source_observed_at)], ['목록 SHA-256', profile.source_list_sha256], ['상세 SHA-256', profile.source_detail_sha256]]
  const adminFields = [['설명', profile.description], ['관리자 메모', profile.admin_note], ['관리 상태', profile.admin_status], ['마지막 검토자', profile.reviewed_by], ['마지막 검토 시각', displayObservedAt(profile.reviewed_at)], ['생성 시각', displayObservedAt(profile.created_at)], ['수정 시각', displayObservedAt(profile.updated_at)]]
  return <div className="importer-official-profile">
    <div><h3>공식 출처와 관측</h3><dl>{officialFields.map(([label, fieldValue]) => <div key={label}><dt>{label}</dt><dd>{officialValue(fieldValue)}</dd></div>)}</dl></div>
    <div><h3>내부 관리 필드 · 읽기 전용</h3><dl>{adminFields.map(([label, fieldValue]) => <div key={label}><dt>{label}</dt><dd>{officialValue(fieldValue)}</dd></div>)}</dl></div>
  </div>
}

function Statistics({ statistics }: { statistics: ImporterDetailData['statistics'] }) {
  const summary = [['수입 기록', statistics.declaration_count, '건'], ['제품', statistics.product_count, '종']]
  return <div className="importer-stat-ledger importer-stat-ledger--compact">{summary.map(([label, count, unit]) => <div key={label}><span>{label}</span><strong>{number(Number(count))}</strong><small>{unit}</small></div>)}<div className="importer-stat-ledger__period"><span>확인 기간</span><strong>{statistics.first_import_date && statistics.last_import_date ? `${statistics.first_import_date} — ${statistics.last_import_date}` : '일치하는 수입 기록 없음'}</strong></div></div>
}

function ImporterProductFolders({ importerID, onOpenDeclaration }: { importerID: number; onOpenDeclaration: (rcno: string) => void }) {
  const [page, setPage] = useState(1)
  const [data, setData] = useState<ImporterProductPage | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    let alive = true
    setLoading(true)
    api.importerProducts(importerID, page, PAGE_SIZE)
      .then((result) => { if (alive) { setData(result); setError(null) } })
      .catch((reason: unknown) => { if (alive) setError(errorMessage(reason)) })
      .finally(() => { if (alive) setLoading(false) })
    return () => { alive = false }
  }, [importerID, page])

  if (loading) return <div className="state state--loading importer-folders-state" role="status"><span />제품 폴더를 읽는 중</div>
  if (error) return <div className="state state--error importer-folders-state" role="alert"><b>제품별 원장을 조회할 수 없습니다.</b><span>{error}</span></div>
  if (!data || data.products.length === 0) return <div className="empty-state"><h2>정확히 일치하는 수입 기록이 없습니다.</h2><p>상호 표기가 다른 기록은 이 목록에 합치지 않았습니다.</p></div>

  const totalPages = Math.max(1, data.total_pages)
  return <div className="importer-product-folders">
    <div className="importer-product-folders__summary"><strong>{number(data.total)}개 제품</strong><span>최근 수입 기록이 있는 제품부터 표시</span></div>
    <div className="importer-product-folders__list">{data.products.map((product) => <ImporterProductFolder key={product.product_key} importerID={importerID} product={product} onOpenDeclaration={onOpenDeclaration} />)}</div>
    <div className="pagination importer-product-pagination"><button type="button" disabled={page <= 1} onClick={() => setPage(page - 1)}>이전 제품</button><span>{page} / {totalPages}</span><button type="button" disabled={page >= totalPages} onClick={() => setPage(page + 1)}>다음 제품</button></div>
  </div>
}

function ImporterProductFolder({ importerID, product, onOpenDeclaration }: { importerID: number; product: ImporterProductGroup; onOpenDeclaration: (rcno: string) => void }) {
  const [open, setOpen] = useState(false)
  const [page, setPage] = useState(1)
  const [data, setData] = useState<ImporterLedgerPage | null>(null)
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const panelID = `product-ledger-${product.product_key.slice(5, 17)}`

  useEffect(() => {
    if (!open) return
    let alive = true
    setLoading(true)
    api.importerProductDeclarations(importerID, product.product_key, page, LEDGER_PAGE_SIZE)
      .then((result) => { if (alive) { setData(result); setError(null) } })
      .catch((reason: unknown) => { if (alive) setError(errorMessage(reason)) })
      .finally(() => { if (alive) setLoading(false) })
    return () => { alive = false }
  }, [importerID, open, page, product.product_key])

  return <article className={`importer-product-folder${open ? ' is-open' : ''}`}>
    <button type="button" className="importer-product-folder__toggle" aria-expanded={open} aria-controls={panelID} onClick={() => setOpen(!open)}>
      <span className="importer-product-folder__name"><i aria-hidden="true" /><strong>{product.product_name}</strong><small>{dateRange(product.first_import_date, product.last_import_date)}</small></span>
      <span className="importer-product-folder__count"><b>{number(product.declaration_count)}</b>건</span>
      <span className="importer-product-folder__action">{open ? '접기' : '원장 보기'}</span>
    </button>
    {open && <div className="importer-product-folder__content" id={panelID}>
      {loading && <div className="state state--loading importer-ledger-state" role="status"><span />수입 원장을 읽는 중</div>}
      {error && <div className="state state--error importer-ledger-state" role="alert"><b>수입 원장을 조회할 수 없습니다.</b><span>{error}</span></div>}
      {!loading && !error && data && <ProductLedgerRows data={data} page={page} onPage={setPage} onOpenDeclaration={onOpenDeclaration} />}
    </div>}
  </article>
}

function ProductLedgerRows({ data, page, onPage, onOpenDeclaration }: { data: ImporterLedgerPage; page: number; onPage: (page: number) => void; onOpenDeclaration: (rcno: string) => void }) {
  const totalPages = Math.max(1, data.total_pages)
  if (data.declarations.length === 0) return <p className="empty-inline">이 제품 폴더에 표시할 수입 원장이 없습니다.</p>
  return <>
    <div className="importer-ledger-table" aria-label="제품별 수입 원장">
      <div className="importer-ledger-table__head"><span>처리일</span><span>수입신고번호</span><span>원장 제품명</span><span>품목</span><span>해외 제조업소</span><span>제조국</span></div>
      {data.declarations.map((item) => <button type="button" className="importer-ledger-row" key={`${item.rcno}-${item.processed_at}`} onClick={() => onOpenDeclaration(item.rcno)} aria-label={`${item.source_name} 수입 원장 상세 열기`}>
        <span data-label="처리일">{value(item.processed_at)}</span>
        <code data-label="수입신고번호">{item.rcno}</code>
        <span className="importer-ledger-row__product" data-label="원장 제품명"><strong>{value(item.source_name)}</strong>{item.source_name_english && item.source_name_english !== item.source_name && <small>{item.source_name_english}</small>}<small>{[item.volume, item.abv].filter(Boolean).join(' · ')}</small></span>
        <span data-label="품목">{value(item.item_name)}</span>
        <span data-label="해외 제조업소">{value(item.manufacturer_name)}</span>
        <span data-label="제조국">{value(item.country)}</span>
      </button>)}
    </div>
    <div className="pagination importer-ledger-pagination"><button type="button" disabled={page <= 1} onClick={() => onPage(page - 1)}>이전 원장</button><span>{page} / {totalPages} · 총 {number(data.total)}건</span><button type="button" disabled={page >= totalPages} onClick={() => onPage(page + 1)}>다음 원장</button></div>
  </>
}

function value(text: string) { return text || '—' }
function officialValue(text: string) { return text || '미제공' }
function dateRange(first: string, last: string) { return first && last ? `${first} — ${last}` : '처리일 미확인' }
function displayObservedAt(text: string) { return text ? text.replace('T', ' ') : '' }
function statusLabel(status: string) { return status === 'UNKNOWN' || !status ? '공식 상세 미확인' : status }
function errorMessage(reason: unknown) { return reason instanceof Error ? reason.message : '수입사 정보를 불러오지 못했습니다.' }
