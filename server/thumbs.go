package main

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Préfixes des vignettes QNAP (QuMagie / Multimedia Console), rangées dans « <dossier>/.@__thumb/ ».
// Toutes sont des JPEG, même quand le nom garde l'extension d'origine (« s800IMG_5902.CR2 »).
// Côté court : s100 = 100 px, default = 400 px, s800 = 800 px. Les vidéos anciennes utilisent
// a100 / adefault ; certaines n'ont qu'un « <nom>.jpg » (720 px, préfixe vide ci-dessous).
var (
	// vignettes de la bonne taille
	thumbExact = map[string][]string{
		"s": {"s100", "a100"},
		"m": {"default", "adefault"},
		"l": {"s800", ""},
	}
	// à défaut de mieux (taille différente), après les vignettes générées
	thumbFallback = map[string][]string{
		"s": {"default", "adefault"},
		"m": {"", "s800", "s100", "a100"},
		"l": {"default", "adefault"},
	}
)

const qnapThumbDir = ".@__thumb"

// Les vignettes QNAP des vidéos AVCHD (pixels non carrés, 1440×1080 affiché en 16:9) sont écrasées
// en 4:3 : on les ignore pour ces extensions, le serveur génère les siennes.
var qnapIgnoredExt = map[string]bool{"mts": true, "m2ts": true}

// Délai d'attente d'une vignette générée à la demande avant de répondre 404
var demandWait = map[string]time.Duration{"photo": 4 * time.Second, "video": 8 * time.Second}

// findQnap cherche une vignette QNAP existante parmi des préfixes, dans l'ordre.
func findQnap(mediaPath string, prefixes []string) (string, os.FileInfo) {
	dir, nom := filepath.Split(mediaPath)
	if qnapIgnoredExt[ext(nom)] {
		return "", nil
	}
	tdir := filepath.Join(dir, qnapThumbDir)
	stem := strings.TrimSuffix(nom, filepath.Ext(nom))
	for _, prefix := range prefixes {
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

func findThumbExact(mediaPath, size string) (string, os.FileInfo) {
	return findQnap(mediaPath, thumbExact[size])
}

func (s *server) thumb(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	size := q.Get("size")
	if _, ok := thumbExact[size]; !ok {
		writeError(w, http.StatusBadRequest, "size doit valoir s, m ou l")
		return
	}
	rel, p, err := s.resolve(q.Get("path"))
	if err != nil || rel == "" || mediaType(rel) == "" {
		writeError(w, http.StatusBadRequest, "chemin invalide")
		return
	}
	media, err := os.Stat(p)
	if err != nil || !media.Mode().IsRegular() {
		writeError(w, http.StatusNotFound, "fichier introuvable")
		return
	}

	// 1. vignette QNAP de la bonne taille
	tp, st := findThumbExact(p, size)
	provisoire := false
	if tp == "" && s.gen != nil {
		// 2. vignette générée, éventuellement à la demande
		key := cacheKey(rel, media)
		gp, gst, failed := s.gen.cached(key, size)
		if gp == "" && !failed {
			if lp, _ := findThumbExact(p, "l"); lp == "" {
				// pas de « l » QNAP : c'est un média à générer
				s.gen.enqueue(genJob{rel: rel, abs: p, key: key, kind: mediaType(rel), smallOnly: size != "l" && isHeic(rel)})
				s.gen.wait(key, size, demandWait[mediaType(rel)])
				gp, gst, _ = s.gen.cached(key, size)
			}
		}
		if gp == "" && size == "l" {
			// « l » pas (encore) prête mais « m » oui (HEIC) : mieux que rien en plein écran,
			// sans cache long pour que la vraie « l » soit servie plus tard
			if gp, gst, _ = s.gen.cached(key, "m"); gp != "" {
				provisoire = true
			}
		}
		tp, st = gp, gst
	}
	if tp == "" {
		// 3. vignette QNAP d'une autre taille
		tp, st = findQnap(p, thumbFallback[size])
	}
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
	if provisoire {
		h.Set("Cache-Control", "public, max-age=60")
	} else {
		h.Set("Cache-Control", "public, max-age=31536000, immutable")
	}
	h.Set("ETag", fmt.Sprintf(`"%x-%x"`, st.ModTime().Unix(), st.Size()))
	http.ServeContent(w, r, "", st.ModTime(), f)
}
