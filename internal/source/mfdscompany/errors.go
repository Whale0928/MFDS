package mfdscompany

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/url"
)

// RequestError retains the endpoint without requiring query strings in logs.
type RequestError struct {
	Endpoint string
	Err      error
}

func (e *RequestError) Error() string { return fmt.Sprintf("endpoint=%s: %v", e.Endpoint, e.Err) }
func (e *RequestError) Unwrap() error { return e.Err }

func ErrorCode(err error) string {
	var u *url.Error
	if errors.As(err, &u) {
		err = u.Err
	}
	if errors.Is(err, context.Canceled) {
		return "CANCELLED"
	}
	var n net.Error
	if errors.Is(err, context.DeadlineExceeded) || (errors.As(err, &n) && n.Timeout()) {
		return "HTTP_TIMEOUT"
	}
	var h *HTTPStatusError
	if errors.As(err, &h) {
		return fmt.Sprintf("HTTP_%d", h.StatusCode)
	}
	if errors.As(err, &n) || errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
		return "NETWORK"
	}
	return "IMPORTER_VALIDATION_FAILED"
}

func IsRetryable(err error) bool {
	code := ErrorCode(err)
	if code == "HTTP_TIMEOUT" || code == "NETWORK" {
		return true
	}
	var h *HTTPStatusError
	return errors.As(err, &h) && (h.StatusCode == 429 || h.StatusCode == 502 || h.StatusCode == 503 || h.StatusCode == 504 || h.StatusCode == 500)
}
