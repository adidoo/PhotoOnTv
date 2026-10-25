# PhotoOnTv — appli TV (Tizen)

Appli web pour les télés Samsung (Tizen 6 et plus, testée sur Tizen 9), pilotée à la télécommande.

- **Grille** : sous-dossiers puis photos et vidéos, vignettes 600 px. Seules les lignes proches de l'écran
  existent en mémoire (des dossiers ont des milliers de photos). Le dossier survolé est préchargé, la
  position est mémorisée quand on remonte.
- **Plein écran** : vignette 1200 px immédiate, puis l'original (JPEG, PNG, GIF, WebP seulement) si l'on
  reste sur la photo ; les deux voisines de chaque côté sont préchargées.
- **Vidéo** : lecteur natif AVPlay (sur la télé) ou `<video>` (navigateur), rotation et ratio réglés
  d'après `/api/info`.
- **Boutons de l'en-tête** (↑ depuis la première ligne) :
  - **Ordre** : inverse l'ordre des dossiers et des photos dans toutes les galeries (mémorisé) ;
  - **Diaporama** : photos du dossier et de tous ses sous-dossiers, parcourus un à un dans l'ordre
    d'affichage, à partir de la photo sélectionnée ;
  - **Aléatoire** : photos du dossier et de tous ses sous-dossiers, mélangées (à la racine : toute la
    photothèque).

  Le diaporama ignore les vidéos, n'affiche rien par-dessus les photos et boucle ; en sortant, on
  arrive dans le dossier de la photo affichée, sur cette photo. Le délai (5 à 60 s) est mémorisé.

## Touches

| Touche | Grille | Plein écran | Vidéo | Diaporama |
|---|---|---|---|---|
| ← → | déplacement | photo précédente / suivante | −10 s / +10 s | photo précédente / suivante |
| ↑ ↓ | déplacement (↑ en haut : boutons) | | | délai +5 s / −5 s |
| OK | ouvrir | lire la vidéo | pause / lecture | pause / reprise |
| Retour | dossier parent (2 fois à la racine : quitter) | grille | arrêt | sortie sur la photo affichée |
| CH+ / CH− | page précédente / suivante | | | |
| ⏯ | diaporama du dossier sélectionné, ou à partir de la photo | | pause / lecture | pause / reprise |
| ⏹ ⏩ ⏪ | | | arrêt, ±10 s | sortie, suivante, précédente |

Dans un navigateur : Retour arrière ou Échap = Retour, Page préc./suiv. = CH±, Espace = ⏯.

## Configuration et déploiement

1. `tv/js/config.js` à partir de [`config.example.js`](js/config.example.js) (adresse du serveur).
2. `deployer.local.ps1` à la racine à partir de `deployer.local.example.ps1` (IP de la télé en mode
   développeur, nom de cible, profil de certificat Samsung).
3. `.\deployer.ps1` (ou `.\deployer.ps1 -Inspecteur` pour l'inspecteur Chrome).

Pièges Tizen : l'ID de paquet fait exactement 10 caractères ; aucun script ni style inline (refusés en
silence par la CSP) ; pas d'espace dans le nom du `.wgt`.

## Tester sans télé ni NAS

```powershell
python tv\outils\faux_serveur.py <dossier de photos> --appli tv --port 8099
```

puis ouvrir `http://127.0.0.1:8099/appli/index.html` dans Chrome (l'appli servie ainsi parle au faux
serveur). Le faux serveur suit [`docs/api.md`](../docs/api.md) et calcule les vignettes avec Pillow.
