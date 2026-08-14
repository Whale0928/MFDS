import { useCallback, useEffect, useMemo, useState } from 'react'
import { api } from './api'
import { date, fieldLabel, number, reasonLabel, statusLabel } from './format'
import { BarChart } from './components/BarChart'
import { DetailDrawer } from './components/DetailDrawer'
import { Hint, Term } from './components/Hint'
import { ImporterDetailDialog, ImporterList, MissingImporterQueue } from './components/ImporterList'
import { hintFor } from './glossary'
import { StatusChip } from './components/StatusChip'
import type { Declaration, DeclarationPage, DeclarationSort, Filters, ImporterGroup, ImporterLinkFilter, MatchFilter, Quality, SortOrder } from './types'

type Section = 'browse' | 'importers' | 'missing' | 'review' | 'quality'
type MatchingSort = `${DeclarationSort}:${SortOrder}`
const PAGE_SIZE = 20
const blankFilters: Filters = { statuses: [], item_names: [], importers: [], countries: [], reason_codes: [] }
type BrowseFilters = { query: string; status: string; itemName: string; importer: string; country: string; reason: string; importerLink: ImporterLinkFilter; processedFrom: string; processedTo: string; alcoholMatch: MatchFilter; distilleryMatch: MatchFilter; regionMatch: MatchFilter; matchingSort: MatchingSort }
const initialBrowseFilters: BrowseFilters = { query: '', status: '', itemName: '', importer: '', country: '', reason: '', importerLink: '', processedFrom: '', processedTo: '', alcoholMatch: '', distilleryMatch: '', regionMatch: '', matchingSort: 'processed_at:desc' }

function useRemote<T>(load: () => Promise<T>, initial: T | null) {
  const [data, setData] = useState<T | null>(initial)
  const [error, setError] = useState<string | null>(null)
  const [loading, setLoading] = useState(true)
  useEffect(() => { let alive = true; setLoading(true); load().then((result) => { if (alive) { setData(result); setError(null) } }).catch((reason: unknown) => { if (alive) setError(reason instanceof Error ? reason.message : '데이터를 불러오지 못했습니다.') }).finally(() => { if (alive) setLoading(false) }); return () => { alive = false } }, [load])
  return { data, error, loading }
}

function useDebouncedValue<T>(value: T, delay: number) {
  const [debounced, setDebounced] = useState(value)
  useEffect(() => { const timer = window.setTimeout(() => setDebounced(value), delay); return () => window.clearTimeout(timer) }, [value, delay])
  return debounced
}

export default function App() {
  const [section, setSection] = useState<Section>('browse')
  const [browseFilters, setBrowseFilters] = useState<BrowseFilters>(initialBrowseFilters)
  const [page, setPage] = useState(1)
  const [reviewPage, setReviewPage] = useState(1)
  const [selected, setSelected] = useState<Declaration | null>(null)
  const [selectedImporter, setSelectedImporter] = useState<Pick<ImporterGroup, 'importer_id' | 'business_name'> | null>(null)
  const [activeOverlay, setActiveOverlay] = useState<'declaration' | 'importer' | null>(null)
  const [detailError, setDetailError] = useState<string | null>(null)
  const filterLoad = useMemo(() => api.filters, [])
  const qualityLoad = useMemo(() => api.quality, [])
  const debouncedQuery = useDebouncedValue(browseFilters.query, 300)
  const declarationLoad = useMemo(() => {
    const [sort, order] = browseFilters.matchingSort.split(':') as [DeclarationSort, SortOrder]
    return () => api.declarations({ page, page_size: PAGE_SIZE, q: debouncedQuery || undefined, status: browseFilters.status || undefined, item_name: browseFilters.itemName || undefined, importer: browseFilters.importer || undefined, country: browseFilters.country || undefined, reason: browseFilters.reason || undefined, importer_link: browseFilters.importerLink || undefined, from: browseFilters.processedFrom || undefined, to: browseFilters.processedTo || undefined, alcohol_match: browseFilters.alcoholMatch || undefined, distillery_match: browseFilters.distilleryMatch || undefined, region_match: browseFilters.regionMatch || undefined, sort, order })
  }, [page, debouncedQuery, browseFilters.status, browseFilters.itemName, browseFilters.importer, browseFilters.country, browseFilters.reason, browseFilters.importerLink, browseFilters.processedFrom, browseFilters.processedTo, browseFilters.alcoholMatch, browseFilters.distilleryMatch, browseFilters.regionMatch, browseFilters.matchingSort])
  const reviewLoad = useMemo(() => () => api.declarations({ page: reviewPage, page_size: PAGE_SIZE, status: 'REVIEW_REQUIRED' }), [reviewPage])
  const filters = useRemote<Filters>(filterLoad, blankFilters)
  const quality = useRemote<Quality>(qualityLoad, null)
  const declarations = useRemote<DeclarationPage>(declarationLoad, null)
  const reviews = useRemote<DeclarationPage>(reviewLoad, null)
  const openDetail = async (entry: Declaration) => {
    setDetailError(null)
    try {
      setSelected(await api.declaration(entry.rcno))
      setActiveOverlay('declaration')
    } catch {
      setSelected(null)
      setDetailError('이 건의 상세 값을 불러오지 못했습니다.')
    }
  }
  const openDetailByRCNO = async (rcno: string) => {
    await openDetail({ rcno, source_name: rcno, status: '' })
  }
  const openImporter = (importerID: number, businessName: string) => {
    if (!importerID || !businessName) return
    setSelectedImporter({ importer_id: importerID, business_name: businessName })
    setActiveOverlay('importer')
  }
  const closeDeclaration = useCallback(() => {
    setSelected(null)
    setActiveOverlay(selectedImporter ? 'importer' : null)
  }, [selectedImporter])
  const closeImporter = useCallback(() => {
    setSelectedImporter(null)
    setActiveOverlay(selected ? 'declaration' : null)
  }, [selected])
  const updateBrowseFilter = <K extends keyof BrowseFilters>(key: K, value: BrowseFilters[K]) => {
    setBrowseFilters((current) => ({ ...current, [key]: value }))
    setPage(1)
  }

  return <main className="shell">
    <header className="masthead">
      <a className="brand" href="#browse" onClick={() => setSection('browse')}><span>MFDS</span><b>수입주류 신고 원장</b><i>시연용</i></a>
      <nav aria-label="화면 이동">{([['browse', '데이터 탐색'], ['importers', '수입사 목록'], ['missing', '미매칭 관리'], ['review', '확인 필요 목록'], ['quality', '전체 통계']] as Array<[Section, string]>).map(([key, label]) => <button key={key} className={section === key ? 'active' : ''} onClick={() => setSection(key)}>{label}</button>)}</nav>
      <p className="masthead__note">읽기 전용 / 이 화면에서는 아무 값도 고치지 않습니다</p>
    </header>

    {section === 'browse' && <BrowseView page={declarations.data} loading={declarations.loading} error={declarations.error} filters={filters.data ?? blankFilters} values={browseFilters} onFilter={updateBrowseFilter} onReset={() => { setBrowseFilters(initialBrowseFilters); setPage(1) }} onOpen={openDetail} onOpenImporter={openImporter} pageNumber={page} onPage={setPage} />}
    {section === 'importers' && <ImporterList onOpenImporter={(importer) => openImporter(importer.importer_id, importer.business_name)} />}
    {section === 'missing' && <MissingImporterQueue />}
    {section === 'review' && <ReviewView quality={quality.data} loading={quality.loading || reviews.loading} error={quality.error ?? reviews.error} declarations={reviews.data} onOpen={openDetail} onOpenImporter={openImporter} pageNumber={reviewPage} onPage={setReviewPage} />}
    {section === 'quality' && <QualityView quality={quality.data} loading={quality.loading} error={quality.error} />}

    {detailError && <div className="detail-error" role="alert"><span>{detailError}</span><button onClick={() => setDetailError(null)}>닫기</button></div>}
    {selected && <div className={`overlay-host${activeOverlay === 'declaration' ? '' : ' overlay-host--suspended'}`}><DetailDrawer declaration={selected} onClose={closeDeclaration} onOpenImporter={openImporter} /></div>}
    {selectedImporter && <div className={`overlay-host${activeOverlay === 'importer' ? '' : ' overlay-host--suspended'}`}><ImporterDetailDialog importer={selectedImporter} active={activeOverlay === 'importer'} onClose={closeImporter} onOpenDeclaration={openDetailByRCNO} /></div>}
  </main>
}

function BrowseView({ page, loading, error, filters, values, onFilter, onReset, onOpen, onOpenImporter, pageNumber, onPage }: { page: DeclarationPage | null; loading: boolean; error: string | null; filters: Filters; values: BrowseFilters; onFilter: <K extends keyof BrowseFilters>(key: K, value: BrowseFilters[K]) => void; onReset: () => void; onOpen: (entry: Declaration) => void; onOpenImporter: (importerID: number, businessName: string) => void; pageNumber: number; onPage: (value: number) => void }) {
  const totalPages = Math.max(1, page?.total_pages ?? 0)
  const activeFilters = Object.entries(values).filter(([key, value]) => value && key !== 'matchingSort').length
  return <div className="page">
    <PageTitle title="식약처 원본과 정제 결과" copy="한 줄이 수입신고 한 건입니다. 줄을 누르면 그 건의 원본 값과 정제된 값을 빠짐없이 볼 수 있습니다." />
    <section className="filter-bar filter-bar--primary">
      <label>통합 검색<input value={values.query} onChange={(event) => onFilter('query', event.target.value)} placeholder="제품명 · RCNO · 상호" maxLength={200} /></label>
      <SelectFilter label="정제 상태" term="정제" value={values.status} values={filters.statuses} labeller={statusLabel} onChange={(value) => onFilter('status', value)} />
      <SelectFilter label="품목" term="품목" value={values.itemName} values={filters.item_names} onChange={(value) => onFilter('itemName', value)} />
      <SelectFilter label="수입사" term="수입사" value={values.importer} values={filters.importers} onChange={(value) => onFilter('importer', value)} />
      <SelectFilter label="제조 국가" term="제조 국가" value={values.country} values={filters.countries} onChange={(value) => onFilter('country', value)} />
      <SelectFilter label="확인 사유" term="확인 사유" value={values.reason} values={filters.reason_codes} labeller={reasonLabel} onChange={(value) => onFilter('reason', value)} />
      <span className="filter-bar__count">{number(page?.total)}건</span>
    </section>
    <section className="filter-bar ledger-filter-bar" aria-label="원장 관계와 기간 필터">
      <div className="matching-filter-bar__title"><strong>원장 범위</strong><small>수입사 연결과 처리일</small></div>
      <label>수입사 연결<select value={values.importerLink} onChange={(event) => onFilter('importerLink', event.target.value as ImporterLinkFilter)}><option value="">전체</option><option value="matched">정제 수입사 연결</option><option value="unlinked">미연결</option></select></label>
      <label>처리일 시작<input type="date" value={values.processedFrom} max={values.processedTo || undefined} onChange={(event) => onFilter('processedFrom', event.target.value)} /></label>
      <label>처리일 종료<input type="date" value={values.processedTo} min={values.processedFrom || undefined} onChange={(event) => onFilter('processedTo', event.target.value)} /></label>
      <button className="filter-reset" type="button" onClick={onReset} disabled={activeFilters === 0}>필터 초기화 {activeFilters > 0 && `(${activeFilters})`}</button>
    </section>
    <section className="filter-bar matching-filter-bar" aria-label="BottleNote 매칭 필터">
      <div className="matching-filter-bar__title"><strong>BottleNote 후보</strong><small>1순위 후보 생성 여부</small></div>
      <MatchSelect label="알코올 매칭" value={values.alcoholMatch} onChange={(value) => onFilter('alcoholMatch', value)} />
      <MatchSelect label="증류소 매칭" value={values.distilleryMatch} onChange={(value) => onFilter('distilleryMatch', value)} />
      <MatchSelect label="리전 매칭" value={values.regionMatch} onChange={(value) => onFilter('regionMatch', value)} />
      <label>정렬<select value={values.matchingSort} onChange={(event) => onFilter('matchingSort', event.target.value as MatchingSort)} aria-label="매칭 여부 정렬"><option value="processed_at:desc">최신 처리일순</option><option value="alcohol:desc">알코올 후보 있음 먼저</option><option value="alcohol:asc">알코올 후보 없음 먼저</option><option value="distillery:desc">증류소 후보 있음 먼저</option><option value="distillery:asc">증류소 후보 없음 먼저</option><option value="region:desc">리전 후보 있음 먼저</option><option value="region:asc">리전 후보 없음 먼저</option></select></label>
    </section>
    {loading && <Loading label="목록을 읽는 중" />}{error && <ErrorPanel message={error} />}
    {page && <><DeclarationTable entries={page.declarations} onOpen={onOpen} onOpenImporter={onOpenImporter} /><div className="pagination"><button disabled={pageNumber <= 1} onClick={() => onPage(pageNumber - 1)}>이전</button><span>{pageNumber} / {totalPages}</span><button disabled={pageNumber >= totalPages} onClick={() => onPage(pageNumber + 1)}>다음</button></div></>}
  </div>
}

function ReviewView({ quality, loading, error, declarations, onOpen, onOpenImporter, pageNumber, onPage }: { quality: Quality | null; loading: boolean; error: string | null; declarations: DeclarationPage | null; onOpen: (entry: Declaration) => void; onOpenImporter: (importerID: number, businessName: string) => void; pageNumber: number; onPage: (value: number) => void }) {
  const reviewItems = declarations?.declarations ?? []
  const reviewTotal = declarations?.total ?? 0
  const reviewPages = Math.max(1, Math.ceil(reviewTotal / PAGE_SIZE))
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
    <section className="panel review-list">
      <div className="panel__head"><h2>확인 대상 목록 {number(reviewTotal)}건</h2><span className="muted">줄을 눌러 그 건의 값을 전부 확인</span></div>
      {reviewItems.length ? <><DeclarationTable entries={reviewItems} onOpen={onOpen} onOpenImporter={onOpenImporter} compact /><div className="pagination"><button disabled={pageNumber <= 1} onClick={() => onPage(pageNumber - 1)}>이전</button><span>{pageNumber} / {reviewPages}</span><button disabled={pageNumber >= reviewPages} onClick={() => onPage(pageNumber + 1)}>다음</button></div></> : <p className="empty-inline">확인이 필요한 건이 없습니다.</p>}
    </section>
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

function DeclarationTable({ entries, onOpen, onOpenImporter, compact = false }: { entries: Declaration[]; onOpen: (entry: Declaration) => void; onOpenImporter?: (importerID: number, businessName: string) => void; compact?: boolean }) {
  if (!entries.length) return <div className="empty-state"><h2>조건에 맞는 신고 건이 없습니다.</h2><p>검색어나 필터를 바꿔 다시 확인하세요.</p></div>
  return <div className={`declaration-table ${compact ? 'declaration-table--compact' : ''}`}>
    <div className="declaration-table__header">
      <Hint text="식약처 원장에 적힌 제품명 그대로입니다. 용량, 도수, 관리번호가 뒤섞여 있습니다."><span>원본 제품명</span></Hint>
      <Hint text="원본에서 용량과 도수를 제자리에 넣고 다시 조립한 이름입니다."><span>정제된 이름</span></Hint>
      <Hint text="용량, 도수, 관리번호를 모두 떼어낸 순수한 제품 이름입니다."><span>제품 이름</span></Hint>
      <Hint text="병 한 개의 용량입니다. 6병 세트여도 한 병 기준입니다."><span>병 하나 용량</span></Hint>
      <Hint text="숙성 연수, 빈티지, 도수, 에디션, 자재 코드처럼 같은 제품을 갈라놓는 값입니다."><span>제품 특성</span></Hint>
      <Hint text="BottleNote 기준 데이터와 비교해 1순위 후보가 만들어졌는지 보여줍니다. 확정 선택 여부와는 다릅니다."><span>BottleNote 후보</span></Hint>
      <Hint text="이 건을 자동으로 어디까지 해석했는지입니다."><span>정제 상태</span></Hint>
      <Hint text="식약처가 이 신고 건을 처리한 날짜입니다."><span>처리일</span></Hint>
    </div>
    {entries.map((entry) => <div className="declaration-row" key={entry.rcno} onClick={() => onOpen(entry)}>
      <span className="declaration-row__identity" data-label="원본 제품명" title={entry.source_name}><strong>{entry.source_name}</strong>{entry.importer_linked && entry.importer_id && entry.importer_business_name
        ? <button type="button" className="importer-inline-link" onClick={(event) => { event.stopPropagation(); onOpenImporter?.(entry.importer_id!, entry.importer_business_name!) }}>수입사 · {entry.importer_business_name}</button>
        : <small className="importer-inline-link importer-inline-link--missing">수입사 미연결 · {entry.source_importer_name || entry.importer_name || '원문 없음'}</small>}</span>
      <span data-label="정제된 이름" title={entry.normalized_name}>{entry.normalized_name || '—'}</span>
      <span data-label="제품 이름">{entry.key_1 || '—'}</span>
      <span data-label="병 하나 용량">{entry.key_2 || '—'}</span>
      <span data-label="제품 특성">{entry.key_3 || '—'}</span>
      <span data-label="BottleNote 후보"><MatchingRail entry={entry} /></span>
      <span data-label="정제 상태"><StatusChip status={entry.status} /></span>
      <span data-label="처리일">{date(entry.processed_at)}</span>
      <button type="button" className="declaration-row__open" onClick={(event) => { event.stopPropagation(); onOpen(entry) }} aria-label={`${entry.source_name} 신고 상세 열기`} />
    </div>)}
  </div>
}

function MatchingRail({ entry }: { entry: Declaration }) {
  const matches = [['알코올', entry.alcohol_matched], ['증류소', entry.distillery_matched], ['리전', entry.region_matched]] as Array<[string, boolean]>
  return <span className="matching-rail" aria-label={matches.map(([label, matched]) => `${label} 후보 ${matched ? '있음' : '없음'}`).join(', ')}>{matches.map(([label, matched]) => <span className={`matching-flag matching-flag--${matched ? 'candidate' : 'missing'}`} key={label}><b>{label}</b><i>{matched ? '후보' : '없음'}</i></span>)}</span>
}

function MatchSelect({ label, value, onChange }: { label: string; value: MatchFilter; onChange: (value: MatchFilter) => void }) {
  return <label>{label}<select value={value} onChange={(event) => onChange(event.target.value as MatchFilter)} aria-label={label}><option value="">전체</option><option value="matched">후보 있음</option><option value="unmatched">후보 없음</option></select></label>
}

function SelectFilter({ label, term, value, values, labeller, onChange }: { label: string; term?: string; value: string; values: string[]; labeller?: (value: string) => string; onChange: (value: string) => void }) {
  return <label><Hint text={hintFor(term ?? label)}>{label}</Hint><select value={value} onChange={(event) => onChange(event.target.value)} aria-label={label}><option value="">{label} 전체</option>{values.map((item) => <option value={item} key={item}>{labeller ? labeller(item) : item}</option>)}</select></label>
}
function Sparkline({ data }: { data: Array<{ month: string; count: number }> }) { const max = Math.max(...data.map((item) => item.count), 1); const points = data.map((item, index) => `${data.length === 1 ? 50 : (index / (data.length - 1)) * 100},${100 - (item.count / max) * 84}`).join(' '); return <>{data.length ? <svg className="sparkline" viewBox="0 0 100 100" preserveAspectRatio="none" aria-label="월별 수집량 추이" role="img"><polyline points={points} /></svg> : <p className="empty-inline">표시할 데이터가 없습니다.</p>}<div className="spark-labels">{data.slice(0, 1).map((item) => <span key={item.month}>{item.month}</span>)}{data.slice(-1).map((item) => <span key={item.month}>{item.month}</span>)}</div></> }
function PageTitle({ title, copy }: { title: string; copy: string }) { return <div className="page-title"><div><h1>{title}</h1></div><p>{copy}</p></div> }
function Loading({ label }: { label: string }) { return <div className="state state--loading" role="status"><span />{label}</div> }
function ErrorPanel({ message }: { message: string }) { return <div className="state state--error" role="alert"><b>조회할 수 없습니다.</b><span>{message}</span></div> }
