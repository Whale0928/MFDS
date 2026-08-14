export type Status = 'NORMALIZED' | 'PARTIAL' | 'REVIEW_REQUIRED' | 'UNPARSED' | 'PENDING' | 'STALE' | string

export interface Overview {
  total_rcno: number
  preserved_rcno: number
  normalized_count: number
  review_required_count: number
  status_counts: Record<string, number>
  field_coverage: Record<string, number>
  latest_processed_at: string
  ledger_preservation_rate: number
  review_required_ratio: number
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
  source_importer_name?: string
  importer_id?: number
  importer_business_name?: string
  importer_linked?: boolean
  importer_link_source?: 'AUTO' | 'ADMIN' | string
  country?: string
  alcohol_matched?: boolean
  distillery_matched?: boolean
  region_matched?: boolean
  reason_codes?: string[]
  evidence?: DetailEvidence[]
  fields?: Record<string, string>
  groups?: DetailGroup[]
  matching_candidates?: MatchingCandidate[]
}

export type MatchFilter = '' | 'matched' | 'unmatched'
export type ImporterLinkFilter = '' | 'matched' | 'unlinked'
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
  total_pages: number
}

export interface ImporterGroup {
  importer_id: number
  official_business_code: string
  license_no: string
  business_name: string
  representative_name: string
  permit_date: string
  institution_name: string
  primary_address: string
  telephone: string
  industry_name: string
  operating_status: string
  source_detail_url: string
  description: string
  admin_status: string
  source_observed_at: string
  updated_at: string
  declaration_count: number
  product_count: number
  first_import_date: string
  last_import_date: string
}

export interface ImporterPage {
  importers: ImporterGroup[]
  total: number
  page: number
  page_size: number
  total_pages: number
}

export interface ImporterStatistics {
  declaration_count: number
  product_count: number
  first_import_date: string
  last_import_date: string
}

export interface ImporterDetail {
  importer_id: number
  business_name: string
  profile: ImporterProfile
  statistics: ImporterStatistics
}

export interface ImporterProfile {
  importer_id: number
  official_business_code: string
  license_no: string
  business_name: string
  business_name_key_sha256: string
  representative_name: string
  permit_date: string
  institution_name: string
  primary_address: string
  telephone: string
  industry_name: string
  operating_status: string
  source_list_url: string
  source_detail_url: string
  source_list_sha256: string
  source_detail_sha256: string
  source_observed_at: string
  description: string
  admin_note: string
  admin_status: string
  reviewed_by: string
  reviewed_at: string
  created_at: string
  updated_at: string
}

export interface ImporterProductGroup {
  product_key: string
  product_name: string
  declaration_count: number
  first_import_date: string
  last_import_date: string
}

export interface ImporterProductPage {
  products: ImporterProductGroup[]
  page: number
  page_size: number
  total: number
  total_pages: number
}

export interface ImporterLedgerItem {
  rcno: string
  source_name: string
  source_name_english: string
  processed_at: string
  item_name: string
  manufacturer_name: string
  country: string
  volume: string
  abv: string
  link_source: string
}

export interface ImporterLedgerPage {
  declarations: ImporterLedgerItem[]
  page: number
  page_size: number
  total: number
  total_pages: number
}

export interface ImporterQuery {
  page: number
  page_size: number
  q?: string
  matched_only?: boolean
  operating_status?: string
  industry_name?: string
  sort?: 'business_name' | 'declarations' | 'last_import' | 'observed_at'
  order?: SortOrder
}

export interface MissingImporter {
  missing_importer_id: number
  source_importer_name: string
  source_name_key_sha256: string
  match_status: 'MISSING' | 'AMBIGUOUS' | string
  candidate_count: number
  candidates: Array<Record<string, unknown>>
  declaration_count: number
  sample_rcno: string
  first_processed_date: string
  last_processed_date: string
  source_list_url: string
  source_list_sha256: string
  source_observed_at: string
  description: string
  admin_note: string
  admin_status: 'OPEN' | 'RESOLVED' | 'DISMISSED' | string
  resolved_importer_id: number
  resolved_importer_business_name: string
  resolution_source: string
  reviewed_by: string
  reviewed_at: string
  created_at: string
  updated_at: string
}

export interface MissingImporterPage {
  missing_importers: MissingImporter[]
  page: number
  page_size: number
  total: number
  total_pages: number
}

export interface MissingImporterQuery {
  page: number
  page_size: number
  q?: string
  match_status?: 'MISSING' | 'AMBIGUOUS'
  admin_status?: 'OPEN' | 'RESOLVED' | 'DISMISSED'
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
  importer_link?: ImporterLinkFilter
  importer_id?: number
  from?: string
  to?: string
  alcohol_match?: MatchFilter
  distillery_match?: MatchFilter
  region_match?: MatchFilter
  sort?: DeclarationSort
  order?: SortOrder
}
