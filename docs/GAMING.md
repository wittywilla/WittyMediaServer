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

| Game | Picture | Frames a second | Encoder delay | Graphics card | CPU |
|---|---|---|---|---|---|
| Nintendo 64, Mupen64Plus-Next | 1280x960 | 59 | 28 ms | 24%, encoder 11% | 0.3 of a thread, and 0.25 for the server |
| PlayStation 2, LRPS2 | 1024x832 to 1280x896 | 42, with the CPU fully taken by other work | 34 ms | 23%, encoder 5% | 1.1 threads |

On its own, NVENC's part is 22 ms at 1280x960, of which FFmpeg's command line holds one frame (17 ms at 60 frames a
second). It was 53 ms before `-delay 0`. The runner runs at below-normal priority so a game never starves a film, which
means a server with its CPU fully taken by other work will slow a game right down: to 4 frames a second, in one test.

Five minutes each of Nintendo 64 and PlayStation 2, running at once, gave no crash and no fault.

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

DS cores use their software renderers, so they play on a server with no graphics card. 3DS cores need one.

**3DS games must be decrypted.** These cores can't read encrypted game files, and nothing here will decrypt them.
A dump made with your own console's tools in decrypted form is what they want.

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
- **`auto` libraries** use the folder name first (`SNES/`, `Super Nintendo/`), then the file
  extension when only one console claims it. Files nothing claims are skipped.
- **Items** have kind `game` and a `console`. Multi-file games: a `.cue` hides its `.bin` tracks,
  and an `.m3u` hides the discs it lists.
- **`.zip`** holding one game is unpacked to a temporary folder when played.
- **What gets skipped:** BIOS folders (`bios`, `system`), and files that aren't games.

### Metadata and box art

| Source | Licence | Use |
|---|---|---|
| The filename, parsed: `Title (Region) (Rev 1)` | | Always |
| Your own files: `Game.png`, `Game-cover.jpg`, `Game.nfo` beside the ROM | yours | Always first |
| [Wikidata](https://www.wikidata.org) | CC0 | Title, year, developer, genre, summary (online, the same switch films use) |
| [libretro-database](https://github.com/libretro/libretro-database) | CC BY-SA 4.0 | Allowed with attribution, never bundled. Not used yet |
| [libretro-thumbnails](https://github.com/libretro-thumbnails) | **No licence stated** | **Not used.** With no licence, permission can't be assumed |
| OpenVGDB | **No licence on the repository** | **Not used** |

With no box art, the library shows a tile made from the game's name and console. Saved states
each show a picture of the game. Wikipedia summaries are CC BY-SA 4.0, as for films.

## Saves, states and resume

A state that arrives before the first frame waits for two frames that nobody sees. Some cores accept one
earlier and then start the game from the beginning anyway.

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
  file-size rlimits, on writable memory and with a much higher ceiling on address space, as real cores reserve
  far more than they use (one holding 30 MB had reserved 1.9 GB); lower priority; killed when the server dies.
- **Windows:** a job object that kills it with the server, limits memory and forbids child
  processes, plus a low-integrity token so it can only write to its save folder; below-normal
  priority.
- **Games on a graphics card** open a little more: see
  [What the sandbox opens](#what-the-sandbox-opens-for-a-graphics-driver).
- **macOS:** a `sandbox-exec` profile denying network and writes elsewhere, plus rlimits.
- **FreeBSD and others:** rlimits and priority only. Docs say so plainly.
- A session is stopped after 60 seconds with no viewer and after 30 idle minutes. The number of
  sessions is capped.

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

## Limits today

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
