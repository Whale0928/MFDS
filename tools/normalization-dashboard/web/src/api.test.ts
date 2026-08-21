import { describe, expect, it } from 'vitest'
import { queryString } from './api'

describe('queryString', () => {
  it('수입사_미연결필터_API계약값을유지한다', () => {
    expect(queryString({ importer_link: 'unlinked' })).toBe('?importer_link=unlinked')
  })

  it('수입사_ID_상세조회계약을구성한다', () => {
    expect(queryString({ importer_id: 7, business_name: '' })).toBe('?importer_id=7')
  })
})
