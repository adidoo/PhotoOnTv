// PhotoOnTv : navigation dans les dossiers de photos du NAS à la télécommande.
(function () {
  var TOUCHES = {
    gauche: 37, haut: 38, droite: 39, bas: 40, ok: 13, retour: 10009,
    lecturePause: 10252, lecture: 415, pause: 19, stop: 413, avance: 417, retourRapide: 412,
    chaineHaut: 427, chaineBas: 428
  };
  // Équivalents clavier pour les essais dans un navigateur
  var CLAVIER = { 8: TOUCHES.retour, 27: TOUCHES.retour, 32: TOUCHES.lecturePause, 33: TOUCHES.chaineHaut, 34: TOUCHES.chaineBas };

  var courant = null;      // dossier affiché, dans l'ordre d'affichage (voir vue())
  var positions = {};      // chemin -> chemin de l'élément sélectionné, pour revenir au même endroit
  var demande = 0;         // numéro de la dernière ouverture de dossier
  var minuteriePrechargement = null, quitterAvant = 0;
  var inverse = false;     // ordre d'affichage inversé (mémorisé sur la télé)
  try { inverse = localStorage.getItem('ordreInverse') === '1'; } catch (e) {}
  var boutons = [].slice.call(document.querySelectorAll('#actions .bouton'));
  var bouton = -1;         // bouton de l'en-tête sélectionné, -1 = sélection dans la grille

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

  // Réponse /api/folder dans l'ordre d'affichage choisi (copie inversée si besoin)
  function vue(d) {
    if (!inverse) return d;
    var v = {};
    Object.keys(d).forEach(function (k) { v[k] = d[k]; });
    v.folders = d.folders.slice().reverse();
    v.items = d.items.slice().reverse();
    return v;
  }

  // Index dans la grille de l'élément de chemin donné (dossier ou média), sinon -1
  function indexDe(d, chemin) {
    var i;
    for (i = 0; i < d.folders.length; i++) if (d.folders[i].path === chemin) return i;
    for (i = 0; i < d.items.length; i++) if (d.items[i].path === chemin) return d.folders.length + i;
    return -1;
  }

  function cheminSelectionne() {
    var s = Grille.selection();
    return s ? s.donnee.path : null;
  }

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

  // Ouvre un dossier et y sélectionne l'élément de chemin « cible » (sous-dossier d'où l'on remonte,
  // photo affichée en fin de diaporama…) ; sans cible, la dernière position connue du dossier.
  function ouvrir(chemin, cible) {
    var n = ++demande;
    var attente = setTimeout(function () { $('chargement').classList.add('visible'); }, 150);
    Api.dossier(chemin).then(function (brut) {
      if (n !== demande) return;
      clearTimeout(attente);
      $('chargement').classList.remove('visible');
      if (courant) positions[courant.path] = cheminSelectionne();
      var d = vue(brut);
      var index = indexDe(d, cible !== undefined ? cible : positions[chemin]);
      courant = d;
      majEntete(d);
      var total = Grille.afficher(d, Math.max(0, index));
      $('vide').textContent = 'Dossier vide';
      $('vide').classList.toggle('visible', !total);
      if (!total) choisirBouton(1); else if (bouton >= 0) choisirBouton(-1);
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

  // ---------- Boutons de l'en-tête ----------
  function majBoutons() {
    boutons.forEach(function (b, i) { b.classList.toggle('focus', i === bouton); });
    $('b-ordre').classList.toggle('inverse', inverse);
    $('b-ordre').querySelector('.libelle').textContent = inverse ? 'Ordre inversé' : 'Ordre';
  }

  function choisirBouton(i) {
    bouton = i;
    Grille.masquerFocus(i >= 0);
    majBoutons();
  }

  function inverserOrdre() {
    inverse = !inverse;
    try { localStorage.setItem('ordreInverse', inverse ? '1' : '0'); } catch (e) {}
    var garde = cheminSelectionne();
    Api.dossier(courant.path).then(function (brut) {
      var d = vue(brut);
      courant = d;
      Grille.afficher(d, Math.max(0, indexDe(d, garde)));
      Grille.masquerFocus(bouton >= 0);
      majBoutons();
    });
  }

  // Diaporama du dossier « chemin » (sous-dossiers compris) ; depart = photo de départ (ordre normal)
  function diaporama(chemin, aleatoire, depart) {
    var o = { order: aleatoire ? 'random' : (inverse ? 'desc' : 'asc'), start: depart };
    Diaporama.lancer(chemin, o, function (photo) {
      // Sortie : on se place sur la photo affichée, dans son dossier
      if (!photo) return;
      var dossierPhoto = photo.path.lastIndexOf('/') >= 0 ? photo.path.slice(0, photo.path.lastIndexOf('/')) : '';
      choisirBouton(-1);
      if (dossierPhoto === courant.path) Grille.placer(Math.max(0, indexDe(courant, photo.path)));
      else ouvrir(dossierPhoto, photo.path);
    }).then(function (ok) {
      if (!ok) message('Aucune photo dans ce dossier');
    }, function (e) {
      message('Diaporama impossible : ' + e.message);
    });
  }

  // Photo de départ du diaporama : la photo sélectionnée dans la grille, s'il y en a une
  function photoSelectionnee() {
    var s = Grille.selection();
    return s && s.type === 'media' && s.donnee.type === 'photo' ? s.donnee.path : undefined;
  }

  function activerBouton() {
    var a = boutons[bouton].getAttribute('data-action');
    if (a === 'ordre') inverserOrdre();
    else if (a === 'diaporama') diaporama(courant.path, false, photoSelectionnee());
    else diaporama(courant.path, true);
  }

  // Touche « lecture » dans la grille : diaporama du dossier sélectionné, ou à partir de la photo
  function lectureSurSelection() {
    var s = Grille.selection();
    if (!s) return;
    if (s.type === 'dossier') diaporama(s.donnee.path, false);
    else if (s.donnee.type === 'photo') diaporama(courant.path, false, s.donnee.path);
    else valider();
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
    if (Diaporama.touche(k, T) || Video.touche(k, T) || Visionneuse.touche(k, T)) { ev.preventDefault(); return; }
    if (!courant) {
      if (k === T.ok) ouvrir('');
      else if (k === T.retour) quitter();
      ev.preventDefault();
      return;
    }
    if (bouton >= 0) {
      // Sélection sur les boutons de l'en-tête
      if (k === T.gauche) choisirBouton(Math.max(0, bouton - 1));
      else if (k === T.droite) choisirBouton(Math.min(boutons.length - 1, bouton + 1));
      else if (k === T.bas) { if (Grille.ligne() >= 0) choisirBouton(-1); }
      else if (k === T.ok) activerBouton();
      else if (k === T.retour) remonter();
      else if (k === T.lecture || k === T.lecturePause) diaporama(courant.path, false);
      else return;
      ev.preventDefault();
      return;
    }
    if (k === T.gauche) Grille.deplacer('gauche');
    else if (k === T.droite) Grille.deplacer('droite');
    else if (k === T.haut) { if (Grille.ligne() <= 0) choisirBouton(1); else Grille.deplacer('haut'); }
    else if (k === T.bas) Grille.deplacer('bas');
    else if (k === T.chaineHaut) Grille.page(-1);
    else if (k === T.chaineBas) Grille.page(1);
    else if (k === T.ok) valider();
    else if (k === T.lecture || k === T.lecturePause) lectureSurSelection();
    else if (k === T.retour) remonter();
    else return;
    ev.preventDefault();
  });

  try {
    ['MediaPlayPause', 'MediaPlay', 'MediaPause', 'MediaStop', 'MediaFastForward', 'MediaRewind',
     'ChannelUp', 'ChannelDown'].forEach(function (k) { tizen.tvinputdevice.registerKey(k); });
  } catch (e) {}

  majBoutons();
  ouvrir('');
})();
