# API du serveur PhotoOnTv

Contrat entre le serveur (`server/`) et l'appli TV (`tv/`). Toute évolution passe d'abord ici.

## Généralités

- Base : `http://<nas>:<port>` (port par défaut **8080**). Pas d'authentification : réseau local uniquement.
- **Chemins** : relatifs à la racine des photos, séparés par `/`, sans `/` initial ; `""` = racine.
  Toujours encodés en paramètre de requête (`encodeURIComponent`). Le serveur refuse tout chemin qui
  sort de la racine (`..`) avec **400**.
- **CORS** : `Access-Control-Allow-Origin: *` sur toutes les réponses (l'appli Tizen tourne en `file://`).
- Dossiers ignorés : configurables côté serveur (par défaut `@Recycle`, `@Recently-Snapshot`, `.streams`,
  `.@__thumb` et tout nom commençant par `.` ou `@`). Ils n'apparaissent nulle part.
- Tri : ordre « naturel » insensible à la casse (`2 < 10`), croissant. L'appli peut inverser.
- Erreurs : statut HTTP + corps JSON `{"error": "message"}`.

## `GET /api/health`

`{"ok": true, "version": "0.2.0", "root": "/photos"}`

## `GET /api/status`

État de la génération des vignettes (information, pour le suivi) :

```json
{"generation": true, "scanning": true, "todo": 7345, "done": 71, "failed": 0,
 "on_demand": 16, "queue": 0, "current": "2013/2013-05 - Divers/DSCN2831.MOV", "scan_start": 1792920967}
```

`todo` / `done` : médias à générer / faits dans le balayage en cours ; `on_demand` : générés à la
demande de l'appli depuis le démarrage ; `queue` : file prioritaire. `{"generation": false}` si le
serveur n'a pas de cache (`CACHE_DIR` vide).

## `GET /api/folder?path=<chemin>`

Contenu **direct** d'un dossier (pas de récursion).

```json
{
  "path": "2015/2015-01-01 - Naissance",
  "name": "2015-01-01 - Naissance",
  "parent": "2015",
  "folders": [
    {
      "name": "Sous-dossier",
      "path": "2015/2015-01-01 - Naissance/Sous-dossier",
      "folders": 0,
      "items": 42,
      "cover": "2015/2015-01-01 - Naissance/Sous-dossier/01.jpg"
    }
  ],
  "items": [
    {
      "name": "02.jpg",
      "path": "2015/2015-01-01 - Naissance/02.jpg",
      "type": "photo",
      "size": 1131276,
      "mtime": 1486771200
    },
    {
      "name": "VID_0001.mp4",
      "path": "2015/2015-01-01 - Naissance/VID_0001.mp4",
      "type": "video",
      "size": 52428800,
      "mtime": 1486771200
    }
  ]
}
```

- `parent` : `null` à la racine.
- `folders[].folders` / `folders[].items` : nombre de sous-dossiers / de médias **directs** du sous-dossier.
- `folders[].cover` : chemin d'un média servant de couverture : le premier média du dossier qui a une
  vignette (QNAP ou déjà générée), sinon la première photo, sinon le premier média ; si le dossier n'a
  aucun média direct, même règle dans ses sous-dossiers, en profondeur ; `null` s'il n'y a aucun média.
- `type` : `photo` (jpg, jpeg, png, gif, tif, tiff, heic, heif, cr2, nef, arw, dng, webp) ou `video`
  (mp4, mov, m4v, 3gp, mts, m2ts, avi, mpg, mpeg, mkv, wmv). Les autres fichiers sont ignorés.
- `mtime` : secondes Unix. Sert aussi de version de cache (`&v=<mtime>`).
- 404 si le dossier n'existe pas.

## `GET /api/photos?path=<dossier>&order=asc|desc|random[&seed=<n>][&offset=<i>][&n=<taille>][&start=<photo>]`

Photos (pas de vidéos) d'un dossier **et de tous ses sous-dossiers**, par paquets, pour le diaporama.

```json
{"path": "2015", "total": 8123, "offset": 0,
 "items": [{"name": "02.jpg", "path": "2015/…/02.jpg", "type": "photo", "size": 1131276, "mtime": 1486771200}]}
```

- `order=asc` : ordre de la grille, en profondeur — chaque sous-dossier entièrement (récursivement),
  puis les photos du dossier lui-même. `desc` : tri inversé à chaque niveau (même parcours que la
  grille en ordre inversé). `random` : toutes les photos mélangées ; même `seed` = même mélange, pour
  que les paquets successifs d'un diaporama se suivent.
- `offset` : rang du premier élément du paquet (défaut 0) ; `n` : taille du paquet (défaut 200,
  max 1000) ; `total` : nombre total de photos. Le paquet suivant commence à `offset + n`.
- `start` (asc/desc) : chemin d'une photo ; le paquet commence à cette photo (l'`offset` renvoyé
  indique sa position).
- La liste complète est gardée ~10 min côté serveur.

## `GET /api/thumb?path=<chemin média>&size=s|m|l[&v=<mtime>]`

Vignette JPEG d'une photo ou d'une vidéo.

| `size` | Côté court (côté long en 3:2) | Usage |
|---|---|---|
| `s` | ~100 px (150) | mosaïque dense, couvertures de dossiers |
| `m` | ~400 px (600) | grille |
| `l` | ~800 px (1200) | plein écran immédiat |

Les tailles réelles dépendent de la source (vignettes QNAP, ou `<nom>.jpg` de 720 px pour certaines
vidéos) : l'appli doit afficher l'image en `object-fit`, sans supposer ses dimensions.

- Réponse `image/jpeg`, `Cache-Control: public, max-age=31536000, immutable`, `ETag`.
- Source, dans l'ordre : vignette QNAP de la bonne taille (`.@__thumb/`), vignette générée par le
  serveur (cache à part), vignette QNAP d'une autre taille.
- Média sans vignette : le serveur le génère **à la demande, en priorité** sur le balayage de fond, et
  attend jusqu'à ~4 s (photo) ou ~8 s (vidéo) avant de répondre. Ouvrir un dossier (`/api/folder`)
  met aussi ses médias sans vignette en tête de file. Ordres de grandeur sur un petit NAS : `s`/`m`
  d'une HEIC 1 à 3 s (miniature intégrée), `l` d'une HEIC ~5 s, vidéo ~3 s.
- `l` d'une HEIC pas encore prête : le serveur peut renvoyer provisoirement la `m`, avec
  `Cache-Control: public, max-age=60` (pas `immutable`) pour que la vraie `l` la remplace ensuite.
- **404** si aucune vignette n'est disponible dans le délai (ou fichier illisible) : l'appli affiche une
  icône générique (photo ou vidéo) et ne réessaie pas en boucle.

## `GET /api/file/<chemin média>` (ou `GET /api/file?path=<chemin média>`)

Fichier original, tel quel. Dans la première forme, chaque segment du chemin est encodé séparément
(`encodeURIComponent`, les `/` restent des `/`) : l'URL se termine par le nom du fichier, ce dont le
lecteur natif de la télé (AVPlay) a besoin pour reconnaître le format. L'appli utilise cette forme.

- Prend en charge `Range` (206) → lecture vidéo et reprise.
- `Content-Type` selon l'extension (`video/mp4`, `video/quicktime`, `video/mp2t`, `image/jpeg`…).
- Usage : vidéos (lecture directe par la télé) et photo pleine résolution chargée en fond après la `l`.
  L'appli ne demande l'original **que pour les jpg/jpeg/png/gif/webp** (la télé ne décode pas HEIC/CR2/TIF :
  pour ceux-là, elle reste sur la vignette `l`).

## `GET /api/info?path=<chemin vidéo>`

Caractéristiques d'une vidéo (ffprobe), pour régler la rotation et le ratio du lecteur :

```json
{"width": 1920, "height": 1080, "rotation": 90, "dar": "16:9", "dar_value": 1.778,
 "duration": 24.95, "vcodec": "h264", "acodec": "aac", "container": "mov,mp4,m4a,3gp,3g2,mj2"}
```

- `width` / `height` : dimensions codées (avant rotation).
- `rotation` : 0, 90, 180 ou 270, sens horaire, à appliquer à l'affichage (téléphone tenu en portrait :
  90).
- `dar` : ratio d'affichage tenant compte des pixels non carrés (une vidéo AVCHD 1440×1080 donne
  `16:9`) ; chaîne `"w:h"` quand le fichier l'indique, sinon la valeur décimale en chaîne ;
  `dar_value` : même ratio en nombre.
- `duration` en secondes ; `vcodec` / `acodec` / `container` : noms ffmpeg.
- 400 si le chemin n'est pas une vidéo, 422 si ffprobe ne lit pas le fichier (délai 5 s). Résultats
  gardés en mémoire côté serveur.
