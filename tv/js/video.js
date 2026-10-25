// Lecture vidéo plein écran : AVPlay (lecteur natif de la télé, HEVC/4K) quand il existe,
// sinon l'élément <video> (navigateur du PC pour les essais).
var Video = (function () {
  var SAUT = 10000;   // ms pour ← / →
  var lecteur = document.getElementById('lecteur');
  var html = document.getElementById('video-html');
  var barre = document.getElementById('l-barre');
  var av = (typeof webapis !== 'undefined' && webapis.avplay) ? webapis.avplay : null;
  var ouvert = false, enPause = false, surFin = null, surErreur = null, minuterieBarre = null;

  function temps(ms) {
    var s = Math.max(0, Math.floor(ms / 1000)), h = Math.floor(s / 3600), m = Math.floor(s / 60) % 60;
    s = s % 60;
    return (h ? h + ':' + ('0' + m).slice(-2) : m) + ':' + ('0' + s).slice(-2);
  }
  function position() {
    try { return av ? av.getCurrentTime() : html.currentTime * 1000; } catch (e) { return 0; }
  }
  function duree() {
    try { return av ? av.getDuration() : (html.duration || 0) * 1000; } catch (e) { return 0; }
  }
  function majBarre() {
    var p = position(), d = duree();
    document.getElementById('l-avance').style.width = (d ? Math.min(100, p * 100 / d) : 0) + '%';
    document.getElementById('l-temps').textContent = temps(p) + ' / ' + temps(d) + (enPause ? '  ❚❚' : '');
  }
  // Affiche la barre puis la masque après quelques secondes (sauf en pause)
  function montrerBarre() {
    majBarre();
    barre.classList.add('visible');
    clearTimeout(minuterieBarre);
    if (!enPause) minuterieBarre = setTimeout(function () { barre.classList.remove('visible'); }, 3000);
  }

  function echec() {
    var f = surErreur;
    fermer();
    if (f) f();
  }
  function terminer() {
    var f = surFin;
    fermer();
    if (f) f();
  }

  function lire(url, nom, fin, erreur) {
    ouvert = true; enPause = false; surFin = fin; surErreur = erreur;
    document.getElementById('l-nom').textContent = nom;
    document.body.classList.add('en-lecture');
    lecteur.classList.add('ouvert');
    lecteur.classList.toggle('avplay', !!av);
    montrerBarre();
    if (av) {
      try {
        av.open(url);
        av.setDisplayRect(0, 0, 1920, 1080);
        av.setListener({
          onstreamcompleted: function () { setTimeout(terminer, 0); },
          onerror: function () { setTimeout(echec, 0); },
          oncurrentplaytime: function () { if (barre.classList.contains('visible')) majBarre(); }
        });
        av.prepareAsync(function () { if (ouvert) { av.play(); montrerBarre(); } }, echec);
      } catch (e) { echec(); }
    } else {
      html.onended = terminer;
      html.onerror = echec;
      html.ontimeupdate = function () { if (barre.classList.contains('visible')) majBarre(); };
      html.src = url;
      var p = html.play();
      if (p && p.catch) p.catch(function () {});
    }
  }

  function fermer() {
    if (!ouvert) return;
    ouvert = false;
    clearTimeout(minuterieBarre);
    if (av) { try { av.stop(); av.close(); } catch (e) {} }
    html.onended = html.onerror = html.ontimeupdate = null;
    html.removeAttribute('src');
    html.load();
    barre.classList.remove('visible');
    lecteur.classList.remove('ouvert');
    document.body.classList.remove('en-lecture');
  }

  function basculer(pause) {
    enPause = pause === undefined ? !enPause : pause;
    try {
      if (av) { if (enPause) av.pause(); else av.play(); }
      else if (enPause) html.pause(); else html.play();
    } catch (e) {}
    montrerBarre();
  }

  function sauter(ms) {
    try {
      if (av) {
        var d = duree(), p = position();
        if (ms > 0 && d && p + ms > d - 1000) return;
        if (ms > 0) av.jumpForward(ms); else av.jumpBackward(-ms);
      } else {
        html.currentTime = Math.max(0, html.currentTime + ms / 1000);
      }
    } catch (e) {}
    montrerBarre();
  }

  // Touche reçue pendant la lecture ; renvoie true si elle a été traitée
  function touche(k, T) {
    if (!ouvert) return false;
    if (k === T.retour || k === T.stop) fermer();
    else if (k === T.ok || k === T.lecturePause) basculer();
    else if (k === T.lecture) basculer(false);
    else if (k === T.pause) basculer(true);
    else if (k === T.droite || k === T.avance) sauter(SAUT);
    else if (k === T.gauche || k === T.retourRapide) sauter(-SAUT);
    else montrerBarre();
    return true;
  }

  document.addEventListener('visibilitychange', function () { if (document.hidden) fermer(); });

  return { lire: lire, fermer: fermer, touche: touche, ouvert: function () { return ouvert; } };
})();
