package api

import (
	"embed"
	"io"
	"net/http"
	"strings"
)

//go:embed swagger-ui
var swaggerUIFS embed.FS

// swaggerMIMETypes maps file extensions to their MIME types for the Swagger UI assets.
var swaggerMIMETypes = map[string]string{
	".html": "text/html; charset=utf-8",
	".css":  "text/css; charset=utf-8",
	".js":   "application/javascript; charset=utf-8",
	".png":  "image/png",
	".json": "application/json",
}

// SwaggerUIHandler returns an http.Handler that serves the embedded Swagger UI
// static files at the /swagger/ path prefix. Files are served with correct MIME
// types and proper caching headers.
func SwaggerUIHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Strip the /swagger/ prefix to get the path within swagger-ui/
		path := strings.TrimPrefix(r.URL.Path, "/swagger/")

		// Serve index.html for the root of /swagger/
		if path == "" || path == "/" {
			path = "index.html"
		}

		// Set correct Content-Type based on file extension
		if idx := strings.LastIndex(path, "."); idx >= 0 {
			ext := strings.ToLower(path[idx:])
			if ct, ok := swaggerMIMETypes[ext]; ok {
				w.Header().Set("Content-Type", ct)
			}
			if ext != ".html" {
				w.Header().Set("Cache-Control", "public, max-age=3600")
			}
		}

		// Open the file directly from the embedded FS
		f, err := swaggerUIFS.Open(path)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		defer f.Close()

		info, err := f.Stat()
		if err != nil {
			http.Error(w, "Failed to stat file", http.StatusInternalServerError)
			return
		}

		if info.IsDir() {
			http.NotFound(w, r)
			return
		}

		// Read the full file content and serve it
		data, err := io.ReadAll(f)
		if err != nil {
			http.Error(w, "Failed to read file", http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusOK)
		w.Write(data)
	})
}
