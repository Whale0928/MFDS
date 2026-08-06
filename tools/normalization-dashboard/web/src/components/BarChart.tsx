import { number, percent } from '../format'

// unit이 'percent'면 값을 비율로 읽고 막대 길이도 100 기준으로 그린다.
export function BarChart({ data, accent = 'copper', unit = 'count' }: { data: Array<{ name: string; count: number }>; accent?: 'copper' | 'blue' | 'red'; unit?: 'count' | 'percent' }) {
  const max = unit === 'percent' ? 100 : Math.max(...data.map((entry) => entry.count), 1)
  if (!data.length) return <p className="empty-inline">표시할 데이터가 없습니다.</p>
  return <div className="bar-chart">
    {data.map((entry) => <div className="bar-row" key={entry.name}>
      <span className="bar-label" title={entry.name}>{entry.name}</span>
      <span className="bar-track"><span className={`bar-fill bar-fill--${accent}`} style={{ width: `${(entry.count / max) * 100}%` }} /></span>
      <strong>{unit === 'percent' ? percent(entry.count) : number(entry.count)}</strong>
    </div>)}
  </div>
}
