import { describe, expect, it } from 'vitest'
import { date, percent, statusLabel } from './format'

describe('presentation formatters', () => {
  it('formats status and safe fallback values', () => {
    expect(statusLabel('REVIEW_REQUIRED')).toBe('검토 필요')
    expect(statusLabel('UNKNOWN')).toBe('UNKNOWN')
    expect(percent(12.34)).toBe('12.3%')
    expect(date()).toBe('—')
  })
})
