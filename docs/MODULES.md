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
| **WittyVisualizer** | Light shows that follow the music: seventeen effects written as shaders, with audio analysis, a flash limiter and a calm mode. The same effect files run on WebGL 2 and on Qt's QRhi. Off until someone turns it on, and costs nothing until then | WittyMediaServer, WittyMediaPlayer |
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

Until [the cutover](MODULES-CUTOVER.md), everything is still developed in this repository, with each plugin in its own folder. Each folder is built to be its repository's root: it holds its own workflows, licence and README, and needs nothing from here to build, sign or publish.

```
web/src/modules/            web plugins (React)
  wittyink/                 → github.com/wittywilla/WittyInk
  wittyebook/               → github.com/wittywilla/WittyEbook
  wittypdf/                 → github.com/wittywilla/WittyPDF
  wittycomics/              → github.com/wittywilla/WittyComics
  wittyvisualizer/          → github.com/wittywilla/WittyVisualizer (with its Qt host in qt/)
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

### Versions

Versions move in small steps as work lands. A bump isn't a release: nothing is tagged or published by one.

```bash
go run ./modules/wittymodule/cmd/wittymodule bump -note "Reads a new format." -changelog CHANGELOG.md -consumer . modules/wittyphotos
scripts/bump-server.sh 0.4.1-alpha "What changed."
```

The first raises a plugin's version in its `module.json` and `package.json`, in the server's `go.mod` and web dependencies, and in the changelog, and refuses anything not newer. The second does the same for the server and says which plugins' ranges no longer cover it. In a plugin's own repository, CI runs `wittymodule needs-bump`, which fails when it has changed since its last tag and still has that tag's version, since tags never move.

Versions stay below v1 while the APIs settle, so a minor version may change them.

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

A plugin is a project of its own, used by the server, rather than a read-only copy of a folder in this
repository. Each one has its own repository, version number, releases and tests, and needs nothing
from this repository to build, sign or publish. The server uses a plugin in two ways, and both come
from the plugin's own releases:

- **The built-in copy**: `go.mod` requires a released version, which is compiled in as the fallback.
- **Newer or older signed releases**: downloaded at runtime from the plugin's own GitHub releases.

The cost is that a change touching both the server and a plugin becomes two commits and two
releases. The [module API version](#the-manifest) is what keeps those two apart safely.

**Where this stands today:** the code still lives in this repository until the cutover, which needs
pushing and tagging and so waits for the owner's go-ahead. Every piece is built and tested. The
checklist and script are in [MODULES-CUTOVER.md](MODULES-CUTOVER.md). Until then no release of any
plugin exists in its new form, so nothing new reaches anyone yet.

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
worth another look if plugins from strangers become common.

### What each plugin can really do

"Apart from the server" means a release of its own reaches a server with no new server. No plugin is all the way there, since part of each lives in the server or the app, so this says which part. The same words are in each plugin's manifest and on the Modules page.

| Plugin | Updates apart from the server? | What a release changes | What still needs a new server, and what it would take |
|---|---|---|---|
| **WittyPhotos** | **Yes**, a helper | Reading EXIF, picture sizes, the preview in RAW files | The photo pages in the app |
| **WittyIntegratedFileBrowser** | **Yes**, a helper | Finding drives and folders, and listing them | The window in the app, and naming what kind each file is |
| **WittyDVD** | **Mostly**, a helper per open disc | The engine a viewer plays through: the disc's structure, menus, buttons and commands, where its video lies, subtitles | Library scanning, the title list on a disc's page and playing a title without its menus still use the built-in copy. Moving those means a helper during scans too. The session code turning steps into streams stays, as does refusing encrypted discs, on purpose |
| **WittyBluray** | **Mostly**, a helper per open disc | Playing a disc: playlists and clips, running its program, reading menus and subtitles | Library scanning, drawing menus, and discs with Java menus, which the built-in copy plays because Java is tied into the server's session. The server opens every disc itself first, to check encryption and Java, so a disc it can't read at all needs a new server |
| **WittyTuner** | **Partly**, a helper per question and per stream | Network tuners, IPTV over HTTP and external backends | Custom commands, capture cards, Linux DVB, SAT>IP and other IPTV kinds, which need other programs or this machine's devices, and a helper may use neither. Recording and the guide. Refusing encrypted channels stays on purpose: the server watches every stream itself |
| **WittyPlay** | **Partly**, a program | Running games: how the runner uses cores, its picture, sound and controller input | The streaming half (WebRTC and encoding), the console tables and the player. Moving those means splitting the server's streaming into a process of its own, which low-end boxes pay for on every frame, so it stays for now |
| **WittyEbook** | **Yes**, a web bundle | The reader itself | The Reader page and the host it provides, and how the server takes a book apart |
| **WittyPDF** | **Mostly**, a web bundle | The reader itself | pdf.js and its worker, which do the actual reading, so it isn't carried twice |
| **WittyComics** | **Yes**, a web bundle | The reader itself | The Reader page and the host, and how the server opens comic archives |
| **WittyVisualizer** | **Yes**, a web bundle | The visualizer itself: its effects, how it listens, its controls and look | Where it shows in the audio player and how settings are saved. The desktop app builds in its own copy, with shaders baked at build time |
| **WittyInk** | **Partly**, a web bundle | What readers share | It only reaches a reader that is itself running a bundle: one using the app's copy has the app's WittyInk |
| **WittyDisc** | **No**: a library | Nothing on its own | It reaches a server inside WittyDVD and WittyBluray's helpers. The built-in copies move with a new server |
| **WittyModule** | **No**: a library | Nothing on its own | The contract between the two sides, so it changes with care and reaches a server inside the plugins built with it |

Films, music, podcasts, vertical videos and creator videos (WittyVideo, WittyAudio, WittyPodcasts, WittyShorts, WittyChannels) are part of the server. They can be added and removed like any plugin, and update with the server.

**What it costs.** Nothing at all until a downloaded version is in use: the built-in copy runs as it always has. With one, opening a disc adds a process of 23 to 109 MB and 1 to 3 seconds, and a button press costs about a millisecond more. Video never passes through a helper. A reader bundle adds one small request when a reader opens, and 3.9 KB to the Reader page.

**Keeping the two sides in step.** The helpers and the server compile the same types separately. The disc engines send them in Go's self-describing format, which tolerates added fields. Anything that changes their meaning raises the module API. WittyPlay's runner has its own number (`ipc.Version`, and `capabilities.ipc` in its manifest), and a server refuses a runner with another. When a session calls a new engine method, it has to be added to that session's `engine.go`, the helper and the checks on what comes back.


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
  "maxServer": "0.5.x",
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
| `helper` | A program per platform | A separate process that speaks the [helper protocol](#the-helper-protocol-module-api-1) |
| `program` | A program per platform | The server starts it for one job, in that job's own protocol. It must also answer the hello, so an install can check it runs |
| `web` | One bundle for every platform | The browser loads it under the page's content security policy |
| `library` | Nothing to install | Linked into others, and reaches a server inside them |
| `builtin` | Nothing to install | Part of the server, and updates with it |

`api` is the **module API version** it was built for. The server has a range it speaks
(`wittymodule.API` down to `wittymodule.MinAPI`). Adding to the API raises only the top. Breaking it
raises the bottom too, which disables older plugins, so that is done rarely. It is one number for the
whole server, as the helper protocol and host interfaces are one contract. API 2 adds attached helpers,
streams, programs and bundles to API 1.

The rest is there for the people choosing: `group`, `summary` and `needs` describe it, `libraries` says
which kinds of library need it, `requires` and `core` say what it is built on, and `updates` and `stays`
say honestly which parts of it a release really changes and which only a new server can.

`dataFormat` and `dataFrom` number the shape of the data a version keeps and the oldest it can read.
See [Data survives a version change](#data-survives-a-version-change).

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

A server whose version can't be read (a build from a commit with no tag) can't be compared, so it uses
built-in copies only and says so.

**When a server upgrade leaves a plugin out of range,** nothing breaks. On start, the server takes the
first of these that passes, and the settings page says in plain words what happened:

1. The chosen installed version.
2. The previous installed version.
3. The built-in copy, which is always compatible because it was compiled in.

The refused version stays on disk, in case the server is rolled back, and the next check looks for a
release whose range fits. If the built-in copy is newer than the installed one, the built-in one is
used, unless someone chose the older version on purpose. A plugin never holds a server back.

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
previous version or built-in copy takes over. A bad file that a parser refuses is not a failure. An admin
can also roll back by hand. A blocked version isn't installed automatically again, but an admin can still
choose to. Only an admin's browser counts towards this: anyone else's report goes in the module's log for an admin to
see, and their page has already fallen back to the app's copy.

**On disk.** Everything is in `<data>/modules/`, never in the program folder, so this also works in
containers and in protected installs. The active, previous and pinned versions are kept. The signed list
stays beside the files and is checked again every time a helper starts, so a file changed on a shared
data folder is refused rather than run. If the data folder doesn't allow programs to run (`noexec`,
common on network shares), the install fails at step 5 with a plain message and the built-in copy
carries on.

### Choosing a version

**Settings, Modules** lists every version a module has released, newest first, with its notes. A
version that doesn't fit this server is shown with the reason, and can't be chosen.

Choosing an older one warns first: it may lack fixes, including security fixes. Then it is installed and
**pinned**, or the next automatic update would undo the choice. A pin holds through checks and restarts,
and can also be set on whatever is in use. **Go back to the latest** ends the pin and moves to the newest
version that fits, whether that is a release or the built-in copy. A version already on disk is switched
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

Every module's code is compiled in, so adding one works straight away and offline. What a server has decides
which **libraries can be made**, and that is enforced by the server, not only by the app: creating a library
of a kind whose module isn't there is answered with what it needs.

- **First-run setup** asks what you want, before you make libraries.
- **Add library** shows the kinds that need a module in a greyed section, each with a button to add it there and
  then, and a link to the store.
- **Settings, Modules** is the store.

**When two modules bring the same kind of library,** they share it: a discs library is one kind, and WittyDVD and
WittyBluray each play their half of what's in it. The store says so under **Brings these libraries**, naming the others.
A kind can also be given a name of this server's own there, such as "Blu-rays and DVDs", which is what Add library then
shows. A name another kind already has is refused, so two can't read alike. What a kind is called doesn't change what it
is, and clearing the name goes back to the usual one. Only the kinds a module's built-in copy declares count: a release
or a fork can't claim a kind it didn't come with. A test lists which modules share a kind on purpose, so a new module
that happens to pick one already in use is caught when it is added.

A kind of library is available if any one module providing it is there, so a discs library needs WittyDVD or
WittyBluray. A module can't be removed while libraries need it, and the reason names them. Removing touches no
data. Its downloaded versions go, because the built-in copy is there if it comes back. Leaving a module out
doesn't save disk space, since the code is still in the server: what it saves is a kind of library you don't
want, and its update checks. Core modules, which others are built on, stay. A server that has never chosen has
everything, so updating to this changes nothing.

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
This isn't a way to add decryption. The refusal doesn't depend on the module. There is no list of recommended forks
and no store of other people's modules, and that is deliberate.

To publish a fork: change `repo` in `module.json` to yours, keep the `id`, run `wittymodule keygen -key <file>`,
and add that key as your repository's `WMS_SIGNING_KEY` secret. The release workflow that comes with the module
does the rest. Give people the public key it printed.

### Custom modules: in development

Wholly new kinds of module, rather than replacements for ones the server already knows, aren't possible yet. The
server only accepts modules it knows about, because it has nothing to connect an unknown one to. The store has a
section saying so. What it would take: a host interface a helper can call back through, a way for one to add a
library type and its pages, and a permission model strong enough for code nobody here has seen. WebAssembly, above,
is the likely way to contain them.

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
stops it after a minute idle, so it costs nothing when unused. A job that keeps state, such as an open
disc, has a helper of its own for as long as it lasts. A stream is a helper that gives one answer and then
writes the stream. `github.com/wittywilla/wittymodule` has both ends of the protocol, plus the manifest,
version rules and release tool, and has no dependencies.

**Limits on a helper:**

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
Encrypted media stays refused: no plugin, helper or not, gets a way around that.

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
clean machine, in a `release` environment, and sees only the checksum list and the key. `scripts/module-kit.sh`
generates these workflows, and they do nothing in this repository, since GitHub only reads workflows from a
repository's root.

CI runs `wittymodule check -server <this server's version>` over every manifest, so a server version
that falls outside a plugin's range fails until its `maxServer` is raised.

### Signing

**The recommendation is one module key, separate from the server's release key, not one per module.**

- A release key can sign a server, which runs with no limits at all. A key that signs modules has to sit in
  thirteen repositories' secrets, where it is far more exposed, so it shouldn't be able to. `moduleKey` in
  `internal/update/keys.go` is trusted for modules and never for a server.
- One key per module would mean thirteen keys to look after, and a new server for every rotation. What it would buy
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

### Moving a plugin to its own repository

[MODULES-CUTOVER.md](MODULES-CUTOVER.md) has the steps, in order, for each repository, and
`scripts/cutover-modules.sh` does them: it proves the whole thing in a throwaway clone by default, and only touches
GitHub with `--apply`.

### Developing a server and a module together

Once modules have their own repositories, the server's `go.mod` requires released versions, and that is all a normal
build needs. To change a module and the server together:

```bash
scripts/dev-modules.sh WittyDVD
```

clones it beside this checkout and makes a local `go.work`, which is never committed, that uses your copy. Change
both, test both, then release the module first and raise its version here with
`go get github.com/wittywilla/wittydvd@vX.Y.Z`.

## How the apps run

- **WittyReader** is a web app at heart: the reader plugins with a host that opens files from
  the disk (or dropped on the window) and keeps notes on the device. It ships as a desktop app
  and can also be installed from the browser. No server needed.
- **WittyMediaPlayer** is a Qt 6 window that decodes with FFmpeg's libraries, and runs the WittyDVD
  and WittyBluray engines in a small helper program beside it, reached over a pipe with the WittyModule
  helper protocol. Its disc modules update on their own, under the rules below. There's no web view and no loopback server, and only
  looking for module updates uses the network. See
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
