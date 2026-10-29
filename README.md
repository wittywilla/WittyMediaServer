# WittyMediaServer

A self-hosted media server with **real DVD menu playback** in any browser.
One static binary per platform (Windows, macOS, Linux, FreeBSD), FFmpeg bundled,
no runtimes to install. Also ships as a Docker image and Kubernetes manifests.

<p align="center">
  <em>Plex-style library · interactive DVD menus · HLS streaming with instant seeking · users &amp; resume</em>
</p>

## What makes it different

Most media servers play DVD images by picking "the longest title". WittyMediaServer
implements the **DVD-Video navigation model itself** — the same thing a hardware
player does:

- reads ISO 9660/UDF images (and `VIDEO_TS` folders) directly, no mounting
- parses the VMG/VTS IFO tables (program chains, cells, chapters, angles, audio/subtitle tables)
- runs the DVD **virtual machine** (16 GPRM + 24 SPRM registers, the full command set: links, jumps, calls, compares, arithmetic, timers)
- follows **first-play → menus → titles** exactly as the disc's own programs dictate
- extracts **button geometry** from NAV packs and decodes the **sub-picture RLE** so highlights look exactly like on a real player
- streams the exact byte ranges of each cell through FFmpeg as HLS, so menus (even animated, looping ones) and features play in the browser without transcoding the whole disc first

The client overlays the decoded highlights and sends remote-control style input
(arrows / OK / Menu / Title / Resume / chapter skip) back to the navigator.
CSS-scrambled discs are refused; decrypt them before adding them.

## Quick start

### Download (recommended)

1. Grab the archive for your platform from the Releases page, e.g. `wittymediaserver-1.0.0-linux-amd64.tar.gz`.
2. Extract it. It contains `wms`, `bin/ffmpeg`, `bin/ffprobe` and an installer.
3. Run it:
   - **Try it**: `./wms` (Windows: double-click `start-wms.cmd`) → open <http://localhost:9797>
   - **Install as a service**: `sudo ./install.sh` (Linux systemd / macOS launchd / FreeBSD rc.d) or `install.ps1` in an elevated PowerShell on Windows.
4. Create the admin account, add a library pointing at your movies / DVD ISOs, done.

### Docker

```bash
docker run -d --name wittymediaserver -p 9797:9797 \
  -v wms-config:/config -v /path/to/media:/media:ro \
  ghcr.io/wittywilla/wittymediaserver:latest
```

or `docker compose up -d` with the included [`docker-compose.yml`](docker-compose.yml).

### Kubernetes

```bash
kubectl apply -k deploy/k8s
```

Edit `deploy/k8s/deployment.yaml` for your media volume (NFS/hostPath/CSI) and
`ingress.yaml` for your hostname. The transcode cache uses an in-memory `emptyDir`.

## Library organisation

Anything video is picked up: `.mkv .mp4 .avi .m2ts …`, **`.iso`/`.img` DVD images**
and **`VIDEO_TS` folders**. Metadata comes from, in order of precedence:

1. `<name>.metadata.json` next to the file (`title`, `year`, `overview`, `genres`, `poster`, …)
2. Kodi-style `<name>.nfo` / `movie.nfo`
3. local artwork (`poster.jpg`, `<name>-poster.jpg`, `fanart.jpg`, `folder.jpg`, …)
4. the file/folder name (`Title (Year).ext`)
5. **TMDB** (optional — paste an API key in *Settings → Server*; overviews, posters, backdrops, ratings)
6. a generated thumbnail from the feature as the last resort

## Playback

| Source | How it plays |
|---|---|
| MP4/WebM with browser codecs | direct play with range requests |
| anything else | H.264/AAC HLS, server-generated playlist, keyframes forced on segment boundaries → seeking anywhere restarts FFmpeg at that segment (≈0.5 s) |
| DVD image/folder | *Play* → main feature; *Play with DVD menu* → the disc's own first-play/menus; per-title and per-chapter play from the detail page |

Hardware H.264 encoding (NVENC, Quick Sync, AMF, VideoToolbox, VAAPI) can be
enabled in *Settings → Server*. Interlaced sources are deinterlaced (`bwdif`),
anamorphic DVDs are output at the correct aspect, DVD subtitles are burned in.

### DVD player keys

`←↑→↓` move the highlight · `Enter` activate · `m` root menu · `t` title menu ·
`n`/`p` chapter · `Space` pause · `f` fullscreen · `Esc` back

## Clients

- **Web** — served by the server; installable as a PWA.
- **Desktop** (Windows/macOS/Linux/FreeBSD) — [`clients/desktop`](clients/desktop): Electron shell with media keys.
- **iOS/Android** — [`clients/mobile`](clients/mobile): Capacitor shell; native HLS playback.

All logic lives in the server; the clients are thin and never need updating for new features.

## Configuration

| Flag | Env | Default | |
|---|---|---|---|
| `-data` | `WMS_DATA_DIR` | per-OS app-data dir, `/config` in containers | database, cache, transcode temp |
| `-listen` | `WMS_LISTEN` | `:9797` | |
| `-ffmpeg` / `-ffprobe` | `WMS_FFMPEG` / `WMS_FFPROBE` | bundled `bin/` → PATH | |
| `-max-transcodes` | `WMS_MAX_TRANSCODES` | 4 | |
| `-log` | `WMS_LOG` | `info` | `debug` shows every FFmpeg command |
| | `WMS_TRANSCODE_DIR` | `<data>/transcode` | put on tmpfs/SSD |

Everything else (TMDB key, hardware encoding, languages, users) is in the UI.

## Command-line tools

```
wms disc ls   <image>             list files inside an ISO/UDF image
wms dvd info  <image>             titles, chapters, menus, languages
wms dvd walk  <image> [button…]   simulate first play and button presses
wms dvd pgc   <image> vtsm 1 1    disassemble a program chain
wms dvd render <image> <dir>      write menu highlight PNGs
```

## Building from source

Requirements: Go 1.26+, Node 22+. FFmpeg is only needed at runtime.

```bash
cd web && npm install && npm run build && cd ..
go build -o wms ./cmd/wms
./wms serve -ffmpeg /path/to/ffmpeg
```

Release archives for every platform (binary + bundled FFmpeg + installers):

```bash
scripts/build-release.sh 1.0.0            # or scripts\build-release.ps1 -Version 1.0.0
```

FreeBSD has no public static FFmpeg builds; `scripts/build-ffmpeg-static.sh`
compiles one (the release workflow does this in a FreeBSD VM).

## Architecture

```
cmd/wms                 entry point + disc CLI
internal/disc/isofs     ISO 9660 + UDF reader → byte extents
internal/disc/dvd       IFO parser · VM · navigator · NAV-pack/PCI · SPU decoder · menu composer
internal/dvdsession     per-viewer DVD player: presentations, overlays, title/menu HLS
internal/hls            on-demand FFmpeg HLS with server-side playlists and seek-restart
internal/ffmpeg         binary discovery, probing, ffconcat/subfile inputs, stills
internal/library        libraries, scanner, metadata (NFO/JSON/TMDB), progress
internal/server         REST API, auth, streaming endpoints, embedded SPA
web/                    React + TypeScript client (hls.js), embedded into the binary
deploy/k8s              Kustomize manifests · Dockerfile · docker-compose.yml
clients/                desktop (Electron) and mobile (Capacitor) shells
```

## Licence

MIT for WittyMediaServer itself. Bundled FFmpeg builds are GPL (they include
libx264); see [THIRD_PARTY_LICENSES.md](THIRD_PARTY_LICENSES.md). This product
uses the TMDB API but is not endorsed or certified by TMDB.
