// Grille virtualisée d'un dossier : sous-dossiers puis médias. Seules les lignes proches de la zone
// visible existent dans le DOM (des dossiers ont plus de 1000 photos : la mémoire de la télé ne suivrait pas).
var Grille = (function () {
  var LARGEUR = 1792, ECART = 24, HAUTEUR_VUE = 940;
  var H_IMAGE = 186, H_LEGENDE = 72, MARGE_HAUT = 20, ECART_SECTIONS = 36;
  var MARGE_RENDU = 500;   // px rendus au-delà de la zone visible, en haut et en bas

  var plan = document.getElementById('plan');
  var cols, larg, dossiers = [], medias = [], lignes = [], total = 0;
  var tuiles = {};          // index -> élément
  var courant = 0, defilement = 0;

  function construireLignes() {
    lignes = [];
    var y = MARGE_HAUT, i;
    for (i = 0; i < dossiers.length; i += cols) {
      lignes.push({ haut: y, h: H_IMAGE + H_LEGENDE, debut: i, fin: Math.min(i + cols, dossiers.length) });
      y += H_IMAGE + H_LEGENDE + ECART;
    }
    if (dossiers.length && medias.length) y += ECART_SECTIONS;
    for (i = 0; i < medias.length; i += cols) {
      var d = dossiers.length + i;
      lignes.push({ haut: y, h: H_IMAGE, debut: d, fin: Math.min(d + cols, total) });
      y += H_IMAGE + ECART;
    }
    plan.style.height = y + 'px';
  }

  function ligneDe(i) {
    var nbLignesDossiers = Math.ceil(dossiers.length / cols);
    return i < dossiers.length ? Math.floor(i / cols)
      : nbLignesDossiers + Math.floor((i - dossiers.length) / cols);
  }

  function image(tuile, url) {
    if (Api.estAbsente(url)) { tuile.classList.add('sans-image'); return; }
    var img = document.createElement('img');
    img.decoding = 'async';
    img.onerror = function () {
      Api.marquerAbsente(url);
      tuile.classList.add('sans-image');
      if (img.parentNode) img.parentNode.removeChild(img);
    };
    img.src = url;
    (tuile.querySelector('.cadre') || tuile).appendChild(img);
  }

  function creerTuile(i) {
    var l = lignes[ligneDe(i)], t = document.createElement('div');
    t.style.left = ((i - l.debut) * (larg + ECART)) + 'px';
    t.style.top = l.haut + 'px';
    t.style.width = larg + 'px';
    if (i < dossiers.length) {
      var d = dossiers[i];
      t.className = 'tuile dossier';
      t.innerHTML = '<div class="cadre"></div><div class="nom"></div><div class="compte"></div>';
      t.querySelector('.nom').textContent = d.name;
      t.querySelector('.compte').textContent = compte(d);
      if (d.cover) image(t, Api.vignette(d.cover, 'm'));
      else t.classList.add('sans-image');
    } else {
      var m = medias[i - dossiers.length];
      t.className = 'tuile media' + (m.type === 'video' ? ' video' : '');
      t.style.height = H_IMAGE + 'px';
      image(t, Api.vignette(m.path, 'm', m.mtime));
    }
    if (i === courant) t.classList.add('focus');
    plan.appendChild(t);
    return t;
  }

  function compte(d) {
    var p = [];
    if (d.folders) p.push(d.folders + (d.folders > 1 ? ' dossiers' : ' dossier'));
    if (d.items) p.push(d.items + (d.items > 1 ? ' éléments' : ' élément'));
    return p.join(' · ') || 'vide';
  }

  function rendre() {
    var haut = defilement - MARGE_RENDU, bas = defilement + HAUTEUR_VUE + MARGE_RENDU, garder = {};
    lignes.forEach(function (l) {
      if (l.haut + l.h < haut || l.haut > bas) return;
      for (var i = l.debut; i < l.fin; i++) {
        garder[i] = true;
        if (!tuiles[i]) tuiles[i] = creerTuile(i);
      }
    });
    Object.keys(tuiles).forEach(function (k) {
      if (garder[k]) return;
      var img = tuiles[k].querySelector('img');
      if (img) img.removeAttribute('src');   // libère l'image décodée
      plan.removeChild(tuiles[k]);
      delete tuiles[k];
    });
    plan.style.transform = 'translateY(' + (-defilement) + 'px)';
  }

  // Garde la ligne sélectionnée dans la zone visible
  function suivre() {
    if (!total) return;
    var l = lignes[ligneDe(courant)];
    if (l.haut - MARGE_HAUT < defilement) defilement = Math.max(0, l.haut - MARGE_HAUT);
    else if (l.haut + l.h + ECART > defilement + HAUTEUR_VUE) defilement = l.haut + l.h + ECART - HAUTEUR_VUE;
  }

  function placer(i) {
    if (!total) return;
    i = Math.max(0, Math.min(total - 1, i));
    if (tuiles[courant]) tuiles[courant].classList.remove('focus');
    courant = i;
    suivre();
    rendre();
    if (tuiles[courant]) tuiles[courant].classList.add('focus');
    if (Grille.surSelection) Grille.surSelection(selection());
  }

  function selection() {
    if (!total) return null;
    return courant < dossiers.length
      ? { type: 'dossier', donnee: dossiers[courant], index: courant }
      : { type: 'media', donnee: medias[courant - dossiers.length], index: courant,
          rang: courant - dossiers.length };
  }

  function versLigne(r) {
    var l = lignes[r];
    if (!l) return;
    var col = courant - lignes[ligneDe(courant)].debut;
    placer(l.debut + Math.min(col, l.fin - l.debut - 1));
  }

  return {
    // donnees = réponse /api/folder ; index = élément à sélectionner
    afficher: function (donnees, index) {
      Object.keys(tuiles).forEach(function (k) { plan.removeChild(tuiles[k]); });
      tuiles = {};
      cols = CONFIG.colonnes || 6;
      larg = Math.floor((LARGEUR - (cols - 1) * ECART) / cols);
      H_IMAGE = Math.round(larg * 2 / 3);
      dossiers = donnees.folders || [];
      medias = donnees.items || [];
      total = dossiers.length + medias.length;
      courant = 0;
      defilement = 0;
      construireLignes();
      if (total) placer(index || 0); else rendre();
      return total;
    },
    deplacer: function (dir) {
      if (!total) return;
      var r = ligneDe(courant);
      if (dir === 'gauche') placer(courant - 1);
      else if (dir === 'droite') placer(courant + 1);
      else if (dir === 'haut') versLigne(r - 1);
      else versLigne(r + 1);
    },
    // Saut d'un écran vers le haut (-1) ou le bas (+1)
    page: function (sens) {
      if (!total) return;
      var r = ligneDe(courant), cible = r, l0 = lignes[r];
      while (lignes[cible + sens] && Math.abs(lignes[cible + sens].haut - l0.haut) < HAUTEUR_VUE - H_IMAGE) cible += sens;
      if (cible === r) cible = r + sens;
      versLigne(Math.max(0, Math.min(lignes.length - 1, cible)));
    },
    // Position dans la liste des médias (visionneuse) -> sélection dans la grille
    placerMedia: function (rang) { placer(dossiers.length + rang); },
    placer: placer,
    selection: selection,
    index: function () { return courant; },
    surSelection: null
  };
})();
