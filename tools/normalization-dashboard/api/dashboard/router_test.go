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
			return &fakeRows{values: [][]any{{"202600000001", "원본명", "정제명", "700 · 40%", "12년", "", "NORMALIZED", "2026-08-01", "위스키", "수입사", "영국", true, false, true, `["AMBIGUOUS"]`}}}, nil
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
	if !strings.Contains(response.Body.String(), `"alcohol_matched":true`) || !strings.Contains(response.Body.String(), `"distillery_matched":false`) || !strings.Contains(response.Body.String(), `"region_matched":true`) {
		t.Fatalf("matching flags are missing: %s", response.Body.String())
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

func TestDeclarations_매칭여부필터와전체목록정렬을지원한다(t *testing.T) {
	filter, _, _, err := parseListRequest(urlValues(
		"alcohol_match", "matched",
		"distillery_match", "unmatched",
		"region_match", "matched",
		"sort", "distillery",
		"order", "asc",
	))
	if err != nil {
		t.Fatal(err)
	}
	where, args := filter.where()
	for _, clause := range []string{
		"declaration_v3.alcohol_candidate_1_id IS NOT NULL",
		"declaration_v3.distillery_candidate_1_id IS NULL",
		"declaration_v3.region_candidate_1_id IS NOT NULL",
	} {
		if !strings.Contains(where, clause) {
			t.Fatalf("where clause does not contain %q: %s", clause, where)
		}
	}
	if len(args) != 0 {
		t.Fatalf("matching filter args = %#v", args)
	}
	wantOrder := "ORDER BY (declaration_v3.distillery_candidate_1_id IS NOT NULL) ASC, source_processed_date DESC, source_item_id DESC"
	if !strings.Contains(filter.orderBy(), wantOrder) {
		t.Fatalf("order by = %q", filter.orderBy())
	}
}

func TestDeclarations_허용하지않은매칭필터와정렬값을거부한다(t *testing.T) {
	for name, values := range map[string]url.Values{
		"match": urlValues("alcohol_match", "maybe"),
		"sort":  urlValues("sort", "source_product_name"),
		"order": urlValues("order", "random"),
	} {
		t.Run(name, func(t *testing.T) {
			if _, _, _, err := parseListRequest(values); err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}

func TestDeclarationDetail_evidenceIsPublicObjectContract(t *testing.T) {
	queryer := &fakeQueryer{respond: func(query string, _ []any) (RowIterator, error) {
		if !strings.Contains(query, "FROM declaration_details") || !strings.Contains(query, "WHERE rcno") {
			return nil, fmt.Errorf("unexpected query")
		}
		if strings.Contains(query, "alcohol_category_ko") {
			row := make([]any, len(detailColumns))
			for index := range row {
				row[index] = "값"
			}
			return &fakeRows{values: [][]any{row}}, nil
		}
		return &fakeRows{values: [][]any{{"202600000001", "원본", "source", "정제", "normalized", "REVIEW_REQUIRED", `["AMBIGUOUS"]`, `["700ml? "]`, "2026-08-01", "위스키", "수입사", "영국", "700ml", "700 mL", "40", "40%", "", "", "12", "12 years", "", "", "한정", "#7", "UNKNOWN", "7", "LOT-1", "B-2", "수입사", "증류소"}}}, nil
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
	groups, ok := body["groups"].([]any)
	if !ok || len(groups) == 0 {
		t.Fatalf("groups = %#v", body["groups"])
	}
}

// 상세 화면은 원장과 정제 값을 모두 노출하되 원본 HTML, 해시, 수집 metadata는 노출하지 않는다.
func TestDetailColumns_원장과정제를모두노출하고수집metadata는제외한다(t *testing.T) {
	titles := map[string]bool{}
	for _, column := range detailColumns {
		titles[column.Group] = true
	}
	for _, expected := range []string{"원장 - 수집한 그대로", "제품명 정제 결과", "용량과 도수", "관리 번호", "증류소 매칭", "리전 매칭", "매칭 이력", "정제 이력"} {
		if !titles[expected] {
			t.Fatalf("그룹 %q 가 없다", expected)
		}
	}
	joined := strings.Join(func() []string {
		expressions := make([]string, len(detailColumns))
		for index, column := range detailColumns {
			expressions[index] = column.Expr
		}
		return expressions
	}(), " ")
	for _, forbidden := range []string{"raw_row_html", "raw_row_sha256", "semantic_sha256", "source_job_id", "source_task_id", "source_fetch_id", "source_detail_href", "claim_"} {
		if strings.Contains(joined, forbidden) {
			t.Fatalf("비공개 컬럼 %q 를 노출한다", forbidden)
		}
	}
}

// 원장과 정제를 좌우로 비교하려면 모든 그룹이 한쪽으로 확실히 갈려야 한다.
func TestGroupSide_원장과정제를좌우로가른다(t *testing.T) {
	sides := map[string][]string{}
	for _, column := range detailColumns {
		side := groupSide(column.Group)
		if side != "ledger" && side != "normalized" {
			t.Fatalf("그룹 %q 의 side 가 %q 다", column.Group, side)
		}
		if len(sides[side]) == 0 || sides[side][len(sides[side])-1] != column.Group {
			sides[side] = append(sides[side], column.Group)
		}
	}
	if len(sides["ledger"]) == 0 || len(sides["normalized"]) == 0 {
		t.Fatalf("한쪽이 비었다: %#v", sides)
	}
	if groupSide("원장 - 수집한 그대로") != "ledger" || groupSide("제품명 정제 결과") != "normalized" {
		t.Fatal("대표 그룹이 반대쪽에 있다")
	}
}

// 모든 필드에 각주가 붙어 있어야 화면에서 용어를 설명할 수 있다.
func TestDetailColumns_모든필드가라벨과각주를가진다(t *testing.T) {
	for _, column := range detailColumns {
		if column.Label == "" || column.Hint == "" || column.Group == "" {
			t.Fatalf("라벨 또는 각주가 비었다: %#v", column)
		}
		if strings.Contains(column.Label, "SKU") || strings.Contains(column.Label, "_") {
			t.Fatalf("라벨에 내부 용어가 남았다: %q", column.Label)
		}
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

func TestDashboardV3Queries_기존View를교체하지않고BaseTable컬럼을조회한다(t *testing.T) {
	for name, query := range map[string]string{
		"list":     declarationListSQL,
		"detail":   declarationDetailSQL,
		"coverage": coverageSQL,
	} {
		if !strings.Contains(query, declarationV3SourceSQL) {
			t.Fatalf("%s query does not join declarations v3 columns: %s", name, query)
		}
	}
	for _, column := range []string{"ingredient_percent_raw", "ingredient_percent", "variant_marker_raw", "variant_marker_type", "variant_marker_value", "alcohol_candidate_1_id", "alcohol_candidate_3_score", "selected_alcohol_id", "distillery_candidate_1_id", "distillery_candidate_3_score", "selected_distillery_id", "region_candidate_1_id", "region_candidate_3_score", "selected_region_id", "matching_version", "matching_run_id", "alcohol_match_decision", "distillery_match_source", "region_match_source", "matched_at"} {
		if !strings.Contains(declarationV3SourceSQL, column) {
			t.Fatalf("v3 source does not select %s: %s", column, declarationV3SourceSQL)
		}
	}
}

func TestMatchingDetail_후보이름과점수를기준테이블에서조회한다(t *testing.T) {
	for _, join := range []string{
		"LEFT JOIN alcohols AS alcohol_candidate_1",
		"LEFT JOIN alcohols AS alcohol_candidate_2",
		"LEFT JOIN alcohols AS alcohol_candidate_3",
		"LEFT JOIN distilleries AS distillery_candidate_1",
		"LEFT JOIN distilleries AS distillery_candidate_2",
		"LEFT JOIN distilleries AS distillery_candidate_3",
		"LEFT JOIN regions AS region_candidate_1",
		"LEFT JOIN regions AS region_candidate_2",
		"LEFT JOIN regions AS region_candidate_3",
	} {
		if !strings.Contains(declarationMatchingSourceSQL, join) {
			t.Fatalf("매칭 상세 조회에 %q 조인이 없다", join)
		}
	}

	labels := map[string]map[string]bool{}
	for _, column := range detailColumns {
		if labels[column.Group] == nil {
			labels[column.Group] = map[string]bool{}
		}
		labels[column.Group][column.Label] = true
	}
	for _, group := range []string{"알코올 매칭", "증류소 매칭", "리전 매칭"} {
		for _, label := range []string{"1순위 후보", "2순위 후보", "3순위 후보"} {
			if !labels[group][label] {
				t.Fatalf("%s 그룹에 %s가 없다", group, label)
			}
		}
	}
	if !labels["매칭 이력"]["매칭 규칙 버전"] || !labels["매칭 이력"]["마지막 매칭 시각"] {
		t.Fatal("매칭 버전 또는 실행 시각이 없다")
	}
	if !labels["매칭 이력"]["매칭 실행 ID"] || !strings.Contains(strings.Join(detailExpressions(), " "), "FORMAT(") {
		t.Fatal("매칭 실행 ID 또는 소수점 두 자리 점수 포맷이 없다")
	}
}

func detailExpressions() []string {
	result := make([]string, len(detailColumns))
	for index, column := range detailColumns {
		result[index] = column.Expr
	}
	return result
}

func urlValues(parts ...string) url.Values {
	values := url.Values{}
	for i := 0; i < len(parts); i += 2 {
		values[parts[i]] = []string{parts[i+1]}
	}
	return values
}
