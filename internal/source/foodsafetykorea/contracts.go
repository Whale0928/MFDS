package foodsafetykorea

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

const (
	APIBaseURL          = "https://openapi.foodsafetykorea.go.kr"
	MaskedAPIKey        = "REDACTED"
	MaxPageSize         = 1000
	MaxResponseBodySize = int64(16 << 20)
	DefaultTimeout      = 20 * time.Second
)

type ServiceID string

const (
	ServiceC001  ServiceID = "C001"
	ServiceI2821 ServiceID = "I2821"
	ServiceI0250 ServiceID = "I0250"
	ServiceI0470 ServiceID = "I0470"
)

type Result struct {
	Message string `json:"MSG"`
	Code    string `json:"CODE"`
}

type Wrapper struct {
	TotalCount string          `json:"total_count"`
	Row        json.RawMessage `json:"row"`
	Result     Result          `json:"RESULT"`
}

type PageRequest struct {
	StartIndex int
	EndIndex   int
	Filters    map[string]string
}

type RequestSnapshot struct {
	Method     string            `json:"method"`
	URL        string            `json:"url"`
	ServiceID  ServiceID         `json:"service_id"`
	StartIndex int               `json:"start_index"`
	EndIndex   int               `json:"end_index"`
	Filters    map[string]string `json:"filters,omitempty"`
}

type HTTPMetadata struct {
	Method          string        `json:"method"`
	URL             string        `json:"url"`
	StatusCode      int           `json:"status_code"`
	ResponseHeaders http.Header   `json:"response_headers,omitempty"`
	ContentType     string        `json:"content_type,omitempty"`
	StartedAt       time.Time     `json:"started_at"`
	FinishedAt      time.Time     `json:"finished_at"`
	Duration        time.Duration `json:"duration"`
	BodySize        int64         `json:"body_size"`
	BodyTruncated   bool          `json:"body_truncated"`
}

type Page[T any] struct {
	ServiceID    ServiceID       `json:"service_id"`
	TotalCount   string          `json:"total_count"`
	Rows         []T             `json:"rows"`
	Result       Result          `json:"result"`
	RawBody      []byte          `json:"raw_body"`
	HTTP         HTTPMetadata    `json:"http"`
	Request      RequestSnapshot `json:"request"`
	SnapshotJSON []byte          `json:"snapshot_json"`
}

type ErrorKind string

const (
	ErrorKindValidation ErrorKind = "VALIDATION"
	ErrorKindNetwork    ErrorKind = "NETWORK"
	ErrorKindHTTP       ErrorKind = "HTTP"
	ErrorKindBodyLimit  ErrorKind = "BODY_LIMIT"
	ErrorKindEmptyBody  ErrorKind = "EMPTY_BODY"
	ErrorKindHTML       ErrorKind = "HTML_RESPONSE"
	ErrorKindNonJSON    ErrorKind = "NON_JSON_RESPONSE"
	ErrorKindContract   ErrorKind = "CONTRACT"
	ErrorKindAPI        ErrorKind = "API"
)

type AdapterError struct {
	Kind    ErrorKind
	Op      string
	Message string
}

func (e *AdapterError) Error() string {
	return fmt.Sprintf("식품안전나라 OPEN-API %s 실패 [%s]: %s", e.Op, e.Kind, e.Message)
}

type C001Row struct {
	PresidentName   string          `json:"PRSDNT_NM"`
	PermissionDate  string          `json:"PRMS_DT"`
	LicenseNumber   string          `json:"LCNS_NO"`
	InstitutionName string          `json:"INSTT_NM"`
	BusinessName    string          `json:"BSSH_NM"`
	LocationAddress string          `json:"LOCP_ADDR"`
	TelephoneNumber string          `json:"TELNO"`
	IndustryName    string          `json:"INDUTY_NM"`
	RawJSON         json.RawMessage `json:"-"`
}

func (row *C001Row) UnmarshalJSON(data []byte) error {
	type wire C001Row
	var decoded wire
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	*row = C001Row(decoded)
	row.RawJSON = append(row.RawJSON[:0], data...)
	return nil
}

type I2821Row struct {
	ClosureDate         string          `json:"CLSBIZ_DT"`
	PresidentName       string          `json:"PRSDNT_NM"`
	PermissionDate      string          `json:"PRMS_DT"`
	LicenseNumber       string          `json:"LCNS_NO"`
	InstitutionName     string          `json:"INSTT_NM"`
	BusinessName        string          `json:"BSSH_NM"`
	ClosureDivisionName string          `json:"CLSBIZ_DVS_CD_NM"`
	LocationAddress     string          `json:"LOCP_ADDR"`
	IndustryName        string          `json:"INDUTY_NM"`
	RawJSON             json.RawMessage `json:"-"`
}

func (row *I2821Row) UnmarshalJSON(data []byte) error {
	type wire I2821Row
	var decoded wire
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	*row = I2821Row(decoded)
	row.RawJSON = append(row.RawJSON[:0], data...)
	return nil
}

type I0250Row struct {
	ExportCountryName            string          `json:"EXCOURY_NATN_CD_NM"`
	ImportedProductExportCompany string          `json:"INCM_PRDT_XPORT_MC_NM"`
	PermissionDate               string          `json:"PRMS_DT"`
	ProductCount                 string          `json:"PRDLST_CNT"`
	LicenseNumber                string          `json:"LCNS_NO"`
	ProductName                  string          `json:"PRDLST_NM"`
	ExcellentImporterNumber      string          `json:"EXCLNC_INCM_BSSH_REGNO"`
	BusinessName                 string          `json:"BSSH_NM"`
	Address                      string          `json:"ADDR"`
	RawJSON                      json.RawMessage `json:"-"`
}

func (row *I0250Row) UnmarshalJSON(data []byte) error {
	type wire I0250Row
	var decoded wire
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	*row = I0250Row(decoded)
	row.RawJSON = append(row.RawJSON[:0], data...)
	return nil
}

type I0470Row struct {
	PresidentName           string          `json:"PRSDNT_NM"`
	LastUpdatedAt           string          `json:"LAST_UPDT_DTM"`
	LicenseNumber           string          `json:"LCNS_NO"`
	DispositionAgencyName   string          `json:"DSPS_INSTTCD_NM"`
	LawOrderName            string          `json:"LAWORD_CD_NM"`
	DispositionDetailSeq    string          `json:"DSPSDTLS_SEQ"`
	Violation               string          `json:"VILTCN"`
	Address                 string          `json:"ADDR"`
	PublishedAt             string          `json:"PUBLIC_DT"`
	IndustryName            string          `json:"INDUTY_CD_NM"`
	DispositionDecisionDate string          `json:"DSPS_DCSNDT"`
	BusinessNameAtDecision  string          `json:"PRCSCITYPOINT_BSSHNM"`
	DispositionStartDate    string          `json:"DSPS_BGNDT"`
	DispositionTypeName     string          `json:"DSPS_TYPECD_NM"`
	DispositionEndDate      string          `json:"DSPS_ENDDT"`
	TelephoneNumber         string          `json:"TELNO"`
	DispositionContent      string          `json:"DSPSCN"`
	RawJSON                 json.RawMessage `json:"-"`
}

func (row *I0470Row) UnmarshalJSON(data []byte) error {
	type wire I0470Row
	var decoded wire
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	*row = I0470Row(decoded)
	row.RawJSON = append(row.RawJSON[:0], data...)
	return nil
}
