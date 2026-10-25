// Serveur PhotoOnTv : expose un dossier de photos (lecture seule) à l'appli TV.
// Contrat d'API : docs/api.md.
package main

import (
	"log"
	"net/http"
	"os"
	"strings"
	"time"
)

const version = "0.2.0"

type config struct {
	root    string          // racine des photos (montée en lecture seule)
	listen  string          // adresse d'écoute
	exclude map[string]bool // noms de dossiers ignorés, en plus de ceux commençant par « . » ou « @ »
	cacheDir string         // vignettes générées ; vide = pas de génération
}

func env(nom, defaut string) string {
	if v := os.Getenv(nom); v != "" {
		return v
	}
	return defaut
}

func main() {
	cfg := config{
		root:    env("PHOTOS_ROOT", "/photos"),
		listen:  env("LISTEN", ":8080"),
		exclude:  map[string]bool{},
		cacheDir: os.Getenv("CACHE_DIR"),
	}
	for _, n := range strings.Split(os.Getenv("EXCLUDE"), ",") {
		if n = strings.TrimSpace(n); n != "" {
			cfg.exclude[n] = true
		}
	}
	if st, err := os.Stat(cfg.root); err != nil || !st.IsDir() {
		log.Fatalf("PHOTOS_ROOT invalide : %s", cfg.root)
	}

	s := newServer(cfg)
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/health", s.health)
	mux.HandleFunc("GET /api/status", s.status)
	mux.HandleFunc("GET /api/info", s.info)
	mux.HandleFunc("GET /api/folder", s.folder)
	mux.HandleFunc("GET /api/thumb", s.thumb)
	mux.HandleFunc("GET /api/file", s.file)
	mux.HandleFunc("GET /api/file/{path...}", s.file)

	log.Printf("PhotoOnTv %s : %s sur %s", version, cfg.root, cfg.listen)
	srv := &http.Server{
		Addr:              cfg.listen,
		Handler:           withCommon(mux),
		ReadHeaderTimeout: 10 * time.Second,
	}
	log.Fatal(srv.ListenAndServe())
}

// withCommon ajoute le CORS et une ligne de journal par requête lente ou en erreur.
func withCommon(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Headers", "Range")
		w.Header().Set("Access-Control-Expose-Headers", "Content-Length, Content-Range, Accept-Ranges")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		debut := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: 200}
		h.ServeHTTP(rec, r)
		if d := time.Since(debut); rec.status >= 400 || d > 300*time.Millisecond {
			log.Printf("%d %s %s (%s)", rec.status, r.Method, r.URL.RequestURI(), d.Round(time.Millisecond))
		}
	})
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}
