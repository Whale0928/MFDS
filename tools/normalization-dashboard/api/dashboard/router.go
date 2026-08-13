// Package dashboard serves an intentionally read-only MFDS normalization view.
package dashboard

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
)

const (
	defaultPageSize = 50
	maxPageSize     = 100
)

// RowIterator is the minimal read-only boundary used by handlers and tests.
type RowIterator interface {
	Close() error
	Err() error
	Next() bool
	Scan(dest ...any) error
}

// Queryer intentionally has no execution or transaction method.
type Queryer interface {
	QueryContext(context.Context, string, ...any) (RowIterator, error)
}

// SQLQueryer is the production adapter. Every database access uses QueryContext.
type SQLQueryer struct{ DB *sql.DB }

func (q SQLQueryer) QueryContext(ctx context.Context, query string, args ...any) (RowIterator, error) {
	return q.DB.QueryContext(ctx, query, args...)
}

type Server struct{ queryer Queryer }

func NewServer(queryer Queryer) *Server { return &Server{queryer: queryer} }

func (s *Server) ListenAndServe(address string) error {
	if address != "127.0.0.1:8787" {
		return fmt.Errorf("dashboard API must bind to 127.0.0.1:8787")
	}
	return http.ListenAndServe(address, s.Handler())
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", s.health)
	mux.HandleFunc("GET /api/overview", s.overview)
	mux.HandleFunc("GET /api/filters", s.filters)
	mux.HandleFunc("GET /api/declarations", s.declarations)
	mux.HandleFunc("GET /api/declarations/{rcno}", s.declaration)
	mux.HandleFunc("GET /api/importers", s.importers)
	mux.HandleFunc("GET /api/importers/detail", s.importerDetail)
	mux.HandleFunc("GET /api/importers/products", s.importerProducts)
	mux.HandleFunc("GET /api/importers/product-declarations", s.importerProductDeclarations)
	mux.HandleFunc("GET /api/quality", s.quality)
	return cors(mux)
}

func (s *Server) health(w http.ResponseWriter, r *http.Request) {
	rows, err := s.queryer.QueryContext(r.Context(), "SELECT 1")
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, "database is unavailable")
		return
	}
	defer rows.Close()
	if !rows.Next() || rows.Err() != nil {
		writeError(w, http.StatusServiceUnavailable, "database is unavailable")
		return
	}
	var value int
	if err := rows.Scan(&value); err != nil || value != 1 {
		writeError(w, http.StatusServiceUnavailable, "database is unavailable")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func cors(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if origin != "" && allowedOrigin(origin) {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Vary", "Origin")
			w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		}
		if r.Method == http.MethodOptions {
			if origin != "" && !allowedOrigin(origin) {
				writeError(w, http.StatusForbidden, "origin is not allowed")
				return
			}
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func allowedOrigin(origin string) bool {
	parsed, err := url.Parse(origin)
	return err == nil && parsed.Scheme == "http" && (parsed.Hostname() == "127.0.0.1" || parsed.Hostname() == "localhost")
}
func writeDatabaseError(w http.ResponseWriter, err error) {
	log.Printf("dashboard database query failed: %v", err)
	writeError(w, http.StatusInternalServerError, "database query failed")
}
func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}
func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
