import { number } from '../format'

export function BarChart({ data, accent = 'copper' }: { data: Array<{ name: string; count: number }>; accent?: 'copper' | 'blue' | 'red' }) {
  const max = Math.max(...data.map((entry) => entry.count), 1)
  if (!data.length) return <p className="empty-inline">표시할 데이터가 없습니다.</p>
  return <div className="bar-chart">
    {data.map((entry) => <div className="bar-row" key={entry.name}>
      <span className="bar-label" title={entry.name}>{entry.name}</span>
      <span className="bar-track"><span className={`bar-fill bar-fill--${accent}`} style={{ width: `${(entry.count / max) * 100}%` }} /></span>
      <strong>{number(entry.count)}</strong>
    </div>)}
  </div>
}
