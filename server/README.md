# Serveur PhotoOnTv

Petit serveur Go, sans dépendance, qui expose un dossier de photos en lecture seule à l'appli TV
(contrat : [`../docs/api.md`](../docs/api.md)).

- **Vignettes** : réutilise celles que QNAP (QuMagie / Multimedia Console) a déjà générées dans les
  dossiers `.@__thumb/` → aucun calcul d'image sur le NAS. Les médias sans vignette renvoient 404
  (génération prévue plus tard).
- **Dossiers** : listes JSON mises en cache en mémoire, cache préchauffé au démarrage par un parcours
  complet de l'arborescence.
- **Vidéos et originaux** : servis tels quels, avec `Range`.

## Configuration (variables d'environnement)

| Variable | Défaut | Rôle |
|---|---|---|
| `PHOTOS_ROOT` | `/photos` | racine des photos |
| `LISTEN` | `:8080` | adresse d'écoute |
| `EXCLUDE` | vide | noms de dossiers à masquer, séparés par des virgules (ceux qui commencent par `.` ou `@` le sont toujours) |

## Installation sur un QNAP (Container Station)

```sh
# sur le NAS, en SSH
mkdir -p /share/Container/photoontv && cd /share/Container/photoontv
# copier ici le contenu de server/ (dans src/) et docker-compose.example.yml → docker-compose.yml
# (adapter le chemin des photos et « build: ./src »)
docker compose up -d --build
curl http://localhost:8080/api/health
```

Sous QTS, le binaire `docker` est dans `/share/CACHEDEV1_DATA/.qpkg/container-station/bin/` ; si
`docker` se plaint de ne pas pouvoir créer son dossier de configuration, définir
`DOCKER_CONFIG=<un dossier inscriptible>`.

Pas d'authentification : à n'utiliser que sur le réseau local, sans redirection de port.
