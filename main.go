package main

import (
	"bytes"
	"context"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

var (
	port       = flag.String("p", "3000", "The port to listen for HTTP traffic on")
	dir        = flag.String("d", "./", "The directory where to serve files from")
	spec       = flag.String("spec", "", "Path to an OpenAPI spec file (JSON or YAML)")
	specUIPath = flag.String("spec-ui-path", "/docs", "URL path to serve Swagger UI at")
	noListing  = flag.Bool("no-listing", false, "Disable directory listing")
)

func logger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Powered-By", "NerdyGeek")

		log.Printf("%s %s %s%s", r.Method, r.Proto, r.Host, r.URL.Path)

		if r.Body != nil && (r.Method == http.MethodPost || r.Method == http.MethodPut || r.Method == http.MethodPatch) {
			body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
			if err != nil {
				log.Printf("Error reading body: %v", err)
			}
			if len(body) > 0 {
				log.Printf("Body (%d bytes): %s", len(body), body)
			}
			r.Body = io.NopCloser(bytes.NewReader(body))
		}

		next.ServeHTTP(w, r)
	})
}

type noListDir struct {
	http.Dir
}

func (d noListDir) Open(name string) (http.File, error) {
	f, err := d.Dir.Open(name)
	if err != nil {
		return nil, err
	}
	stat, err := f.Stat()
	if err != nil {
		f.Close()
		return nil, err
	}
	if stat.IsDir() {
		indexFile, err := d.Dir.Open(filepath.Join(name, "index.html"))
		if err == nil {
			indexFile.Close()
			f.Close()
			return d.Dir.Open(filepath.Join(name, "index.html"))
		}
		f.Close()
		return nil, os.ErrNotExist
	}
	return f, nil
}

func swaggerUI(specURL string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		html := fmt.Sprintf(`<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8">
  <title>API Docs</title>
  <link rel="stylesheet" href="https://unpkg.com/swagger-ui-dist@5/swagger-ui.css">
</head>
<body>
  <div id="swagger-ui"></div>
  <script src="https://unpkg.com/swagger-ui-dist@5/swagger-ui-bundle.js"></script>
  <script>
    SwaggerUIBundle({ url: %q, dom_id: "#swagger-ui" });
  </script>
</body>
</html>`, specURL)
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(html))
	}
}

func serveSpec(specPath string) (http.HandlerFunc, error) {
	ext := filepath.Ext(specPath)
	if ext != ".json" && ext != ".yaml" && ext != ".yml" {
		return nil, fmt.Errorf("unsupported spec format %q (use .json, .yaml, or .yml)", ext)
	}

	data, err := os.ReadFile(specPath)
	if err != nil {
		return nil, fmt.Errorf("cannot read spec file: %w", err)
	}

	contentType := "application/json"
	if ext == ".yaml" || ext == ".yml" {
		contentType = "text/yaml; charset=utf-8"
	}

	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", contentType)
		w.Header().Set("Content-Length", fmt.Sprintf("%d", len(data)))
		w.WriteHeader(http.StatusOK)
		w.Write(data)
	}, nil
}

func main() {
	flag.Parse()

	var fileServer http.Handler
	if *noListing {
		fileServer = http.FileServer(noListDir{http.Dir(*dir)})
	} else {
		fileServer = http.FileServer(http.Dir(*dir))
	}

	httpMux := http.NewServeMux()

	if *spec != "" {
		specHandler, err := serveSpec(*spec)
		if err != nil {
			log.Fatalf("OpenAPI spec error: %v", err)
		}

		ext := filepath.Ext(*spec)
		specURL := fmt.Sprintf("/openapi%s", ext)
		httpMux.Handle(specURL, logger(specHandler))

		if *specUIPath == "/" {
			log.Fatalf("-spec-ui-path cannot be /")
		}
		if !strings.HasPrefix(*specUIPath, "/") {
			log.Fatalf("-spec-ui-path must start with /")
		}
		httpMux.Handle(*specUIPath, logger(swaggerUI(specURL)))
	}

	httpMux.Handle("/", logger(fileServer))

	srv := &http.Server{
		Addr:    fmt.Sprintf(":%s", *port),
		Handler: httpMux,
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	go func() {
		log.Printf("Up and running on port %s", *port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Server error: %v", err)
		}
	}()

	<-ctx.Done()
	log.Println("Shutting down...")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Fatalf("Shutdown error: %v", err)
	}
}
