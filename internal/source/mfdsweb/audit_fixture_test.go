package mfdsweb

import (
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"unicode"
)

func TestAuditFixture_원본크기와Hash를검증한뒤목록을해석한다(t *testing.T) {
	tests := []struct {
		name       string
		file       string
		size       int
		sha256     string
		request    ListRequest
		wantTotal  int
		wantPages  int
		wantRows   int
		wantErr    ErrorKind
		wantFirst  string
		wantNoWarn bool
	}{
		{
			name:      "page1",
			file:      "audit-order-page1.html.gz.b64",
			size:      631646,
			sha256:    "ddd5d858b706a83da3074469415ded66748dc40b2145bfbcb15ee1dd631cafa9",
			request:   auditRequest("위스키", "C0314210000000000000", "2026-07-27", 1, 10),
			wantTotal: 20,
			wantPages: 2,
			wantRows:  10,
		},
		{
			name:       "no data",
			file:       "audit-retention-empty.html.gz.b64",
			size:       614644,
			sha256:     "9824cf4a47b27c2176894b90f1299a6a5dbc36f53e6de6e88738f02d0d183e27",
			request:    auditRequest("브랜디", "C0314220000000000000", "2025-07-28", 1, 1),
			wantTotal:  0,
			wantPages:  0,
			wantRows:   0,
			wantNoWarn: true,
		},
		{
			name:    "generic error",
			file:    "audit-invalid-date-error.html.gz.b64",
			size:    5153,
			sha256:  "26aed11b0cf519f613952316b648cae15cd38d460c7dff539f87d43479a2397c",
			request: auditRequest("위스키", "C0314210000000000000", "2026-07-27", 1, 10),
			wantErr: ErrorKindParse,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			body := decodeAuditFixture(t, test.file, test.size, test.sha256)

			page, err := ParseList(body, test.request)
			if errorKind(err) != test.wantErr {
				t.Fatalf("ParseList() error = %v, want kind %q", err, test.wantErr)
			}
			if test.wantErr != "" {
				return
			}
			if page.Total != test.wantTotal || page.TotalPages != test.wantPages || len(page.Rows) != test.wantRows {
				t.Fatalf("page = %+v", page)
			}
			if test.wantNoWarn && len(page.Warnings) != 0 {
				t.Fatalf("warnings = %v", page.Warnings)
			}
		})
	}
}

func decodeAuditFixture(t *testing.T, name string, wantSize int, wantSHA256 string) []byte {
	t.Helper()

	encoded, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	compact := strings.Map(func(r rune) rune {
		if unicode.IsSpace(r) {
			return -1
		}
		return r
	}, string(encoded))
	compressed, err := base64.StdEncoding.DecodeString(compact)
	if err != nil {
		t.Fatalf("base64 decode: %v", err)
	}
	reader, err := gzip.NewReader(bytes.NewReader(compressed))
	if err != nil {
		t.Fatalf("gzip reader: %v", err)
	}
	body, readErr := io.ReadAll(reader)
	closeErr := reader.Close()
	if readErr != nil {
		t.Fatalf("gzip read: %v", readErr)
	}
	if closeErr != nil {
		t.Fatalf("gzip close: %v", closeErr)
	}
	if len(body) != wantSize {
		t.Fatalf("decoded size = %d, want %d", len(body), wantSize)
	}
	if got := fmt.Sprintf("%x", sha256.Sum256(body)); got != wantSHA256 {
		t.Fatalf("decoded sha256 = %s, want %s", got, wantSHA256)
	}
	return body
}

func auditRequest(name, code, date string, page, limit int) ListRequest {
	processDate, err := time.Parse(time.DateOnly, date)
	if err != nil {
		panic(err)
	}
	return ListRequest{
		Item:        Item{Name: name, Code: code},
		ProcessDate: processDate,
		Page:        page,
		Limit:       limit,
	}
}
