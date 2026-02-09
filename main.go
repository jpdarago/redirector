package main

import (
	"log"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync/atomic"
	"time"
)

var validPath = regexp.MustCompile(`^/[a-zA-Z0-9_-]+(/[a-zA-Z0-9_-]+)*$`)

func loadRoutes(dir string) map[string]string {
	routes := make(map[string]string)
	filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
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
		data, err := os.ReadFile(path)
		if err != nil {
			log.Printf("read error: %s: %v", path, err)
			return nil
		}
		rel, _ := filepath.Rel(dir, path)
		key := "/" + strings.TrimSuffix(rel, ".txt")
		routes[key] = strings.TrimSpace(string(data))
		return nil
	})
	return routes
}

func logRoutes(routes map[string]string) {
	for key, target := range routes {
		log.Printf("  %s -> %s", key, target)
	}
}

func redirectHandler(routes *atomic.Pointer[map[string]string]) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if len(r.URL.Path) > 64 || !validPath.MatchString(r.URL.Path) {
			log.Printf("%s %s -> 400 invalid path", r.Method, r.URL.Path)
			http.Error(w, "invalid path", http.StatusBadRequest)
			return
		}
		m := *routes.Load()
		target, ok := m[r.URL.Path]
		if !ok {
			log.Printf("%s %s -> 404", r.Method, r.URL.Path)
			http.NotFound(w, r)
			return
		}
		if !strings.Contains(target, "://") {
			target = "https://" + target
		}
		log.Printf("%s %s -> 301 %s", r.Method, r.URL.Path, target)
		http.Redirect(w, r, target, http.StatusMovedPermanently)
	}
}

func main() {
	dir := os.Getenv("REDIRECT_DIR")
	if dir == "" {
		log.Fatal("REDIRECT_DIR is required")
	}

	info, err := os.Stat(dir)
	if err != nil || !info.IsDir() {
		log.Fatalf("REDIRECT_DIR %q is not a valid directory", dir)
	}

	var routes atomic.Pointer[map[string]string]
	initial := loadRoutes(dir)
	routes.Store(&initial)
	log.Printf("loaded %d routes from %s", len(initial), dir)
	logRoutes(initial)

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

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /", redirectHandler(&routes))

	addr := ":" + port
	log.Printf("listening on %s", addr)
	log.Fatal(http.ListenAndServe(addr, mux))
}
