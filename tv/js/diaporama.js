// Diaporama plein écran des photos d'un dossier et de ses sous-dossiers (vidéos ignorées). Rien en
// surimpression, sauf 5 s d'indication après une pause / reprise ou un changement de délai.
// ↑ / ↓ : délai +/- 5 s ; ← / → : photo précédente / suivante ; OK : pause / reprise ;
// Retour : sortie sur la photo affichée.
// Les photos arrivent du serveur par paquets (/api/photos) : la racine en compte plus de 100 000.
var Diaporama = (function () {
  var PAS = 5, MIN = 5, MAX = 60, PAQUET = 200, DUREE_INFO = 5000;
  var ecran = document.getElementById('diapo');
  var info = document.getElementById('d-info'), minuterieInfo = null;
  var calques = [document.getElementById('d-a'), document.getElementById('d-b')];
  var visible = 0;                 // calque affiché
  var ouvert = false, pause = false, minuterie = null, jeton = 0;
  var dossier = '', ordre = 'asc', graine, total = 0, rang = 0;
  var paquets = {};                // numéro de paquet -> Promise des items
  var prets = {};                  // rang -> image préparée { etat, url, photo, attentes }
  var surFin = null, echecsDeSuite = 0;
  var delai = 10;
  try { delai = parseInt(localStorage.getItem('delaiDiaporama'), 10) || 10; } catch (e) {}

  // Affiche l'état (lecture / pause) et le délai pendant 5 s
  function indiquer() {
    info.classList.toggle('pause', pause);
    info.querySelector('.texte').textContent = (pause ? 'Pause' : 'Lecture') + ' · ' + delai + ' s';
    info.classList.add('visible');
    clearTimeout(minuterieInfo);
    minuterieInfo = setTimeout(function () { info.classList.remove('visible'); }, DUREE_INFO);
  }

  function sauver() { try { localStorage.setItem('delaiDiaporama', String(delai)); } catch (e) {} }

  function photo(i) {
    var p = Math.floor(i / PAQUET);
    if (!paquets[p]) {
      paquets[p] = Api.photos(dossier, { order: ordre, seed: graine, offset: p * PAQUET, n: PAQUET })
        .then(function (r) { return r.items; }, function (e) { delete paquets[p]; throw e; });
    }
    return paquets[p].then(function (items) { return items[i - p * PAQUET] || null; });
  }

  function charger(url, ok, ko) {
    var img = new Image();
    img.onload = function () { ok(url); };
    img.onerror = ko;
    img.src = url;
  }

  // Prépare l'image du rang i : l'original si la télé le décode, sinon (ou en cas d'échec) la
  // vignette 1200 px, puis la 600 px
  function preparer(i) {
    if (prets[i]) return prets[i];
    var e = prets[i] = { etat: 'charge', url: null, photo: null, attentes: [] };
    function fini(etat, url) {
      e.etat = etat; e.url = url;
      e.attentes.forEach(function (f) { f(); });
      e.attentes = [];
    }
    photo(i).then(function (m) {
      if (!m) { fini('ko'); return; }
      e.photo = m;
      var urls = [Api.vignette(m.path, 'l', m.mtime), Api.vignette(m.path, 'm', m.mtime)];
      if (Api.originalLisible(m.path)) urls.unshift(Api.fichier(m.path));
      (function essayer(k) {
        if (k >= urls.length) { fini('ko'); return; }
        charger(urls[k], function (u) { fini('ok', u); }, function () { essayer(k + 1); });
      })(0);
    }, function () { fini('ko'); });
    return e;
  }

  function quandPret(e, f) { if (e.etat === 'charge') e.attentes.push(f); else f(); }

  // Oublie les images éloignées (mémoire de la télé)
  function nettoyer() {
    Object.keys(prets).forEach(function (k) {
      var d = k - rang;
      if (d < -1 || d > 2) delete prets[k];
    });
  }

  function programmer() {
    clearTimeout(minuterie);
    if (!pause && ouvert) minuterie = setTimeout(function () { aller(rang + 1); }, delai * 1000);
  }

  function aller(i) {
    if (!ouvert || !total) return;
    i = ((i % total) + total) % total;   // le diaporama boucle
    rang = i;
    clearTimeout(minuterie);
    var j = ++jeton, e = preparer(i);
    quandPret(e, function () {
      if (j !== jeton || !ouvert) return;
      if (e.etat !== 'ok') {
        // Photo illisible : on passe à la suivante, sans boucler indéfiniment si tout échoue
        if (++echecsDeSuite < 20) aller(i + 1);
        return;
      }
      echecsDeSuite = 0;
      var autre = 1 - visible;
      calques[autre].src = e.url;
      calques[autre].classList.add('visible');
      calques[visible].classList.remove('visible');
      visible = autre;
      programmer();
      nettoyer();
      preparer((i + 1) % total);
    });
  }

  function ecranDeVeille(actif) {
    try {
      var etats = webapis.appcommon.AppCommonScreenSaverState;
      webapis.appcommon.setScreenSaver(actif ? etats.SCREEN_SAVER_ON : etats.SCREEN_SAVER_OFF);
    } catch (e) {}
  }

  function fermer() {
    if (!ouvert) return;
    ouvert = false;
    ++jeton;
    clearTimeout(minuterie);
    clearTimeout(minuterieInfo);
    info.classList.remove('visible');
    ecran.classList.remove('ouvert');
    calques.forEach(function (c) { c.classList.remove('visible'); c.removeAttribute('src'); });
    ecranDeVeille(true);
    var e = prets[rang], m = e && e.photo;
    prets = {}; paquets = {};
    if (surFin) surFin(m || null);
  }

  return {
    // chemin = dossier ; o = { order: 'asc' | 'desc' | 'random', start: photo de départ (asc/desc) } ;
    // fin(photo) à la sortie, avec la photo affichée (ou null). Promise : false s'il n'y a aucune photo.
    lancer: function (chemin, o, fin) {
      dossier = chemin; ordre = o.order; surFin = fin; pause = false; echecsDeSuite = 0;
      graine = ordre === 'random' ? Math.floor(Math.random() * 1e9) : undefined;
      paquets = {}; prets = {}; total = 0;
      // Requête d'un seul élément : nombre total et position de la photo de départ
      return Api.photos(chemin, { order: ordre, seed: graine, n: 1, start: o.start }).then(function (r) {
        if (!r.total) return false;
        total = r.total;
        ouvert = true;
        visible = 0;
        ecran.classList.add('ouvert');
        ecranDeVeille(false);
        aller(r.offset);
        return true;
      });
    },
    ouvert: function () { return ouvert; },
    touche: function (k, T) {
      if (!ouvert) return false;
      if (k === T.haut) { delai = Math.min(MAX, delai + PAS); sauver(); programmer(); indiquer(); }
      else if (k === T.bas) { delai = Math.max(MIN, delai - PAS); sauver(); programmer(); indiquer(); }
      else if (k === T.droite || k === T.avance) aller(rang + 1);
      else if (k === T.gauche || k === T.retourRapide) aller(rang - 1);
      else if (k === T.ok || k === T.lecturePause) { pause = !pause; programmer(); indiquer(); }
      else if (k === T.lecture) { pause = false; programmer(); indiquer(); }
      else if (k === T.pause) { pause = true; programmer(); indiquer(); }
      else if (k === T.retour || k === T.stop) fermer();
      return true;
    }
  };
})();
