package mfdscompany

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestScraper_SearchAndDetail_ParsesOfficialFieldsAndMetadata(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "text/html; charset=UTF-8")
		switch request.URL.Path {
		case ListPath:
			if request.URL.Query().Get("indutyCd") != "141" || request.URL.Query().Get("limit") != "2" {
				t.Fatalf("list query = %s", request.URL.RawQuery)
			}
			_, _ = writer.Write([]byte(listFixture))
		case DetailPath:
			if request.URL.Query().Get("srchBsshCode") != "2024001000000001" {
				t.Fatalf("detail query = %s", request.URL.RawQuery)
			}
			_, _ = writer.Write([]byte(detailFixture))
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()

	baseURL, err := url.Parse(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	wantRetrievedAt := time.Date(2026, time.August, 15, 1, 2, 3, 0, time.UTC)
	scraper, err := NewScraper(Options{BaseURL: baseURL, Now: func() time.Time { return wantRetrievedAt }})
	if err != nil {
		t.Fatal(err)
	}

	page, err := scraper.Search(context.Background(), SearchRequest{Page: 1, Limit: 2, IndustryCode: "141"})
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	if page.Total != 2 || page.TotalPages != 1 || len(page.Rows) != 2 {
		t.Fatalf("page = %+v", page)
	}
	if page.Rows[0].InternalBusinessCode != "2024001000000001" || page.Rows[0].LicenseNumber != "20240001234" ||
		page.Rows[0].BusinessName != "주식회사 테스트수입사" || page.Rows[0].Representative != "김**" ||
		page.Rows[0].Address != "서울특별시 중구 세종대로 1" || page.Rows[0].Industry != "수입식품등 수입판매업" ||
		page.Rows[0].Institution != "서울청" {
		t.Fatalf("first row = %+v", page.Rows[0])
	}
	if page.Rows[0].BusinessName != page.Rows[1].BusinessName || page.Rows[0].InternalBusinessCode == page.Rows[1].InternalBusinessCode {
		t.Fatalf("duplicate-name candidates were collapsed: %+v", page.Rows)
	}
	if !strings.Contains(page.Rows[0].DetailURL, "srchBsshCode=2024001000000001") ||
		page.Source.Endpoint != ListPath || page.Source.HTTPStatus != http.StatusOK ||
		page.Source.RequestURL == "" || len(page.Source.BodySHA256) != 64 || page.Source.RetrievedAt != wantRetrievedAt {
		t.Fatalf("list source = %+v", page.Source)
	}

	detail, err := scraper.Detail(context.Background(), page.Rows[0].InternalBusinessCode)
	if err != nil {
		t.Fatalf("Detail() error = %v", err)
	}
	wantDetail := BusinessDetail{
		InternalBusinessCode: "2024001000000001",
		LicenseNumber:        "20240001234",
		BusinessName:         "주식회사 테스트수입사",
		Representative:       "김대표",
		PermitDate:           "2024-01-02",
		Institution:          "서울청",
		Address:              "서울특별시 중구 세종대로 1",
		Phone:                "02-1234-5678",
		Industry:             "수입식품등 수입판매업",
		OperatingStatus:      "정상",
	}
	if !reflect.DeepEqual(detail.InternalBusinessCode, wantDetail.InternalBusinessCode) ||
		detail.LicenseNumber != wantDetail.LicenseNumber || detail.BusinessName != wantDetail.BusinessName ||
		detail.Representative != wantDetail.Representative || detail.PermitDate != wantDetail.PermitDate ||
		detail.Institution != wantDetail.Institution || detail.Address != wantDetail.Address ||
		detail.Phone != wantDetail.Phone || detail.Industry != wantDetail.Industry ||
		detail.OperatingStatus != wantDetail.OperatingStatus {
		t.Fatalf("detail = %+v, want %+v", detail, wantDetail)
	}
	if detail.Source.Endpoint != DetailPath || detail.Source.HTTPStatus != http.StatusOK || len(detail.Source.BodySHA256) != 64 {
		t.Fatalf("detail source = %+v", detail.Source)
	}
}

func TestScraper_Search_RejectsInexactResultCount(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "text/html")
		_, _ = writer.Write([]byte(strings.Replace(listFixture, "총 <strong>2</strong>건", "총 <strong>3</strong>건", 1)))
	}))
	defer server.Close()
	baseURL, _ := url.Parse(server.URL)
	scraper, err := NewScraper(Options{BaseURL: baseURL})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := scraper.Search(context.Background(), SearchRequest{Page: 2, Limit: 2, IndustryCode: "141"}); err == nil || !strings.Contains(err.Error(), "정확하지 않습니다") {
		t.Fatalf("Search() error = %v", err)
	}
}

func TestScraper_Search_EmptyResultIsSuccessful(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "text/html")
		_, _ = writer.Write([]byte(emptyListFixture))
	}))
	defer server.Close()
	baseURL, _ := url.Parse(server.URL)
	scraper, err := NewScraper(Options{BaseURL: baseURL})
	if err != nil {
		t.Fatal(err)
	}
	page, err := scraper.Search(context.Background(), SearchRequest{Page: 1, Limit: 2, IndustryCode: "141"})
	if err != nil || page.Total != 0 || len(page.Rows) != 0 {
		t.Fatalf("page = %+v error = %v", page, err)
	}
}

func TestScraper_Detail_RejectsNonHTML(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()
	baseURL, _ := url.Parse(server.URL)
	scraper, err := NewScraper(Options{BaseURL: baseURL})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := scraper.Detail(context.Background(), "2024001000000001"); err == nil || !strings.Contains(err.Error(), "Content-Type") {
		t.Fatalf("Detail() error = %v", err)
	}
}

const listFixture = `<!doctype html>
<html lang="ko"><body>
<ul><li>
<div class="board_count"><span>총 <strong>2</strong>건</span></div>
<table class="board_list auto_title mo-list">
<caption><span>국내영업자 검색 조회 테이블 :: 번호, 인허가번호, 업소명, 업종, 대표자, 소재지, 인허가기관 정보제공</span></caption>
<thead><tr><th>번호</th><th>인허가번호</th><th>업소명</th><th>업종</th><th>대표자</th><th>소재지</th><th>인허가기관</th></tr></thead>
<tbody>
<tr><td><a href="javascript:fnDetailPopup1(&#39;2024001000000001&#39;);">1</a></td><td>20240001234</td><td>주식회사 테스트수입사</td><td>수입식품등 수입판매업</td><td>김**</td><td>서울특별시 중구 세종대로 1</td><td>서울청</td></tr>
<tr><td><a href="javascript:fnDetailPopup1(&#39;2024001000000002&#39;);">2</a></td><td>20240001235</td><td>주식회사 테스트수입사</td><td>수입식품등 수입판매업</td><td>이**</td><td>부산광역시 해운대구 2</td><td>부산청</td></tr>
</tbody></table>
</li></ul></body></html>`

const emptyListFixture = `<!doctype html><html lang="ko"><body><ul><li>
<div class="board_count"><span>총 <strong>0</strong>건</span></div>
<table class="board_list auto_title mo-list"><caption><span>국내영업자 검색 조회 테이블</span></caption>
<thead><tr><th>번호</th><th>인허가번호</th><th>업소명</th><th>업종</th><th>대표자</th><th>소재지</th><th>인허가기관</th></tr></thead>
<tbody><tr><td colspan="7"><span class="nodata">조회 결과가 없습니다.</span></td></tr></tbody></table>
</li></ul></body></html>`

const detailFixture = `<!doctype html>
<html lang="ko"><body><section id="content_pop" class="popup">
<h2><span>국내업소</span></h2>
<table class="board_view auto_title mo-view"><caption><span>업소 기본정보 :: 업소명, 인허가번호, 연락처, 영업상태, 인허가 종류, 주소 정보제공</span></caption>
<tbody>
<tr><th><span>업소명</span></th><td>주식회사 테스트수입사</td><th><span>인허가번호</span></th><td>20240001234</td></tr>
<tr><th><span>연락처</span></th><td>02-1234-5678</td><th><span>영업상태</span></th><td>정상</td></tr>
<tr><th><span>인허가 종류</span></th><td colspan="3">수입식품등 수입판매업</td></tr>
<tr><th><span>주소</span></th><td colspan="3">서울특별시 중구 세종대로 1</td></tr>
<tr><th><span>대표자</span></th><td>김대표</td><th><span>허가일자</span></th><td>2024-01-02</td></tr>
<tr><th><span>인허가기관</span></th><td colspan="3">서울청</td></tr>
</tbody></table></section></body></html>`
