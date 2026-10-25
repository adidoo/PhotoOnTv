package main

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// Génération des vignettes absentes des dossiers QNAP (HEIC, vidéos…), dans un cache à part :
// le dossier des photos reste en lecture seule. Une vignette « l » (côté long 1200 px) est tirée de
// l'original par vips (photos) ou ffmpeg (vidéos), puis « m » (600) et « s » (150) sont réduites
// depuis « l ». Deux travailleurs : un pour les demandes de l'appli (prioritaire), un pour le
// balayage de fond, en priorité CPU et disque minimale.

var genSizes = map[string]int{"s": 150, "m": 600, "l": 1200}

const (
	cmdTimeout   = 2 * time.Minute
	rescanPeriod = 6 * time.Hour
	logEvery     = 250
)

type genJob struct {
	rel  string
	abs  string
	key  string
	kind string // "photo" ou "video"
	// smallOnly : seulement « m » et « s » depuis la miniature intégrée d'une HEIC (dossier ouvert,
	// grille) ; « l » viendra d'une demande plein écran ou du balayage de fond
	smallOnly bool
}

func (j genJob) queueKey() string {
	if j.smallOnly {
		return j.key + "-small"
	}
	return j.key
}

func isHeic(rel string) bool { e := ext(rel); return e == "heic" || e == "heif" }

type generator struct {
	dir    string
	ionice bool

	mu       sync.Mutex
	inflight map[string]chan struct{} // clé -> fermé quand la génération est finie
	queued   map[string]bool          // clés déjà dans la file des demandes

	demand chan genJob // demandes de l'appli et dossiers ouverts

	// Compteurs exposés par /api/status
	todo, done, failed, onDemand atomic.Int64
	scanning                     atomic.Bool
	current                      atomic.Value // string
	scanStart                    atomic.Int64 // Unix
}

func newGenerator(dir string) (*generator, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	_, errIo := exec.LookPath("ionice")
	g := &generator{
		dir:      dir,
		ionice:   errIo == nil,
		inflight: map[string]chan struct{}{},
		queued:   map[string]bool{},
		demand:   make(chan genJob, 512),
	}
	g.current.Store("")
	return g, nil
}

// Version du générateur vidéo, incluse dans la clé : la changer régénère toutes les vidéos.
// v2 : pixels non carrés (SAR) corrigés avant la mise à l'échelle.
const videoGenVersion = "v2"

// cacheKey dépend du chemin, de la date et de la taille : un fichier modifié est régénéré.
func cacheKey(rel string, st os.FileInfo) string {
	v := ""
	if mediaType(rel) == "video" {
		v = "|" + videoGenVersion
	}
	h := sha1.Sum([]byte(fmt.Sprintf("%s|%d|%d%s", rel, st.ModTime().Unix(), st.Size(), v)))
	return hex.EncodeToString(h[:])
}

func (g *generator) base(key string) string { return filepath.Join(g.dir, key[:2], key) }

func (g *generator) thumbPath(key, size string) string { return g.base(key) + "-" + size + ".jpg" }

func (g *generator) failPath(key string) string { return g.base(key) + ".fail" }

// cached renvoie la vignette générée si elle existe ; failed indique un échec mémorisé.
func (g *generator) cached(key, size string) (p string, st os.FileInfo, failed bool) {
	p = g.thumbPath(key, size)
	if st, err := os.Stat(p); err == nil {
		return p, st, false
	}
	if _, err := os.Stat(g.failPath(key)); err == nil {
		return "", nil, true
	}
	return "", nil, false
}

func (g *generator) hasSmall(key string) bool {
	_, errM := os.Stat(g.thumbPath(key, "m"))
	_, errS := os.Stat(g.thumbPath(key, "s"))
	return errM == nil && errS == nil
}

// isDone : les trois tailles sont là, ou l'échec est mémorisé. Chaque taille est publiée dès
// qu'elle est prête (renommage atomique), dans un ordre qui dépend du type de média.
func (g *generator) isDone(key string) bool {
	if _, err := os.Stat(g.failPath(key)); err == nil {
		return true
	}
	for s := range genSizes {
		if _, err := os.Stat(g.thumbPath(key, s)); err != nil {
			return false
		}
	}
	return true
}

// enqueue place un média dans la file prioritaire (une seule fois tant qu'il y est).
func (g *generator) enqueue(j genJob) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.queued[j.queueKey()] {
		return
	}
	select {
	case g.demand <- j:
		g.queued[j.queueKey()] = true
	default: // file pleine : le balayage de fond s'en chargera
	}
}

// wait attend, au plus d, que la vignette de cette taille existe (ou que le média soit clos).
func (g *generator) wait(key, size string, d time.Duration) {
	deadline := time.Now().Add(d)
	for time.Now().Before(deadline) {
		if p, _, failed := g.cached(key, size); p != "" || failed || g.isDone(key) {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
}

// run génère un média s'il ne l'est pas déjà ; renvoie false s'il n'y avait rien à faire.
func (g *generator) run(j genJob, background bool) bool {
	g.mu.Lock()
	delete(g.queued, j.queueKey())
	if ch := g.inflight[j.key]; ch != nil {
		g.mu.Unlock()
		<-ch
		return false
	}
	if g.isDone(j.key) || (j.smallOnly && g.hasSmall(j.key)) {
		g.mu.Unlock()
		return false
	}
	ch := make(chan struct{})
	g.inflight[j.key] = ch
	g.mu.Unlock()

	g.current.Store(j.rel)
	debut := time.Now()
	err := g.generate(j, background)
	if err != nil {
		g.failed.Add(1)
		os.MkdirAll(filepath.Dir(g.failPath(j.key)), 0o755)
		os.WriteFile(g.failPath(j.key), []byte(j.rel+"\n"+err.Error()+"\n"), 0o644)
		log.Printf("vignette impossible (%s) : %s : %v", time.Since(debut).Round(time.Millisecond), j.rel, err)
	}
	g.current.Store("")

	g.mu.Lock()
	delete(g.inflight, j.key)
	g.mu.Unlock()
	close(ch)
	return true
}

func (g *generator) demandWorker() {
	for j := range g.demand {
		if g.run(j, false) {
			g.onDemand.Add(1)
		}
	}
}

// command prépare une commande, en priorité minimale pour le balayage de fond.
func (g *generator) command(ctx context.Context, background bool, name string, args ...string) *exec.Cmd {
	niceness := "5"
	if background {
		niceness = "19"
	}
	full := []string{"-n", niceness}
	if background && g.ionice {
		full = append(full, "ionice", "-c", "3")
	}
	full = append(full, name)
	full = append(full, args...)
	cmd := exec.CommandContext(ctx, "nice", full...)
	if background {
		cmd.Env = append(os.Environ(), "VIPS_CONCURRENCY=1")
	}
	return cmd
}

func (g *generator) exec(background bool, name string, args ...string) error {
	ctx, cancel := context.WithTimeout(context.Background(), cmdTimeout)
	defer cancel()
	out, err := g.command(ctx, background, name, args...).CombinedOutput()
	if err != nil {
		msg := strings.TrimSpace(string(out))
		if len(msg) > 300 {
			msg = msg[:300]
		}
		return fmt.Errorf("%s : %v : %s", name, err, msg)
	}
	return nil
}

func (g *generator) generate(j genJob, background bool) error {
	if err := os.MkdirAll(filepath.Dir(g.base(j.key)), 0o755); err != nil {
		return err
	}
	tmp := func(size string) string { return g.base(j.key) + "-" + size + ".tmp.jpg" }
	l := tmp("l")
	defer func() {
		for s := range genSizes {
			os.Remove(tmp(s))
		}
	}()

	// HEIC : le décodage complet prend plusieurs secondes sur un petit NAS, mais la miniature
	// intégrée (~384×512) se lit en une seconde et suffit pour « m » et « s » : elles sont
	// publiées tout de suite, « l » suit.
	smallDone := g.hasSmall(j.key)
	if !smallDone && isHeic(j.rel) {
		q := tmp("q")
		defer os.Remove(q)
		if g.exec(background, "vips", "heifload", j.abs, q+"[Q=85]", "--thumbnail") == nil {
			smallDone = g.reduce(q, j.key, tmp, background) == nil
		}
	}
	if j.smallOnly && smallDone {
		return nil
	}

	lPub := g.thumbPath(j.key, "l")
	if _, err := os.Stat(lPub); err != nil {
		var err error
		if j.kind == "video" {
			err = g.videoFrame(j.abs, l, background)
		} else {
			err = g.vipsThumb(j.abs, l, genSizes["l"], background)
		}
		if err != nil {
			return err
		}
		if st, e := os.Stat(l); e != nil || st.Size() == 0 {
			return errors.New("aucune image produite")
		}
		// « l » publiée d'abord : la réduction en « m » et « s » ne prend qu'un instant
		if err := os.Rename(l, lPub); err != nil {
			return err
		}
	}
	if !smallDone {
		return g.reduce(lPub, j.key, tmp, background)
	}
	return nil
}

// reduce produit et publie « m » puis « s » à partir d'une image source.
func (g *generator) reduce(src, key string, tmp func(string) string, background bool) error {
	for _, s := range []string{"m", "s"} {
		if err := g.vipsThumb(src, tmp(s), genSizes[s], background); err != nil {
			return err
		}
		if err := os.Rename(tmp(s), g.thumbPath(key, s)); err != nil {
			return err
		}
	}
	return nil
}

// vipsThumb réduit une image (orientation EXIF appliquée) sans jamais l'agrandir.
func (g *generator) vipsThumb(in, out string, size int, background bool) error {
	n := strconv.Itoa(size)
	return g.exec(background, "vips", "thumbnail", in, out+"[Q=80,strip]", n, "--height", n, "--size", "down")
}

// videoFrame extrait une image vers 10 % de la durée (rotation appliquée par ffmpeg).
func (g *generator) videoFrame(in, out string, background bool) error {
	// à la demande, image à 1 s (on évite ffprobe pour répondre vite) ; en fond, vers 10 %
	at := 1.0
	if background {
		ctx, cancel := context.WithTimeout(context.Background(), cmdTimeout)
		o, err := g.command(ctx, background, "ffprobe", "-v", "error", "-show_entries", "format=duration",
			"-of", "csv=p=0", in).Output()
		cancel()
		if err == nil {
			if d, e := strconv.ParseFloat(strings.TrimSpace(string(o)), 64); e == nil && d > 0 {
				at = min(d*0.1, 30)
			}
		}
	}
	n := strconv.Itoa(genSizes["l"])
	// pixels rendus carrés d'abord (AVCHD 1440×1080 → 1920×1080), puis réduction sans agrandir
	vf := "yadif=deint=interlaced,scale=trunc(iw*sar/2)*2:ih,setsar=1," +
		"scale=w=min(" + n + "\\,iw):h=min(" + n + "\\,ih):force_original_aspect_ratio=decrease"
	args := func(t float64) []string {
		a := []string{"-nostdin", "-v", "error", "-y"}
		if t > 0 {
			a = append(a, "-ss", strconv.FormatFloat(t, 'f', 2, 64))
		}
		a = append(a, "-i", in, "-frames:v", "1", "-vf", vf, "-q:v", "4")
		if background {
			a = append(a, "-threads", "1")
		}
		return append(a, out)
	}
	err := g.exec(background, "ffmpeg", args(at)...)
	if st, e := os.Stat(out); err != nil || e != nil || st.Size() == 0 {
		// vidéo très courte ou horodatage faux : première image
		os.Remove(out)
		err = g.exec(background, "ffmpeg", args(0)...)
	}
	return err
}

// needsGeneration : pas de vignette QNAP de taille « l » et rien en cache.
func (g *generator) needsGeneration(abs, key string) bool {
	if p, _ := findThumbExact(abs, "l"); p != "" {
		return false
	}
	return !g.isDone(key)
}

// scanLoop balaie toute l'arborescence et génère ce qui manque, puis recommence périodiquement.
func (s *server) scanLoop() {
	g := s.gen
	for {
		g.scanning.Store(true)
		g.scanStart.Store(time.Now().Unix())
		var jobs []genJob
		s.walkMedia("", func(rel, abs string, st os.FileInfo) {
			key := cacheKey(rel, st)
			if g.needsGeneration(abs, key) {
				jobs = append(jobs, genJob{rel: rel, abs: abs, key: key, kind: mediaType(rel)})
			}
		})
		g.todo.Store(int64(len(jobs)))
		g.done.Store(0)
		log.Printf("balayage : %d médias sans vignette", len(jobs))

		debut := time.Now()
		for i, j := range jobs {
			g.run(j, true)
			g.done.Store(int64(i + 1))
			if (i+1)%logEvery == 0 {
				reste := time.Duration(float64(time.Since(debut)) / float64(i+1) * float64(len(jobs)-i-1))
				log.Printf("balayage : %d/%d, %d échecs, reste ~%s", i+1, len(jobs), g.failed.Load(), reste.Round(time.Minute))
			}
		}
		if len(jobs) > 0 {
			log.Printf("balayage terminé : %d médias en %s, %d échecs", len(jobs), time.Since(debut).Round(time.Second), g.failed.Load())
		}
		g.scanning.Store(false)
		time.Sleep(rescanPeriod)
	}
}

// walkMedia appelle f pour chaque média de l'arborescence (dossiers masqués exclus).
func (s *server) walkMedia(rel string, f func(rel, abs string, st os.FileInfo)) {
	dir := filepath.Join(s.cfg.root, filepath.FromSlash(rel))
	dirs, media, err := s.readSorted(dir)
	if err != nil {
		return
	}
	for _, e := range media {
		if st, err := e.Info(); err == nil {
			f(path.Join(rel, e.Name()), filepath.Join(dir, e.Name()), st)
		}
	}
	for _, d := range dirs {
		s.walkMedia(path.Join(rel, d), f)
	}
}
