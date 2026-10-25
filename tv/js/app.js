// PhotoOnTv : navigation dans les dossiers de photos du NAS à la télécommande.
(function () {
  var TOUCHES = {
    gauche: 37, haut: 38, droite: 39, bas: 40, ok: 13, retour: 10009,
    lecturePause: 10252, lecture: 415, pause: 19, stop: 413, avance: 417, retourRapide: 412,
    chaineHaut: 427, chaineBas: 428
  };
  // Équivalents clavier pour les essais dans un navigateur
  var CLAVIER = { 8: TOUCHES.retour, 27: TOUCHES.retour, 32: TOUCHES.lecturePause, 33: TOUCHES.chaineHaut, 34: TOUCHES.chaineBas };

  var courant = null;      // réponse /api/folder affichée
  var positions = {};      // chemin -> index sélectionné, pour revenir au même endroit
  var demande = 0;         // numéro de la dernière ouverture de dossier
  var minuteriePrechargement = null, quitterAvant = 0;

  function $(id) { return document.getElementById(id); }

  var minuterieMessage = null;
  function message(texte) {
    var m = $('message');
    m.textContent = texte;
    m.classList.add('visible');
    clearTimeout(minuterieMessage);
    minuterieMessage = setTimeout(function () { m.classList.remove('visible'); }, 3000);
  }

  function pluriel(n, mot) { return n + ' ' + mot + (n > 1 ? 's' : ''); }

  function majEntete(d) {
    var morceaux = d.path ? d.path.split('/') : [];
    var fil = [CONFIG.racine || 'Photos'].concat(morceaux);
    if (fil.length > 4) fil = [fil[0], '…'].concat(fil.slice(-2));
    $('fil').innerHTML = '';
    fil.forEach(function (nom, i) {
      var s = document.createElement('span');
      s.textContent = nom;
      if (i === fil.length - 1) s.className = 'dernier';
      $('fil').appendChild(s);
    });
    var photos = 0, videos = 0;
    d.items.forEach(function (m) { if (m.type === 'video') videos++; else photos++; });
    var c = [];
    if (d.folders.length) c.push(pluriel(d.folders.length, 'dossier'));
    if (photos) c.push(pluriel(photos, 'photo'));
    if (videos) c.push(pluriel(videos, 'vidéo'));
    $('compteurs').textContent = c.join(' · ');
  }

  // Ouvre un dossier ; enfant = chemin du sous-dossier d'où l'on remonte (pour le resélectionner)
  function ouvrir(chemin, enfant) {
    var n = ++demande;
    var attente = setTimeout(function () { $('chargement').classList.add('visible'); }, 150);
    Api.dossier(chemin).then(function (d) {
      if (n !== demande) return;
      clearTimeout(attente);
      $('chargement').classList.remove('visible');
      if (courant) positions[courant.path] = Grille.index();
      var index = positions[chemin] || 0;
      if (enfant !== undefined) {
        d.folders.forEach(function (f, i) { if (f.path === enfant) index = i; });
      }
      courant = d;
      majEntete(d);
      var total = Grille.afficher(d, index);
      $('vide').textContent = 'Dossier vide';
      $('vide').classList.toggle('visible', !total);
    }, function (e) {
      if (n !== demande) return;
      clearTimeout(attente);
      $('chargement').classList.remove('visible');
      if (!courant) {
        $('vide').textContent = 'Serveur injoignable (' + CONFIG.serveur + ') — OK pour réessayer';
        $('vide').classList.add('visible');
      } else {
        message('Impossible d’ouvrir le dossier : ' + e.message);
      }
    });
  }

  function remonter() {
    if (courant.parent === null || courant.parent === undefined) {
      // À la racine : deux appuis rapprochés pour quitter
      var t = Date.now();
      if (t - quitterAvant < 2000) quitter();
      else { quitterAvant = t; message('Appuyez encore sur Retour pour quitter'); }
      return;
    }
    ouvrir(courant.parent, courant.path);
  }

  function valider() {
    var s = Grille.selection();
    if (!s) return;
    if (s.type === 'dossier') ouvrir(s.donnee.path);
    else Visionneuse.ouvrir(courant.items, s.rang, function (r) { Grille.placerMedia(r); }, message);
  }

  // Précharge le dossier survolé : l'ouverture devient instantanée
  Grille.surSelection = function (s) {
    clearTimeout(minuteriePrechargement);
    if (s && s.type === 'dossier' && !Api.enCache(s.donnee.path)) {
      minuteriePrechargement = setTimeout(function () { Api.prechargerDossier(s.donnee.path); }, 250);
    }
  };

  function quitter() {
    try { tizen.application.getCurrentApplication().exit(); } catch (e) {}
  }

  // ---------- Télécommande ----------
  document.addEventListener('keydown', function (ev) {
    var k = CLAVIER[ev.keyCode] || ev.keyCode, T = TOUCHES;
    if (Video.touche(k, T) || Visionneuse.touche(k, T)) { ev.preventDefault(); return; }
    if (!courant) {
      if (k === T.ok) ouvrir('');
      else if (k === T.retour) quitter();
      ev.preventDefault();
      return;
    }
    if (k === T.gauche) Grille.deplacer('gauche');
    else if (k === T.droite) Grille.deplacer('droite');
    else if (k === T.haut) Grille.deplacer('haut');
    else if (k === T.bas) Grille.deplacer('bas');
    else if (k === T.chaineHaut) Grille.page(-1);
    else if (k === T.chaineBas) Grille.page(1);
    else if (k === T.ok || k === T.lecture || k === T.lecturePause) valider();
    else if (k === T.retour) remonter();
    else return;
    ev.preventDefault();
  });

  try {
    ['MediaPlayPause', 'MediaPlay', 'MediaPause', 'MediaStop', 'MediaFastForward', 'MediaRewind',
     'ChannelUp', 'ChannelDown'].forEach(function (k) { tizen.tvinputdevice.registerKey(k); });
  } catch (e) {}

  ouvrir('');
})();
