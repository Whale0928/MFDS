package mfdsweb

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/bottle-note/mfds-crawler/internal/usecase/weblist"
)

func TestUsecaseAdapter_Fetch와Parse계약을무손실Mapping한다(t *testing.T) {
	fixture := readFixture(t, "list_page1.html")
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Query().Get("page") != "1" || request.URL.Query().Get("totalCnt") != "" {
			t.Errorf("query = %q", request.URL.RawQuery)
		}
		writer.Header().Set("Content-Type", "text/html")
		_, _ = writer.Write(fixture)
	}))
	defer server.Close()
	baseURL, err := url.Parse(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	client, err := NewClient(ClientOptions{BaseURL: baseURL})
	if err != nil {
		t.Fatal(err)
	}
	adapter := NewUsecaseAdapter(client)
	sourceRequest := whiskyRequest(1, 2, nil)
	request := weblist.ListRequest{
		ItemName: sourceRequest.Item.Name, ItemCode: sourceRequest.Item.Code,
		ProcessDate: sourceRequest.ProcessDate, Page: 1, Limit: 2,
	}

	fetch, err := adapter.FetchList(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	page, err := adapter.ParseList(fetch.Body, request)
	if err != nil {
		t.Fatal(err)
	}
	if !fetch.BodyCaptured || len(fetch.BodyGZIP) == 0 || page.Total != 4 || page.TotalPages != 2 ||
		len(page.Rows) != 2 || page.Rows[0].RCNO != "202600519647" {
		t.Fatalf("fetch/page = %+v/%+v", fetch, page)
	}
}
