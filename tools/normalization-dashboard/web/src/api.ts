import type { Declaration, DeclarationPage, DeclarationQuery, Filters, ImporterPage, ImporterQuery, Overview, Quality } from './types'

const API_BASE = import.meta.env.VITE_MFDS_API_BASE ?? '/api'

async function request<T>(path: string): Promise<T> {
  const response = await fetch(`${API_BASE}${path}`, { headers: { Accept: 'application/json' } })
  if (!response.ok) throw new Error(`데이터를 불러오지 못했습니다. (${response.status})`)
  return response.json() as Promise<T>
}

function queryString(query: object) {
  const params = new URLSearchParams()
  Object.entries(query).forEach(([key, value]) => { if (typeof value === 'string' || typeof value === 'number') { if (value !== '') params.set(key, String(value)) } })
  const result = params.toString()
  return result ? `?${result}` : ''
}

export const api = {
  overview: () => request<Overview>('/overview'),
  filters: () => request<Filters>('/filters'),
  declarations: (query: DeclarationQuery) => request<DeclarationPage>(`/declarations${queryString(query)}`),
  declaration: (rcno: string) => request<Declaration>(`/declarations/${encodeURIComponent(rcno)}`),
  importers: (query: ImporterQuery) => request<ImporterPage>(`/importers${queryString(query)}`),
  quality: () => request<Quality>('/quality'),
}
