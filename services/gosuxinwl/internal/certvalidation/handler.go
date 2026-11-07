// Package certvalidation exposes the single public domain ownership proof.
package certvalidation

import (
	"net/http"
	"os"
	"path/filepath"
	"strconv"
)

const Path = "/.well-known/pki-validation/fileauth.txt"

// Handler never serves a directory or other files from the project root.
// Read on each request so a renewed challenge does not require a restart.
func Handler(root string) http.Handler {
	filename := filepath.Join(root, ".well-known", "pki-validation", "fileauth.txt")
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != Path {
			http.NotFound(w, r)
			return
		}
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.Header().Set("Allow", "GET, HEAD")
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		content, err := os.ReadFile(filename)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Header().Set("Content-Length", strconv.Itoa(len(content)))
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.WriteHeader(http.StatusOK)
		if r.Method == http.MethodGet {
			_, _ = w.Write(content)
		}
	})
}
