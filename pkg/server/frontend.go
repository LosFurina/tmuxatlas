package server

import (
	"errors"
	"io/fs"
	"net/http"
	"path"
	"strings"
)

func isSPARoute(requestPath string) bool {
	cleaned := path.Clean("/" + strings.TrimPrefix(requestPath, "/"))
	switch cleaned {
	case "/", "/settings", "/setup":
		return true
	}
	if strings.HasPrefix(cleaned, "/session/") {
		return true
	}

	// Preserve the existing client-side fallback for future extensionless routes
	// while ensuring missing file-like paths are observable as 404 responses.
	return path.Ext(cleaned) == ""
}

func serveFrontendPath(fileServer http.Handler, target string, w http.ResponseWriter, r *http.Request) {
	request := r.Clone(r.Context())
	urlCopy := *r.URL
	request.URL = &urlCopy
	request.URL.Path = target
	fileServer.ServeHTTP(w, request)
}

func serveFrontendDocument(fileServer http.Handler, target string, w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-cache")
	serveFrontendPath(fileServer, target, w, r)
}

func frontendHandler(frontend fs.FS) http.Handler {
	fileServer := http.FileServer(http.FS(frontend))

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestPath := path.Clean("/" + strings.TrimPrefix(r.URL.Path, "/"))
		resourcePath := strings.TrimPrefix(requestPath, "/")

		if resourcePath != "" {
			file, err := frontend.Open(resourcePath)
			if err == nil {
				file.Close()
				switch resourcePath {
				case "manifest.json":
					w.Header().Set("Content-Type", "application/manifest+json")
					w.Header().Set("Cache-Control", "no-cache")
				case "sw.js":
					w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
					w.Header().Set("Cache-Control", "no-cache")
					w.Header().Set("Service-Worker-Allowed", "/")
				case "index.html":
					w.Header().Set("Cache-Control", "no-cache")
				}
				serveFrontendPath(fileServer, requestPath, w, r)
				return
			}
			if !errors.Is(err, fs.ErrNotExist) {
				http.Error(w, "could not read frontend resource", http.StatusInternalServerError)
				return
			}
		}

		if !isSPARoute(requestPath) {
			http.NotFound(w, r)
			return
		}
		serveFrontendDocument(fileServer, "/", w, r)
	})
}
