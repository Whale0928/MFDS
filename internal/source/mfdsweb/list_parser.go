package mfdsweb

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"
	"time"

	"golang.org/x/net/html"
)

var (
	expectedHeaders = []string{
		"구분",
		"수입업체",
		"제품명(한글)",
		"제품명(영문)",
		"품목(유형)",
		"제조/작업/수출업소",
		"처리일자",
		"소비기한",
		"제조국",
		"수출국",
	}
	rcnoPattern       = regexp.MustCompile(`^javascript:fnFoodDetail\(['"]([0-9]{12})['"]\);?$`)
	totalPagesPattern = regexp.MustCompile(`/\s*([0-9]+)`)
)

func ParseList(body []byte, expected ListRequest) (ListPage, error) {
	if err := validateListRequest(expected); err != nil {
		return ListPage{}, err
	}

	document, err := html.Parse(bytes.NewReader(body))
	if err != nil {
		return ListPage{}, parseError("HTML 문서 해석", err)
	}
	if isGenericErrorPage(document) {
		return ListPage{}, parseError("응답 shape 검증", fmt.Errorf("generic error page가 반환되었습니다"))
	}

	tables := findElements(document, func(node *html.Node) bool {
		return node.Data == "table" && hasClass(node, "board_list") && hasClass(node, "auto_title")
	})
	if len(tables) != 1 {
		return ListPage{}, parseError("목록 table 검증", fmt.Errorf("목록 table 수가 %d개입니다", len(tables)))
	}
	table := tables[0]
	if err := validateHeaders(table); err != nil {
		return ListPage{}, err
	}

	form, err := uniqueElement(document, func(node *html.Node) bool {
		return node.Data == "form" && attribute(node, "id") == "frm"
	}, "paging form")
	if err != nil {
		return ListPage{}, err
	}
	if err := validateEchoedRequest(form, expected); err != nil {
		return ListPage{}, err
	}

	total, err := parseTotal(document, form)
	if err != nil {
		return ListPage{}, err
	}
	if expected.Page > 1 && total != *expected.TotalSnapshot {
		return ListPage{}, parseError(
			"total snapshot 검증",
			fmt.Errorf("응답 total %d, 요청 snapshot %d", total, *expected.TotalSnapshot),
		)
	}
	totalPages := 0
	if total > 0 {
		totalPages = (total + expected.Limit - 1) / expected.Limit
	}
	if err := validatePageNavigation(document, total, totalPages, expected.Page, expected.Limit); err != nil {
		return ListPage{}, err
	}

	tbodyNodes := directChildrenByTag(table, "tbody")
	if len(tbodyNodes) != 1 {
		return ListPage{}, parseError("목록 tbody 검증", fmt.Errorf("tbody 수가 %d개입니다", len(tbodyNodes)))
	}
	rowNodes := directChildrenByTag(tbodyNodes[0], "tr")
	rawRows, err := extractTargetTableRows(body)
	if err != nil {
		return ListPage{}, err
	}
	if len(rawRows) != len(rowNodes) {
		return ListPage{}, parseError(
			"원본 row 대응",
			fmt.Errorf("DOM row %d개, 원본 row %d개", len(rowNodes), len(rawRows)),
		)
	}

	page := ListPage{
		Total:      total,
		Page:       expected.Page,
		Limit:      expected.Limit,
		TotalPages: totalPages,
		Rows:       make([]ListRow, 0, len(rowNodes)),
	}
	if total == 0 {
		if len(rowNodes) != 1 || !isNoDataRow(rowNodes[0]) {
			return ListPage{}, parseError("no-data shape 검증", fmt.Errorf("total 0 응답의 no-data row가 유효하지 않습니다"))
		}
		return page, nil
	}

	for index, rowNode := range rowNodes {
		row, err := parseListRow(rowNode, rawRows[index], index+1, expected)
		if err != nil {
			return ListPage{}, err
		}
		page.Rows = append(page.Rows, row)
	}
	expectedRows := expected.Limit
	remaining := total - (expected.Page-1)*expected.Limit
	if remaining < expectedRows {
		expectedRows = remaining
	}
	if len(page.Rows) != expectedRows {
		page.Warnings = append(page.Warnings, fmt.Sprintf(
			"reconciliation: expected_rows=%d parsed_rows=%d",
			expectedRows,
			len(page.Rows),
		))
	}
	seen := make(map[string]struct{}, len(page.Rows))
	for _, row := range page.Rows {
		if _, exists := seen[row.RCNO]; exists {
			page.Warnings = append(page.Warnings, fmt.Sprintf("reconciliation: duplicate_rcno=%s", row.RCNO))
			continue
		}
		seen[row.RCNO] = struct{}{}
	}
	return page, nil
}

func validateHeaders(table *html.Node) error {
	theadNodes := directChildrenByTag(table, "thead")
	if len(theadNodes) != 1 {
		return parseError("목록 header 검증", fmt.Errorf("thead 수가 %d개입니다", len(theadNodes)))
	}
	headers := findElements(theadNodes[0], func(node *html.Node) bool {
		return node.Data == "th"
	})
	if len(headers) != len(expectedHeaders) {
		return parseError("목록 header 검증", fmt.Errorf("header 수가 %d개입니다", len(headers)))
	}
	for index, header := range headers {
		if actual := normalizedText(header); actual != expectedHeaders[index] {
			return parseError(
				"목록 header 검증",
				fmt.Errorf("header %d가 %q입니다, 예상 %q", index+1, actual, expectedHeaders[index]),
			)
		}
	}
	return nil
}

func validateEchoedRequest(form *html.Node, expected ListRequest) error {
	checks := []struct {
		label string
		name  string
		id    string
		value string
	}{
		{label: "page", id: "paramPage", value: strconv.Itoa(expected.Page)},
		{label: "limit", id: "paramLimit", value: strconv.Itoa(expected.Limit)},
		{label: "품목명", id: "paramRpsntItmNm", value: expected.Item.Name},
		{label: "품목 코드", name: "rpsntItmCd", value: expected.Item.Code},
		{label: "시작 처리일", id: "paramSrchStrtDt", value: expected.ProcessDate.Format(time.DateOnly)},
		{label: "종료 처리일", id: "paramSrchEndDt", value: expected.ProcessDate.Format(time.DateOnly)},
	}
	for _, check := range checks {
		inputs := findElements(form, func(node *html.Node) bool {
			if node.Data != "input" {
				return false
			}
			if check.id != "" {
				return attribute(node, "id") == check.id
			}
			return attribute(node, "name") == check.name
		})
		if len(inputs) != 1 {
			return parseError("echoed request 검증", fmt.Errorf("%s input 수가 %d개입니다", check.label, len(inputs)))
		}
		if actual := attribute(inputs[0], "value"); actual != check.value {
			return parseError(
				"echoed request 검증",
				fmt.Errorf("%s 값이 %q입니다, 예상 %q", check.label, actual, check.value),
			)
		}
	}
	return nil
}

func parseTotal(document, form *html.Node) (int, error) {
	countContainers := findElements(document, func(node *html.Node) bool {
		return node.Data == "div" && hasClass(node, "board_count")
	})
	if len(countContainers) != 1 {
		return 0, parseError("total 검증", fmt.Errorf("board_count 수가 %d개입니다", len(countContainers)))
	}
	strongNodes := findElements(countContainers[0], func(node *html.Node) bool {
		return node.Data == "strong"
	})
	if len(strongNodes) != 1 {
		return 0, parseError("total 검증", fmt.Errorf("total strong 수가 %d개입니다", len(strongNodes)))
	}
	visibleTotal, err := parseNonNegativeInt(normalizedText(strongNodes[0]), "화면 total")
	if err != nil {
		return 0, err
	}
	hiddenNodes := findElements(form, func(node *html.Node) bool {
		return node.Data == "input" && attribute(node, "id") == "paramTotalCnt"
	})
	if len(hiddenNodes) != 1 {
		return 0, parseError("total 검증", fmt.Errorf("paramTotalCnt 수가 %d개입니다", len(hiddenNodes)))
	}
	hiddenTotal, err := parseNonNegativeInt(attribute(hiddenNodes[0], "value"), "hidden total")
	if err != nil {
		return 0, err
	}
	if visibleTotal != hiddenTotal {
		return 0, parseError(
			"total 검증",
			fmt.Errorf("화면 total %d, hidden total %d", visibleTotal, hiddenTotal),
		)
	}
	return visibleTotal, nil
}

func validatePageNavigation(document *html.Node, total, totalPages, page, limit int) error {
	navigation := findElements(document, func(node *html.Node) bool {
		return node.Data == "p" && hasClass(node, "page_navi")
	})
	if total == 0 {
		if len(navigation) != 0 {
			return parseError("page navigation 검증", fmt.Errorf("total 0 응답에 navigation이 있습니다"))
		}
		return nil
	}
	if len(navigation) == 0 {
		if total > limit {
			return parseError("page navigation 검증", fmt.Errorf("복수 page 응답에 navigation이 없습니다"))
		}
		return nil
	}
	if len(navigation) != 1 {
		return parseError("page navigation 검증", fmt.Errorf("navigation 수가 %d개입니다", len(navigation)))
	}
	currentPageInputs := findElements(navigation[0], func(node *html.Node) bool {
		return node.Data == "input" && hasClass(node, "number")
	})
	if len(currentPageInputs) != 1 {
		return parseError(
			"page navigation 검증",
			fmt.Errorf("현재 page input 수가 %d개입니다", len(currentPageInputs)),
		)
	}
	if actual := attribute(currentPageInputs[0], "value"); actual != strconv.Itoa(page) {
		return parseError(
			"page navigation 검증",
			fmt.Errorf("navigation 현재 page %q, 요청 page %d", actual, page),
		)
	}
	matches := totalPagesPattern.FindStringSubmatch(normalizedText(navigation[0]))
	if len(matches) != 2 {
		return parseError("page navigation 검증", fmt.Errorf("전체 page 수를 찾을 수 없습니다"))
	}
	navigationPages, err := strconv.Atoi(matches[1])
	if err != nil || navigationPages != totalPages {
		return parseError(
			"page navigation 검증",
			fmt.Errorf("navigation pages %q, 계산 pages %d", matches[1], totalPages),
		)
	}
	return nil
}

func parseListRow(node *html.Node, raw []byte, rowNumber int, expected ListRequest) (ListRow, error) {
	cells := directChildrenByTag(node, "td")
	if len(cells) != len(expectedHeaders) {
		return ListRow{}, parseError("목록 row 검증", fmt.Errorf("row %d의 cell 수가 %d개입니다", rowNumber, len(cells)))
	}
	values := make([]string, len(cells))
	var rcno string
	var detailHref string
	for index, cell := range cells {
		values[index] = normalizedText(cell)
		anchors := findElements(cell, func(candidate *html.Node) bool {
			return candidate.Data == "a"
		})
		if len(anchors) != 1 {
			return ListRow{}, parseError(
				"rcno 검증",
				fmt.Errorf("row %d cell %d의 anchor 수가 %d개입니다", rowNumber, index+1, len(anchors)),
			)
		}
		href := attribute(anchors[0], "href")
		matches := rcnoPattern.FindStringSubmatch(href)
		if len(matches) != 2 {
			return ListRow{}, parseError(
				"rcno 검증",
				fmt.Errorf("row %d cell %d의 detail href가 유효하지 않습니다", rowNumber, index+1),
			)
		}
		if rcno == "" {
			rcno = matches[1]
			detailHref = href
		} else if rcno != matches[1] {
			return ListRow{}, parseError(
				"rcno 검증",
				fmt.Errorf("row %d의 rcno가 cell 사이에서 일치하지 않습니다", rowNumber),
			)
		}
	}

	processedDate, err := time.ParseInLocation(time.DateOnly, values[6], expected.ProcessDate.Location())
	if err != nil {
		return ListRow{}, parseError("처리일 검증", fmt.Errorf("row %d: %w", rowNumber, err))
	}
	if processedDate.Format(time.DateOnly) != expected.ProcessDate.Format(time.DateOnly) {
		return ListRow{}, parseError(
			"처리일 검증",
			fmt.Errorf("row %d 처리일 %s, 요청일 %s", rowNumber, values[6], expected.ProcessDate.Format(time.DateOnly)),
		)
	}
	if values[4] != expected.Item.Name {
		return ListRow{}, parseError(
			"품목명 검증",
			fmt.Errorf("row %d 품목명 %q, 요청 품목명 %q", rowNumber, values[4], expected.Item.Name),
		)
	}

	canonicalValues, err := json.Marshal(values)
	if err != nil {
		return ListRow{}, parseError("canonical values 생성", err)
	}
	semanticValues := make([]string, 0, len(values)+2)
	semanticValues = append(semanticValues, ParserVersion, rcno)
	semanticValues = append(semanticValues, values...)
	semanticMaterial, err := json.Marshal(semanticValues)
	if err != nil {
		return ListRow{}, parseError("semantic material 생성", err)
	}

	return ListRow{
		RowNo:                     rowNumber,
		RCNO:                      rcno,
		ProductDivisionName:       values[0],
		ImporterName:              values[1],
		ProductNameKO:             values[2],
		ProductNameEN:             values[3],
		ItemName:                  values[4],
		OverseasEstablishmentName: values[5],
		ProcessedDateRaw:          values[6],
		ProcessedDate:             processedDate,
		ExpiryText:                values[7],
		ManufactureCountryName:    values[8],
		ExportCountryName:         values[9],
		DetailHref:                detailHref,
		CanonicalValuesJSON:       canonicalValues,
		RawRowHTML:                append([]byte(nil), raw...),
		RawRowSHA256:              sha256.Sum256(raw),
		SemanticSHA256:            sha256.Sum256(semanticMaterial),
	}, nil
}

func extractTargetTableRows(body []byte) ([][]byte, error) {
	tokenizer := html.NewTokenizer(bytes.NewReader(body))
	offset := 0
	inTargetTable := false
	tableDepth := 0
	tbodyDepth := 0
	rowDepth := 0
	rowStart := 0
	var rows [][]byte

	for {
		tokenType := tokenizer.Next()
		raw := tokenizer.Raw()
		start := offset
		offset += len(raw)

		switch tokenType {
		case html.ErrorToken:
			if tokenizer.Err() != nil && !errors.Is(tokenizer.Err(), io.EOF) {
				return nil, parseError("원본 row 탐색", tokenizer.Err())
			}
			if rowDepth != 0 || (inTargetTable && tableDepth != 0) {
				return nil, parseError("원본 row 탐색", fmt.Errorf("HTML이 row 또는 table 중간에서 종료되었습니다"))
			}
			return rows, nil
		case html.StartTagToken, html.SelfClosingTagToken:
			token := tokenizer.Token()
			switch token.Data {
			case "table":
				if inTargetTable {
					tableDepth++
				} else if tokenHasClass(token, "board_list") && tokenHasClass(token, "auto_title") {
					inTargetTable = true
					tableDepth = 1
				}
			case "tbody":
				if inTargetTable {
					tbodyDepth++
				}
			case "tr":
				if inTargetTable && tbodyDepth == 1 {
					if rowDepth == 0 {
						rowStart = start
					}
					rowDepth++
				}
			}
		case html.EndTagToken:
			token := tokenizer.Token()
			switch token.Data {
			case "tr":
				if inTargetTable && tbodyDepth == 1 && rowDepth > 0 {
					rowDepth--
					if rowDepth == 0 {
						rows = append(rows, append([]byte(nil), body[rowStart:offset]...))
					}
				}
			case "tbody":
				if inTargetTable && tbodyDepth > 0 {
					tbodyDepth--
				}
			case "table":
				if inTargetTable {
					tableDepth--
					if tableDepth == 0 {
						inTargetTable = false
					}
				}
			}
		}
	}
}

func isGenericErrorPage(document *html.Node) bool {
	titles := findElements(document, func(node *html.Node) bool {
		return node.Data == "title"
	})
	for _, title := range titles {
		if strings.EqualFold(normalizedText(title), "Error") {
			return true
		}
	}
	return strings.Contains(normalizedText(document), "에러가 발생하였습니다")
}

func isNoDataRow(row *html.Node) bool {
	cells := directChildrenByTag(row, "td")
	if len(cells) != 1 || attribute(cells[0], "colspan") != "10" {
		return false
	}
	markers := findElements(cells[0], func(node *html.Node) bool {
		return node.Data == "span" && hasClass(node, "nodata")
	})
	return len(markers) == 1 && normalizedText(markers[0]) == "조회 결과가 없습니다."
}

func parseNonNegativeInt(value, label string) (int, error) {
	if value == "" {
		return 0, parseError("total 검증", fmt.Errorf("%s이 비어 있습니다", label))
	}
	number, err := strconv.Atoi(value)
	if err != nil || number < 0 {
		return 0, parseError("total 검증", fmt.Errorf("%s %q이 유효하지 않습니다", label, value))
	}
	return number, nil
}

func uniqueElement(root *html.Node, predicate func(*html.Node) bool, label string) (*html.Node, error) {
	elements := findElements(root, predicate)
	if len(elements) != 1 {
		return nil, parseError(label+" 검증", fmt.Errorf("%s 수가 %d개입니다", label, len(elements)))
	}
	return elements[0], nil
}

func findElements(root *html.Node, predicate func(*html.Node) bool) []*html.Node {
	var elements []*html.Node
	var visit func(*html.Node)
	visit = func(node *html.Node) {
		if node.Type == html.ElementNode && predicate(node) {
			elements = append(elements, node)
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			visit(child)
		}
	}
	visit(root)
	return elements
}

func directChildrenByTag(node *html.Node, tag string) []*html.Node {
	var children []*html.Node
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		if child.Type == html.ElementNode && child.Data == tag {
			children = append(children, child)
		}
	}
	return children
}

func normalizedText(node *html.Node) string {
	var builder strings.Builder
	var visit func(*html.Node)
	visit = func(current *html.Node) {
		if current.Type == html.TextNode {
			builder.WriteString(current.Data)
			builder.WriteByte(' ')
		}
		for child := current.FirstChild; child != nil; child = child.NextSibling {
			visit(child)
		}
	}
	visit(node)
	return strings.Join(strings.Fields(builder.String()), " ")
}

func hasClass(node *html.Node, expected string) bool {
	for _, className := range strings.Fields(attribute(node, "class")) {
		if className == expected {
			return true
		}
	}
	return false
}

func tokenHasClass(token html.Token, expected string) bool {
	for _, attribute := range token.Attr {
		if attribute.Key != "class" {
			continue
		}
		for _, className := range strings.Fields(attribute.Val) {
			if className == expected {
				return true
			}
		}
	}
	return false
}

func attribute(node *html.Node, name string) string {
	for _, attribute := range node.Attr {
		if attribute.Key == name {
			return attribute.Val
		}
	}
	return ""
}

func parseError(op string, err error) error {
	return &AdapterError{Kind: ErrorKindParse, Op: op, Err: err}
}
