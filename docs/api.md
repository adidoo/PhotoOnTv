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

`{"ok": true, "version": "0.1.0", "root": "/photos"}`

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
- `folders[].cover` : chemin d'un média servant de couverture (premier média du dossier, sinon du premier
  sous-dossier qui en a, en profondeur) ; `null` si le dossier ne contient aucun média.
- `type` : `photo` (jpg, jpeg, png, gif, tif, tiff, heic, heif, cr2, nef, arw, dng, webp) ou `video`
  (mp4, mov, m4v, 3gp, mts, m2ts, avi, mpg, mpeg, mkv, wmv). Les autres fichiers sont ignorés.
- `mtime` : secondes Unix. Sert aussi de version de cache (`&v=<mtime>`).
- 404 si le dossier n'existe pas.

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
- Source : vignettes QuMagie existantes (`.@__thumb/`), sinon cache généré par le serveur.
- **404** si aucune vignette n'est (encore) disponible : l'appli affiche une icône générique (photo ou
  vidéo) et ne réessaie pas en boucle.

## `GET /api/file/<chemin média>` (ou `GET /api/file?path=<chemin média>`)

Fichier original, tel quel. Dans la première forme, chaque segment du chemin est encodé séparément
(`encodeURIComponent`, les `/` restent des `/`) : l'URL se termine par le nom du fichier, ce dont le
lecteur natif de la télé (AVPlay) a besoin pour reconnaître le format. L'appli utilise cette forme.

- Prend en charge `Range` (206) → lecture vidéo et reprise.
- `Content-Type` selon l'extension (`video/mp4`, `video/quicktime`, `video/mp2t`, `image/jpeg`…).
- Usage : vidéos (lecture directe par la télé) et photo pleine résolution chargée en fond après la `l`.
  L'appli ne demande l'original **que pour les jpg/jpeg/png/gif/webp** (la télé ne décode pas HEIC/CR2/TIF :
  pour ceux-là, elle reste sur la vignette `l`).
