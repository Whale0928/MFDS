export function number(value?: number) { return new Intl.NumberFormat('ko-KR').format(value ?? 0) }
export function percent(value?: number) { return `${Math.round((value ?? 0) * 10) / 10}%` }
export function date(value?: string) {
  if (!value) return '—'
  const parsed = new Date(value)
  return Number.isNaN(parsed.valueOf()) ? value : new Intl.DateTimeFormat('ko-KR', { dateStyle: 'medium' }).format(parsed)
}
export function statusLabel(status: string) {
  return ({ NORMALIZED: '정제 완료', PARTIAL: '일부 추출', REVIEW_REQUIRED: '검토 필요', UNPARSED: '미해석', PENDING: '대기', STALE: '재정제 필요' } as Record<string, string>)[status] ?? status
}
