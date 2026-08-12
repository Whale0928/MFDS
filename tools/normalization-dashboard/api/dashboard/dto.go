package dashboard

type statusCount struct {
	Status string `json:"status"`
	Count  int64  `json:"count"`
}
type fieldCoverage struct {
	Field      string  `json:"field"`
	Populated  int64   `json:"populated"`
	Total      int64   `json:"total"`
	Percentage float64 `json:"percentage"`
}

type overviewResponse struct {
	TotalRCNO              int64              `json:"total_rcno"`
	PreservedRCNO          int64              `json:"preserved_rcno"`
	NormalizedCount        int64              `json:"normalized_count"`
	ReviewRequiredCount    int64              `json:"review_required_count"`
	LatestProcessedAt      string             `json:"latest_processed_at"`
	StatusCounts           map[string]int64   `json:"status_counts"`
	FieldCoverage          map[string]float64 `json:"field_coverage"`
	LedgerPreservationRate float64            `json:"ledger_preservation_rate"`
	ReviewRequiredRatio    float64            `json:"review_required_ratio"`
}

type declarationListItem struct {
	RCNO              string   `json:"rcno"`
	SourceName        string   `json:"source_name"`
	NormalizedName    string   `json:"normalized_name"`
	Key1              string   `json:"key_1"`
	Key2              string   `json:"key_2"`
	Key3              string   `json:"key_3"`
	Status            string   `json:"status"`
	ProcessedAt       string   `json:"processed_at"`
	ItemName          string   `json:"item_name"`
	ImporterName      string   `json:"importer_name"`
	Country           string   `json:"country"`
	AlcoholMatched    bool     `json:"alcohol_matched"`
	DistilleryMatched bool     `json:"distillery_matched"`
	RegionMatched     bool     `json:"region_matched"`
	ReasonCodes       []string `json:"reason_codes"`
}
type declarationListResponse struct {
	Declarations []declarationListItem `json:"declarations"`
	Page         int                   `json:"page"`
	PageSize     int                   `json:"page_size"`
	Total        int64                 `json:"total"`
	TotalPages   int                   `json:"total_pages"`
}

type importerListItem struct {
	LicenseNo         string `json:"license_no"`
	BusinessName      string `json:"business_name"`
	Representative    string `json:"representative_name"`
	PermitDate        string `json:"permit_date"`
	InstitutionName   string `json:"institution_name"`
	Address           string `json:"address"`
	Telephone         string `json:"telephone"`
	IndustryName      string `json:"industry_name"`
	ClosureStatusName string `json:"closure_status_name"`
	ClosureDate       string `json:"closure_date"`
	ObservedAt        string `json:"observed_at"`
	Source            string `json:"source"`
}

type importerListResponse struct {
	Importers  []importerListItem `json:"importers"`
	Page       int                `json:"page"`
	PageSize   int                `json:"page_size"`
	Total      int64              `json:"total"`
	TotalPages int                `json:"total_pages"`
}
type declarationDetail struct {
	RCNO                  string                    `json:"rcno"`
	SourceName            string                    `json:"source_name"`
	SourceNameEnglish     string                    `json:"source_name_english"`
	NormalizedName        string                    `json:"normalized_name"`
	NormalizedNameEnglish string                    `json:"normalized_name_english"`
	Status                string                    `json:"status"`
	ReasonCodes           []string                  `json:"reason_codes"`
	Evidence              []evidenceItem            `json:"evidence"`
	ProcessedAt           string                    `json:"processed_at"`
	ItemName              string                    `json:"item_name"`
	ImporterName          string                    `json:"importer_name"`
	Country               string                    `json:"country"`
	Fields                map[string]string         `json:"fields"`
	Groups                []detailGroup             `json:"groups"`
	MatchingCandidates    []matchingCandidateDetail `json:"matching_candidates"`
}

type matchingCandidateDetail struct {
	TargetType string                   `json:"target_type"`
	TargetID   int64                    `json:"target_id"`
	Rank       int                      `json:"rank"`
	NameKO     string                   `json:"name_ko"`
	NameEN     string                   `json:"name_en"`
	Score      float64                  `json:"score"`
	Evidence   []matchingEvidenceDetail `json:"evidence"`
}

type matchingEvidenceDetail struct {
	FeatureCode      string  `json:"feature_code"`
	Source           string  `json:"source"`
	InputValue       string  `json:"input_value"`
	ReferenceValue   string  `json:"reference_value"`
	RuleCode         string  `json:"rule_code"`
	Weight           float64 `json:"weight"`
	UpstreamTargetID int64   `json:"upstream_target_id"`
}

type detailField struct {
	Label string `json:"label"`
	Hint  string `json:"hint"`
	Value string `json:"value"`
}
type detailGroup struct {
	Title  string        `json:"title"`
	Side   string        `json:"side"`
	Fields []detailField `json:"fields"`
}

type countPoint struct {
	Name  string `json:"name"`
	Count int64  `json:"count"`
}
type monthlyPoint struct {
	Month string `json:"month"`
	Count int64  `json:"count"`
}
type evidenceItem struct {
	Label           string `json:"label"`
	RawValue        string `json:"raw_value"`
	NormalizedValue string `json:"normalized_value"`
	ReasonCode      string `json:"reason_code"`
}
type reviewDistributions struct {
	Items     []countPoint `json:"items"`
	Importers []countPoint `json:"importers"`
	Countries []countPoint `json:"countries"`
	Statuses  []countPoint `json:"statuses"`
}
type candidateGroup struct {
	DisplayName  string `json:"name"`
	UnitVolumeML int64  `json:"unit_volume_ml"`
	Declarations int64  `json:"declarations"`
	Importers    int64  `json:"importers"`
}
type duplicateObservation struct {
	Name         string `json:"name"`
	Declarations int64  `json:"declarations"`
	Observations int64  `json:"observations"`
}
type qualityResponse struct {
	MonthlyCollections    []monthlyPoint         `json:"monthly_collections"`
	ItemDistribution      []countPoint           `json:"item_distribution"`
	FieldCoverage         map[string]float64     `json:"field_coverage"`
	StatusDistribution    map[string]int64       `json:"status_distribution"`
	ReviewReasons         []reasonPoint          `json:"review_reasons"`
	ReviewDistributions   reviewDistributions    `json:"review_distributions"`
	SKUGroups             []candidateGroup       `json:"sku_groups"`
	DuplicateObservations []duplicateObservation `json:"duplicate_observations"`
}
type reasonPoint struct {
	Code  string `json:"code"`
	Count int64  `json:"count"`
}
