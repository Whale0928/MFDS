package mfdscompany

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"
)

func TestErrorClassification(t *testing.T) {
	for _, tc := range []struct {
		err   error
		code  string
		retry bool
	}{
		{context.Canceled, "CANCELLED", false},
		{context.DeadlineExceeded, "HTTP_TIMEOUT", true},
		{&HTTPStatusError{StatusCode: 503}, "HTTP_503", true},
		{&HTTPStatusError{StatusCode: 429}, "HTTP_429", true},
		{&HTTPStatusError{StatusCode: 403}, "HTTP_403", false},
		{&url.Error{Op: "Get", URL: "https://example.invalid", Err: errors.New("invalid certificate")}, "IMPORTER_VALIDATION_FAILED", false},
		{errors.New("invalid HTML"), "IMPORTER_VALIDATION_FAILED", false},
	} {
		t.Run(tc.code, func(t *testing.T) {
			err := &RequestError{Endpoint: ListPath, Err: tc.err}
			if ErrorCode(err) != tc.code || IsRetryable(err) != tc.retry {
				t.Fatalf("code=%s retry=%v", ErrorCode(err), IsRetryable(err))
			}
		})
	}
}

func TestScraperTimeoutRetainsEndpoint(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { <-r.Context().Done() }))
	defer server.Close()
	base, _ := url.Parse(server.URL)
	scraper, err := NewScraper(Options{BaseURL: base, HTTPClient: &http.Client{Timeout: 20 * time.Millisecond}})
	if err != nil {
		t.Fatal(err)
	}
	_, err = scraper.Search(context.Background(), SearchRequest{Page: 1, Limit: 50, IndustryCode: "141"})
	var requestError *RequestError
	if !errors.As(err, &requestError) || requestError.Endpoint != ListPath || ErrorCode(err) != "HTTP_TIMEOUT" {
		t.Fatalf("err=%v", err)
	}
	defaultScraper, err := NewScraper(Options{BaseURL: base})
	if err != nil {
		t.Fatal(err)
	}
	if defaultScraper.httpClient.Timeout != 60*time.Second {
		t.Fatalf("timeout=%v", defaultScraper.httpClient.Timeout)
	}
}
