# Games: design

How games are built and why, for anyone changing them. To play games, see [Games](GAMES.md).

[The API for clients](GAMING-API.md) is what a client uses, including TV apps. This page is about games the server emulates. Playing on a PC running Sunshine is a different thing beside it: see [Game PCs](GAME-RIGS.md).

## Where the code lives

| Part | Where | What it holds |
|---|---|---|
| The module | The **WittyPlay** repository | Console tables, settings, the runner (`host`, `libretro`, `cmd/wittyplay-game-runner`), streaming (`stream`), core installs (`cores`), patches (`patch`), the service (`rpc`, `cmd/wittyplay-module`), the player (`web/`) |
| Stubs in the server | `internal/play`, `internal/play/stream`, `cores`, `ipc`, `patch` | Written by `kit/facade` in WittyModule from `facade.json` in WittyPlay: the module's types, and functions that call its service. Never edited by hand, except `manual.go` |
| Routes | `internal/server/api_games*.go`, `api_game_options.go`, `api_game_profiles.go`, `game_peers.go` | Sign-in, access rules and saving settings |
| Scanning | `internal/library/games*.go`, `internal/gameid`, `internal/gamearchive`, `internal/gamemeta`, `internal/gameregion` | What a file is, what a game's called, and unpacking. These stay in the server |
| Pages | `web/src/pages/Games.tsx`, `pages/games/`, `GameReview.tsx`, `GameMeta.tsx`, `settings/Gaming.tsx`, `settings/GamingCustom.tsx` | The library, a game's page and couch mode. They have to work with the module missing |

A change to what the server calls is two changes in two repositories, and means running the generator again:

```bash
go run ./kit/facade -config <WittyPlay>/facade.json -module <WittyPlay> -server <this checkout>
```

Run it from a WittyModule checkout. [Modules](MODULES.md) has the rest of how a service runs and is released.

## What it has to do

- A Gaming library type, organised by console, with several libraries of one console allowed.
- Games run through libretro cores and stream to the browser.
- Every controller: gamepads in any button order, keyboard, on-screen touch controls, remapping by pressing, named profiles, hot-plug, several players.
- Every browser: Chromium, Firefox and Safari, on desktop and phones.
- Saves and save states per account, and resume where you left off.
- Runs on low-end servers. Emulators are memory-unsafe code fed untrusted files, so they stay boxed in.
- WittyMediaServer stays AGPL-3.0-or-later. GPL programs such as cores and RetroArch stay separate processes rather than being built into its binaries, no non-commercial cores offered, and no ROMs or BIOS files bundled, downloaded or linked.

## The architecture

```
 Browser              wittymediaserver      wittyplay-module              wittyplay-game-runner
+-----------+         +------------+  HTTP  +-------------------+  pipes  +---------------------+
| <video>   |  HTTP   | sign-in,   | on     | stream.Session    |<--------| libretro frontend   |
| gamepad,  |-------->| access,    | 127.   |  pion WebRTC      |  raw    |  loads one core     |
| keyboard, |         | settings,  | 0.0.1  |  WebSocket        |  frames,|  runs at the core's |
| touch     |         | scanning   |<------>|  fan-out          |  PCM    |  own frame rate     |
|           |<--------+------------+        |  saves to disk    |-------->|  sandboxed,         |
+-----------+    WebRTC: H.264 or VP8 + Opus|  ffmpeg (video)   |  input, |  no network         |
     ^           input on a data channel    |  ffmpeg (audio)   |  orders +---------------------+
     +--------------------------------------+-------------------+
```

Four kinds of process, each doing one job:

1. **`wittymediaserver`** signs people in, checks what they may play, keeps settings and scans libraries. It has no module code compiled in and never loads a core. Its stubs call the service. A WebSocket viewer is passed through to the service's own route, `/ws/{sid}`.
2. **`wittyplay-module`** is the service: one long-running process, while the server has a use for it, on `127.0.0.1` with a random port and a token made for that start. It holds the games being played, their WebRTC connections and encoders, and writes their save files.
3. **`wittyplay-game-runner`** is our own small libretro frontend, written in Go with no cgo. One per game. It loads a core, runs it one frame at a time and writes raw frames and audio to its stdout, reading input and commands on stdin. It holds no network connection and no database. A crash or an exploited core takes down one game. Cores print to stdout too, so the runner moves the pipe to a handle of its own before it loads one.
4. **FFmpeg**, which comes with the server. Video and audio each get their own process, so the hardware encoder detection the transcoder uses works here too.

### Two runners, one protocol

"The process that runs a game" is called a **runner**, and it's defined by a protocol rather than by a program: length-prefixed messages over a pipe (`ipc` in WittyPlay).

| | `wittyplay-game-runner` | WittyRetroArch |
|---|---|---|
| What it is | Our own small libretro frontend, built and in use | A fork of RetroArch with a mode that speaks the protocol. **Not built** |
| Licence | AGPL-3.0-or-later | GPLv3, a separate program |
| Software-rendered cores | Yes | Would |
| Hardware-rendered cores | Yes, with OpenGL, on Windows and Linux | Would |
| Size and upkeep | A few thousand lines of Go, cross-compiled from one machine | A large C codebase to build per platform and merge from upstream |

No console needs the fork. The server would use it if it found a program named `wittyretroarch` (`gameRunner` in `internal/server/api_games.go`), and **Game runner** in Settings, Consoles & cores can choose. See [WittyRetroArch](WITTYRETROARCH.md).

Cores are the same files RetroArch uses, and a RetroArch install's cores are found.

### Standalone emulators in Docker (optional)

WittyPlay can also run a standalone emulator (PCSX2, Cemu, xemu, Flycast and others) in a hardened container, one per game, instead of a core in the runner. It streams into the same WebRTC session, or through KasmVNC behind an authenticated proxy for emulators that need a desktop. It's off by default (`docker.enabled`), libretro cores always stay in the process sandbox, and a server without Docker carries on as before. The design, security model, setup and limits are in WittyPlay's [docs/DOCKER-BACKEND.md](https://github.com/wittywilla/WittyPlay/blob/master/docs/DOCKER-BACKEND.md). The server reaches its routes under `/api/modules/wittyplay/docker/...` and `/api/modules/wittyplay/kasm/...`. Only administrators can change its settings or install emulators, and only the player can open a game's desktop. See [MODULES.md](MODULES.md#who-is-asking).

### Why no cgo

A cgo frontend would need a C toolchain for every platform in the release matrix. We use [purego](https://github.com/ebitengine/purego) (Apache-2.0) to open the core and to receive its callbacks. It's linked **only** into `wittyplay-game-runner`, because on Linux it changes how the runtime starts threads and makes the binary depend on the system's libc.

## Consoles that draw with a graphics card

Nintendo 64, PlayStation 2, PSP, Dreamcast, 3DS and the faster PlayStation cores draw with OpenGL, so the server's graphics card renders the game and the result is streamed like any other frame. This runs on Windows and Linux servers, and on a Mac with a runner that makes an OpenGL context there (see [A Mac's graphics card](#a-macs-graphics-card)); elsewhere the runner refuses at once. There, and on any server without a usable card, Nintendo 64 and PSP have the processor draw instead, and the other consoles say they can't play on this server: see [Without a graphics card](#without-a-graphics-card).

**The choice: OpenGL in the built-in runner, using the system's own libraries through purego.** It needs no C compiler, no second program and no GPL code. Every core we offer for these consoles can use OpenGL. Vulkan isn't built: no core we offer needs it. Cores are told OpenGL is what's preferred, and those that can also draw with Vulkan or Direct3D use it.

```
 core -- draws into --> framebuffer we made -- glReadPixels --> pixel buffer -- fence --> frame
                        (colour + depth/stencil)                (no stall)                to the server
```

- **The context** is a hidden window with WGL on Windows, which nothing ever shows, and EGL with no display on Linux. On Linux every card is an EGL device, so the runner takes the first that isn't drawn by the CPU, preferring anything to integrated Intel. Under WSL it goes through `/dev/dxg` and Mesa's D3D12 driver.
- **Reading frames back** doesn't stall the game. The copy is queued behind the frame's own drawing, and the runner only waits for that drawing to finish, after the core has returned. OpenGL's rows are bottom-up, so they are flipped once, on the copy that goes down the pipe.
- **Encoding:** a game on the graphics card uses whichever hardware encoder works here, whatever the transcoder's setting says, because its frames are large and its card is awake. NVENC is told not to keep a queue of finished frames (`-delay 0`). Its own conversion from RGB is always BT.601, which browsers wouldn't undo correctly, so frames go through FFmpeg's scaler first: see [Encoding](#encoding).
- **Internal resolution** is a setting in Settings, Consoles & cores: Automatic, or 1x to 4x. Automatic is 2x on Nintendo 64, PlayStation, PSP and Dreamcast, and native otherwise (`DefaultResolution`). The per-core options are in `traits.go`. A choice a core doesn't offer is ignored, so an updated core can't break.
- **PlayStation 2 stays native** because its games draw full-screen effects in small tiles, and the fixes that hold those together when upscaled are worked out per game by the core's own database, which doesn't always succeed. One racing game at 2x comes out in bands of the wrong colour with a grid over them, from the moment of a fresh start. A state loaded into a fresh runner hides it, because those fixes aren't applied to one, which makes it look intermittent. Native has no such seams, and a quarter of the pixels to draw, encode and send. Upscaling is still there to choose, game by game.
- **How many at once:** two by default, set in Settings, Consoles & cores. Past that, starting another says the card is full. Software-rendered games don't count.
- **No usable card:** OpenGL drawn by the CPU (`llvmpipe`, Microsoft's GDI renderer) doesn't count. Settings, Consoles & cores names the card in use, or says why there is none. What happens to these consoles then is in [Without a graphics card](#without-a-graphics-card).

**What it costs.** [Where the time goes](#where-the-time-goes) has the numbers for each console. At 1280x960 on an RTX 3070, NVENC takes about 5 ms a frame. PlayStation 2 on an 8-core laptop runs at 45 to 60 frames a second in a race, with VU1 on a thread of its own (the core's database still turns that off for games known to hang with it) and without waiting on the graphics card at every read-back. Both are in `traits.go`.

**What PPSSPP does by itself:** the first time a PSP game starts for an account, the core unpacks support files from that game's own disc into the save folder, about 20 MB. That's part of the core, which has no option to turn it off, and nothing in this server asks for it or sees the results.

### Without a graphics card

The runner's answer to "which card would you draw with?" (`stream.ProbeGPU`, asked once in the background) decides it, and until it's in, a Mac or FreeBSD server is taken to have none (`gpuMissingFrom` in `internal/server/games_gpu.go`). A PlayStation 2 game with no BIOS for LRPS2 says it needs one, rather than that it can't play here. Then `pickCore` chooses, for the game page (`GET /games/{id}/check`), the console list, Prepare and Play alike:

1. A core that needs no card plays as it always does.
2. A card's core that can have the processor draw starts that way (`cpuDrawing`): its options for that are laid over everything else, it gets no `-gpu` room in the sandbox and doesn't count against the card's limit, and the game page, the console list and the session's `notices` carry a speed caveat.
3. Anything else is **not on this server**: `reason: "gpu"`, with the runner's reason, everywhere a game can be started from, and Play and Prepare refuse with 409 before a runner starts. A crash on a context that was never there isn't left to happen.

| Core | Options that have the processor draw | Measured on an Apple M3 Pro, macOS 27 |
|---|---|---|
| Mupen64Plus-Next | `mupen64plus-rdp-plugin=angrylion`, `mupen64plus-rsp-plugin=cxd4` | Banjo-Kazooie and Bomberman 64: full speed (the core ran 60 frames a second), about one core |
| ParaLLEl N64 | `parallel-n64-gfxplugin=angrylion`, `parallel-n64-rspplugin=cxd4` | The same games: full speed, under one core. What libdragon homebrew already used |
| PPSSPP | `ppsspp_backend=none` (its own software renderer; never asks for a context) | God of War: Ghost of Sparta: 58 frames a second on average, with dips to the 30s and 40s, on three to five cores |
| LRPS2 | `pcsx2_renderer=Software (SW)` (hands frames over without a context) | Crash Bandicoot: The Wrath of Cortex, with the owner's SCPH-39001 BIOS: its 3D intro mostly at 60 frames a second, down to 6 to 20 in stretches; streamed to a browser at 60 |

Measured in the runner alone, with no sandbox or encoder. Mupen64Plus-Next and PPSSPP were then streamed end to end from a Mac server to a browser, at normal speed, with libx264 taking 3 to 7 ms a frame. angrylion is the accurate Nintendo 64 renderer, so it draws at the console's own resolution and the internal resolution setting doesn't apply; PPSSPP's software renderer is forced to 1x the same way.

Not built, and why:

- **Dreamcast, 3DS, Play! and the faster PlayStation cores** have no software path. On a Mac without a card, PlayStation plays on PCSX ReARMed instead, and PlayStation 2 needs LRPS2 and a BIOS.
- **Vulkan through MoltenVK** would cover LRPS2's Vulkan and paraLLEl-GS renderers, PPSSPP's Vulkan backend and paraLLEl-RDP: the runner would have to offer cores a Vulkan context, and ship MoltenVK. The largest of these.

### A Mac's graphics card

WittyPlay's runner can make an OpenGL context on a Mac (CGL, OpenGL 4.1: the `mac-opengl` work, not yet in a release). The runner then names the card, such as "Apple M3 Pro", and the server uses it. Measured in the runner alone on an Apple M3 Pro with the owner's own games, 30 to 60 seconds each, on a Mac that was busy with other work:

| Console and game | Core, and how it drew | Frames a second | Processor |
|---|---|---|---|
| PSP, God of War: Ghost of Sparta | PPSSPP, OpenGL | 59.6 on average | 0.16 of a core, against 3 to 5 cores with the processor drawing |
| PlayStation, Final Fantasy VII | SwanStation, OpenGL | 59.6 | 0.14 of a core |
| PlayStation, Final Fantasy VII | Beetle PSX HW, OpenGL | 60 | 0.3 of a core |
| PlayStation 2, Final Fantasy X International | Play!, OpenGL | 60.5 | 0.36 of a core |
| PlayStation 2, Gran Turismo 4 | Play!, OpenGL | 60 once loaded; it stalls while loading | 0.27 of a core |
| Nintendo 64, Super Mario 64 and Banjo-Kazooie | Mupen64Plus-Next, GLideN64 | 38 to 56, and every picture black | 0.25 of a core |
| Nintendo 64, Super Mario 64 | the same, framebuffer emulation off | 60, drawn | 0.22 of a core |
| Nintendo 64, Banjo-Kazooie | the same, framebuffer emulation off | 38, black after half a minute | 0.24 of a core |
| Nintendo 64, Super Mario 64 | ParaLLEl N64, Glide64 | no pictures at all | |
| PlayStation 2, Crash Bandicoot and Final Fantasy X | LRPS2, OpenGL | no picture: it needs `GL_ARB_shading_language_420pack`, which a Mac doesn't have | |

So on a Mac with a card, Mupen64Plus-Next, ParaLLEl N64 and LRPS2 are still drawn by the processor (`cardFaults` in `internal/server/games_gpu.go`), which draws Nintendo 64 at full speed on about one core. Where nobody chose a core, PlayStation 2 plays on Play!, on the card, rather than on LRPS2, which comes first but can't use the card (`rightOnTheCard`); a game Play! can't open, such as a `.bin`, still goes on to LRPS2. The game page, the console list and the session say why, and Settings shows them as drawn by the processor. Everything else uses the card. Dreamcast and 3DS weren't measured: the owner's games have no Dreamcast disc, and their only 3DS game is an encrypted eShop package, which no core opens. The Nintendo DS needs no card: melonDS draws Zelda: Phantom Hourglass at 60 frames a second on 0.22 of a core.

### Two graphics cards

On a laptop with Intel and NVIDIA, Windows hands OpenGL to the card driving the display, usually the slow one. The runner sets `SHIM_MCCOMPAT=0x800000001` in its own process before OpenGL loads, which NVIDIA's driver reads as "use the discrete card". No system setting changes. On a laptop with an AMD discrete card, Windows may pick the integrated one, and [Games](GAMES.md#consoles-that-need-a-graphics-card) says what to do.

### What the sandbox opens for a graphics driver

Only games on a core marked as drawing with a graphics card get these (the `-gpu` flag). A core the table doesn't know gets them too, since nothing says it doesn't need them.

| Platform | What changes | Why |
|---|---|---|
| Linux | Landlock also allows writing to `/dev/dri`, `/dev/dxg`, `/dev/kfd` and `/dev/nvidia*`, and nothing else | Opening these read-write is how every driver talks to its card |
| Linux | The address-space limit is dropped; the limit on memory actually written to stays, at 8 GB | Drivers map far more addresses than a machine has memory |
| Windows | The job's memory limit is 8 GB, not 2 GB | Graphics drivers and the larger emulators commit a lot |

Everything else stays: Linux's namespaces, `no_new_privs` and the seccomp filter, and Windows's job object, which kills the runner with the server and forbids child processes, and its low-integrity token.

What a sandbox can't cover: a graphics driver is a large body of code running inside the process, talking to the kernel through interfaces built for speed. A core taken over by a bad ROM can reach the same driver and kernel surface a game can. Treat a server with a GPU as a machine where a hostile ROM has more to work with.

**As a Windows service:** session 0 has no desktop, and consumer OpenGL drivers often refuse to draw there, so Settings, Consoles & cores says there's no usable card.

**Under the Linux sandbox,** LRPS2, melonDS, SwanStation, PPSSPP and Flycast make files in `/dev/shm` to lay out the console's memory, and crash or refuse to start if they can't. So these may (`-shared-memory`, `CoreTraits.SharedMemory`), in a folder of their own: the runner starts in a mount namespace with one capability, puts an empty `tmpfs` over `/dev/shm`, gives the capability up and starts itself again before anything is loaded. Nothing else can reach that folder and nothing's left behind, however the runner ends. Where namespaces aren't allowed, Landlock lets the core write to the system's `/dev/shm` instead, which is shared with whatever else runs under the same account, so no other core and no custom core is given this. LRPS2's `cache` folder stays beside the save between games (`CoreTraits.CacheIn`), and backups leave it out. No graphics driver here supports program binaries, so a shader file being written and read back hasn't been seen.

**What else a few cores ask for, and how they get it.** glibc 2.41 and later refuse to load a library that asks for an executable stack, as melonDS does. Its asking is an accident of how one file was built: it runs 60 frames a second without one. So a core that asks is opened from a private copy with the request taken off (`libretro/execstack_linux.go`), and no core ever gets an executable stack, on any glibc. The core's file isn't touched, so its recorded checksum still holds. melonDS reserves 16 GiB of empty address space and has no fallback if it can't, so it gets a higher ceiling on that (`-large-address-space`, `CoreTraits.LargeAddressSpace`). The data limit still holds what it really uses. Play! stops on starting if it can't make its folder under the home folder, so it has its save folder as one (`-own-home`, `CoreTraits.OwnHome`), and its memory cards are kept there.

### Cores that keep their own stacks

Several cores run the game on stacks they allocate themselves, and call back from there. The Go runtime refuses a callback on a foreign stack on a thread it created, so on Linux the runner hosts the core on a thread it didn't: `libretro.OnCoreThread`.

## The game pages

The library, a game's page and couch mode are in `web/src/pages/games`, and the menus and screens around a game are WittyPlay's own (`web/QuickMenu.tsx`, `web/Screens.tsx`, `web/prompts.tsx`). The prompts have a copy on each side, `pages/games/prompts.tsx` for the pages, because the pages have to work with the module missing.

- **One request for the home screen.** `GET /games/home` answers with every shelf, so a TV app shows the same as the web page.
- **Only what's on screen is on the page.** `VirtualGames.tsx` draws the rows near the screen and asks for the pages of results those rows need, and stops asking during a fling.
- **Moving costs a transform.** The cover flow moves its covers with `transform` and `opacity`. The tint behind the page is a colour from a 16-pixel copy of the cover, faded between two layers. Nothing is blurred.
- **Controllers come through `tv.ts`.** It turns buttons into key presses, which aren't "trusted" as real ones are, so a page can tell a controller's X from someone typing. X searches, Y marks a favourite, View changes the sort, the bumpers move between sections, and the triggers jump along the alphabet when sorted by title. Start opens couch mode. While a game is playing, `tv.ts` leaves the controller to the game, and Start and Select together open the game menu.
- **Prompts follow the last thing touched.** `useInputKind` watches for it. A controller shows its own letters or shapes, with Nintendo's confirm button on the right as `tv.ts` maps it. A mouse or a finger sees no prompts.
- **What a game needs is checked before Play.** `GET /games/{id}/check` looks for a core and a BIOS the way pressing Play does, so a page can say what's missing and link to the fix. The player's error screens read the same flags from a refused start (`needCore`, `needBios`, `encrypted`), and `web/words.ts` turns them into what to do next.

Couch mode is `/big`. It's the same pages in a frame of their own, with larger sizes through `html.ten-foot`, and it survives going into a game and back for that tab.

## Touch screens

Nintendo DS and 3DS games are played on a touch screen. A press on the picture is the stylus, with a mouse or a finger, and sends a position as fractions of the picture ([the API](GAMING-API.md#touch-screens)). The cores get it as libretro's pointer device, and work out which screen it fell on. With on-screen controls showing, a finger on a button presses the button and a finger anywhere else is the stylus.

DS cores use their software renderers, so they play on a server with no graphics card. 3DS cores need one.

**3DS games must be unencrypted.** Nothing here decrypts them or looks for keys (`threeds.go`). The server reads the plain headers of `.3ds`, `.cci`, `.cxi`, `.app` and `.cia` files, and an encrypted one is still listed, with a clear message on its page and on *Needs a look → Can't play*. A game's bytes are what count: a package marks its contents as encrypted, but its program must also start with `NCCH`, and that program must have its NoCrypto flag set. A package marked encrypted whose program is plainly readable plays. One marked clear whose program is scrambled doesn't.

A ticket and a title key say nothing either way, as tools that remove a package's encryption leave them in. Nor do a readable title and icon, as a package keeps those in the open while its program stays encrypted. Fixed-key programs, whose key is public, are still encrypted and still refused. `wittyplay-probe cia-info -hash file.cia` prints each of these bytes and where it is, and compares the contents with the checksums in the table of contents, which cover their plain form.

A `.cia` is an install package, and Azahar's list of what it opens doesn't include it, so an unencrypted one has its main program copied out, in the server's `gaming/tmp/3ds`. A big game's copy takes a while and as much room as the game, so the last one stays for next time, and goes when another package is played with nothing running, or the server restarts. Updates and extra content inside a package aren't applied. A title's type says if a package is an update or extra content rather than a game, and those aren't listed as games. A game made only for the New 3DS says so in its header, and Azahar is told to be one (`citra_is_new_3ds`). No retail 3DS game has been played: the one tried is encrypted. A plain package has, using `wittyplay-probe test-cia`, which writes one from a few hand-assembled instructions.

## Why WebRTC, and what happens when it can't connect

Every device now connects the same way: [one stream for every device](GAMING-STREAM.md) has the decision, the
measurements behind it and the wire protocol, version 2. This section is about the browser.

Latency is the whole product. WebRTC has no segment buffering, it drops late frames rather than stalling, and the sender can tell a browser to keep no buffer at all. Input goes over an unreliable, unordered data channel carrying the complete pad state in every packet, so a lost packet is corrected by the next one.

Not every network or browser allows it, so the player steps down by itself:

| Step | Video | Audio | When |
|---|---|---|---|
| 1. **WebRTC** | H.264, or VP8 where the browser has no H.264 | Opus | The normal case. Input on a data channel |
| (chosen) **WebRTC, drawn directly** | H.264 taken as it arrives, decoded by `VideoDecoder`, drawn to a canvas | Opus | Only when someone picks it under Connection. It can tear |
| 2. **WebSocket + WebCodecs** | H.264 decoded by `VideoDecoder`, drawn to a canvas | PCM | UDP is blocked, or only a reverse proxy is reachable. Input on the WebSocket |
| 3. **WebSocket + JPEG** | One JPEG per frame, encoded in Go, drawn to a canvas | PCM | No WebRTC, no WebCodecs and no H.264, or FFmpeg can't encode |

The steps share the same session, so changing transport never restarts the game. A recovering player keeps the old transport until the new one has a picture (`connect()` in `Player.tsx`).

## Where the time goes

From a press to the screen, a frame passes through these stages. Every one is measured on real frames, all the time the on-screen numbers are showing.

| Stage | What happens | What's been done about it |
|---|---|---|
| Reading the pad | The browser's pad state is read | Several times per screen refresh (`PAD_READ_MS`), not once. A TV keeps to the screen's pace |
| Network, up | One packet on an unreliable channel | Nothing waits for anything, and a lost packet is corrected by the next |
| Waiting for the core | A press arrives while the game waits for its next frame | **A new press starts the next frame straight away.** Frames stay on the same schedule afterwards, so the game keeps its speed. For the smaller consoles this takes the wait from about 8 ms to nothing. `host.waitForFrame` |
| The core | The game runs a frame, and a graphics card's frame is read back | Unavoidable, and it's the core's own speed |
| Runner to server | The picture crosses a pipe | Newest frame wins: a picture still waiting is replaced |
| Queue for the encoder | Waiting for the encoder | **It holds one frame at a time.** FFmpeg has queues between its threads, which can hide many frames, every one of them delay. A frame that arrives while it's busy replaces the one waiting |
| Encoding | | The fastest settings of each encoder, nothing buffered |
| Network, down | | A frame's RTP timestamp comes from the real time, and it isn't paced out |
| Jitter buffer | A browser holds frames back to smooth them out | **Gone.** The sender's `playout-delay` header extension says frames may not wait. A page can only ask for a smaller buffer, and even then a frame waits for an estimate of the network's jitter and 10 ms more. This one change took frames from 50 to 200 ms down to about 1 ms in the browser |
| Sound's delay | Tracks of one stream are held back to match each other | Sound has a stream of its own, so its buffer can't become the picture's delay. Both follow real time, so they don't drift apart |
| The display | A frame waits for the next refresh | **Frames land just before the display wants them** where a browser shows that helps: `PhaseLock` in `web/latency.ts` moves when the game makes them, a little at a time. It only moves them through one refresh looking for the edge, and gives up if the wait doesn't change |
| Sound | A browser holds back enough sound to cover how unevenly it arrives | Sound is sent evenly, as fast as it plays, and its clock jumps over any gap. A browser holds about 35 to 80 ms of it, which leaves sound that much behind the picture. After a one-second stall it's back within about 15 seconds |

The numbers are from `wittyplay-probe` (see [Measuring it](#measuring-it)):

| Console, core | Picture | Finished frame to screen, ms | Press to screen, ms |
|---|---|---|---|
| NES, FCEUmm | 256x224 | 9 | 11 |
| Super Nintendo, Mesen-S | 256x239 | 13 | 22 |
| Nintendo 64, Mupen64Plus-Next | 1280x960 | 22 | 31 |
| PlayStation, PCSX ReARMed | 364x240 | 10 | 12 |
| PlayStation 2, LRPS2 | 640x448 | 14 | 49 |
| PSP, PPSSPP | 960x540 | 16 | 23 |
| Nintendo DS, melonDS | 256x384 | 14 | 19 |

**These are headless, on one machine.** Medians, from an RTX 3070 laptop with an i7-12700H, NVENC, WebRTC on the same machine, and Edge with no window, while other work kept the CPU 20 to 60% busy. With the CPU at full load, they're about 10 to 25. Where the time still goes on a Nintendo 64, at 1280x960: about 4 ms in the core, 2 reaching the server, 5 to 7 encoding, under 1 for the network, 1 decoding and 8 waiting for the display. A PlayStation 2 game takes most of a frame to run, so a press waits for the one in progress.

**Press to screen** is from a press leaving the player to the first frame on the screen that the core made having seen that press. It's the most this path can claim. Games themselves take a frame or more to react to a button, as they did on the real console.

**What it can't see:** "on the screen" is when the browser expects the frame to reach the glass. Real displays add their own delay, and a headless browser's idea of the display is not a monitor. Only a camera can measure the whole of it. Firefox, Safari, phones, TVs and real networks haven't been run.

### Run-ahead, by default on the older consoles

A game's own reaction is the largest stage left, and only run-ahead can change it. On a real NES game and a real SNES game (`wittyplay-probe react`), the picture changes two frames after a press without it, and one frame after with it. So it's on by default, one frame, for cores whose whole state saves in a moment (`quickStates` in `stream/control.go`), on servers with four CPU threads or more. It costs under a millisecond a frame on a NES and about 5 ms on the SNES's most accurate core, and turns itself off, quietly, for a game that falls behind. The setting is *Automatic run-ahead*, under Speed and rewind. Administrators can rule out run-ahead altogether.

### Speed or steadiness

`emulation.latency`, under Speed and rewind. **Fastest** shows every frame as it arrives. **Balanced** lets the browser hold a frame for up to 80 ms in a bad moment. **Steadiest** leaves it to the browser's own judgement, and doesn't start frames early on a press. **Automatic**, the default, is Fastest until frames start arriving unevenly for three seconds, and goes back after twenty calm ones.

### Drawn directly

WebRTC's encoded frames can be taken as soon as they're whole, before the browser's jitter buffer and renderer, decoded with `VideoDecoder` and drawn on a canvas that doesn't wait for the page's compositor (`web/direct.ts`). It keeps WebRTC's retransmissions. On a real display this may show a frame a whole refresh sooner, and it can tear. A headless browser has no display to measure that against, so it's for choosing under Connection, and isn't the default until someone has measured it with a camera.

### Poor networks

- **Rate:** bitrate follows the browser's reports of its video (`rateControl`). It falls straight to what's getting through when loss is heavy, and by steps when round trips grow. It goes back up after 20 calm seconds. Changing it means a new encoder, so changes are at least 5 seconds apart. On a link capped at 4 Mbit/s with 30 ms of delay, a 1280x960 game settles in a few seconds with nothing lost, at 37 ms from frame to screen.
- **Bursts:** a frame can't use more than a quarter of a second's bits. Left alone, encoders give a keyframe a second's worth or more, which a slow network drops, and the browser asks for another, for good.
- **Lost pictures:** a browser that has lost track of the picture asks for a keyframe, which starts another encoder, as FFmpeg can't be told. That stops the picture for about a quarter of a second, so if the regular keyframe is due within 300 ms, it waits for that.

These have only run against the probe's simulated network.

### When something stops

Everything comes back by itself, with the last picture up until there's a new one:

| What stops | What happens | Measured |
|---|---|---|
| Video encoder dies | Another starts at the next frame | Picture back in 0.25 s |
| Video encoder hangs | Noticed after 3 s with a frame inside it, then the same | 3.5 s |
| Sound encoder dies | Another starts | Under 1 s |
| Network drops, or changes | The player reconnects, over the same list of ways as at the start. The server pauses the game after about 6 s with nobody connected, and keeps it for a minute | Under 1 s after it returns |
| Decoder or browser sticks | No new frame for 4 s on a visible page means reconnecting | About 5 s |
| Tab closed, or browser dies | The game pauses, and carries on when someone returns within a minute | |
| Runner dies | The game ends and the player says why. What was saved stays, and it starts again from there | |
| Server dies | Nothing it started is left behind. Once it's back, the player finds the game is gone, as below | |
| The game has gone from the server | The server or the module restarted, or this device was away too long. A player that finds this starts the game again from its last save. If that happens twice in two minutes, it says so instead | Module killed: playing again in about 5 s |

**A game that won't start** is recorded against its core (`game_core_plays`, which Settings shows for custom cores), with the reason. It doesn't count against WittyPlay itself: only its helper going away or answering nonsense does (`stream.ErrHelper`), so one console's core crashing again and again can't take every other console's games away. See [Modules](MODULES.md).

**The soak test:** `wittyplay-probe soak` ran a Nintendo 64 game for 65 minutes, through NVENC to a real browser, on a machine busy with other work. The game kept 60 frames a second and frame to screen stayed at 18 to 27 ms. Comparing the first ten minutes with the last: the runner 956 MB and 950 MB, the encoder 350 and 352, the server's heap 25 and 26, the page 12 and 17, goroutines 43 and 43, the runner's handles 380 and 384. Nothing grew. A second run took an NES game with its sound and run-ahead for 61 minutes: frame to screen stayed at 9 to 13 ms, a press at 10 to 14, the runner at 28 MB, and nothing was lost or dropped.

### Measuring it

`wittyplay-probe` (`cmd/wittyplay-probe` in WittyPlay) runs a game through the real path to a real browser. It's given the runner, a core, a game and a browser, and starts everything:

```
wittyplay-probe run -runner ./wittyplay-game-runner -core fceumm_libretro.so -core-id fceumm -console nes -rom game.nes -save /tmp/save \
    -ffmpeg ffmpeg -encoders h264_nvenc,libx264,libopus -browser /path/to/chromium -seconds 30 -shots /tmp/pictures
```

It draws each frame's number and the newest input it saw into the top of the picture, with known colours under them. The page reads those back from what the browser actually shows, so nothing else has to be believed: not the clocks, not the records. It reports each stage's middle and 95th percentile, how far the colours have moved, and with `-shots`, what the core made beside what the browser showed.

| Command | What it does |
|---|---|
| `run -impair delay=30,jitter=5,loss=2,kbps=4000` | Through a network of that kind, on this machine |
| `soak -minutes 65 -out soak.csv` | A line a minute of every stage, memory and handle counts, and a verdict on whether anything grew |
| `chaos` | Kills each process in turn, and takes the network away, and says whether the picture came back |
| `frames -out folder` | What leaves the core, before any encoder, as pictures. A fault in these isn't the stream's |
| `react -button start` | How many frames a game takes to show a press, on a still screen |

On Windows, use `-loopback`, or Windows Firewall will stop to ask. `scripts/build-probe.mjs` rebuilds the test page after a change to the player's transport.

### Encoding

- **Encoder order:** the hardware encoders the transcoder has already test-encoded with (NVENC, Quick Sync, VideoToolbox, AMF, VAAPI), then `libx264`, then `libopenh264`, then `libvpx`. A failed start falls through to the next one.
- **Settings:** no B-frames, no lookahead, a keyframe every second so a lost packet heals quickly, and the fastest preset of each encoder. `libx264` uses sliced threads, because frame threads hold back a frame per thread: about 250 ms on a machine with many cores.
- **Colours:** frames are converted to BT.709 and marked as that (VP8, which has only BT.601, is converted to that). Browsers show an unmarked H.264 stream as BT.709 and not all of them read the mark, so what's sent has to be what they assume. The probe measures this on every run.
- **Sharp pixels:** retro games are small and 4:2:0 chroma smears pixel art, so frames up to 400 pixels wide are doubled with nearest-neighbour before encoding. On a server with fewer than four CPU threads and no hardware encoder they are not doubled.
- **Framing:** FFmpeg writes FLV for H.264 and IVF for VP8. Both mark exactly where a frame ends, so we never hold a finished frame while waiting for the next one to prove it finished, which raw Annex B would cost us one full frame of delay.
- **A slow encoder drops frames instead of building a queue:** the session keeps only the newest unencoded frame, and gives the encoder one at a time.

### What low-end servers can expect

A retro console at 2x is about 512x448 at 60 frames a second. Two sessions is the default limit. On Linux and macOS the runner runs at a slightly lower priority, so a film being transcoded comes first without the game being starved, except for games on a graphics card and the Nintendo DS, which keep normal priority. Windows has no such gentle setting: below-normal gets next to nothing while other programs are busy (one game measured 1 frame a second, against 61 at normal, with the machine under half busy). So on Windows it runs at normal priority, sharing fairly, and a software-rendered core uses about one CPU core. Audio costs about 1% of a core.

## On-device play (not planned)

Every game runs on the server and is streamed. The device you hold only decodes the picture, plays the sound and sends your presses: no game runs in a browser, a phone or a TV, and no copy of a game is sent to one. Libretro cores also exist compiled to WebAssembly, and that way was weighed and set aside in 2026-11.

**Why:** at home it wouldn't be faster. A game in a page runs in step with the page's frames, so a press waits half a refresh on average before the game looks at it, where a streamed game starts its next frame at once. An upscaled or 3D game is far beyond what a browser can run anyway. It would also need a copy of the game on every device that plays it, a second way for saves and controllers to work, and it leaves out the usual cores for the Super Nintendo and the Genesis, whose licences don't allow it. The plan is to make the stream itself better instead: lower delay, steadier sound and picture, and picture filters run on the server, so every device shows a game the same way.

## Libraries and scanning

- **Library type `games`**, with a `console` on the library (`nes`, `snes`, and so on) or `auto` for one folder holding many consoles. Several libraries of one console work the same way as several Movies libraries: the add flow's "combine or separate" choice applies per console.
- **Items** have kind `game` and a `console`. Multi-file games: a `.cue`, `.gdi` or `.ccd` hides the files it names, and an `.m3u` hides the discs it lists.
- **Archives** count as the game inside them, so nothing needs unpacking first.
- **What gets skipped:** BIOS folders (`bios`, `system`), and anything shown not to be a game.

### What a scan believes

A file's name is where the question starts, not where it ends. So every file is judged on evidence (`internal/gameid`, and `internal/library/games_detect.go`), and a game's page shows an administrator what the evidence was.

| Evidence | How sure | What it covers |
|---|---|---|
| **The file's own bytes** | 100 | iNES, UNIF and Famicom Disk System headers; the Super Nintendo's internal header, scored; the Nintendo 64's first word in any byte order; the logos a Game Boy, Game Boy Advance and DS check before starting; `SEGA` at 0x100 on a Mega Drive; TMR SEGA, with its console digit; Neo Geo Pocket's copyright line; Lynx and Atari 7800 headers; and on discs, `SYSTEM.CNF` (`BOOT` is PlayStation, `BOOT2` is PlayStation 2), `PSP_GAME` and its `PARAM.SFO`, `PBP`, the boot sector of Dreamcast, Saturn and Sega CD discs, GameCube and Wii magic numbers. `.cso` and `.zso` are read through their compression. A sheet is judged by the file it names |
| **The folder or library**, and a file that can't confirm it | 70 to 75 | `.chd`, `.cdi`, `.gdi`; consoles with no header at all, like the Atari 2600, checked for size (`gameid.CheckFor`) |
| **An extension only one console uses**, with nothing else to go on | 60 | `.smc`, say, with no folder to help, and big enough to be a cartridge |
| **A person said so** | 100 | Anything on the *Needs a look* list |

The file wins an argument. A PlayStation 2 game in a folder named for the PlayStation is a PlayStation 2 game. In a library set to one console, a game for another isn't quietly listed or quietly dropped: it goes on *Needs a look*.

**Nothing unproven becomes a game.** Every file is either listed, or on one of two lists on the library's page, each with its reason:

- **Needs a look** is for files that might be games: no header where every game has one (`mustHaveHeader`), a disc with nothing to say which console, a console with no core here, an archive holding hundreds of different things. One click says what it is, or that it isn't a game.
- **Left out** is for what's been shown not to be one: BIOS files (by name and by bytes), updates and extra content (a 3DS title's type in its title id, a PSP's `CATEGORY`, a name's tags), test cartridges and cheat devices, music CDs, chiptunes, text, programs, film discs, and a second copy of something. These can be overruled.

These choices are kept in the database, beside the library. A header-less NES dump is listed if its size fits, with a note: only Mesen can play one, by its checksum.

**Several files, one game.** After each file has been judged:

- **Copies** are found by checksum, across folders and whatever they're named. Only files the same size as another are ever read for this. The one that needs no unpacking is kept.
- **Discs** come together by name: `Disc 2`, `(CD2)`, `disc2of3`, or a folder called `Disc 2`. They can be loose, in archives, or both, and they're offered to the core as a playlist, so the game's menu can change disc. A disc kept on another drive from the data folder is copied beside the playlist for as long as the game is running, unless a link can be made, because cores can't follow a playlist across drives.
- **Versions** (regions, revisions, betas) of one name on one console share a tile, headed by a released version in the first region of the owner's order. The head stays the head, as saves belong to it. Each version is still its own game, with its own saves.
- **Sets:** an archive of versions of one game plays the verified one in the first region of the owner's order, leaving out bad dumps, hacks and translations, and its page lets an administrator choose another (`games_sets.go`).
- **What someone typed stays.** A field marked as edited that no source gave is theirs, and no scan changes it, whatever an NFO beside the file says. A field that's both edited and from a source is an old answer an earlier version read back from an NFO it wrote, which a better one can replace.

**Importing a big library** needs nobody watching. It runs in the background, and carries on from where it stopped after a restart: what's been judged is saved every 200 files, and what's been looked up as it goes. A file is only read again if it changes. On this project's test library of 8,500 files on a hard disk, the scan took 30 seconds and every game had a name and a checksum match within a minute, with online sources and pictures carrying on after that.

### Games in archives

`.7z`, `.zip`, `.rar` and `.tar` work wherever a console accepts a game file, and so does anything compressed with gzip, bzip2, xz or Zstandard, as one file (`Game.sfc.xz`) or as a tar (`.tar.gz`, `.tgz`, `.tar.bz2`, `.tar.xz`, `.tar.zst` and their short forms). That includes split sets (`.part1.rar` on, `.7z.001` on: all the parts stay together in one folder), and files whose extension is wrong for what they really are. An archive inside an archive doesn't count. Code: `internal/gamearchive`.

**Scanning** reads only an archive's headers, and only when the file is new or changed: what each one holds is kept in the database. A collection of about 2,000 archives takes about a second the first time. A compressed tar has no index, so a scan reads through the first 64 MB of one at most. The title comes from the archive's file name, with its tags (`Game (USA) [!].7z` is "Game", "USA").

**Which file:** a playlist first, then a `.cue` or `.gdi`, then `.ccd` or `.toc`, then any other game file for that console. Among several candidates, it prefers one named after the archive, then a verified dump in a widely-played region, and puts bad dumps and hacks last. A tie goes to the first in the archive.

| What's in it | What's unpacked | What the core opens |
|---|---|---|
| One game file, or several versions | The chosen file, as `game.<ext>` | `game.<ext>` |
| A disc, with its tracks | Everything but documents and pictures, since cores also want things no `.cue` names (`.sbi`, `.sub`) | The `.cue`, `.gdi` or `.ccd` under its own name, with the tracks beside it |
| Several discs of one game, and a playlist | The same | The playlist |
| Several discs of one game, and no playlist | The same | `game.m3u`, written then. A console that can't open one gets the first disc |

A `.cue`, `.gdi` or `.m3u` from an archive is checked before it's used: each file it names must have arrived. A name that differs only in case, or carries a path from the computer it was made on, is corrected in the unpacked copy. A name that matches nothing gives a clear error, which also means a list can't point a core at anything outside the game's folder.

**The cache** is `gaming/unpacked/<id>/` under the data folder, sized in Settings, Consoles & cores (16 GB by default). Nothing is ever written beside the original.

- A game is reused on the next play, and the longest unplayed is evicted when room is needed, except a game being played.
- The id comes from the file's path, size and time, and the console, so a changed archive is a new game.
- A game unpacks into `.tmp-…` and appears under its final name in one rename, after every file is synced to disk and a manifest of their sizes is written. Anything a crash leaves behind is cleared at the next start. A finished game is checked against its manifest each time it starts, and is unpacked again if it fails.
- Everyone asking for the same game shares one unpack. At most two run at a time, and the rest wait. One that nobody has asked about for two minutes is stopped.
- A game that won't fit, leaving 512 MB free, gets a clear error.

**Saves** don't depend on any of this: they belong to the account and the item (`u<id>/i<id>`), and the item is the archive's path in the library. A one-file game is always opened as `game.<ext>`, in case a core names files after what it opened.

**Starting** a game that isn't unpacked yet: `POST /games/{id}/prepare` starts the unpack and says how far it has got, and the player shows that. `POST /games/{id}/play` works without it, and just takes as long as the unpack does.

### Metadata and box art

Scans only list files. Details, ratings and pictures come along behind them in the background (`internal/library/games_meta.go`), and [Metadata](METADATA.md#video-games) has the sources, what each one is sent, and how games are matched. Your own files always come first: `Game.png`, `Game-cover.jpg`, `Game-poster.jpg`, `Game-fanart.jpg` and `Game.nfo` beside a game. With no box art, the library shows a tile made from the game's name and console. Native apps get all this through `GET /games/{id}/metadata`: see [the client API](CLIENT-API.md#game-details).

**Console icons** are the logo service's kinds `console` and `gamestudio`, downloaded only after an administrator agrees (`ConsoleMark.tsx`, and [the logo service](METADATA.md#the-logo-service)). Pages have to look right with text alone.

## Saves, states and resume

A state that arrives before the first frame waits for two frames that nobody sees. Some cores accept one earlier and then start the game from the beginning anyway.

Everything is stored per account under the data folder:

```
gaming/saves/u<user id>/i<item id>/save.srm       battery save
                                   auto.state     written when you stop; resumes next time
                                   state-1…99.state  manual slots, each with a .png picture and a .json
                                   undo.state     the game as it was before the last load
                                   autoprev.state the automatic save that starting over would have replaced
                                   patches/p<n>.patch  a person's patches
                                   system/             the system folder of a core that keeps saves in it
```

**Memory cards.** The PlayStation 2 core keeps its memory cards under its system folder, as `pcsx2/memcards`. A running core can't write to the shared system folder, and shouldn't see other people's cards there, so this core gets a system folder of its own, in the save folder (`SavesIn` in `traits.go`, `stream/ownsystem.go`). The cards stay there. Copies of the BIOS in use and the core's support files go in beside them when the game starts and are removed when it ends, up to 64 MB, so firmware doesn't end up in backups. They're copies, not links, because on Linux a core could write through a hard link to the real file. Each game has its own cards for each account. A new card is unformatted, as a new card was on a real console. This has been seen working on Windows, and not yet in Linux with the sandbox. Saves on Play! and Dreamcast cores are untested.

Paths are built only from numeric IDs and a slot number. The runner sends the bytes over its pipe, and the service writes them atomically, so a runner that has been exploited can't choose a path. Battery saves are written within 5 seconds of changing, and on stop. One game's folder can hold 4 GB for one account (`MaxSaveBytes`), and imports and patches can bring an account's total to 10 GB (`maxAccountGameBytes`). A save that would go over is refused with a message.

A person can download a state or a battery save and put one back. An exported state has a small header with the console, the core, its size and a checksum, and an import checks all of them. A plain state file from elsewhere is taken with a warning: it's only ever read by a core inside its sandbox. A state that won't load leaves the game as it was and says so, and Undo load is always there.

## Which version comes first

*Settings, Consoles & cores, Which version comes first* puts regions in order (`gamingRegionOrder`, such as `japan,europe`: `usa`, `europe`, `australia`, `japan`, `asia`, `brazil`, with any left out going after those, in that order). A country counts as its region, and a version for the world counts as the first. It decides three things:

- which file of an archive plays,
- which version leads a tile,
- which record names a game whose file says nothing about its region.

Changing it scans the games libraries again. A file someone chose stays. So does the version of a game someone has played, as their saves belong to it, or has given a title of their own. New versions that arrive later never take over from the one leading a tile.

## Settings

Settings are described once, as data, in `settings.go` in WittyPlay: a key, a kind, a range, a default, a label and any caution. The server checks values against that, and every client draws its controls from it (`GET /api/games/settings/schema`). A new setting is a line in that table, plus whatever uses it.

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

Administrators also set **limits**, which aren't defaults, because nobody can go past them: the fastest fast-forward (`gamingMaxSpeed`), rewind's memory per game (`gamingRewindMB`) and whether run-ahead is allowed (`gamingRunAhead`). Left alone they follow the machine: 4× and 32 MB, or 2× and 16 MB on one with fewer than four CPU threads.

Stored values are checked again when they're used, so a value that's stopped being valid is ignored and a game still starts. Reading settings never fails a start.

### Where each thing runs

| What | Where | Cost to a server |
|---|---|---|
| Picture effects, shape, crop, colour, bezels | The browser | None |
| On-screen numbers | The browser, asking `GET /play/{id}/stats` once a second while on | A few counters |
| Volume, mute, sound buffer | The browser | None |
| Fast-forward, slow motion, frame skip, frame advance | The runner | Fast-forward runs the core faster, and sends no more pictures than normal |
| Rewind, manual run-ahead | The runner | Real, so off by default and limited |
| Core options, cheats | The core, through its own interfaces | None |
| Patches | The server, once, when a game starts | A file read and write |

### The picture

`web/display.ts` and `web/layout.ts`. With no effect on, the transport's video element is on the page by itself. Colour is a CSS filter. Turning on an effect that needs a shader puts a canvas over the video and draws each frame on it the moment it's decoded, using `requestVideoFrameCallback`. The shader is our own, about 60 lines. Its only uniforms are numbers from the settings.

- **Without WebGL**, or when a context is lost, CSS lays scanlines and the grid over the picture and does colour. The context coming back restores the shader.
- **Flicker** is the one effect that changes on its own. It's capped at a small part of the brightness, is off by default, carries a caution, and is forced off under `prefers-reduced-motion`. Fast-forward carries a caution too.
- **A person's image** is decoded in Go, must be PNG, JPEG or WebP and within 16 MP and 16 MB, and only the decoded pixels are kept, as a PNG this server wrote. It's served as `image/png` with `nosniff` and a sandboxing CSP, to its owner only.

### How a game's speed changes

`host/control.go` stands between the runner's loop and the core.

- **Fast-forward** runs the core faster and sends at most the game's own frame rate. The core is told it needn't draw or mix (libretro's `GET_AUDIO_VIDEO_ENABLE`), and many cores then run a lot faster. It has no sound.
- **Slow motion** has no sound either: stretched sound would need resampling for a feature that's used for moments.
- **Frame skip** leaves frames out of what's sent, on request or, when set to automatic, only once the loop is more than a frame behind, and never more than three in a row.
- **Rewind** (`host/rewind.go`) keeps the newest state whole, and each older one as its difference from the one after. The oldest can then be dropped at any moment, which keeps it inside a byte limit however busy the game is. If taking a state costs over a quarter of a frame it takes them less often, and if it still doesn't fit it turns off and says so. Loading a state or resetting starts its history again.
- **Run-ahead** runs the real frame with its sound, saves, runs on with only the last picture kept, and goes back. If the game falls behind its schedule for a couple of seconds, it turns off, with a message unless it was automatic.

### Core options

A runner tells the server what a core offers as it starts: labels, allowed values and their names, descriptions, categories, and which options the core currently wants hidden. The newest libretro option format is read, and the older two for cores that only have those. The server keeps that per core, so settings screens can show it with no game running. For a core never run here, it starts one with no game for a moment to ask, boxed in as any game is. The runner refuses a key the core didn't offer and a value it didn't list, so nothing written in a setting can reach anything else.

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

- **It's data.** Nothing in it is run. A control can only set a core option. Values go through the same checks as everything else, and a core only takes what it listed.
- **It's hostile until proven otherwise.** 256 KB at most, 8 levels deep, 20,000 values, 64 controls, 32 presets. Sizes are checked before anything is set aside, and the parser walks tokens so that it can stop part-way. It's fuzz-tested.
- **Mistakes are shown where they are.** Errors stop it saving, with the line. Things it doesn't know, and core options a core won't take, are **ignored and listed**, never dropped silently.
- A controller can use a file's presets and controls. Writing one takes a keyboard.

### If a setting breaks a game

- A change to the picture only ever affects the browser, and one a browser can't do falls back.
- A running game is told about changes with a command it can refuse. The game isn't restarted.
- **Safe mode** (`POST /games/{id}/play` with `"safeMode": true`) starts with default settings, no cheats and no patches, and takes no setting changes until it's stopped. It's offered on the game's page, in the game menu, and whenever a game fails to start.
- A patch that won't apply leaves the game unpatched and playable, with a notice.
- Resetting a level is one button in the settings file screen.

### Shortcuts

`web/hotkeys.ts` works on a player's RetroPad buttons after the input layer has mapped them, so shortcuts follow a remapped pad. Holding the shortcut button (Select unless changed) and pressing another does the shortcut and keeps that button from the game until it's let go. Start and Select together, or Guide, always opens the menu. Hold actions let go by themselves if the window loses focus or a menu opens, because a stuck fast-forward would be worse than a lost press. Speed changes go by reliable HTTP, not the input channel, for the same reason. A keyboard shortcut gives way to a key a game button is using.

## Adding your own core

[Games](GAMES.md#your-own-core) has the steps for a person adding one. This is what's behind them (`internal/server/api_games_custom.go`, `web/src/pages/settings/GamingCustom.tsx`, `cores/custom.go`).

| Guard | How |
|---|---|
| Who | Administrators only, with an adult account. Adding, approving again and going back all ask for the password. Kids and guests can't reach any of it |
| Off by default | Two switches, both needed: `WMS_GAMING_CUSTOM_CORES=1`, `-gaming-custom-cores` or `"gamingCustomCores": true` on the machine, and one server-wide setting, which the general settings route can't change. `WMS_GAMING_NO_CUSTOM_CORES=1` beats both. Where `SandboxStrength` says a core can't be boxed in, it can't be turned on |
| Where it runs | Always in a runner process with the same sandbox as any game, listed under [Security](#security). Custom cores get no more rights than the cores we offer |
| Tested first | The first thing a file ever does is load in that sandbox, where a crash is an exit code and a hang is a timeout, after two minutes. Its answers are treated as hostile and cleaned before they're shown |
| The file | SHA-256 recorded when approved, and read in full before every game. It's the file as approved or it doesn't run |
| Sizes | 1.5 GB for an upload or download, 1 GB for a core, 4 GB unpacked, 20,000 files |
| Paths | Nothing a person or a file sends becomes a path to write to. File names that name folders are refused. A zip with any entry that climbs out, or any link, is refused whole. A placed file is chosen from the server's own list, and links aren't followed |
| Web addresses | `https` only, and only the public internet: checked when connecting, so a name can't resolve its way onto your network, and again at every redirect. No proxy, no passwords in addresses |
| Record | Every addition, change, removal, refused file and failure during play is in the history on that page and in the server's log |

What it can't do: the sandbox limits what a core can do to a machine, and it's as strong as the system allows. On Linux, some layers need a recent kernel, and the report says if one wasn't available. A core reads files like any program under the server's account, and can't send anything anywhere because it has no network. The SHA-256 check notices a changed file. It doesn't stop someone who can already write to the data folder.

**Licence shown:** from its info file, our list going by its name, or nothing at all, when it says **Unknown license**. Non-commercial cores are labelled, and are never offered by us. The list of what we offer is kept apart from custom cores in the code.

**A custom console** needs a name, a short name and its file extensions. The **JSON** tab takes the same thing:

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

Its ID starts with `custom-`. An extension another console already uses, like `.iso`, is only trusted in a library set to this console or a folder named after it, so nothing gets taken from a console that already works.

**A core's zip:**

```
something_libretro.dll      the core itself, at the top
something_libretro.info     its info file, optional
assets/...                  files only this core uses
system/...                  files that belong in the BIOS folder
```

Anything else in the zip is ignored. Files for the BIOS folder never overwrite one already there. Removing a core takes back only the files it added.

**Copy protection stays in force.** The server's own refusals, such as password-protected archives and encrypted discs, are unchanged, and a core is handed the game file it was asked to open and no keys.

## Clients other than a browser

Games have to work from TV apps as well, so nothing about a game is decided inside the web player:

- **The server holds** sessions, saves, resume, play time, each console's controller and default layouts, and everyone's saved layouts. They're plain JSON, and the web player reads them like any other client.
- **The protocol is standard, and one for every device:** WebRTC offer and answer over HTTP, data channels with documented messages, and a WebSocket carrying the same messages where UDP can't reach ([version 2](GAMING-STREAM.md)). A test (`TestNativeClientPlaysAGame`) plays a game both ways with no browser.
- **The server says which modes a client can use.** Anything only a browser could do is marked `browserOnly` and is never offered to an app.
- **Everything works with a D-pad.** A TV remote is a controller with its own default map, and Back opens the game's menu, which can press Start and Select for remotes that have neither.

## Controllers

Everything becomes one thing: a **RetroPad state** per player, which is 16 buttons, two sticks and how far two triggers are held. The libretro API speaks RetroPad, so each console needs only labels. These, and all the default layouts, are data from the server (`controls.go` in WittyPlay).

- **Gamepads:** standard mapping by position (bottom button is RetroPad B, right is A, and so on), which is how libretro's cores expect pads to be laid out.
- **Pads without standard mapping:** a browser that can't put a pad in order reports raw numbers. Our own table of common models by vendor and product (`layouts.go`) puts them in order. A D-pad reported as one axis, or as two, is found by looking at it, and a trigger reported as an axis that rests at one end counts from there. An unknown pad gets a first-to-last guess, and the screen says to check it.
- **Sticks and triggers:** the dead zone is a circle and the travel is rescaled from its edge. Triggers reach the core as how far they're held, for cores that ask. A value near the press point doesn't flicker.
- **Keyboard:** arrows, Z, X, A, S, Q, W, E, R, Enter and right Shift by default. Consoles with sticks add I, J, K and L. Remappable.
- **TV remote:** the D-pad, OK as A, Play/Pause as B, Back for the menu.
- **Hot-plug:** connecting a pad gives it the player it had last time on this device, or the next free one. Disconnecting one lets go of everything it held at once. Browsers that don't fire the events are found by the heartbeat.
- **Several players:** up to four (`MaxPlayers`). Moving a pad onto a player who's taken swaps them.
- **Turbo and rumble:** turbo per button, per profile. Rumble reaches the right player's pad with each of its two motors. The runner only passes on a motor's change. The player asks for a second at a time and renews it while it's wanted, so a page that goes away can't leave a pad shaking.
- **Device types:** a core's `SET_CONTROLLER_INFO` list is read, and each console's preferred one is plugged in: a DualShock on PlayStation where the core offers it. Players can choose another.
- **Touch:** an on-screen controller per console. The editor moves, resizes and sets the opacity of each control and chooses which ones show, saved per console and per device, for portrait and landscape separately. Sliding between buttons works, and Android gets haptics.
- **Touch sticks:** consoles with a stick get an on-screen analog one. On the Nintendo 64, the on-screen D-pad is that stick. On PlayStation, whose games nearly all use the D-pad, it starts hidden and the editor can show it.

Layouts are saved on the server: gamepads per console and model, keyboards per console, and touch layouts per console, device and orientation. They follow the account.

### How it stays cheap

Pads are read every 4 ms while one is connected, as browsers update them far more often than the screen refreshes (a TV stays with the screen), and sending happens only when a state changes. Reading allocates nothing, and a pad whose timestamp hasn't moved isn't read again. With no pad connected, or the page hidden, no frames are asked for at all, and all that's left is one heartbeat every quarter of a second.

### When someone looks away

A hidden page lets go of every button and stops rumble, because browsers stop updating pads for one and anything held would stay held. Losing the window's focus lets go of keys, which may be released elsewhere, but pads carry on, so a game on a second screen still works.

### What browsers don't allow

Most browsers only show gamepads to secure pages, and only once a button on them has been pressed. On plain `http://`, with no pad to be seen, the controller screen says so and what to do. Rumble needs a browser that has the feature and a pad that can. A TV app reads controllers itself and has none of these limits. None of this has been tried with a real controller.

### Profiles

People make their own named mappings, stored per account on the server ([the API](GAMING-API.md#controller-profiles)). Each can be limited to a controller model, a console and a game, and the most exact one in use wins: game, then console, then controller, then the default. Files of them can be exported and imported; an import is untrusted input, checked and limited like anything from a client.

Profile numbers are W3C standard numbers wherever the pad can be put in order, so one follows you between browsers. A pad that can't be stores its own numbers, marked `raw`, and such a profile applies only to that pad in that state.

### The controller screen

One screen, in the game menu and at `/controllers`, with three tabs: Buttons, Players and Background. Touch controls open from it.

- **The picture** (`web/art.tsx`) is our own drawing as inline SVG: a flat two-button pad, a four-button pad with shoulders, a six-button pad, a three-handled pad, a two-stick pad and a stick on a base. They're generic shapes, with no maker's logo and nothing traced from a product. Each console's `pad.art` picks one.
- **Its buttons are real `<button>`s** laid over the drawing, so remotes, keyboards and screen readers reach them. They light up as they're pressed, updated directly rather than by drawing again. The same mappings are always there as a plain list.
- **Remapping is by pressing:** choose a button, press what you want for it. What was already held doesn't count. Whatever was pressed isn't also taken for OK when the screen gets control back (`.wp-capture` makes `tv.ts` stand down). It stops waiting by itself, so a person with only a controller can't get stuck.
- **A source used twice is allowed, and shown** on both rows, with a Swap.
- **Everything works from a controller or remote:** steppers instead of sliders and drop-downs. Choosing a file can't be done on a TV, so importing and uploading are left out there.
- **Colours are the theme's**, in the pairs `scripts/theme-contrast.py` checks.

### The background picture

Uploads are hostile until proven otherwise. Only PNG, JPEG and WebP are decoded, whatever the file claims. The size in pixels is read from the header first (24 megapixels at most) and one picture is decoded at a time. It's drawn again as a new JPEG of up to 2560 pixels, which leaves behind everything but the picture. A photo stored sideways is turned upright first.

It's stored at `gaming/backgrounds/u<user id>.jpg`, from numbers only, and served to its owner with `nosniff` and the sandboxing policy every API answer has. No route takes an account number.

## Security

Treat a ROM as hostile and a core as buggy.

**The runner**

- Starts with an empty environment, in a working folder of its own, with only pipes open.
- Only the core the server picked runs: the request names a game, never a file. Core files come from the server's own list.
- **Linux:** new network, IPC and user namespaces where allowed; `no_new_privs`; a seccomp filter refusing sockets, `execve` and `ptrace`; Landlock limiting writes to its save folder, on every thread (the runner sets Landlock, then starts itself again so that all its threads inherit it); memory and file-size rlimits, on writable memory and with a much higher ceiling on address space, as real cores reserve far more than they use; killed when the server dies.
- **Windows:** a job object that kills it with the server, limits memory and forbids child processes, plus a low-integrity token so it can only write to its save folder.
- **macOS:** a `sandbox-exec` profile denying network, other programs and writes elsewhere, plus rlimits. If that tool is missing, only the rlimits. A Mac won't start one sandbox inside another, so the server starts WittyPlay's helper outside the sandbox it gives other helpers: inside it, every game failed as "The emulator crashed". Run on macOS 27 on Apple silicon: a Game Boy game on Gambatte and an NES game on FCEUmm started, and one played in a browser over WebRTC.
- **FreeBSD:** rlimits and priority, and it can't start programs, or open a socket with no graphics card (`hostsandbox/limits_freebsd.go`). That isn't a sandbox, and custom cores are refused. The limits were tried in a FreeBSD 15.1 virtual machine, and no game has been run there.
- **Games on a graphics card** open a little more: see [What the sandbox opens](#what-the-sandbox-opens-for-a-graphics-driver).
- A session is stopped after 60 seconds with no viewer and after 30 idle minutes. The number of sessions is capped.

**Archives**

- An archive is hostile input, and its decoders are other people's code. Everything that reads one runs in a throwaway worker: the server started again with an internal flag. It boxes itself in with the same layers as the runner (`internal/play/hostsandbox`), and has a memory limit of 4 GB and a lower priority. On Windows it uses a job object for the limit. A crafted header that makes a decoder panic ends the worker, and the server reports that one archive as damaged. A scan's worker that stops answering for 60 seconds is killed.
- A name that climbs out of the folder, is absolute, or names a drive or a device refuses the whole archive. Writing also goes through `os.Root`.
- Links, devices and folders in an archive are ignored. No link is ever created.
- At most 10,000 entries and 32 GB, with at most 2,000 times the archive's own size above 1 GB. Those are checked against the headers first. Then the bytes are counted as they're written, since headers can lie. Checksums are checked where the format has them.
- Encrypted archives get a clear message, and no password is ever tried. A `.7z` that keeps its names readable and locks only its contents can't be told from its headers, so it shows in the library and says so when played. The other kinds are reported by the scan: password-protected, damaged, oversized, unsafe.

**The service's side of the pipe**

- Everything the runner says is untrusted: every message has a size limit, dimensions are checked, and frames go to FFmpeg as raw bytes only.

**The API**

- Every endpoint checks sign-in. Starting a game goes through the same per-account access rules as any other item, and a session belongs to the account that started it.
- WebSockets check the Origin, as the rest of the API's CSRF rules do.
- Only admins can install cores. Downloads go to one fixed host over HTTPS, the core name comes from our own list, the size is capped, only the single expected file is unpacked, and it is written atomically with its SHA-256 recorded.
- Input packets have a fixed size, and players outside the session's limit are ignored.
- SDP offers are capped in size. One game takes at most 8 viewers and 4 connecting at once, and past that the oldest goes.
- Guest profiles can't upload anything to do with games. Kids profiles can bring their own saves and pictures, but not patches (`authz_gameuploads_test.go`).

Downloading a core means running someone else's native code on the server. That's why only an admin can do it, why each core's licence is shown first, and why the runner is sandboxed.

## Licences

| Thing | Licence | How it's kept apart |
|---|---|---|
| WittyPlay (runner, stream, web) | AGPL-3.0-or-later | Ours |
| pion/webrtc and its family | MIT | Linked into `wittyplay-module` |
| coder/websocket | ISC | Linked into `wittyplay-module` |
| bodgit/sevenzip, nwaples/rardecode and their family | BSD, MIT, Apache-2.0, MPL-2.0: see the third-party licences | Linked into the server. No GPL code |
| ebitengine/purego | Apache-2.0 | Linked into `wittyplay-game-runner` only |
| libretro API definitions | MIT | The constants and struct layouts only; no libretro code is copied |
| RetroArch | GPLv3 | Not shipped. Its core folder is read if present |
| Libretro cores | Each core's own | **Never bundled.** A separate file, installed on an admin's request, loaded by a separate process |
| FFmpeg | GPL builds | A separate program |

**Non-commercial cores** (`snes9x`, `genesis_plus_gx`, `picodrive`, `fbneo`, `mame2003*`) are not offered for download.

**ROMs and BIOS files are never bundled, downloaded or linked.** The console page lists the BIOS file names a core expects, so people can supply their own. `bios.go` copies what a core wants from a `BIOS` folder beside a game or one level up, to the system folder, leaving the originals alone. A PS2 BIOS is recognised by its name and size (`ps2BIOS` in `traits.go`). With a whole set, the server picks a late first-generation US model, then any US model, then European.

### Which cores are offered

Licences are from the [libretro core-info](https://github.com/libretro/libretro-core-info) files (MIT), checked on 2026-10-30. The table is `Consoles` in `consoles.go`.

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
| Atari 2600 | stella2014, stella | GPLv2 |
| Atari 7800, Lynx | prosystem, handy | GPLv2, zlib |
| WonderSwan, Neo Geo Pocket, Virtual Boy | mednafen_wswan, mednafen_ngp, mednafen_vb | GPLv2 |

If a game won't load on the chosen core, the other installed cores for that console are tried. Some cores start with settings other than their own defaults, where those don't suit streaming (`coreoptions.go`).

\* Draws with a graphics card. Every one of these exists on the build server for Windows and Linux amd64, checked on 2026-10-31. LRPS2 and PPSSPP also need support files, which the install fetches from the same host.

## Testing with real games

`cmd/wittyplay-check` in WittyPlay plays real games with no browser. It starts the real runner with its sandbox on, exactly as a game session does (`stream.StartRunner`), loads each game on a real core, and records what happened.

```
cd WittyPlay
go build -o /some/folder/ ./cmd/wittyplay-check ./cmd/wittyplay-game-runner
/some/folder/wittyplay-check -console nes -cores-dir /some/cores -install -out /some/report -jobs 8 /path/to/nes/games
```

For each game on each core it:

1. Loads it, and records how long that took and anything the core complained about.
2. Plays a set number of frames, pressing buttons to get past title screens, and on a console with a touch screen, tapping it too.
3. Saves a state, records what follows, goes back and checks the same pictures come again. Then it replays with buttons held, and a different picture proves input reached the game.
4. Quits the way the server does, with a save, and checks the process ended cleanly.
5. Starts a new process and restores the battery save and state the way a session does. The pictures must match, which is what proves a resume really resumes.
6. On a game with several discs, changes disc and back.

`-quick` stops after step 2, for sweeping a large collection.

A crash, hang or failure in one game only ends that game's check. The result is `ok`, `load-failed`, `crash`, `hang`, `blank` (only ever one colour), `frozen` (never changes), `shut-itself-down`, `skipped`, `refused` (an encrypted game, which nothing will try to open) or `too-slow-to-judge` (a busy machine couldn't run enough frames; `-continue` tries those again). Warnings say what else looked wrong: `slow`, `silent`, `no-audio`, `audio-rate`, `no-states`, `state-inexact`, `state-load-failed`, `no-input-effect`, `one-screen-blank`, `resume-*`, `quit-*`, `disc-change-failed`.

The report folder has `summary.txt`, `results.jsonl` with every number per game, each game's last picture, and `sheets/`, pages of pictures to look over, because wrong colours and garbled pictures are things only eyes catch. The tool refuses a report folder inside a git checkout, as the report names your games, and its terminal output uses numbers only.

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

`.zip` games are unpacked by the tool, and `.7z` needs a 7-Zip program.

Read speed and hangs only on a quiet machine: the first time a new build of the runner starts it can stall for many seconds. CPU numbers from Windows are too low for games that do little per frame, as Windows counts a process's time in ticks. Linux's are the ones to believe. Read `gameFps` beside `fps` in the server log.

### What has been tried

Run on 2026-10-31 on Windows, with the same cores checked on Linux where the table says so. "OK" means it loaded, drew a changing picture and quit cleanly. NES, SNES and PlayStation had full-speed runs on a quiet machine, and the speeds below come from those.

| Console | What was played | Result |
|---|---|---|
| NES | 318 real games on all 3 cores | fceumm 311 OK, Nestopia 314, Mesen 318. Every game plays on some core. fceumm runs at 100% speed, peak 25 MB |
| SNES | 1,861 real games, one dump of each, on Supafaust | 1,818 OK at 100% speed, peak 49 MB, no crashes or hangs. A sample of 187 with the full checks: 182 OK, and every state and resume in them good |
| SNES, others | The same 187 on Mesen-S; all 1,861 on bsnes | Mesen-S 180 OK. bsnes 1,642 OK on a busy machine. It wouldn't load 32, mostly games with chips whose firmware it wants, and 28 of those play on Supafaust |
| PlayStation | 1 loose game and 1 three-disc game | OK on PCSX ReARMed and Beetle PSX, with disc changes, resume and battery saves. PCSX ReARMed at 95% speed |
| Nintendo DS | 1 real game | OK on melonDS and DeSmuME, both screens counted, with states and resume. Speed not measured on a quiet machine |
| PSP | 1 real game | OK on PPSSPP, though a resumed picture doesn't quite match. Speed not measured on a quiet machine |
| 3DS | 1 real game, encrypted; a plain test program in a `.cia` | The real one is refused with a plain message, as it should be. The test program plays. No retail 3DS game has been played |
| Game Boy, GBA | 6 and 3 homebrew or test ROMs | OK on all cores. The GBA ROMs have no sound, so GBA audio is unproven |
| Mega Drive, Master System, Game Gear | 3, 3 and 1 homebrew ROMs | OK on all cores. ClownMDEmu's resume doesn't match on any of the 3; BlastEm, the default, is fine |
| PC Engine, Atari 2600, Virtual Boy, WonderSwan, Neo Geo Pocket | 3, 3, 3, 2 and 1 homebrew ROMs | OK on all cores |
| Lynx | 20 homebrew or test ROMs | 16 OK. The other 4 are test ROMs that draw one colour. All are silent |
| Atari 7800, Dreamcast | No game with a clear licence was found; a test program written for each (`wittyplay-probe test-rom`) | Both run through the stream with a changing picture. No real game has been played |

Every core offered at the time, 27 of them, installed and opened in the sandbox, on Windows, on Linux and in the container image. NES, SNES, PlayStation and homebrew samples also ran on Linux, where the sandbox is stricter.

**On Linux, under the sandbox, on 2026-11-01:** every core on every console, through the real session manager, runner, encoder and a viewer over the WebSocket, with what arrived put through a real decoder (`wittyplay-probe sweep`). Every one reaches frames with time passing. Every console held its full speed apart from these, on a machine with other work holding its load at 15 to 30: Beetle PSX HW at 22 frames a second and PPSSPP at 9 in a game's own menus, in both cases on this machine's only graphics driver, Mesa's D3D12 layer under WSL. PPSSPP's time goes on copying buffers inside that driver. It's the same at half the resolution, and a native driver hasn't been tried. About one time in four, Azahar's runner crashes on its way out, after everything is saved, at one place inside WSL's own `libd3d12core.so`. Nobody playing sees it, and PPSSPP's looks the same.

Mesen's NES core was reported to abort on Linux. It hasn't been made to, with all 318 real NES games under the full checks, in a real browser, through run-ahead, rewind, save slots, reset and fast forward, and on the earlier version it was reported on.

In a browser, on the real server, with pictures looked at: WebRTC with NES, SNES and PlayStation; WebSocket with H.264 and with JPEG; input, sound, phone size with the touch controls, a change of disc, save slots, a resume after quitting, and clean exits when the server or the runner is killed. Streaming through the module has played a real game in a browser, and the graphics-card consoles' delay figures are above.

**Nintendo 64 homebrew built with libdragon** only runs on ParaLLEl N64, which it's sent to whatever core is chosen, with its software renderer, the cxd4 RSP and the cached interpreter. No graphics card is needed. Mupen64Plus-Next can't run these: its recompiler ends the process, and its interpreters never make a frame. Without ParaLLEl N64, pressing Play says to install it. Newer homebrew can still ask for more than the core can do: `N64brew Game Jam 2024` stops on its own error screen. A custom core assigned to such a game is left alone.

**The SNES games that stay blank on Supafaust**, 43 of them: 7 play on another core. 13 are Satellaview dumps, and 2 of those play on Mesen-S with `BS-X.bin`. 7 are Sufami Turbo cartridges and 2 are data packs, which need a base cartridge plugged in. 6 aren't games: start-up programs of copiers and cheat devices. 8 others, some using chips whose firmware the cores want. These ran for 25 seconds on all three cores, so they aren't merely slow starters.

## Limits today

- **Delay figures** are headless on one machine, and not from glass to glass. PlayStation 2 is the slowest, at about 49 ms from a press to the screen. Firefox, Safari, phones, TVs and real networks haven't been run. Nor has a real controller or TV remote.
- **3DS and Dreamcast** aren't tested with a real game.
- **Graphics-card consoles** run on Windows and Linux only, and with OpenGL only. melonDS's own OpenGL mode produces no frames here, so DS games draw in software at native resolution.
- **PlayStation 2 on Linux:** a real game boots under the sandbox, on a software renderer here. A real save hasn't been tried under it, and nor has a graphics card.
- **Play!** reads a whole PlayStation 2 disc before its first frame, which on a slow disk or network share can look stuck for a minute or more. It stops with an error on a raw `.bin` with no `.cue`. LRPS2 plays those.
- **Deflate64 and LZMA inside a `.zip`** aren't read, and say so. Deflate, bzip2 and Zstandard are.
- **A `.7z` with locked contents** is only found out when played.
- **Cheats depend on the core.** libretro gives a frontend no way to know whether a core acted on a code. Some cores have no cheat support at all.
- **Slow motion and fast-forward have no sound.**
- **A core's option labels are its own**, and in English: this frontend tells a core its language is English.
- **Patches** work on games that are a single file, not on discs (`.cue`, `.chd`, `.iso` and so on).
- **A blank picture isn't noticed.** If a core won't load a game, the next is tried. If it loads and draws one colour, it stays there. 4 NES games on fceumm and 7 SNES games on Supafaust do this and play on another core, which an admin can pick.
- **Few games on some consoles.** Only NES and SNES have been tried with many real games. See [What has been tried](#what-has-been-tried).
- **Container image:** cores from the build server need Debian's C library rather than Alpine's, which makes the image larger.
- **No netplay.** Several players means several controllers on one device.
- **Plain http on a home network:** browsers limit some features to secure pages. Firefox hides gamepads, and WebCodecs is unavailable, which leaves WebRTC or JPEG. Reaching the server over https or `localhost` removes both limits.
- **Containers:** WebRTC needs UDP. Publish a fixed port set in Settings, Consoles & cores, and fill in the public address, or use host networking.

---

<sub>WittyMediaServer documentation</sub>
