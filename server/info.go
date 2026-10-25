package main

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"
)

// /api/info : caractéristiques d'une vidéo lues par ffprobe, pour que l'appli règle la rotation et
// le ratio du lecteur de la télé. Résultats gardés en mémoire (clé : chemin + date + taille).

type videoInfo struct {
	Width     int     `json:"width"`
	Height    int     `json:"height"`
	Rotation  int     `json:"rotation"` // sens horaire d'affichage : 0, 90, 180 ou 270
	DAR       string  `json:"dar"`      // ratio d'affichage (SAR compris), ex. « 16:9 »
	DARValue  float64 `json:"dar_value"`
	Duration  float64 `json:"duration"`
	VCodec    string  `json:"vcodec"`
	ACodec    string  `json:"acodec"`
	Container string  `json:"container"`
}

var infoCache sync.Map // clé -> *videoInfo

type probeJSON struct {
	Streams []struct {
		CodecType string `json:"codec_type"`
		CodecName string `json:"codec_name"`
		Width     int    `json:"width"`
		Height    int    `json:"height"`
		SAR       string `json:"sample_aspect_ratio"`
		DAR       string `json:"display_aspect_ratio"`
		Tags      struct {
			Rotate string `json:"rotate"`
		} `json:"tags"`
		SideData []struct {
			Rotation *float64 `json:"rotation"`
		} `json:"side_data_list"`
	} `json:"streams"`
	Format struct {
		FormatName string `json:"format_name"`
		Duration   string `json:"duration"`
	} `json:"format"`
}

func ratio(s string) float64 {
	a, b, ok := strings.Cut(s, ":")
	if !ok {
		return 0
	}
	x, e1 := strconv.ParseFloat(a, 64)
	y, e2 := strconv.ParseFloat(b, 64)
	if e1 != nil || e2 != nil || x <= 0 || y <= 0 {
		return 0
	}
	return x / y
}

func probe(abs string) (*videoInfo, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "ffprobe", "-v", "error", "-show_streams", "-show_format", "-of", "json", abs).Output()
	if err != nil {
		return nil, err
	}
	var p probeJSON
	if err := json.Unmarshal(out, &p); err != nil {
		return nil, err
	}
	info := &videoInfo{Container: p.Format.FormatName}
	info.Duration, _ = strconv.ParseFloat(p.Format.Duration, 64)
	video := false
	for _, s := range p.Streams {
		switch {
		case s.CodecType == "video" && !video && s.Width > 0:
			video = true
			info.Width, info.Height, info.VCodec = s.Width, s.Height, s.CodecName
			rot := 0.0
			if r, err := strconv.ParseFloat(s.Tags.Rotate, 64); err == nil {
				rot = r
			}
			for _, sd := range s.SideData {
				if sd.Rotation != nil {
					rot = -*sd.Rotation // displaymatrix : angle anti-horaire
				}
			}
			info.Rotation = ((int(math.Round(rot/90))*90)%360 + 360) % 360
			dar := ratio(s.DAR)
			if dar == 0 {
				sar := ratio(s.SAR)
				if sar == 0 {
					sar = 1
				}
				dar = float64(s.Width) * sar / float64(s.Height)
				info.DAR = strconv.FormatFloat(math.Round(dar*1000)/1000, 'f', -1, 64)
			} else {
				info.DAR = s.DAR
			}
			info.DARValue = math.Round(dar*1000) / 1000
		case s.CodecType == "audio" && info.ACodec == "":
			info.ACodec = s.CodecName
		}
	}
	if !video {
		return nil, fmt.Errorf("pas de flux vidéo")
	}
	return info, nil
}

func (s *server) info(w http.ResponseWriter, r *http.Request) {
	rel, p, err := s.resolve(r.URL.Query().Get("path"))
	if err != nil || rel == "" || mediaType(rel) != "video" {
		writeError(w, http.StatusBadRequest, "chemin de vidéo invalide")
		return
	}
	st, err := os.Stat(p)
	if err != nil {
		writeError(w, http.StatusNotFound, "fichier introuvable")
		return
	}
	key := cacheKey(rel, st)
	v, ok := infoCache.Load(key)
	if !ok {
		vi, err := probe(p)
		if err != nil {
			writeError(w, http.StatusUnprocessableEntity, "vidéo illisible : "+err.Error())
			return
		}
		infoCache.Store(key, vi)
		v = vi
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "public, max-age=86400")
	json.NewEncoder(w).Encode(v)
}
