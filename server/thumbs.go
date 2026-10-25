package main

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// Préfixes des vignettes QNAP (QuMagie / Multimedia Console), rangées dans « <dossier>/.@__thumb/ ».
// Toutes sont des JPEG, même quand le nom garde l'extension d'origine (« s800IMG_5902.CR2 »).
// Côté court : s100 = 100 px, default = 400 px, s800 = 800 px. Les vidéos anciennes utilisent
// a100 / adefault ; certaines n'ont qu'un « <nom>.jpg » (720 px).
var thumbPrefixes = map[string][]string{
	"s": {"s100", "a100", "default", "adefault"},
	"m": {"default", "adefault", "", "s800", "s100", "a100"},
	"l": {"s800", "", "default", "adefault"},
}

const qnapThumbDir = ".@__thumb"

// findThumb cherche une vignette existante pour un média, par ordre de préférence pour la taille demandée.
func findThumb(mediaPath, size string) (string, os.FileInfo) {
	dir, nom := filepath.Split(mediaPath)
	tdir := filepath.Join(dir, qnapThumbDir)
	stem := strings.TrimSuffix(nom, filepath.Ext(nom))
	for _, prefix := range thumbPrefixes[size] {
		var noms []string
		if prefix == "" {
			noms = []string{nom + ".jpg"}
		} else {
			noms = []string{prefix + nom}
			if ext(nom) != "jpg" {
				noms = append(noms, prefix+stem+".jpg")
			}
		}
		for _, n := range noms {
			p := filepath.Join(tdir, n)
			if st, err := os.Stat(p); err == nil && st.Mode().IsRegular() && st.Size() > 0 {
				return p, st
			}
		}
	}
	return "", nil
}

func (s *server) thumb(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	size := q.Get("size")
	if _, ok := thumbPrefixes[size]; !ok {
		writeError(w, http.StatusBadRequest, "size doit valoir s, m ou l")
		return
	}
	rel, p, err := s.resolve(q.Get("path"))
	if err != nil || rel == "" || mediaType(rel) == "" {
		writeError(w, http.StatusBadRequest, "chemin invalide")
		return
	}
	tp, st := findThumb(p, size)
	if tp == "" {
		writeError(w, http.StatusNotFound, "pas de vignette")
		return
	}
	f, err := os.Open(tp)
	if err != nil {
		writeError(w, http.StatusNotFound, "pas de vignette")
		return
	}
	defer f.Close()
	h := w.Header()
	h.Set("Content-Type", "image/jpeg")
	h.Set("Cache-Control", "public, max-age=31536000, immutable")
	h.Set("ETag", fmt.Sprintf(`"%x-%x"`, st.ModTime().Unix(), st.Size()))
	http.ServeContent(w, r, "", st.ModTime(), f)
}
