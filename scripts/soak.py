#!/usr/bin/env python3
"""A long soak run: hours of ordinary use, watching for slow leaks and slow answers.

Starts a test server with its own data folder and the demo media, keeps it busy the way a
household would (music with lyrics, a library channel, a 1080p transcode, a DVD menu, a
scan every 30 minutes, the web app's pages in a loop from several clients), and records
every minute: the Go heap and the process's memory, goroutines, open files, FFmpeg
processes, and how long /api/health and a library page take to answer. It fails when any
of them keeps rising, an answer takes longer than its limit, or the server stops, and writes
a short report with a graph.

Linux first (open files and processes come from /proc); runs in WSL. Python 3, nothing to
install. Nothing goes online: lyrics come from a stand-in this script runs, which answers
quickly, slowly, with nothing, or not at all, the way real services do (issue #17).

    go build -o /tmp/wms ./cmd/wittymediaserver
    python3 scripts/soak.py --binary /tmp/wms --media /mnt/d/WittyDemoMedia --hours 4

See scripts/README.md. Exit code 0 is a pass, 1 a failure, 2 a run that couldn't start.
"""

import argparse
import http.client
import http.server
import json
import os
import random
import re
import secrets
import signal
import socketserver
import statistics
import subprocess
import sys
import threading
import time
import urllib.parse

# ---------------------------------------------------------------- settings

def parse_args():
    p = argparse.ArgumentParser(description=__doc__.split("\n\n")[0])
    p.add_argument("--binary", required=True, help="the wittymediaserver to run")
    p.add_argument("--media", default=os.environ.get("WMS_SOAK_MEDIA", "/mnt/d/WittyDemoMedia"), help="the demo media folder")
    p.add_argument("--make-media", action="store_true", help="make a small library with FFmpeg in <out>/media instead (for CI)")
    p.add_argument("--out", default="soak-out", help="where the data folder, log, samples and report go")
    p.add_argument("--hours", type=float, default=4.0)
    p.add_argument("--minutes", type=float, default=0, help="instead of --hours, for a short try")
    p.add_argument("--port", type=int, default=19780)
    p.add_argument("--ffmpeg", default="", help="FFmpeg for the server; its own search if empty")
    p.add_argument("--clients", type=int, default=4, help="web app clients fetching pages")
    p.add_argument("--sample", type=float, default=60, help="seconds between samples")
    p.add_argument("--scan-every", type=float, default=30, help="minutes between library scans")
    p.add_argument("--max-health", type=float, default=2.0, help="seconds /api/health may take")
    p.add_argument("--max-page", type=float, default=5.0, help="seconds a library page may take")
    p.add_argument("--growth", type=float, default=0.25, help="how much a measure may rise, start to end, as a fraction")
    p.add_argument("--warmup", type=float, default=20, help="minutes left out of the trend: caches filling is not a leak")
    return p.parse_args()

# ---------------------------------------------------------------- talking to the server

class Server:
    def __init__(self, port):
        self.host, self.port, self.token = "127.0.0.1", port, ""
        self.base_host = "soak.localhost:%d" % port

    def headers(self):
        h = {"Host": self.base_host, "Origin": "http://" + self.base_host, "User-Agent": "wms-soak"}
        if self.token:
            h["Authorization"] = "Bearer " + self.token
        return h

    def call(self, method, path, body=None, timeout=60, raw=False, limit=None):
        """One request. raw reads the answer through (up to limit bytes) and gives its size."""
        c = http.client.HTTPConnection(self.host, self.port, timeout=timeout)
        h = self.headers()
        data = None
        if body is not None:
            data = json.dumps(body)
            h["Content-Type"] = "application/json"
        try:
            c.request(method, path, body=data, headers=h)
            r = c.getresponse()
            if raw:
                n = 0
                while True:
                    chunk = r.read(65536)
                    if not chunk:
                        break
                    n += len(chunk)
                    if limit and n >= limit:
                        break
                return r.status, n
            b = r.read()
            try:
                return r.status, json.loads(b) if b else None
            except ValueError:
                return r.status, b.decode(errors="replace")
        finally:
            c.close()

    def timed(self, path, timeout=60):
        t = time.monotonic()
        try:
            st, _ = self.call("GET", path, timeout=timeout)
        except Exception as e:  # noqa: BLE001 - any failure is a failed answer
            return time.monotonic() - t, str(e)
        return time.monotonic() - t, None if st == 200 else "answered %d" % st

# ---------------------------------------------------------------- a lyrics service that misbehaves

class Lyrics(http.server.BaseHTTPRequestHandler):
    """LRCLIB's routes. Most answers are quick; some slow, some empty, some never."""

    def do_GET(self):
        roll = random.random()
        if roll < 0.10:
            time.sleep(60)  # takes the call and never answers in time
            return
        if roll < 0.25:
            time.sleep(5)
        if roll < 0.45 or not self.path.startswith("/api/"):
            self.send_response(404)
            self.send_header("Content-Length", "0")
            self.end_headers()
            return
        q = urllib.parse.parse_qs(urllib.parse.urlparse(self.path).query)
        title = (q.get("track_name") or q.get("q") or ["Song"])[0]
        rec = {"id": random.randint(1, 10**6), "trackName": title, "artistName": "Artist", "albumName": "", "duration": 0,
               "instrumental": False, "plainLyrics": "la la la\nla la", "syncedLyrics": "[00:01.00]la la la\n[00:03.00]la la"}
        body = json.dumps([rec] if self.path.startswith("/api/search") else rec).encode()
        self.send_response(200)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(body)))
        self.end_headers()
        self.wfile.write(body)

    def log_message(self, *a):
        pass

class Threaded(socketserver.ThreadingMixIn, http.server.HTTPServer):
    daemon_threads = True

# ---------------------------------------------------------------- media for a run without the demo media

def make_media(root, ffmpeg):
    """A small library made with FFmpeg's own test pictures and tones: three five-minute 1080p films, an
    album of songs with tags, and a show. Enough to soak on a machine without the demo media."""
    def ff(*a):
        subprocess.run([ffmpeg, "-hide_banner", "-loglevel", "error", "-nostdin", "-y", *a], check=True)
    films = os.path.join(root, "Movies")
    for i, (name, src) in enumerate([("Test Pattern (2001)", "testsrc2"), ("Moving Colours (2002)", "smptehdbars"), ("Mandel (2003)", "mandelbrot")]):
        d = os.path.join(films, name)
        os.makedirs(d, exist_ok=True)
        out = os.path.join(d, name + ".mkv")
        if not os.path.exists(out):
            ff("-f", "lavfi", "-t", "300", "-i", "%s=s=1920x1080:r=24" % src, "-f", "lavfi", "-t", "300", "-i", "sine=f=%d" % (220 + 110 * i),
               "-c:v", "libx264", "-preset", "ultrafast", "-crf", "30", "-c:a", "aac", "-shortest", out)
    album = os.path.join(root, "Music", "Test Artist", "Test Album")
    os.makedirs(album, exist_ok=True)
    for i in range(1, 25):
        out = os.path.join(album, "%02d Tone %d.mp3" % (i, i))
        if not os.path.exists(out):
            ff("-f", "lavfi", "-t", "120", "-i", "sine=f=%d" % (200 + 20 * i), "-c:a", "libmp3lame", "-b:a", "96k",
               "-metadata", "title=Tone %d" % i, "-metadata", "artist=Test Artist", "-metadata", "album=Test Album", "-metadata", "track=%d" % i, out)
    show = os.path.join(root, "TV", "Test Show", "Season 01")
    os.makedirs(show, exist_ok=True)
    for i in range(1, 7):
        out = os.path.join(show, "Test Show - S01E%02d.mkv" % i)
        if not os.path.exists(out):
            ff("-f", "lavfi", "-t", "120", "-i", "testsrc=s=1280x720:r=24", "-f", "lavfi", "-t", "120", "-i", "sine=f=%d" % (300 + 50 * i),
               "-c:v", "libx264", "-preset", "ultrafast", "-crf", "32", "-c:a", "aac", "-shortest", out)

# ---------------------------------------------------------------- what is happening

class Run:
    def __init__(self):
        self.stop = threading.Event()
        self.lock = threading.Lock()
        self.counts = {}  # activity -> [done, failed]
        self.notes = []  # things the report should say

    def did(self, what, ok=True):
        with self.lock:
            c = self.counts.setdefault(what, [0, 0])
            c[0 if ok else 1] += 1

    def note(self, text):
        with self.lock:
            if text in self.notes or len(self.notes) > 200:
                return
            self.notes.append(text)
        print("note:", text, flush=True)

    def sleep(self, sec):
        """Waits, and says whether the run has ended meanwhile."""
        return self.stop.wait(sec)

def loop(run, name, fn):
    """Runs one activity until the run ends. A failure is counted, and it carries on."""
    def go():
        while not run.stop.is_set():
            try:
                fn()
            except Exception as e:  # noqa: BLE001
                run.did(name, False)
                run.note("%s: %s" % (name, str(e)[:160]))
                run.sleep(10)
    t = threading.Thread(target=go, name=name, daemon=True)
    t.start()
    return t

def listing(out):
    """A list from an answer that's a list, or a page with one inside."""
    if isinstance(out, list):
        return out
    if isinstance(out, dict):
        for k in ("items", "songs", "channels"):
            if isinstance(out.get(k), list):
                return out[k]
    return []

# ---------------------------------------------------------------- setting up

def setup(srv, run, media, lyrics_port):
    cred = {"name": "soak", "password": secrets.token_urlsafe(18)}
    st, out = srv.call("POST", "/api/setup", dict(cred, device="soak", acknowledgedLegal=True))
    if st != 200:
        raise RuntimeError("setup answered %d: %s" % (st, out))
    st, out = srv.call("POST", "/api/login", dict(cred, device="soak"))
    srv.token = out["token"]
    st, out = srv.call("PUT", "/api/settings", {
        "saveMetadata": "0", "updateCheck": "0", "moduleUpdateCheck": "0", "tvmaze": "0", "wikidata": "0", "musicbrainz": "0",
        "lyricsOnline": "1", "lyricsLrclib": "0", "lyricsMirror": "http://127.0.0.1:%d" % lyrics_port})
    if st != 200:
        run.note("settings answered %d: %s" % (st, out))
    libs = {}
    for name, kind, folder in [("Music", "music", "Music"), ("Movies", "movies", "Movies"), ("TV", "shows", "TV"), ("DVD", "discs", "Discs/DVD")]:
        path = os.path.join(media, folder)
        if not os.path.isdir(path):
            run.note("no %s folder in the media: %s is left out" % (folder, name))
            continue
        st, lib = srv.call("POST", "/api/libraries", {"name": name, "type": kind, "paths": [path]})
        if st not in (200, 201):
            run.note("library %s answered %d: %s" % (name, st, lib))
            continue
        libs[name] = lib["id"]
    # The first scans: the soak is about what comes after them.
    deadline = time.time() + 60 * 60
    while time.time() < deadline:
        busy = False
        for lid in libs.values():
            st, p = srv.call("GET", "/api/libraries/%d/scan" % lid)
            if st == 200 and isinstance(p, dict) and (p.get("scanning") or p.get("running") or p.get("active")):
                busy = True
        if not busy:
            break
        time.sleep(10)
    return libs

def items(srv, lib, limit=500):
    st, out = srv.call("GET", "/api/items?" + urllib.parse.urlencode({"library": lib, "limit": limit}))
    return listing(out) if st == 200 else []

# ---------------------------------------------------------------- the activities

def music(srv, run, lib):
    st, out = srv.call("GET", "/api/music/songs?" + urllib.parse.urlencode({"library": lib, "limit": 500}))
    songs = listing(out)
    if not songs:
        run.note("no songs: music is left out")
        return

    def one():
        s = random.choice(songs)
        st, play = srv.call("POST", "/api/items/%d/playback" % s["id"], {"profile": "web"})
        run.did("music: play", st == 200)
        if st == 200 and isinstance(play, dict) and play.get("url"):
            st, _ = srv.call("GET", play["url"], raw=True, limit=4 << 20, timeout=120)
            run.did("music: stream", st in (200, 206))
        t = time.monotonic()
        st, _ = srv.call("GET", "/api/music/lyrics/%d?fetch=1" % s["id"], timeout=90)
        run.did("music: lyrics", st == 200)
        if time.monotonic() - t > 25:
            run.note("a lyrics request took %.0f s" % (time.monotonic() - t))
        if random.random() < 0.2:  # skipping through an album with the lyrics open
            for s2 in random.sample(songs, min(6, len(songs))):
                c = http.client.HTTPConnection(srv.host, srv.port, timeout=5)
                c.request("GET", "/api/music/lyrics/%d?fetch=1" % s2["id"], headers=srv.headers())
                time.sleep(0.3)
                c.close()  # moved on: the player gives the look-up up
            run.did("music: skip")
        run.sleep(random.uniform(20, 40))
    loop(run, "music", one)

def hls(srv, run, url, seconds):
    """Plays an HLS address for a while: the playlist, then its segments, at about real time."""
    st, master = srv.call("GET", url)
    if st != 200 or not isinstance(master, str):
        return False
    variant = next((l for l in master.splitlines() if l and not l.startswith("#")), None)
    base = url.split("?")[0].rsplit("/", 1)[0] + "/"
    query = ("?" + url.split("?", 1)[1]) if "?" in url else ""
    if variant and "#EXT-X-STREAM-INF" in master:
        pl_url = variant if variant.startswith("/") else base + variant
        if "?" not in pl_url:
            pl_url += query
    else:
        pl_url = url
    seen, end = set(), time.time() + seconds
    while time.time() < end and not run.stop.is_set():
        st, pl = srv.call("GET", pl_url, timeout=60)
        if st != 200 or not isinstance(pl, str):
            run.sleep(2)
            continue
        pbase = pl_url.split("?")[0].rsplit("/", 1)[0] + "/"
        segs = [l for l in pl.splitlines() if l and not l.startswith("#")]
        for sgm in [x for x in segs if x not in seen][:3]:
            seen.add(sgm)
            seg = sgm if sgm.startswith("/") else pbase + sgm
            if "?" not in seg:
                seg += query
            st, _ = srv.call("GET", seg, raw=True, timeout=120)
            run.did("segments", st == 200)
        run.sleep(4)
    return True

def transcode(srv, run, lib):
    films = [i for i in items(srv, lib) if (i.get("height") or 0) >= 1080] or items(srv, lib)
    if not films:
        run.note("no films: the transcode is left out")
        return

    def one():
        f = random.choice(films)
        start = random.randint(0, max(0, min(600, int(f.get("runtimeSec") or 0) - 200)))
        st, play = srv.call("POST", "/api/items/%d/playback" % f["id"], {"profile": "web", "forceTranscode": True, "startSec": start})
        run.did("transcode: play", st == 200)
        if st == 200 and isinstance(play, dict) and play.get("protocol") == "hls":
            hls(srv, run, play["url"], 180)
            srv.call("DELETE", "/api/hls/%s" % play["streamId"])
        run.sleep(random.uniform(20, 60))
    loop(run, "transcode", one)

def channel(srv, run):
    st, g = srv.call("GET", "/api/guide")
    chans = listing(g)
    if not chans:
        st, sug = srv.call("GET", "/api/channels/suggestions")
        for s in (sug if isinstance(sug, list) else [])[:5]:
            st, _ = srv.call("POST", "/api/channels", {"name": s["name"], "enabled": True, "definition": s["definition"]})
            if st == 200:
                st, g = srv.call("GET", "/api/guide")
                chans = listing(g)
                break
    if not chans:
        run.note("no channel could be made: the channel is left out")
        return
    key = chans[0]["key"]
    viewer = "soak-" + secrets.token_hex(4)

    def one():
        st, t = srv.call("GET", "/api/tv/%s/tune?viewer=%s" % (key, viewer), timeout=60)
        run.did("channel: tune", st in (200, 503))
        if st == 503:  # still starting
            run.sleep(5)
            return
        if st == 200 and isinstance(t, dict) and t.get("url"):
            hls(srv, run, t["url"], 180)
        srv.call("POST", "/api/tv/%s/leave?viewer=%s" % (key, viewer))
        run.sleep(random.uniform(30, 90))
    loop(run, "channel", one)

def dvd(srv, run, lib):
    discs = [i for i in items(srv, lib) if str(i.get("kind", "")).startswith("dvd")]
    if not discs:
        run.note("no DVDs: the DVD menu is left out")
        return
    gone = threading.Event()

    def one():
        if gone.is_set():
            run.stop.wait()
            return
        d = random.choice(discs)
        st, s = srv.call("POST", "/api/items/%d/dvd" % d["id"], {"mode": "menu"}, timeout=60)
        if st == 424:
            run.note("the DVD module isn't installed here: the DVD menu is left out")
            gone.set()
            return
        run.did("dvd: open", st == 200)
        if st == 200 and isinstance(s, dict) and s.get("sessionId"):
            sid = s["sessionId"]
            for _ in range(3):
                if run.sleep(10):
                    break
                st, _ = srv.call("POST", "/api/dvd/%s/action" % sid, {"type": "press", "button": 1, "positionSec": 0})
                run.did("dvd: press", st == 200)
            srv.call("DELETE", "/api/dvd/%s" % sid)
        run.sleep(random.uniform(60, 120))
    loop(run, "dvd", one)

def pages(srv, run, libs, n):
    paths = ["/", "/api/status", "/api/home", "/api/libraries", "/api/guide", "/api/system/tasks"]
    for lid in libs.values():
        paths += ["/api/items?library=%d&limit=60" % lid, "/api/items?library=%d&limit=60&sort=added" % lid]
    if "Music" in libs:
        paths += ["/api/music/songs?library=%d&limit=60" % libs["Music"]]

    def one():
        p = random.choice(paths)
        st, _ = srv.call("GET", p)
        run.did("pages", st == 200)
        if st != 200:
            run.note("%s answered %d" % (p, st))
        run.sleep(random.uniform(1, 4))
    for _ in range(n):
        loop(run, "pages", one)

def scans(srv, run, libs, every_min):
    def one():
        if run.sleep(every_min * 60):
            return
        for lid in libs.values():
            st, _ = srv.call("POST", "/api/libraries/%d/scan" % lid)
            run.did("scan", st in (200, 202, 204))
    loop(run, "scan", one)

# ---------------------------------------------------------------- measuring

def descendants(pid):
    """The names of every process started by pid, and by those."""
    kids = {}
    if not os.path.isdir("/proc"):
        return []  # not Linux: FFmpeg processes aren't counted
    for d in os.listdir("/proc"):
        if not d.isdigit():
            continue
        try:
            with open("/proc/%s/stat" % d) as f:
                stat = f.read()
        except OSError:
            continue
        m = re.match(r"\d+ \((.*)\) \S (\d+)", stat)
        if m:
            kids.setdefault(int(m.group(2)), []).append((int(d), m.group(1)))
    out, todo = [], [pid]
    while todo:
        for c, name in kids.get(todo.pop(), []):
            out.append(name)
            todo.append(c)
    return out

def sample(srv, pid, library_page):
    row = {"t": time.time()}
    try:
        st, m = srv.call("GET", "/api/system/metrics/now", timeout=30)
    except Exception:  # noqa: BLE001 - the answer times below say why
        st, m = 0, None
    proc = (m.get("process") or {}) if isinstance(m, dict) else {}
    row["heap_mb"] = (proc.get("heapAlloc") or 0) / 2**20
    row["rss_mb"] = (proc.get("rss") or 0) / 2**20
    row["goroutines"] = proc.get("goroutines") or 0
    try:
        row["open_files"] = len(os.listdir("/proc/%d/fd" % pid))
    except OSError:
        row["open_files"] = proc.get("handles") or 0
    row["ffmpeg"] = sum(1 for n in descendants(pid) if n.startswith(("ffmpeg", "ffprobe")))
    row["health_s"], err = srv.timed("/api/health", timeout=30)
    if err:
        row["health_err"] = err
    row["page_s"], err = srv.timed(library_page, timeout=60)
    if err:
        row["page_err"] = err
    return row

MEASURES = [("heap_mb", "Go heap (MB)"), ("rss_mb", "Process memory (MB)"), ("goroutines", "Goroutines"),
            ("open_files", "Open files"), ("ffmpeg", "FFmpeg processes"), ("health_s", "/api/health (s)"), ("page_s", "Library page (s)")]
# What may not keep rising, and by how much at least before it counts.
RISING = {"heap_mb": 8, "rss_mb": 24, "goroutines": 20, "open_files": 20}

def judge(rows, args, counts=None):
    """Says what failed. A measure fails when the median of its last third is above its first
    third by more than --growth and an amount that matters, with the middle third in between:
    rising, not one spike."""
    fails = []
    if not rows:
        return ["no samples were taken"]
    start = rows[0]["t"] + args.warmup * 60
    steady = [r for r in rows if r["t"] >= start and not r.get("idle")]
    if len(steady) >= 9:
        third = len(steady) // 3
        for key, floor in RISING.items():
            a = statistics.median(r[key] for r in steady[:third])
            b = statistics.median(r[key] for r in steady[third:2 * third])
            c = statistics.median(r[key] for r in steady[-third:])
            if c > a * (1 + args.growth) and c - a >= floor and b >= a:
                fails.append("%s keeps rising: %.0f, then %.0f, then %.0f" % (key, a, b, c))
    else:
        fails.append("too few samples after the warm-up to judge a trend (%d)" % len(steady))
    for r in rows:
        when = time.strftime("%H:%M", time.localtime(r["t"]))
        if r.get("health_err") or r["health_s"] > args.max_health:
            fails.append("%s: /api/health took %.1f s %s" % (when, r["health_s"], r.get("health_err", "")))
        if r.get("page_err") or r["page_s"] > args.max_page:
            fails.append("%s: the library page took %.1f s %s" % (when, r["page_s"], r.get("page_err", "")))
    for what, (ok, bad) in sorted((counts or {}).items()):
        if bad >= 5 and bad > (ok + bad) * 0.05:
            fails.append("%s failed %d times out of %d" % (what, bad, ok + bad))
    idle = [r for r in rows if r.get("idle")]
    if idle and idle[-1]["ffmpeg"] > 0:
        fails.append("%d FFmpeg processes still running after everyone left" % idle[-1]["ffmpeg"])
    return fails

# ---------------------------------------------------------------- the report

def svg(rows):
    """Small line charts, one a measure: no plotting library to install."""
    w, h, cols = 360, 150, 2
    W, H = cols * (w + 20) + 20, ((len(MEASURES) + 1) // cols) * (h + 50) + 20
    out = ['<svg xmlns="http://www.w3.org/2000/svg" width="%d" height="%d" font-family="sans-serif" font-size="11">' % (W, H),
           '<rect width="100%" height="100%" fill="#fff"/>']
    t0 = rows[0]["t"] if rows else 0
    span = max(rows[-1]["t"] - t0, 1) if rows else 1
    for i, (key, label) in enumerate(MEASURES):
        x0, y0 = 20 + (i % cols) * (w + 20), 20 + (i // cols) * (h + 50)
        vals = [r.get(key, 0) for r in rows]
        top = max(vals + [1e-9]) * 1.1
        out.append('<text x="%d" y="%d" fill="#222">%s, highest %.2f</text>' % (x0, y0 + 10, label, max(vals + [0])))
        out.append('<rect x="%d" y="%d" width="%d" height="%d" fill="none" stroke="#bbb"/>' % (x0, y0 + 16, w, h))
        pts = " ".join("%.1f,%.1f" % (x0 + (r["t"] - t0) / span * w, y0 + 16 + h - r.get(key, 0) / top * h) for r in rows)
        out.append('<polyline points="%s" fill="none" stroke="#5b3cc4" stroke-width="1.5"/>' % pts)
        out.append('<text x="%d" y="%d" fill="#666">%.0f min</text>' % (x0 + w - 44, y0 + h + 30, span / 60))
    out.append("</svg>")
    return "\n".join(out)

def span(hours):
    return "%.1f hours" % hours if hours >= 1 else "%.0f minutes" % (hours * 60)

def report(path, args, rows, fails, run, version, ended):
    busy = [r for r in rows if not r.get("idle")]
    lasted = (busy[-1]["t"] - busy[0]["t"]) / 3600 if busy else 0
    lines = ["# Soak run", "",
             "%s on %s, started %s." % (version, os.uname().sysname, time.strftime("%Y-%m-%d %H:%M", time.localtime(rows[0]["t"])) if rows else "?"),
             "Ran %s (asked for %s), %d samples, %d page clients. %s" % (span(lasted), span(args.hours), len(rows), args.clients, ended), "",
             "**%s**" % ("PASSED" if not fails else "FAILED"), ""]
    lines += ["- " + f for f in fails[:40]] + ([""] if fails else [])
    lines += ["![graph](graph.svg)", "", "| Measure | Start | Highest | End |", "|---|---|---|---|"]
    steady = [r for r in rows if not r.get("idle")] or rows
    for key, label in MEASURES:
        v = [r.get(key, 0) for r in steady]
        if v:
            lines.append("| %s | %.2f | %.2f | %.2f |" % (label, v[0], max(v), v[-1]))
    idle = [r for r in rows if r.get("idle")]
    if idle:
        lines += ["", "After everyone left: %d FFmpeg processes, %d goroutines, %.0f MB heap." % (idle[-1]["ffmpeg"], idle[-1]["goroutines"], idle[-1]["heap_mb"])]
    lines += ["", "| Activity | Done | Failed |", "|---|---|---|"]
    for k, (ok, bad) in sorted(run.counts.items()):
        lines.append("| %s | %d | %d |" % (k, ok, bad))
    if run.notes:
        lines += ["", "Notes:", ""] + ["- " + n for n in run.notes[:60]]
    with open(path, "w") as f:
        f.write("\n".join(lines) + "\n")

# ---------------------------------------------------------------- the run

def main():
    args = parse_args()
    if args.minutes:
        args.hours = args.minutes / 60
        args.warmup = min(args.warmup, args.minutes / 4)
    os.makedirs(args.out, exist_ok=True)
    data = os.path.join(args.out, "data")
    if os.path.exists(os.path.join(data, "wms.db")):
        print("%s holds a data folder already: use a new --out" % args.out, file=sys.stderr)
        return 2
    if args.make_media:
        args.media = os.path.abspath(os.path.join(args.out, "media"))
        print("making a small library in %s" % args.media, flush=True)
        make_media(args.media, args.ffmpeg or "ffmpeg")
    run = Run()
    lyrics = Threaded(("127.0.0.1", 0), Lyrics)
    threading.Thread(target=lyrics.serve_forever, daemon=True).start()
    cmd = [args.binary, "serve", "-data", data, "-listen", "127.0.0.1:%d" % args.port, "-open-browser=false"]
    if args.ffmpeg:
        cmd += ["-ffmpeg", args.ffmpeg]
    log = open(os.path.join(args.out, "server.log"), "wb")
    proc = subprocess.Popen(cmd, stdout=log, stderr=subprocess.STDOUT)
    srv = Server(args.port)
    for _ in range(300):
        if proc.poll() is not None:
            print("the server stopped as it started: see server.log", file=sys.stderr)
            return 2
        if srv.timed("/api/health", timeout=5)[1] is None:
            break
        time.sleep(1)
    else:
        print("the server didn't answer within 5 minutes of starting", file=sys.stderr)
        proc.kill()
        return 2
    st, status = srv.call("GET", "/api/status")
    version = "WittyMediaServer " + str(status.get("version", "?")) if isinstance(status, dict) else "WittyMediaServer"
    print("%s is up; setting up libraries (the first scans can take a while)" % version, flush=True)
    try:
        libs = setup(srv, run, args.media, lyrics.server_address[1])
    except Exception as e:  # noqa: BLE001
        print("setting up failed: %s" % e, file=sys.stderr)
        proc.kill()
        return 2
    page = "/api/items?library=%d&limit=60" % (libs.get("Movies") or next(iter(libs.values()), 1))
    if "Music" in libs:
        music(srv, run, libs["Music"])
    if "Movies" in libs:
        transcode(srv, run, libs["Movies"])
    channel(srv, run)
    if "DVD" in libs:
        dvd(srv, run, libs["DVD"])
    pages(srv, run, libs, args.clients)
    scans(srv, run, libs, args.scan_every)

    rows, ended = [], "It ran to the end."
    csv = open(os.path.join(args.out, "samples.csv"), "w")
    csv.write("time," + ",".join(k for k, _ in MEASURES) + ",idle\n")
    stop_now = threading.Event()
    signal.signal(signal.SIGINT, lambda *_: stop_now.set())
    signal.signal(signal.SIGTERM, lambda *_: stop_now.set())

    def take(idle=False):
        r = sample(srv, proc.pid, page)
        r["idle"] = idle
        rows.append(r)
        csv.write("%d,%s,%d\n" % (r["t"], ",".join("%.3f" % r.get(k, 0) for k, _ in MEASURES), idle))
        csv.flush()
        print(time.strftime("%H:%M"), " ".join("%s=%.2f" % (k, r.get(k, 0)) for k, _ in MEASURES), flush=True)

    end = time.time() + args.hours * 3600
    while time.time() < end and not stop_now.is_set():
        if proc.poll() is not None:
            ended = "The server stopped by itself (exit %s)." % proc.returncode
            break
        take()
        stop_now.wait(args.sample)
    if stop_now.is_set():
        ended = "Stopped early by hand."
    run.stop.set()
    fails = []
    if proc.poll() is None:
        # Everyone leaves. Streams pause after 90 seconds without requests: by then, no FFmpeg.
        print("everyone has left; waiting for the server to tidy up", flush=True)
        time.sleep(150)
        take(idle=True)
        proc.send_signal(signal.SIGTERM)
        try:
            proc.wait(60)
        except subprocess.TimeoutExpired:
            fails.append("the server didn't stop within 60 seconds of being asked")
            proc.kill()
    else:
        fails.append(ended)
    csv.close()
    log.close()
    with open(os.path.join(args.out, "server.log"), errors="replace") as f:
        text = f.read()
    for bad in ("the server stopped answering", "level=CRASH", "panic:", "fatal error:", "DATA RACE"):
        n = text.count(bad)
        if n:
            fails.append("the server's log says %r %d times" % (bad, n))
    fails = judge(rows, args, run.counts) + fails
    with open(os.path.join(args.out, "graph.svg"), "w") as f:
        f.write(svg(rows))
    report(os.path.join(args.out, "report.md"), args, rows, fails, run, version, ended)
    print("\n".join(["PASSED" if not fails else "FAILED"] + fails[:20]))
    print("report: %s" % os.path.join(args.out, "report.md"))
    return 1 if fails else 0

if __name__ == "__main__":
    sys.exit(main())
