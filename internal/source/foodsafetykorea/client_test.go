package foodsafetykorea

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/bottle-note/mfds-crawler/internal/usecase/companyregistry"
)

const testAPIKey = "test-secret-api-key"

func TestFetchC001_정상응답_Raw와HTTP메타데이터를보존한다(t *testing.T) {
	rowJSON := `{"PRSDNT_NM":"대표","PRMS_DT":"20200123","LCNS_NO":"L-1","INSTT_NM":"기관","BSSH_NM":"업체","LOCP_ADDR":"주소","TELNO":"02-0000","INDUTY_NM":"업종","EXTRA":"keep"}`
	body := []byte(`{"C001":{"total_count":"81805","row":[` + rowJSON + `],"RESULT":{"MSG":"정상처리되었습니다.","CODE":"INFO-000"}}}`)
	var observedScheme atomic.Value
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet {
			t.Errorf("method = %s", request.Method)
		}
		if request.URL.Path != "/api/"+testAPIKey+"/C001/json/1/1000/CHNG_DT=20260809&LCNS_NO=prefix/"+testAPIKey+"/A/B" {
			t.Errorf("path = %q", request.URL.Path)
		}
		if request.Header.Get("Accept") != "application/json" || request.Header.Get("Accept-Encoding") != "identity" {
			t.Errorf("headers = %#v", request.Header)
		}
		writer.Header().Set("Content-Type", "application/json; charset=utf-8")
		writer.Header().Set("X-Request-URL", "https://example.test/api/"+testAPIKey+"/C001")
		writeBody(t, writer, body)
	}))
	defer server.Close()

	client := newServerClient(t, server, &observedScheme)
	page, err := client.FetchC001(context.Background(), PageRequest{
		StartIndex: 1,
		EndIndex:   1000,
		Filters: map[string]string{
			"LCNS_NO": "prefix/" + testAPIKey + "/A/B",
			"CHNG_DT": "20260809",
		},
	})
	if err != nil {
		t.Fatalf("FetchC001() error = %v", err)
	}
	if observedScheme.Load() != "https" {
		t.Fatalf("original request scheme = %v", observedScheme.Load())
	}
	if page.TotalCount != "81805" || page.Result.Code != "INFO-000" || len(page.Rows) != 1 {
		t.Fatalf("page result = total %q, code %q, rows %d", page.TotalCount, page.Result.Code, len(page.Rows))
	}
	if page.Rows[0].LicenseNumber != "L-1" || string(page.Rows[0].RawJSON) != rowJSON {
		t.Fatalf("row = %+v, raw = %s", page.Rows[0], page.Rows[0].RawJSON)
	}
	if !bytes.Equal(page.RawBody, body) || page.HTTP.StatusCode != http.StatusOK || page.HTTP.BodySize != int64(len(body)) {
		t.Fatalf("raw/http metadata = %d bytes, status %d, size %d", len(page.RawBody), page.HTTP.StatusCode, page.HTTP.BodySize)
	}
	assertRedacted(t, page.HTTP.URL, string(page.SnapshotJSON), page.HTTP.ResponseHeaders.Get("X-Request-URL"), fmt.Sprintf("%+v", client))
	if !strings.Contains(page.HTTP.URL, "/api/"+MaskedAPIKey+"/C001/") {
		t.Fatalf("masked URL = %q", page.HTTP.URL)
	}
}

func TestFetchI2821_정상응답_9개필드와Raw를Decode한다(t *testing.T) {
	row := `{"CLSBIZ_DT":"20040515","PRSDNT_NM":"대표","PRMS_DT":"18991230","LCNS_NO":"L","INSTT_NM":"기관","BSSH_NM":"업체","CLSBIZ_DVS_CD_NM":"폐업","LOCP_ADDR":"주소","INDUTY_NM":"업종"}`
	server := serviceJSONServer(t, ServiceI2821, row)
	defer server.Close()

	page, err := newServerClient(t, server, nil).FetchI2821(context.Background(), PageRequest{StartIndex: 1, EndIndex: 1})
	if err != nil || len(page.Rows) != 1 || page.Rows[0].ClosureDate != "20040515" || !json.Valid(page.Rows[0].RawJSON) {
		t.Fatalf("FetchI2821() page=%+v error=%v", page, err)
	}
}

func TestFetchI0250_정상응답_9개필드와Raw를Decode한다(t *testing.T) {
	row := `{"EXCOURY_NATN_CD_NM":"미국","INCM_PRDT_XPORT_MC_NM":"수출사","PRMS_DT":"20190611","PRDLST_CNT":"7","LCNS_NO":"L","PRDLST_NM":"제품","EXCLNC_INCM_BSSH_REGNO":"R","BSSH_NM":"업체","ADDR":"주소"}`
	server := serviceJSONServer(t, ServiceI0250, row)
	defer server.Close()

	page, err := newServerClient(t, server, nil).FetchI0250(context.Background(), PageRequest{StartIndex: 1, EndIndex: 1})
	if err != nil || len(page.Rows) != 1 || page.Rows[0].ProductCount != "7" || !json.Valid(page.Rows[0].RawJSON) {
		t.Fatalf("FetchI0250() page=%+v error=%v", page, err)
	}
}

func TestFetchI0470_정상응답_17개필드와Raw를Decode한다(t *testing.T) {
	row := `{"PRSDNT_NM":"대표","LAST_UPDT_DTM":"2024-08-13 18:41:31.649","LCNS_NO":"L","DSPS_INSTTCD_NM":"기관","LAWORD_CD_NM":"법령","DSPSDTLS_SEQ":"1","VILTCN":"위반","ADDR":"주소","PUBLIC_DT":"2026-08-11 00:00:00.0","INDUTY_CD_NM":"업종","DSPS_DCSNDT":"20240812","PRCSCITYPOINT_BSSHNM":"업체","DSPS_BGNDT":"20240812","DSPS_TYPECD_NM":"처분","DSPS_ENDDT":"-","TELNO":"02","DSPSCN":"내용"}`
	server := serviceJSONServer(t, ServiceI0470, row)
	defer server.Close()

	page, err := newServerClient(t, server, nil).FetchI0470(context.Background(), PageRequest{StartIndex: 1, EndIndex: 1})
	if err != nil || len(page.Rows) != 1 || page.Rows[0].DispositionEndDate != "-" || !json.Valid(page.Rows[0].RawJSON) {
		t.Fatalf("FetchI0470() page=%+v error=%v", page, err)
	}
}

func TestFetchC001_INFO200_Rows없는성공으로처리한다(t *testing.T) {
	body := []byte(`{"C001":{"total_count":"0","row":null,"RESULT":{"MSG":"해당 데이터 없음","CODE":"INFO-200"}}}`)
	server := jsonServer(t, body)
	defer server.Close()

	client := newServerClient(t, server, nil)
	page, err := client.FetchC001(context.Background(), PageRequest{StartIndex: 1, EndIndex: 1})
	if err != nil {
		t.Fatalf("FetchC001() error = %v", err)
	}
	if page.Result.Code != "INFO-200" || len(page.Rows) != 0 || !bytes.Equal(page.RawBody, body) {
		t.Fatalf("page = %+v", page)
	}
}

func TestFetchC001_HTML빈Body비JSON200_명시적오류와Raw를반환한다(t *testing.T) {
	tests := []struct {
		name        string
		contentType string
		body        []byte
		wantKind    ErrorKind
	}{
		{name: "HTML", contentType: "text/html;charset=utf-8", body: []byte("<html>temporary</html>"), wantKind: ErrorKindHTML},
		{name: "empty", contentType: "", body: nil, wantKind: ErrorKindEmptyBody},
		{name: "non JSON content type", contentType: "text/plain", body: []byte("not json"), wantKind: ErrorKindNonJSON},
		{name: "invalid JSON", contentType: "application/json", body: []byte("not json"), wantKind: ErrorKindNonJSON},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
				if test.contentType != "" {
					writer.Header().Set("Content-Type", test.contentType)
				}
				writeBody(t, writer, test.body)
			}))
			defer server.Close()
			client := newServerClient(t, server, nil)

			page, err := client.FetchC001(context.Background(), PageRequest{StartIndex: 1, EndIndex: 1})
			if errorKind(err) != test.wantKind {
				t.Fatalf("FetchC001() error = %v, want kind %s", err, test.wantKind)
			}
			if !bytes.Equal(page.RawBody, test.body) || page.HTTP.StatusCode != http.StatusOK {
				t.Fatalf("raw/status = %q/%d", page.RawBody, page.HTTP.StatusCode)
			}
		})
	}
}

func TestFetchC001_Redirect_따라가지않고메타데이터를반환한다(t *testing.T) {
	var redirected atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path == "/next" {
			redirected.Store(true)
			writer.Header().Set("Content-Type", "application/json")
			writeBody(t, writer, []byte(`{"unexpected":true}`))
			return
		}
		http.Redirect(writer, request, "/next", http.StatusFound)
	}))
	defer server.Close()

	client := newServerClient(t, server, nil)
	page, err := client.FetchC001(context.Background(), PageRequest{StartIndex: 1, EndIndex: 1})
	if errorKind(err) != ErrorKindHTTP {
		t.Fatalf("FetchC001() error = %v", err)
	}
	if redirected.Load() || page.HTTP.StatusCode != http.StatusFound || len(page.RawBody) == 0 {
		t.Fatalf("redirected=%t status=%d raw=%q", redirected.Load(), page.HTTP.StatusCode, page.RawBody)
	}
}

func TestFetchC001_API오류와Wrapper불일치_계약오류를반환한다(t *testing.T) {
	tests := []struct {
		name     string
		body     []byte
		wantKind ErrorKind
	}{
		{name: "API code", body: []byte(`{"C001":{"RESULT":{"MSG":"인증키 무효 ` + testAPIKey + `","CODE":"INFO-100"}}}`), wantKind: ErrorKindAPI},
		{name: "top-level API code", body: []byte(`{"RESULT":{"MSG":"권한 없음","CODE":"INFO-400"}}`), wantKind: ErrorKindAPI},
		{name: "wrong wrapper", body: []byte(`{"I2821":{"total_count":"0","row":[],"RESULT":{"MSG":"정상","CODE":"INFO-000"}}}`), wantKind: ErrorKindContract},
		{name: "number total", body: []byte(`{"C001":{"total_count":1,"row":[],"RESULT":{"MSG":"정상","CODE":"INFO-000"}}}`), wantKind: ErrorKindContract},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := jsonServer(t, test.body)
			defer server.Close()
			client := newServerClient(t, server, nil)
			page, err := client.FetchC001(context.Background(), PageRequest{StartIndex: 1, EndIndex: 1})
			if errorKind(err) != test.wantKind {
				t.Fatalf("FetchC001() error = %v", err)
			}
			assertRedacted(t, err.Error())
			if !bytes.Equal(page.RawBody, test.body) || page.HTTP.StatusCode != http.StatusOK {
				t.Fatal("error page did not preserve raw body and HTTP metadata")
			}
		})
	}
}

func TestValidatePageRequest_Page범위경계_1부터1000건만허용한다(t *testing.T) {
	validRequests := []PageRequest{
		{StartIndex: 1, EndIndex: 1},
		{StartIndex: 1, EndIndex: 1000},
		{StartIndex: 1001, EndIndex: 2000},
	}
	for _, request := range validRequests {
		if err := validatePageRequest(request); err != nil {
			t.Errorf("validatePageRequest(%+v) error = %v", request, err)
		}
	}
	invalidRequests := []PageRequest{
		{StartIndex: 0, EndIndex: 1},
		{StartIndex: 2, EndIndex: 1},
		{StartIndex: 1, EndIndex: 1001},
	}
	for _, request := range invalidRequests {
		if err := validatePageRequest(request); errorKind(err) != ErrorKindValidation {
			t.Errorf("validatePageRequest(%+v) error = %v", request, err)
		}
	}
}

func TestFetchPage_서비스별미지원필터_요청전에거부한다(t *testing.T) {
	tests := []struct {
		name    string
		service ServiceID
		filter  map[string]string
	}{
		{name: "I0250 has no filter", service: ServiceI0250, filter: map[string]string{"CHNG_DT": "20260809"}},
		{name: "C001 does not support decision date", service: ServiceC001, filter: map[string]string{"DSPS_DCSNDT": "20260809"}},
		{name: "I0470 filter value is empty", service: ServiceI0470, filter: map[string]string{"LCNS_NO": " "}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			client, err := NewClient(ClientOptions{APIKey: testAPIKey})
			if err != nil {
				t.Fatal(err)
			}
			_, err = fetchPage[json.RawMessage](context.Background(), client, test.service, PageRequest{
				StartIndex: 1,
				EndIndex:   1,
				Filters:    test.filter,
			})
			if errorKind(err) != ErrorKindValidation {
				t.Fatalf("fetchPage() error = %v", err)
			}
		})
	}
}

func TestUsecaseAdapter_HTML200_Raw메타데이터와재시도분류를보존한다(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "text/html;charset=utf-8")
		writeBody(t, writer, []byte("<html>temporary</html>"))
	}))
	defer server.Close()
	adapter := NewUsecaseAdapter(newServerClient(t, server, nil))

	page, err := adapter.FetchPage(context.Background(), companyregistry.PageRequest{
		Service: companyregistry.ServiceC001, StartIndex: 1, EndIndex: 1, Attempt: 2,
	})

	var retryable companyregistry.RetryableError
	if !errors.As(err, &retryable) || !retryable.Retryable() {
		t.Fatalf("FetchPage() error = %v", err)
	}
	var classified interface{ Kind() string }
	if !errors.As(err, &classified) || classified.Kind() != string(ErrorKindHTML) {
		t.Fatalf("FetchPage() error kind = %v", err)
	}
	if page.HTTPStatus != http.StatusOK || page.ContentType != "text/html;charset=utf-8" ||
		string(page.RawBody) != "<html>temporary</html>" || page.Attempt != 2 {
		t.Fatalf("page = %+v", page)
	}
}

func TestUsecaseAdapter_정상응답_Raw행과Count를변환한다(t *testing.T) {
	row := `{"EXCOURY_NATN_CD_NM":"미국","INCM_PRDT_XPORT_MC_NM":"수출사","PRMS_DT":"20190611","PRDLST_CNT":"7","LCNS_NO":"L","PRDLST_NM":"제품","EXCLNC_INCM_BSSH_REGNO":"R","BSSH_NM":"업체","ADDR":"주소"}`
	server := serviceJSONServer(t, ServiceI0250, row)
	defer server.Close()
	adapter := NewUsecaseAdapter(newServerClient(t, server, nil))

	page, err := adapter.FetchPage(context.Background(), companyregistry.PageRequest{
		Service: companyregistry.ServiceI0250, StartIndex: 1, EndIndex: 1, Attempt: 1,
	})

	if err != nil {
		t.Fatal(err)
	}
	if page.TotalCount != 1 || len(page.Rows) != 1 || string(page.Rows[0]) != row ||
		page.ResultCode != "INFO-000" || page.FinishedAt.Before(page.StartedAt.Add(-time.Nanosecond)) {
		t.Fatalf("page = %+v", page)
	}
}

func TestFetchC001_Body크기경계_16MiB초과만거부한다(t *testing.T) {
	validBody := make([]byte, MaxResponseBodySize)
	copy(validBody, `{"C001":{"total_count":"0","row":[],"RESULT":{"MSG":"정상","CODE":"INFO-000"}}}`)
	for index := len(`{"C001":{"total_count":"0","row":[],"RESULT":{"MSG":"정상","CODE":"INFO-000"}}}`); index < len(validBody); index++ {
		validBody[index] = ' '
	}
	invalidBody := append(append([]byte(nil), validBody...), ' ')

	tests := []struct {
		name      string
		body      []byte
		wantError ErrorKind
	}{
		{name: "exactly 16MiB", body: validBody},
		{name: "over 16MiB", body: invalidBody, wantError: ErrorKindBodyLimit},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := jsonServer(t, test.body)
			defer server.Close()
			client := newServerClient(t, server, nil)
			page, err := client.FetchC001(context.Background(), PageRequest{StartIndex: 1, EndIndex: 1})
			if errorKind(err) != test.wantError {
				t.Fatalf("FetchC001() error = %v, want kind %s", err, test.wantError)
			}
			if test.wantError == ErrorKindBodyLimit && (!page.HTTP.BodyTruncated || len(page.RawBody) != int(MaxResponseBodySize)) {
				t.Fatalf("truncated=%t raw size=%d", page.HTTP.BodyTruncated, len(page.RawBody))
			}
		})
	}
}

func TestFetchC001_Network오류_Key를Error와Snapshot에서마스킹한다(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	client := newServerClient(t, server, nil)
	server.Close()

	page, err := client.FetchC001(context.Background(), PageRequest{StartIndex: 1, EndIndex: 1})
	if errorKind(err) != ErrorKindNetwork {
		t.Fatalf("FetchC001() error = %v", err)
	}
	assertRedacted(t, err.Error(), page.HTTP.URL, string(page.SnapshotJSON), fmt.Sprintf("%#v", client))
}

func newServerClient(t *testing.T, server *httptest.Server, observedScheme *atomic.Value) *Client {
	t.Helper()
	serverURL, err := url.Parse(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	client, err := NewClient(ClientOptions{
		APIKey: testAPIKey,
		HTTPClient: &http.Client{Transport: &rewriteTransport{
			target:         serverURL,
			base:           http.DefaultTransport,
			observedScheme: observedScheme,
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	return client
}

type rewriteTransport struct {
	target         *url.URL
	base           http.RoundTripper
	observedScheme *atomic.Value
}

func (transport *rewriteTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	if transport.observedScheme != nil {
		transport.observedScheme.Store(request.URL.Scheme)
	}
	rewritten := request.Clone(request.Context())
	rewritten.URL.Scheme = transport.target.Scheme
	rewritten.URL.Host = transport.target.Host
	rewritten.Host = transport.target.Host
	return transport.base.RoundTrip(rewritten)
}

func jsonServer(t *testing.T, body []byte) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		writeBody(t, writer, body)
	}))
}

func serviceJSONServer(t *testing.T, serviceID ServiceID, row string) *httptest.Server {
	t.Helper()
	body := []byte(fmt.Sprintf(`{"%s":{"total_count":"1","row":[%s],"RESULT":{"MSG":"정상","CODE":"INFO-000"}}}`, serviceID, row))
	return httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if !strings.Contains(request.URL.Path, "/"+string(serviceID)+"/json/1/1") {
			t.Errorf("path = %q", request.URL.Path)
		}
		writer.Header().Set("Content-Type", "application/json")
		writeBody(t, writer, body)
	}))
}

func writeBody(t *testing.T, writer http.ResponseWriter, body []byte) {
	t.Helper()
	if _, err := writer.Write(body); err != nil {
		t.Errorf("ResponseWriter.Write() error = %v", err)
	}
}

func errorKind(err error) ErrorKind {
	if err == nil {
		return ""
	}
	var adapterErr *AdapterError
	if errors.As(err, &adapterErr) {
		return adapterErr.Kind
	}
	return ""
}

func assertRedacted(t *testing.T, values ...string) {
	t.Helper()
	for _, value := range values {
		if strings.Contains(value, testAPIKey) || strings.Contains(value, url.PathEscape(testAPIKey)) {
			t.Fatalf("API key leaked: %q", value)
		}
	}
}

var _ http.RoundTripper = (*rewriteTransport)(nil)
