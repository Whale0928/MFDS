import { useEffect, useMemo, useState } from 'react'
import { api } from './api'
import { date, number, percent } from './format'
import { BarChart } from './components/BarChart'
import { DetailDrawer } from './components/DetailDrawer'
import { ProvenanceRail } from './components/ProvenanceRail'
import { StatusChip } from './components/StatusChip'
import type { Declaration, DeclarationPage, Filters, Overview, Quality } from './types'

type Section = 'overview' | 'browse' | 'review' | 'quality'
const PAGE_SIZE = 20
const blankFilters: Filters = { statuses: [], item_names: [], importers: [], countries: [], reason_codes: [] }

function useRemote<T>(load: () => Promise<T>, initial: T | null) {
  const [data, setData] = useState<T | null>(initial)
  const [error, setError] = useState<string | null>(null)
  const [loading, setLoading] = useState(true)
  useEffect(() => { let alive = true; setLoading(true); load().then((result) => { if (alive) { setData(result); setError(null) } }).catch((reason: unknown) => { if (alive) setError(reason instanceof Error ? reason.message : '데이터를 불러오지 못했습니다.') }).finally(() => { if (alive) setLoading(false) }); return () => { alive = false } }, [load])
  return { data, error, loading }
}

export default function App() {
  const [section, setSection] = useState<Section>('overview')
  const [query, setQuery] = useState('')
  const [status, setStatus] = useState('')
  const [itemName, setItemName] = useState('')
  const [importer, setImporter] = useState('')
  const [country, setCountry] = useState('')
  const [reason, setReason] = useState('')
  const [page, setPage] = useState(1)
  const [selected, setSelected] = useState<Declaration | null>(null)
  const [detailError, setDetailError] = useState<string | null>(null)
  const overviewLoad = useMemo(() => api.overview, [])
  const filterLoad = useMemo(() => api.filters, [])
  const qualityLoad = useMemo(() => api.quality, [])
  const declarationLoad = useMemo(() => () => api.declarations({ page, page_size: PAGE_SIZE, q: query || undefined, status: status || undefined, item_name: itemName || undefined, importer: importer || undefined, country: country || undefined, reason: reason || undefined }), [page, query, status, itemName, importer, country, reason])
  const reviewLoad = useMemo(() => () => api.declarations({ page: 1, page_size: PAGE_SIZE, status: 'REVIEW_REQUIRED' }), [])
  const overview = useRemote<Overview>(overviewLoad, null)
  const filters = useRemote<Filters>(filterLoad, blankFilters)
  const quality = useRemote<Quality>(qualityLoad, null)
  const declarations = useRemote<DeclarationPage>(declarationLoad, null)
  const reviews = useRemote<DeclarationPage>(reviewLoad, null)
  const browse = () => { setSection('browse'); setPage(1) }
  const openDetail = async (entry: Declaration) => {
    setDetailError(null)
    try {
      setSelected(await api.declaration(entry.rcno))
    } catch {
      setSelected(null)
      setDetailError('상세 정제 근거를 불러오지 못했습니다.')
    }
  }

  return <main className="shell">
    <header className="masthead">
      <a className="brand" href="#overview" onClick={() => setSection('overview')}><span>MFDS</span><b>정제 원장</b><i>DEMONSTRATION</i></a>
      <nav aria-label="대시보드 탐색">{([['overview', '개요'], ['browse', '데이터 탐색'], ['review', '리뷰 큐'], ['quality', '품질 분석']] as Array<[Section, string]>).map(([key, label]) => <button key={key} className={section === key ? 'active' : ''} onClick={() => setSection(key)}>{label}</button>)}</nav>
      <p className="masthead__note">READ-ONLY / LOCAL</p>
    </header>

    {overview.loading && <Loading label="정제 원장을 읽는 중" />}
    {overview.error && <ErrorPanel message={overview.error} />}
    {overview.data && <>
      {section === 'overview' && <OverviewView overview={overview.data} quality={quality.data} qualityError={quality.error} onBrowse={browse} />}
      {section === 'browse' && <BrowseView page={declarations.data} loading={declarations.loading} error={declarations.error} filters={filters.data ?? blankFilters} query={query} status={status} itemName={itemName} importer={importer} country={country} reason={reason} onQuery={setQuery} onStatus={setStatus} onItemName={setItemName} onImporter={setImporter} onCountry={setCountry} onReason={setReason} onOpen={openDetail} pageNumber={page} onPage={setPage} />}
      {section === 'review' && <ReviewView quality={quality.data} loading={quality.loading || reviews.loading} error={quality.error ?? reviews.error} declarations={reviews.data} onOpen={openDetail} />}
      {section === 'quality' && <QualityView quality={quality.data} loading={quality.loading} error={quality.error} />}
    </>}
    {detailError && <div className="detail-error" role="alert"><span>{detailError}</span><button onClick={() => setDetailError(null)}>닫기</button></div>}
    <DetailDrawer declaration={selected} onClose={() => setSelected(null)} />
  </main>
}

function OverviewView({ overview, quality, qualityError, onBrowse }: { overview: Overview; quality: Quality | null; qualityError: string | null; onBrowse: () => void }) {
  const coverage = Object.entries(overview.field_coverage).map(([name, count]) => ({ name, count }))
  return <div className="page overview-page">
    <div className="page-heading"><div><p className="eyebrow">MFDS IMPORT LIQUOR / NORMALIZATION LEDGER</p><h1>원본은 남기고,<br />판단은 드러냅니다.</h1></div><p className="heading-copy">고객 시연용 읽기 전용 화면입니다. 선언 단위의 원본·정제·검토 근거를 한 눈에 따라갑니다.</p></div>
    <ProvenanceRail overview={overview} />
    <div className="overview-ledger"><div><span>검토 필요</span><strong>{number(overview.review_required_count)}</strong><small>규칙으로 보정하지 않은 선언</small></div><div><span>최근 처리</span><strong className="date-number">{date(overview.latest_processed_at)}</strong><small>최신 원본을 기준으로 연결</small></div><div><span>상태 분포</span><div className="micro-status">{Object.entries(overview.status_counts).map(([key, value]) => <span key={key}><StatusChip status={key} /> {number(value)}</span>)}</div></div></div>
    <section className="content-grid"><article className="panel panel--wide"><div className="panel__head"><div><p className="eyebrow">FIELD COVERAGE</p><h2>라벨에서 읽어낸 핵심 항목</h2></div><button className="text-button" onClick={onBrowse}>선언 탐색</button></div><BarChart data={coverage} /><p className="panel__foot">입력값이 없거나 모호하면 공란을 채우지 않고 검토 사유로 남깁니다.</p></article>
      <article className="panel"><p className="eyebrow">REVIEW SIGNAL</p><h2>정제 결과의 읽는 법</h2><ul className="rules-list"><li><b>완료</b> 명시된 값만 구조화</li><li><b>일부</b> 확인된 필드만 보존</li><li><b>검토</b> 자동 보정을 멈춘 지점</li></ul>{qualityError && <p className="muted">품질 세부 집계는 현재 불러올 수 없습니다.</p>}{quality && <p className="muted">검토 사유 {number(quality.review_reasons.length)}종을 추적 중입니다.</p>}</article></section>
  </div>
}

function BrowseView({ page, loading, error, filters, query, status, itemName, importer, country, reason, onQuery, onStatus, onItemName, onImporter, onCountry, onReason, onOpen, pageNumber, onPage }: { page: DeclarationPage | null; loading: boolean; error: string | null; filters: Filters; query: string; status: string; itemName: string; importer: string; country: string; reason: string; onQuery: (value: string) => void; onStatus: (value: string) => void; onItemName: (value: string) => void; onImporter: (value: string) => void; onCountry: (value: string) => void; onReason: (value: string) => void; onOpen: (entry: Declaration) => void; pageNumber: number; onPage: (value: number) => void }) {
  const totalPages = Math.max(1, Math.ceil((page?.total ?? 0) / PAGE_SIZE))
  return <div className="page"><PageTitle eyebrow="DECLARATION EXPLORER" title="원본 라벨과 정제 결과" copy="검색과 다섯 가지 필터는 공개된 선언 필드만 조회합니다." />
    <section className="filter-bar"><label>검색<input value={query} onChange={(event) => onQuery(event.target.value)} placeholder="품명 또는 수입신고 번호" /></label><SelectFilter label="정제 상태" value={status} values={filters.statuses} onChange={onStatus} /><SelectFilter label="품목" value={itemName} values={filters.item_names} onChange={onItemName} /><SelectFilter label="수입사" value={importer} values={filters.importers} onChange={onImporter} /><SelectFilter label="국가" value={country} values={filters.countries} onChange={onCountry} /><SelectFilter label="검토 사유" value={reason} values={filters.reason_codes} onChange={onReason} /><span>{number(page?.total)}건</span></section>
    {loading && <Loading label="선언 목록을 읽는 중" />}{error && <ErrorPanel message={error} />}{page && <><DeclarationTable entries={page.declarations} onOpen={onOpen} /><div className="pagination"><button disabled={pageNumber <= 1} onClick={() => onPage(pageNumber - 1)}>이전</button><span>{pageNumber} / {totalPages}</span><button disabled={pageNumber >= totalPages} onClick={() => onPage(pageNumber + 1)}>다음</button></div></>}
  </div>
}

function ReviewView({ quality, loading, error, declarations, onOpen }: { quality: Quality | null; loading: boolean; error: string | null; declarations: DeclarationPage | null; onOpen: (entry: Declaration) => void }) {
  const reviewItems = declarations?.declarations ?? []
  const distributions = quality?.review_distributions
  return <div className="page"><PageTitle eyebrow="REVIEW QUEUE" title="보정하지 않은 판단 지점" copy="모호한 값은 자동으로 추정하지 않습니다. 근거를 열어 확인할 수 있습니다." />{loading && <Loading label="리뷰 집계를 읽는 중" />}{error && <ErrorPanel message={error} />}{quality && <><div className="content-grid"><article className="panel panel--wide"><div className="panel__head"><div><p className="eyebrow">REASON PARETO</p><h2>검토 사유 상위 항목</h2></div></div><BarChart data={quality.review_reasons.map((entry) => ({ name: entry.code, count: entry.count }))} accent="red" /></article><article className="panel"><p className="eyebrow">QUEUE GUIDE</p><h2>우선 확인</h2><p className="muted">빈도 높은 사유부터 원본 표기와 정제값을 비교합니다. 이 화면에서는 결과를 수정하지 않습니다.</p></article></div><section className="review-distributions" aria-label="리뷰 큐 분포">{([['품목', distributions?.items], ['수입사', distributions?.importers], ['국가', distributions?.countries], ['정제 상태', distributions?.statuses]] as Array<[string, Array<{ name: string; count: number }> | undefined]>).map(([label, data]) => <article className="panel" key={label}><p className="eyebrow">REVIEW BY {label.toUpperCase()}</p><h2>{label} 분포</h2><BarChart data={data ?? []} /></article>)}</section></>}
    <section className="panel review-list"><div className="panel__head"><div><p className="eyebrow">EVIDENCE DRAWER</p><h2>검토 대상</h2></div><span className="muted">상세를 열어 공개 근거를 확인</span></div>{reviewItems.length ? <DeclarationTable entries={reviewItems} onOpen={onOpen} compact /> : <p className="empty-inline">검토 대상 선언이 없습니다.</p>}</section>
  </div>
}

function QualityView({ quality, loading, error }: { quality: Quality | null; loading: boolean; error: string | null }) {
  const duplicates = quality?.duplicate_observations ?? []
  return <div className="page"><PageTitle eyebrow="QUALITY ANALYSIS" title="원장의 밀도와 신호" copy="수집량, 품목, 필드 완성도와 후보 SKU 관찰을 함께 봅니다." />{loading && <Loading label="품질 분석을 읽는 중" />}{error && <ErrorPanel message={error} />}{quality && <><div className="quality-grid"><article className="panel"><p className="eyebrow">MONTHLY COLLECTIONS</p><h2>월별 수집량</h2><Sparkline data={quality.monthly_collections} /></article><article className="panel"><p className="eyebrow">ITEM MIX</p><h2>품목 비중</h2><BarChart data={quality.item_distribution} accent="blue" /></article><article className="panel"><p className="eyebrow">STATUS MIX</p><h2>정제 상태</h2><BarChart data={Object.entries(quality.status_distribution).map(([name, count]) => ({ name, count }))} /></article></div><section className="content-grid quality-coverage"><article className="panel"><p className="eyebrow">FIELD COVERAGE</p><h2>필드 완성도</h2><BarChart data={Object.entries(quality.field_coverage).map(([name, count]) => ({ name, count }))} /></article><article className="panel"><p className="eyebrow">CANDIDATE SKU GROUPS</p><h2>후보 SKU 그룹</h2><BarChart data={(quality.sku_groups ?? []).map((entry) => ({ name: entry.name, count: entry.declarations }))} accent="blue" /></article></section><section className="panel"><div className="panel__head"><div><p className="eyebrow">DUPLICATE OBSERVATIONS</p><h2>후보 SKU와 중복 관찰</h2></div><p className="muted">동일 SKU로 확정한 목록이 아닙니다.</p></div><div className="sku-table"><div className="sku-table__head"><span>후보 그룹</span><span>선언</span><span>관찰</span></div>{duplicates.length ? duplicates.map((entry) => <div key={entry.name}><strong>{entry.name}</strong><span>{number(entry.declarations)}</span><span>{number(entry.observations)}</span></div>) : <p className="empty-inline">중복 관찰 후보가 없습니다.</p>}</div></section></>}</div>
}

function DeclarationTable({ entries, onOpen, compact = false }: { entries: Declaration[]; onOpen: (entry: Declaration) => void; compact?: boolean }) { if (!entries.length) return <div className="empty-state"><h2>조건에 맞는 선언이 없습니다.</h2><p>검색어 또는 필터를 조정해 다시 확인하세요.</p></div>; return <div className={`declaration-table ${compact ? 'declaration-table--compact' : ''}`}><div className="declaration-table__header"><span>원본명</span><span>정제명</span><span>키 1</span><span>키 2</span><span>키 3</span><span>상태</span><span>처리일</span></div>{entries.map((entry) => <button className="declaration-row" key={entry.rcno} onClick={() => onOpen(entry)} aria-label={`${entry.source_name} 상세 열기`}><span data-label="원본명" title={entry.source_name}>{entry.source_name}</span><span data-label="정제명" title={entry.normalized_name}>{entry.normalized_name || '—'}</span><span data-label="키 1">{entry.key_1 || '—'}</span><span data-label="키 2">{entry.key_2 || '—'}</span><span data-label="키 3">{entry.key_3 || '—'}</span><span data-label="상태"><StatusChip status={entry.status} /></span><span data-label="처리일">{date(entry.processed_at)}</span></button>)}</div> }
function SelectFilter({ label, value, values, onChange }: { label: string; value: string; values: string[]; onChange: (value: string) => void }) { return <label>{label}<select value={value} onChange={(event) => onChange(event.target.value)}><option value="">전체 {label}</option>{values.map((item) => <option value={item} key={item}>{item}</option>)}</select></label> }
function Sparkline({ data }: { data: Array<{ month: string; count: number }> }) { const max = Math.max(...data.map((item) => item.count), 1); const points = data.map((item, index) => `${data.length === 1 ? 50 : (index / (data.length - 1)) * 100},${100 - (item.count / max) * 84}`).join(' '); return <>{data.length ? <svg className="sparkline" viewBox="0 0 100 100" preserveAspectRatio="none" aria-label="월별 수집량 추이" role="img"><polyline points={points} /></svg> : <p className="empty-inline">표시할 데이터가 없습니다.</p>}<div className="spark-labels">{data.slice(0, 1).map((item) => <span key={item.month}>{item.month}</span>)}{data.slice(-1).map((item) => <span key={item.month}>{item.month}</span>)}</div></> }
function PageTitle({ eyebrow, title, copy }: { eyebrow: string; title: string; copy: string }) { return <div className="page-title"><div><p className="eyebrow">{eyebrow}</p><h1>{title}</h1></div><p>{copy}</p></div> }
function Loading({ label }: { label: string }) { return <div className="state state--loading" role="status"><span />{label}</div> }
function ErrorPanel({ message }: { message: string }) { return <div className="state state--error" role="alert"><b>조회할 수 없습니다.</b><span>{message}</span></div> }
