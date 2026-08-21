package dashboard

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strings"
	"testing"
)

type fakeQueryer struct {
	queries []string
	args    [][]any
	respond func(string, []any) (RowIterator, error)
}

func (f *fakeQueryer) QueryContext(_ context.Context, query string, args ...any) (RowIterator, error) {
	f.queries = append(f.queries, query)
	f.args = append(f.args, args)
	return f.respond(query, args)
}

type fakeRows struct {
	values [][]any
	index  int
	err    error
}

func (r *fakeRows) Close() error { return nil }
func (r *fakeRows) Err() error   { return r.err }
func (r *fakeRows) Next() bool {
	if r.index >= len(r.values) {
		return false
	}
	r.index++
	return true
}
func (r *fakeRows) Scan(dest ...any) error {
	if r.index == 0 || r.index > len(r.values) {
		return fmt.Errorf("Scan called without a row")
	}
	row := r.values[r.index-1]
	if len(row) != len(dest) {
		return fmt.Errorf("column count: got %d want %d", len(row), len(dest))
	}
	for i := range dest {
		destination := reflect.ValueOf(dest[i])
		if destination.Kind() != reflect.Pointer || destination.IsNil() {
			return fmt.Errorf("destination %d is not a pointer", i)
		}
		value := reflect.ValueOf(row[i])
		if !value.IsValid() {
			return fmt.Errorf("nil value at column %d", i)
		}
		if value.Type().AssignableTo(destination.Elem().Type()) {
			destination.Elem().Set(value)
			continue
		}
		return fmt.Errorf("column %d: %T cannot scan into %T", i, row[i], dest[i])
	}
	return nil
}

func TestImporters_usesImporterAndDeclarationRelationsOnly(t *testing.T) {
	queryer := &fakeQueryer{respond: func(query string, args []any) (RowIterator, error) {
		switch {
		case strings.Contains(query, "SELECT COUNT(*) FROM grouped_importers"):
			if len(args) != 1 || args[0] != "CLOSED" {
				return nil, fmt.Errorf("unexpected count args: %#v", args)
			}
			return &fakeRows{values: [][]any{{int64(1)}}}, nil
		case strings.Contains(query, "SELECT g.importer_id"):
			if len(args) != 3 || args[0] != "CLOSED" || args[1] != 20 || args[2] != 20 {
				return nil, fmt.Errorf("unexpected list args: %#v", args)
			}
			return &fakeRows{values: [][]any{{int64(7), "BIZ-7", "LIC-7", "수입사", "대표자", "2021-01-02", "서울청", "서울시", "02", "수입판매업", "CLOSED", "https://detail", "설명", "ACTIVE", "2026-08-12T20:00:00", "2026-08-12T21:00:00", int64(42), int64(17), "2023-01-03", "2026-08-11"}}}, nil
		default:
			return nil, fmt.Errorf("unexpected query: %s", query)
		}
	}}
	response := httptest.NewRecorder()
	NewServer(queryer).Handler().ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/importers?operating_status=CLOSED&page=2&page_size=20", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", response.Code, response.Body.String())
	}
	var body importerListResponse
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body.Total != 1 || len(body.Importers) != 1 || body.Importers[0].OfficialBusinessCode != "BIZ-7" || body.Importers[0].DeclarationCount != 42 {
		t.Fatalf("unexpected importer list: %#v", body)
	}
	for _, query := range queryer.queries {
		for _, forbidden := range []string{"mfds_importer_licenses", "mfds_importer_evidence", "_raw", "C001", "I2821", "I0250", "I0470"} {
			if strings.Contains(query, forbidden) {
				t.Fatalf("legacy source %q found in query: %s", forbidden, query)
			}
		}
		if strings.Contains(strings.ToUpper(query), "INSERT ") || strings.Contains(strings.ToUpper(query), "UPDATE ") || strings.Contains(strings.ToUpper(query), "DELETE ") {
			t.Fatalf("write query found: %s", query)
		}
	}
}

func TestImporterDetail_exposesOfficialProfileAndLedgerStatistics(t *testing.T) {
	queryer := &fakeQueryer{respond: func(query string, args []any) (RowIterator, error) {
		if strings.Contains(query, "SELECT id, official_business_code") {
			return &fakeRows{values: [][]any{{int64(7), "BIZ-7", "LIC-7", "수입사", "NAME-HASH", "대표자", "2021-01-02", "서울청", "서울시", "02", "수입판매업", "ACTIVE", "https://list", "https://detail", "AA", "BB", "2026-08-12T20:00:00", "설명", "메모", "ACTIVE", "admin", "2026-08-13T20:00:00", "2026-08-01T20:00:00", "2026-08-13T20:00:00"}}}, nil
		}
		if strings.Contains(query, "FROM mfds_declaration_details") {
			return &fakeRows{values: [][]any{{int64(42), int64(17), "2023-01-03", "2026-08-11"}}}, nil
		}
		return nil, fmt.Errorf("unexpected query: %s", query)
	}}
	response := httptest.NewRecorder()
	NewServer(queryer).Handler().ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/importers/detail?importer_id=7", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", response.Code, response.Body.String())
	}
	var body importerDetailResponse
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body.Profile.OfficialBusinessCode != "BIZ-7" || body.Profile.BusinessNameKeySHA256 != "NAME-HASH" || body.Profile.SourceListSHA256 != "AA" || body.Profile.AdminNote != "메모" || body.Statistics.DeclarationCount != 42 {
		t.Fatalf("unexpected importer detail: %#v", body)
	}
}

func TestImporterLedger_internalPaginationAndLinkSource(t *testing.T) {
	productKey := "name:" + strings.Repeat("a", 64)
	queryer := &fakeQueryer{respond: func(query string, args []any) (RowIterator, error) {
		if strings.HasPrefix(query, "SELECT EXISTS") {
			if len(args) != 3 || args[0] != int64(7) || args[1] != int64(7) || args[2] != productKey {
				return nil, fmt.Errorf("unexpected count args: %#v", args)
			}
			return &fakeRows{values: [][]any{{true, int64(12)}}}, nil
		}
		if strings.Contains(query, "SELECT rcno") {
			return &fakeRows{values: [][]any{{"RCNO-1", "원문", "EN", "2026-08-01", "위스키", "증류소", "영국", "700 mL", "40%", "PAGE_NAME"}}}, nil
		}
		return nil, fmt.Errorf("unexpected query: %s", query)
	}}
	response := httptest.NewRecorder()
	target := "/api/importers/product-declarations?importer_id=7&product_key=" + url.QueryEscape(productKey) + "&page=2&page_size=10"
	NewServer(queryer).Handler().ServeHTTP(response, httptest.NewRequest(http.MethodGet, target, nil))
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", response.Code, response.Body.String())
	}
	var body importerLedgerPage
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body.Total != 12 || body.Page != 2 || body.Declarations[0].LinkSource != "PAGE_NAME" {
		t.Fatalf("unexpected ledger page: %#v", body)
	}
}

func TestDeclarations_exposesImporterLinkSource(t *testing.T) {
	queryer := &fakeQueryer{respond: func(query string, args []any) (RowIterator, error) {
		if strings.HasPrefix(query, "SELECT COUNT(*)") {
			return &fakeRows{values: [][]any{{int64(1)}}}, nil
		}
		if strings.Contains(query, "FROM mfds_declaration_details") {
			return &fakeRows{values: [][]any{{"RCNO-1", "원본", "정제", "700 mL", "40%", "", "NORMALIZED", "2026-08-01", "위스키", "정제 수입사", "원장 수입사", int64(7), "정제 수입사", true, "PAGE_RCNO", "영국", true, false, true, `[]`}}}, nil
		}
		return nil, fmt.Errorf("unexpected query: %s args=%#v", query, args)
	}}
	response := httptest.NewRecorder()
	NewServer(queryer).Handler().ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/declarations?page=1&page_size=20", nil))
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"importer_link_source":"PAGE_RCNO"`) {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestValidateRequiredSchema_requiresImporterTablesDeclarationAndDetailViewColumns(t *testing.T) {
	ready := &fakeQueryer{respond: func(query string, _ []any) (RowIterator, error) {
		if query != requiredSchemaSQL {
			return nil, fmt.Errorf("unexpected query: %s", query)
		}
		return &fakeRows{values: [][]any{{2, 2, 2}}}, nil
	}}
	if err := ValidateRequiredSchema(context.Background(), ready); err != nil {
		t.Fatal(err)
	}
	incomplete := &fakeQueryer{respond: func(string, []any) (RowIterator, error) {
		return &fakeRows{values: [][]any{{1, 1, 1}}}, nil
	}}
	if err := ValidateRequiredSchema(context.Background(), incomplete); err == nil {
		t.Fatal("expected schema guard error")
	}
}

func TestDashboardImporterQueries_haveNoWriteOperations(t *testing.T) {
	queries := []string{importerBaseCTE, importerListSQL, importerProfileSQL, importerStatisticsSQL, importerProductGroupSQL, importerLedgerListSQL, requiredSchemaSQL}
	for _, query := range queries {
		upper := strings.ToUpper(query)
		for _, forbidden := range []string{"INSERT ", "UPDATE ", "DELETE ", "REPLACE ", "MFDS_IMPORTER_LICENSES", "MFDS_IMPORTER_EVIDENCE", "MFDS_C001_IMPORTER_LICENSES_RAW", "MFDS_I2821_IMPORTER_CLOSURES_RAW", "MFDS_I0250_EXCELLENT_IMPORTERS_RAW", "MFDS_I0470_ADMINISTRATIVE_DISPOSITIONS_RAW"} {
			if strings.Contains(upper, forbidden) {
				t.Fatalf("forbidden token %q in query: %s", forbidden, query)
			}
		}
	}
}

func TestCORS_rejectsNonLoopbackOrigin(t *testing.T) {
	request := httptest.NewRequest(http.MethodOptions, "/healthz", nil)
	request.Header.Set("Origin", "http://example.test")
	response := httptest.NewRecorder()
	NewServer(&fakeQueryer{}).Handler().ServeHTTP(response, request)
	if response.Code != http.StatusForbidden {
		t.Fatalf("status = %d", response.Code)
	}
}

func TestImporterFilter_validatesPaginationAndFilters(t *testing.T) {
	filter, page, size, err := parseImporterListRequest(urlValues("q", "서울", "matched_only", "true", "operating_status", "ACTIVE", "industry_name", "수입판매업", "sort", "declarations", "order", "desc", "page", "3", "page_size", "10"))
	if err != nil || page != 3 || size != 10 || !filter.MatchedOnly {
		t.Fatalf("parsed request = %#v %d %d %v", filter, page, size, err)
	}
	where, args := filter.where(true)
	for _, clause := range []string{"g.business_name LIKE ? ESCAPE '='", "stats.importer_id IS NOT NULL", "g.operating_status = ?", "g.industry_name = ?"} {
		if !strings.Contains(where, clause) {
			t.Fatalf("missing %s in %s", clause, where)
		}
	}
	if len(args) != 7 || !strings.Contains(filter.orderBy(), "COALESCE(stats.declaration_count, 0) DESC") {
		t.Fatalf("where args/order = %q %#v", filter.orderBy(), args)
	}
}

func TestContainsLikePattern_와일드카드를문자그대로검색한다(t *testing.T) {
	t.Parallel()

	if actual := containsLikePattern("100%_A=B"); actual != "%100=%=_A==B%" {
		t.Fatalf("containsLikePattern() = %q", actual)
	}
}

func urlValues(parts ...string) url.Values {
	values := url.Values{}
	for i := 0; i < len(parts); i += 2 {
		values[parts[i]] = []string{parts[i+1]}
	}
	return values
}
