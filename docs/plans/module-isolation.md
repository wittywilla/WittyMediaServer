# Modules that stand on their own

For developers and the project's owner: how much of each module's feature lives in the server today, the shape that lets someone use a module's whole feature set in their own project without WittyMediaServer, and the order the code moves in. Tracked in [#98](https://github.com/wittywilla/WittyMediaServer/issues/98).

Surveyed 2026-11-04 against server 0.6.2-alpha (`6e2c592`) and every module repository at its published tag. Line counts are lines of Go, TypeScript and CSS, tests apart, and are meant for comparison, not to the line.

- [The short answer](#the-short-answer)
- [How a module talks to the server today](#how-a-module-talks-to-the-server-today)
- [The survey, module by module](#the-survey-module-by-module)
- [Code that is in two places](#code-that-is-in-two-places)
- [The server's import graph](#the-servers-import-graph)
- [The target shape](#the-target-shape)
- [Core libraries](#core-libraries)
- [Module by module, where the code goes](#module-by-module-where-the-code-goes)
- [Declaring and installing what a module needs](#declaring-and-installing-what-a-module-needs)
- [Encryption stays refused, everywhere](#encryption-stays-refused-everywhere)
- [Licences](#licences)
- [Order of work](#order-of-work)

---

## The short answer

**No module imports any of the server's code.** Every module builds from its own repository against WittyModule (and WittyDisc for the disc engines), and none reaches into `wittymediaserver/internal`. In that direction the split is already clean.

**The other direction is where the feature sets are split.** For several modules, a large part of what a person would call "the feature" is written in the server:

| Feature | In its module | In the server | Most of the feature is in |
|---|---:|---:|---|
| Games (WittyPlay) | 23,800 Go, 10,200 web | 26,200 Go, 4,000 web | Both: the module plays, the server identifies, catalogues and describes |
| Game PCs (WittyRigs) | 75 Go | 1,570 Go, 550 web | **The server** |
| Books (WittyEbook, PDF, Comics, Ink) | 3,100 web | 5,000 Go, 190 web | Both: the module reads, the server converts and takes books apart |
| Photos (WittyPhotos) | 900 Go | 1,700 Go, 680 web | The server's library, by design |
| Statistics (WittyStats) | 3,800 web | 1,700 Go | The module; the numbers are the server's, by design |
| TV tuners (WittyTuner) | 5,200 Go | 5,600 Go, 1,000 web | Both: the module tunes, the server guides, records and guards |
| DVD (WittyDVD) | 8,600 Go | 4,600 Go | The module, but turning its steps into something to watch is the server's |
| Blu-ray (WittyBluray) | 9,700 Go, 10,000 Java | 5,600 Go | As DVD |
| CD (WittyCD) | 7,800 Go | 2,200 Go, 1,400 web | The module |

So the modules look small for three reasons, and only the first needs fixing:

1. **Shared code that has no home of its own** sits in the server: identifying game files, reading game archives, the region order, reading MPEG transport streams and XMLTV guides, converting books, the encryption checks. A project that wants these today has to copy them out of the server.
2. **Copies kept on purpose.** The server holds its own copy of the types each module answers in, and its own encryption guard, so that it never takes a helper's word. Some of those copies are now byte for byte the module's own code.
3. **Host features.** Libraries, accounts, saves, recording schedules and who sees which numbers belong to whatever program hosts the module. Those stay in the server, and another project brings its own.

The plan: give the shared code a home in a few **core libraries**, each a Go module in its own repository that modules and the server both build with; move what is a module's feature into that module; generate the server's copies of types from the module rather than keeping them by hand; and keep the encryption refusal inside the libraries, so whoever uses them gets it too.

---

## How a module talks to the server today

- **Module API 3** (`wittymodule.API` and `MinAPI` both 3). The [module guide](../MODULES.md) is the contract; API 4 with release channels is planned for 0.7.0-alpha.
- **Helpers** (`delivery: helper`): a separate, sandboxed process, one JSON call at a time over pipes, optionally with a binary blob. WittyDVD, WittyBluray, WittyCD, WittyPhotos, WittyTuner, WittyIntegratedFileBrowser and WittyRigs. The server keeps a copy of the types each one answers in (`internal/dvd`, `internal/bluray`, `internal/cd`, `internal/tuner`), matched by field name in gob or JSON.
- **Services** (helpers that listen on this machine): WittyPlay and WittyCouch. `kit/facade` in WittyModule reads `facade.json` in the module and writes the server's stubs (`internal/play/...`, `internal/couch/...`): the module's types, and functions that call the service. Only `manual.go` is written by hand.
- **Programs** (`delivery: program`): WittyCouch Host, started for one job.
- **Web bundles** (`delivery: web`, or a helper's `webEntry`): loaded by the browser on first use. They get React and shared parts from `globalThis.__wmsModuleHost` (`web/src/moduleHost.ts`). The readers speak only through `ReaderHost`, declared in WittyInk, and WittyReader already backs it with local files. Pages get `host.request`, `host.store`, `host.user`. The web app imports module packages for their **types only** (`web/src/moduletypes`, `import type` from `@wittywilla/*`).
- **Libraries** (`delivery: library`): WittyModule and WittyDisc, linked into others.
- **`requires`** already exists in the manifest, the catalogue and the store: the store installs what a module requires first (`orderByNeeds` in `internal/modules/storerun.go`) and won't remove a module others require. What it lacks is a version range: see [Declaring and installing what a module needs](#declaring-and-installing-what-a-module-needs).

---

## The survey, module by module

"In the module" is what its repository does. "In the server" is code that exists only for that module's feature. Shared parts are counted once, under the first module that uses them.

### WittyDisc (library)

| In the module (1,250 Go) | In the server for it (1,060 Go, 1,400 web) |
|---|---|
| `isofs`: reading ISO 9660 and UDF images without mounting them (770). `session`: the contract between a disc engine and a player that decodes byte ranges itself, used by WittyMediaPlayer (520) | `internal/isofs` (770): **the same file, byte for byte**. `api_disc.go`: inserting and choosing discs (300). Web: the disc player shell `DvdPlayer.tsx`, `Remote.tsx`, `InsertDisc.tsx`, `DiscSections.tsx` (1,400), shared by DVD and Blu-ray |

### WittyDVD (helper)

| In the module (8,600 Go) | In the server for it (4,650 Go) |
|---|---|
| The whole engine: IFO structures, the navigation machine and its commands, menus and buttons, where video lies, sub-picture decoding, its own refusal of scrambled sectors. `cli` and `cmd/wittydvd`: a command-line player and inspector. `diagnose`: disc checks. `wittydvdplayer`: the engine for WittyMediaPlayer | `internal/dvd` (1,500): a copy of the module's types and wire format. `internal/dvdsession` (1,700): turning navigator steps into HLS streams or stills, PNG overlays for sub-pictures and highlights, button geometry, which subtitles to show. `modules/dvdremote` (1,060): running the helper, checking all it says, and the server's own refusal of encrypted discs (`scramble.go`). `api_dvd.go` (380) |

### WittyBluray (helper)

| In the module (9,700 Go, 10,000 Java) | In the server for it (5,630 Go) |
|---|---|
| The whole engine: index, movie objects, playlists, clips, the HDMV machine, interactive and pop-up menus, text and picture subtitles, its BD-J runtime (Java, run only with a Java the person installs) and the BD-J sandbox. `cli`, `diagnose`, `helper`, `wittyblurayplayer` as for DVD | `internal/bluray` (1,440): a copy of its types. `internal/bdsession` (2,700): steps into HLS, menu overlays, subtitles, running BD-J within limits, and the server's own AACS check (`encrypted.go`). `modules/bdremote` (1,040). `api_bluray.go`, `library/bluray.go` (460) |

### WittyCD (helper)

| In the module (7,800 Go) | In the server for it (2,160 Go, 1,440 web) |
|---|---|
| Reading CUE/BIN, CCD, CHD and other images, playing them as one stream, a FLAC encoder, CD-Text, and `lookup`: finding a disc on MusicBrainz from its table of contents (475) | `internal/cd` (510): a copy of its types. `internal/cdlookup` (385): **the module's `lookup`, all but 12 lines the same**. `modules/cdengine.go` (280). `api_cd.go` and `library/cd.go` (1,270). Web: the CD player and its drawing (`CdPlayer.tsx`, `CdGraphics.tsx`, `cdEngine.ts`, `cd.css`) |

### WittyPlay (service, with a part in the browser)

| In the module (23,800 Go, 10,200 web) | In the server for it (26,200 Go, 4,000 web) |
|---|---|
| The console tables, the libretro runner, cores and their downloads, patches, the host sandbox, streaming over WebRTC, controllers, the player in the browser, `wittyplay-check` and `wittyplay-probe` | `internal/play/...` (3,640, generated stubs; plus `hostsandbox`, 1,140, **a copy of the module's own with 22 lines changed**). `internal/gameid` (2,070): what a game file is from its bytes. `internal/gamearchive` (2,710): reading game archives safely in a child process. `internal/gamemeta` (4,660): names, art and facts from seven sources. `internal/gameregion` (160). The games library in `internal/library/games*.go` (4,100) and `api_games*.go`, `api_game_*.go`, `api_play.go` (7,900). Web: the games pages, game settings, region order (4,000) |

### WittyCouch (service) and WittyCouch Host (program)

| In the module (3,800 Go, 710 web; Host 570 C on Moonlight's core) | In the server for it (2,810 Go, 580 web) |
|---|---|
| Pairing with and talking to a game PC (`gamestream`), the relay to the browser, the player. Host: the separate program (AGPL-3.0-or-later, with Moonlight-derived parts under GPL-3.0) that speaks to the PC | `internal/couch/...` (1,360, generated stubs). `api_couch.go` (1,450): the PCs people added, pairing as a flow of pages, who may use which PC. Web: the Couch pages |

### WittyRigs (helper)

| In the module (75 Go) | In the server for it (1,570 Go, 550 web) |
|---|---|
| Finding where Sunshine is installed | `api_rigs.go`, `api_wittyrigs.go` (1,470): setting up, starting and watching Sunshine on the server, its apps list, and its settings. `library/rigs.go` (100). Web: `Rigs.tsx`, `GameRigs.tsx`, `WittyRigs.tsx` |

**The smallest module, with the most of its feature in the server.**

### WittyTuner (helper)

| In the module (5,220 Go) | In the server for it (5,650 Go, 1,000 web) |
|---|---|
| Every kind of tuner (network tuners, SAT>IP, DVB devices, IPTV playlists), finding them, scanning, opening a stream; `mpegts` (890) and `xmltv` (230) | `internal/tuner` (250): a copy of its types. `internal/tuner/mpegts` (600): the server's own scrambling check and table reader, **a part of the module's `mpegts`**. `internal/tuner/xmltv` (230): **the module's file, byte for byte**. `internal/livetv` (3,080): settings, channels, the guide from XMLTV and the broadcasts, opening streams within each tuner's limits, recording. `modules/tuner.go` (480), `api_tuners.go`, `streams_live.go`. Web: the guide, recordings, the encrypted-channel help |

### WittyPhotos (helper)

| In the module (910 Go) | In the server for it (1,730 Go, 680 web) |
|---|---|
| EXIF, picture sizes, the preview inside RAW files | `modules/photoengine` (70). `library/photos.go` (900): the photo library, places and thumbnails. `api_photos.go` (760). Web: the photo pages and map |

### Readers: WittyEbook, WittyPDF, WittyComics, WittyInk (web)

| In the modules (3,120 web) | In the server for them (5,010 Go, 190 web) |
|---|---|
| The readers themselves, and in WittyInk the `ReaderHost` contract, notes, definitions and handwriting. Already used outside the server by WittyReader | `internal/ebook` (2,600): turning Kindle, Palm, FictionBook, Word, OpenDocument, RTF, Markdown, web pages and text into EPUB, with a sanitiser. `library/books.go`, `comics.go`, `reading.go` (1,960): taking books and comics apart into chapters and pages. `api_books.go`, `api_reading.go`. Web: `Books.tsx`, `Reader.tsx` (the host) |

### WittyStats (web)

| In the module (3,790 web) | In the server for it (1,680 Go) |
|---|---|
| The screen: widgets, charts, tables, layouts, exports | `internal/metrics` and `api_metrics.go`: collecting the numbers and deciding who sees what. The module reads one stream, `/api/system/metrics/stream` |

### WittyVisualizer (web)

| In the module (3,950 web, 2,500 for the desktop player) | In the server for it (110 web) |
|---|---|
| All of it, with its own `VisualizerHost` and a copy for WittyMediaPlayer | `audio/visualizer.tsx`: where it's shown and saving its settings per person |

**Already stands on its own.**

### WittyIntegratedFileBrowser (helper, with a part in the browser)

| In the module (340 Go, 520 web) | In the server for it (430 Go, 45 web) |
|---|---|
| Finding drives and usual folders, listing a folder, the folder window | `api_browse.go`, `api_fs.go`: the routes, and reading a folder itself when a version can't |

**Already stands on its own.**

### WittyModule (library) and the host

| In the module (4,770 Go) | In the server (11,600 Go, 2,600 web) |
|---|---|
| The manifest, version rules, both ends of the helper protocol, services, `kit/facade`, and the release tool | `internal/modules`: installing, the store, the catalogue, sandboxes on each system, running helpers and services, failures. The store's pages. This is the host, and stays |

---

## Code that is in two places

| Server | Module | Difference |
|---|---|---|
| `internal/isofs/isofs.go` | `WittyDisc/isofs/isofs.go` | None. Gone since [#124](https://github.com/wittywilla/WittyMediaServer/pull/124): the server builds with WittyDisc's |
| `internal/tuner/xmltv/xmltv.go` | `WittyTuner/xmltv/xmltv.go` | None |
| `internal/cdlookup/cdlookup.go` | `WittyCD/lookup/lookup.go` | 12 lines (its package name, and `cd.TOC` from the server's copy of types) |
| `internal/play/hostsandbox/*` | `WittyPlay/hostsandbox/*` | 22 lines in `sandbox.go`; the module has FreeBSD and macOS limits the server's copy lacks |
| `internal/tuner/mpegts/mpegts.go` (600) | `WittyTuner/mpegts/mpegts.go` (890) | The server's is the part needed to refuse a stream: scrambling bits and the tables that name channels |
| `internal/dvd`, `internal/bluray`, `internal/cd`, `internal/tuner` | Each module's own types | Kept by hand, matched by field name |

The copies of `isofs`, `xmltv` and `hostsandbox` are already drifting apart from nothing: each fix has to be made twice.

---

## The server's import graph

What each feature package imports from the server (`internal/` left out):

| Package | Imports | Imported by |
|---|---|---|
| `isofs`, `dvd`, `bluray`, `cd`, `tuner/mpegts`, `tuner/xmltv`, `gameid`, `gameregion`, `zipsafe`, `ebook`, `midi`, `lyrics` | **Nothing** | — |
| `cdlookup` | `cd` | `library` |
| `tuner` | `tuner/mpegts` | `livetv`, `modules`, `server` |
| `subtitle` | `ebook` | `server` |
| `ffmpeg` | `dvd` (only `FileExtent`), `layout`, `logging` | most |
| `hls` | `ffmpeg`, `logging` | sessions, `server` |
| `dvdsession` | `dvd`, `ffmpeg`, `hls`, `logging` | `library`, `modules/dvdremote`, `server` |
| `bdsession` | `bluray`, `dvd`, `dvdsession`, `ffmpeg`, `hls`, `isofs`, `logging` | `library`, `modules/bdremote`, `server` |
| `gamearchive` | `gameregion`, `logging` (`Go`, `ReportPanic`), `play`, `play/hostsandbox`, `zipsafe` | `gamemeta`, `library`, `server` |
| `gamemeta` | `gamearchive`, `gameid`, `gameregion`, `logging` (`AddSecret`), `play`, **`db` (`Search`)** | `library`, `server` |
| `livetv` | `db`, `ffmpeg`, `library`, `logging`, `remote`, `sysinfo`, `tasks`, `tuner/...` | `server` |
| `play/...`, `couch/...` | WittyModule's `service` | `server`, `library`, `gamearchive`, `gamemeta`, `licenses` |
| `modules` | `cd`, `logging`, `tasks`, `tuner`, `tuner/mpegts`, `update`, WittyModule | `server`, `diagnose` |

So twelve packages could leave the server today with no change at all, and `gamearchive` and `gamemeta` need only a logger and a search function passed in.

---

## The target shape

1. **A module's repository holds its whole feature set**, as Go packages (or a TypeScript package) another project can import, plus the helper, service or bundle the server runs. Each module's README gets a section **Using it without WittyMediaServer**, with a short example.
2. **The server is a host.** It keeps what a host must: accounts and access, libraries and their database, saves, schedules, the store and the sandbox, routes and pages. Everything it knows of a module comes through WittyModule's API.
3. **Shared code lives in core libraries**, each its own Go module, repository and version, with `delivery: library` and `core: true`. Modules and the server build with them.
4. **The server may build with core libraries, never with a module.** A core library is compiled into the server at a version its `go.mod` pins and its `go.sum` checks, so a fork or a release can't change it. That keeps the reason the server has its own guard (it never takes a helper's word) while ending the copies. `scripts/publish-all.sh` checks this rule before every release.
5. **Types the server keeps of a module are generated from it**, the way `kit/facade` already does for WittyPlay and WittyCouch, never kept by hand.
6. **Nothing moves without its tests**, and a move never changes behaviour: the server's tests with real modules (`WMS_TEST_CATALOG`) pass before and after.

---

## Core libraries

| Library | Repository | Holds | Built with by | Size moved |
|---|---|---|---|---:|
| **WittyDisc** (exists, 0.2.0) | WittyDisc | `isofs` (there); `session` (there); **`guard`**: telling whether a DVD's sectors are scrambled and whether a Blu-ray's units carry AACS, from the server's `scramble.go` and `encrypted.go` and the engines' own checks, detection only; **`cdtoc`**: finding an audio CD from its table of contents (WittyCD's `lookup`) | WittyDVD, WittyBluray, WittyCD, WittyMediaPlayer, the server | 1,100 |
| **WittyBroadcast** (new) | WittyBroadcast | `mpegts`: transport stream tables, the programme guide inside streams, and the scrambling check; `xmltv`: guide files | WittyTuner, the server | 1,120 |
| **WittyGameCore** (new) | WittyGameCore | `gameid`, `gameregion`, `gamearchive` with `zipsafe`, the metadata sources from `gamemeta` (asked through an interface, with the caller's own keys), and `hostsandbox` | WittyPlay, the server | 10,500 |

Why these three and no more:

- **WittyDisc already is the disc core**: a library, `core`, required by both disc engines. It grows instead of a new one appearing. CD lookup joins it because it's what a disc *is*, and WittyCD can then drop its own copy.
- **WittyBroadcast** can't go in WittyTuner, because the server must not build with a module, and WittyTuner can't be forked while a library can't be loaded at all. It holds detection only.
- **WittyGameCore** is the biggest gap: a project wanting "games from files" today gets WittyPlay's runner and none of the identifying, unpacking or describing. `hostsandbox` goes with it because both WittyPlay's runner and the archive worker sandbox themselves with it, and WittyModule promises to have no dependencies (it needs `golang.org/x/sys`).
  **One thing to settle first:** `gamearchive` and `gamemeta` read WittyPlay's console table (`wittyplay.ByID` and `Console`, through the server's generated stubs) to know which file types each console takes. WittyGameCore can't import WittyPlay while WittyPlay builds with WittyGameCore, so the table's facts (consoles, their names, file types and regions) move into WittyGameCore with `gameid`, and WittyPlay keeps only what it runs them with (cores, controls, settings). That is part of the WittyGameCore move, not of 0.7.0-alpha.
- **Not proposed:** a separate library for disc menus or sessions. The shared part of `dvdsession` and `bdsession` is small once each engine owns its own steps, and it can live in WittyDisc (`present`) when that move comes.

---

## Module by module, where the code goes

| Module | Moves into it | Stays in the server, as host |
|---|---|---|
| WittyDisc | `guard`, `cdtoc`; later `present` (what's common to turning disc steps into streams and overlays) | Inserting and choosing discs; the player's pages |
| WittyDVD | Its steps turned into presentations: the DVD half of `dvdsession`, given a transcoder interface instead of the server's FFmpeg and HLS. Calls `wittydisc/guard` in `Open`, which can't be skipped | Running the helper, checking all it says (`dvdremote/check.go`), calling the guard itself, the transcoder, the routes |
| WittyBluray | As DVD, with `bdsession`, including the limits BD-J runs within | As DVD |
| WittyCD | Uses `wittydisc/cdtoc` instead of its own `lookup` | The library's CD pages; asking MusicBrainz (network stays the server's choice) |
| WittyPlay | Builds with WittyGameCore; can identify a file before choosing a core | The games library, saves, profiles, custom games, review, the metadata keys |
| WittyCouch | Anything in `api_couch.go` that is the pairing protocol rather than its pages and storage (to be read closely first) | The list of PCs, who may use them, the pages |
| WittyRigs | Setting up, starting, watching and configuring Sunshine (from `api_rigs.go`), behind calls in a `facade.json` | Routes, who may use it, where its settings are kept |
| WittyTuner | Builds with WittyBroadcast instead of its own `mpegts` and `xmltv` | The guide, recording, channel settings (`livetv`), and the server's own check of every stream |
| WittyEbook | Book conversion (`internal/ebook`), as a Go package and a helper that runs it boxed in, so WittyReader and others can open Kindle and Word books too | Taking a library's books apart for the API; the Reader page and its host |
| WittyComics | Taking comic archives apart into pages (`library/comics.go`), as for WittyEbook | As above |
| WittyStats | A `StatsSource` interface, as `ReaderHost` is for readers, so another program can feed it numbers | Collecting the numbers, who sees what |
| WittyPhotos, WittyPDF, WittyInk, WittyVisualizer, WittyIntegratedFileBrowser, WittyCouch Host | Nothing: each already holds its feature | — |
| WittyModule | `requireRanges` (below); a types-only mode of `kit/facade`, so the server's `internal/dvd`, `bluray`, `cd` and `tuner` are generated | — |

What a host must still bring, written once in each module's README: a transcoder for the disc engines, a place to keep saves and settings, and an account system, if it wants one.

---

## Declaring and installing what a module needs

`requires` is already in the manifest, the catalogue and the store, and the store installs requirements first. Two things are missing:

1. **Which versions.** Add **`requireRanges`**, an object from module id to a range in the same terms as `minServer` and `maxServer`:

   ```json
   "requires": ["wittyink"],
   "requireRanges": { "wittyink": { "min": "0.2.0", "max": "0.2.x" } }
   ```

   `requires` stays a plain list, so a 0.6.x server reads every catalogue as before. A requirement without a range takes any version. The store won't install a version whose requirements can't be met together, and says which one is in the way. `wittymodule check` refuses a range on something not in `requires`. Part of module API 4 (WittyModule 0.4.0), and of catalogue format +1 with release channels.
2. **Libraries are built in, not installed.** A requirement whose `delivery` is `library` (WittyDisc, WittyGameCore, WittyBroadcast) is met inside the module's own programs, at the version its `go.mod` names. The store shows it as "built in", installs nothing, and never lets it hold back an update. `wittymodule pack` writes the versions of every core library a program was built with into the archive's notices, so the store can show them.

---

## Encryption stays refused, everywhere

- **The checks move into the libraries, and only checks.** `wittydisc/guard` and `wittybroadcast/mpegts` say whether something is encrypted. They have no keys, no decryption, no option to turn them off, and no hook another package could fill. Their READMEs say why.
- **The engines can't open what the guard refuses.** WittyDVD's and WittyBluray's `Open`, WittyTuner's stream opening, and every `present` function call the guard first, so a project using only the libraries gets the refusal without asking for it.
- **The server still checks for itself.** It calls the same guard, at the version it pins, before a helper hears of a disc and on the bytes FFmpeg reads, and watches every broadcast stream. A helper's word that something is clear still means nothing.
- **WittyTuner still can't be forked**, and moving its parsing into WittyBroadcast doesn't change that: a library can't be loaded into a running server.
- Each library's `SECURITY.md` says that a way to make it pass encrypted media is a security report.

---

## Licences

Every file that would move was checked on 2026-11-04.

- **All of it is the project's own code, under AGPL-3.0-or-later**, as are the server and every module (WittyCouch Host's Moonlight-derived parts excepted, which stay GPL-3.0). Nothing that moves is derived from another project's code. The new libraries are AGPL-3.0-or-later, with the same NOTICE as the modules.
- **What comes along with WittyGameCore:** `bodgit/sevenzip` (BSD-3-Clause, with `afero` and `go4` under Apache-2.0 and `golang-lru` under MPL-2.0, used unmodified), `nwaples/rardecode` (BSD-2-Clause), `ulikunitz/xz`, `klauspost/compress`, `pierrec/lz4` (BSD-3-Clause), `brotli` and `ppmd` (MIT), `golang.org/x/sys` (BSD-3-Clause). All are already listed in the server's notices; the library's `THIRD-PARTY-NOTICES.txt` lists them again.
- **Game metadata sources** are services asked at run time, not code or data that moves. Their terms (CC BY-SA, CC BY-NC-SA, each service's API terms) go in WittyGameCore's README, and every program using it keeps showing them. The ScreenScraper developer credentials stay a build setting of the program, never in the library.
- **WittyCouch Host stays AGPL-3.0-or-later, with its Moonlight-derived parts under GPL-3.0,** in its own repository, and nothing moves into or out of it.
- No GPL code goes into a core library. FFmpeg stays a separate program, reached through the host's transcoder.

---

## Order of work

One issue per move, in the repository it lands in. Moves into the two new libraries are filed on the server until their repositories exist ([`docs/REPOSITORIES.md`](../REPOSITORIES.md) lists them for the owner to make).

**For 0.7.0-alpha**, only moves that change no behaviour:

| # | Move | Where | Issue |
|---|---|---|---|
| 1 | A test that the packages meant to leave import nothing else of the server, so they stay movable | WittyMediaServer | [#103](https://github.com/wittywilla/WittyMediaServer/issues/103): **done**, [#117](https://github.com/wittywilla/WittyMediaServer/pull/117) |
| 2 | The server reads disc images with WittyDisc's `isofs` (the same code) and deletes its copy; the release check allows core libraries | WittyMediaServer | [#104](https://github.com/wittywilla/WittyMediaServer/issues/104): **done**, [#124](https://github.com/wittywilla/WittyMediaServer/pull/124) |
| 3 | The owner makes the WittyBroadcast and WittyGameCore repositories | WittyMediaServer (owner step) | [#105](https://github.com/wittywilla/WittyMediaServer/issues/105) |

**For 0.8.0-alpha**, once those land:

| # | Move | Where | Issue |
|---|---|---|---|
| 4 | `requireRanges` in the manifest and the store's rules, and libraries shown as built in | WittyModule | [WittyModule#2](https://github.com/wittywilla/WittyModule/issues/2) |
| 5 | The catalogue carries `requireRanges` | WittyModuleCatalog | [WittyModuleCatalog#2](https://github.com/wittywilla/WittyModuleCatalog/issues/2) |
| 6 | The server honours `requireRanges` in the store | WittyMediaServer | [#106](https://github.com/wittywilla/WittyMediaServer/issues/106) |
| 7 | `guard` (DVD and Blu-ray encryption detection) in WittyDisc | WittyDisc | [WittyDisc#1](https://github.com/wittywilla/WittyDisc/issues/1) |
| 8 | WittyDVD calls `wittydisc/guard` | WittyDVD | [WittyDVD#1](https://github.com/wittywilla/WittyDVD/issues/1) |
| 9 | WittyBluray calls `wittydisc/guard` | WittyBluray | [WittyBluray#2](https://github.com/wittywilla/WittyBluray/issues/2) |
| 10 | The server's own disc checks use `wittydisc/guard` | WittyMediaServer | [#107](https://github.com/wittywilla/WittyMediaServer/issues/107) |
| 11 | `cdtoc` (CD lookup) in WittyDisc | WittyDisc | [WittyDisc#2](https://github.com/wittywilla/WittyDisc/issues/2) |
| 12 | WittyCD uses `wittydisc/cdtoc` | WittyCD | [WittyCD#2](https://github.com/wittywilla/WittyCD/issues/2) |
| 13 | The server uses `wittydisc/cdtoc` and deletes `internal/cdlookup` | WittyMediaServer | [#108](https://github.com/wittywilla/WittyMediaServer/issues/108) |
| 14 | WittyBroadcast: `mpegts` and `xmltv`, from WittyTuner's | WittyMediaServer, until the repository exists | [#109](https://github.com/wittywilla/WittyMediaServer/issues/109) |
| 15 | WittyTuner builds with WittyBroadcast | WittyTuner | [WittyTuner#2](https://github.com/wittywilla/WittyTuner/issues/2) |
| 16 | WittyGameCore: `gameid`, `gameregion`, `gamearchive`, `zipsafe`, `hostsandbox`, then the metadata sources, from the server; the server builds with it | WittyMediaServer, until the repository exists | [#110](https://github.com/wittywilla/WittyMediaServer/issues/110) |
| 17 | WittyPlay builds with WittyGameCore's `hostsandbox`, and identifies files with `gameid` | WittyPlay | [WittyPlay#19](https://github.com/wittywilla/WittyPlay/issues/19) |
| 18 | `kit/facade` generates types only, for helpers | WittyModule | [WittyModule#3](https://github.com/wittywilla/WittyModule/issues/3) |
| 19 | The server's `dvd`, `bluray`, `cd` and `tuner` types are generated | WittyMediaServer | [#111](https://github.com/wittywilla/WittyMediaServer/issues/111) |

**Later**, each a release of its own module:

| # | Move | Where | Issue |
|---|---|---|---|
| 20 | DVD presentation moves into WittyDVD, behind a transcoder interface; the common part into WittyDisc `present` | WittyDVD | [WittyDVD#2](https://github.com/wittywilla/WittyDVD/issues/2) |
| 21 | Blu-ray presentation moves into WittyBluray | WittyBluray | [WittyBluray#3](https://github.com/wittywilla/WittyBluray/issues/3) |
| 22 | The server hands disc sessions to the engines and keeps the transcoder and the checks | WittyMediaServer | [#112](https://github.com/wittywilla/WittyMediaServer/issues/112) |
| 23 | Sunshine set-up and control move into WittyRigs | WittyRigs | [WittyRigs#1](https://github.com/wittywilla/WittyRigs/issues/1) |
| 24 | Book conversion moves into WittyEbook | WittyEbook | [WittyEbook#2](https://github.com/wittywilla/WittyEbook/issues/2) |
| 25 | Comic archives taken apart in WittyComics | WittyComics | [WittyComics#2](https://github.com/wittywilla/WittyComics/issues/2) |
| 26 | `StatsSource`, so WittyStats runs on other numbers | WittyStats | [WittyStats#2](https://github.com/wittywilla/WittyStats/issues/2) |
| 27 | Read `api_couch.go` for protocol code that belongs in WittyCouch | WittyCouch | [WittyCouch#3](https://github.com/wittywilla/WittyCouch/issues/3) |
| 28 | "Using it without WittyMediaServer" in every module's README | each module, with its move | — |

Each issue links back to [#98](https://github.com/wittywilla/WittyMediaServer/issues/98) and to the issues it needs first. The pull requests go in that order across repositories: the library, then the module, then the server, then the catalogue.

---

<sub>WittyMediaServer development plans</sub>
