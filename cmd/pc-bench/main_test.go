package main

import (
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestHTTPIsReadOnlyAndLoopback(t *testing.T) {
	d := t.TempDir()
	if err := os.WriteFile(filepath.Join(d, "rapport_x.txt"), []byte("========== CPU SYSBENCH ==========\n--- CPU 1 threads ---\nevents per second:  100\n"), 0600); err != nil {
		t.Fatal(err)
	}
	mux := newHandler(d)
	for _, tt := range []struct {
		method, path string
		status       int
	}{
		{"GET", "/api/runs", 200},
		{"GET", "/api/compare?before=../../etc/passwd&after=x", 400},
		{"GET", "/api/compare?before=bad&after=bad", 404},
		{"POST", "/api/runs", 405},
		{"GET", "/", 200},
		{"GET", "/app.js", 200},
	} {
		req := httptest.NewRequest(tt.method, tt.path, nil)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if rec.Code != tt.status {
			t.Errorf("%s %s got %d want %d", tt.method, tt.path, rec.Code, tt.status)
		}
		if rec.Header().Get("Content-Security-Policy") == "" {
			t.Error("CSP absent")
		}
		if strings.Contains(rec.Body.String(), "sensitive") {
			t.Error("data leaked")
		}
	}
}

func TestNonLocalBindRejected(t *testing.T) {
	d := t.TempDir()
	err := run([]string{"serve", "--reports", d, "--listen", "0.0.0.0:8765"})
	if err == nil || !strings.Contains(err.Error(), "loopback") {
		t.Fatalf("bind not rejected: %v", err)
	}
}
