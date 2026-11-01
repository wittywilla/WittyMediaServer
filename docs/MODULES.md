# Plugins and apps

Each kind of media that isn't part of the server's core is a **plugin** (module): a project of its
own, with its own repository, version and releases. The server starts and is fully usable with none
of them. A plugin arrives at run time as a signed archive and runs as a **program of its own**, or in
the browser as a **web bundle**. The same plugin can also run **inside a standalone app** built for
one job. This page lists the family, the rules every plugin follows, and how they reach a server.

**From this release on.** Modules work with WittyMediaServer **0.6.0-alpha and later, and nothing
older**: API 3 and `minServer` 0.6.0-alpha are the start of the line. The server refuses a version
built for less, and says why. Limited backward compatibility may exist in an unofficial capacity,
where a module chooses to carry it, but nothing promises it, and no release is tested against it.

## The family

### Apps

Each app has its own repository.

| App | What it is |
|---|---|
| **WittyMediaServer** | The host: accounts and profiles, libraries and scanning, metadata, transcoding and streaming, remote access, Live TV. Every plugin below runs in it. |
| **WittyMediaPlayer** | A desktop player for local video and audio files, folder discs, disc images and real disc drives. No server or accounts. Discs play with full menus through the WittyDVD and WittyBluray plugins, and it makes disc images for a server's library. |
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
| **WittyCD** | Audio CD images: cue, toc, ccd, nrg and chd, played as one continuous stream, with its own FLAC encoder. Go library + CLI + diagnostics, no dependencies | WittyMediaServer |
| **WittyPlay** | Console games streamed to a browser or an app: a libretro game runner in its own sandboxed process, WebRTC with fallbacks, every controller, a touch layout editor | WittyMediaServer |
| **WittyCouch** | A Moonlight client in the page, for connecting to any Sunshine server: finding, pairing, its games, and playing them with every kind of controller. Apache-2.0: it starts WittyCouch Host for the stream. Not an official Moonlight project | WittyMediaServer, WittyMediaClient |
| **WittyCouch Host** | Moonlight's streaming core as a program of its own, speaking a documented protocol over pipes. **GPL-3.0**, as it's built on Moonlight's code. Installed separately | WittyCouch |
| **WittyRigs** | An advanced option, off by default: hosting demanding games on the server's own hardware, using a separately installed Sunshine. Not an official Sunshine project | WittyMediaServer |
| **WittyIntegratedFileBrowser** | Choosing a server's folders from any device: a Go listing of drives, folders and files, and the React window that shows it (in `web/`). Read-only | WittyMediaServer |
| **WittyModule** | What makes a plugin a project of its own: the manifest, version ranges, the helper protocol and the release tool. Go, no dependencies | Every plugin, WittyMediaServer |
| **WittyPhotos** | What a photo says about itself, without decoding it: EXIF, dimensions and the preview in a RAW file. Go, fuzz-tested, and can update without a new server | WittyMediaServer |
| **WittyVisualizer** | Light shows that follow the music: seventeen effects written as shaders, with audio analysis, a flash limiter and a calm mode. The same effect files run on WebGL 2 and on Qt's QRhi. Off until someone turns it on, and costs nothing until then | WittyMediaServer, WittyMediaPlayer |
| **WittyTuner** | TV tuners and IPTV behind one interface: network tuners, M3U playlists and streams, Linux DVB, V4L2 and other capture cards, SAT>IP, external backends, a custom command. Also reads XMLTV and the guide data in broadcasts. Refuses encrypted channels | WittyMediaServer |

### Server libraries

The server's libraries carry the name where it reads naturally:

| Library | Plugin |
|---|---|
| Films, shows and anime | **WittyVideo** |
| Discs | **WittyDVD**, **WittyBluray**, **WittyCD** |
| Music and audiobooks | **WittyAudio** |
| Podcasts | **WittyPodcasts** |
| Books, PDFs and comics | **WittyEbook**, **WittyPDF**, **WittyComics** |
| Photos | **WittyPhotos** |
| Vertical videos | **WittyShorts** |
| Creator videos, music videos and concerts | **WittyChannels** |
| Live TV and DVR | **WittyTV** |
| Games | **WittyPlay**; on any PC running Sunshine, **WittyCouch**; hosted on this server, **WittyRigs** |

The plumbing underneath (accounts, transcoding, metadata, settings, logging) keeps plain
names. So do features inside a plugin, such as the selection toolbar or the eraser.

## How the code is organised

The server holds no module's source. Each module is at `github.com/wittywilla/<Repository>`, listed in
[REPOSITORIES.md](REPOSITORIES.md). `go.mod` names one thing from them, **WittyModule**, which holds the
contract between the two sides and has no dependencies, and CI fails if anything else appears.

What the server keeps for a module is the shape of what it says:

| In the server | What it is |
|---|---|
| `internal/play/...`, `internal/couch/...` | Stubs written by `kit/facade` in WittyModule from `facade.json` in the module: the module's types, and functions that call its service. Never edited by hand, except `manual.go` |
| `internal/dvd`, `internal/bluray`, `internal/cd`, `internal/tuner` | The types those modules answer in, kept as a copy. A helper and the server compile them separately, and they match by field name |
| `internal/tuner/mpegts`, `internal/isofs` | The server's **own** encryption guard and disc image reader, so it refuses encrypted media itself, whatever a module says |
| `web/src/moduletypes` | The TypeScript types of the browser parts, from `scripts/sync-module-types.mjs`. Types only: nothing from them runs |

Each plugin and app repository carries what it needs:

- **README.md:** what it does.
- **wiki/:** the full guide, published to the repository's GitHub wiki.
- **LICENSE:** Apache-2.0, the same as WittyMediaServer, except for WittyCouch Host, which is GPL-3.0 because it's built on Moonlight's code.
- **NOTICE:** credits *"<name>, developed by WittyWilla"*. The licence requires anyone who
  redistributes the code or builds on it to keep that attribution.
- **THIRD-PARTY-NOTICES.txt**, written by `wittymodule pack` into each archive: the license of every
  library in its programs. Each module carries its own, as it's passed on by itself, and the server
  shows it at `GET /api/modules/{id}/notices`.

### Versions

Versions move in small steps as work lands. A bump isn't a release: nothing is tagged or published by one.

```bash
go run github.com/wittywilla/wittymodule/cmd/wittymodule@v0.3.0 bump -note "Reads a new format." -changelog CHANGELOG.md .
scripts/bump-server.sh 0.4.1-alpha "What changed."
```

The first raises a plugin's version in its `module.json` and `package.json` and in its changelog, and refuses anything not newer. The second does the same for the server and says which plugins' ranges no longer cover it. In a plugin's own repository, CI runs `wittymodule needs-bump`, which fails when it has changed since its last tag and still has that tag's version, since tags never move.

Versions stay below v1 while the APIs settle, so a minor version may change them.

### Rules every plugin follows

1. **No reaching into the host.** A plugin imports only its own files, its declared libraries
   and the front door (`index`) of plugins it builds on. Everything else (files, saving progress
   and notes, accounts, settings) comes through a small **host interface** that the server and
   each app implement. `web/scripts/check-modules.mjs` runs before every web build and fails it
   if a plugin imports anything else.
2. **Isn't in the server.** A plugin reaches a server only as a signed release (see
   [Independent updates](#independent-updates)), and nothing unsigned is ever fetched and run. A page
   whose plugin is missing says so and offers to add it. For a machine with no network, a release of
   the server can carry a [starter set](#a-starter-set-for-offline-use).
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

### Pages

A module's manifest can list `pages`, each with an `id`, `title`, `icon`, `where` (`main` or `settings`) and `admin`. They appear
in the app's menus while the module is in use, at `/m/<module>/<page>`, and its bundle exports `pages`: components by id. Each gets
one prop, `host`:

| | |
|---|---|
| `host.request(method, path, body?)` | Calls this server's API with the viewer's own access, and returns parsed JSON. Only this server: any other address is refused |
| `host.user` | `{ admin }` |
| `host.store` | `get(key)` and `set(key, value)`, as promises: a little JSON for this person and this module. At most 64 KB a key and 32 keys |
| `host.token()`, `host.base()` | For a page that opens a stream itself: the sign-in, and the server's address, or `''` on the same origin |

`admin: true` hides a page from anyone else. It keeps nothing safe by itself, so what the page calls has to check for itself.

### What a plugin can reach

A plugin has no way into the server's memory or its database. A signed-in person's request reaches it
only through a route the server chose to give it, after the server's own checks. Access control stays in
the server, so a plugin can never show an item that an account is not allowed to see.

## Independent updates

A plugin can be updated without a new server, and each plugin says which servers it works with.

### Three kinds of plugin process

The server has no module code compiled in, so what a module does happens in one of these:

| Kind | Used by | How it runs | Between the two |
|---|---|---|---|
| **Helper** | WittyPhotos, WittyIntegratedFileBrowser, WittyTuner, WittyDVD, WittyBluray, WittyCD, WittyRigs | Started when needed, one per open disc or stream, and stopped after a minute idle | Pipes, one call at a time: [the helper protocol](#the-helper-protocol-module-api-3) |
| **Service** | WittyPlay, WittyCouch | One long-running process, while the server has a use for it | HTTP on `127.0.0.1` with a random port and a token made for that start. `wittymodule/service` is both ends, and `kit/facade` writes the stubs |
| **Web bundle** | WittyInk, WittyEbook, WittyPDF, WittyComics, WittyVisualizer | Loaded by the browser when a page first needs it | Through `ReaderHost` and `globalThis.__wmsModuleHost` |

A helper or service can also have a part in the browser: a web bundle in its archive, named by `webEntry`
and `webStyle`. WittyPlay's player, WittyCouch's player and the file browser's window work this way.

These are the options that were weighed for running a plugin's code:

| Option | Works on every platform we ship? | Speed | Isolation | Verdict |
|---|---|---|---|---|
| Go's `plugin` package | No: no Windows, and needs the exact same toolchain build | Native | None, a crash takes the server down | Rejected |
| **A process of its own** | Yes: one build per platform, same as the server's 10 targets | Native; one more small process while in use | A crash or hang is only a failed call | **Chosen** |
| WebAssembly (wazero) | Yes, but ARMv7 and 386 only get the slow interpreter | Compiling costs more than the job on a small box | Best: no files or network unless granted | Not now |
| **Runtime-loaded web bundle** | Yes, it runs in the browser | Same as the app's own scripts | Same as the app's own scripts, so signing is what protects it | **Chosen for web** |

### What stays in the server

| Plugin | What it does here | What stays with the server |
|---|---|---|
| **WittyPhotos** | Reading EXIF, picture sizes, the preview in RAW files | The photo pages. Without it a photo library still scans, with sizes from common formats |
| **WittyIntegratedFileBrowser** | Finding drives and folders, listing them, and the window | Without it, a folder is typed as a path |
| **WittyDVD**, **WittyBluray** | Everything about a disc: structure, menus, commands, subtitles, Java programs, the disc check | Drawing a menu from what they say, and playing the video through FFmpeg. Without them a disc stays listed, waiting, and is filled in when one arrives |
| **WittyCD** | Reading images and their tracks, and the disc check | The MusicBrainz lookup, the player page |
| **WittyTuner** | Every kind of tuner and stream, and checking playlists for encryption | The guide, recordings, and the guard every stream passes through |
| **WittyPlay** | Consoles and cores, the game runner, streaming, controllers, the player | The games library, its metadata and where saves are kept |
| **WittyCouch**, **WittyCouch Host** | Pairing, the relay, the player, and the program that speaks to the PC | The list of rigs |
| **WittyRigs** | Finding and setting up a Sunshine on this machine | Nothing more |
| **WittyEbook**, **WittyPDF**, **WittyComics**, **WittyInk** | The readers themselves | The Reader page and its host, and pdf.js, so it isn't carried twice |
| **WittyVisualizer** | Effects, listening, controls | Where it shows in the audio player |
| **WittyDisc**, **WittyModule** | Libraries, with nothing of their own to install | They reach a server inside what's built with them |

Films, music, podcasts, vertical videos and creator videos (WittyVideo, WittyAudio, WittyPodcasts, WittyShorts,
WittyChannels) are part of the server.

**What it costs.** Nothing until a module is used. A helper is gone after a minute idle. Opening a disc adds a
process of 23 to 109 MB, and a service holds about 20 MB while the server has a use for it. A service that dies is
started again on the next call, with what it needs told to it again.

**Keeping the two sides in step.** Types are matched by name, in a self-describing format, so a field one side
hasn't heard of is ignored. `wittymodule check` keeps a manifest honest, and a change the other side has to know of
raises the API version.

**What a module may ask for.** The manifest's `network`, `programs`, `devices` and `data` say what the sandbox leaves
open, and the server has its own list of which stock modules may have each, in `internal/modules/service.go`. A
manifest can't grant itself anything, and a fork has none of them.

### Modules at run time

| To… | What happens | Rebuild? | Restart? |
|---|---|---|---|
| **Add** | `POST /api/modules/{id}/add` installs the newest version that fits, and what it's built on | No | No |
| **Update** | Installs beside the old version, verifies it, and switches when nothing's using the old one | No | No |
| **Turn off** | `POST /api/modules/{id}/disable` stops it. Its pages say it's missing, and its data stays. Refused, with names, while libraries need it | No | No |
| **Remove** | `POST /api/modules/{id}/remove` takes its files and leaves its data. Refused, with names, while libraries need it | No | No |

A signed-in person who calls a route of a missing module gets `424 Failed Dependency` with `module` and `moduleName`.
The web app's `ModuleMissing` shows that with a button to add it, for an admin. Someone not signed in sees only what
they always saw. A library waiting for a module is scanned when it arrives. A scan with no WittyPlay leaves a game
library as it is, since not knowing what a game is isn't the same as the games being gone.

### A starter set for offline use

A release of the server can carry signed modules, for a machine that never reaches the network. This is a step in
packaging, and nothing in the source changes: it only copies a folder that a catalogue was built into.

```bash
WMS_STARTER=<a folder with catalog.json, catalog.json.sig and the archives> scripts/build-release.sh 0.6.0-alpha linux/amd64
```

They land in `modules/` beside the program, and go through the same checks as a download: the catalogue's signature,
its serial, and each archive's size, hash and signature. Nothing's installed until someone adds it.

### The manifest

Every plugin has a `module.json`, shipped in its releases and covered by their signature:

```json
{
  "id": "wittyphotos",
  "name": "WittyPhotos",
  "version": "0.3.0",
  "repo": "wittywilla/WittyPhotos",
  "delivery": "helper",
  "entry": "wittyphotos-module",
  "api": 3,
  "minServer": "0.6.0-alpha",
  "maxServer": "0.6.x",
  "group": "photos",
  "summary": "Photo libraries: what a photo says about itself.",
  "libraries": ["photos"],
  "updates": "Reading EXIF, picture sizes, and the preview inside RAW files.",
  "stays": "The photo pages in the app."
}
```

`delivery` is one of:

| Delivery | What a release holds | How it runs |
|---|---|---|
| `helper` | A program per platform | A separate process that speaks the [helper protocol](#the-helper-protocol-module-api-3) |
| `program` | A program per platform | The server starts it for one job, in that job's own protocol. It must also answer the hello, so an install can check it runs |
| `web` | One bundle for every platform | The browser loads it under the page's content security policy |
| `library` | Nothing to install | Linked into others, and reaches a server inside them |
| `builtin` | Nothing to install | Part of the server, and updates with it |

`api` is the **module API version** it was built for, and `minServer` the oldest server it asks for. The
server has a range it speaks (`wittymodule.API` down to `wittymodule.MinAPI`), and both are 3 today. Adding to the
API raises only the top. Breaking it raises the bottom too, which disables older plugins, so that is done rarely.
`wittymodule check` refuses a manifest with an API below 3 or a `minServer` below `0.6.0-alpha`, and a server refuses
such a release with a message that says so. It is one number for the whole server, as the helper protocol and host
interfaces are one contract. API 3 adds services, notifications, what a module may ask for, and a part in the browser
for a helper. Since 0.6.0-alpha is where this starts, an older server never had modules that work like this at all.

The rest is there for the people choosing: `group`, `summary` and `needs` describe it, `libraries` says
which kinds of library need it, `requires` and `core` say what it is built on, and `updates` and `stays`
say honestly which parts of it a release really changes and which only a new server can.

`dataFormat` and `dataFrom` number the shape of the data a version keeps and the oldest it can read.
See [Data survives a version change](#data-survives-a-version-change).

### The catalogue

A server learns which modules exist from one signed file, with nothing compiled in. It lists every module and every
version, with what a store shows and where each version's files are. This project's catalogue is in
[WittyModuleCatalog](https://github.com/wittywilla/WittyModuleCatalog), at

```
https://github.com/wittywilla/WittyModuleCatalog/releases/download/catalog/catalog.json
https://github.com/wittywilla/WittyModuleCatalog/releases/download/catalog/catalog.json.sig
```

`Catalog` in WittyModule (`catalog.go`) is the code that reads and checks one, and `wittymodule catalog-add`,
`catalog-check` and `catalog-sign` write them. The `.sig` is the same as a release's: base64 of Ed25519 over the file's exact bytes.

```json
{
  "format": 1,
  "name": "WittyMediaServer modules",
  "serial": 12,
  "generated": "2026-11-01T12:00:00Z",
  "modules": [
    {
      "id": "wittyphotos",
      "name": "WittyPhotos",
      "summary": "Photo libraries: what a photo says about itself.",
      "description": "Plain text, as long as a store page needs.",
      "icon": { "url": "icons/wittyphotos.png", "size": 4210, "sha256": "…" },
      "publisher": { "name": "WittyWilla", "url": "https://github.com/wittywilla" },
      "repo": "wittywilla/WittyPhotos",
      "group": "photos",
      "libraries": ["photos"],
      "requires": [],
      "core": false,
      "needs": [],
      "updates": "Reading EXIF, picture sizes, and the preview inside RAW files.",
      "stays": "The photo pages in the app.",
      "versions": [
        {
          "version": "0.3.0",
          "delivery": "helper",
          "api": 3,
          "minServer": "0.6.0-alpha",
          "maxServer": "0.6.x",
          "license": "Apache-2.0",
          "permissions": [],
          "released": "2026-11-01",
          "prerelease": false,
          "changelog": "Plain text: this version's section of the module's changelog.",
          "sums": "https://github.com/wittywilla/WittyPhotos/releases/download/v0.3.0/wittyphotos-0.3.0-SHA256SUMS.txt",
          "sig": "https://github.com/wittywilla/WittyPhotos/releases/download/v0.3.0/wittyphotos-0.3.0-SHA256SUMS.txt.sig",
          "manifest": "https://github.com/wittywilla/WittyPhotos/releases/download/v0.3.0/wittyphotos-0.3.0-module.json",
          "platforms": {
            "linux-amd64": { "url": "https://github.com/…/wittyphotos-0.3.0-linux-amd64.tar.gz", "size": 1446318, "sha256": "…" }
          },
          "withdrawn": { "severity": "security", "reason": "Only there when a version has been taken back." }
        }
      ]
    }
  ]
}
```

| Field | What it means |
|---|---|
| `format` | This file's format. A server refuses a higher one in words ("a newer server can read it") and keeps its last good catalogue. Adding a field doesn't raise it, and readers ignore fields they don't know |
| `serial` | Only ever goes up, and a server keeps the highest it has seen, so an older catalogue can't be replayed to bring a withdrawn version back |
| `summary`, `description`, `changelog`, `reason` | **Plain text**, with no control characters and line breaks only in the long ones. Always show it as text, never as HTML or Markdown |
| `icon` | Optional. A PNG of at most 256 KiB, checked against its size and hash. A server fetches and keeps it, so a page never asks anyone else for it. No icon: the store draws one from the name |
| `screenshots` | Optional. Up to 8 PNG or JPEG pictures of at most 2 MiB each, checked the same way, fetched when someone opens the module's page |
| `publisher` | Who it comes from. `key`, also optional, is the base64 public key its releases are signed with, when that isn't the catalogue's own |
| `group`, `libraries`, `requires`, `core`, `needs`, `updates`, `stays` | As in the [manifest](#the-manifest), from the newest version |
| `versions` | Newest first, up to 100, and may be empty for a module listed before its first release. A version never changes once listed, except for `withdrawn` |
| `delivery`, `api`, `minServer`, `maxServer`, `license`, `permissions` | As in the manifest. An install is refused unless all of these match the release's signed manifest (`MatchesManifest`). A permission this server doesn't know means the version can't run here, and it's shown with that reason |
| `prerelease` | Offered under the rules in [Version ranges](#version-ranges-including-the-edges). A version with a label such as `-beta` counts anyway |
| `changelog` | May be empty when there's nothing to report: the store then shows a plain line of its own, such as "Fixes and improvements" |
| `platforms` | Keyed `GOOS-GOARCH` (`linux-arm` is the armv7 build), or `all` for a web bundle. A platform not listed means no build for it |
| `sums`, `sig`, `manifest`, each `url` | `https`, or a path below the catalogue's own folder, which is how an offline starter set and a test catalogue work. Nothing else: no `http`, `file`, `..` or query |
| `withdrawn` | `broken` warns, and `security` refuses it, as in [Withdrawing a version](#withdrawing-a-version). A module's own signed list still counts as well |

Limits: 8 MiB for the file, read with that limit in force; 1000 modules; 100 versions each; 256 MiB for an archive.

**What the signature means, and doesn't.** This project's catalogue is signed with the module key: it can't sign a server,
and a leak of it already allows a signed module. The catalogue only offers. An install still checks the release's own signed
list and manifest and every file in the archive, and also that the catalogue's description of the version matches. So a
catalogue can't describe a version as asking for less than it does, and holding only a catalogue's key runs nothing.

**The rules for a server.**

1. Check the signature before reading anything in the file (`VerifyCatalog`), then compare `serial`. Failing either keeps the last good catalogue, and the store says why in a line.
2. Keep the file and its signature in `<data>/modules/`, and check them again when reading them back, so the store works offline.
3. Offer a version only if `Supports` passes for this server. Show the reason on one that doesn't, rather than hiding it.
4. Ask for one file at a time, with its limit in force, and hash as it arrives.
5. Never let a page fetch from an address in a catalogue. The server fetches and serves what the page shows.

**Stores other than this project's**: a store is a `catalog.json` at an `https` address, with its key pinned when an admin adds it, after a warning. See [Other stores](#other-stores). Its signature and serial are kept for it alone.
A module from one is held to the fork rules: an admin chooses each version, nothing updates by itself, and it gets a fork's limits.
Its releases must be signed with that store's key or the `publisher.key` it gives. An `id` that this project's catalogue also lists is a
fork of that module, and needs the choice a fork needs: a store can't replace a module by listing it, and WittyTuner can't be replaced at all.
A store's addresses are fetched like any other address someone typed: no private or local addresses, and no following redirects to them.

**A test or offline catalogue** is a folder holding `catalog.json`, `catalog.json.sig` and the files, with relative paths:

```bash
wittymodule pack -out release/wittyphotos .
wittymodule sign -key test.key release/wittyphotos/wittyphotos-0.3.0-SHA256SUMS.txt
wittymodule catalog-add -catalog catalog.json -base wittyphotos -pub <test public key> release/wittyphotos
wittymodule catalog-sign -key test.key catalog.json
```

### Version ranges, including the edges

Ordering is [semantic versioning](https://semver.org), so a pre-release comes before its release.

| Server | `minServer` / `maxServer` | Result | Why |
|---|---|---|---|
| 0.4.0 | `0.4.0` / `0.5.x` | Works | In range |
| 0.4.0-alpha | `0.4.0` / `0.5.x` | **Refused** | The alpha comes before 0.4.0. A plugin that works on the alpha says `minServer: 0.4.0-alpha` |
| 0.4.0-beta | `0.4.0-alpha` / `0.5.x` | Works | Beta is after alpha |
| 0.5.7 | `0.4.0` / `0.5.x` | Works | `x` covers every release and pre-release in that series |
| 0.6.0-alpha | `0.4.0` / `0.5.x` | **Refused** | A pre-release of a later series is past the maximum |
| 0.5.0-rc1 | `0.4.0` / `0.5.0` | Works | Comes before 0.5.0 |
| 0.5.1 | `0.4.0` / `0.5.0` | **Refused** | An exact maximum means exactly that |

`maxServer` is required, and `x` alone is not allowed: every plugin states a real maximum.

Pre-release *plugins* are offered only to a server that is itself a pre-release, or that already runs
that plugin's pre-release. A stable server never gets one by surprise, but keeps one it already has, and
an admin may still choose one from the list of versions, after a warning.

A server whose version can't be read (a build from a commit with no tag) can't be compared, so it installs nothing and
says so.

**When a server upgrade leaves a plugin out of range,** nothing breaks. On start, the server takes the
first of these that passes, and the settings page says in plain words what happened:

1. The chosen installed version.
2. The previous installed version.
3. Going without it: its pages say so, and offer to add a version that fits.

The refused version stays on disk, in case the server is rolled back, and the next check looks for a
release whose range fits. A plugin never holds a server back.

### Updating

Nothing leaves the server unless settings allow it, and the server works the same offline.

- `moduleUpdateCheck` looks for plugin updates twice a day. If never set, it follows `updateCheck`, so
  an owner who turned off update checks gets no new traffic. Pressing **Check now** always works, and it
  is the only other thing that goes online. Opening a list of versions doesn't.
- `moduleAutoInstall` installs compatible updates without asking. Off by default.
- Each module that updates on its own also has its own switch, **Update this module by itself**. Off, it still looks for updates and shows them, and waits for an admin to install them. Unlike a pin, this says nothing about which version stays, so choosing a newer one by hand leaves it free. Parts of the server have no such switch, since they update with the server.
- A module taken off this server asks nobody anything.

**The same trust as server updates:** Ed25519 over a checksum list, GitHub over HTTPS as the only source.
Only plugins the server already knows are accepted. GitHub is trusted for nothing except delivering files:
it can hold a release back, but it can't forge one, and release notes are shown as plain text, marked as
coming from GitHub.

A release of version `V` has one signed list covering everything:

```
<id>-V-SHA256SUMS.txt       hashes of each archive, the manifest, and every file in each archive
<id>-V-SHA256SUMS.txt.sig   its signature
<id>-V-module.json          the manifest, so a release can be judged before downloading it
<id>-V-<os>-<arch>.tar.gz   a program's files (.zip on Windows), or
<id>-V-web.tar.gz           a bundle's files, the same for every platform
```

**Check.** Reads the plugin's releases. An unchanged answer costs GitHub nothing (a conditional request
isn't counted against the limit on requests). A release not seen before has its list, signature and
manifest fetched and verified, up to twelve per check. What was learned is kept in
`<data>/modules/<id>/feed.json` and verified again when it is read, so the list of versions works offline
and a changed file there is ignored. If GitHub asks this network to wait, every plugin waits until then, and
the page says until when.

**Install**, each step of which leaves the running plugin untouched if it fails:

1. Verify the list's signature, and that the manifest is for this plugin, this version and this repository.
2. Download the archive, hashing as it arrives, and stop at the size in the release.
3. Unpack to a work folder, refusing links and paths that climb out.
4. Check every file against the signed list, refusing any extra or missing file.
5. Start the helper once, where there is one. Its hello must name the same plugin, version and API.
6. Rename the folder into `versions/V`, then save the state atomically.
7. Switch over. The next call uses the new version, with no server restart.

**Rollback.** Automatic if three calls in a row fail at the process level (crash, hang, garbage), or a
browser reports a bundle failing three times in a day: that version is blocked with the reason and the
previous version takes over, or the module goes without until one fits. A bad file that a parser refuses is not a failure. An admin
can also roll back by hand. A blocked version isn't installed automatically again, but an admin can still
choose to. Only an admin's browser counts towards this: anyone else's report goes in the module's log for an admin to
see, and their page has already fallen back to the app's copy.

**On disk.** Everything is in `<data>/modules/`, never in the program folder, so this also works in
containers and in protected installs. The active, previous and pinned versions are kept. The signed list
stays beside the files and is checked again every time a helper starts, so a file changed on a shared
data folder is refused rather than run. If the data folder doesn't allow programs to run (`noexec`,
common on network shares), the install fails at step 5 with a plain message and what was in use
carries on.

### Choosing a version

**Settings, Modules** lists every version a module has released, newest first, with its notes. A
version that doesn't fit this server is shown with the reason, and can't be chosen.

Choosing an older one warns first: it may lack fixes, including security fixes. Then it is installed and
**pinned**, or the next automatic update would undo the choice. A pin holds through checks and restarts,
and can also be set on whatever is in use. **Go back to the latest** ends the pin and moves to the newest
version that fits, whichever release that is. A version already on disk is switched
to with no download, so going back works offline.

Safety comes before a pin: a version withdrawn for security reasons, or one that keeps failing, goes out
of use anyway, and the page says so.

### Withdrawing a version

A release can't be taken back from everyone who has seen it, and GitHub isn't trusted to say which ones are
bad. So each plugin can publish a signed list, `<id>-withdrawn.json` and its `.sig`, on a release with the
tag `withdrawals`. The **Withdraw a version** workflow in a plugin's repository does this, from the Actions tab.

| Severity | Choosing it | If it is in use | Automatic updates |
|---|---|---|---|
| `broken` | A warning | Stays, with a warning | Never picked |
| `security` | **Refused** | Goes out of use straight away, even if pinned | Never picked |

The list has the plugin's name inside what is signed, so one plugin's list can't be used for another. It has a
serial number that only goes up, and a server keeps the highest it has seen, so an old list can't bring a
withdrawn version back, and hiding the list doesn't either. A newer signed list can restore a version.

### Data survives a version change

Nothing here changes a module's data. A version says, in its signed manifest, the newest format of data it
writes (`dataFormat`) and the oldest it can read (`dataFrom`), and the server remembers the newest format used
so far. A version that can't read what's there is **refused**, with the reason, both when choosing it and at
every start: going back past a change of format can't lose data, because it doesn't happen. Today no module keeps
private data, as notes, progress and saves belong to the server and survive anyway, so none declares a format
yet. The first that does must set these.

### Adding and removing modules

No module's code is in the server, so what a server has decides what it can do, and that is enforced by the server, not
only by the app: creating a library of a kind whose module isn't there is answered with what it needs.

- **First-run setup** asks what you want, before you make libraries.
- **Add library** shows the kinds that need a module in a greyed section, each with a button to add it there and
  then, and a link to the store.
- **Settings, Modules** is the store.

**When two modules bring the same kind of library,** they share it: a discs library is one kind, and WittyDVD and
WittyBluray each play their half of what's in it. The store says so under **Brings these libraries**, naming the others.
A kind can also be given a name of this server's own there, such as "Blu-rays and DVDs", which is what Add library then
shows. A name another kind already has is refused, so two can't read alike. What a kind is called doesn't change what it
is, and clearing the name goes back to the usual one. Only the kinds the signed catalogue lists for a module count: a
release or a fork can't claim a kind it didn't come with. A test lists which modules share a kind on purpose, so a new
module that happens to pick one already in use is caught when it is added.

A kind of library is available if any one module providing it is there, so a discs library needs WittyDVD or
WittyBluray. A module can't be removed while libraries need it, and the reason names them. Removing touches no
data. Core modules, which others are built on, are added with what needs them and stay while it does.

### Using someone's fork

Someone can publish their own version of a stock module, and an admin can point that module at it. Every other
part of the system then treats it as that module: the versions list, pinning, rollback, and the same place in
the app. In the module's details, under Advanced, enter its repository and the public key its publisher gives
you.

**This is someone else's code running on your server, so only do it for a publisher you trust.**

- Nobody has reviewed it. The signature proves it came from whoever holds that key, not that it's safe.
- A helper or program can read what the server's account can read, which may include your media and the
  server's database.
- A reader bundle runs in the browser of whoever is reading, with their access, including an admin's. That is
  the highest risk here, and the app can't limit it.
- A key that leaks, or a publisher who turns, is as bad as the code being malicious from the start.

What the server does about it:

| | A stock module | A fork |
|---|---|---|
| Signed | With this project's key | With the key an admin gave, for this one module only |
| Automatic updates | If the owner turned them on | **Never**: an admin chooses each version |
| Network, Linux | Only WittyTuner | **None: no sockets at all, and it won't run if the system can't apply that** |
| Changing what's in files, Linux | Refused | **Refused, and it won't run on a kernel too old to do so** (5.19 or newer) |
| Windows | Can't change the server's files or start programs | The same, and **it keeps its network**, which the warning says |
| macOS, FreeBSD | No limits | **No limits**, which the warning says |
| Encrypted media | Refused | **Refused**: the server checks for itself, and only accepts file locations inside the disc |

A forked game runner goes through the same limits, and may also write to its own save folder and open devices such as a
graphics card. On Linux a fork's helper has no 32-bit build it can run on, since the filter those limits rely on needs a
64-bit system.

The server shows the key's fingerprint and the warning before anything changes, and shows "from another
publisher" on that module for as long as it is in use. Check the fingerprint with the publisher, somewhere other
than where you got the key. **Go back to the official version** undoes it, and removes what the fork installed.

**WittyTuner can't be replaced by a fork.** It is what refuses encrypted channels, and it handles live streams
with network access, so nothing outside it could tell whether a fork had kept that line. Disc engines can be
replaced, because video never passes through them: FFmpeg reads the disc from locations the server has checked.
This isn't a way to add decryption. The refusal doesn't depend on the module. This project keeps no list of recommended
forks. An admin can add a store run by someone else, which is the same choice with its own warning: see
[The store](#the-store).

To publish a fork: change `repo` in `module.json` to yours, keep the `id`, run `wittymodule keygen -key <file>`,
and add that key as your repository's `WMS_SIGNING_KEY` secret. The release workflow that comes with the module
does the rest. Give people the public key it printed.

### Custom modules: in development

Wholly new kinds of module, rather than replacements for ones the server already knows, aren't possible yet. The
server only accepts modules it knows about, because it has nothing to connect an unknown one to. The store has a
section saying so. What it would take: a host interface a helper can call back through, a way for one to add a
library type and its pages, and a permission model strong enough for code nobody here has seen. WebAssembly, above,
is the likely way to contain them.

### The helper protocol (module API 3)

A helper is started with `--wms-helper` and talks over stdin and stdout: one line of JSON per message,
each at most 1 MiB, optionally followed by a binary blob of the size it announced (at most 128 MiB).
Sizes are checked before memory is set aside, and the server treats everything a helper says as
untrusted.

```
helper → {"module":"wittyphotos","version":"0.3.0","api":3}
server → {"id":1,"method":"read","params":{"path":"..."}}
helper → {"id":1,"result":{...},"blob":0}
helper → {"id":2,"error":"...","code":"no-preview"}
helper → {"notify":"task","params":{...}}
```

A helper exits when stdin closes, so it can't outlive the server. The server starts it on first use and
stops it after a minute idle, so it costs nothing when unused. A job that keeps state, such as an open
disc, has a helper of its own for as long as it lasts. A stream is a helper that gives one answer and then
writes the stream. A helper may tell the server things as it goes with a notification, which is how a task's
progress and a disc check's findings arrive. `github.com/wittywilla/wittymodule` has both ends of the protocol, plus
the manifest, version rules and release tool, and has no dependencies.

A **service** starts the same way, says hello, and then prints the address it listens on, on `127.0.0.1` only. Every
request carries `X-Witty-Service-Token`, a token made for that start and given to it in its environment. It is held to
the same limits below, except where its manifest asks for what the server's list allows.

**Before anything runs:** the checksum list's signature against the module key, the archive's size and hash, each
file in it against the list, and every path in it, so nothing lands outside its folder. Links, devices, absolute paths
and `..` are refused. What's on disk is checked again each time a program is started.

**Limits on a helper or a service:**

| | Everywhere | Linux | Windows | macOS, FreeBSD |
|---|---|---|---|---|
| Small environment, soft memory limit, one call at a time with a time limit | Yes | | | |
| Hard memory limit | | Yes | Yes | No |
| Can't write to or create files | | Yes (Landlock). Emptying a file too, from kernel 5.19 | Yes (low integrity) | No |
| Can't change who owns a file or may use it, or reach into another process | | Yes on 64-bit (a call filter) | Yes | No |
| No network | | Yes on 64-bit (no sockets, which also keeps it from socket files), and an empty network where namespaces are allowed | No | No |
| Can't signal the server | | Yes where namespaces are allowed, or from kernel 6.12 | Yes | No |
| Can't start programs | | No | Yes (job) | No |
| Dies with the server | Yes, when stdin closes | Also on the server's death | Also by the job | |

**What the server lets a module have, and only these:**

| | What it opens | Who has it |
|---|---|---|
| `network` | Sockets, as a tuner or a game streamed to a browser needs them | WittyTuner, WittyPlay, WittyCouch, WittyRigs |
| `programs` | Starting FFmpeg and its own programs | WittyTuner, WittyPlay, WittyCouch, WittyRigs, and WittyBluray, for a Java someone chose to install |
| `devices` | Capture cards, tuners and graphics on this machine | WittyTuner, WittyPlay |
| `data` | Writing in a folder of its own, given in `WITTYMODULE_DATA`, and those in `WITTYMODULE_WRITE` | WittyBluray, WittyPlay, WittyCouch, WittyRigs |

A module that has `programs`, `devices` or `data` can't have namespaces or Windows' low integrity, since what it starts or
writes would be refused. Those run with the limits that are left: memory, the job, and the call filter. A fork has none of these.

The Linux limits are applied by the server starting itself as a launcher, which applies them and then becomes
the helper, so nothing in a helper can skip them. Containers often refuse unprivileged namespaces; a stock
helper then runs without them, and a fork doesn't run. Reading is left alone on every system, since a helper reads
media wherever it is: that is the one limit no system here gives, and why the warning above says so.

On Windows a helper goes into its job just after it starts, so for a moment it is outside: a fork could in principle
start a program in that moment, which would still have the low integrity. Starting it suspended closes that gap, and made
helpers hang at random for half a minute in testing, so it isn't done.

**What these don't protect against.** Someone who can write to the data folder can change the accounts in the
database, so that folder has to be protected whatever is done here. `verifyInstalled` catches a changed file, and a fork
named in the state file that couldn't have been chosen is ignored, but such a person can still name a fork of a module
that can be forked. A fork's menus and subtitles are pictures the server can't read, and it checks only samples of a disc
for encryption, so a disc scrambled only in places a sample misses could in principle show a fork's pictures of it.
FFmpeg still can't play such a disc's video.
Encrypted media stays refused: no plugin, helper or not, gets a way around that. The server reads disc structure and
watches streams with its own code for this reason. What FFmpeg opens for WittyTuner must name a limited list of
protocols, never with `crypto` or files on it, and looking inside a playlist for encryption is WittyTuner's part, which
is why a fork can't replace WittyTuner.

### Releasing a plugin

`wittymodule` does this from the plugin's own folder, with no help from this repository:

```bash
go run github.com/wittywilla/wittymodule/cmd/wittymodule check .
go run github.com/wittywilla/wittymodule/cmd/wittymodule pack -out release .            # programs
go run github.com/wittywilla/wittymodule/cmd/wittymodule pack -out release -web dist . # a bundle
go run github.com/wittywilla/wittymodule/cmd/wittymodule sign release/wittyphotos-0.2.0-SHA256SUMS.txt
```

A plugin's `.github/workflows/release.yml` runs these when a `v*` tag lands in its own repository, then
publishes the GitHub release. The building job has no signing key and no write token. The signing job runs on a
clean machine, in a `release` environment, and sees only the checksum list and the key. `kit/module-kit.sh` in WittyModule
generates these workflows. A module with a part in the browser builds that first, and packs them together.

A release then has to be listed. The refresh in [WittyModuleCatalog](https://github.com/wittywilla/WittyModuleCatalog) adds it
and signs the catalogue, and servers find it from there.

### Signing

**The recommendation is one module key, separate from the server's release key, not one per module.**

- A release key can sign a server, which runs with no limits at all. A key that signs modules has to sit in
  the secrets of ten repositories, where it is far more exposed, so it shouldn't be able to. `moduleKey` in
  `internal/update/keys.go` is trusted for modules and never for a server.
- One key per module would mean ten keys to look after, and a new server for every rotation. What it would buy
  is real but limited: a leak from one repository couldn't sign another module's helper. A bundle signed with a leaked
  key can already act as an admin in that admin's browser, though, which reaches everything the same way.
- Because one key is shared, a compromised build in any repository could ask for any module to be signed. So the
  signing job refuses a checksum list naming anything but its own module and version, and **required approval on the
  `release` environment, with a protection rule on `v*` tags, is strongly recommended**. It's the owner's click, in
  [MODULES-CUTOVER.md](MODULES-CUTOVER.md).
- Forks already have keys of their own, good for one module only.

Until the owner makes one with `scripts/new-module-key.sh`, modules are signed with the release key, which works
the same but gives up that separation. If a module key leaks: remove it from `moduleKey` and release a new server,
and any version it signed stops being trusted.

### Developing a server and a module together

A normal build of the server needs only WittyModule, at a released version. Nothing about a module needs to be beside it.

**A change that crosses the line** is two changes in two repositories. For WittyPlay and WittyCouch, the server's stubs come
from the module's source, so a new exported function means running the generator from a WittyModule checkout, which
rewrites `gen.go` on both sides. Anything it can't write goes in `manual.go`:

```bash
go run ./kit/facade -config <module>/facade.json -module <module> -server <this checkout>
```

**To try a real module on a server,** build a catalogue from your checkouts with a throwaway key. Only a server built with
the `updatetest` tag reads it, and no release can be:

```bash
scripts/make-test-catalog.sh /tmp/catalog <folder holding the repositories> WittyPlay WittyDVD
python3 -m http.server --bind 127.0.0.1 8099 --directory /tmp/catalog &
go build -tags updatetest -o /tmp/wms ./cmd/wms
WMS_UPDATE_KEY=$(cat /tmp/catalog/test-key.pub) WMS_MODULE_CATALOG=http://127.0.0.1:8099/catalog.json /tmp/wms
```

**Tests of what the server does with a real module** need that folder, and are skipped without it:

```bash
WMS_TEST_CATALOG=/tmp/catalog go test ./...
```

## Which modules run on which server

**Modules start with 0.6.0-alpha.** A module built for an older server can't run on this one or any later one, whatever range it claims, and those older alpha versions won't be fixed or updated. The store and the installed list both say so in those words, and offer the current version where there is one.

**Limited backward compatibility, unofficially.** A newer server may keep running a module built for 0.6.0-alpha or later, if the module's own range says it fits. That's a courtesy, not a promise: nothing about it is guaranteed until the beta, which is when proper versioning for modules begins. Until then a module can stop fitting at any new server, and the page says so when it does.

## The store

**Settings, Modules** has what's on this server: what's new, **Update All Modules**, and each module. **Settings, Modules, Store** (`/settings/modules/store`) has everything that can be added.

### Finding a module

Search goes across name, description, category and publisher, ignoring case and accents, and every word has to match. Modules can be browsed by category (`group` in the catalogue) and filtered by whether they're installed, whether they have an update, who publishes them and which platforms they're built for. The search and filters are in the page's address, so Back returns to the same results.

A module's own page has its icon, description, screenshots, what's new, what it asks for, licence, publisher, what it's built on, which servers it fits, a link to its source, every version, and the things that can be done to it: install, update, go back, turn off, remove, and its log.

Everything in a catalogue is shown as text. Pictures are fetched and checked by the server, then served from it, so a page never asks anyone else for anything. An icon is fetched when the page needs it, and kept.

### Installing

Installing brings what a module is built on, and says so first. If something it's built on fails, the module isn't attempted.

**Update All Modules** updates everything with an update, what others are built on first. One failure doesn't stop the rest. It shows each module's progress, and ends with a summary that stays, across a restart, until it's dismissed. **Take these updates back** returns each module to the version it was using, and that version isn't installed by itself again. Skipped, with the reason: a pinned module, a module from another publisher (you choose each of its versions), and a version that was taken out of use before.

Automatic updates are the owner's setting, **Install updates automatically**. Pinned modules and anything from another publisher are left alone.

### When something goes wrong

Every failure says what happened in plain words, what to try, and has **Show details**: the exact words behind it, this module's log lines, and a button that copies all of it. Each module also has **Show log**. The words are the server's (`Explain` in `internal/modules/failure.go`), so every client says the same thing.

| What happened | What it says to do |
|---|---|
| The signature doesn't check out | Try again; a damaged download usually goes away. If not, tell the publisher, and don't look for another copy |
| The package is damaged | Try again |
| The download didn't work | Check this server's internet, or try later |
| The disk is full | Free some space on the data folder's disk |
| Built for an older server | Install the current version, as above |
| Needs a newer server | Update this server, or choose an older version |
| The program wouldn't run | Try again, in case a virus scanner was holding it. Check that the data folder allows programs to run |
| The store's description doesn't match | Check for updates. If it stays, tell the publisher |
| A store's signing key has changed | See below |

### Other stores

An admin can add a store run by someone else: **Settings, Modules, Other stores**, with the address of its `catalog.json`.

**This is someone else's code running on your server, so only do it for someone you trust.** Before a store is added, the server fetches and checks it and shows the warning (`StoreWarning` in `internal/modules/stores.go`), what it offers, and its key's fingerprint. A box has to be ticked first.

| Rule | How |
|---|---|
| Who holds the key | Whoever runs the store. The signature only proves a file came from them |
| Where the key comes from | Pasted, which is better: then even the first answer doesn't have to be taken on trust. Otherwise the store's own suggestion, `catalog.json.key` beside its catalogue, for the admin to check |
| The key is pinned | From the moment it's added. A store signed by anything else is a loud refusal, however it asks. To accept a real change, remove the store and add it again, after checking the new fingerprint with its owner. There's deliberately no button for this |
| The catalogue is hostile input | Its signature is checked against the pinned key before anything in it is read. Its serial only goes up. The same serial with different contents is refused. Failing any check keeps the last good one, and the store says why |
| Where a store can be | `https` on the public internet, with no redirect that breaks that. An address on this machine or the admin's own network only with **Allow an address on my own network**, for that one store, and then plain `http` too. Link-local addresses are never allowed |
| Limits | 8 MiB for a catalogue, 4 KiB for a signature, 256 KiB for an icon, 2 MiB for a screenshot, enforced while reading |
| Words | Characters that reorder or hide text are taken out, and it's only ever shown as text. A module using the name of one of this project's modules is told apart |
| Labels | Every module from one says **Third-party** wherever it appears, with its store's name and address. They're listed separately |
| On, off and removing | A store that's off isn't fetched or listed, and what's installed from it stays. Removing it first puts anything using it back to the official version |

What a store can offer follows the [forks policy](#using-someones-fork):

| It lists | What the server does |
|---|---|
| A module this server knows | A fork, with a fork's warning, key and limits. An admin chooses it module by module, and nothing updates by itself. The release's own signed files must match what the store said |
| WittyTuner | **Refused**: it can't be replaced |
| A wholly new module | Shown as **In development**, and can't be installed: see [Custom modules](#custom-modules-in-development) |

To run a store, make a folder with `catalog.json` and the release files, as in [the catalogue](#the-catalogue), and run `wittymodule catalog-sign -key <file> catalog.json`, which writes `catalog.json.sig` and `catalog.json.key`. Give people the address and, somewhere else, the key.

## How the apps run

- **WittyReader** is a web app at heart: the reader plugins with a host that opens files from
  the disk (or dropped on the window) and keeps notes on the device. It ships as a desktop app
  and can also be installed from the browser. No server needed.
- **WittyMediaPlayer** is a Qt 6 window that decodes with FFmpeg's libraries, and runs the WittyDVD
  and WittyBluray engines in a small helper program beside it, reached over a pipe with the WittyModule
  helper protocol. Its disc modules update on their own, under the rules below. There's no web view and no loopback server, and only
  looking for module updates uses the network. See
  its design note in that repository.
- **Other native windows** use the system's own web view (WebView2 on Windows 10+, WebKit on macOS and
  Linux), which is far lighter than bundling a browser. The one exception is the Windows 7
  desktop client, which needs an older bundled browser engine and stays a thin client.



## Logging

WittyLog notices a module by itself and gives it a level and options in *Settings → WittyLog*. A helper's stderr is
kept, bounded, and logged if it fails. To offer more, list **topics** in the manifest:

```json
"capabilities": { "logTopics": ["scan|Scanning|Each file looked at", "cache|Cache|Hits and misses"] }
```

The helper is told what the owner chose through its environment when it starts: `WITTYMODULE_LOG` holds a level
(`debug`, `info`, `warn`, `error` or `off`) and `WITTYMODULE_LOG_TOPICS` a comma-separated list of topics that are on.
Print what is wanted to stderr, one thing per line. Never print tokens, passwords or the contents of files.
See [how every part logs](LOGGING.md#module-logging-levels-and-options-per-module).

## File types

A module that needs to know about file types shouldn't keep a list of its own. `GET /api/media-types` gives every format,
codec and container the server knows, with extensions, how each plays, which libraries take it and whether this machine can
handle it. `docs/media-types.json` is the same list as a file, for use when a module is built. See
[Supported formats](SUPPORTED-FORMATS.md).
