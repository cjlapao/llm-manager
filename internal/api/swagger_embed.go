package api

import (
	"embed"
	"io/fs"
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
	sub, err := fs.Sub(swaggerUIFS, "swagger-ui")
	if err != nil {
		// This should never happen since we embed the directory above.
		panic("swagger-ui: failed to create sub-FS: " + err.Error())
	}

	fileServer := http.FileServer(http.FS(sub))

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Strip the /swagger/ prefix to get the path within swagger-ui/
		path := strings.TrimPrefix(r.URL.Path, "/swagger/")

		// Serve index.html for the root of /swagger/
		if path == "" || path == "/" {
			path = "index.html"
		}

		// Set correct Content-Type based on file extension
		ext := strings.ToLower(path[strings.LastIndex(path, "."):])
		if ct, ok := swaggerMIMETypes[ext]; ok {
			w.Header().Set("Content-Type", ct)
		}

		// Add cache headers for static assets
		if ext != ".html" {
			w.Header().Set("Cache-Control", "public, max-age=3600")
		}

		fileServer.ServeHTTP(w, r)
	})
}
