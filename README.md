# PhotoOnTv

Parcourir très vite les photos et vidéos d'un NAS sur une télé Samsung (Tizen), dossier par dossier,
à la télécommande.

- **[`server/`](server/)** : petit serveur Go à installer sur le NAS (Docker / Container Station). Il
  réutilise les vignettes déjà générées par QNAP (QuMagie) au lieu d'en calculer, garde la liste des
  dossiers en mémoire et sert les vidéos telles quelles.
- **[`tv/`](tv/)** : appli web Tizen (grille de dossiers et de photos, plein écran avec préchargement,
  lecture vidéo par le lecteur natif de la télé).
- **[`docs/api.md`](docs/api.md)** : le contrat entre les deux.

La fluidité vient surtout du serveur : vignettes de 150 / 600 / 1200 px au lieu des originaux, listes
JSON déjà prêtes, cache HTTP long, et préchargement côté télé.

## Démarrage rapide

1. Installer le serveur sur le NAS : voir [`server/README.md`](server/README.md).
2. Copier `tv/js/config.example.js` en `tv/js/config.js` et y mettre l'adresse du serveur.
3. Copier `deployer.local.example.ps1` en `deployer.local.ps1` (IP de la télé, cible, profil de
   certificat Samsung), puis `.\deployer.ps1` : voir [`tv/README.md`](tv/README.md).

Réseau local uniquement : le serveur n'a pas d'authentification.

## État

- Vignettes : servies quand QNAP les a déjà générées ; les autres (HEIC, beaucoup de vidéos) affichent
  une icône en attendant la génération par le serveur.
- Lecture vidéo sur la télé : à valider selon les formats.
