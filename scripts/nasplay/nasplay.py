#!/usr/bin/env python3
# SPDX-License-Identifier: AGPL-3.0-or-later
# Copyright (C) 2026 WittyWilla and contributors
"""NAS playback check: plays an item from a server in a container the way a player does, at a
playback rate, and records stalls, how long each segment took, and how far ahead of the player
the server's conversion stayed. Python 3, nothing to install. See scripts/nasplay/README.md.

  nasplay.py setup --url http://127.0.0.1:19811 --container wms-nas
  nasplay.py play  --title "NAS Test" --rate 2 [--client exo] [--no-rate] [--fail-on-stall]

--client hlsjs is the web app (hls.js as it is set up there); exo is ExoPlayer as the Android app
set it up before 0.6.3 (8 s timeouts, 3 retries, then 3 new requests for the stream). The player
says its rate and buffers by it, as the players do from 0.8.5; --no-rate plays as they did before.
Prints one JSON line per run, and what happened along the way with -v.
"""
import argparse, json, os, re, subprocess, sys, threading, time, urllib.request, urllib.error

WEB_CAPS = {"containers": ["mp4", "webm", "mov"], "videoCodecs": ["h264", "vp8", "vp9", "av1"],
            "audioCodecs": ["aac", "mp3", "opus", "vorbis", "flac"], "hlsVideo": ["h264"], "fmp4Video": ["h264"],
            "hlsAudio": ["aac", "mp3"], "remux": True, "tenBit": False}

RATE = [1.0]  # the player's rate, sent with every request as the fixed players do

class API:
    def __init__(self, url, token=None):
        self.url, self.token = url.rstrip('/'), token
    def req(self, method, path, body=None, timeout=90, raw=False):
        u = path if path.startswith('http') else self.url + path
        data = json.dumps(body).encode() if body is not None else None
        r = urllib.request.Request(u, data=data, method=method)
        if data is not None: r.add_header('Content-Type', 'application/json')
        if self.token: r.add_header('Authorization', 'Bearer ' + self.token)
        if RATE[0] != 1: r.add_header('X-WMS-Playback-Rate', str(RATE[0]))
        with urllib.request.urlopen(r, timeout=timeout) as resp:
            b = resp.read()
            return b if raw else (json.loads(b) if b else None)

def dexec(container, *cmd):
    return subprocess.run(['docker', 'exec', container, *cmd], capture_output=True, text=True).stdout

def setup(a):
    api = API(a.url)
    for _ in range(120):
        try: api.req('GET', '/api/health', timeout=3); break
        except Exception: time.sleep(1)
    code = ''
    for _ in range(60):
        code = dexec(a.container, 'cat', '/config/setup-code.txt').strip()
        if code: break
        time.sleep(1)
    pw = 'nas-test-only-pw-' + os.urandom(6).hex()
    r = api.req('POST', '/api/setup', {"name": "nas", "password": pw, "setupCode": code, "acknowledgedLegal": True, "device": "nasplay"})
    api.token = r['token']
    api.req('POST', '/api/setup/complete', {})
    def tryreq(*x):
        try: return api.req(*x)
        except urllib.error.HTTPError as e: print('setup step', x[:2], e.code, e.read()[:300], file=sys.stderr)
    tryreq('PUT', '/api/settings', {"saveMetadata": "0", "saveWittyinfo": "0"})
    for name, typ, path in (("Movies", "movies", "/media/Movies"), ("Music", "music", "/media/Music")):
        lib = tryreq('POST', '/api/libraries', {"name": name, "type": typ, "paths": [path]})
        if lib: tryreq('POST', f"/api/libraries/{lib['id']}/scan", {})
    time.sleep(8)
    json.dump({"url": a.url, "token": api.token}, open(a.state, 'w'))
    print('ready', a.url)

def find_item(api, title):
    res = api.req('GET', "/api/items?limit=500")
    items = res.get('items', res) if isinstance(res, dict) else res
    for it in items:
        if title.lower() in (it.get('title') or '').lower():
            return it
    raise SystemExit('no item ' + title)

def parse_m3u8(text, base):
    segs, dur, ended, init = [], None, False, None
    for line in text.splitlines():
        line = line.strip()
        if line.startswith('#EXTINF:'): dur = float(line[8:].split(',')[0])
        elif line.startswith('#EXT-X-ENDLIST'): ended = True
        elif line.startswith('#EXT-X-MAP:'):
            m = re.search(r'URI="([^"]+)"', line); init = urllib.request.urljoin(base, m.group(1))
        elif line and not line.startswith('#'):
            segs.append((urllib.request.urljoin(base, line), dur))
    return segs, ended, init

class Lead(threading.Thread):
    """Samples the newest finished segment in the stream's folder (the server's lead)."""
    def __init__(self, container, stream_id, seg_sec):
        super().__init__(daemon=True); self.c, self.id, self.seg = container, stream_id, seg_sec
        self.newest, self.stop, self.cpu = -1, False, []
    def run(self):
        while not self.stop:
            out = dexec(self.c, 'sh', '-c', f'ls /config/transcode/ | grep "^{self.id}-r" | head -1 | xargs -I{{}} ls /config/transcode/{{}}')
            nums = [int(m.group(1)) for m in re.finditer(r'seg(\d+)\.(?:ts|m4s)\b', out)]
            self.newest = max(nums) if nums else -1
            time.sleep(1)

def play(a):
    st = json.load(open(a.state))
    api = API(st['url'], st['token'])
    it = find_item(api, a.title)
    body = {"profile": a.profile, "startSec": 0}
    if a.rate != 1 and not a.no_rate: body["rate"] = a.rate
    if a.profile == 'web': body["capabilities"] = WEB_CAPS
    if a.quality: body["quality"] = a.quality
    if a.force: body["forceTranscode"] = True
    t0 = time.time()
    pb = api.req('POST', f"/api/items/{it['id']}/playback", body)
    out = {"title": it["title"], "client": a.client, "rate": a.rate, "profile": a.profile, "decision": pb.get('decision'), "protocol": pb.get('protocol'),
           "encoder": (pb.get('output') or {}).get('encoder'), "duration": pb.get('durationSec'), "buffer": a.buffer}
    log = (lambda *x: print(f"[{time.time()-t0:6.1f}]", *x, file=sys.stderr)) if a.verbose else (lambda *x: None)
    if pb['protocol'] == 'file':
        out.update(play_file(api, pb, a, log)); print(json.dumps(out)); return
    url = urllib.request.urljoin(st['url'] + '/', pb['url'].lstrip('/'))
    text = api.req('GET', url, raw=True).decode()
    if '#EXT-X-STREAM-INF' in text:
        media = [l for l in text.splitlines() if l and not l.startswith('#')][0]
        url = urllib.request.urljoin(url, media); text = api.req('GET', url, raw=True).decode()
    segs, ended, init = parse_m3u8(text, url)
    growing = not ended
    sid = pb.get('streamId') or re.search(r'/hls/([^/]+)/', url).group(1)
    lead = Lead(a.container, sid, pb.get('streaming', {}).get('segmentSec') or 4); lead.start()
    if init: api.req('GET', init, raw=True, timeout=120)
    duration = float(pb.get('durationSec') or sum(d for _, d in segs))
    # the player
    pos, buf_end, nxt, started = 0.0, 0.0, 0, False
    stalls, stall_time, in_stall, waits, errors, leads = 0, 0.0, None, [], [], []
    fetching = {"busy": False}
    seg_starts = [0.0]
    for _, d in segs: seg_starts.append(seg_starts[-1] + (d or 0))
    # hls.js (the web app): 60 s per try, 6 retries with growing waits. ExoPlayer as the Android
    # app sets it up in 0.6.2: 8 s connect and read timeouts, 3 retries 0/1/2 s apart, then the
    # app's own 3 new requests for the stream at the same place, and a 500 ends it at once.
    exo = a.client == 'exo'
    exo_new = a.client == 'exo-new'  # the Android app from 0.6.3: 75 s per try, 6 retries
    if exo_new: a.exo_timeout = 75
    per_try, retries = (a.exo_timeout, 3) if exo else (75, 6) if exo_new else (60, 6)
    reopens = {"n": 0}
    def fetch(i, u):
        tries = 0
        while True:
            s = time.time()
            try:
                api.req('GET', u, raw=True, timeout=per_try); fetching['done'] = (i, time.time() - s); return
            except urllib.error.HTTPError as e:
                tries += 1; errors.append((round(time.time()-t0, 1), i, e.code))
                log('segment', i, 'HTTP', e.code, e.read()[:200])
                if tries > retries: fetching['fail'] = (i, e.code); return
                time.sleep(min((tries - 1) * 1, 5) if exo else min(1 * 2 ** (tries - 1), 8))
            except Exception as e:
                tries += 1; errors.append((round(time.time()-t0, 1), i, str(e)[:60]))
                log('segment', i, 'failed', str(e)[:80])
                if tries > retries: fetching['fail'] = (i, 'timeout'); return
                if exo: time.sleep(min((tries - 1) * 1, 5))
    last, end_reason = time.time(), None
    # The fixed players keep the same wait ahead at any speed: the buffer grows with the rate.
    target = a.buffer if a.no_rate else min(a.buffer * max(1, a.rate), 150 if (exo or exo_new) else 300)
    out["buffer"] = target
    tick = 0
    while True:
        now = time.time(); dt = now - last; last = now
        if growing and nxt >= len(segs) and tick % 10 == 0:
            text = api.req('GET', url, raw=True).decode(); segs, ended, _ = parse_m3u8(text, url)
            seg_starts = [0.0]
            for _, d in segs: seg_starts.append(seg_starts[-1] + (d or 0))
            if ended: growing = False
        if 'done' in fetching:
            i, w = fetching.pop('done'); waits.append(round(w, 2)); buf_end = seg_starts[i + 1]; nxt = i + 1; fetching['busy'] = False
            log('seg', i, 'took', round(w, 2), 'buffer', round(buf_end - pos, 1))
        if 'fail' in fetching:
            i, why = fetching.pop('fail')
            if exo and why != 500 and reopens['n'] < 3:
                # The app asks for the stream again at the same place, after 1, 4 and 6 s.
                reopens['n'] += 1; log('player error, reopen', reopens['n'], 'at', round(pos, 1))
                time.sleep([1, 4, 6][reopens['n'] - 1])
                buf_end = pos; nxt = i; fetching['busy'] = False
                api.req('POST', f"/api/items/{it['id']}/playback", dict(body, startSec=pos))
                last = time.time(); continue
            end_reason = f"stopped early: segment {i} failed ({why})"; break
        if not fetching['busy'] and nxt < len(segs) and buf_end - pos < target:
            fetching['busy'] = True
            threading.Thread(target=fetch, args=(nxt, segs[nxt][0]), daemon=True).start()
        if not started and buf_end - pos >= min(4, duration):
            started = True; out['startup'] = round(now - t0, 2)
        if started:
            if pos < buf_end:
                if in_stall is not None: stall_time += now - in_stall; in_stall = None
                pos = min(buf_end, pos + dt * a.rate)
            elif pos < duration - 0.3 and in_stall is None:
                in_stall = now; stalls += 1; log('STALL at', round(pos, 1))
        if not growing and nxt >= len(segs) and pos >= buf_end - 0.01:
            end_reason = 'end'; break
        if tick % 10 == 0 and lead.newest >= 0:
            leads.append(round(seg_starts[min(lead.newest + 1, len(segs))] - pos, 1))
        if now - t0 > a.timeout: end_reason = 'timeout'; break
        tick += 1
        time.sleep(0.1)
    lead.stop = True
    if in_stall is not None: stall_time += time.time() - in_stall
    wall = time.time() - t0
    out.update({"result": end_reason, "reached": round(pos, 1), "of": round(seg_starts[-1], 1), "stalls": stalls, "stallSec": round(stall_time, 1),
                "wall": round(wall, 1), "segments": len(segs), "waitMax": max(waits or [0]), "waitAvg": round(sum(waits) / max(len(waits), 1), 2),
                "reopens": reopens["n"], "leadMin": min(leads or [0]), "leadAvg": round(sum(leads) / max(len(leads), 1), 1), "errors": errors[:10]})
    try: api.req('DELETE', f"/api/hls/{sid}")
    except Exception: pass
    print(json.dumps(out))
    if a.fail_on_stall and (end_reason != 'end' or stalls or out["reached"] < out["of"] - 1):
        sys.exit(f"FAIL: {end_reason}, {stalls} stalls ({out['stallSec']} s), reached {out['reached']} of {out['of']}")

def play_file(api, pb, a, log):
    """Direct play: read the file in 2 MB ranges at the pace of a player at this rate."""
    u = urllib.request.urljoin(api.url + '/', pb['url'].lstrip('/'))
    dur = float(pb.get('durationSec') or 1)
    r = urllib.request.Request(u, method='HEAD'); r.add_header('Authorization', 'Bearer ' + api.token)
    size = int(urllib.request.urlopen(r).headers['Content-Length'])
    bps = size / dur  # bytes per media second
    got, pos, t0, last, stalls, stall_t, ins = 0, 0.0, time.time(), time.time(), 0, 0.0, None
    while pos < dur - 0.3:
        if (got / bps) - pos < a.buffer and got < size:
            rq = urllib.request.Request(u); rq.add_header('Authorization', 'Bearer ' + api.token)
            rq.add_header('Range', f'bytes={got}-{min(size, got + 2_000_000) - 1}')
            got += len(urllib.request.urlopen(rq, timeout=60).read())
        now = time.time(); dt = now - last; last = now
        if pos < got / bps:
            if ins: stall_t += now - ins; ins = None
            pos = min(got / bps, pos + dt * a.rate)
        elif ins is None: ins = now; stalls += 1
        time.sleep(0.05)
    return {"result": "end", "reached": round(pos, 1), "of": dur, "stalls": stalls, "stallSec": round(stall_t, 1), "wall": round(time.time() - t0, 1)}

if __name__ == '__main__':
    p = argparse.ArgumentParser()
    p.add_argument('cmd'); p.add_argument('--url', default='http://127.0.0.1:19811'); p.add_argument('--container', default='wms-nas')
    p.add_argument("--state", default=os.environ.get("NASPLAY_STATE", os.path.expanduser("~/nasplay.json")))
    p.add_argument('--title'); p.add_argument('--rate', type=float, default=1); p.add_argument('--profile', default='web')
    p.add_argument('--buffer', type=float, default=30, help='media seconds the player buffers ahead (hls.js maxBufferLength)')
    p.add_argument('--quality'); p.add_argument('--force', action='store_true'); p.add_argument('--timeout', type=float, default=900)
    p.add_argument('--client', default='hlsjs', choices=['hlsjs', 'exo', 'exo-new']); p.add_argument('--exo-timeout', type=float, default=8)
    p.add_argument('--no-rate', action='store_true', help='say nothing of the rate, as players before 0.8.5')
    p.add_argument('--fail-on-stall', action='store_true', help='exit 1 on any stall after the start, or an early end')
    p.add_argument('-v', '--verbose', action='store_true')
    a = p.parse_args()
    if a.cmd == 'play' and not a.no_rate: RATE[0] = a.rate
    {'setup': setup, 'play': play}[a.cmd](a)
