package mfdsweb

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestParseList_Page1_10개필드와Hash를반환한다(t *testing.T) {
	body := readFixture(t, "list_page1.html")

	page, err := ParseList(body, whiskyRequest(1, 2, nil))
	if err != nil {
		t.Fatalf("ParseList() error = %v", err)
	}
	if page.Total != 4 || page.Page != 1 || page.Limit != 2 || page.TotalPages != 2 || len(page.Rows) != 2 {
		t.Fatalf("page = %+v", page)
	}
	if len(page.Warnings) != 0 {
		t.Fatalf("warnings = %v", page.Warnings)
	}

	row := page.Rows[0]
	if row.RCNO != "202600519647" ||
		row.ProductNameKO != "윌슨앤모건 클라이넬리쉬 20년 2005" ||
		row.ProductNameEN != "W&M609 - W&M CLYNELISH 20 YO" ||
		row.ExpiryText != "2025-12-11 ~" ||
		row.ManufactureCountryName != "영국" ||
		row.ExportCountryName != "이탈리아" {
		t.Fatalf("row = %+v", row)
	}
	if !bytes.Contains(row.RawRowHTML, []byte(`data-source="fixture"`)) ||
		!bytes.Contains(row.RawRowHTML, []byte(`W&amp;M609`)) {
		t.Fatalf("raw row was not preserved: %s", row.RawRowHTML)
	}
	if row.RawRowSHA256 != sha256.Sum256(row.RawRowHTML) {
		t.Fatalf("raw row hash = %x", row.RawRowSHA256)
	}

	var canonical []string
	if err := json.Unmarshal(row.CanonicalValuesJSON, &canonical); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if len(canonical) != 10 || canonical[4] != "위스키" || canonical[6] != "2026-07-27" {
		t.Fatalf("canonical = %#v", canonical)
	}
	semantic := append([]string{ParserVersion, row.RCNO}, canonical...)
	material, err := json.Marshal(semantic)
	if err != nil {
		t.Fatal(err)
	}
	if row.SemanticSHA256 != sha256.Sum256(material) {
		t.Fatalf("semantic hash = %x", row.SemanticSHA256)
	}
}

func TestParseList_Page2_TotalSnapshot과행을검증한다(t *testing.T) {
	total := 4
	page, err := ParseList(readFixture(t, "list_page2.html"), whiskyRequest(2, 2, &total))
	if err != nil {
		t.Fatalf("ParseList() error = %v", err)
	}
	if page.Total != 4 || page.TotalPages != 2 || len(page.Rows) != 2 ||
		page.Rows[0].RCNO != "202600522066" {
		t.Fatalf("page = %+v", page)
	}
}

func TestParseList_조회결과없음_빈Page를반환한다(t *testing.T) {
	location := time.FixedZone("Asia/Seoul", 9*60*60)
	request := ListRequest{
		Item:        Item{Name: "브랜디", Code: "C0314220000000000000"},
		ProcessDate: time.Date(2025, time.July, 28, 0, 0, 0, 0, location),
		Page:        1,
		Limit:       10,
	}

	page, err := ParseList(readFixture(t, "list_empty.html"), request)
	if err != nil {
		t.Fatalf("ParseList() error = %v", err)
	}
	if page.Total != 0 || page.TotalPages != 0 || len(page.Rows) != 0 {
		t.Fatalf("page = %+v", page)
	}
}

func TestParseList_GenericError_오류를반환한다(t *testing.T) {
	_, err := ParseList(readFixture(t, "error.html"), whiskyRequest(1, 2, nil))
	if errorKind(err) != ErrorKindParse {
		t.Fatalf("ParseList() error = %v", err)
	}
}

func TestParseList_응답계약불일치_오류를반환한다(t *testing.T) {
	original := string(readFixture(t, "list_page1.html"))
	total := 5
	firstImporterCell := `<td><a href="javascript:fnFoodDetail(&#39;202600519647&#39;);">앤트인터내셔널</a></td>`
	firstDivisionAnchor := `<a href="javascript:fnFoodDetail(&#39;202600519647&#39;);">가공식품</a>`
	firstRowStart := strings.Index(original, `<tr data-source="fixture">`)
	firstRowEnd := strings.Index(original[firstRowStart:], "</tr>") + firstRowStart
	extraCell := `<td><a href="javascript:fnFoodDetail(&#39;202600519647&#39;);">추가</a></td>`
	withElevenCells := original[:firstRowEnd] + extraCell + original[firstRowEnd:]
	navigationStart := strings.Index(original, `<p class="page_navi">`)
	navigationEnd := strings.Index(original[navigationStart:], "</p>") + navigationStart + len("</p>")
	tests := []struct {
		name    string
		body    string
		request ListRequest
	}{
		{
			name:    "header",
			body:    strings.Replace(original, "<th>구분</th>", "<th>분류</th>", 1),
			request: whiskyRequest(1, 2, nil),
		},
		{
			name:    "processed date",
			body:    strings.Replace(original, ">2026-07-27</a>", ">2026-07-26</a>", 1),
			request: whiskyRequest(1, 2, nil),
		},
		{
			name:    "item",
			body:    strings.Replace(original, ">위스키</a>", ">브랜디</a>", 1),
			request: whiskyRequest(1, 2, nil),
		},
		{
			name:    "rcno disagreement",
			body:    strings.Replace(original, "202600519647", "202600519648", 1),
			request: whiskyRequest(1, 2, nil),
		},
		{
			name:    "invalid rcno length",
			body:    strings.ReplaceAll(original, "202600519647", "20260051964"),
			request: whiskyRequest(1, 2, nil),
		},
		{
			name:    "non-numeric rcno",
			body:    strings.ReplaceAll(original, "202600519647", "ABCDEFGHIJKL"),
			request: whiskyRequest(1, 2, nil),
		},
		{
			name:    "13-digit rcno",
			body:    strings.ReplaceAll(original, "202600519647", "2026005196470"),
			request: whiskyRequest(1, 2, nil),
		},
		{
			name:    "9 cells",
			body:    strings.Replace(original, firstImporterCell, "", 1),
			request: whiskyRequest(1, 2, nil),
		},
		{
			name:    "11 cells",
			body:    withElevenCells,
			request: whiskyRequest(1, 2, nil),
		},
		{
			name:    "missing anchor",
			body:    strings.Replace(original, firstDivisionAnchor, "가공식품", 1),
			request: whiskyRequest(1, 2, nil),
		},
		{
			name:    "hidden total",
			body:    strings.Replace(original, `id="paramTotalCnt" name="totalCnt" value="4"`, `id="paramTotalCnt" name="totalCnt" value="5"`, 1),
			request: whiskyRequest(1, 2, nil),
		},
		{
			name:    "echoed page",
			body:    strings.Replace(original, `id="paramPage" name="page" value="1"`, `id="paramPage" name="page" value="2"`, 1),
			request: whiskyRequest(1, 2, nil),
		},
		{
			name:    "echoed limit",
			body:    strings.Replace(original, `id="paramLimit" name="limit" value="2"`, `id="paramLimit" name="limit" value="3"`, 1),
			request: whiskyRequest(1, 2, nil),
		},
		{
			name:    "echoed item name",
			body:    strings.Replace(original, `id="paramRpsntItmNm" name="rpsntItmNm" value="위스키"`, `id="paramRpsntItmNm" name="rpsntItmNm" value="브랜디"`, 1),
			request: whiskyRequest(1, 2, nil),
		},
		{
			name:    "echoed item code",
			body:    strings.Replace(original, `name="rpsntItmCd" value="C0314210000000000000"`, `name="rpsntItmCd" value="C0314220000000000000"`, 1),
			request: whiskyRequest(1, 2, nil),
		},
		{
			name:    "echoed start date",
			body:    strings.Replace(original, `id="paramSrchStrtDt" name="srchStrtDt" value="2026-07-27"`, `id="paramSrchStrtDt" name="srchStrtDt" value="2026-07-26"`, 1),
			request: whiskyRequest(1, 2, nil),
		},
		{
			name:    "echoed end date",
			body:    strings.Replace(original, `id="paramSrchEndDt" name="srchEndDt" value="2026-07-27"`, `id="paramSrchEndDt" name="srchEndDt" value="2026-07-28"`, 1),
			request: whiskyRequest(1, 2, nil),
		},
		{
			name:    "missing positive navigation",
			body:    original[:navigationStart] + original[navigationEnd:],
			request: whiskyRequest(1, 2, nil),
		},
		{
			name:    "page2 snapshot",
			body:    string(readFixture(t, "list_page2.html")),
			request: whiskyRequest(2, 2, &total),
		},
		{
			name:    "truncated",
			body:    original[:strings.Index(original, "</tbody>")],
			request: whiskyRequest(1, 2, nil),
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := ParseList([]byte(test.body), test.request)
			if errorKind(err) != ErrorKindParse {
				t.Fatalf("ParseList() error = %v", err)
			}
		})
	}
}

func TestParseList_RowCount불일치_행을보존하고Warning을반환한다(t *testing.T) {
	original := string(readFixture(t, "list_page1.html"))
	secondRowStart := strings.Index(original, `<tr><td><a href="javascript:fnFoodDetail(&#39;202600524242&#39;);">`)
	secondRowEnd := strings.Index(original[secondRowStart:], "</tr>") + secondRowStart + len("</tr>")
	body := original[:secondRowStart] + original[secondRowEnd:]

	page, err := ParseList([]byte(body), whiskyRequest(1, 2, nil))
	if err != nil {
		t.Fatalf("ParseList() error = %v", err)
	}
	if len(page.Rows) != 1 || len(page.Warnings) != 1 ||
		!strings.Contains(page.Warnings[0], "expected_rows=2 parsed_rows=1") {
		t.Fatalf("rows=%d warnings=%v", len(page.Rows), page.Warnings)
	}
}

func TestParseList_DuplicateRCNO_행을보존하고Warning을반환한다(t *testing.T) {
	body := bytes.ReplaceAll(
		readFixture(t, "list_page1.html"),
		[]byte("202600524242"),
		[]byte("202600519647"),
	)

	page, err := ParseList(body, whiskyRequest(1, 2, nil))
	if err != nil {
		t.Fatalf("ParseList() error = %v", err)
	}
	if len(page.Rows) != 2 || page.Rows[0].RCNO != page.Rows[1].RCNO {
		t.Fatalf("rows = %+v", page.Rows)
	}
	if len(page.Warnings) != 1 || !strings.Contains(page.Warnings[0], "duplicate_rcno") {
		t.Fatalf("warnings = %v", page.Warnings)
	}
}

func TestParseList_표현차이_SemanticHash는같고RawHash는달라진다(t *testing.T) {
	original := readFixture(t, "list_page1.html")
	modified := bytes.Replace(
		original,
		[]byte(`<tr data-source="fixture">`),
		[]byte(`<tr class="visual-only" data-source="fixture">`),
		1,
	)

	first, err := ParseList(original, whiskyRequest(1, 2, nil))
	if err != nil {
		t.Fatal(err)
	}
	second, err := ParseList(modified, whiskyRequest(1, 2, nil))
	if err != nil {
		t.Fatal(err)
	}
	if first.Rows[0].SemanticSHA256 != second.Rows[0].SemanticSHA256 {
		t.Fatal("semantic hash changed for presentation-only change")
	}
	if first.Rows[0].RawRowSHA256 == second.Rows[0].RawRowSHA256 {
		t.Fatal("raw row hash did not change")
	}
}

func TestParseList_의미값변경_SemanticHash가달라진다(t *testing.T) {
	original := readFixture(t, "list_page1.html")
	modified := bytes.Replace(
		original,
		[]byte("윌슨앤모건 클라이넬리쉬 20년 2005"),
		[]byte("윌슨앤모건 클라이넬리쉬 21년 2005"),
		1,
	)

	first, err := ParseList(original, whiskyRequest(1, 2, nil))
	if err != nil {
		t.Fatal(err)
	}
	second, err := ParseList(modified, whiskyRequest(1, 2, nil))
	if err != nil {
		t.Fatal(err)
	}
	if first.Rows[0].SemanticSHA256 == second.Rows[0].SemanticSHA256 {
		t.Fatal("semantic hash did not change for semantic value change")
	}
}
