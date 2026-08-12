import { type FormEvent, useEffect, useState } from 'react'
import { api } from '../api'
import { number } from '../format'
import type { Importer, ImporterPage } from '../types'

const PAGE_SIZE = 20

export function ImporterList() {
  const [input, setInput] = useState('')
  const [query, setQuery] = useState('')
  const [page, setPage] = useState(1)
  const [data, setData] = useState<ImporterPage | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    let alive = true
    setLoading(true)
    api.importers({ page, page_size: PAGE_SIZE, q: query || undefined })
      .then((result) => {
        if (!alive) return
        setData(result)
        setError(null)
      })
      .catch((reason: unknown) => {
        if (!alive) return
        setError(reason instanceof Error ? reason.message : '수입사 정보를 불러오지 못했습니다.')
      })
      .finally(() => { if (alive) setLoading(false) })
    return () => { alive = false }
  }, [page, query])

  const search = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault()
    setPage(1)
    setQuery(input.trim())
  }
  const totalPages = Math.max(1, data?.total_pages ?? 0)

  return <div className="page importer-page">
    <div className="page-title importer-page__title">
      <div><p className="eyebrow">OFFICIAL IMPORTER LICENSE REGISTER</p><h1>수입사 목록</h1></div>
      <p>수입 기록과 연결하기 전, 식약처에 신고된 수입판매업 인허가 정보만 따로 확인합니다.</p>
    </div>

    <section className="importer-register-note" aria-label="수입사 목록 기준">
      <div><span>목록 기준</span><strong>인허가번호 하나당 한 줄</strong></div>
      <p>같은 상호가 여러 번 보여도 임의로 한 회사로 합치지 않았습니다. 현재 화면에는 수입 기록이나 수입 제품을 섞지 않습니다.</p>
      <span className="importer-source">C001(수입식품 영업신고 정보)</span>
    </section>

    <form className="importer-search" onSubmit={search}>
      <label htmlFor="importer-search">수입사 검색</label>
      <div><input id="importer-search" value={input} onChange={(event) => setInput(event.target.value)} maxLength={200} placeholder="상호명, 인허가번호 또는 주소" /><button type="submit">검색</button></div>
      <span aria-live="polite">{number(data?.total)}개 인허가</span>
    </form>

    {loading && <div className="state state--loading" role="status"><span />수입사 목록을 읽는 중</div>}
    {error && <div className="state state--error" role="alert"><b>수입사 목록을 조회할 수 없습니다.</b><span>{error}</span></div>}
    {!loading && !error && data && <>
      <ImporterRegister importers={data.importers} />
      <div className="pagination"><button disabled={page <= 1} onClick={() => setPage(page - 1)}>이전</button><span>{page} / {totalPages}</span><button disabled={page >= totalPages} onClick={() => setPage(page + 1)}>다음</button></div>
    </>}
  </div>
}

function ImporterRegister({ importers }: { importers: Importer[] }) {
  if (!importers.length) return <div className="empty-state"><h2>조건에 맞는 수입사가 없습니다.</h2><p>상호명, 인허가번호 또는 주소를 바꿔 검색하세요.</p></div>
  return <section className="importer-register" aria-label="수입사 인허가 목록">
    <div className="importer-register__header"><span>수입사 / 인허가번호</span><span>허가일</span><span>관할기관</span><span>대표자</span><span>소재지</span><span>폐업정보</span></div>
    {importers.map((importer) => <article className="importer-record" key={importer.license_no}>
      <div className="importer-record__identity" data-label="수입사 / 인허가번호"><strong>{importer.business_name || '상호명 미기재'}</strong><code>{importer.license_no}</code></div>
      <span data-label="허가일">{value(importer.permit_date)}</span>
      <span data-label="관할기관">{value(importer.institution_name)}</span>
      <span data-label="대표자">{value(importer.representative_name)}</span>
      <span data-label="소재지" title={importer.address}>{value(importer.address)}</span>
      <span data-label="폐업정보" className={importer.closure_status_name ? 'importer-record__closed' : 'importer-record__unconfirmed'}>{importer.closure_status_name ? `${importer.closure_status_name}${importer.closure_date ? ` · ${importer.closure_date}` : ''}` : '확인된 정보 없음'}</span>
    </article>)}
  </section>
}

function value(text: string) { return text || '—' }
