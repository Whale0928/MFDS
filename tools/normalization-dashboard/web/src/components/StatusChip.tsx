import { statusLabel } from '../format'

export function StatusChip({ status }: { status: string }) {
  return <span className={`status status--${status.toLowerCase()}`}>{statusLabel(status)}</span>
}
