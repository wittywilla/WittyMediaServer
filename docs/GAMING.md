# Gaming: design

Games sit in libraries next to films and music. You open one in a browser, on any device, and
play it straight away with whatever controller you have. Saves follow your account. Games run
on the server and stream to the browser with low latency, through **WittyPlay**
(`modules/wittyplay`).

This page explains how it is built and why. It is the reference for anyone changing it.
[GAMING-API.md](GAMING-API.md) is the API a client uses, including the Apple TV and Android TV apps.

Everything here is about games the server emulates. Playing on any PC running Sunshine, through a Moonlight client
in the page, is a different thing that lives alongside it: see [GAME-RIGS.md](GAME-RIGS.md). A shared PC's games are
items with `console: rig`, and `POST /games/{id}/play` sends them there.

## What it has to do

- A Gaming library type, organised by console, with several libraries of one console allowed.
- Games run through libretro cores and stream to the browser.
- Every controller: gamepads in any button order, keyboard, on-screen touch controls, remapping by
  pressing, named profiles, hot-plug, several players.
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
   exploited core takes down one game, not the server. Cores print to stdout too, so the runner
   moves the pipe to a handle of its own before it loads one.
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
| Hardware-rendered cores (N64, PS2, PSP, Dreamcast, 3DS) | Yes, with OpenGL | Yes |
| Server with no GPU | Yes, for software-rendered cores | For software-rendered cores |
| Size and upkeep | A few thousand lines of Go, cross-compiled from one machine | A large C codebase to build per platform and merge from upstream |

The server uses WittyRetroArch when it finds one, and the built-in runner otherwise. An admin can choose
in Settings, Gaming.

Unmodified RetroArch can't fill this role: its only way to hand frames to another program is its
recording feature, which muxes and adds several frames of delay, and it needs a display or careful
configuration on a headless server. So a fork is what makes it usable here. The built-in runner now does
what the fork was going to be for, so the fork is no longer needed for any console.

Pipes are also what makes the licences work. **[WITTYRETROARCH.md](WITTYRETROARCH.md) explains how GPL
code and the Apache binaries stay compatible**, what the fork has to do, and what the protocol is.

Cores are the same files RetroArch uses, and a RetroArch install's cores are found and offered automatically.

### Why no cgo

A cgo frontend would need a C toolchain for every platform in the release matrix. We use
[purego](https://github.com/ebitengine/purego) (Apache-2.0) to open the core and to receive its
callbacks. It is linked **only** into `wittyplay-host`, because on Linux it changes how the
runtime starts threads and makes the binary depend on the system's libc. The server stays the
static binary it has always been.

## Consoles that draw with a graphics card

Nintendo 64, PlayStation 2, PSP, Dreamcast, 3DS and the faster PlayStation cores draw with OpenGL, so the
server's graphics card renders the game and the result is streamed like any other frame.

**The choice: OpenGL in the built-in runner, using the system's own libraries through purego.** It needs no
C compiler, no second program, no GPL code, and stays one small binary per platform. Every core we offer
for these consoles can use OpenGL. A runner built on RetroArch would give nothing more for any of them and
would cost a C codebase to build for every platform. Vulkan isn't built: no core we offer needs it, and the
one thing it would add, choosing a card directly on a two-GPU machine, is already solved for NVIDIA below.
Cores are told OpenGL is what's preferred, and those that can also draw with Vulkan or Direct3D use it.

```
 core ── draws into ──► framebuffer we made ── glReadPixels ──► pixel buffer ── fence ──► frame
                        (colour + depth/stencil)                (no stall)                to the server
```

- **The context** is a hidden window with WGL on Windows, which nothing ever shows, and EGL with no display on
  Linux. On Linux every card is an EGL device, so the runner takes the first that isn't drawn by the CPU,
  preferring anything to integrated Intel. Under WSL it goes through `/dev/dxg` and Mesa's D3D12 driver.
- **Reading frames back** doesn't stall the game. The copy is queued behind the frame's own drawing, and the
  runner only waits for that drawing to finish, after the core has returned. OpenGL's rows are bottom-up, so
  they are flipped once, on the copy that goes down the pipe.
- **Encoding:** a game on the graphics card uses whichever hardware encoder works here, whatever the
  transcoder's setting says, because its frames are large and its card is awake. NVENC takes the frame as it
  is and converts its colours on the card, and it's told not to keep a queue of finished frames (`-delay 0`).
  Only a frame that needs doubling goes through FFmpeg's scaler.
- **Internal resolution** is a setting in Settings, Gaming: Automatic, or 1x to 4x. Automatic is 2x on
  Nintendo 64, PlayStation, PlayStation 2, PSP and Dreamcast, and native on 3DS. The per-core options are in
  `modules/wittyplay/traits.go`. A choice a core doesn't offer is ignored, so an updated core can't break.
- **How many at once:** two by default, set in Settings, Gaming. Past that, starting another says the card is
  full. Software-rendered games don't count.
- **No usable card:** the game says so plainly and older consoles carry on. OpenGL drawn by the CPU
  (`llvmpipe`, Microsoft's GDI renderer) doesn't count: it works, far too slowly to play. Settings, Gaming names
  the card in use, or says why there is none.

### What it costs

Measured on one laptop with an RTX 3070 and an 8-core CPU, through the whole path (sandboxed runner, NVENC, WebRTC to
a browser) while other work was keeping the CPU busy, so these are on the cautious side:

| Game | Picture | Frames a second | Encoder delay |
|---|---|---|---|
| Nintendo 64, Mupen64Plus-Next | 1280x960 | 50 to 60 | 28 to 36 ms |
| PlayStation 2, LRPS2 | 1280x896 | 50 | 31 ms |
| PSP, PPSSPP | 960x540 | 45 to 60 | 26 ms |
| Nintendo DS, melonDS, drawn in software | 256x384, doubled | 45 to 53 | 28 ms |

The encoder always kept up: the log's `gameFps` and `fps` matched in every run, so anything under 60 was the game itself
competing for the CPU. The card itself had room, at about a quarter of its capacity and under half of that for encoding.

On its own, NVENC's part is 22 ms at 1280x960, of which FFmpeg's command line holds one frame (17 ms at 60 frames a
second). It was 53 ms before `-delay 0`.

Five minutes each of Nintendo 64 and PlayStation 2, running at once, gave no crash and no fault.

**What PPSSPP does by itself:** the first time a PSP game starts for an account, the core unpacks the PSP firmware from
the updater on that game's own disc into the save folder, about 20 MB, decrypting it as a PSP would. That is part of the
core, it has no option to turn it off, and nothing in this server asks for it or sees the results.

### Two graphics cards

On a laptop with Intel and NVIDIA, Windows hands OpenGL to the card driving the display, usually the slow one.
The runner sets `SHIM_MCCOMPAT=0x800000001` in its own process before OpenGL loads, which NVIDIA's driver reads
as "use the discrete card". No system setting changes and nothing needs clicking. Settings, Gaming shows
which card was chosen.

That variable is NVIDIA's. On a laptop with an AMD discrete card and integrated graphics, Windows may pick the
integrated one. To fix that, open Windows Settings, System, Display, Graphics, add `wittyplay-host.exe`,
choose Options, then High performance.

### What the sandbox opens for a graphics driver

Only games on a core marked as drawing with a graphics card get these (the `-gpu` flag), and a software-rendered
game's sandbox is as it was. A core the table doesn't know gets them too, since nothing says it doesn't need them.

| Platform | What changes | Why |
|---|---|---|
| Linux | Landlock also allows writing to `/dev/dri`, `/dev/dxg`, `/dev/kfd` and `/dev/nvidia*`, and nothing else | Opening these read-write is how every driver talks to its card |
| Linux | The address-space limit is dropped; the limit on memory actually written to stays, at 8 GB | Drivers map far more addresses than a machine has memory |
| Windows | The job's memory limit is 8 GB, not 2 GB | Graphics drivers and the larger emulators commit a lot |

Everything else stays: Linux's namespaces, `no_new_privs` and the seccomp filter, and Windows's job object, which
kills the runner with the server and forbids child processes, and its low-integrity token. The low-integrity
token needed no change: drivers are read from where they're installed, and graphics devices aren't blocked by it.

What a sandbox can't cover: a graphics driver is a large body of code running inside the process, talking to
the kernel through interfaces built for speed. A core taken over by a bad ROM can reach the same driver and kernel
surface a game can. That's unavoidable for hardware rendering and is why these games have the limit above and
stay opt-in by core. Treat a server with a GPU as a machine where a hostile ROM has more to work with.

**Running as a Windows service:** session 0 has no desktop, and consumer OpenGL drivers often refuse to draw there.
Settings, Gaming will say there's no usable card. Run the server in a signed-in session to play these consoles.

**Containers:** pass the card in, with `--device /dev/dri` for Intel and AMD, or `--gpus all` for NVIDIA.

### Cores that keep their own stacks

Several cores run the game on stacks they allocate themselves, and call back from there. The Go runtime refuses a
callback on a foreign stack on a thread it created, so on Linux the runner hosts the core on a thread it
didn't: `libretro.OnCoreThread`.

## Touch screens

Nintendo DS and 3DS games are played on a touch screen. A press on the picture is the stylus, with a mouse or a
finger, and sends a position as fractions of the picture (`docs/GAMING-API.md`). The cores get it as libretro's
pointer device, and work out which screen it fell on. With on-screen controls showing, a finger on a button
presses the button and a finger anywhere else is the stylus.

DS cores use their software renderers, so they play on a server with no graphics card. 3DS cores need one. A real DS
game that's played by touch has been played this way, from its title screen on.

**3DS games must be decrypted.** These cores can't read encrypted game files, and nothing here will decrypt them or
look for keys. The server reads the plain headers of `.3ds`, `.cci`, `.cxi`, `.app` and `.cia` files when you press
Play, and an encrypted one gets a clear refusal rather than a core's failure. A `.cia` is an install package, which
the cores can't open: an unencrypted one has its main program copied out to a temporary file for the session. Updates
and extra content inside a package aren't applied.

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
  and the fastest preset of each encoder. `libx264` uses sliced threads, because frame threads hold back
  a frame per thread: about 250 ms on a machine with many cores.
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
The host runs at below-normal priority so a game never starves a film being transcoded, except
for games on a graphics card and the Nintendo DS, which run at normal priority. At below-normal,
a real PSP game got a sixteenth of a core and 2 frames a second on a machine only 77% busy, which
isn't a game. Audio costs about 1% of a core.
On Linux and macOS the host runs at a slightly lower priority, so a film being transcoded comes
first without the game being starved. Windows has no such gentle setting: below-normal gets next
to nothing while other programs are busy (one game measured 1 frame a second, against 61 at normal,
with the machine under half busy). So on Windows it runs at normal priority, sharing fairly, and a
software-rendered core uses about one CPU core. Audio costs about 1% of a core.

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

A state that arrives before the first frame waits for two frames that nobody sees. Some cores accept one
earlier and then start the game from the beginning anyway.

Everything is stored per account under the data folder:

```
gaming/saves/u<user id>/i<item id>/save.srm       battery save
                                   auto.state     written when you stop; resumes next time
                                   state-1…99.state  manual slots, each with a .png picture and a .json
                                   undo.state     the game as it was before the last load
                                   autoprev.state the automatic save that starting over would have replaced
                                   patches/p<n>.patch  a person's patches
```

Paths are built only from numeric IDs and a slot number. The host sends the bytes over its pipe, and the server
writes them atomically, so a host that has been exploited can't choose a path. Battery saves are written every 30
seconds when they have changed, and on stop. One game's folder can hold 4 GB for one account, and a save that would
go over is refused with a message, rather than failing later.

A person can download a state or a battery save and put one back. An exported state has a small header with the
console, the core, its size and a checksum, and an import checks all of them. A plain state file from elsewhere is
taken with a warning: nothing can vouch for it, and it's only ever read by a core inside its sandbox. A state that
won't load leaves the game as it was and says so, and Undo load is always there.

## Settings

Settings are described once, as data, in `modules/wittyplay/settings.go`: a key, a kind, a range, a default, a label
and any caution. The server checks values against that, and every client draws its controls from it
(`GET /api/games/settings/schema`). A new setting is a line in that table, plus whatever uses it.

### Levels

Values live in layers. The most specific one that has a value wins, one key at a time:

| Level | Who can set it | Stored in |
|---|---|---|
| Built-in default | Nobody | `settings.go` |
| Everyone on this server | An administrator | The `settings` table, `gamingDefaults` |
| Everyone, one console | An administrator | The `settings` table, `gamingDefaults.<console>` |
| All of a person's games | That person | `wittyplay_settings` |
| A person's games on one console | That person | `wittyplay_settings` |
| One game, for one person | That person | `wittyplay_settings` |

Administrators also set **limits**, which aren't defaults, because nobody can go past them: the fastest fast-forward
(`gamingMaxSpeed`), rewind's memory per game (`gamingRewindMB`) and whether run-ahead is allowed (`gamingRunAhead`).
Left alone they follow the machine: 4× and 32 MB, or 2× and 16 MB on one with fewer than four CPU threads.

Stored values are checked again when they're used, so a value that's stopped being valid is ignored and a game
still starts. Reading settings never fails a start: anything unreadable counts as nothing set.

### Where each thing runs

| What | Where | Cost to a server |
|---|---|---|
| Picture effects, shape, crop, colour, bezels | The browser | None |
| On-screen numbers | The browser, asking `GET /play/{id}/stats` once a second while on | A few counters |
| Volume, mute, sound buffer | The browser | None |
| Fast-forward, slow motion, frame skip, frame advance | The runner | Fast-forward runs the core faster, and sends no more pictures than normal |
| Rewind, run-ahead | The runner | Real, so off by default and limited |
| Core options, cheats | The core, through its own interfaces | None |
| Patches | The server, once, when a game starts | A file read and write |

### The picture

`web/display.ts` and `web/layout.ts`. With no effect on, the transport's video element is on the page by itself, which
is as fast as a browser can show anything. Colour is a CSS filter. Turning on an effect that needs a shader puts a
canvas over the video and draws each frame on it the moment it's decoded, using `requestVideoFrameCallback`, so
nothing waits for a timer. The shader is our own, about 60 lines. Its only uniforms are numbers from the settings.

- **Without WebGL**, or when a context is lost, CSS lays scanlines and the grid over the picture and does colour.
  The context coming back restores the shader. The player never stops for this.
- **Flicker** is the one effect that changes on its own. It's capped at a small part of the brightness, is off by
  default, carries a caution, and is forced off under `prefers-reduced-motion`. Fast-forward carries a caution too.
- **A person's image** is decoded in Go, must be PNG, JPEG or WebP and within 16 MP and 16 MB, and only the decoded
  pixels are kept, as a PNG this server wrote. It's served as `image/png` with `nosniff` and a sandboxing CSP, to its
  owner only. An SVG, or anything hiding after the image's end, can't get through.

### How a game's speed changes

`host/control.go` stands between the runner's loop and the core.

- **Fast-forward** runs the core faster and sends at most the game's own frame rate. The core is told it needn't draw
  or mix (libretro's `GET_AUDIO_VIDEO_ENABLE`), and many cores then run a lot faster. It has no sound.
- **Slow motion** has no sound either: stretched sound would need resampling for a feature that's used for moments.
- **Frame skip** leaves frames out of what's sent, on request or, when set to automatic, only once the loop is more
  than a frame behind, and never more than three in a row.
- **Rewind** (`host/rewind.go`) keeps the newest state whole, and each older one as its difference from the one after.
  The oldest can then be dropped at any moment, which keeps it inside a byte limit however busy the game is. If taking
  a state costs over a quarter of a frame it takes them less often, and if it still doesn't fit it turns off and says
  so. Loading a state or resetting starts its history again.
- **Run-ahead** runs the real frame with its sound, saves, runs on with only the last picture kept, and goes back. That
  takes frames off the delay at the cost of a save, a load and extra frames each time. On FCEUmm that's about a millisecond a
  frame. If the game falls behind its schedule for a couple of seconds, it turns off, with a message.

### Core options

A runner tells the server what a core offers as it starts: labels, allowed values and their names, descriptions, categories, and
which options the core currently wants hidden (a core can hide options that don't apply, and show them again when another changes). The
newest libretro option format is read, and the older two for cores that only have those. The server keeps that
per core, so settings screens can show it with no game running. For a core never run here, it starts one with no game
for a moment to ask, boxed in as any game is. The runner refuses a key the core didn't offer and a value it didn't
list, so nothing written in a setting can reach anything else, whatever the server let through.

### Custom settings files

`custom.go`. A settings file is one level of settings as text:

```json
{
  "wittyplay": 1,
  "settings": { "display.scanlines": 0.4 },
  "coreOptions": { "fceumm_sndquality": "High" },
  "controls": [
    { "label": "Sound quality", "type": "choice", "option": "fceumm_sndquality", "choices": ["Low", "High", "Very High"] },
    { "label": "No sprite limit", "type": "toggle", "option": "fceumm_nospritelimit" },
    { "label": "Overclock", "type": "slider", "option": "fceumm_overclocking", "values": ["disabled", "2x-Postrender", "2x-VBlank"] }
  ],
  "presets": [
    { "name": "Crisp", "settings": { "display.scaling": "pixel" }, "coreOptions": { "fceumm_sndquality": "Very High" } }
  ]
}
```

- **It's data.** Nothing in it is run. It can hold settings and core options, and a control can only set a core option.
  Values go through the same checks as everything else, and a core only takes what it listed.
- **It's hostile until proven otherwise.** 256 KB at most, 8 levels deep, 20,000 values, 64 controls, 32 presets. Sizes
  are checked before anything is set aside, and the parser walks tokens so that it can stop part-way. It's fuzz-tested.
- **Mistakes are shown where they are.** Errors stop it saving, with the line. Things it doesn't know, and core options a
  core won't take, are **ignored and listed**, never dropped silently.
- A controller can use a file's presets and controls. Writing one takes a keyboard.

### If a setting breaks a game

- A change to the picture only ever affects the browser, and one a browser can't do falls back.
- A running game is told about changes with a command it can refuse. The game isn't restarted.
- **Safe mode** (`POST /games/{id}/play` with `"safeMode": true`) starts with default settings, no cheats and no patches,
  and takes no setting changes until it's stopped. It's offered on the game's page, in the game menu, and whenever a
  game fails to start.
- A patch that won't apply, such as one for a different version of the game, leaves the game unpatched and playable,
  with a notice. It never stops the game starting.
- Resetting a level is one button in the settings file screen.

### Shortcuts

`web/hotkeys.ts` works on a player's RetroPad buttons after the input layer has mapped them, so shortcuts follow a
remapped pad. Holding the shortcut button (Select unless changed) and pressing another does the shortcut and keeps that
button from the game until it's let go. Start and Select together, or Guide, always opens the menu. Hold actions let go
by themselves if the window loses focus or a menu opens, because a stuck fast-forward would be worse than a lost press.
Speed changes go by reliable HTTP, not the input channel, for the same reason. A keyboard shortcut gives way to a key a
game button is using.

## Adding your own core

A **core** is the program that emulates a console. This server offers a list of them with their licences checked, under
Settings, Gaming, Consoles and cores. If you want one that isn't on the list, such as a newer build, or a core for a
system we don't have, you can add it yourself. This is the guide for doing that. It assumes you've never done it before.

### Before you start

**A core is a program, and it will run on your server** whenever someone plays with it. It always runs boxed in, in a
process of its own, never inside the server itself. A box can have holes, though, so only add cores you trust.

You'll need:

- **The core file**, built for the computer your server runs on. On Windows it ends in `.dll`, on Linux in `.so`, on a Mac in
  `.dylib`, and its name usually ends in `_libretro`, like `something_libretro.dll`. It may come in a `.zip`.
- **Your password.** You'll be asked for it each time.
- **Where to get a core is your call.** This project doesn't suggest sources. Get it from the people who make it. WittyMediaServer
  has not checked anything you add here, and doesn't offer or support it.

### 1. Turn it on

Custom cores are **off on every server until an administrator turns them on**. Go to Settings, Gaming, Custom cores, and
turn on **Allow custom cores**. It asks for your password.

Why off by default: most people never need this, and a server shared with others shouldn't be one setting away from running
new programs. Turning it on takes a moment, once. Turning it off needs no password: the cores stay where they are, don't run,
and can still be removed.

If someone else runs the machine, they can keep it off whatever the settings say by starting the server with
`WMS_GAMING_NO_CUSTOM_CORES=1`. On a system that can't box a core in (anything but Windows, Linux or macOS), it can't be turned on
at all.

Kids and guest profiles, and anyone who isn't an administrator, never see any of this.

### 2. Add the core

Choose **Add a core**, then one of three ways:

| Way | What to do |
|---|---|
| **Upload a file** | Pick the core, or a `.zip` holding it, from your computer |
| **Web address** | Paste an `https://` address. It has to be on the public internet: this server won't fetch from itself or your own network |
| **In the cores folder** | Put the file in the cores folder (the page shows where) first, then pick it from the list |

If the place you got it from publishes a SHA-256, paste it in, and the file is refused if it doesn't match.

Enter your password and choose **Test this core**. The server test-loads it in the sandbox. It takes a few seconds, or longer if
a virus scanner wants a look first. A core gets two minutes before it counts as hung.

If it fails, it says why, in plain words, and nothing is kept:

| It says | What that means |
|---|---|
| Built for something else | You have the Windows build and a Linux server, say. Get the one for your server |
| Isn't a program library | It's a text file, a web page saved by mistake, or an archive that needs unpacking |
| Isn't a libretro core | It's a real library, but a different kind of program. It needs to be a libretro core |
| Needs another library | It depends on something this server doesn't have. Check what its makers say it needs |
| Crashed, or didn't finish loading | The core is broken, or doesn't suit this server. Only the test stopped; your server is fine |

### 3. Look at what it reports

If it passes, the page shows what the core says about itself: its name, version and the files it opens, the computer it's built
for, whether it wants BIOS files or a graphics card, its size and its SHA-256. All of that comes from the file, so read it as what
the core claims. Passing shows it's a libretro core, not that it's safe or that your games will run well on it.

It also shows its **licence**, and where that came from: its info file, our list going by its name, or nothing at all, when it says
**Unknown license**. Non-commercial cores are labelled, and are never offered by us. You can correct it if you know better.

### 4. Say which consoles it's for

Tick the consoles, which are sorted so that the ones its file types fit come first. Then:

- **Use by default** makes it what that console plays with. Without it, the core is available but only used if nothing from our list is
  installed for that console.
- Other cores stay available. Change a console's core on its card in Settings, Gaming.
- **For one game:** on the game's own page, administrators see a Core setting. That page also shows how each core has gone on that game,
  and marks the one that has gone best, once there's been enough real play to say.

Confirm that you understand it runs as code on the server, and choose **Add this core**.

### A core for a system we don't have

If no console fits, make a **custom console**, either from the link in that same step or under Custom consoles. It needs a name, a short
name and its file extensions. You can also add other folder names that mean it, BIOS file names, and which controller it should look
like. Prefer to paste? The **JSON** tab takes the same thing:

```json
{
  "name": "Fantasy Box 3000",
  "short": "FBox",
  "extensions": [".fbx", ".fb3"],
  "aliases": ["fantasy box"],
  "bios": [{"file": "fbox.rom", "for": "the start-up screen", "required": false}],
  "padLike": "snes"
}
```

It then appears when adding a library, like any other console. Games can sit in a folder named after it, in a library set to it, or in
archives, as usual. An extension another console already uses, like `.iso`, is only trusted in a library set to this console or a folder
named after it, so nothing gets taken from a console that already works.

### Cores that need more than one file

Some cores come with a small **info file** (`something_libretro.info`), and some need **extra files** to work. To add those with the core, make
a `.zip` laid out like this:

```
something_libretro.dll      the core itself, at the top
something_libretro.info     its info file, optional
assets/...                  files only this core uses
system/...                  files that belong in the BIOS folder
```

Anything else in the zip is ignored. Or add them later, with **Add files** on the core's card. A file placed in the cores folder brings an info
file of the same name with it. Files for the BIOS folder never overwrite one already there, because that one is yours. Removing a core takes back
only the files it added.

### Replacing, going back and removing

- **Replace** takes a new file the same way as adding one, and keeps its consoles and settings.
- **Going back:** the previous versions are kept, and a core's card lists them under What it reports. Each is tested again first. A core never updates
  by itself: what runs is exactly the file you approved, until you approve another.
- **Remove** takes the core. **Saves are never touched**, by any of this: they belong to your account and the game. A battery save usually
  works in any version of a core. A save state is only sure to work in the version that made it.

### If a core's file changes

If a core's file stops matching the SHA-256 you approved, it **won't run**. Players are told the core has changed, and its card shows both
hashes and an entry in its history. Unless you changed that file yourself, take it seriously: something else wrote to your data folder. If it was you,
**Test and approve again**. Otherwise, **Remove** it and add a copy you trust.

### Safety: what this does and doesn't protect

| Guard | How |
|---|---|
| Who | Administrators only, with an adult account. Adding, approving again and going back all ask for the password. Kids and guests can't reach any of it |
| Off by default | One server-wide setting, which the general settings route can't change, plus `WMS_GAMING_NO_CUSTOM_CORES=1` for whoever runs the machine |
| Where it runs | Always in a runner process with the same sandbox as any game, listed under [Security](#security). The server never loads one. Custom cores get no more rights than the cores we offer |
| Tested first | The first thing a file ever does is load in that sandbox, where a crash is an exit code and a hang is a timeout. Its answers are treated as hostile and cleaned before they're shown |
| The file | SHA-256 recorded when approved, and read in full before every game. It's the file as approved or it doesn't run |
| Sizes | 1.5 GB for an upload or download, 1 GB for a core, 4 GB unpacked, 20,000 files |
| Paths | Nothing a person or a file sends becomes a path to write to. File names that name folders are refused. A zip with any entry that climbs out, or any link, is refused whole. A placed file is chosen from the server's own list, and links aren't followed |
| Web addresses | `https` only, and only the public internet: checked when connecting, so a name can't resolve its way onto your network, and again at every redirect. No proxy, no passwords in addresses |
| Record | Every addition, change, removal, refused file and failure during play is in the history on that page and in the server's log |

What it can't do: the sandbox limits what a core can do to your machine, and it's as strong as your system allows. On Linux, some layers need a recent
kernel, and the report says if one wasn't available. It reads files like any program under your server's account, so a hostile core can read what that account
can read, and it can't send anything anywhere because it has no network. The SHA-256 check notices a changed file. It doesn't stop someone who can already write to your
data folder, and it says nothing about whether the file you approved was good to begin with.

**Copy protection stays in force.** Nothing here decrypts or gets around anything. The server's own refusals, such as password-protected archives and encrypted discs, are unchanged, and a core is handed the game file it was asked to open and no keys. What a third-party core does with a file is down to its authors, and to you for choosing it.

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

Everything becomes one thing: a **RetroPad state** per player, which is 16 buttons, two sticks and how far
two triggers are held. The libretro API speaks RetroPad, so each console needs only labels. These, and all the
default layouts, are data from the server (`modules/wittyplay/controls.go`).

- **Gamepads:** standard mapping by position (bottom button is RetroPad B, right is A, and so on),
  which is how libretro's cores expect pads to be laid out.
- **Pads without standard mapping:** a browser that can't put a pad in order reports raw numbers. Our own table
  of common models by vendor and product (`modules/wittyplay/layouts.go`) puts them in order. A D-pad reported as one axis, or as two,
  is found by looking at it, and a trigger reported as an axis that rests at one end counts from there. An unknown pad gets a
  first-to-last guess, and the screen says to check it.
- **Sticks and triggers:** the dead zone is a circle and the travel is rescaled from its edge, so sticks reach full speed and small movements
  still count. Triggers reach the core as how far they're held, for cores that ask. A value near the press point doesn't flicker.
- **Keyboard:** arrows, Z, X, A, S, Q, W, Enter and Shift by default. Consoles with sticks add I, J, K and L. Remappable.
- **TV remote:** the D-pad, OK as A, Play/Pause as B, Back for the menu.
- **Hot-plug:** connecting a pad gives it the player it had last time on this device, or the next free one. Disconnecting one lets go
  of everything it held at once. Browsers that don't fire the events are found by the heartbeat.
- **Several players:** up to four in the UI, with eight supported by the protocol. Moving a pad onto a player who's taken swaps them.
- **Turbo and rumble:** turbo per button, per profile. Rumble reaches the right player's pad with each of its two motors.
  The runner only passes on a motor's change. The player asks for a second at a time and renews it while it's wanted, so a page that goes away can't leave a pad shaking.
- **Device types:** a core's `SET_CONTROLLER_INFO` list is read, and each console's preferred one is plugged in: a DualShock on PlayStation
  where the core offers it, which is what gives games their sticks and rumble. Players can choose another. Toggling a DualShock between digital
  and analog is the core's own business, by its own button combination.

### How it stays cheap

Pads are read in step with the screen, because browsers update them no more often than that, and sending happens only when a state changes.
Reading allocates nothing, and a pad whose timestamp hasn't moved isn't read again. With no pad connected, or the page hidden, no frames are
asked for at all, and all that's left is one heartbeat every quarter of a second.

### When someone looks away

A hidden page lets go of every button and stops rumble, because browsers stop updating pads for one and anything held would stay held. Losing
the window's focus lets go of keys, which may be released elsewhere, but pads carry on, so a game on a second screen still works.

### What browsers don't allow

Most browsers only show gamepads to secure pages, and only once a button on them has been pressed. On plain `http://`, with no pad to be seen,
the controller screen says so and what to do: open the server over https, or at `localhost` on the same computer. Rumble needs a browser
that has the feature (Chromium-based ones, and recent Safari) and a pad that can. A TV app reads controllers itself and has none of these limits.

### Profiles

People make their own named mappings, stored per account on the server (`docs/GAMING-API.md`, "Controller profiles"). Each can be limited to a controller
model, a console and a game, and the most exact one in use wins: game, then console, then controller, then the default. Files of them can be exported and
imported; an import is untrusted input, checked and limited like anything from a client.

Profile numbers are W3C standard numbers wherever the pad can be put in order, so one follows you between browsers. A pad that can't be stores its own
numbers, marked `raw`, and such a profile applies only to that pad in that state. This is also why any-controller profiles can't apply to it.

### The controller screen

One screen, in the game menu and at `/controllers`, with three tabs: Buttons, Players and Background. Touch controls open from it.

- **The picture** (`web/art.tsx`) is our own drawing as inline SVG: a flat two-button pad, a four-button pad with shoulders, a six-button pad, a three-handled
  pad, a two-stick pad and a stick on a base. They're generic shapes, with no maker's logo and nothing traced from a product. Each console's `pad.art` picks one.
- **Its buttons are real `<button>`s** laid over the drawing, so remotes, keyboards and screen readers reach them. They light up as they're pressed, updated
  directly rather than by drawing again, to stay cheap at sixty times a second. The same mappings are always there as a plain list.
- **Remapping is by pressing:** choose a button, press what you want for it. What was already held doesn't count. Whatever was pressed isn't also taken for OK when the
  screen gets control back (`.wp-capture` makes `tv.ts` stand down). It stops waiting by itself, so a person with only a controller can't get stuck.
- **A source used twice is allowed, and shown** on both rows, with a Swap.
- **Everything works from a controller or remote:** steppers instead of sliders and drop-downs, and nothing that needs pointing. Choosing a file can't be done on a TV, so importing and uploading are
  left out there, and what's set elsewhere follows the account.
- **Colours are the theme's**, in the pairs `scripts/theme-contrast.py` checks. Labels have their own solid backing, so they stay readable on any picture.

### The background picture

Uploads are hostile until proven otherwise. Only PNG, JPEG and WebP are decoded, whatever the file claims, and an SVG is refused. The size in pixels is read from the header first (24 megapixels at most) and
one picture is decoded at a time, so a small file claiming to be huge can't use up memory. It's drawn again as a new JPEG of up to 2560 pixels, which leaves behind everything but the picture: EXIF, location,
anything appended. A photo stored sideways is turned upright first, as that's in the part dropped.

It's stored at `gaming/backgrounds/u<user id>.jpg`, from numbers only, and served to its owner with `nosniff` and the sandboxing policy every API answer has. No route takes an account number, so there's
nothing to change to reach someone else's. It goes with the account.
- **Touch sticks:** consoles with a stick get an on-screen analog one, read as how far a thumb pushes it. On the Nintendo 64, where the stick is how
  games steer, the on-screen D-pad is that stick. On PlayStation, whose games nearly all use the D-pad, it starts hidden and the editor can show it.
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
  file-size rlimits, on writable memory and with a much higher ceiling on address space, as real cores reserve
  far more than they use (one holding 30 MB had reserved 1.9 GB); lower priority; killed when the server dies.
- **Windows:** a job object that kills it with the server, limits memory and forbids child
  processes, plus a low-integrity token so it can only write to its save folder; below-normal
  priority.
- **Games on a graphics card** open a little more: see
  [What the sandbox opens](#what-the-sandbox-opens-for-a-graphics-driver).
  processes, plus a low-integrity token so it can only write to its save folder. It keeps normal
  priority: see below.
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
- Custom cores have their own rules: see [Adding your own core](#adding-your-own-core).
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
offered for download. An admin can add one with [Adding your own core](#adding-your-own-core), where it is labelled as
non-commercial, as that's their own copy and their own decision.

**Custom cores are third-party and unverified.** Each one shows its licence if there's anything to go on, and says where that came
from, or says "Unknown license". They are never offered, and the list of what we offer is kept apart from them in the code.

**ROMs and BIOS files are never bundled, downloaded or linked.** The console page lists the BIOS
file names a core expects, so you can supply your own.

### Where BIOS files go

Put them in a folder named `BIOS` inside a console's games folder. Any capitalisation works, and so do folders
inside it:

```
Games/
  Playstation 2 (PS2)/
    Some Game.iso
    BIOS/
      SCPH-39001/
        SCPH-39001_BIOS_V7_USA_160.BIN
        SCPH-39001_BIOS_V7_USA_160.NVM
```

The folder is never listed as games. When someone presses Play, the server looks in a `BIOS` folder beside the
game and one level up, and copies what the core wants to its own system folder (shown in Settings, Gaming),
leaving your copies alone. You can also put files straight in the system folder: a PlayStation 2 BIOS goes in
`pcsx2/bios` inside it.

**Satellaview games in `.bs` files** need the receiver's cartridge, named `BS-X.bin`, and only play on Mesen-S, which
opens on the receiver's own screens. Satellaview games dumped like cartridges need nothing.

**PlayStation 2 needs one** on LRPS2, the recommended core, and any region runs any game. With a whole set, the
server picks a late first-generation US model, then any US model, then European. Without one, the game falls
back to Play!, which needs no BIOS and runs fewer games. If only LRPS2 is installed the player says what's missing
and where to put it.

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
| PlayStation | pcsx_rearmed, mednafen_psx, swanstation\*, mednafen_psx_hw\* | GPLv2, GPLv2, GPLv3, GPLv2 |
| Nintendo 64 | mupen64plus_next\*, parallel_n64\* | GPLv2 |
| PlayStation 2 | pcsx2 (LRPS2)\*, play\* | GPLv3, MIT |
| PSP | ppsspp\* | GPLv2 |
| Dreamcast | flycast\* | GPLv2 |
| Nintendo DS | melonds, desmume | GPLv3, GPLv2 |
| Nintendo 3DS | azahar\*, panda3ds\* | GPLv2+, GPLv3 |
| PC Engine / TurboGrafx-16 | mednafen_pce_fast | GPLv2 |
| Atari 2600, 7800, Lynx | stella2014, prosystem, handy | GPLv2, GPLv2, zlib |
| WonderSwan, Neo Geo Pocket, Virtual Boy | mednafen_wswan, mednafen_ngp, mednafen_vb | GPLv2 |

If a game won't load on the chosen core, the other installed cores for that console are tried in turn. Some cores
start with settings other than their own defaults, where those don't suit streaming (`coreoptions.go`).

\* Draws with a graphics card. Every one of these exists on the build server for Windows and Linux amd64, checked
on 2026-10-31. LRPS2 and PPSSPP also need support files, which the install fetches from the same host.

## Testing with real games

`cmd/wittyplay-check` in `modules/wittyplay` plays real games with no browser. It starts the real runner
with its sandbox on, exactly as a game session does (`stream.StartRunner`), loads each game on a real core,
and records what happened.

```
cd modules/wittyplay
go build -o /some/folder/ ./cmd/wittyplay-check ./cmd/wittyplay-host
/some/folder/wittyplay-check -console nes -cores-dir /some/cores -install -out /some/report -jobs 8 /path/to/nes/games
```

For each game on each core it:

1. Loads it, and records how long that took and anything the core complained about.
2. Plays a set number of frames, pressing buttons to get past title screens, and on a console with a touch screen,
   tapping it too.
3. Saves a state, records what follows, goes back and checks the same pictures come again. Then it replays
   with buttons held, and a different picture proves input reached the game.
4. Quits the way the server does, with a save, and checks the process ended cleanly.
5. Starts a new process and restores the battery save and state the way a session does. The pictures must
   match, which is what proves a resume really resumes.
6. On a game with several discs, changes disc and back.

`-quick` stops after step 2, for sweeping a large collection.

A crash, hang or failure in one game only ends that game's check. The result is `ok`, `load-failed`, `crash`,
`hang`, `blank` (only ever one colour), `frozen` (never changes), `shut-itself-down`, `skipped`, `refused` (an
encrypted game, which nothing will try to open) or `too-slow-to-judge` (a busy machine couldn't run enough frames;
`-continue` tries those again). Warnings say what else looked wrong: `slow`, `silent`, `no-audio`, `audio-rate`,
`no-states`, `state-inexact`, `state-load-failed`, `no-input-effect`, `one-screen-blank`, `resume-*`, `quit-*`,
`disc-change-failed`.

The report folder has `summary.txt`, `results.jsonl` with every number per game, each game's last picture, and
`sheets/`, pages of pictures to look over, because wrong colours and garbled pictures are things only eyes catch. The
tool refuses a report folder inside a git checkout, as the report names your games, and its terminal output uses
numbers only.

| Flag | What it does |
|---|---|
| `-console` | A console id such as `nes`, or `all` with `-install` or `-smoke` |
| `-core` | One core id, a path to a core file, or `all` (the default) for every core offered |
| `-cores-dir` | The folder of cores. `-install` fills it with the app's own installer |
| `-smoke` | With no games: every core must open in the sandboxed runner and cope with a file of junk |
| `-jobs` | Games at once. Each one's sandbox is its own |
| `-seconds` | How much of each game to play, in game time. A busy machine takes longer, up to four times this |
| `-limit`, `-every` | Test only some of the games |
| `-quick` | Only load and play |
| `-shots` | Whether to keep pictures and make the sheets |
| `-trace` | Writes everything each core says, every option, and every environment call it was refused, to `trace-*.log`, plus pictures around each restore |
| `-system`, `-options`, `-memory-mb`, `-7z` | The BIOS folder, core options, the memory limit, and a 7-Zip program for `.7z` games |
| `-load-timeout`, `-hang` | How long a load may take, and how long with no new frame counts as a hang |
| `-continue` | Carries on a stopped run |

`.zip` games are unpacked by the tool, and `.7z` needs a 7-Zip program. Point it at unpacked copies otherwise.

Read speed and hangs only on a quiet machine: the runner has low priority on purpose, and the first time a new build
of it starts it can stall for many seconds. CPU numbers from Windows are too low for games that do little per frame, as
Windows counts a process's time in ticks. Linux's are the ones to believe.

### What has been tried

Run on 2026-10-31 on Windows, with the same cores checked on Linux where the table says so. "OK" means it loaded, drew
a changing picture and quit cleanly. NES, SNES and PlayStation had full-speed runs on a quiet machine, and the speeds
below come from those.

| Console | What was played | Result |
|---|---|---|
| NES | 318 real games on all 3 cores | fceumm 311 OK, Nestopia 314, Mesen 318. Every game plays on some core. fceumm runs at 100% speed, peak 25 MB |
| SNES | 1,861 real games, one dump of each, on Supafaust | 1,818 OK at 100% speed, peak 49 MB, no crashes or hangs. A sample of 187 with the full checks: 182 OK, and every state and resume in them good |
| SNES, others | The same 187 on Mesen-S; all 1,861 on bsnes | Mesen-S 180 OK. bsnes 1,642 OK on a busy machine. It wouldn't load 32, mostly games with chips whose firmware it wants, and 28 of those play on Supafaust |
| PlayStation | 1 loose game and 1 three-disc game | OK on PCSX ReARMed and Beetle PSX, with disc changes, resume and battery saves. PCSX ReARMed at 95% speed |
| Nintendo DS | 1 real game | OK on melonDS and DeSmuME, both screens counted, with states and resume. Speed not measured on a quiet machine |
| PSP | 1 real game | OK on PPSSPP, though a resumed picture doesn't quite match. Speed not measured on a quiet machine |
| 3DS | 1 real game, encrypted | Refused with a plain message, as it should be. No 3DS game has been played |
| Game Boy, GBA | 6 and 3 homebrew or test ROMs | OK on all cores. The GBA ROMs have no sound, so GBA audio is unproven |
| Mega Drive, Master System, Game Gear | 3, 3 and 1 homebrew ROMs | OK on all cores. ClownMDEmu's resume doesn't match on any of the 3; BlastEm, the default, is fine |
| PC Engine, Atari 2600, Virtual Boy, WonderSwan, Neo Geo Pocket | 3, 3, 3, 2 and 1 homebrew ROMs | OK on all cores |
| Lynx | 20 homebrew or test ROMs | 16 OK. The other 4 are test ROMs that draw one colour. All are silent |
| Atari 7800 | No game with a clear licence was found | Only that the core installs and opens |

Every core offered installs and opens in the sandbox, on Windows, on Linux and in the container image: 27 of 27 at the
time of that run. NES, SNES, PlayStation and homebrew samples also ran on Linux, where the sandbox is stricter.

In a browser, on the real server, with pictures looked at: WebRTC with NES, SNES and PlayStation; WebSocket with H.264
and with JPEG; input, sound, phone size with the touch controls, a change of disc, save slots, a resume after quitting,
and clean exits when the server or the runner is killed.

**The SNES games that stay blank on Supafaust**, 43 of them:

- 7 play on another core.
- 13 are Satellaview dumps. 31 of the 45 in the set play on Supafaust as they are; 2 of those that don't play on Mesen-S
  with `BS-X.bin`.
- 7 are Sufami Turbo cartridges and 2 are data packs, which all need a base cartridge plugged in.
- 6 aren't games: start-up programs of copiers and cheat devices.
- 8 others. Some use chips whose firmware the cores want, and none was available to test with.

These ran for 25 seconds on all three cores, so they aren't merely slow starters. About one in five games made no sound
in the first few seconds, with Supafaust and Mesen-S agreeing on 40 of the sample's 45, and that is usually a quiet title screen.
Most games ignore the buttons in those seconds too, which is why `no-input-effect` is so common.
**Listed but not playable yet** because they need hardware rendering: Nintendo 64, PSP, Dreamcast. Their controllers are
already set up, numbered as those cores number them.
Nintendo DS needs a pointer input for its touch screen, which comes after the first release.

## Limits today

- **Deflate64 and LZMA inside a `.zip`** aren't read, and say so. Deflate, bzip2 and Zstandard are. Nearly all zips are Deflate.
- **A `.7z` with locked contents** is only found out when played. See Security.
- **Cheats depend on the core.** libretro gives a frontend no way to know whether a core acted on a code, so a code that
  does nothing can't be told apart from a wrong one. Some cores have no cheat support at all.
- **Slow motion and fast-forward have no sound.**
- **A core's option labels are its own**, and in English: this frontend tells a core its language is English.
- **Patches** work on games that are a single file, not on discs (`.cue`, `.chd`, `.iso` and so on).
- **Software-rendered cores only** with the built-in runner.
- **Real cores:** the tests use the built-in test core. Loading and running real libretro cores has
  not been tested end to end yet.
- **A blank picture isn't noticed.** If a core won't load a game, the next is tried. If it loads and draws one colour, it
  stays there. 4 NES games on fceumm and 7 SNES games on Supafaust do this and play on another core, which an admin can
  pick in Settings, Gaming.
- **Few games on some consoles.** Only NES, SNES, PlayStation, DS and PSP have been tried with real games, and only NES
  and SNES with many. See "What has been tried".
- **Container image:** about 800 MB, up from about 150 MB, as cores from the build server need Debian's C library rather
  than Alpine's. Most of the size is Debian's FFmpeg, and the release's static FFmpeg could replace it.
- **OpenGL only** with the built-in runner: see above for why. melonDS's own OpenGL mode produces no frames here,
  so DS games draw in software at native resolution.
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
