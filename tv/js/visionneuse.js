// Visionneuse plein écran : vignette 1200 px tout de suite, original en fond si la télé sait le décoder
// et si l'on reste sur la photo. Les voisines (±2) sont préchargées.
var Visionneuse = (function () {
  var DELAI_ORIGINAL = 600, VOISINES = 2;
  var visio = document.getElementById('visio');
  var image = document.getElementById('v-image');
  var info = document.getElementById('v-info');
  var liste = [], rang = 0, jeton = 0, surFermeture = null, surMessage = null;
  var ouverte = false, minuterieInfo = null, minuterieOriginal = null;
  var cache = {};   // url -> { img, etat: 'charge' | 'ok' | 'ko', attentes: [] }

  // Charge une image (une seule fois par url) ; ok(url) ou ko()
  function charger(url, ok, ko) {
    var c = cache[url];
    if (Api.estAbsente(url)) { ko(); return; }
    if (c && c.etat === 'ok') { ok(url); return; }
    if (c && c.etat === 'ko') { ko(); return; }
    if (!c) {
      c = cache[url] = { img: new Image(), etat: 'charge', attentes: [] };
      c.img.onload = function () { c.etat = 'ok'; c.attentes.forEach(function (a) { a[0](url); }); c.attentes = []; };
      c.img.onerror = function () {
        c.etat = 'ko';
        Api.marquerAbsente(url);
        c.attentes.forEach(function (a) { a[1](); });
        c.attentes = [];
      };
      c.img.src = url;
    }
    c.attentes.push([ok, ko]);
  }

  // Sources possibles d'un média, de la plus rapide à la plus lourde
  function sources(m) {
    var s = [Api.vignette(m.path, 'l', m.mtime), Api.vignette(m.path, 'm', m.mtime)];
    if (m.type === 'photo' && Api.originalLisible(m.path)) s.push(Api.fichier(m.path));
    return s;
  }

  function essayer(liste, j, fini) {
    if (!liste.length) { fini(null); return; }
    charger(liste[0], function (u) { fini(u); }, function () {
      if (j === jeton) essayer(liste.slice(1), j, fini);
    });
  }

  function montrerInfo() {
    info.classList.add('visible');
    clearTimeout(minuterieInfo);
    minuterieInfo = setTimeout(function () { info.classList.remove('visible'); }, 2500);
  }

  function precharger() {
    var garder = {};
    for (var d = -VOISINES; d <= VOISINES; d++) {
      var m = liste[rang + d];
      if (!m) continue;
      var u = Api.vignette(m.path, 'l', m.mtime);
      garder[u] = true;
      if (d) charger(u, function () {}, function () {});
    }
    // Oublie les images éloignées (mémoire de la télé)
    Object.keys(cache).forEach(function (u) {
      if (!garder[u] && cache[u].etat !== 'charge') delete cache[u];
    });
  }

  function afficher(i) {
    rang = i;
    var m = liste[i], j = ++jeton;
    document.getElementById('v-nom').textContent = m.name;
    document.getElementById('v-position').textContent = (i + 1) + ' / ' + liste.length;
    visio.classList.toggle('est-video', m.type === 'video');
    visio.classList.remove('absent');
    image.classList.add('attente');
    montrerInfo();
    clearTimeout(minuterieOriginal);

    essayer(sources(m), j, function (u) {
      if (j !== jeton) return;
      if (!u) { visio.classList.add('absent'); image.removeAttribute('src'); return; }
      image.src = u;
      image.classList.remove('attente');
      // Original pleine résolution si l'on reste sur la photo
      var orig = Api.fichier(m.path);
      if (m.type === 'photo' && Api.originalLisible(m.path) && u !== orig) {
        minuterieOriginal = setTimeout(function () {
          var o = new Image();
          o.onload = function () { if (j === jeton) image.src = orig; };
          o.src = orig;
        }, DELAI_ORIGINAL);
      }
    });
    precharger();
  }

  function lireVideo() {
    var m = liste[rang];
    Video.lire(Api.fichier(m.path), m.name, null, function () {
      if (surMessage) surMessage('Format non lu par la télé : ' + m.name);
    });
  }

  function fermer() {
    ouverte = false;
    ++jeton;
    clearTimeout(minuterieOriginal);
    visio.classList.remove('ouvert');
    image.removeAttribute('src');
    cache = {};
    if (surFermeture) surFermeture(rang);
  }

  return {
    // medias = items du dossier ; i = rang de départ ; fin(rang) appelé à la fermeture
    ouvrir: function (medias, i, fin, message) {
      liste = medias; surFermeture = fin; surMessage = message; ouverte = true;
      visio.classList.add('ouvert');
      afficher(i);
      if (medias[i].type === 'video') lireVideo();
    },
    ouverte: function () { return ouverte; },
    touche: function (k, T) {
      if (!ouverte) return false;
      if (k === T.gauche) { if (rang > 0) afficher(rang - 1); }
      else if (k === T.droite) { if (rang < liste.length - 1) afficher(rang + 1); }
      else if (k === T.ok || k === T.lecture || k === T.lecturePause) {
        if (liste[rang].type === 'video') lireVideo(); else montrerInfo();
      }
      else if (k === T.retour || k === T.stop) fermer();
      else montrerInfo();
      return true;
    }
  };
})();
