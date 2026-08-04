package mfdsweb

import (
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"
)

var retainedResponseHeaders = []string{
	"Content-Type",
	"Content-Length",
	"Date",
	"ETag",
	"Last-Modified",
}

func captureBody(reader io.Reader, maximum int64) ([]byte, []byte, [32]byte, error) {
	body, err := io.ReadAll(io.LimitReader(reader, maximum+1))
	if err != nil {
		return nil, nil, [32]byte{}, err
	}
	if int64(len(body)) > maximum {
		return nil, nil, [32]byte{}, ErrBodyTooLarge
	}

	bodyHash := sha256.Sum256(body)
	var compressed bytes.Buffer
	writer, err := gzip.NewWriterLevel(&compressed, gzip.DefaultCompression)
	if err != nil {
		return nil, nil, [32]byte{}, fmt.Errorf("gzip writer 생성 실패: %w", err)
	}
	writer.ModTime = time.Unix(0, 0)
	writer.Name = ""
	writer.Comment = ""
	writer.OS = 255
	if _, err := writer.Write(body); err != nil {
		return nil, nil, [32]byte{}, fmt.Errorf("gzip 압축 실패: %w", err)
	}
	if err := writer.Close(); err != nil {
		return nil, nil, [32]byte{}, fmt.Errorf("gzip 종료 실패: %w", err)
	}
	return body, compressed.Bytes(), bodyHash, nil
}

func sanitizeResponseHeaders(header http.Header) ([]byte, error) {
	snapshot := make(map[string]string, len(retainedResponseHeaders))
	for _, name := range retainedResponseHeaders {
		if value := header.Get(name); value != "" {
			snapshot[name] = value
		}
	}
	return json.Marshal(snapshot)
}

func errorsIsBodyLimit(err error) bool {
	return errors.Is(err, ErrBodyTooLarge)
}
