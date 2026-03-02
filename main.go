package main

import (
	"bytes"
	"flag"
	"html/template"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync/atomic"
	"time"

	qrcode "github.com/skip2/go-qrcode"
	"github.com/yuin/goldmark"
)

type routeEntry struct {
	Target  string
	Comment string
	ModTime time.Time
}

type route struct {
	Path    string
	Href    string
	Comment template.HTML
}

const listHTML = `<!DOCTYPE html>
<html>
<head><title>Redirects</title></head>
<body>
<h1>Available Redirects</h1>
<ul>{{range .}}
<li><a href="{{.Path}}">{{.Path}}</a> &rarr; {{.Href}}{{if .Comment}}<br>{{.Comment}}{{end}}</li>{{end}}
</ul>
</body>
</html>`

var listTmpl = template.Must(template.New("list").Parse(listHTML))

var validPath = regexp.MustCompile(`^/[a-zA-Z0-9_-]+(/[a-zA-Z0-9_-]+)*/?$`)

// parseRouteFile splits a route file into target URL and comment.
// Lines starting with # are comments; the first non-comment non-empty line is the target.
func parseRouteFile(content string) (target, comment string) {
	var commentLines []string
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "#") {
			commentLines = append(commentLines, strings.TrimSpace(strings.TrimPrefix(line, "#")))
		} else if line != "" && target == "" {
			target = line
		}
	}
	comment = strings.Join(commentLines, "\n")
	return target, comment
}

func renderMarkdown(src string) template.HTML {
	var buf bytes.Buffer
	if err := goldmark.Convert([]byte(src), &buf); err != nil {
		log.Printf("markdown error: %v", err)
		return template.HTML(template.HTMLEscapeString(src))
	}
	return template.HTML(buf.String())
}

func loadRoutes(dir string) map[string]routeEntry {
	routes := make(map[string]routeEntry)
	_ = filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			log.Printf("walk error: %s: %v", path, err)
			return nil
		}
		if d.IsDir() {
			return nil
		}
		if filepath.Ext(path) != ".txt" {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			log.Printf("stat error: %s: %v", path, err)
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			log.Printf("read error: %s: %v", path, err)
			return nil
		}
		rel, _ := filepath.Rel(dir, path)
		name := strings.TrimSuffix(rel, ".txt")
		if filepath.Base(name) == "_index" {
			parent := filepath.Dir(name)
			if parent == "." {
				// Root _index.txt would map to "/", skip it since that's the listing page
				return nil
			}
			name = parent
		}
		target, comment := parseRouteFile(string(data))
		key := "/" + name
		routes[key] = routeEntry{
			Target:  target,
			Comment: comment,
			ModTime: info.ModTime(),
		}
		return nil
	})
	return routes
}

func logRoutes(routes map[string]routeEntry) {
	for key, entry := range routes {
		log.Printf("  %s -> %s", key, entry.Target)
	}
}

func listHandler(routes *atomic.Pointer[map[string]routeEntry], basePath string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		m := *routes.Load()
		keys := make([]string, 0, len(m))
		for k := range m {
			keys = append(keys, k)
		}
		sort.Strings(keys)

		data := make([]route, 0, len(keys))
		for _, k := range keys {
			entry := m[k]
			href := entry.Target
			if !strings.Contains(href, "://") {
				href = "https://" + href
			}
			var comment template.HTML
			if entry.Comment != "" {
				comment = renderMarkdown(entry.Comment)
			}
			data = append(data, route{Path: basePath + k, Href: href, Comment: comment})
		}

		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if err := listTmpl.Execute(w, data); err != nil {
			log.Printf("template error: %v", err)
		}
	}
}

func redirectHandler(routes *atomic.Pointer[map[string]routeEntry], now func() time.Time) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if len(r.URL.Path) > 64 || !validPath.MatchString(r.URL.Path) {
			log.Printf("%s %s -> 400 invalid path", r.Method, r.URL.Path)
			http.Error(w, "invalid path", http.StatusBadRequest)
			return
		}
		m := *routes.Load()
		lookupPath := strings.TrimRight(r.URL.Path, "/")
		entry, ok := m[lookupPath]
		if !ok {
			log.Printf("%s %s -> 404", r.Method, r.URL.Path)
			http.NotFound(w, r)
			return
		}
		target := entry.Target
		if !strings.Contains(target, "://") {
			target = "https://" + target
		}
		if r.URL.Query().Has("qr") {
			png, err := qrcode.Encode(target, qrcode.Medium, 256)
			if err != nil {
				log.Printf("%s %s?qr -> 500: %v", r.Method, r.URL.Path, err)
				http.Error(w, "failed to generate QR code", http.StatusInternalServerError)
				return
			}
			w.Header().Set("Content-Type", "image/png")
			_, _ = w.Write(png)
			return
		}
		log.Printf("%s %s -> 308 %s", r.Method, r.URL.Path, target)
		if now().Sub(entry.ModTime) > 3*24*time.Hour {
			w.Header().Set("Cache-Control", "max-age=31536000, immutable")
		} else {
			w.Header().Set("Cache-Control", "max-age=86400")
		}
		http.Redirect(w, r, target, http.StatusPermanentRedirect)
	}
}

func main() {
	check := flag.Bool("check", false, "verify configuration and exit")
	flag.Parse()

	dir := os.Getenv("REDIRECT_DIR")
	if dir == "" {
		log.Fatal("REDIRECT_DIR is required")
	}

	info, err := os.Stat(dir)
	if err != nil || !info.IsDir() {
		log.Fatalf("REDIRECT_DIR %q is not a valid directory", dir)
	}

	var routes atomic.Pointer[map[string]routeEntry]
	initial := loadRoutes(dir)
	routes.Store(&initial)
	log.Printf("loaded %d routes from %s", len(initial), dir)
	logRoutes(initial)

	if *check {
		log.Print("check passed")
		return
	}

	go func() {
		prev := len(initial)
		for {
			time.Sleep(100 * time.Millisecond)
			m := loadRoutes(dir)
			routes.Store(&m)
			if len(m) != prev {
				log.Printf("reload: %d routes (was %d)", len(m), prev)
				logRoutes(m)
				prev = len(m)
			}
		}
	}()

	basePath := os.Getenv("BASE_PATH")

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", listHandler(&routes, basePath))
	mux.HandleFunc("GET /", redirectHandler(&routes, time.Now))

	addr := ":" + port
	log.Printf("listening on %s", addr)
	log.Fatal(http.ListenAndServe(addr, mux))
}
