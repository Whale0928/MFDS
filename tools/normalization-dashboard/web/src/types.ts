export type Status = 'NORMALIZED' | 'PARTIAL' | 'REVIEW_REQUIRED' | 'UNPARSED' | 'PENDING' | 'STALE' | string

export interface Overview {
  total_rcno: number
  preserved_rcno: number
  normalized_count: number
  review_required_count: number
  status_counts: Record<string, number>
  field_coverage: Record<string, number>
  latest_processed_at?: string
}

export interface Filters {
  statuses: string[]
  item_names: string[]
  importers: string[]
  countries: string[]
  reason_codes: string[]
}

export interface Declaration {
  rcno: string
  source_name: string
  normalized_name?: string
  key_1?: string
  key_2?: string
  key_3?: string
  status: Status
  processed_at?: string
  item_name?: string
  importer_name?: string
  country?: string
  alcohol_matched?: boolean
  distillery_matched?: boolean
  region_matched?: boolean
  reason_codes?: string[]
  evidence?: DetailEvidence[]
  fields?: Record<string, string>
  groups?: DetailGroup[]
  official_company_records?: OfficialRecord[]
  matching_candidates?: MatchingCandidate[]
}

export type MatchFilter = '' | 'matched' | 'unmatched'
export type DeclarationSort = 'processed_at' | 'alcohol' | 'distillery' | 'region'
export type SortOrder = 'asc' | 'desc'

export interface MatchingCandidate {
  target_type: 'ALCOHOL' | 'DISTILLERY' | 'REGION' | string
  target_id: number
  rank: number
  name_ko: string
  name_en: string
  score: number
  evidence: MatchingEvidence[]
}

export interface MatchingEvidence {
  feature_code: string
  source: string
  input_value: string
  reference_value: string
  rule_code: string
  weight: number
  upstream_target_id: number
}

export interface DetailField {
  label: string
  hint: string
  value: string
}

export interface DetailGroup {
  title: string
  side: 'ledger' | 'normalized' | string
  fields: DetailField[]
}

export interface Evidence {
  label: string
  raw_value?: string
  normalized_value?: string
  reason_code?: string
}

export type DetailEvidence = Evidence | string

export interface DeclarationPage {
  declarations: Declaration[]
  total: number
  page: number
  page_size: number
}

export interface Quality {
  monthly_collections: Array<{ month: string; count: number }>
  item_distribution: Array<{ name: string; count: number }>
  field_coverage: Record<string, number>
  status_distribution: Record<string, number>
  review_reasons: Array<{ code: string; count: number }>
  review_distributions?: {
    items: Array<{ name: string; count: number }>
    importers: Array<{ name: string; count: number }>
    countries: Array<{ name: string; count: number }>
    statuses: Array<{ name: string; count: number }>
  }
  duplicate_observations?: Array<{ name: string; declarations: number; observations: number }>
  sku_groups?: Array<{ name: string; unit_volume_ml: number; declarations: number; importers: number }>
}

export interface DeclarationQuery {
  page: number
  page_size: number
  q?: string
  status?: string
  item_name?: string
  importer?: string
  country?: string
  reason?: string
  alcohol_match?: MatchFilter
  distillery_match?: MatchFilter
  region_match?: MatchFilter
  sort?: DeclarationSort
  order?: SortOrder
}

export interface OfficialRecord {
  source_type: 'BUSINESS_LICENSE' | 'CLOSURE' | 'EXCELLENT_IMPORTER' | 'DISPOSITION' | string
  source_name: string
  observed_at: string
  fields: DetailField[]
}

export interface Company {
  business_name: string
  license_number: string
  industry_name: string
  address: string
  latest_observed_at: string
  has_business_license: boolean
  has_closure: boolean
  has_excellent_importer: boolean
  has_disposition: boolean
}

export interface CompanyPage {
  companies: Company[]
  total: number
  page: number
  page_size: number
  total_pages: number
}

export interface CompanyDetail {
  business_name: string
  license_number: string
  records: OfficialRecord[]
}
