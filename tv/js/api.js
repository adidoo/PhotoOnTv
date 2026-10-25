// Accès au serveur PhotoOnTv (contrat : docs/api.md). Les dossiers déjà lus restent en mémoire.
var Api = (function () {
  var dossiers = {};   // chemin -> réponse /api/folder
  var enCours = {};    // chemin -> Promise en vol
  var sansVignette = {};  // url -> true : vignette absente (404), on ne redemande pas

  function url(route, chemin, extra) {
    return CONFIG.serveur + route + '?path=' + encodeURIComponent(chemin) + (extra || '');
  }

  function dossier(chemin) {
    if (dossiers[chemin]) return Promise.resolve(dossiers[chemin]);
    if (enCours[chemin]) return enCours[chemin];
    var p = fetch(url('/api/folder', chemin)).then(function (r) {
      if (!r.ok) return r.json().catch(function () { return {}; }).then(function (j) {
        throw new Error(j.error || ('HTTP ' + r.status));
      });
      return r.json();
    }).then(function (d) {
      dossiers[chemin] = d;
      delete enCours[chemin];
      return d;
    }, function (e) {
      delete enCours[chemin];
      throw e;
    });
    enCours[chemin] = p;
    return p;
  }

  // À incrémenter quand des vignettes déjà servies changent côté serveur (elles sont en cache un an
  // sur la télé) : 2 = vidéos anamorphiques (MTS) corrigées
  var VERSION_VIGNETTES = 2;

  function vignette(chemin, taille, mtime) {
    return url('/api/thumb', chemin, '&size=' + taille + (mtime ? '&v=' + mtime : '') + '&g=' + VERSION_VIGNETTES);
  }

  // Formats que la télé décode elle-même : seuls ceux-là sont demandés en original
  var ORIGINAL_LISIBLE = /\.(jpe?g|png|gif|webp)$/i;

  return {
    dossier: dossier,
    // Précharge sans attendre ni signaler d'erreur (dossier survolé)
    prechargerDossier: function (chemin) { dossier(chemin).catch(function () {}); },
    enCache: function (chemin) { return dossiers[chemin] || null; },
    vignette: vignette,
    // Forme « /api/file/<chemin> » : l'URL se termine par le nom du fichier, comme l'attend AVPlay
    fichier: function (chemin) {
      return CONFIG.serveur + '/api/file/' + chemin.split('/').map(encodeURIComponent).join('/');
    },
    // Dimensions, rotation et ratio d'une vidéo (ffprobe côté serveur) ; null si indisponible
    info: function (chemin) {
      return fetch(url('/api/info', chemin)).then(function (r) {
        return r.ok ? r.json() : null;
      }).catch(function () { return null; });
    },
    originalLisible: function (chemin) { return ORIGINAL_LISIBLE.test(chemin); },
    marquerAbsente: function (u) { sansVignette[u] = true; },
    estAbsente: function (u) { return !!sansVignette[u]; }
  };
})();
