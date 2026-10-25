# Serveur PhotoOnTv

Petit serveur Go, sans dépendance, qui expose un dossier de photos en lecture seule à l'appli TV
(contrat : [`../docs/api.md`](../docs/api.md)).

- **Vignettes** : réutilise celles que QNAP (QuMagie / Multimedia Console) a déjà générées dans les
  dossiers `.@__thumb/`. Celles qui manquent (HEIC, beaucoup de vidéos…) sont générées avec vips
  (+ libheif) et ffmpeg dans un cache à part (`CACHE_DIR`), jamais dans le dossier des photos :
  - **à la demande** de l'appli, en priorité (et pour tout dossier qu'elle ouvre) ;
  - par un **balayage de fond** à priorité CPU/disque minimale (`nice 19`, `ionice -c3`, un seul fil),
    qui reprend où il en était après un redémarrage (le cache fait foi) et repasse toutes les 6 h ;
  - HEIC : `s` et `m` tirées de la miniature intégrée (~1 s), `l` du décodage complet (~5 s sur un
    Celeron N3060) ; vidéos : image vers 10 % de la durée (1 s à la demande), rotation appliquée ;
  - échecs (fichier corrompu…) mémorisés dans le cache (`*.fail`), pas de nouvel essai. Supprimer le
    dossier de cache relance tout.
- **Infos vidéo** (`/api/info`) : dimensions, rotation, ratio d'affichage, codecs, par ffprobe.
- **Dossiers** : listes JSON mises en cache en mémoire, cache préchauffé au démarrage par un parcours
  complet de l'arborescence.
- **Vidéos et originaux** : servis tels quels, avec `Range`.

## Configuration (variables d'environnement)

| Variable | Défaut | Rôle |
|---|---|---|
| `PHOTOS_ROOT` | `/photos` | racine des photos |
| `LISTEN` | `:8080` | adresse d'écoute |
| `EXCLUDE` | vide | noms de dossiers à masquer, séparés par des virgules (ceux qui commencent par `.` ou `@` le sont toujours) |
| `CACHE_DIR` | `/cache` (image Docker) | vignettes générées ; vide = pas de génération |

Le dossier monté sur `/cache` doit être inscriptible par l'utilisateur du conteneur (uid 65534), par
exemple `mkdir cache && chmod 777 cache`. Compter quelques centaines de Mo pour ~10 000 médias.
Suivi : `curl http://<nas>:8080/api/status` et `docker logs photoontv` (progression toutes les 250).

## Installation sur un QNAP (Container Station)

```sh
# sur le NAS, en SSH
mkdir -p /share/Container/photoontv/cache && cd /share/Container/photoontv && chmod 777 cache
# copier ici le contenu de server/ (dans src/) et docker-compose.example.yml → docker-compose.yml
# (adapter le chemin des photos et « build: ./src »)
docker compose up -d --build
curl http://localhost:8080/api/health
```

Sous QTS, le binaire `docker` est dans `/share/CACHEDEV1_DATA/.qpkg/container-station/bin/` ; si
`docker` se plaint de ne pas pouvoir créer son dossier de configuration, définir
`DOCKER_CONFIG=<un dossier inscriptible>`.

Pas d'authentification : à n'utiliser que sur le réseau local, sans redirection de port.
