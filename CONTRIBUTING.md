# Contributing to WittyMediaServer

Thanks for helping! This page explains how to build, test and change the project, even if you
haven't worked on a Go or React project before.

## What you need

| Tool | Version | Get it |
|---|---|---|
| Go | 1.26 or newer | <https://go.dev/dl/> |
| Node.js | 22 or newer | <https://nodejs.org/> |
| FFmpeg | any recent build (only to *run* the server) | <https://ffmpeg.org/download.html>, or the copy inside a release archive |
| Git | any | <https://git-scm.com/downloads> |

## Build and run

```bash
git clone https://github.com/wittywilla/WittyMediaServer
cd WittyMediaServer
cd web && npm install && npm run build && cd ..   # the web interface -> web/dist
go build -o wms ./cmd/wms                          # the server (wms.exe on Windows)
./wms serve -ffmpeg /path/to/ffmpeg -data ./devdata -listen :9797
```

Open <http://localhost:9797>. Use a separate `-data` folder while developing so your real
library is never touched.

**Working on the web interface?** Run the server with `-dev` (serves `web/dist` from disk) and
rebuild with `npm run build` after each change, or run `npm run dev` in `web/` for hot reload
(it proxies `/api` to `localhost:9797`).

## Tests and checks

Run these before sending a change:

```bash
gofmt -l .            # must print nothing
go vet ./...
go test ./...
cd web && npx tsc --noEmit -p . && npm run build
```

Real-disc test: `WMS_TEST_ISO=/path/to/disc.iso go test ./internal/wittydvd`.
To check a whole disc's menus: `go run ./cmd/wms dvd crawl /path/to/disc.iso`.

## Where things are

| Folder | What it is |
|---|---|
| `cmd/wms` | the server and command-line tools |
| `cmd/wms-tray` | the tray / menu-bar launcher |
| `internal/wittydvd` | **WittyDVD**: IFO parser, command VM, navigator, NAV packs, sub-pictures |
| `internal/dvdsession` | turns WittyDVD steps into what the browser shows (streams, buttons, subtitles) |
| `internal/library` | libraries, scanner, TV parsing, TMDB/TheTVDB clients, NFO read/write |
| `internal/hls`, `internal/ffmpeg` | on-demand HLS transcoding, encoder selection |
| `internal/server` | the HTTP API |
| `internal/themes`, `themes/` | theme ZIP handling; built-in theme sources |
| `web/src` | the React + TypeScript interface |
| `docs/` | user guides |

## Guidelines

- Keep the DVD engine free of third-party player code (no libdvdnav/libdvdread): it is written
  from the public DVD-Video structure.
- Match the surrounding code style; explain *why* in comments, not *what*.
- New settings need a sensible default, a tooltip in the UI and a line in the docs.
- For disc bugs, attach `wms dvd info` and `wms dvd crawl` output — never disc images.
- Contributions are licensed under the Apache License 2.0 (see `LICENSE`).

## Sending a change

1. Fork the repository on GitHub and clone your fork.
2. Create a branch: `git checkout -b fix-menu-highlight`.
3. Commit with a clear message, push, and open a pull request against `master`.
4. CI runs the same checks as above.

New to Git/GitHub? See [docs/PUBLISHING.md](docs/PUBLISHING.md) for a step-by-step walkthrough.

---
<sub>WittyMediaServer 0.3.0-alpha documentation</sub>
