package adminweb

import (
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
)

// Handler serves the admin SPA without rewriting API or missing asset requests.
func Handler(root string) http.Handler {
	const prefix = "/suxinweb"
	files := http.StripPrefix(prefix, http.FileServer(http.Dir(root)))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != prefix && !strings.HasPrefix(r.URL.Path, prefix+"/") {
			http.NotFound(w, r)
			return
		}
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.Header().Set("Allow", "GET, HEAD")
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if r.URL.Path == prefix {
			target := prefix + "/"
			if r.URL.RawQuery != "" {
				target += "?" + r.URL.RawQuery
			}
			http.Redirect(w, r, target, http.StatusPermanentRedirect)
			return
		}
		rel := strings.TrimPrefix(r.URL.Path, prefix)
		f, err := http.Dir(root).Open(rel)
		if err == nil {
			info, statErr := f.Stat()
			f.Close()
			if statErr == nil && !info.IsDir() {
				if rel == "/index.html" || rel == "/config.js" {
					w.Header().Set("Cache-Control", "no-cache")
				}
				files.ServeHTTP(w, r)
				return
			}
		}
		accept := r.Header.Get("Accept")
		navigation := accept == "" || strings.Contains(accept, "text/html") || strings.Contains(accept, "*/*")
		// Browser navigation may fall back to the SPA, but asset errors must stay 404.
		if path.Ext(rel) != "" || strings.HasPrefix(rel, "/assets/") ||
			strings.HasPrefix(rel, "/iconfont/") || !navigation {
			http.NotFound(w, r)
			return
		}
		index, err := os.Open(filepath.Join(root, "index.html"))
		if err != nil {
			http.NotFound(w, r)
			return
		}
		defer index.Close()
		info, err := index.Stat()
		if err != nil {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Cache-Control", "no-cache")
		http.ServeContent(w, r, "index.html", info.ModTime(), index)
	})
}
