package mfdscompany

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"unicode"

	"golang.org/x/net/html"
)

var detailCodePattern = regexp.MustCompile(`fnDetailPopup1\(\s*['"]([^'"]+)['"]\s*\)`)
var totalPattern = regexp.MustCompile(`총\s*([0-9][0-9,]*)\s*건`)
var galleryProductPattern = regexp.MustCompile(`fnGalleryMobileDetail\(\s*['"]?([0-9]+)['"]?\s*\)`)
var galleryParamPattern = regexp.MustCompile(`(?s)var\s+param\s*=\s*(\{.*?\})\s*;`)

func parseSearchPage(body []byte, request SearchRequest) (SearchPage, error) {
	document, err := html.Parse(bytes.NewReader(body))
	if err != nil {
		return SearchPage{}, fmt.Errorf("목록 HTML 해석: %w", err)
	}
	tables := findElements(document, func(node *html.Node) bool {
		return node.Data == "table" && captionContains(node, "국내영업자 검색 조회 테이블")
	})
	if len(tables) != 1 {
		return SearchPage{}, fmt.Errorf("국내영업자 목록 table 수가 %d개입니다", len(tables))
	}
	table := tables[0]
	wantHeaders := []string{"번호", "인허가번호", "업소명", "업종", "대표자", "소재지", "인허가기관"}
	if err := validateTableHeaders(table, wantHeaders); err != nil {
		return SearchPage{}, err
	}
	section := nearestAncestor(table, "li")
	if section == nil {
		section = document
	}
	total, err := parseTotal(section)
	if err != nil {
		return SearchPage{}, err
	}
	if request.Page > 1 && total != *request.TotalSnapshot {
		return SearchPage{}, fmt.Errorf(
			"국내영업자 응답 total이 snapshot과 다릅니다: expected=%d actual=%d",
			*request.TotalSnapshot, total,
		)
	}
	bodyNodes := directChildrenByTag(table, "tbody")
	if len(bodyNodes) != 1 {
		return SearchPage{}, fmt.Errorf("국내영업자 목록 tbody 수가 %d개입니다", len(bodyNodes))
	}
	rowNodes := directChildrenByTag(bodyNodes[0], "tr")
	if total == 0 {
		if len(rowNodes) != 1 || !strings.Contains(normalizedText(rowNodes[0]), "조회 결과가 없습니다") {
			return SearchPage{}, fmt.Errorf("조회 결과 없음 응답의 row shape가 올바르지 않습니다")
		}
		return SearchPage{Total: 0, Page: request.Page, Limit: request.Limit}, nil
	}
	expectedRows := total - (request.Page-1)*request.Limit
	if expectedRows < 0 {
		expectedRows = 0
	}
	if expectedRows > request.Limit {
		expectedRows = request.Limit
	}
	if len(rowNodes) != expectedRows {
		return SearchPage{}, fmt.Errorf("목록 결과 수가 정확하지 않습니다: expected=%d actual=%d", expectedRows, len(rowNodes))
	}
	rows := make([]SearchResult, 0, len(rowNodes))
	for index, rowNode := range rowNodes {
		row, err := parseSearchRow(rowNode)
		if err != nil {
			return SearchPage{}, fmt.Errorf("목록 row %d: %w", index+1, err)
		}
		rows = append(rows, row)
	}
	totalPages := (total + request.Limit - 1) / request.Limit
	return SearchPage{Total: total, Page: request.Page, Limit: request.Limit, TotalPages: totalPages, Rows: rows}, nil
}

func parseBusinessDetail(body []byte, internalCode string) (BusinessDetail, error) {
	document, err := html.Parse(bytes.NewReader(body))
	if err != nil {
		return BusinessDetail{}, fmt.Errorf("상세 HTML 해석: %w", err)
	}
	tables := findElements(document, func(node *html.Node) bool {
		return node.Data == "table" && hasClass(node, "board_view") && captionContains(node, "업소 기본정보")
	})
	if len(tables) != 1 {
		return BusinessDetail{}, fmt.Errorf("국내업소 기본정보 table 수가 %d개입니다", len(tables))
	}
	values := make(map[string]string)
	for _, row := range tableRows(tables[0]) {
		cells := directCellChildren(row)
		for index := 0; index+1 < len(cells); index += 2 {
			label := normalizedText(cells[index])
			if cells[index].Data != "th" || label == "" {
				continue
			}
			values[label] = normalizedText(cells[index+1])
		}
	}
	detail := BusinessDetail{
		InternalBusinessCode: internalCode,
		LicenseNumber:        values["인허가번호"],
		BusinessName:         values["업소명"],
		Representative:       firstValue(values, "대표자", "대표자명"),
		PermitDate:           firstValue(values, "허가일자", "인허가일자", "허가일", "인허가일"),
		Institution:          firstValue(values, "인허가기관", "허가기관"),
		Address:              values["주소"],
		Phone:                values["연락처"],
		Industry:             values["인허가 종류"],
		OperatingStatus:      values["영업상태"],
	}
	if detail.LicenseNumber == "" || detail.BusinessName == "" {
		return BusinessDetail{}, fmt.Errorf("상세 기본정보에 인허가번호 또는 업소명이 없습니다")
	}
	return detail, nil
}

func parseGalleryPage(body []byte, request GallerySearchRequest) (GalleryPage, error) {
	document, err := html.Parse(bytes.NewReader(body))
	if err != nil {
		return GalleryPage{}, fmt.Errorf("갤러리 목록 HTML 해석: %w", err)
	}
	total, err := parseGalleryTotal(document)
	if err != nil {
		return GalleryPage{}, err
	}
	anchors := findElements(document, func(node *html.Node) bool {
		return node.Data == "a" && galleryProductPattern.MatchString(attribute(node, "href"))
	})
	products := make([]GalleryProduct, 0, len(anchors))
	seen := make(map[string]struct{}, len(anchors))
	for _, anchor := range anchors {
		match := galleryProductPattern.FindStringSubmatch(attribute(anchor, "href"))
		if len(match) != 2 {
			continue
		}
		code := strings.TrimSpace(match[1])
		if _, exists := seen[code]; exists {
			continue
		}
		seen[code] = struct{}{}
		titles := findElements(anchor, func(node *html.Node) bool {
			return node.Data == "strong" && hasClass(node, "title")
		})
		name := ""
		if len(titles) == 1 {
			name = normalizedText(titles[0])
		}
		products = append(products, GalleryProduct{ProductCode: code, ProductName: name})
	}
	expected := total - (request.Page-1)*request.Limit
	if expected < 0 {
		expected = 0
	}
	if expected > request.Limit {
		expected = request.Limit
	}
	if len(products) != expected {
		return GalleryPage{}, fmt.Errorf("갤러리 목록 결과 수가 정확하지 않습니다: expected=%d actual=%d", expected, len(products))
	}
	totalPages := 0
	if total > 0 {
		totalPages = (total + request.Limit - 1) / request.Limit
	}
	return GalleryPage{Total: total, Page: request.Page, Limit: request.Limit, TotalPages: totalPages, Products: products}, nil
}

func parseGalleryTotal(document *html.Node) (int, error) {
	counts := findElements(document, func(node *html.Node) bool {
		return node.Data == "div" && hasClass(node, "board_count") && totalPattern.MatchString(normalizedText(node))
	})
	if len(counts) == 0 {
		return 0, fmt.Errorf("갤러리 total을 찾을 수 없습니다")
	}
	match := totalPattern.FindStringSubmatch(normalizedText(counts[0]))
	total, err := strconv.Atoi(strings.ReplaceAll(match[1], ",", ""))
	if err != nil || total < 0 {
		return 0, fmt.Errorf("갤러리 total이 유효하지 않습니다: %q", match[1])
	}
	return total, nil
}

func parseGalleryDetail(body []byte, productCode string) (GalleryDetail, error) {
	match := galleryParamPattern.FindSubmatch(body)
	if len(match) != 2 {
		return GalleryDetail{}, fmt.Errorf("갤러리 상세 param 데이터를 찾을 수 없습니다")
	}
	var payload struct {
		RCNO                 string `json:"rcno"`
		InternalBusinessCode string `json:"grpBsnLcnsLedgNo"`
		BusinessName         string `json:"bsshNm"`
	}
	if err := json.Unmarshal(match[1], &payload); err != nil {
		return GalleryDetail{}, fmt.Errorf("갤러리 상세 param JSON 해석: %w", err)
	}
	payload.RCNO = strings.TrimSpace(payload.RCNO)
	payload.InternalBusinessCode = strings.TrimSpace(payload.InternalBusinessCode)
	payload.BusinessName = strings.TrimSpace(payload.BusinessName)
	if payload.RCNO == "" || payload.InternalBusinessCode == "" || payload.BusinessName == "" {
		return GalleryDetail{}, fmt.Errorf("갤러리 상세에 RCNO, 내부업소코드 또는 업소명이 없습니다")
	}
	return GalleryDetail{
		ProductCode: productCode, RCNO: payload.RCNO,
		InternalBusinessCode: payload.InternalBusinessCode, BusinessName: payload.BusinessName,
	}, nil
}

func parseSearchRow(row *html.Node) (SearchResult, error) {
	cells := directCellChildren(row)
	if len(cells) != 7 {
		return SearchResult{}, fmt.Errorf("cell 수가 %d개입니다", len(cells))
	}
	code, err := detailCode(cells[0])
	if err != nil {
		return SearchResult{}, err
	}
	rowNumber, err := strconv.Atoi(normalizedText(cells[0]))
	if err != nil || rowNumber < 1 {
		return SearchResult{}, fmt.Errorf("번호가 유효하지 않습니다: %q", normalizedText(cells[0]))
	}
	result := SearchResult{
		RowNumber:            rowNumber,
		InternalBusinessCode: code,
		LicenseNumber:        normalizedText(cells[1]),
		BusinessName:         normalizedText(cells[2]),
		Industry:             normalizedText(cells[3]),
		Representative:       normalizedText(cells[4]),
		Address:              normalizedText(cells[5]),
		Institution:          normalizedText(cells[6]),
	}
	if result.LicenseNumber == "" || result.BusinessName == "" {
		return SearchResult{}, fmt.Errorf("인허가번호 또는 업소명이 비어 있습니다")
	}
	result.DetailURL = DetailPath + "?srchBsshCode=" + urlQueryEscape(code)
	return result, nil
}

func detailCode(cell *html.Node) (string, error) {
	anchors := findElements(cell, func(node *html.Node) bool { return node.Data == "a" })
	if len(anchors) != 1 {
		return "", fmt.Errorf("상세 링크 수가 %d개입니다", len(anchors))
	}
	match := detailCodePattern.FindStringSubmatch(attribute(anchors[0], "href"))
	if len(match) != 2 || strings.TrimSpace(match[1]) == "" {
		return "", fmt.Errorf("상세 링크에서 내부업소코드를 찾을 수 없습니다")
	}
	return strings.TrimSpace(match[1]), nil
}

func parseTotal(section *html.Node) (int, error) {
	counts := findElements(section, func(node *html.Node) bool {
		return node.Data == "div" && hasClass(node, "board_count")
	})
	if len(counts) != 1 {
		return 0, fmt.Errorf("국내영업자 total container 수가 %d개입니다", len(counts))
	}
	match := totalPattern.FindStringSubmatch(normalizedText(counts[0]))
	if len(match) != 2 {
		return 0, fmt.Errorf("국내영업자 total을 찾을 수 없습니다")
	}
	total, err := strconv.Atoi(strings.ReplaceAll(match[1], ",", ""))
	if err != nil || total < 0 {
		return 0, fmt.Errorf("국내영업자 total이 유효하지 않습니다: %q", match[1])
	}
	return total, nil
}

func validateTableHeaders(table *html.Node, expected []string) error {
	headers := findElements(table, func(node *html.Node) bool { return node.Data == "th" })
	if len(headers) != len(expected) {
		return fmt.Errorf("목록 header 수가 %d개입니다", len(headers))
	}
	for index, header := range headers {
		if actual := normalizedText(header); actual != expected[index] {
			return fmt.Errorf("목록 header %d가 %q입니다, 예상 %q", index+1, actual, expected[index])
		}
	}
	return nil
}

func firstValue(values map[string]string, labels ...string) string {
	for _, label := range labels {
		if value := values[label]; value != "" {
			return value
		}
	}
	return ""
}

func findElements(root *html.Node, predicate func(*html.Node) bool) []*html.Node {
	var result []*html.Node
	var visit func(*html.Node)
	visit = func(node *html.Node) {
		if predicate(node) {
			result = append(result, node)
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			visit(child)
		}
	}
	visit(root)
	return result
}

func directChildrenByTag(root *html.Node, tag string) []*html.Node {
	var result []*html.Node
	for child := root.FirstChild; child != nil; child = child.NextSibling {
		if child.Type == html.ElementNode && child.Data == tag {
			result = append(result, child)
		}
	}
	return result
}

func directCellChildren(row *html.Node) []*html.Node {
	var result []*html.Node
	for child := row.FirstChild; child != nil; child = child.NextSibling {
		if child.Type == html.ElementNode && (child.Data == "td" || child.Data == "th") {
			result = append(result, child)
		}
	}
	return result
}

func tableRows(table *html.Node) []*html.Node {
	var result []*html.Node
	for _, body := range directChildrenByTag(table, "tbody") {
		result = append(result, directChildrenByTag(body, "tr")...)
	}
	return result
}

func nearestAncestor(node *html.Node, tag string) *html.Node {
	for parent := node.Parent; parent != nil; parent = parent.Parent {
		if parent.Data == tag {
			return parent
		}
	}
	return nil
}

func captionContains(node *html.Node, expected string) bool {
	for _, caption := range directChildrenByTag(node, "caption") {
		if strings.Contains(normalizedText(caption), expected) {
			return true
		}
	}
	return false
}

func hasClass(node *html.Node, className string) bool {
	for _, class := range strings.Fields(attribute(node, "class")) {
		if class == className {
			return true
		}
	}
	return false
}

func attribute(node *html.Node, name string) string {
	for _, attr := range node.Attr {
		if attr.Key == name {
			return attr.Val
		}
	}
	return ""
}

func normalizedText(node *html.Node) string {
	var builder strings.Builder
	var visit func(*html.Node)
	visit = func(current *html.Node) {
		if current.Type == html.TextNode {
			builder.WriteString(current.Data)
			return
		}
		for child := current.FirstChild; child != nil; child = child.NextSibling {
			visit(child)
		}
	}
	visit(node)
	raw := strings.TrimSpace(builder.String())
	var normalized strings.Builder
	space := false
	for _, character := range raw {
		if unicode.IsSpace(character) {
			space = true
			continue
		}
		if space && normalized.Len() > 0 {
			normalized.WriteByte(' ')
		}
		normalized.WriteRune(character)
		space = false
	}
	return normalized.String()
}

func urlQueryEscape(value string) string {
	return url.QueryEscape(value)
}
