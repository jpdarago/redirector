package main

import (
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"
)

func loadRoutes(dir string) map[string]string {
	routes := make(map[string]string)
	filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		if filepath.Ext(path) != ".txt" {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		rel, _ := filepath.Rel(dir, path)
		key := "/" + strings.TrimSuffix(rel, ".txt")
		routes[key] = strings.TrimSpace(string(data))
		return nil
	})
	return routes
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

	go func() {
		for {
			time.Sleep(100 * time.Millisecond)
			m := loadRoutes(dir)
			routes.Store(&m)
		}
	}()

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /", func(w http.ResponseWriter, r *http.Request) {
		m := *routes.Load()
		target, ok := m[r.URL.Path]
		if !ok {
			http.NotFound(w, r)
			return
		}
		if !strings.Contains(target, "://") {
			target = "https://" + target
		}
		http.Redirect(w, r, target, http.StatusMovedPermanently)
	})

	addr := ":" + port
	log.Printf("listening on %s", addr)
	log.Fatal(http.ListenAndServe(addr, mux))
}
