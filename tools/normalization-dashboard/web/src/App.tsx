import { useEffect, useMemo, useState } from 'react'
import { api } from './api'
import { date, fieldLabel, number, reasonLabel, statusLabel } from './format'
import { BarChart } from './components/BarChart'
import { DetailDrawer } from './components/DetailDrawer'
import { Hint, Term } from './components/Hint'
import { hintFor } from './glossary'
import { StatusChip } from './components/StatusChip'
import type { Declaration, DeclarationPage, Filters, Quality } from './types'

type Section = 'browse' | 'review' | 'quality'
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
  const [section, setSection] = useState<Section>('browse')
  const [query, setQuery] = useState('')
  const [status, setStatus] = useState('')
  const [itemName, setItemName] = useState('')
  const [importer, setImporter] = useState('')
  const [country, setCountry] = useState('')
  const [reason, setReason] = useState('')
  const [page, setPage] = useState(1)
  const [selected, setSelected] = useState<Declaration | null>(null)
  const [detailError, setDetailError] = useState<string | null>(null)
  const filterLoad = useMemo(() => api.filters, [])
  const qualityLoad = useMemo(() => api.quality, [])
  const declarationLoad = useMemo(() => () => api.declarations({ page, page_size: PAGE_SIZE, q: query || undefined, status: status || undefined, item_name: itemName || undefined, importer: importer || undefined, country: country || undefined, reason: reason || undefined }), [page, query, status, itemName, importer, country, reason])
  const reviewLoad = useMemo(() => () => api.declarations({ page: 1, page_size: PAGE_SIZE, status: 'REVIEW_REQUIRED' }), [])
  const filters = useRemote<Filters>(filterLoad, blankFilters)
  const quality = useRemote<Quality>(qualityLoad, null)
  const declarations = useRemote<DeclarationPage>(declarationLoad, null)
  const reviews = useRemote<DeclarationPage>(reviewLoad, null)
  const openDetail = async (entry: Declaration) => {
    setDetailError(null)
    try {
      setSelected(await api.declaration(entry.rcno))
    } catch {
      setSelected(null)
      setDetailError('이 건의 상세 값을 불러오지 못했습니다.')
    }
  }

  return <main className="shell">
    <header className="masthead">
      <a className="brand" href="#browse" onClick={() => setSection('browse')}><span>MFDS</span><b>수입주류 신고 원장</b><i>시연용</i></a>
      <nav aria-label="화면 이동">{([['browse', '데이터 탐색'], ['review', '확인 필요 목록'], ['quality', '전체 통계']] as Array<[Section, string]>).map(([key, label]) => <button key={key} className={section === key ? 'active' : ''} onClick={() => setSection(key)}>{label}</button>)}</nav>
      <p className="masthead__note">읽기 전용 / 이 화면에서는 아무 값도 고치지 않습니다</p>
    </header>

    {section === 'browse' && <BrowseView page={declarations.data} loading={declarations.loading} error={declarations.error} filters={filters.data ?? blankFilters} query={query} status={status} itemName={itemName} importer={importer} country={country} reason={reason} onQuery={setQuery} onStatus={setStatus} onItemName={setItemName} onImporter={setImporter} onCountry={setCountry} onReason={setReason} onOpen={openDetail} pageNumber={page} onPage={setPage} />}
    {section === 'review' && <ReviewView quality={quality.data} loading={quality.loading || reviews.loading} error={quality.error ?? reviews.error} declarations={reviews.data} onOpen={openDetail} />}
    {section === 'quality' && <QualityView quality={quality.data} loading={quality.loading} error={quality.error} />}

    {detailError && <div className="detail-error" role="alert"><span>{detailError}</span><button onClick={() => setDetailError(null)}>닫기</button></div>}
    <DetailDrawer declaration={selected} onClose={() => setSelected(null)} />
  </main>
}

function BrowseView({ page, loading, error, filters, query, status, itemName, importer, country, reason, onQuery, onStatus, onItemName, onImporter, onCountry, onReason, onOpen, pageNumber, onPage }: { page: DeclarationPage | null; loading: boolean; error: string | null; filters: Filters; query: string; status: string; itemName: string; importer: string; country: string; reason: string; onQuery: (value: string) => void; onStatus: (value: string) => void; onItemName: (value: string) => void; onImporter: (value: string) => void; onCountry: (value: string) => void; onReason: (value: string) => void; onOpen: (entry: Declaration) => void; pageNumber: number; onPage: (value: number) => void }) {
  const totalPages = Math.max(1, Math.ceil((page?.total ?? 0) / PAGE_SIZE))
  return <div className="page">
    <PageTitle title="식약처 원본과 정제 결과" copy="한 줄이 수입신고 한 건입니다. 줄을 누르면 그 건의 원본 값과 정제된 값을 빠짐없이 볼 수 있습니다." />
    <section className="filter-bar">
      <label>검색<input value={query} onChange={(event) => onQuery(event.target.value)} placeholder="제품명 또는 수입신고번호" /></label>
      <SelectFilter label="정제 상태" term="정제" value={status} values={filters.statuses} labeller={statusLabel} onChange={onStatus} />
      <SelectFilter label="품목" term="품목" value={itemName} values={filters.item_names} onChange={onItemName} />
      <SelectFilter label="수입사" term="수입사" value={importer} values={filters.importers} onChange={onImporter} />
      <SelectFilter label="제조 국가" term="제조 국가" value={country} values={filters.countries} onChange={onCountry} />
      <SelectFilter label="확인 사유" term="확인 사유" value={reason} values={filters.reason_codes} labeller={reasonLabel} onChange={onReason} />
      <span className="filter-bar__count">{number(page?.total)}건</span>
    </section>
    {loading && <Loading label="목록을 읽는 중" />}{error && <ErrorPanel message={error} />}
    {page && <><DeclarationTable entries={page.declarations} onOpen={onOpen} /><div className="pagination"><button disabled={pageNumber <= 1} onClick={() => onPage(pageNumber - 1)}>이전</button><span>{pageNumber} / {totalPages}</span><button disabled={pageNumber >= totalPages} onClick={() => onPage(pageNumber + 1)}>다음</button></div></>}
  </div>
}

function ReviewView({ quality, loading, error, declarations, onOpen }: { quality: Quality | null; loading: boolean; error: string | null; declarations: DeclarationPage | null; onOpen: (entry: Declaration) => void }) {
  const reviewItems = declarations?.declarations ?? []
  const distributions = quality?.review_distributions
  return <div className="page">
    <PageTitle title="사람이 봐야 하는 건" copy="값끼리 어긋나거나 뜻이 여러 갈래여서 자동 판단을 멈춘 건입니다. 값을 지어내지 않고 원문을 그대로 둔 채 이유를 남겼습니다." />
    {loading && <Loading label="집계를 읽는 중" />}{error && <ErrorPanel message={error} />}
    {quality && <>
      <div className="content-grid">
        <article className="panel panel--wide"><div className="panel__head"><h2><Term>확인 사유</Term> 순위</h2></div><BarChart data={quality.review_reasons.map((entry) => ({ name: reasonLabel(entry.code), count: entry.count }))} accent="red" /></article>
        <article className="panel"><h2>보는 순서</h2><p className="muted">건수가 많은 사유부터 봅니다. 같은 사유끼리는 판단 기준이 같아서 한 번에 처리할 수 있습니다. 이 화면에서는 값을 고칠 수 없습니다.</p></article>
      </div>
      <section className="review-distributions" aria-label="확인 필요 건의 분포">{([['품목', distributions?.items, undefined], ['수입사', distributions?.importers, undefined], ['제조 국가', distributions?.countries, undefined], ['정제 상태', distributions?.statuses, statusLabel]] as Array<[string, Array<{ name: string; count: number }> | undefined, ((value: string) => string) | undefined]>).map(([label, data, labeller]) => <article className="panel" key={label}><h2>{label}별 분포</h2><BarChart data={(data ?? []).map((entry) => ({ ...entry, name: labeller ? labeller(entry.name) : entry.name }))} /></article>)}</section>
    </>}
    <section className="panel review-list"><div className="panel__head"><h2>확인 대상 목록</h2><span className="muted">줄을 눌러 그 건의 값을 전부 확인</span></div>{reviewItems.length ? <DeclarationTable entries={reviewItems} onOpen={onOpen} compact /> : <p className="empty-inline">확인이 필요한 건이 없습니다.</p>}</section>
  </div>
}

function QualityView({ quality, loading, error }: { quality: Quality | null; loading: boolean; error: string | null }) {
  const duplicates = quality?.duplicate_observations ?? []
  return <div className="page">
    <PageTitle title="원장 전체 통계" copy="얼마나 모았고, 어떤 항목을 얼마나 읽어냈는지 봅니다." />
    {loading && <Loading label="통계를 읽는 중" />}{error && <ErrorPanel message={error} />}
    {quality && <>
      <div className="quality-grid">
        <article className="panel"><h2>월별 수집량</h2><p className="panel__lead">식약처 처리일 기준으로 매달 몇 건을 모았는지입니다.</p><Sparkline data={quality.monthly_collections} /></article>
        <article className="panel"><h2><Term>품목</Term>별 건수</h2><BarChart data={quality.item_distribution} accent="blue" /></article>
        <article className="panel"><h2>정제 상태별 건수</h2><BarChart data={Object.entries(quality.status_distribution).map(([name, count]) => ({ name: statusLabel(name), count }))} /></article>
      </div>
      <section className="content-grid quality-coverage">
        <article className="panel"><h2><Term>추출 성공률</Term></h2><p className="panel__lead">항목별로 값을 실제 읽어낸 비율입니다. 낮은 항목은 원본에 그 정보가 애초에 없었다는 뜻입니다.</p><BarChart data={Object.entries(quality.field_coverage).map(([name, count]) => ({ name: fieldLabel(name), count }))} unit="percent" /></article>
        <article className="panel"><h2><Term>같은 제품 묶음</Term> 후보</h2><p className="panel__lead">제품 이름과 병 용량이 같아 한 제품으로 보이는 묶음입니다. 확정이 아니라 후보입니다.</p><BarChart data={(quality.sku_groups ?? []).map((entry) => ({ name: entry.name, count: entry.declarations }))} accent="blue" /></article>
      </section>
      <section className="panel">
        <div className="panel__head"><h2>여러 번 신고된 같은 제품 후보</h2><p className="muted">같은 제품이라고 확정한 목록이 아닙니다.</p></div>
        <div className="sku-table">
          <div className="sku-table__head"><span>제품 묶음</span><Hint text="이 묶음에 들어 있는 수입신고 건수입니다."><span>신고 건수</span></Hint><Hint text="같은 신고 건을 여러 날에 걸쳐 다시 수집한 횟수까지 더한 값입니다."><span>수집 횟수</span></Hint></div>
          {duplicates.length ? duplicates.map((entry) => <div key={entry.name}><strong>{entry.name}</strong><span>{number(entry.declarations)}</span><span>{number(entry.observations)}</span></div>) : <p className="empty-inline">여러 번 신고된 후보가 없습니다.</p>}
        </div>
      </section>
    </>}
  </div>
}

function DeclarationTable({ entries, onOpen, compact = false }: { entries: Declaration[]; onOpen: (entry: Declaration) => void; compact?: boolean }) {
  if (!entries.length) return <div className="empty-state"><h2>조건에 맞는 신고 건이 없습니다.</h2><p>검색어나 필터를 바꿔 다시 확인하세요.</p></div>
  return <div className={`declaration-table ${compact ? 'declaration-table--compact' : ''}`}>
    <div className="declaration-table__header">
      <Hint text="식약처 원장에 적힌 제품명 그대로입니다. 용량, 도수, 관리번호가 뒤섞여 있습니다."><span>원본 제품명</span></Hint>
      <Hint text="원본에서 용량과 도수를 제자리에 넣고 다시 조립한 이름입니다."><span>정제된 이름</span></Hint>
      <Hint text="용량, 도수, 관리번호를 모두 떼어낸 순수한 제품 이름입니다."><span>제품 이름</span></Hint>
      <Hint text="병 한 개의 용량입니다. 6병 세트여도 한 병 기준입니다."><span>병 하나 용량</span></Hint>
      <Hint text="숙성 연수, 빈티지, 도수, 에디션, 자재 코드처럼 같은 제품을 갈라놓는 값입니다."><span>제품 특성</span></Hint>
      <Hint text="이 건을 자동으로 어디까지 해석했는지입니다."><span>정제 상태</span></Hint>
      <Hint text="식약처가 이 신고 건을 처리한 날짜입니다."><span>처리일</span></Hint>
    </div>
    {entries.map((entry) => <button className="declaration-row" key={entry.rcno} onClick={() => onOpen(entry)} aria-label={`${entry.source_name} 상세 열기`}>
      <span data-label="원본 제품명" title={entry.source_name}>{entry.source_name}</span>
      <span data-label="정제된 이름" title={entry.normalized_name}>{entry.normalized_name || '—'}</span>
      <span data-label="제품 이름">{entry.key_1 || '—'}</span>
      <span data-label="병 하나 용량">{entry.key_2 || '—'}</span>
      <span data-label="제품 특성">{entry.key_3 || '—'}</span>
      <span data-label="정제 상태"><StatusChip status={entry.status} /></span>
      <span data-label="처리일">{date(entry.processed_at)}</span>
    </button>)}
  </div>
}

function SelectFilter({ label, term, value, values, labeller, onChange }: { label: string; term?: string; value: string; values: string[]; labeller?: (value: string) => string; onChange: (value: string) => void }) {
  return <label><Hint text={hintFor(term ?? label)}>{label}</Hint><select value={value} onChange={(event) => onChange(event.target.value)} aria-label={label}><option value="">{label} 전체</option>{values.map((item) => <option value={item} key={item}>{labeller ? labeller(item) : item}</option>)}</select></label>
}
function Sparkline({ data }: { data: Array<{ month: string; count: number }> }) { const max = Math.max(...data.map((item) => item.count), 1); const points = data.map((item, index) => `${data.length === 1 ? 50 : (index / (data.length - 1)) * 100},${100 - (item.count / max) * 84}`).join(' '); return <>{data.length ? <svg className="sparkline" viewBox="0 0 100 100" preserveAspectRatio="none" aria-label="월별 수집량 추이" role="img"><polyline points={points} /></svg> : <p className="empty-inline">표시할 데이터가 없습니다.</p>}<div className="spark-labels">{data.slice(0, 1).map((item) => <span key={item.month}>{item.month}</span>)}{data.slice(-1).map((item) => <span key={item.month}>{item.month}</span>)}</div></> }
function PageTitle({ title, copy }: { title: string; copy: string }) { return <div className="page-title"><div><h1>{title}</h1></div><p>{copy}</p></div> }
function Loading({ label }: { label: string }) { return <div className="state state--loading" role="status"><span />{label}</div> }
function ErrorPanel({ message }: { message: string }) { return <div className="state state--error" role="alert"><b>조회할 수 없습니다.</b><span>{message}</span></div> }
