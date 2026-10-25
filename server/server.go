package main

import (
	"encoding/json"
	"errors"
	"io/fs"
	"log"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// Durée maximale pendant laquelle une liste de dossier est resservie sans relecture : la date de
// modification d'un dossier ne bouge pas quand le contenu d'un de ses sous-dossiers change.
const folderTTL = 10 * time.Minute

type server struct {
	cfg   config
	mu    sync.Mutex
	cache map[string]cachedFolder
	gen   *generator // nil si la génération est désactivée (pas de CACHE_DIR)

	opened sync.Map // dossiers déjà ouverts par l'appli : leurs médias à générer passent devant
}

type cachedFolder struct {
	modTime time.Time
	built   time.Time
	body    []byte
}

type folderJSON struct {
	Path    string       `json:"path"`
	Name    string       `json:"name"`
	Parent  *string      `json:"parent"`
	Folders []subfolder  `json:"folders"`
	Items   []mediaEntry `json:"items"`
}

type subfolder struct {
	Name    string  `json:"name"`
	Path    string  `json:"path"`
	Folders int     `json:"folders"`
	Items   int     `json:"items"`
	Cover   *string `json:"cover"`
}

type mediaEntry struct {
	Name  string `json:"name"`
	Path  string `json:"path"`
	Type  string `json:"type"`
	Size  int64  `json:"size"`
	Mtime int64  `json:"mtime"`
}

func newServer(cfg config) *server {
	s := &server{cfg: cfg, cache: map[string]cachedFolder{}}
	if cfg.cacheDir != "" {
		g, err := newGenerator(cfg.cacheDir)
		if err != nil {
			log.Printf("génération des vignettes désactivée : %v", err)
		} else {
			s.gen = g
			go g.demandWorker()
		}
	}
	go func() {
		s.warmUp()
		if s.gen != nil {
			s.scanLoop()
		}
	}()
	return s
}

// prioritize met en tête de file les médias sans vignette d'un dossier que l'appli vient d'ouvrir.
func (s *server) prioritize(rel string) {
	if s.gen == nil {
		return
	}
	if _, deja := s.opened.LoadOrStore(rel, true); deja {
		return
	}
	dir := filepath.Join(s.cfg.root, filepath.FromSlash(rel))
	_, media, err := s.readSorted(dir)
	if err != nil {
		return
	}
	for _, e := range media {
		st, err := e.Info()
		if err != nil {
			continue
		}
		mrel, abs := path.Join(rel, e.Name()), filepath.Join(dir, e.Name())
		key := cacheKey(mrel, st)
		if s.gen.needsGeneration(abs, key) {
			s.gen.enqueue(genJob{rel: mrel, abs: abs, key: key, kind: mediaType(mrel), smallOnly: isHeic(mrel)})
		}
	}
}

func (s *server) status(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if s.gen == nil {
		json.NewEncoder(w).Encode(map[string]any{"generation": false})
		return
	}
	g := s.gen
	json.NewEncoder(w).Encode(map[string]any{
		"generation": true,
		"scanning":   g.scanning.Load(),
		"todo":       g.todo.Load(),
		"done":       g.done.Load(),
		"failed":     g.failed.Load(),
		"on_demand":  g.onDemand.Load(),
		"queue":      len(g.demand),
		"current":    g.current.Load(),
		"scan_start": g.scanStart.Load(),
	})
}

// hidden indique si un nom de dossier ou de fichier doit être ignoré.
func (s *server) hidden(nom string) bool {
	return strings.HasPrefix(nom, ".") || strings.HasPrefix(nom, "@") || s.cfg.exclude[nom]
}

// resolve valide un chemin relatif de l'API et renvoie le chemin disque correspondant.
func (s *server) resolve(rel string) (string, string, error) {
	rel = strings.Trim(strings.ReplaceAll(rel, "\\", "/"), "/")
	if rel == "" {
		return "", s.cfg.root, nil
	}
	for _, p := range strings.Split(rel, "/") {
		if p == "" || p == "." || p == ".." || s.hidden(p) {
			return "", "", errBadPath
		}
	}
	return rel, filepath.Join(s.cfg.root, filepath.FromSlash(rel)), nil
}

var errBadPath = errors.New("chemin invalide")

func writeError(w http.ResponseWriter, code int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

func (s *server) health(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"ok": true, "version": version, "root": s.cfg.root})
}

func (s *server) folder(w http.ResponseWriter, r *http.Request) {
	rel, dir, err := s.resolve(r.URL.Query().Get("path"))
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	body, err := s.folderBody(rel, dir)
	if errors.Is(err, fs.ErrNotExist) {
		writeError(w, http.StatusNotFound, "dossier introuvable")
		return
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-cache")
	w.Write(body)
	go s.prioritize(rel)
}

// folderBody renvoie le JSON d'un dossier, depuis le cache s'il est encore valable.
func (s *server) folderBody(rel, dir string) ([]byte, error) {
	st, err := os.Stat(dir)
	if err != nil {
		return nil, err
	}
	if !st.IsDir() {
		return nil, fs.ErrNotExist
	}
	s.mu.Lock()
	c, ok := s.cache[rel]
	s.mu.Unlock()
	if ok && c.modTime.Equal(st.ModTime()) && time.Since(c.built) < folderTTL {
		return c.body, nil
	}

	f, err := s.listFolder(rel, dir)
	if err != nil {
		return nil, err
	}
	body, err := json.Marshal(f)
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	s.cache[rel] = cachedFolder{modTime: st.ModTime(), built: time.Now(), body: body}
	s.mu.Unlock()
	return body, nil
}

// readSorted lit un dossier et sépare sous-dossiers et médias, triés dans l'ordre naturel.
func (s *server) readSorted(dir string) (dirs []string, media []fs.DirEntry, err error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, nil, err
	}
	for _, e := range entries {
		nom := e.Name()
		if s.hidden(nom) {
			continue
		}
		switch {
		case e.IsDir():
			dirs = append(dirs, nom)
		case e.Type().IsRegular() && mediaType(nom) != "":
			media = append(media, e)
		}
	}
	sort.Slice(dirs, func(i, j int) bool { return naturalLess(dirs[i], dirs[j]) })
	sort.Slice(media, func(i, j int) bool { return naturalLess(media[i].Name(), media[j].Name()) })
	return dirs, media, nil
}

func (s *server) listFolder(rel, dir string) (*folderJSON, error) {
	dirs, media, err := s.readSorted(dir)
	if err != nil {
		return nil, err
	}
	f := &folderJSON{Path: rel, Name: path.Base(rel), Folders: []subfolder{}, Items: []mediaEntry{}}
	if rel == "" {
		f.Name = ""
	} else {
		p := path.Dir(rel)
		if p == "." {
			p = ""
		}
		f.Parent = &p
	}
	for _, nom := range dirs {
		subRel := path.Join(rel, nom)
		subDirs, subMedia, err := s.readSorted(filepath.Join(dir, nom))
		if err != nil {
			continue // dossier illisible : on le saute
		}
		sf := subfolder{Name: nom, Path: subRel, Folders: len(subDirs), Items: len(subMedia)}
		sf.Cover = s.cover(subRel, subDirs, subMedia, 4)
		f.Folders = append(f.Folders, sf)
	}
	for _, e := range media {
		info, err := e.Info()
		if err != nil {
			continue
		}
		f.Items = append(f.Items, mediaEntry{
			Name:  e.Name(),
			Path:  path.Join(rel, e.Name()),
			Type:  mediaType(e.Name()),
			Size:  info.Size(),
			Mtime: info.ModTime().Unix(),
		})
	}
	return f, nil
}

// cover choisit le premier média du dossier, sinon cherche en profondeur dans ses sous-dossiers.
func (s *server) cover(rel string, dirs []string, media []fs.DirEntry, depth int) *string {
	if c := s.pickCover(rel, media); c != nil {
		return c
	}
	if depth == 0 {
		return nil
	}
	for _, nom := range dirs {
		subRel := path.Join(rel, nom)
		subDirs, subMedia, err := s.readSorted(filepath.Join(s.cfg.root, filepath.FromSlash(subRel)))
		if err != nil {
			continue
		}
		if c := s.cover(subRel, subDirs, subMedia, depth-1); c != nil {
			return c
		}
	}
	return nil
}

// pickCover choisit la couverture parmi les médias d'un dossier : de préférence le premier qui a
// une petite vignette QNAP (une seule lecture de « .@__thumb »), sinon une déjà générée, sinon la
// première photo, sinon le premier média.
func (s *server) pickCover(rel string, media []fs.DirEntry) *string {
	if len(media) == 0 {
		return nil
	}
	choix := func(e fs.DirEntry) *string { c := path.Join(rel, e.Name()); return &c }
	dir := filepath.Join(s.cfg.root, filepath.FromSlash(rel))
	if entries, err := os.ReadDir(filepath.Join(dir, qnapThumbDir)); err == nil {
		noms := make(map[string]bool, len(entries))
		for _, t := range entries {
			noms[t.Name()] = true
		}
		for _, e := range media {
			nom := e.Name()
			if qnapIgnoredExt[ext(nom)] {
				continue
			}
			stem := strings.TrimSuffix(nom, filepath.Ext(nom))
			if noms[nom+".jpg"] {
				return choix(e)
			}
			for _, prefix := range []string{"s100", "default", "a100", "adefault"} {
				if noms[prefix+nom] || noms[prefix+stem+".jpg"] {
					return choix(e)
				}
			}
		}
	}
	if s.gen != nil {
		for i, e := range media {
			if i == 30 {
				break
			}
			if st, err := e.Info(); err == nil {
				if p, _, _ := s.gen.cached(cacheKey(path.Join(rel, e.Name()), st), "s"); p != "" {
					return choix(e)
				}
			}
		}
	}
	for _, e := range media {
		if mediaType(e.Name()) == "photo" {
			return choix(e)
		}
	}
	return choix(media[0])
}

// warmUp parcourt toute l'arborescence au démarrage pour remplir le cache des dossiers
// (et celui du système de fichiers du NAS) : la première navigation est alors instantanée.
func (s *server) warmUp() {
	debut := time.Now()
	n := 0
	var visit func(rel string)
	visit = func(rel string) {
		dir := filepath.Join(s.cfg.root, filepath.FromSlash(rel))
		if _, err := s.folderBody(rel, dir); err != nil {
			return
		}
		n++
		dirs, _, err := s.readSorted(dir)
		if err != nil {
			return
		}
		for _, d := range dirs {
			visit(path.Join(rel, d))
		}
	}
	visit("")
	log.Printf("cache préchauffé : %d dossiers en %s", n, time.Since(debut).Round(time.Millisecond))
}

func (s *server) file(w http.ResponseWriter, r *http.Request) {
	// Deux formes : /api/file?path=<chemin> et /api/file/<chemin> (URL terminée par le nom du fichier)
	chemin := r.PathValue("path")
	if chemin == "" {
		chemin = r.URL.Query().Get("path")
	}
	rel, p, err := s.resolve(chemin)
	if err != nil || rel == "" || mediaType(rel) == "" {
		writeError(w, http.StatusBadRequest, "chemin invalide")
		return
	}
	f, err := os.Open(p)
	if err != nil {
		writeError(w, http.StatusNotFound, "fichier introuvable")
		return
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil || st.IsDir() {
		writeError(w, http.StatusNotFound, "fichier introuvable")
		return
	}
	ct := mimeTypes[ext(rel)]
	if ct == "" {
		ct = "application/octet-stream"
	}
	w.Header().Set("Content-Type", ct)
	w.Header().Set("Cache-Control", "public, max-age=86400")
	// ServeContent gère Range (206), If-Modified-Since et HEAD
	http.ServeContent(w, r, "", st.ModTime(), f)
}
