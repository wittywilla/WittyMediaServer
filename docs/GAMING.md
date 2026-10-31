# Gaming: design

Games sit in libraries next to films and music. You open one in a browser, on any device, and
play it straight away with whatever controller you have. Saves follow your account. Games run
on the server and stream to the browser with low latency, through **WittyPlay**
(`modules/wittyplay`).

This page explains how it is built and why. It is the reference for anyone changing it.
[GAMING-API.md](GAMING-API.md) is the API a client uses, including the Apple TV and Android TV apps.

## What it has to do

- A Gaming library type, organised by console, with several libraries of one console allowed.
- Games run through libretro cores and stream to the browser.
- Every controller: gamepads (standard mapping), keyboard, on-screen touch controls, remapping,
  hot-plug, several players.
- Every browser: Chromium, Firefox and Safari, on desktop and phones.
- Saves and save states per account, and resume where you left off.
- Runs on low-end servers. Emulators are memory-unsafe code fed untrusted files, so they stay
  boxed in.
- WittyMediaServer stays Apache-2.0. No GPL code in its binaries, no non-commercial cores
  offered, and no ROMs or BIOS files bundled, downloaded or linked.

## The architecture

```
 Browser                     wms (the server)                 wittyplay-host (one per game)
┌─────────────┐  WebRTC   ┌───────────────────────┐  pipes   ┌──────────────────────────┐
│ <video>     │◄──────────│ stream.Session        │◄─────────│ libretro frontend        │
│ gamepad,    │  H.264 or │  ├ pion WebRTC        │  raw     │  ├ loads one core        │
│ keyboard,   │  VP8 +    │  ├ WebSocket fallback │  frames, │  ├ runs at the core's    │
│ touch       │  Opus     │  ├ fan-out to viewers │  PCM     │  │  own frame rate       │
│             │──────────►│  └ saves to disk      │─────────►│  └ sandboxed, no network │
└─────────────┘  input on │        ▲ │            │  input,  └──────────────────────────┘
                 a data   │        │ ▼            │  commands
                 channel  │  ffmpeg (video)       │
                          │  ffmpeg (audio)       │
                          └───────────────────────┘
```

Three kinds of process, each doing one job:

1. **`wittyplay-host`**: our own small libretro frontend, written in Go with no cgo. It loads a
   core, runs it one frame at a time and writes raw frames and audio to its stdout. It reads
   input and commands on stdin. It holds no network connection and no database. A crash or an
   exploited core takes down one game, not the server.
2. **FFmpeg**: already part of every install. Video and audio each get their own process, so the
   hardware encoder detection the transcoder uses works here too.
3. **`wms`**: holds the WebRTC connections and writes save files. It never loads a core.

### Two runners, one protocol

"The process that runs a game" is called a **runner**, and it's defined by a protocol rather than by a
program: length-prefixed messages over a pipe (`modules/wittyplay/ipc`). Two programs can fill that role.

| | `wittyplay-host` (built in, works today) | WittyRetroArch (a GPL fork, not built yet) |
|---|---|---|
| What it is | Our own small libretro frontend | RetroArch with a mode that speaks the protocol |
| Licence | Apache-2.0 | GPLv3, a separate program |
| Software-rendered cores | Yes | Yes |
| Hardware-rendered cores (N64, PSP, Dreamcast) | No | Yes: this is why it would exist |
| Server with no GPU | Yes | For software-rendered cores |
| Size and upkeep | About a thousand lines of Go, cross-compiled from one machine | A large C codebase to build per platform and merge from upstream |

The server uses WittyRetroArch when it finds one, and the built-in runner otherwise. An admin can choose
in Settings, Gaming.

Unmodified RetroArch can't fill this role: its only way to hand frames to another program is its
recording feature, which muxes and adds several frames of delay, and it needs a display or careful
configuration on a headless server. So a fork is what makes it usable here.

Pipes are also what makes the licences work. **[WITTYRETROARCH.md](WITTYRETROARCH.md) explains how GPL
code and the Apache binaries stay compatible**, what the fork has to do, and what the protocol is.

Cores are the same files RetroArch uses, and a RetroArch install's cores are found and offered automatically.

### Why no cgo

A cgo frontend would need a C toolchain for every platform in the release matrix. We use
[purego](https://github.com/ebitengine/purego) (Apache-2.0) to open the core and to receive its
callbacks. It is linked **only** into `wittyplay-host`, because on Linux it changes how the
runtime starts threads and makes the binary depend on the system's libc. The server stays the
static binary it has always been.

## Why WebRTC, and what happens when it can't connect

Latency is the whole product. WebRTC has no segment buffering, it drops late frames rather than
stalling, and its receive buffer can be asked to stay close to empty. Input goes over an
unreliable, unordered data channel carrying the complete pad state in every packet, so a lost
packet is corrected by the next one.

Not every network or browser allows it, so the player steps down automatically:

| Step | Video | Audio | Input | When |
|---|---|---|---|---|
| 1. **WebRTC** | H.264, or VP8 where the browser has no H.264 | Opus | Data channel | The normal case on every current browser |
| 2. **WebSocket + WebCodecs** | H.264 decoded by `VideoDecoder`, drawn to a canvas | PCM | WebSocket | UDP is blocked, or only a reverse proxy is reachable |
| 3. **WebSocket + JPEG** | One JPEG per frame, encoded in Go, drawn to a canvas | PCM | WebSocket | No WebRTC, no WebCodecs and no H.264, or FFmpeg can't encode |

The steps share the same session, so changing transport never restarts the game. You can force a
step in the player's settings.

### Encoding

- **Encoder order:** the hardware encoders the transcoder has already test-encoded with (NVENC,
  Quick Sync, VideoToolbox, AMF, VAAPI), then `libx264`, then `libopenh264`, then `libvpx`. A
  failed start falls through to the next one.
- **Settings:** no B-frames, no lookahead, a keyframe every second so a lost packet heals quickly,
  and the fastest preset of each encoder.
- **Sharp pixels:** retro games are small and 4:2:0 chroma smears pixel art, so frames up to 400
  pixels wide are doubled with nearest-neighbour before encoding. On a server with fewer than
  four CPU threads and no hardware encoder they are not doubled. The source resolution is so low
  that the encoder has little to do either way.
- **Framing:** FFmpeg writes FLV for H.264 and IVF for VP8. Both mark exactly where a frame ends,
  so we never hold a finished frame while waiting for the next one to prove it finished, which
  raw Annex B would cost us one full frame of delay.
- **A slow encoder drops frames instead of building a queue:** the session keeps only the newest
  unencoded frame.

### What low-end servers can expect

A retro console at 2x is about 512x448 at 60 frames a second. Two sessions is the default limit.
The host runs at below-normal priority so a game never starves a film being transcoded. Audio
costs about 1% of a core.

The in-browser mode below is the answer for servers that can't keep up.

## In-browser emulation (designed, not built yet)

Libretro cores also exist compiled to WebAssembly, in RetroArch's web build. A second mode can
send the ROM to the browser and run it there, with no server CPU, no latency and offline use.

- It would be GPLv3 code, downloaded only on request into the data folder, and served as separate
  files from a sandboxed frame. Nothing would be compiled into the app's bundle.
- `auto` would pick it per device and game: when the server is overloaded, or when the session
  has poor round-trip times and the browser is fast. A manual override would sit in the player.
- Saves would upload to the same per-account store, and input would use the same layers.
- It would always be optional. It only works in a browser, so the server would list it as `browserOnly`, and the
  TV apps would never see it.

The player has been kept independent of the transport so that this can be added without a rewrite.

## Libraries and scanning

- **Library type `games`**, with a `console` on the library (`nes`, `snes`, and so on) or `auto`
  for one folder holding many consoles. Several libraries of one console work the same way as
  several Movies libraries: the add flow's "combine or separate" choice applies per console.
- **`auto` libraries** use the folder name first (`SNES/`, `Super Nintendo/`, `Super Nintendo (SNES)/`),
  then the file extension when only one console claims it. Files nothing claims are skipped.
- **Items** have kind `game` and a `console`. Multi-file games: a `.cue` hides its `.bin` tracks,
  and an `.m3u` hides the discs it lists.
- **Archives** count as the game inside them, so nothing needs unpacking first. See below.
- **What gets skipped:** BIOS folders (`bios`, `system`), and files that aren't games. A scan reports what it left
  out, on the library's page, for admins.

### Games in archives

`.7z`, `.zip`, `.rar` and `.tar` work wherever a console accepts a game file, and so does anything compressed with gzip, bzip2,
xz or Zstandard, as one file (`Game.sfc.xz`) or as a tar (`.tar.gz`, `.tgz`, `.tar.bz2`, `.tar.xz`, `.tar.zst` and their short
forms). That includes split sets (`.part1.rar` on, `.7z.001` on: all the parts stay together in one folder), and files whose
extension is wrong for what they really are. An archive inside an archive doesn't count. Code: `internal/gamearchive`.

**Scanning** reads only an archive's headers, and only when the file is new or changed: what each one holds is kept in
the database. A collection of about 2,000 archives takes about a second the first time. A compressed tar has no index,
so a scan reads through the first 64 MB of one at most, which is usually far enough to see what it is. The title comes
from the archive's file name, with its tags (`Game (USA) [!].7z` is "Game", "USA").

**Which file:** a playlist first, then a `.cue` or `.gdi`, then `.ccd` or `.toc`, then any other game file for that console.
Among several candidates, it prefers one named after the archive, then a verified dump in a widely-played region, and
puts bad dumps and hacks last. A tie goes to the first in the archive.

| What's in it | What's unpacked | What the core opens |
|---|---|---|
| One game file, or several versions | The chosen file, as `game.<ext>` | `game.<ext>` |
| A disc, with its tracks | Everything but documents and pictures, since cores also want things no `.cue` names (`.sbi`, `.sub`) | The `.cue`, `.gdi` or `.ccd` under its own name, with the tracks beside it |
| Several discs of one game, and a playlist | The same | The playlist |
| Several discs of one game, and no playlist | The same | `game.m3u`, written then. A console that can't open one gets the first disc |

A `.cue`, `.gdi` or `.m3u` from an archive is checked before it's used: each file it names must have arrived. A name that
differs only in case, or carries a path from the computer it was made on, is corrected in the unpacked copy, as those
fail on Linux. A name that matches nothing gives a clear error, which also means a list can't point a core at anything
outside the game's folder.

**The cache** is `gaming/unpacked/<id>/` under the data folder, sized in Settings, Gaming (16 GB by default). Nothing is
ever written beside the original.

- A game is reused on the next play, and the longest unplayed is evicted when room is needed, except a game being played.
- The id comes from the file's path, size and time, and the console, so a changed archive is a new game.
- A game unpacks into `.tmp-…` and appears under its final name in one rename, after every file is synced to disk and
  a manifest of their sizes is written. Anything a crash leaves behind is under the temporary name, and is cleared at
  the next start. A finished game is checked against its manifest each time it starts, and is unpacked again if it fails.
- Everyone asking for the same game shares one unpack. At most two run at a time, and the rest wait. One that nobody has
  asked about for two minutes is stopped.
- Disk space comes first: a game that won't fit, leaving 512 MB free, gets a clear error.
- Settings, Gaming shows how much it's using and can empty it. A game being played stays.

**Saves** don't depend on any of this: they belong to the account and the item (`u<id>/i<id>`), and the item is the
archive's path in the library. A one-file game is always opened as `game.<ext>`, the name `.zip` games have always had, in
case a core names files after what it opened.

**Starting** a game that isn't unpacked yet: `POST /games/{id}/prepare` starts the unpack and says how far it has got, and
the player shows that. `POST /games/{id}/play` works without it, and just takes as long as the unpack does.

### Metadata and box art

Scans only list files. Details, ratings and pictures come along behind them in the background, from sources you choose in
*Settings → Metadata agents → Video games*. Three work with no key: the [libretro database](https://github.com/libretro/libretro-database)
(CC BY-SA 4.0, downloaded when needed and never bundled), [libretro's thumbnails](https://github.com/libretro-thumbnails/libretro-thumbnails)
(no licence stated, so fetched to your server only, never bundled or republished) and [Wikidata](https://www.wikidata.org) with Wikipedia.
IGDB, ScreenScraper, RAWG and TheGamesDB take a free key each. [METADATA.md](METADATA.md#video-games) has all of it: what
each one sends, how games are matched, and how ratings reach Kids profiles.

Your own files always come first: `Game.png`, `Game-cover.jpg`, `Game-poster.jpg`, `Game-fanart.jpg` and `Game.nfo` beside a game. With no box
art, the library shows a tile made from the game's name and console. Saved states each show a picture of the game. Native apps get all
this through `GET /games/{id}/metadata`: see [CLIENT-API.md](CLIENT-API.md#game-details).

## Saves, states and resume

Everything is stored per account under the data folder:

```
gaming/saves/u<user id>/i<item id>/save.srm      battery save
                                   auto.state    written when you stop; resumes next time
                                   state-1…9.state  manual slots, each with a .png thumbnail
```

Paths are built only from numeric IDs and a slot number between 0 and 9. Clients can't upload save
files. The host sends the bytes over its pipe, and the server writes them atomically, so a host
that has been exploited can't choose a path. Battery saves are written every 30 seconds when they
have changed, and on stop.

## Clients other than a browser

Games have to work from the Apple TV and Android TV apps as well, so nothing about a game is decided
inside the web player:

- **The server holds** sessions, saves, resume, play time, each console's controller and default layouts, and
  everyone's saved layouts. They're plain JSON, and the web player reads them like any other client.
- **The protocol is standard:** WebRTC offer and answer over HTTP, data channels with documented messages, and a
  WebSocket with documented frames. A test (`TestNativeClientPlaysAGame`) plays a game this way with no browser.
- **The server says which modes a client can use.** Anything only a browser can do, such as in-browser emulation
  when it exists, is marked `browserOnly` and is never offered to an app.
- **Everything works with a D-pad.** A TV remote is a controller with its own default map, and Back opens the
  game's menu, which can press Start and Select for remotes that have neither. No drop-downs or sliders are needed
  to play.

What's left for a TV app to write is listed at the end of [GAMING-API.md](GAMING-API.md).

## Controllers

Everything becomes one thing: a **RetroPad state** per player, which is 16 buttons and two sticks.
The libretro API speaks RetroPad, so each console needs only labels. These, and all the default layouts, are data
from the server (`modules/wittyplay/controls.go`).

- **Gamepads:** standard mapping by position (bottom button is RetroPad B, right is A, and so on),
  which is how libretro's cores expect pads to be laid out.
- **Pads without standard mapping:** detected, and the remap screen opens for them.
- **Keyboard:** arrows, Z, X, A, S, Q, W, Enter and Shift by default. Remappable.
- **TV remote:** the D-pad, OK as A, Play/Pause as B, Back for the menu.
- **Hot-plug:** connecting a pad assigns it to the next free player. Disconnecting one leaves that
  player's slot open. Browsers that don't fire the events are polled.
- **Several players:** up to four in the UI, with eight supported by the protocol.
- **Touch:** an on-screen controller per console. The editor moves, resizes and sets the opacity of
  each control and chooses which ones show, saved **per console and per device, for portrait and
  landscape separately**. Sliding between buttons works, and Android gets haptics (iOS has no web
  API for them).

Layouts are saved on the server: gamepads per console and model, keyboards per console, and touch layouts per
console, device and orientation. They follow the account.

## Security

Treat a ROM as hostile and a core as buggy:

**The host process**

- Starts with an empty environment, in a working folder of its own, with only pipes open.
- Only the core the server picked runs: the request names a game, never a file. Core files come
  from the server's own list.
- **Linux:** new network, IPC and user namespaces where allowed; `no_new_privs`; a seccomp filter
  refusing sockets, `execve` and `ptrace`; Landlock limiting writes to its save folder; memory and
  file-size rlimits; lower priority; killed when the server dies.
- **Windows:** a job object that kills it with the server, limits memory and forbids child
  processes, plus a low-integrity token so it can only write to its save folder; below-normal
  priority.
- **macOS:** a `sandbox-exec` profile denying network and writes elsewhere, plus rlimits.
- **FreeBSD and others:** rlimits and priority only. Docs say so plainly.
- A session is stopped after 60 seconds with no viewer and after 30 idle minutes. The number of
  sessions is capped.

**Archives**

- An archive is hostile input, and its decoders are other people's code. Everything that reads one runs in a throwaway
  worker: the server started again with an internal flag. It boxes itself in with the same layers as the host, and has a
  memory limit of 4 GB and a lower priority. On Windows it uses a job object for the limit. A crafted header that makes a
  decoder panic or ask for all the memory in the world ends the worker, and the server reports that one archive as damaged.
  A scan's worker that stops answering for 60 seconds is killed.
- A name that climbs out of the folder, is absolute, or names a drive or a device refuses the whole archive. Writing also
  goes through `os.Root`, so a name the checks miss still can't leave the folder.
- Links, devices and folders in an archive are ignored. No link is ever created.
- At most 10,000 entries and 32 GB, with at most 2,000 times the archive's own size above 1 GB. Those are checked against
  the headers first. Then the bytes are counted as they're written, since headers can lie: writing more than a file's
  headers promised stops it. Checksums are checked where the format has them.
- Encrypted archives get a clear message, and no password is ever tried. A `.7z` that keeps its names readable and locks
  only its contents can't be told from its headers, so it shows in the library and says so when played. The other kinds are
  reported by the scan: password-protected, damaged, oversized, unsafe.

**The server's side of the pipe**

- Everything the host says is untrusted: every message has a size limit, dimensions are checked,
  and frames go to FFmpeg as raw bytes only.

**The API**

- Every endpoint checks sign-in. Starting a game goes through the same per-account access rules as
  any other item, and a session belongs to the account that started it.
- WebSockets check the Origin, as the rest of the API's CSRF rules do.
- Only admins can install cores. Downloads go to one fixed host over HTTPS, the core name comes
  from our own list, the size is capped, only the single expected file is unpacked, and it is
  written atomically with its SHA-256 recorded.
- Input packets have a fixed size, and players outside the session's limit are ignored.
- SDP offers are capped in size.

Downloading a core means running someone else's native code on the server. That's why only an
admin can do it, why each core's licence and source are shown first, and why the host is
sandboxed.

## Licences

| Thing | Licence | How it's kept apart |
|---|---|---|
| WittyPlay (host, stream, web) | Apache-2.0 | Ours |
| pion/webrtc and its family | MIT | Linked into `wms` |
| coder/websocket | ISC | Linked into `wms` |
| bodgit/sevenzip, nwaples/rardecode and their family | BSD, MIT, Apache-2.0, MPL-2.0: see THIRD_PARTY_LICENSES.md | Linked into `wms`. No GPL code |
| ebitengine/purego | Apache-2.0 | Linked into `wittyplay-host` only |
| libretro API definitions | MIT | The constants and struct layouts only; no libretro code is copied |
| RetroArch | GPLv3 | Not shipped. Its core folder is read if present. The fork would be a separate program ([WITTYRETROARCH.md](WITTYRETROARCH.md)) |
| Libretro cores | Each core's own | **Never bundled.** A separate file, installed on an admin's request, loaded by a separate process |
| FFmpeg | GPL builds | Already a separate program |

**Non-commercial cores** (`snes9x`, `genesis_plus_gx`, `picodrive`, `fbneo`, `mame2003*`) are not
offered for download. If an admin places one in the cores folder, it's listed with its licence
shown, as that's their own copy and their own decision.

**ROMs and BIOS files are never bundled, downloaded or linked.** The console page lists the BIOS
file names a core expects, so you can supply your own.

### Which cores are offered

Licences are from the [libretro core-info](https://github.com/libretro/libretro-core-info) files
(MIT), checked on 2026-10-30.

| Console | Cores offered, in order | Licence |
|---|---|---|
| NES / Famicom | fceumm, nestopia, mesen | GPLv2, GPLv2, GPLv3 |
| SNES / Super Famicom | mednafen_supafaust, bsnes2014_performance, mesen-s | GPLv2+, GPLv3, GPLv3 |
| Game Boy / Color | gambatte, sameboy, mgba | GPLv2, MIT, MPL-2.0 |
| Game Boy Advance | mgba, vba_next | MPL-2.0, GPLv2 |
| Mega Drive / Genesis | blastem, clownmdemu | GPLv3, AGPLv3 |
| Master System, Game Gear | gearsystem, smsplus | GPLv3, GPLv2 |
| PlayStation | pcsx_rearmed, mednafen_psx | GPLv2 |
| PC Engine / TurboGrafx-16 | mednafen_pce_fast | GPLv2 |
| Atari 2600, 7800, Lynx | stella2014, prosystem, handy | GPLv2, GPLv2, zlib |
| WonderSwan, Neo Geo Pocket, Virtual Boy | mednafen_wswan, mednafen_ngp, mednafen_vb | GPLv2 |

**Listed but not playable yet** because they need hardware rendering: Nintendo 64, PSP, Dreamcast.
Nintendo DS needs a pointer input for its touch screen, which comes after the first release.

## Limits today

- **Deflate64 and LZMA inside a `.zip`** aren't read, and say so. Deflate, bzip2 and Zstandard are. Nearly all zips are Deflate.
- **A `.7z` with locked contents** is only found out when played. See Security.
- **Software-rendered cores only** with the built-in runner.
- **Real cores:** the tests use the built-in test core. Loading and running real libretro cores has
  not been tested end to end yet.
- **No bandwidth adaptation yet:** the bitrate is a setting (Auto, Data saver, High), not a live
  response to the network.
- **No netplay yet.** Several players means several controllers on one device.
- **Plain http on a home network:** browsers limit some features to secure pages. Firefox hides
  gamepads, and WebCodecs is unavailable, which leaves WebRTC or JPEG. Reaching the server over
  https or `localhost` removes both limits.
- **Containers:** WebRTC needs UDP. Publish a fixed port set in Settings, Gaming, and fill in the public
  address, or use host networking.

## Phases

1. This document.
2. The library and scanning.
3. Launching a core and streaming one game end to end.
4. Controllers and the touch editor.
5. Polish.
