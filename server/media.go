package main

import (
	"path"
	"strings"
	"unicode"
)

var photoExt = map[string]bool{
	"jpg": true, "jpeg": true, "png": true, "gif": true, "tif": true, "tiff": true, "webp": true,
	"heic": true, "heif": true, "cr2": true, "nef": true, "arw": true, "dng": true,
}

var videoExt = map[string]bool{
	"mp4": true, "mov": true, "m4v": true, "3gp": true, "mts": true, "m2ts": true,
	"avi": true, "mpg": true, "mpeg": true, "mkv": true, "wmv": true,
}

// Types MIME servis par /api/file ; les autres extensions partent en application/octet-stream.
var mimeTypes = map[string]string{
	"jpg": "image/jpeg", "jpeg": "image/jpeg", "png": "image/png", "gif": "image/gif",
	"webp": "image/webp", "tif": "image/tiff", "tiff": "image/tiff",
	"heic": "image/heic", "heif": "image/heif",
	"mp4": "video/mp4", "m4v": "video/mp4", "mov": "video/quicktime", "3gp": "video/3gpp",
	"mts": "video/mp2t", "m2ts": "video/mp2t", "avi": "video/x-msvideo",
	"mpg": "video/mpeg", "mpeg": "video/mpeg", "mkv": "video/x-matroska", "wmv": "video/x-ms-wmv",
}

func ext(nom string) string {
	return strings.ToLower(strings.TrimPrefix(path.Ext(nom), "."))
}

// mediaType renvoie "photo", "video" ou "" pour un fichier à ignorer.
func mediaType(nom string) string {
	e := ext(nom)
	switch {
	case photoExt[e]:
		return "photo"
	case videoExt[e]:
		return "video"
	}
	return ""
}

// naturalLess compare deux noms dans l'ordre « naturel », insensible à la casse : « 2 » avant « 10 ».
func naturalLess(a, b string) bool {
	ra, rb := []rune(strings.ToLower(a)), []rune(strings.ToLower(b))
	i, j := 0, 0
	for i < len(ra) && j < len(rb) {
		if unicode.IsDigit(ra[i]) && unicode.IsDigit(rb[j]) {
			si, sj := i, j
			for i < len(ra) && unicode.IsDigit(ra[i]) {
				i++
			}
			for j < len(rb) && unicode.IsDigit(rb[j]) {
				j++
			}
			na := strings.TrimLeft(string(ra[si:i]), "0")
			nb := strings.TrimLeft(string(rb[sj:j]), "0")
			if len(na) != len(nb) {
				return len(na) < len(nb)
			}
			if na != nb {
				return na < nb
			}
			continue
		}
		if ra[i] != rb[j] {
			return ra[i] < rb[j]
		}
		i++
		j++
	}
	if len(ra)-i != len(rb)-j {
		return len(ra)-i < len(rb)-j
	}
	return a < b
}
