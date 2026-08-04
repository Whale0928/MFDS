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
	respond func(string, []any) (RowIterator, error)
}

func (f *fakeQueryer) QueryContext(_ context.Context, query string, args ...any) (RowIterator, error) {
	f.queries = append(f.queries, query)
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
		reflect.ValueOf(dest[i]).Elem().Set(reflect.ValueOf(row[i]))
	}
	return nil
}

func TestOverview_terminalStatusesAreNormalized(t *testing.T) {
	queryer := &fakeQueryer{respond: func(query string, _ []any) (RowIterator, error) {
		switch {
		case strings.Contains(query, "MAX(source_processed_date)"):
			return &fakeRows{values: [][]any{{int64(11924), int64(9780), int64(11924), "2026-08-05"}}}, nil
		case strings.Contains(query, "GROUP BY normalization_status"):
			return &fakeRows{values: [][]any{{"NORMALIZED", int64(2144)}, {"PARTIAL", int64(3)}, {"REVIEW_REQUIRED", int64(9775)}, {"UNPARSED", int64(2)}}}, nil
		case strings.Contains(query, "field_name"):
			return &fakeRows{values: [][]any{{"abv", int64(100), int64(200)}}}, nil
		default:
			return nil, fmt.Errorf("unexpected query: %s", query)
		}
	}}
	request := httptest.NewRequest(http.MethodGet, "/api/overview", nil)
	response := httptest.NewRecorder()
	NewServer(queryer).Handler().ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", response.Code, response.Body.String())
	}
	var body overviewResponse
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body.TotalRCNO != 11924 || body.NormalizedCount != 11924 || body.ReviewRequiredCount != 9775 {
		t.Fatalf("unexpected overview: %#v", body)
	}
	if body.FieldCoverage["abv"] != 50 {
		t.Fatalf("coverage = %#v", body.FieldCoverage)
	}
}

func TestDeclarations_usesReadOnlyPaginationAndSnakeCase(t *testing.T) {
	queryer := &fakeQueryer{respond: func(query string, args []any) (RowIterator, error) {
		switch {
		case strings.HasPrefix(query, "SELECT COUNT(*)"):
			if len(args) != 4 {
				return nil, fmt.Errorf("count args = %d", len(args))
			}
			return &fakeRows{values: [][]any{{int64(1)}}}, nil
		case strings.Contains(query, "FROM declaration_details"):
			if len(args) != 6 {
				return nil, fmt.Errorf("list args = %d", len(args))
			}
			return &fakeRows{values: [][]any{{"202600000001", "원본명", "정제명", "700 · 40%", "12년", "", "NORMALIZED", "2026-08-01", "위스키", "수입사", "영국", `["AMBIGUOUS"]`}}}, nil
		default:
			return nil, fmt.Errorf("unexpected query")
		}
	}}
	request := httptest.NewRequest(http.MethodGet, "/api/declarations?search=%EC%9B%90%EB%B3%B8&status=NORMALIZED&page=2&page_size=20", nil)
	response := httptest.NewRecorder()
	NewServer(queryer).Handler().ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", response.Code, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), `"declarations"`) || !strings.Contains(response.Body.String(), `"source_name"`) || strings.Contains(response.Body.String(), `"internal_path"`) {
		t.Fatalf("unexpected public response: %s", response.Body.String())
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

func TestDeclarations_supportsQAndReasonFilter(t *testing.T) {
	filter, page, size, err := parseListRequest(urlValues("q", "우선", "search", "별칭", "reason", "AMBIGUOUS", "page", "3", "page_size", "10"))
	if err != nil || filter.Search != "우선" || filter.Reason != "AMBIGUOUS" || page != 3 || size != 10 {
		t.Fatalf("parsed request = %#v, %d, %d, %v", filter, page, size, err)
	}
	where, args := filter.where()
	if !strings.Contains(where, "JSON_CONTAINS(normalization_reasons") || len(args) != 4 || args[3] != "AMBIGUOUS" {
		t.Fatalf("reason filter = %q %#v", where, args)
	}
}

func TestDeclarationDetail_evidenceIsPublicObjectContract(t *testing.T) {
	queryer := &fakeQueryer{respond: func(query string, _ []any) (RowIterator, error) {
		if !strings.Contains(query, "FROM declaration_details WHERE rcno") {
			return nil, fmt.Errorf("unexpected query")
		}
		return &fakeRows{values: [][]any{{"202600000001", "원본", "source", "정제", "normalized", "REVIEW_REQUIRED", `["AMBIGUOUS"]`, `["700ml? "]`, "2026-08-01", "위스키", "수입사", "영국", "700ml", "700 mL", "40", "40%", "12", "12 years", "", "", "한정", "LOT-1", "B-2", "수입사", "증류소"}}}, nil
	}}
	request := httptest.NewRequest(http.MethodGet, "/api/declarations/202600000001", nil)
	response := httptest.NewRecorder()
	NewServer(queryer).Handler().ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", response.Code, response.Body.String())
	}
	var body map[string]any
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	evidence, ok := body["evidence"].([]any)
	if !ok || len(evidence) == 0 {
		t.Fatalf("evidence = %#v", body["evidence"])
	}
	first, ok := evidence[0].(map[string]any)
	if !ok || first["label"] != "용량" || first["raw_value"] != "700ml" || first["normalized_value"] != "700 mL" {
		t.Fatalf("evidence item = %#v", evidence[0])
	}
}

func TestQualityAndKeyContracts(t *testing.T) {
	if !strings.Contains(declarationListSQL, "base_product_name_ko") || !strings.Contains(declarationListSQL, "unit_volume_ml") || strings.Contains(declarationListSQL, "lot_number") {
		t.Fatalf("list key SQL does not preserve the public key contract: %s", declarationListSQL)
	}
	payload, err := json.Marshal(qualityResponse{
		MonthlyCollections:    []monthlyPoint{{Month: "2026-08", Count: 3}},
		FieldCoverage:         map[string]float64{"abv": 50},
		StatusDistribution:    map[string]int64{"NORMALIZED": 3},
		ReviewReasons:         []reasonPoint{{Code: "AMBIGUOUS", Count: 1}},
		SKUGroups:             []candidateGroup{{DisplayName: "상품", UnitVolumeML: 700, Declarations: 2, Importers: 1}},
		DuplicateObservations: []duplicateObservation{{Name: "위스키", Declarations: 2, Observations: 4}},
	})
	if err != nil {
		t.Fatal(err)
	}
	text := string(payload)
	for _, expected := range []string{`"month":"2026-08"`, `"field_coverage":{"abv":50}`, `"status_distribution":{"NORMALIZED":3}`, `"code":"AMBIGUOUS"`, `"name":"상품"`, `"duplicate_observations"`} {
		if !strings.Contains(text, expected) {
			t.Fatalf("missing %s in %s", expected, text)
		}
	}
}

func urlValues(parts ...string) url.Values {
	values := url.Values{}
	for i := 0; i < len(parts); i += 2 {
		values[parts[i]] = []string{parts[i+1]}
	}
	return values
}
