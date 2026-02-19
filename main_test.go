package main

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	qrcode "github.com/skip2/go-qrcode"
)

var testNow = time.Date(2025, 1, 10, 12, 0, 0, 0, time.UTC)

func recentModTime() time.Time { return testNow.Add(-1 * time.Hour) }
func oldModTime() time.Time    { return testNow.Add(-7 * 24 * time.Hour) }

func fixedNow() time.Time { return testNow }

func setupTestDir(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for path, content := range files {
		full := filepath.Join(dir, path)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
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
		} else if got.Target != want {
			t.Errorf("route %s = %q, want %q", key, got.Target, want)
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
	if got := routes["/a"].Target; got != "google.com" {
		t.Errorf("got %q, want %q", got, "google.com")
	}
}

func makeRoutes(m map[string]string, modTime time.Time) map[string]routeEntry {
	entries := make(map[string]routeEntry, len(m))
	for k, v := range m {
		entries[k] = routeEntry{Target: v, ModTime: modTime}
	}
	return entries
}

func TestRedirectHandler(t *testing.T) {
	var routes atomic.Pointer[map[string]routeEntry]
	m := makeRoutes(map[string]string{
		"/a":   "google.com",
		"/b/c": "https://example.com/path",
	}, recentModTime())
	routes.Store(&m)

	handler := redirectHandler(&routes, fixedNow)

	tests := []struct {
		path       string
		wantCode   int
		wantTarget string
	}{
		{"/a", http.StatusPermanentRedirect, "https://google.com"},
		{"/b/c", http.StatusPermanentRedirect, "https://example.com/path"},
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
			cc := rec.Header().Get("Cache-Control")
			if cc != "max-age=86400" {
				t.Errorf("%s: Cache-Control = %q, want %q", tt.path, cc, "max-age=86400")
			}
		}
	}
}

func TestRedirectHandlerCacheImmutable(t *testing.T) {
	var routes atomic.Pointer[map[string]routeEntry]
	m := makeRoutes(map[string]string{
		"/a": "google.com",
	}, oldModTime())
	routes.Store(&m)

	handler := redirectHandler(&routes, fixedNow)
	req := httptest.NewRequest("GET", "/a", nil)
	rec := httptest.NewRecorder()
	handler(rec, req)

	cc := rec.Header().Get("Cache-Control")
	if cc != "max-age=31536000, immutable" {
		t.Errorf("Cache-Control = %q, want %q", cc, "max-age=31536000, immutable")
	}
}

func TestRedirectHandlerRejectsInvalidPaths(t *testing.T) {
	var routes atomic.Pointer[map[string]routeEntry]
	m := makeRoutes(map[string]string{}, recentModTime())
	routes.Store(&m)

	handler := redirectHandler(&routes, fixedNow)

	tests := []struct {
		name string
		path string
	}{
		{"dots", "/go/hello.world"},
		{"special chars", "/go/a@b"},
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
	var routes atomic.Pointer[map[string]routeEntry]
	m := makeRoutes(map[string]string{
		"/go/github":       "github.com",
		"/go/my-repo":      "github.com/my-repo",
		"/go/my_repo":      "github.com/my_repo",
		"/go/ABC-123_test": "example.com",
		"/a/b/c":           "example.com",
		"/go/todo":         "example.com/todo",
	}, recentModTime())
	routes.Store(&m)

	handler := redirectHandler(&routes, fixedNow)

	paths := make([]string, 0, len(m)+1)
	for p := range m {
		paths = append(paths, p)
	}
	// Also test a trailing-slash variant
	paths = append(paths, "/go/todo/")

	for _, path := range paths {
		t.Run(path, func(t *testing.T) {
			req := httptest.NewRequest("GET", path, nil)
			rec := httptest.NewRecorder()
			handler(rec, req)
			if rec.Code != http.StatusPermanentRedirect {
				t.Errorf("%s: status = %d, want %d", path, rec.Code, http.StatusPermanentRedirect)
			}
		})
	}
}

func TestListHandler(t *testing.T) {
	var routes atomic.Pointer[map[string]routeEntry]
	m := makeRoutes(map[string]string{
		"/b":   "google.com",
		"/a":   "https://example.com",
		"/c/d": "github.com/foo",
	}, recentModTime())
	routes.Store(&m)

	handler := listHandler(&routes, "")
	req := httptest.NewRequest("GET", "/", nil)
	rec := httptest.NewRecorder()
	handler(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	ct := rec.Header().Get("Content-Type")
	if !strings.Contains(ct, "text/html") {
		t.Errorf("Content-Type = %q, want text/html", ct)
	}
	body := rec.Body.String()
	// Check routes appear in sorted order
	aIdx := strings.Index(body, "/a")
	bIdx := strings.Index(body, "/b")
	cdIdx := strings.Index(body, "/c/d")
	if aIdx == -1 || bIdx == -1 || cdIdx == -1 {
		t.Fatalf("missing routes in body: %s", body)
	}
	if aIdx > bIdx || bIdx > cdIdx {
		t.Error("routes not sorted alphabetically")
	}
	// Check targets are present
	if !strings.Contains(body, "https://google.com") {
		t.Error("missing https://google.com in body")
	}
	if !strings.Contains(body, "https://example.com") {
		t.Error("missing https://example.com in body")
	}
	if !strings.Contains(body, "https://github.com/foo") {
		t.Error("missing https://github.com/foo in body")
	}
}

func TestListHandlerEmpty(t *testing.T) {
	var routes atomic.Pointer[map[string]routeEntry]
	m := makeRoutes(map[string]string{}, recentModTime())
	routes.Store(&m)

	handler := listHandler(&routes, "")
	req := httptest.NewRequest("GET", "/", nil)
	rec := httptest.NewRecorder()
	handler(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if !strings.Contains(rec.Body.String(), "<ul>\n</ul>") {
		t.Error("expected empty list")
	}
}

func TestListHandlerBasePath(t *testing.T) {
	var routes atomic.Pointer[map[string]routeEntry]
	m := makeRoutes(map[string]string{
		"/a": "google.com",
	}, recentModTime())
	routes.Store(&m)

	handler := listHandler(&routes, "/go")
	req := httptest.NewRequest("GET", "/", nil)
	rec := httptest.NewRecorder()
	handler(rec, req)

	body := rec.Body.String()
	if !strings.Contains(body, `href="/go/a"`) {
		t.Errorf("expected href with base path /go, got: %s", body)
	}
	if !strings.Contains(body, `>/go/a<`) {
		t.Errorf("expected link text with base path /go, got: %s", body)
	}
}

func TestLoadRoutesIndexFile(t *testing.T) {
	dir := setupTestDir(t, map[string]string{
		"todo/_index.txt":       "todoist.com",
		"work/tools/_index.txt": "tools.example.com",
		"a.txt":                 "google.com",
	})

	routes := loadRoutes(dir)

	tests := map[string]string{
		"/todo":       "todoist.com",
		"/work/tools": "tools.example.com",
		"/a":          "google.com",
	}
	for key, want := range tests {
		got, ok := routes[key]
		if !ok {
			t.Errorf("missing route %s", key)
		} else if got.Target != want {
			t.Errorf("route %s = %q, want %q", key, got.Target, want)
		}
	}
	if _, ok := routes["/todo/_index"]; ok {
		t.Error("_index.txt should not create a /_index route")
	}
	if len(routes) != len(tests) {
		t.Errorf("got %d routes, want %d", len(routes), len(tests))
	}
}

func TestLoadRoutesRootIndexSkipped(t *testing.T) {
	dir := setupTestDir(t, map[string]string{
		"_index.txt": "example.com",
		"a.txt":      "google.com",
	})

	routes := loadRoutes(dir)

	if _, ok := routes["/"]; ok {
		t.Error("root _index.txt should be skipped")
	}
	if len(routes) != 1 {
		t.Errorf("got %d routes, want 1", len(routes))
	}
}

func TestRedirectHandlerTrailingSlash(t *testing.T) {
	var routes atomic.Pointer[map[string]routeEntry]
	m := makeRoutes(map[string]string{
		"/a":   "google.com",
		"/b/c": "example.com",
	}, recentModTime())
	routes.Store(&m)

	handler := redirectHandler(&routes, fixedNow)

	tests := []struct {
		path       string
		wantCode   int
		wantTarget string
	}{
		{"/a/", http.StatusPermanentRedirect, "https://google.com"},
		{"/b/c/", http.StatusPermanentRedirect, "https://example.com"},
		{"/nope/", http.StatusNotFound, ""},
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

func TestRedirectHandlerPreservesScheme(t *testing.T) {
	var routes atomic.Pointer[map[string]routeEntry]
	m := makeRoutes(map[string]string{
		"/a": "http://insecure.com",
	}, recentModTime())
	routes.Store(&m)

	req := httptest.NewRequest("GET", "/a", nil)
	rec := httptest.NewRecorder()
	redirectHandler(&routes, fixedNow)(rec, req)

	got := rec.Header().Get("Location")
	if got != "http://insecure.com" {
		t.Errorf("Location = %q, want %q", got, "http://insecure.com")
	}
}

func TestRedirectHandlerQR(t *testing.T) {
	var routes atomic.Pointer[map[string]routeEntry]
	m := makeRoutes(map[string]string{
		"/a": "https://google.com",
	}, recentModTime())
	routes.Store(&m)

	req := httptest.NewRequest("GET", "/a?qr", nil)
	rec := httptest.NewRecorder()
	redirectHandler(&routes, fixedNow)(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	ct := rec.Header().Get("Content-Type")
	if ct != "image/png" {
		t.Errorf("Content-Type = %q, want %q", ct, "image/png")
	}
	pngMagic := []byte("\x89PNG")
	if !bytes.HasPrefix(rec.Body.Bytes(), pngMagic) {
		t.Error("body does not start with PNG magic bytes")
	}
}

func TestRedirectHandlerQRNotFound(t *testing.T) {
	var routes atomic.Pointer[map[string]routeEntry]
	m := makeRoutes(map[string]string{}, recentModTime())
	routes.Store(&m)

	req := httptest.NewRequest("GET", "/nope?qr", nil)
	rec := httptest.NewRecorder()
	redirectHandler(&routes, fixedNow)(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestRedirectHandlerQRPrependsScheme(t *testing.T) {
	var routes atomic.Pointer[map[string]routeEntry]
	m := makeRoutes(map[string]string{
		"/a": "google.com",
	}, recentModTime())
	routes.Store(&m)

	req := httptest.NewRequest("GET", "/a?qr", nil)
	rec := httptest.NewRecorder()
	redirectHandler(&routes, fixedNow)(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	// Generate expected QR code for the https-prefixed URL and compare
	expected, err := qrcode.Encode("https://google.com", qrcode.Medium, 256)
	if err != nil {
		t.Fatalf("failed to generate expected QR code: %v", err)
	}
	if !bytes.Equal(rec.Body.Bytes(), expected) {
		t.Error("QR code does not match expected output for https://google.com")
	}
}
