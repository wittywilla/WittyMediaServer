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
| **WittyPlay** | Console games streamed to a browser or an app: a libretro game runner in its own sandboxed process, WebRTC with fallbacks, every controller, a touch layout editor | WittyMediaServer |
| **WittyIntegratedFileBrowser** | Choosing a server's folders from any device: a Go listing of drives, folders and files, and the React window that shows it (in `web/`). Read-only | WittyMediaServer |
| **WittyModule** | What makes a plugin a project of its own: the manifest, version ranges, the helper protocol and the release tool. Go, no dependencies | Every plugin, WittyMediaServer |
| **WittyPhotos** | What a photo says about itself, without decoding it: EXIF, dimensions and the preview in a RAW file. Go, fuzz-tested, and can update without a new server | WittyMediaServer |
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
| Games | **WittyPlay** |

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
  wittymodule/              → github.com/wittywilla/WittyModule
  wittyphotos/              → github.com/wittywilla/WittyPhotos
  wittytuner/               → github.com/wittywilla/WittyTuner
  wittyplay/                → github.com/wittywilla/WittyPlay (Go, with its React half in web/)
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
2. **Always has a built-in copy.** Every plugin is compiled into the server, so the server works
   offline and on any platform. A plugin that has been converted can also be replaced at runtime
   by a newer signed release of its own (see [Independent updates](#independent-updates)), and
   nothing unsigned is ever fetched and run.
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

## Independent updates

A plugin can be updated without a new server, and each plugin says which servers it works with.

### Plugins are their own projects

A plugin is heading towards being a project of its own, used by the server, rather than a
read-only copy of a folder in this repository. Each one has its own repository, version number,
releases and tests, and needs nothing from this repository to build, sign or publish. The server
uses a plugin in two ways, and both come from the plugin's own releases:

- **The built-in copy**: `go.mod` requires a released version, which is compiled in as the fallback.
- **Newer signed releases**: downloaded at runtime from the plugin's own GitHub releases.

The cost is that a change touching both the server and a plugin becomes two commits and two
releases. The [module API version](#the-manifest) is what keeps those two apart safely.

**Where this stands today:** the code is still developed in this repository and mirrored, because
moving it means pushing and changing repositories. Each plugin folder is already built to stand
alone, so [the cutover](#moving-a-plugin-to-its-own-repository) is short.

### How each kind can really be replaced

Go linked into the binary cannot be swapped. These are the options that were weighed:

| Option | Works on every platform we ship? | Speed | Isolation | Verdict |
|---|---|---|---|---|
| Go's `plugin` package | No: no Windows, and needs the exact same toolchain build | Native | None, a crash takes the server down | Rejected |
| **Helper process** | Yes: one build per platform, same as the server's 10 targets | Native; one more small process while in use | A crash or hang is only a failed call. Can be sandboxed | **Chosen for Go** |
| WebAssembly (wazero) | Yes, but ARMv7 and 386 only get the slow interpreter | Below; see the numbers | Best: no files or network unless granted | Not now |
| **Runtime-loaded web bundle** | Yes, it runs in the browser | Same as today | Same as the app's own scripts, so signing is what protects it | **Chosen for web** |

Measured with the WittyPhotos code on the development PC (a low-end device will be several times
slower):

| | Helper | WebAssembly |
|---|---|---|
| Download size, compressed | 1.3 to 1.5 MB per platform | 1.4 MB, one file for all |
| Getting ready to run | About 0.2 s to start, including the shell's overhead | 1.3 s to compile, using 47 MB more memory, or 0.1 s and 64 MB with the interpreter |
| Memory while running | 22 MB while scanning, and none after a minute idle | At least what is above, while the server runs |
| Added to the server | Nothing | About 2.5 MB for the wazero runtime |

WebAssembly's one-file-for-all is appealing and its sandbox is the strongest, but compiling costs
more than the job it does on a small box, and the two 32-bit targets would run it interpreted. It is
worth another look if a plugin ever runs code from strangers. Today every plugin is signed by the
project.

| Plugin | Decision | Why |
|---|---|---|
| **WittyPhotos** | Helper: **converted** | Stateless parsers of untrusted files, and scanning already does file I/O per call |
| WittyDVD, WittyBluray | Helper, one per open disc, **later** | Performance turns out not to be the obstacle: FFmpeg already reads the disc image directly from the extents the engine gives it, so video never passes through the engine, only menus and button presses. The work is the size of the API the session code uses (`internal/dvdsession`, `internal/bdsession`) |
| WittyDisc | Goes with the engines | A library they link. It has no release of its own to run |
| WittyTuner | Helper, **later** | Possible, since streams can go straight to FFmpeg, but it holds device access and needs care per platform |
| WittyPlay | Already half done | `wittyplay-host` is a separate process and can be delivered as is. The streaming half (WebRTC) is still in the server |
| WittyIntegratedFileBrowser | Stays built in | About 230 lines of read-only listing. A process for it would cost more than it could ever save, and it is security-sensitive enough to move with the server |
| WittyInk, WittyEbook, WittyPDF, WittyComics | Web bundle, **later** | Designed below; not built yet |

**Web bundles (design, not built).** A release holds ES modules built with `react` and
`@wittywilla/wittyink` left external, served from `/api/modules/{id}/web/`, so the app's existing
`script-src 'self'` still applies. The app asks for the active version and uses `import()` on it, and
if that fails it uses the copy in its own bundle. A bundle runs with the signed-in person's access, so
it is held to the same signature rule as a helper. Service workers and the desktop and TV clients'
caching need checking when this is built.

### The manifest

Every plugin has a `module.json`, compiled into its built-in copy and shipped in its releases, so the
two can't disagree:

```json
{
  "id": "wittyphotos",
  "name": "WittyPhotos",
  "version": "0.2.0",
  "repo": "wittywilla/WittyPhotos",
  "delivery": "helper",
  "entry": "wittyphotos-module",
  "api": 1,
  "minServer": "0.4.0-alpha",
  "maxServer": "0.5.x"
}
```

`delivery` is `helper`, `web`, or `builtin` for a plugin that isn't converted yet and only updates with
the server. `api` is the **module API version** it was built for. The server has a range it speaks
(`wittymodule.API` down to `wittymodule.MinAPI`). Adding to the API raises only the top. Breaking it
raises the bottom too, which disables older plugins, so that is done rarely. It is one number for the
whole server, as the helper protocol and host interfaces are one contract.

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
that plugin's pre-release. A stable server never gets one by surprise, but keeps one it already has.

A server whose version can't be read (a build from a commit with no tag) can't be compared, so it uses
built-in copies only and says so.

**When a server upgrade leaves a plugin out of range,** nothing breaks. On start, the server takes the
first of these that passes, and the settings page says in plain words what happened:

1. The chosen installed version.
2. The previous installed version.
3. The built-in copy, which is always compatible because it was compiled in.

The refused version stays on disk, in case the server is rolled back, and the next check looks for a
release whose range fits. If the built-in copy is newer than the installed one, the built-in one is
used. A plugin never holds a server back.

### Updating

Nothing leaves the server unless settings allow it, and the server works the same offline.

- `moduleUpdateCheck` looks for plugin updates twice a day. If never set, it follows `updateCheck`, so
  an owner who turned off update checks gets no new traffic. Pressing **Check now** always works.
- `moduleAutoInstall` installs compatible updates without asking. Off by default.

**The same trust as server updates:** the keys in `internal/update/keys.go`, Ed25519 over a checksum
list, GitHub over HTTPS as the only source. There is one signing key rather than one per plugin, since
a helper runs with the same power as the server and a second key would protect nothing more. Only
plugins the server already knows are accepted, and this project's key must have signed them; plugins
from other people aren't supported.

A release of version `V` has one signed list covering everything:

```
<id>-V-SHA256SUMS.txt       hashes of each archive, the manifest, and every file in each archive
<id>-V-SHA256SUMS.txt.sig   its signature
<id>-V-module.json          the manifest, so a release can be judged before downloading it
<id>-V-<os>-<arch>.tar.gz   the files (.zip on Windows)
```

**Check.** Reads the plugin's releases, newest first. For each newer one, up to five, it fetches the
list, signature and manifest, and verifies them. The first compatible one is offered. A newer one that
doesn't fit is reported ("0.3.0 needs server 0.5.0 or newer") so the reason is visible.

**Install**, each step of which leaves the running plugin untouched if it fails:

1. Verify the list's signature, and the manifest's hash and compatibility.
2. Download the archive, hashing as it arrives, and stop at the size in the release.
3. Unpack to a work folder, refusing links and paths that climb out.
4. Check every file against the signed list, refusing any extra or missing file.
5. Start the helper once. Its hello must name the same plugin, version and API.
6. Rename the folder into `versions/V`, then save the state atomically.
7. Switch over. The next call uses the new version, with no server restart.

**Rollback.** Automatic if three calls in a row fail at the process level (crash, hang, garbage): that
version is blocked with the reason and the previous version or built-in copy takes over. A bad file
that a parser refuses is not a failure. An admin can also roll back by hand. A blocked version isn't
installed automatically again, but an admin can still choose to.

**On disk.** Everything is in `<data>/modules/`, never in the program folder, so this also works in
containers and in protected installs. Only the active and previous versions are kept. The signed list
stays beside the files and is checked again every time a helper starts, so a file changed on a shared
data folder is refused rather than run. If the data folder doesn't allow programs to run (`noexec`,
common on network shares), the install fails at step 5 with a plain message and the built-in copy
carries on.

### The helper protocol (module API 1)

A helper is started with `--wms-helper` and talks over stdin and stdout: one line of JSON per message,
each at most 1 MiB, optionally followed by a binary blob of the size it announced (at most 128 MiB).
Sizes are checked before memory is set aside, and the server treats everything a helper says as
untrusted.

```
helper → {"module":"wittyphotos","version":"0.2.0","api":1}
server → {"id":1,"method":"read","params":{"path":"..."}}
helper → {"id":1,"result":{...},"blob":0}
helper → {"id":2,"error":"...","code":"no-preview"}
```

A helper exits when stdin closes, so it can't outlive the server. The server starts it on first use and
stops it after a minute idle, so it costs nothing when unused. `github.com/wittywilla/wittymodule` has
both ends of the protocol, plus the manifest, version rules and release tool, and has no dependencies.

**Limits on a helper:** a small environment, a soft memory limit, one call at a time with a time limit,
and a lower priority. Windows adds a low-integrity token (it can't write to the server's files) and a
job (it dies with the server, memory cap, no child processes). Linux adds empty network and IPC
namespaces where the system allows them. Encrypted media stays refused: no plugin, helper or not, gets
a way around that.

### Releasing a plugin

`wittymodule` does this from the plugin's own folder, with no help from this repository:

```bash
go run github.com/wittywilla/wittymodule/cmd/wittymodule check .
go run github.com/wittywilla/wittymodule/cmd/wittymodule pack -out release .
go run github.com/wittywilla/wittymodule/cmd/wittymodule sign release/wittyphotos-0.2.0-SHA256SUMS.txt
```

`sign` uses the same key as `wms-sign` (`WMS_SIGNING_KEY`, then `~/.wms-signing/release.key`). A plugin's
`.github/workflows/release.yml` runs these when a `v*` tag lands in its own repository, then publishes
the GitHub release. That workflow does nothing in this repository, since GitHub only reads workflows
from a repository's root. While a plugin still lives here, `scripts/release-module.sh modules/wittyphotos
v0.2.0` does the same from your own machine.

CI runs `wittymodule check -server <this server's version>` over every manifest, so a server version
that falls outside a plugin's range fails until its `maxServer` is raised.

### Moving a plugin to its own repository

These steps need a person, and go in this order. WittyPhotos goes second, after WittyModule, since it
requires it:

1. Publish `WittyModule`: `scripts/mirror.sh WittyModule v0.1.0`. It has no dependencies.
2. In `modules/wittyphotos`, run `GOWORK=off go mod tidy` and commit, then publish it with
   `scripts/mirror.sh WittyPhotos v0.2.0`.
3. Add the secret `WMS_SIGNING_KEY` to that repository, so tags there produce signed releases.
4. From now on, change WittyPhotos in its own repository. In this one, remove its folder, its line in
   `go.work`, its `replace` in `go.mod`, its `COPY` in the `Dockerfile` and its line in `scripts/mirror.sh`.
5. Run `go get github.com/wittywilla/wittyphotos@v0.2.0` here. Raising this version later is how the
   built-in copy moves forward.

## How the apps run

- **WittyReader** is a web app at heart: the reader plugins with a host that opens files from
  the disk (or dropped on the window) and keeps notes on the device. It ships as a desktop app
  and can also be installed from the browser. No server needed.
- **WittyMediaPlayer** is a Qt 6 window that decodes with FFmpeg's libraries, and runs the WittyDVD
  and WittyBluray engines in a small helper program beside it, reached over a pipe with the WittyModule
  helper protocol. There's no web view and no loopback server, and it never uses the network. See
  [its design note](../apps/wittymediaplayer/DESIGN.md).
- **Other native windows** use the system's own web view (WebView2 on Windows 10+, WebKit on macOS and
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
5. **WittyMediaPlayer** (first version done, on Windows): a Qt 6 window with the WittyDVD and WittyBluray
   plugins for discs, under `apps/wittymediaplayer`. Other platforms come after Windows.
