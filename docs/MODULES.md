# Plugins and apps

WittyMediaServer is being split into **plugins** (modules): each kind of media is a
self-contained piece that runs **inside the server**, where everything is in one place with
accounts, sharing and remote access, or **inside a standalone app** built for one job. This page
lists the family, the rules every plugin follows, and the order the work is done in.

## The family

### Apps

Each app has its own repository.

| App | What it is |
|---|---|
| **WittyMediaServer** | The host: accounts and profiles, libraries and scanning, metadata, transcoding and streaming, remote access, Live TV. Every plugin below runs in it. |
| **WittyMediaPlayer** | A desktop player for local video and audio files, folders, disc images and discs. No server or accounts. Discs play with full menus through the WittyDVD and WittyBluray plugins. |
| **WittyReader** | A desktop and web app for reading and annotating local books, PDFs and comics, built from WittyEbook, WittyPDF and WittyComics. |
| **WittyMediaClient** | Apps that connect to a server: desktop (including Windows 7), Android TV, Apple TV, phones. |
| **WittyMediaRemoteMedia** | An agent that shares NAS folders with a server. |

### Plugins

Each plugin has its own repository, separate from the apps that use it.

| Plugin | What it does | Used by |
|---|---|---|
| **WittyInk** | What every reader shares: annotations with undo, highlight colours and their meanings, notes panel, definitions, citations, and handwriting with pen pressure | WittyEbook, WittyPDF, WittyComics |
| **WittyEbook** | Reflowable books (EPUB; more formats later): book view, typography, study tools | WittyMediaServer, WittyReader |
| **WittyPDF** | PDFs: selectable text, zoom, outline, thumbnails, search, study tools | WittyMediaServer, WittyReader |
| **WittyComics** | Comics and manga: pages, spreads, vertical strips, either reading direction | WittyMediaServer, WittyReader |
| **WittyDisc** | Reads DVD and Blu-ray disc images (UDF 1.02 and 2.50, ISO 9660) without mounting them | WittyDVD, WittyBluray |
| **WittyDVD** | DVD-Video engine: menus, navigation, angles, subtitles. Go library + CLI + diagnostics | WittyMediaServer, WittyMediaPlayer |
| **WittyBluray** | The same for Blu-ray, including 3D discs | WittyMediaServer, WittyMediaPlayer |
| **WittyIntegratedFileBrowser** | Choosing a server's folders from any device: a Go listing of drives, folders and files, and the React window that shows it (in `web/`). Read-only | WittyMediaServer |
| **WittyPhotos** | What a photo says about itself, without decoding it: EXIF, dimensions and the preview in a RAW file. Go, no dependencies, fuzz-tested | WittyMediaServer |
| **WittyTuner** | TV tuners and IPTV behind one interface: network tuners, M3U playlists and streams, Linux DVB, V4L2 and other capture cards, SAT>IP, external backends, a custom command. Also reads XMLTV and the guide data in broadcasts. Refuses encrypted channels | WittyMediaServer |

### Server libraries

The server's libraries carry the name where it reads naturally:

| Library | Plugin |
|---|---|
| Films, shows and anime | **WittyVideo** |
| Discs | **WittyDVD**, **WittyBluray** |
| Music and audiobooks | **WittyAudio** |
| Podcasts | **WittyPodcasts** |
| Books, PDFs and comics | **WittyEbook**, **WittyPDF**, **WittyComics** |
| Photos | **WittyPhotos** |
| Vertical videos | **WittyShorts** |
| Creator videos, music videos and concerts | **WittyChannels** |
| Live TV and DVR | **WittyTV** |

The plumbing underneath (accounts, transcoding, metadata, settings, logging) keeps plain
names. So do features inside a plugin, such as the selection toolbar or the eraser.

## How the code is organised

Everything is developed **in this repository**, so a change that touches the server and a plugin
is one commit and one test run. Each plugin lives in its own folder, and that folder is
**mirrored to the plugin's public repository** when a version is released. Apps that are not
the server live under `apps/` and are mirrored the same way.

```
web/src/modules/            web plugins (React)
  wittyink/                 → github.com/wittywilla/WittyInk
  wittyebook/               → github.com/wittywilla/WittyEbook
  wittypdf/                 → github.com/wittywilla/WittyPDF
  wittycomics/              → github.com/wittywilla/WittyComics
modules/                    Go plugins
  wittydisc/                → github.com/wittywilla/WittyDisc
  wittydvd/                 → github.com/wittywilla/WittyDVD
  wittybluray/              → github.com/wittywilla/WittyBluray
  wittyintegratedfilebrowser/ → github.com/wittywilla/WittyIntegratedFileBrowser (Go, with its React half in web/)
  wittyphotos/              → github.com/wittywilla/WittyPhotos
  wittytuner/               → github.com/wittywilla/WittyTuner
apps/
  wittyreader/              → github.com/wittywilla/WittyReader
  wittymediaplayer/         → github.com/wittywilla/WittyMediaPlayer
```

Each plugin and app folder carries what its own repository needs:

- **README.md:** what it does.
- **wiki/:** the full guide, published to the repository's GitHub wiki.
- **LICENSE:** Apache-2.0, the same as WittyMediaServer.
- **NOTICE:** credits *"<name>, developed by WittyWilla"*. The licence requires anyone who
  redistributes the code or builds on it to keep that attribution.

`scripts/mirror.sh` publishes the folders and their wikis.

### Releasing the Go modules

WittyDisc, WittyDVD and WittyBluray are versioned Go modules, so anyone can
`go install github.com/wittywilla/wittydvd/cmd/wittydvd@latest` without this repository.
In here, `go.work` builds them against each other's local source, and the server's `go.mod`
points at the same folders with `replace` directives. The engines' own `go.mod` files require
released versions only, which is what their mirrors are built from.

`scripts/mirror.sh <Repository> <vX.Y.Z>` publishes a module and tags what it published. It
refuses when the folder has uncommitted changes, when its `go.mod` has a `replace` directive, or
when the module does not build and pass its tests on its own (`GOWORK=off`). Tags never move.
WittyDisc goes first, because the engines require it:

```bash
scripts/mirror.sh WittyDisc v0.2.0
(cd modules/wittydvd && GOWORK=off go get github.com/wittywilla/wittydisc@v0.2.0 && GOWORK=off go mod tidy)
(cd modules/wittybluray && GOWORK=off go get github.com/wittywilla/wittydisc@v0.2.0 && GOWORK=off go mod tidy)
git commit -am "WittyDVD and WittyBluray require WittyDisc v0.2.0"
scripts/mirror.sh WittyDVD v0.2.0
scripts/mirror.sh WittyBluray v0.2.0
```

An engine whose code has not changed since its last release needs no new tag. Versions stay
below v1 while the APIs settle, so a minor version may change them.

### Rules every plugin follows

1. **No reaching into the host.** A plugin imports only its own files, its declared libraries
   and the front door (`index`) of plugins it builds on. Everything else (files, saving progress
   and notes, accounts, settings) comes through a small **host interface** that the server and
   each app implement. `web/scripts/check-modules.mjs` runs before every web build and fails it
   if a plugin imports anything else.
2. **Built in, not downloaded.** Plugins are compiled into each app. Nothing is fetched and run
   at runtime, which keeps the server safe and works the same on every platform. (Go has no
   portable way to load plugins at runtime anyway.)
3. **Owns its data.** Server-side, a plugin's tables carry its name and its migrations run with
   the server's. In an app, the same data lives on the device and can be exported and imported,
   so notes made in WittyReader can move into the server and back.
4. **Light by default.** Heavy parts load only when used (pdf.js loads only when a PDF is
   opened). Every plugin is expected to run on low-end hardware.
5. **Untrusted files stay boxed in.** Books, PDFs and disc menus are untrusted input: no scripts
   run, documents are shown in sandboxed frames with a strict Content-Security-Policy, and
   parsers have size and time limits.

### The reader host (web)

WittyEbook, WittyPDF and WittyComics talk to the app around them only through
`ReaderHost` (in `web/src/modules/wittyink/host.tsx`):

```ts
interface ReaderHost {
  key: string; book: { title; author?; year? }; format: 'epub' | 'pdf' | 'cbz'; reading?: string
  start: number; saveProgress(position, total): void
  annotations: { list(); add(a); update(a, patch); remove(a) }
  pdf?: { url?; data? }                                  // what pdf.js opens
  comic?: { pages; pageUrl(n) }
  epub?: { info; chapterUrl(path); search(q) }
  define?(word): Promise<Definition[]>                   // optional dictionary
  toast(message): void; close(): void
}
```

In the server (`web/src/pages/Reader.tsx`) the host is backed by the HTTP API: notes and the
reading position are kept per account and follow the reader to every device. In WittyReader it
is backed by local files and storage on the device, with optional sign-in to a server to sync.

### The plugin interface (server, Go)

```go
type Module interface {
    ID() string                         // "wittyaudio", "wittyphotos"…
    LibraryTypes() []LibraryType        // what the setup wizard and settings offer
    Migrations() []string               // its own tables, prefixed with its ID
    Scan(ctx, lib, files) error         // indexes the files it understands
    Routes(mux *http.ServeMux, h Host)  // its API, behind the server's auth and access rules
}
```

`Host` gives a plugin the server's services: the database, the signed-in account and its access
rules, settings, logging, FFmpeg and the transcoder. Access control stays in the host, so a
plugin can never show an item that an account is not allowed to see.

## How the apps run

- **WittyReader** is a web app at heart: the reader plugins with a host that opens files from
  the disk (or dropped on the window) and keeps notes on the device. It ships as a desktop app
  and can also be installed from the browser. No server needed.
- **WittyMediaPlayer** pairs a small native window with the same Go engine the server uses,
  running inside the app on the loopback address only, with a secret that changes every launch.
  Files never leave the computer. Discs play with full menus through the WittyDVD and WittyBluray
  plugins, with the same remote and disc insert animation as the server's player.
- Native windows use the system's own web view (WebView2 on Windows 10+, WebKit on macOS and
  Linux), which is far lighter than bundling a browser. The one exception is the Windows 7
  desktop client, which needs an older bundled browser engine and stays a thin client.

## Order of work

1. **Reader plugins** (done): WittyInk, WittyEbook, WittyPDF and WittyComics are separate
   plugins behind `ReaderHost`, used by the server.
2. **WittyReader**: the app under `apps/wittyreader`, with EPUBs unpacked in the app and notes
   stored on the device. Then the mirror repositories for the plugins and the app.
3. **Go plugins** (WittyDVD done): WittyDVD, its ISO reader and its CLI live in
   `modules/wittydvd` with their own `go.mod`; the server uses it through a `replace` directive,
   and `wms dvd …` and the standalone `wittydvd` tool share one CLI package. WittyBluray
   (`modules/wittybluray`) and WittyDisc (`modules/wittydisc`) have the same shape.
4. **Server plugin interface**: books, discs and audio move behind `Module` first, then the rest
   as each is built or touched.
5. **WittyMediaPlayer**: local engine, native window and the existing player interface, with the
   WittyDVD and WittyBluray plugins for discs.
