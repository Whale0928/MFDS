import { type FormEvent, useEffect, useState } from 'react'
import { api } from '../api'
import { number } from '../format'
import type { Importer, ImporterDetail as ImporterDetailData, ImporterGroup, ImporterLedgerPage, ImporterPage, ImporterProductGroup, ImporterProductPage } from '../types'

const PAGE_SIZE = 20
const LEDGER_PAGE_SIZE = 10

export function ImporterList() {
  const [input, setInput] = useState('')
  const [query, setQuery] = useState('')
  const [page, setPage] = useState(1)
  const [data, setData] = useState<ImporterPage | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [selectedBusinessName, setSelectedBusinessName] = useState<string | null>(null)
  const [matchedOnly, setMatchedOnly] = useState(true)

  useEffect(() => {
    let alive = true
    setLoading(true)
    api.importers({ page, page_size: PAGE_SIZE, q: query || undefined, matched_only: matchedOnly })
      .then((result) => { if (alive) { setData(result); setError(null) } })
      .catch((reason: unknown) => { if (alive) setError(errorMessage(reason)) })
      .finally(() => { if (alive) setLoading(false) })
    return () => { alive = false }
  }, [page, query, matchedOnly])

  useEffect(() => {
    if (!selectedBusinessName) return
    const closeOnEscape = (event: KeyboardEvent) => { if (event.key === 'Escape') setSelectedBusinessName(null) }
    window.addEventListener('keydown', closeOnEscape)
    return () => window.removeEventListener('keydown', closeOnEscape)
  }, [selectedBusinessName])

  const search = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault()
    setPage(1)
    setQuery(input.trim())
  }
  const totalPages = Math.max(1, data?.total_pages ?? 0)

  return <div className="page importer-page">
    <div className="page-title importer-page__title">
      <div><p className="eyebrow">OFFICIAL IMPORTER REGISTER</p><h1>수입사 목록</h1></div>
      <p>같은 상호로 등록된 인허가를 한 묶음으로 보고, 상호별 공식정보와 제품별 수입 원장을 확인합니다.</p>
    </div>
    <section className="importer-register-note" aria-label="수입사 목록 기준">
      <div><span>목록 기준</span><strong>동일 상호 하나당 한 줄</strong></div>
      <p>앞뒤 공백을 제외한 상호가 완전히 같을 때만 묶습니다. 표기 변형이나 비슷한 이름은 별도 상호로 유지합니다.</p>
      <span className="importer-source">C001(수입식품 영업신고 정보)</span>
    </section>
    <form className="importer-search" onSubmit={search}>
      <label htmlFor="importer-search">상호명 검색</label>
      <div><input id="importer-search" value={input} onChange={(event) => setInput(event.target.value)} maxLength={200} placeholder="상호명" /><button type="submit">검색</button></div>
      <label className="importer-match-filter"><input type="checkbox" checked={matchedOnly} onChange={(event) => { setMatchedOnly(event.target.checked); setPage(1) }} /><span><strong>수입 원장과 매칭되는 상호만</strong><small>상호명이 완전히 같은 수입 기록이 있는 경우</small></span></label>
      <span aria-live="polite">{number(data?.total)}개 상호</span>
    </form>
    {loading && <div className="state state--loading" role="status"><span />수입사 목록을 읽는 중</div>}
    {error && <div className="state state--error" role="alert"><b>수입사 목록을 조회할 수 없습니다.</b><span>{error}</span></div>}
    {!loading && !error && data && <>
      <ImporterRegister importers={data.importers} onOpen={setSelectedBusinessName} />
      <div className="pagination"><button disabled={page <= 1} onClick={() => setPage(page - 1)}>이전</button><span>{page} / {totalPages}</span><button disabled={page >= totalPages} onClick={() => setPage(page + 1)}>다음</button></div>
    </>}
    {selectedBusinessName && <ImporterDetailDialog businessName={selectedBusinessName} onClose={() => setSelectedBusinessName(null)} />}
  </div>
}

function ImporterRegister({ importers, onOpen }: { importers: ImporterGroup[]; onOpen: (businessName: string) => void }) {
  if (!importers.length) return <div className="empty-state"><h2>조건에 맞는 수입사가 없습니다.</h2><p>다른 상호명으로 검색하세요.</p></div>
  return <section className="importer-register" aria-label="상호별 수입사 목록">
    <div className="importer-register__header"><span>상호명</span><span>인허가</span><span>소재지</span><span>관할기관</span><span>최초 허가일</span><span>상세</span></div>
    {importers.map((importer) => <button type="button" className="importer-record" key={importer.business_name} onClick={() => onOpen(importer.business_name)} aria-label={`${importer.business_name} 상호 상세 열기`}>
      <span className="importer-record__identity" data-label="상호명"><strong>{importer.business_name}</strong><small>동일 상호 기준</small></span>
      <span data-label="인허가"><b>{number(importer.license_count)}</b>건</span>
      <span data-label="소재지"><b>{number(importer.address_count)}</b>곳</span>
      <span data-label="관할기관"><b>{number(importer.institution_count)}</b>곳</span>
      <span data-label="최초 허가일">{value(importer.first_permit_date)}</span>
      <span className="importer-record__open" data-label="상세">정보 · 원장 보기</span>
    </button>)}
  </section>
}

function ImporterDetailDialog({ businessName, onClose }: { businessName: string; onClose: () => void }) {
  const [data, setData] = useState<ImporterDetailData | null>(null)
  const [error, setError] = useState<string | null>(null)
  useEffect(() => {
    let alive = true
    api.importer(businessName)
      .then((result) => { if (alive) { setData(result); setError(null) } })
      .catch((reason: unknown) => { if (alive) setError(errorMessage(reason)) })
    return () => { alive = false }
  }, [businessName])

  return <div className="importer-detail-layer" role="presentation" onMouseDown={(event) => { if (event.target === event.currentTarget) onClose() }}>
    <aside className="importer-detail-modal" role="dialog" aria-modal="true" aria-labelledby="importer-detail-title">
      <header className="importer-detail-modal__head"><div><p>IMPORTER DOSSIER · EXACT TRADE NAME</p><h1 id="importer-detail-title">{businessName}</h1><span>법인 식별자가 아닌 원장 상호의 완전 일치 기준입니다.</span></div><button type="button" onClick={onClose} aria-label="수입사 상세 닫기">닫기</button></header>
      <div className="importer-profile">
        {!data && !error && <div className="state state--loading" role="status"><span />상호 정보와 수입 현황을 읽는 중</div>}
        {error && <div className="state state--error" role="alert"><b>수입사 상세를 조회할 수 없습니다.</b><span>{error}</span><button type="button" onClick={onClose}>닫기</button></div>}
        {data && <>
          <section className="importer-profile__section" aria-labelledby="importer-license-title">
            <SectionHeading number="1" title="상호의 모든 정보" description={`현재 확인되는 인허가 ${number(data.licenses.length)}건을 빠짐없이 표시합니다.`} id="importer-license-title" />
            <div className="importer-license-grid">{data.licenses.map((license) => <LicenseCard key={license.license_no} license={license} />)}</div>
          </section>
          <section className="importer-profile__section" aria-labelledby="importer-stat-title">
            <SectionHeading number="2" title="수입 현황" description="제품별 원장을 읽기 전에 필요한 규모와 확인 기간만 표시합니다." id="importer-stat-title" />
            <Statistics statistics={data.statistics} />
          </section>
          <section className="importer-profile__section" aria-labelledby="importer-ledger-title">
            <SectionHeading number="3" title="제품별 수입 원장" description="수입사 원문명이 이 상호와 정확히 같은 기록을 제품명별 폴더로 묶었습니다. 폴더를 펼치면 원장 10건씩 확인할 수 있습니다." id="importer-ledger-title" />
            <ImporterProductFolders businessName={businessName} />
          </section>
        </>}
      </div>
    </aside>
  </div>
}

function SectionHeading({ number: sectionNumber, title, description, id }: { number: string; title: string; description: string; id: string }) {
  return <header className="importer-section-head"><span>{sectionNumber}</span><div><h2 id={id}>{title}</h2><p>{description}</p></div></header>
}

function LicenseCard({ license }: { license: Importer }) {
  const fields = [['대표자', license.representative_name], ['업종', license.industry_name], ['허가일', license.permit_date], ['관할기관', license.institution_name], ['소재지', license.address], ['전화번호', license.telephone], ['폐업 상태', license.closure_status_name], ['폐업일', license.closure_date], ['마지막 수집', displayObservedAt(license.observed_at)], ['출처', license.source]]
  return <article className="importer-license-card">
    <header><div><span>인허가번호</span><code>{license.license_no}</code></div><strong className={license.closure_status_name ? 'is-closed' : ''}>{license.closure_status_name || '폐업정보 없음'}</strong></header>
    <dl>{fields.map(([label, fieldValue]) => <div key={label}><dt>{label}</dt><dd>{value(fieldValue)}</dd></div>)}</dl>
    {!license.closure_status_name && <p>폐업정보가 없다는 뜻이며 현재 영업 중임을 확정하지 않습니다.</p>}
  </article>
}

function Statistics({ statistics }: { statistics: ImporterDetailData['statistics'] }) {
  const summary = [['수입 기록', statistics.declaration_count, '건'], ['제품', statistics.product_count, '종']]
  return <div className="importer-stat-ledger importer-stat-ledger--compact">{summary.map(([label, count, unit]) => <div key={label}><span>{label}</span><strong>{number(Number(count))}</strong><small>{unit}</small></div>)}<div className="importer-stat-ledger__period"><span>확인 기간</span><strong>{statistics.first_import_date && statistics.last_import_date ? `${statistics.first_import_date} — ${statistics.last_import_date}` : '일치하는 수입 기록 없음'}</strong></div></div>
}

function ImporterProductFolders({ businessName }: { businessName: string }) {
  const [page, setPage] = useState(1)
  const [data, setData] = useState<ImporterProductPage | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    let alive = true
    setLoading(true)
    api.importerProducts(businessName, page, PAGE_SIZE)
      .then((result) => { if (alive) { setData(result); setError(null) } })
      .catch((reason: unknown) => { if (alive) setError(errorMessage(reason)) })
      .finally(() => { if (alive) setLoading(false) })
    return () => { alive = false }
  }, [businessName, page])

  if (loading) return <div className="state state--loading importer-folders-state" role="status"><span />제품 폴더를 읽는 중</div>
  if (error) return <div className="state state--error importer-folders-state" role="alert"><b>제품별 원장을 조회할 수 없습니다.</b><span>{error}</span></div>
  if (!data || data.products.length === 0) return <div className="empty-state"><h2>정확히 일치하는 수입 기록이 없습니다.</h2><p>상호 표기가 다른 기록은 이 목록에 합치지 않았습니다.</p></div>

  const totalPages = Math.max(1, data.total_pages)
  return <div className="importer-product-folders">
    <div className="importer-product-folders__summary"><strong>{number(data.total)}개 제품</strong><span>최근 수입 기록이 있는 제품부터 표시</span></div>
    <div className="importer-product-folders__list">{data.products.map((product) => <ImporterProductFolder key={product.product_key} businessName={businessName} product={product} />)}</div>
    <div className="pagination importer-product-pagination"><button type="button" disabled={page <= 1} onClick={() => setPage(page - 1)}>이전 제품</button><span>{page} / {totalPages}</span><button type="button" disabled={page >= totalPages} onClick={() => setPage(page + 1)}>다음 제품</button></div>
  </div>
}

function ImporterProductFolder({ businessName, product }: { businessName: string; product: ImporterProductGroup }) {
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
    api.importerProductDeclarations(businessName, product.product_key, page, LEDGER_PAGE_SIZE)
      .then((result) => { if (alive) { setData(result); setError(null) } })
      .catch((reason: unknown) => { if (alive) setError(errorMessage(reason)) })
      .finally(() => { if (alive) setLoading(false) })
    return () => { alive = false }
  }, [businessName, open, page, product.product_key])

  return <article className={`importer-product-folder${open ? ' is-open' : ''}`}>
    <button type="button" className="importer-product-folder__toggle" aria-expanded={open} aria-controls={panelID} onClick={() => setOpen(!open)}>
      <span className="importer-product-folder__name"><i aria-hidden="true" /><strong>{product.product_name}</strong><small>{dateRange(product.first_import_date, product.last_import_date)}</small></span>
      <span className="importer-product-folder__count"><b>{number(product.declaration_count)}</b>건</span>
      <span className="importer-product-folder__action">{open ? '접기' : '원장 보기'}</span>
    </button>
    {open && <div className="importer-product-folder__content" id={panelID}>
      {loading && <div className="state state--loading importer-ledger-state" role="status"><span />수입 원장을 읽는 중</div>}
      {error && <div className="state state--error importer-ledger-state" role="alert"><b>수입 원장을 조회할 수 없습니다.</b><span>{error}</span></div>}
      {!loading && !error && data && <ProductLedgerRows data={data} page={page} onPage={setPage} />}
    </div>}
  </article>
}

function ProductLedgerRows({ data, page, onPage }: { data: ImporterLedgerPage; page: number; onPage: (page: number) => void }) {
  const totalPages = Math.max(1, data.total_pages)
  if (data.declarations.length === 0) return <p className="empty-inline">이 제품 폴더에 표시할 수입 원장이 없습니다.</p>
  return <>
    <div className="importer-ledger-table" role="table" aria-label="제품별 수입 원장">
      <div className="importer-ledger-table__head" role="row"><span>처리일</span><span>수입신고번호</span><span>원장 제품명</span><span>품목</span><span>해외 제조업소</span><span>제조국</span></div>
      {data.declarations.map((item) => <div className="importer-ledger-row" role="row" key={`${item.rcno}-${item.processed_at}`}>
        <span role="cell" data-label="처리일">{value(item.processed_at)}</span>
        <code role="cell" data-label="수입신고번호">{item.rcno}</code>
        <span className="importer-ledger-row__product" role="cell" data-label="원장 제품명"><strong>{value(item.source_name)}</strong>{item.source_name_english && item.source_name_english !== item.source_name && <small>{item.source_name_english}</small>}<small>{[item.volume, item.abv].filter(Boolean).join(' · ')}</small></span>
        <span role="cell" data-label="품목">{value(item.item_name)}</span>
        <span role="cell" data-label="해외 제조업소">{value(item.manufacturer_name)}</span>
        <span role="cell" data-label="제조국">{value(item.country)}</span>
      </div>)}
    </div>
    <div className="pagination importer-ledger-pagination"><button type="button" disabled={page <= 1} onClick={() => onPage(page - 1)}>이전 원장</button><span>{page} / {totalPages} · 총 {number(data.total)}건</span><button type="button" disabled={page >= totalPages} onClick={() => onPage(page + 1)}>다음 원장</button></div>
  </>
}

function value(text: string) { return text || '—' }
function dateRange(first: string, last: string) { return first && last ? `${first} — ${last}` : '처리일 미확인' }
function displayObservedAt(text: string) { return text ? text.replace('T', ' ') : '' }
function errorMessage(reason: unknown) { return reason instanceof Error ? reason.message : '수입사 정보를 불러오지 못했습니다.' }
