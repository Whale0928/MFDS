package mfdsweb

import (
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/json"
	"io"
	"net/http"
	"testing"
)

func TestCaptureBody_동일본문_결정적Gzip과Hash를반환한다(t *testing.T) {
	source := []byte("<html>MFDS 원문</html>")

	body1, compressed1, hash1, err := captureBody(bytes.NewReader(source), int64(len(source)))
	if err != nil {
		t.Fatalf("captureBody() error = %v", err)
	}
	body2, compressed2, hash2, err := captureBody(bytes.NewReader(source), int64(len(source)))
	if err != nil {
		t.Fatalf("captureBody() second error = %v", err)
	}

	if !bytes.Equal(body1, source) || !bytes.Equal(body2, source) {
		t.Fatal("captured body differs from source")
	}
	if !bytes.Equal(compressed1, compressed2) {
		t.Fatal("gzip output is not deterministic")
	}
	if hash1 != sha256.Sum256(source) || hash2 != hash1 {
		t.Fatalf("body hash mismatch: %x / %x", hash1, hash2)
	}

	reader, err := gzip.NewReader(bytes.NewReader(compressed1))
	if err != nil {
		t.Fatalf("gzip.NewReader() error = %v", err)
	}
	decompressed, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("io.ReadAll() error = %v", err)
	}
	if err := reader.Close(); err != nil {
		t.Fatalf("gzip reader close error = %v", err)
	}
	if !bytes.Equal(decompressed, source) {
		t.Fatal("gzip round-trip differs from source")
	}
}

func TestCaptureBody_허용크기초과_오류를반환한다(t *testing.T) {
	_, _, _, err := captureBody(bytes.NewReader([]byte("12345")), 4)
	if !errorsIsBodyLimit(err) {
		t.Fatalf("captureBody() error = %v, want body limit", err)
	}
}

func TestSanitizeResponseHeaders_허용목록만보존한다(t *testing.T) {
	header := make(http.Header)
	header.Set("Content-Type", "text/html; charset=UTF-8")
	header.Set("ETag", "fixture-etag")
	header.Set("Set-Cookie", "SESSION=secret")
	header.Set("Authorization", "Bearer secret")
	header.Set("X-Trace-Token", "secret-token")

	raw, err := sanitizeResponseHeaders(header)
	if err != nil {
		t.Fatalf("sanitizeResponseHeaders() error = %v", err)
	}
	var snapshot map[string]string
	if err := json.Unmarshal(raw, &snapshot); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if len(snapshot) != 2 ||
		snapshot["Content-Type"] != "text/html; charset=UTF-8" ||
		snapshot["ETag"] != "fixture-etag" {
		t.Fatalf("snapshot = %#v", snapshot)
	}
}
