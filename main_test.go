package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
)

func setupTestDir(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for path, content := range files {
		full := filepath.Join(dir, path)
		os.MkdirAll(filepath.Dir(full), 0o755)
		os.WriteFile(full, []byte(content), 0o644)
	}
	return dir
}

func TestLoadRoutes(t *testing.T) {
	dir := setupTestDir(t, map[string]string{
		"a.txt":     "google.com",
		"b/c.txt":   "amazon.com",
		"e/f/g.txt": "youtube.com",
		"skip.md":   "should be ignored",
	})

	routes := loadRoutes(dir)

	tests := map[string]string{
		"/a":     "google.com",
		"/b/c":   "amazon.com",
		"/e/f/g": "youtube.com",
	}
	for key, want := range tests {
		got, ok := routes[key]
		if !ok {
			t.Errorf("missing route %s", key)
		} else if got != want {
			t.Errorf("route %s = %q, want %q", key, got, want)
		}
	}
	if _, ok := routes["/skip"]; ok {
		t.Error("non-txt file should be ignored")
	}
	if len(routes) != len(tests) {
		t.Errorf("got %d routes, want %d", len(routes), len(tests))
	}
}

func TestLoadRoutesEmptyDir(t *testing.T) {
	dir := t.TempDir()
	routes := loadRoutes(dir)
	if len(routes) != 0 {
		t.Errorf("got %d routes, want 0", len(routes))
	}
}

func TestLoadRoutesTrimsWhitespace(t *testing.T) {
	dir := setupTestDir(t, map[string]string{
		"a.txt": "  google.com  \n",
	})
	routes := loadRoutes(dir)
	if got := routes["/a"]; got != "google.com" {
		t.Errorf("got %q, want %q", got, "google.com")
	}
}

func TestRedirectHandler(t *testing.T) {
	var routes atomic.Pointer[map[string]string]
	m := map[string]string{
		"/a":   "google.com",
		"/b/c": "https://example.com/path",
	}
	routes.Store(&m)

	handler := redirectHandler(&routes)

	tests := []struct {
		path       string
		wantCode   int
		wantTarget string
	}{
		{"/a", http.StatusMovedPermanently, "https://google.com"},
		{"/b/c", http.StatusMovedPermanently, "https://example.com/path"},
		{"/nope", http.StatusNotFound, ""},
	}

	for _, tt := range tests {
		req := httptest.NewRequest("GET", tt.path, nil)
		rec := httptest.NewRecorder()
		handler(rec, req)

		if rec.Code != tt.wantCode {
			t.Errorf("%s: status = %d, want %d", tt.path, rec.Code, tt.wantCode)
		}
		if tt.wantTarget != "" {
			got := rec.Header().Get("Location")
			if got != tt.wantTarget {
				t.Errorf("%s: Location = %q, want %q", tt.path, got, tt.wantTarget)
			}
		}
	}
}

func TestRedirectHandlerRejectsInvalidPaths(t *testing.T) {
	var routes atomic.Pointer[map[string]string]
	m := map[string]string{}
	routes.Store(&m)

	handler := redirectHandler(&routes)

	tests := []struct {
		name string
		path string
	}{
		{"dots", "/go/hello.world"},
		{"special chars", "/go/a@b"},
		{"trailing slash", "/go/a/"},
		{"bare slash", "/"},
		{"too long", "/" + strings.Repeat("a", 64)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/valid", nil)
			req.URL.Path = tt.path
			rec := httptest.NewRecorder()
			handler(rec, req)
			if rec.Code != http.StatusBadRequest {
				t.Errorf("%s: status = %d, want %d", tt.path, rec.Code, http.StatusBadRequest)
			}
		})
	}
}

func TestRedirectHandlerAcceptsValidPaths(t *testing.T) {
	var routes atomic.Pointer[map[string]string]
	m := map[string]string{
		"/go/github":       "github.com",
		"/go/my-repo":      "github.com/my-repo",
		"/go/my_repo":      "github.com/my_repo",
		"/go/ABC-123_test": "example.com",
		"/a/b/c":           "example.com",
	}
	routes.Store(&m)

	handler := redirectHandler(&routes)

	for path := range m {
		t.Run(path, func(t *testing.T) {
			req := httptest.NewRequest("GET", path, nil)
			rec := httptest.NewRecorder()
			handler(rec, req)
			if rec.Code != http.StatusMovedPermanently {
				t.Errorf("%s: status = %d, want %d", path, rec.Code, http.StatusMovedPermanently)
			}
		})
	}
}

func TestRedirectHandlerPreservesScheme(t *testing.T) {
	var routes atomic.Pointer[map[string]string]
	m := map[string]string{
		"/a": "http://insecure.com",
	}
	routes.Store(&m)

	req := httptest.NewRequest("GET", "/a", nil)
	rec := httptest.NewRecorder()
	redirectHandler(&routes)(rec, req)

	got := rec.Header().Get("Location")
	if got != "http://insecure.com" {
		t.Errorf("Location = %q, want %q", got, "http://insecure.com")
	}
}
