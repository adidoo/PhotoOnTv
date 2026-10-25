package main

import (
	"encoding/json"
	"math/rand/v2"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"sync"
	"time"
)

// Liste des photos d'un dossier et de tous ses sous-dossiers (diaporama), gardée un moment pour que
// les paquets successifs ne reparcourent pas l'arborescence.
const photosTTL = 10 * time.Minute

type photoList struct {
	built time.Time
	paths []string
}

var (
	photosMu    sync.Mutex
	photosCache = map[string]photoList{}
)

// allPhotos parcourt rel en profondeur, dans l'ordre d'affichage de la grille : sous-dossiers d'abord
// (chacun entièrement), puis les photos du dossier. desc inverse le tri à chaque niveau.
func (s *server) allPhotos(rel string, desc bool) []string {
	key := rel + "\x00" + strconv.FormatBool(desc)
	photosMu.Lock()
	c, ok := photosCache[key]
	photosMu.Unlock()
	if ok && time.Since(c.built) < photosTTL {
		return c.paths
	}
	var paths []string
	var visit func(r string)
	visit = func(r string) {
		dirs, media, err := s.readSorted(filepath.Join(s.cfg.root, filepath.FromSlash(r)))
		if err != nil {
			return
		}
		if desc {
			for i, j := 0, len(dirs)-1; i < j; i, j = i+1, j-1 {
				dirs[i], dirs[j] = dirs[j], dirs[i]
			}
			for i, j := 0, len(media)-1; i < j; i, j = i+1, j-1 {
				media[i], media[j] = media[j], media[i]
			}
		}
		for _, d := range dirs {
			visit(path.Join(r, d))
		}
		for _, e := range media {
			if mediaType(e.Name()) == "photo" {
				paths = append(paths, path.Join(r, e.Name()))
			}
		}
	}
	visit(rel)
	photosMu.Lock()
	photosCache[key] = photoList{built: time.Now(), paths: paths}
	photosMu.Unlock()
	return paths
}

// photos renvoie un paquet des photos d'un dossier et de ses sous-dossiers, pour le diaporama.
func (s *server) photos(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	rel, dir, err := s.resolve(q.Get("path"))
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if st, err := os.Stat(dir); err != nil || !st.IsDir() {
		writeError(w, http.StatusNotFound, "dossier introuvable")
		return
	}
	order := q.Get("order")
	if order == "" {
		order = "asc"
	}
	if order != "asc" && order != "desc" && order != "random" {
		writeError(w, http.StatusBadRequest, "order doit valoir asc, desc ou random")
		return
	}
	n, _ := strconv.Atoi(q.Get("n"))
	if n <= 0 || n > 1000 {
		n = 200
	}
	offset, _ := strconv.Atoi(q.Get("offset"))

	all := s.allPhotos(rel, order == "desc")
	var at func(i int) string
	if order == "random" {
		// Même graine = même mélange : les paquets successifs d'un diaporama restent cohérents
		seed, _ := strconv.ParseUint(q.Get("seed"), 10, 64)
		perm := rand.New(rand.NewPCG(seed, 0x9e3779b97f4a7c15)).Perm(len(all))
		at = func(i int) string { return all[perm[i]] }
	} else {
		at = func(i int) string { return all[i] }
		if start := q.Get("start"); start != "" {
			for i, p := range all {
				if p == start {
					offset = i
					break
				}
			}
		}
	}
	if offset < 0 {
		offset = 0
	}
	if offset > len(all) {
		offset = len(all)
	}
	end := min(offset+n, len(all))

	items := make([]mediaEntry, 0, end-offset)
	for i := offset; i < end; i++ {
		p := at(i)
		st, err := os.Stat(filepath.Join(s.cfg.root, filepath.FromSlash(p)))
		if err != nil {
			continue
		}
		items = append(items, mediaEntry{
			Name: path.Base(p), Path: p, Type: "photo", Size: st.Size(), Mtime: st.ModTime().Unix(),
		})
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	json.NewEncoder(w).Encode(map[string]any{"path": rel, "total": len(all), "offset": offset, "items": items})
}
