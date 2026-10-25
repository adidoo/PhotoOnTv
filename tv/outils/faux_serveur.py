"""Faux serveur PhotoOnTv pour tester l'appli TV dans un navigateur.

Implémente docs/api.md sur un dossier local ; les vignettes sont calculées à la volée avec Pillow
(pas les vignettes QuMagie). Les vidéos n'ont pas de vignette (404), comme sur un NAS pas encore indexé.

    python faux_serveur.py <dossier photos> [--port 8080] [--latence 0.05] [--appli ../]

Avec --appli, sert aussi l'appli TV sur /appli/ (même origine, pratique pour Chrome).
"""
import argparse
import io
import json
import mimetypes
import os
import re
import sys
import time
import urllib.parse
from http.server import SimpleHTTPRequestHandler, ThreadingHTTPServer

from PIL import Image, ImageOps

PHOTOS = {'jpg', 'jpeg', 'png', 'gif', 'tif', 'tiff', 'heic', 'heif', 'cr2', 'nef', 'arw', 'dng', 'webp'}
VIDEOS = {'mp4', 'mov', 'm4v', '3gp', 'mts', 'm2ts', 'avi', 'mpg', 'mpeg', 'mkv', 'wmv'}
TAILLES = {'s': 150, 'm': 600, 'l': 1200}
EXCLUS = {'@Recycle', '@Recently-Snapshot', '.streams', '.@__thumb'}

args = None
cache_vignettes = {}


def tri_naturel(nom):
    return [int(t) if t.isdigit() else t.lower() for t in re.split(r'(\d+)', nom)]


def type_media(nom):
    ext = nom.rsplit('.', 1)[-1].lower() if '.' in nom else ''
    return 'photo' if ext in PHOTOS else 'video' if ext in VIDEOS else None


def ignore(nom):
    return nom in EXCLUS or nom.startswith('.') or nom.startswith('@')


def absolu(chemin):
    racine = os.path.realpath(args.racine)
    p = os.path.realpath(os.path.join(racine, *[c for c in chemin.split('/') if c]))
    if p != racine and not p.startswith(racine + os.sep):
        raise ValueError('chemin hors de la racine')
    return p


def lister(p):
    dossiers, medias = [], []
    for e in os.scandir(p):
        if ignore(e.name):
            continue
        if e.is_dir():
            dossiers.append(e)
        elif type_media(e.name):
            medias.append(e)
    dossiers.sort(key=lambda e: tri_naturel(e.name))
    medias.sort(key=lambda e: tri_naturel(e.name))
    return dossiers, medias


def couverture(p, rel, profondeur=0):
    dossiers, medias = lister(p)
    if medias:
        return rel + '/' + medias[0].name if rel else medias[0].name
    if profondeur < 6:
        for d in dossiers:
            c = couverture(d.path, (rel + '/' if rel else '') + d.name, profondeur + 1)
            if c:
                return c
    return None


class Gestionnaire(SimpleHTTPRequestHandler):
    def end_headers(self):
        self.send_header('Access-Control-Allow-Origin', '*')
        super().end_headers()

    def log_message(self, fmt, *a):
        sys.stderr.write('%s %s\n' % (time.strftime('%H:%M:%S'), fmt % a))

    def json(self, code, obj):
        corps = json.dumps(obj, ensure_ascii=False).encode('utf-8')
        self.send_response(code)
        self.send_header('Content-Type', 'application/json; charset=utf-8')
        self.send_header('Content-Length', str(len(corps)))
        self.end_headers()
        self.wfile.write(corps)

    def do_GET(self):
        u = urllib.parse.urlsplit(self.path)
        q = dict(urllib.parse.parse_qsl(u.query, keep_blank_values=True))
        if args.appli and u.path == '/appli/js/config.js':
            # L'appli servie ici parle à ce faux serveur, quelle que soit sa config.js
            corps = ("var CONFIG = { serveur: 'http://%s', racine: 'Photos', colonnes: 6 };"
                     % self.headers.get('Host')).encode()
            self.send_response(200)
            self.send_header('Content-Type', 'application/javascript')
            self.send_header('Content-Length', str(len(corps)))
            self.end_headers()
            return self.wfile.write(corps)
        if args.appli and u.path.startswith('/appli/'):
            self.path = u.path[len('/appli'):]
            self.directory = os.path.abspath(args.appli)
            return super().do_GET()
        if args.latence:
            time.sleep(args.latence)
        try:
            if u.path == '/api/health':
                return self.json(200, {'ok': True, 'version': 'faux', 'root': args.racine})
            chemin = q.get('path', '')
            if u.path.startswith('/api/file/'):
                chemin = urllib.parse.unquote(u.path[len('/api/file/'):])
                u = u._replace(path='/api/file')
            p = absolu(chemin)
            if u.path == '/api/folder':
                return self.dossier(chemin, p)
            if u.path == '/api/thumb':
                return self.vignette(p, q.get('size', 'm'))
            if u.path == '/api/file':
                return self.fichier(p)
            self.json(404, {'error': 'route inconnue'})
        except ValueError as e:
            self.json(400, {'error': str(e)})
        except FileNotFoundError:
            self.json(404, {'error': 'introuvable'})

    def dossier(self, chemin, p):
        if not os.path.isdir(p):
            return self.json(404, {'error': 'dossier introuvable'})
        dossiers, medias = lister(p)
        chemin = chemin.strip('/')
        pref = chemin + '/' if chemin else ''
        res = {
            'path': chemin,
            'name': chemin.split('/')[-1] if chemin else '',
            'parent': None if not chemin else '/'.join(chemin.split('/')[:-1]),
            'folders': [],
            'items': [],
        }
        for d in dossiers:
            sd, sm = lister(d.path)
            res['folders'].append({'name': d.name, 'path': pref + d.name, 'folders': len(sd),
                                   'items': len(sm), 'cover': couverture(d.path, pref + d.name)})
        for m in medias:
            st = m.stat()
            res['items'].append({'name': m.name, 'path': pref + m.name, 'type': type_media(m.name),
                                 'size': st.st_size, 'mtime': int(st.st_mtime)})
        self.json(200, res)

    def vignette(self, p, taille):
        if taille not in TAILLES or type_media(p) != 'photo':
            return self.json(404, {'error': 'pas de vignette'})
        cle = (p, taille)
        if cle not in cache_vignettes:
            try:
                im = ImageOps.exif_transpose(Image.open(p)).convert('RGB')
            except Exception:
                return self.json(404, {'error': 'pas de vignette'})
            im.thumbnail((TAILLES[taille], TAILLES[taille]))
            b = io.BytesIO()
            im.save(b, 'JPEG', quality=82)
            cache_vignettes[cle] = b.getvalue()
        corps = cache_vignettes[cle]
        self.send_response(200)
        self.send_header('Content-Type', 'image/jpeg')
        self.send_header('Content-Length', str(len(corps)))
        self.send_header('Cache-Control', 'public, max-age=31536000, immutable')
        self.end_headers()
        self.wfile.write(corps)

    def fichier(self, p):
        if not os.path.isfile(p):
            return self.json(404, {'error': 'fichier introuvable'})
        taille = os.path.getsize(p)
        debut, fin, code = 0, taille - 1, 200
        m = re.match(r'bytes=(\d*)-(\d*)', self.headers.get('Range', ''))
        if m:
            code = 206
            if m.group(1):
                debut = int(m.group(1))
                if m.group(2):
                    fin = min(int(m.group(2)), taille - 1)
            else:
                debut = max(0, taille - int(m.group(2)))
        self.send_response(code)
        self.send_header('Content-Type', mimetypes.guess_type(p)[0] or 'application/octet-stream')
        self.send_header('Accept-Ranges', 'bytes')
        self.send_header('Content-Length', str(fin - debut + 1))
        if code == 206:
            self.send_header('Content-Range', 'bytes %d-%d/%d' % (debut, fin, taille))
        self.end_headers()
        with open(p, 'rb') as f:
            f.seek(debut)
            reste = fin - debut + 1
            try:
                while reste > 0:
                    bloc = f.read(min(65536, reste))
                    if not bloc:
                        break
                    self.wfile.write(bloc)
                    reste -= len(bloc)
            except (BrokenPipeError, ConnectionResetError):
                pass


if __name__ == '__main__':
    a = argparse.ArgumentParser()
    a.add_argument('racine')
    a.add_argument('--port', type=int, default=8080)
    a.add_argument('--latence', type=float, default=0.0, help='secondes ajoutées à chaque requête API')
    a.add_argument('--appli', help="dossier de l'appli TV à servir sur /appli/")
    args = a.parse_args()
    print('Faux serveur sur http://127.0.0.1:%d (racine %s)' % (args.port, args.racine))
    ThreadingHTTPServer(('0.0.0.0', args.port), Gestionnaire).serve_forever()
