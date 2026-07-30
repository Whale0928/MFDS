package mfdsweb

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestFetchList_Page1_익명GET과원문을보존한다(t *testing.T) {
	fixture := readFixture(t, "list_page1.html")
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet || request.URL.Path != ListPath {
			t.Errorf("request = %s %s", request.Method, request.URL.Path)
		}
		wantQuery := url.Values{
			"limit":      {"2"},
			"page":       {"1"},
			"rpsntItmCd": {"C0314210000000000000"},
			"rpsntItmNm": {"위스키"},
			"srchEndDt":  {"2026-07-27"},
			"srchStrtDt": {"2026-07-27"},
		}
		if request.URL.Query().Encode() != wantQuery.Encode() {
			t.Errorf("query = %q, want %q", request.URL.Query().Encode(), wantQuery.Encode())
		}
		if request.Header.Get("User-Agent") != DefaultUserAgent ||
			request.Header.Get("Accept") != "text/html" ||
			request.Header.Get("Accept-Encoding") != "identity" {
			t.Errorf("headers = %#v", request.Header)
		}
		if request.Header.Get("Cookie") != "" || request.Header.Get("Authorization") != "" {
			t.Errorf("secret-bearing request headers = %#v", request.Header)
		}
		writer.Header().Set("Content-Type", "text/html; charset=UTF-8")
		writer.Header().Set("ETag", "fixture")
		writer.Header().Set("Set-Cookie", "SESSION=secret")
		writer.Header().Set("X-Trace-Token", "secret-token")
		_, _ = writer.Write(fixture)
	}))
	defer server.Close()

	client := newTestClient(t, server, ClientOptions{})
	artifact, err := client.FetchList(context.Background(), whiskyRequest(1, 2, nil))
	if err != nil {
		t.Fatalf("FetchList() error = %v", err)
	}
	if artifact.HTTPStatus != http.StatusOK || !bytes.Equal(artifact.Body, fixture) {
		t.Fatalf("artifact status/body = %d/%d bytes", artifact.HTTPStatus, len(artifact.Body))
	}
	if artifact.BodySHA256 != sha256.Sum256(fixture) || artifact.BodySize != int64(len(fixture)) {
		t.Fatalf("artifact hash/size = %x/%d", artifact.BodySHA256, artifact.BodySize)
	}
	if strings.Contains(string(artifact.ResponseHeadersJSON), "secret") ||
		strings.Contains(string(artifact.ResponseHeadersJSON), "Set-Cookie") ||
		strings.Contains(string(artifact.ResponseHeadersJSON), "X-Trace") {
		t.Fatalf("response headers contain secret: %s", artifact.ResponseHeadersJSON)
	}
	if artifact.RequestKeySHA256 != sha256.Sum256([]byte(ListPath+"?"+buildListQuery(whiskyRequest(1, 2, nil)).Encode())) {
		t.Fatalf("request key = %x", artifact.RequestKeySHA256)
	}
}

func TestFetchList_HTTPClientCookieJar를사용하지않는다(t *testing.T) {
	fixture := readFixture(t, "list_page1.html")
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Header.Get("Cookie") != "" {
			t.Errorf("Cookie header = %q", request.Header.Get("Cookie"))
		}
		writer.Header().Set("Content-Type", "text/html")
		_, _ = writer.Write(fixture)
	}))
	defer server.Close()

	baseURL, err := url.Parse(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	jar.SetCookies(baseURL, []*http.Cookie{{Name: "SESSION", Value: "secret"}})
	client, err := NewClient(ClientOptions{
		BaseURL:    baseURL,
		HTTPClient: &http.Client{Jar: jar},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.FetchList(context.Background(), whiskyRequest(1, 2, nil)); err != nil {
		t.Fatalf("FetchList() error = %v", err)
	}
}

func TestNewClient_호출자HTTPClient를변경하지않고비양수Timeout을정규화한다(t *testing.T) {
	baseURL, err := url.Parse("https://example.com")
	if err != nil {
		t.Fatal(err)
	}
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	redirect := func(*http.Request, []*http.Request) error {
		return nil
	}
	redirectRequest, err := http.NewRequest(http.MethodGet, "https://example.com/next", nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, timeout := range []time.Duration{0, -time.Second} {
		t.Run(timeout.String(), func(t *testing.T) {
			original := &http.Client{
				Timeout:       timeout,
				Jar:           jar,
				CheckRedirect: redirect,
			}

			client, err := NewClient(ClientOptions{BaseURL: baseURL, HTTPClient: original})
			if err != nil {
				t.Fatalf("NewClient() error = %v", err)
			}
			if original.Timeout != timeout || original.Jar != jar || original.CheckRedirect == nil {
				t.Fatal("caller HTTP client was mutated")
			}
			if err := original.CheckRedirect(redirectRequest, nil); err != nil {
				t.Fatalf("caller CheckRedirect() error = %v", err)
			}
			if client.httpClient == original ||
				client.httpClient.Timeout != DefaultRequestTimeout ||
				client.httpClient.Jar != nil {
				t.Fatalf("adapter client = %+v", client.httpClient)
			}
			if err := client.httpClient.CheckRedirect(redirectRequest, nil); !errors.Is(err, http.ErrUseLastResponse) {
				t.Fatalf("adapter CheckRedirect() error = %v", err)
			}
		})
	}
}

func TestFetchList_SlowServer와ContextCancel_요청을중단한다(t *testing.T) {
	fixture := readFixture(t, "list_page1.html")
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		select {
		case <-request.Context().Done():
			return
		case <-time.After(200 * time.Millisecond):
			writer.Header().Set("Content-Type", "text/html")
			_, _ = writer.Write(fixture)
		}
	}))
	defer server.Close()

	t.Run("client timeout", func(t *testing.T) {
		baseURL, err := url.Parse(server.URL)
		if err != nil {
			t.Fatal(err)
		}
		client, err := NewClient(ClientOptions{
			BaseURL:      baseURL,
			HTTPClient:   &http.Client{Timeout: 20 * time.Millisecond},
			MaxBodyBytes: DefaultMaxBodyBytes,
		})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := client.FetchList(context.Background(), whiskyRequest(1, 2, nil)); errorKind(err) != ErrorKindNetwork {
			t.Fatalf("FetchList() error = %v", err)
		}
	})
	t.Run("context cancel", func(t *testing.T) {
		client := newTestClient(t, server, ClientOptions{})
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		if _, err := client.FetchList(ctx, whiskyRequest(1, 2, nil)); errorKind(err) != ErrorKindNetwork {
			t.Fatalf("FetchList() error = %v", err)
		}
	})
}

func TestFetchList_MisleadingContentLength_실제Body크기로제한한다(t *testing.T) {
	baseURL, err := url.Parse("https://example.com")
	if err != nil {
		t.Fatal(err)
	}
	client, err := NewClient(ClientOptions{
		BaseURL: baseURL,
		HTTPClient: &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode:    http.StatusOK,
				Header:        http.Header{"Content-Type": {"text/html"}, "Content-Length": {"1"}},
				Body:          io.NopCloser(strings.NewReader("12345")),
				ContentLength: 1,
			}, nil
		})},
		MaxBodyBytes: 4,
	})
	if err != nil {
		t.Fatal(err)
	}

	artifact, err := client.FetchList(context.Background(), whiskyRequest(1, 2, nil))
	if errorKind(err) != ErrorKindBodyLimit {
		t.Fatalf("FetchList() error = %v", err)
	}
	if len(artifact.Body) != 0 {
		t.Fatal("misleading Content-Length bypassed body bound")
	}
}

func TestFetchList_Page2_TotalSnapshot을전달한다(t *testing.T) {
	fixture := readFixture(t, "list_page2.html")
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Query().Get("totalCnt") != "4" || request.URL.Query().Get("page") != "2" {
			t.Errorf("query = %q", request.URL.RawQuery)
		}
		writer.Header().Set("Content-Type", "text/html")
		_, _ = writer.Write(fixture)
	}))
	defer server.Close()

	total := 4
	client := newTestClient(t, server, ClientOptions{})
	artifact, err := client.FetchList(context.Background(), whiskyRequest(2, 2, &total))
	if err != nil {
		t.Fatalf("FetchList() error = %v", err)
	}
	var query map[string]string
	if err := json.Unmarshal(artifact.QueryJSON, &query); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if query["totalCnt"] != "4" || len(query) != 7 {
		t.Fatalf("query snapshot = %#v", query)
	}
}

func TestFetchList_Redirect_따라가지않고Raw를반환한다(t *testing.T) {
	var redirected atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path == ListPath {
			http.Redirect(writer, request, "/error", http.StatusFound)
			return
		}
		redirected.Store(true)
		writer.Header().Set("Content-Type", "text/html")
		_, _ = writer.Write([]byte("<title>Error</title>"))
	}))
	defer server.Close()

	client := newTestClient(t, server, ClientOptions{})
	artifact, err := client.FetchList(context.Background(), whiskyRequest(1, 2, nil))
	if errorKind(err) != ErrorKindHTTP {
		t.Fatalf("FetchList() error = %v", err)
	}
	if artifact.HTTPStatus != http.StatusFound || redirected.Load() {
		t.Fatalf("status=%d redirected=%t", artifact.HTTPStatus, redirected.Load())
	}
}

func TestFetchList_응답제한과ContentType_오류를반환한다(t *testing.T) {
	tests := []struct {
		name        string
		contentType string
		body        string
		maximum     int64
		wantKind    ErrorKind
	}{
		{name: "oversize", contentType: "text/html", body: "12345", maximum: 4, wantKind: ErrorKindBodyLimit},
		{name: "json", contentType: "application/json", body: "{}", maximum: 10, wantKind: ErrorKindContentType},
		{name: "invalid charset", contentType: "text/html; charset=euc-kr", body: "ok", maximum: 10, wantKind: ErrorKindContentType},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
				writer.Header().Set("Content-Type", test.contentType)
				_, _ = writer.Write([]byte(test.body))
			}))
			defer server.Close()

			client := newTestClient(t, server, ClientOptions{MaxBodyBytes: test.maximum})
			artifact, err := client.FetchList(context.Background(), whiskyRequest(1, 2, nil))
			if errorKind(err) != test.wantKind {
				t.Fatalf("FetchList() error = %v, want kind %s", err, test.wantKind)
			}
			if test.wantKind == ErrorKindContentType && !bytes.Equal(artifact.Body, []byte(test.body)) {
				t.Fatal("invalid content raw body was not preserved")
			}
			if test.wantKind == ErrorKindBodyLimit && len(artifact.Body) != 0 {
				t.Fatal("partial oversized body was exposed as complete")
			}
		})
	}
}

func TestNewClient_비밀값포함BaseURL_오류를반환한다(t *testing.T) {
	for _, rawURL := range []string{
		"https://user@example.com",
		"https://example.com?token=secret",
		"https://example.com?",
		"https://example.com#fragment",
		"ftp://example.com",
		"/relative",
	} {
		t.Run(rawURL, func(t *testing.T) {
			parsed, err := url.Parse(rawURL)
			if err != nil {
				t.Fatal(err)
			}
			_, err = NewClient(ClientOptions{BaseURL: parsed})
			if errorKind(err) != ErrorKindValidation {
				t.Fatalf("NewClient() error = %v", err)
			}
		})
	}

	invalidURLs := []*url.URL{
		{Scheme: "https", Host: ":443"},
		{Scheme: "https", Host: "example.com", Opaque: "opaque"},
	}
	for index, parsed := range invalidURLs {
		if _, err := NewClient(ClientOptions{BaseURL: parsed}); errorKind(err) != ErrorKindValidation {
			t.Fatalf("invalid URL %d error = %v", index, err)
		}
	}
}

func TestFetchList_유효하지않은요청_Server를호출하지않는다(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		calls.Add(1)
	}))
	defer server.Close()
	client := newTestClient(t, server, ClientOptions{})
	total := 4

	tests := []ListRequest{
		whiskyRequest(1, 0, nil),
		whiskyRequest(1, 51, nil),
		whiskyRequest(1, 2, &total),
		whiskyRequest(2, 2, nil),
		whiskyRequest(3, 2, &total),
		{Item: Item{Name: " 위스키", Code: "C0314210000000000000"}, ProcessDate: time.Now(), Page: 1, Limit: 2},
		{Item: Item{Name: "위스키", Code: "bad-code"}, ProcessDate: time.Now(), Page: 1, Limit: 2},
	}
	for index, request := range tests {
		if _, err := client.FetchList(context.Background(), request); errorKind(err) != ErrorKindValidation {
			t.Fatalf("case %d error = %v", index, err)
		}
	}
	if calls.Load() != 0 {
		t.Fatalf("server calls = %d", calls.Load())
	}
}

func newTestClient(t *testing.T, server *httptest.Server, overrides ClientOptions) *Client {
	t.Helper()
	baseURL, err := url.Parse(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	overrides.BaseURL = baseURL
	overrides.HTTPClient = server.Client()
	client, err := NewClient(overrides)
	if err != nil {
		t.Fatal(err)
	}
	return client
}

func whiskyRequest(page, limit int, total *int) ListRequest {
	location := time.FixedZone("Asia/Seoul", 9*60*60)
	return ListRequest{
		Item:          Item{Name: "위스키", Code: "C0314210000000000000"},
		ProcessDate:   time.Date(2026, time.July, 27, 0, 0, 0, 0, location),
		Page:          page,
		Limit:         limit,
		TotalSnapshot: total,
	}
}

func readFixture(t *testing.T, name string) []byte {
	t.Helper()
	body, err := os.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatal(err)
	}
	return body
}

func errorKind(err error) ErrorKind {
	var adapterError *AdapterError
	if !errors.As(err, &adapterError) {
		return ""
	}
	return adapterError.Kind
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (function roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return function(request)
}
