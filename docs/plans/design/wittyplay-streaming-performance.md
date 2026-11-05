# Design: WittyPlay streaming, latency and resource allocation

> **Status on 2026-11-04**
>
> **In progress as a fault:** [WittyPlay#2](https://github.com/wittywilla/WittyPlay/issues/2). **Planned:** [streaming like the console](https://github.com/wittywilla/WittyMediaServer/discussions/54) and [instant start and hand-off](https://github.com/wittywilla/WittyMediaServer/discussions/55).

For WittyPlay's versions **0.3.8 to 1.0.0**. The work is spread over many small releases, and [WittyPlay's timeline](../modules/WittyPlay.md) says which version carries which part. [Section 17](#17-phases) maps each phase onto those versions. **Everything runs on the server and is streamed** (the owner, 2026-11-02): clients only decode, display, play sound and send input, and picture filters are made on the server. The owner:

> I need optimizations and more resource usage allocation. The games can be laggy with audio and feel, as well as glitchy with video and graphics, as well as hitching. Allow the user to select different streaming options as well as resource allocations (with limits that are set automatically and can be overrided by admin settings), as well as making this robust as humanly possible. I want this to be on par with [the best commercial cloud gaming service] (with how well the latency feels locally and remotely). Test as many games as you need. This is for a future version, so write out an extensive, industry leading design doc to reference later.

This is the reference document for that work. Nothing in it has been built or measured for this document. Figures marked **(0.6.0)** were measured during 0.6.0-alpha and are quoted from `docs/GAMING.md` and the games-streaming notes; everything else is a target or an estimate.

---

## Contents

1. [The bar](#1-the-bar)
2. [Where we are](#2-where-we-are)
3. [What the owner feels, and what probably causes it](#3-what-the-owner-feels-and-what-probably-causes-it)
4. [The target pipeline](#4-the-target-pipeline)
5. [Capture and the core](#5-capture-and-the-core)
6. [Encoding](#6-encoding)
7. [Transport](#7-transport)
8. [Decoding and display](#8-decoding-and-display)
9. [Sound](#9-sound)
10. [Input](#10-input)
11. [Streaming options people choose](#11-streaming-options-people-choose)
12. [Resource allocation](#12-resource-allocation)
13. [Remote play](#13-remote-play)
14. [Robustness](#14-robustness)
15. [Measuring it](#15-measuring-it)
16. [Testing many games](#16-testing-many-games)
17. [Phases](#17-phases)
18. [Licences and boundaries](#18-licences-and-boundaries)
19. [Questions for the owner](#19-questions-for-the-owner)

---

## 1. The bar

"On par with the best cloud gaming service, locally and remotely" means, in numbers we can measure:

| Measure | Definition | Local target (wired LAN) | Remote target (30 ms round trip, 1% loss) |
|---|---|---|---|
| **Press to picture** | Button down to the first frame showing the reaction leaving the browser for the display | 8/16-bit: game's own reaction + **≤ 12 ms**. 3D consoles: game's reaction + **≤ 20 ms** | Round trip + **≤ 25 ms** over the local figure |
| **Frame to screen** | A finished frame in the core to the browser handing it to the display | **p50 ≤ 10 ms, p99 ≤ 20 ms** | p50 ≤ RTT/2 + 12 ms |
| **Hitches** | Frames shown more than 1.5 intervals after the previous one, not caused by the game | **< 1 per 10 minutes** | < 1 per 2 minutes |
| **Sound behind picture** | Offset between a frame and its sound at the speaker | **≤ 20 ms** (0.6.0: 35 to 80 ms) | ≤ 30 ms |
| **Sound gaps** | Audible dropouts | **0 per 30 minutes** | < 1 per 10 minutes |
| **Picture faults** | Corruption, smearing, blocks after loss | **None** on LAN; any after loss heals within **100 ms** | heals within 150 ms |
| **Start** | Play pressed to first frame, warm | ≤ 2 s for cartridges, ≤ 4 s for discs | + 0.5 s |
| **Recovery** | Encoder or network fault to picture back | ≤ 250 ms (0.6.0: encoder death 0.25 s, hang 3.5 s) | ≤ 500 ms |

These are the project's own goals, not claims for anyone's marketing. "Least lag on the market" isn't claimed until a camera shows it (the 0.6.0 report said the same).

**What "feels" means.** People feel three things more than the median: **consistency** (a steady 30 ms beats a 15 ms that sometimes jumps to 80), **sound** (late or crackling sound makes everything feel laggy), and **hitches** (one stall a minute ruins a racing game). The plan weighs p99 and hitch counts above p50.

---

## 2. Where we are

From `docs/GAMING.md` **(0.6.0)**:

**Architecture.** `wittyplay-module` is a long-running service on `127.0.0.1` holding sessions, WebRTC connections (pion) and encoders. Each game runs in a sandboxed runner process (`wittyplay-game-runner`) that loads one libretro core with no network. Video and audio are encoded by **separate FFmpeg processes**, fed raw frames through pipes.

**Delivery, in order of preference:** WebRTC (H.264, or VP8 without H.264) with Opus; WebRTC drawn directly (`VideoDecoder` to a canvas, opt-in); WebSocket with JPEG per frame and PCM, as a last resort.

**Already done to cut delay:**

| Stage | What was done |
|---|---|
| Waiting for the core | A new press starts the next frame straight away (`host.waitForFrame`) |
| Encoder queue | One frame held at a time; a newer frame replaces a waiting one |
| Encoding | Fastest presets, no B-frames, no lookahead, NVENC `-delay 0`, `libx264` sliced threads, keyframe every second |
| Framing | FLV for H.264, IVF for VP8, so frame ends are known without waiting for the next frame |
| Browser jitter buffer | `playout-delay` header extension: 50 to 200 ms down to about 1 ms |
| Sound | Its own stream, sent evenly, clock jumps over gaps |
| Display | `PhaseLock` in `web/latency.ts` moves frame production to land just before the display wants it |
| Run-ahead | One frame by default on quick-state cores with 4+ threads |
| Latency modes | Fastest, Balanced, Steadiest, Automatic |
| Rate control | Follows the browser's reports; falls fast, rises after 20 calm seconds; **changing it means a new encoder, so changes are at least 5 s apart** |
| Bursts | A frame can't use more than a quarter of a second's bits |
| Lost pictures | A keyframe request **starts another encoder**, stopping the picture about 0.25 s; waits for the regular keyframe if due within 300 ms |
| Pixels | Frames up to 400 px wide doubled with nearest-neighbour before 4:2:0 encoding |
| Pads | Read every 4 ms while connected (12 ms in the server's web app after games-ui-2) |

**Measured (0.6.0, headless, one machine, RTX 3070 laptop, i7-12700H, NVENC, Edge with no window, CPU 20 to 60% busy):** N64 at 1280x960: about 4 ms core, 2 to the server, 5 to 7 encoding, under 1 network, 1 decode, 8 waiting for the display. Frame to screen 18 to 27 ms over a 65-minute soak; NES with sound and run-ahead 9 to 13 ms, press 10 to 14 ms. PS2 the slowest, about 49 ms. Sound 35 to 80 ms behind the picture. Nothing grew in memory over an hour.

**Known faults left (0.6.0):** PS2 shader cache can't be written under the sandbox, so it's rebuilt each run; Mesen's NES core aborts on Linux; melonDS refused by glibc 2.43's executable-stack rule and Play! aborting on load (games follow-up); PPSSPP 9 fps and Beetle PSX HW 22 fps at 2x under WSL (suspected Mesa's D3D12 layer, needs a real card); Azahar's runner crashing on exit about 1 in 4 under WSL.

**Not tried (0.6.0):** a real display with a camera, Firefox, Safari, phones, TVs, real networks, 3DS. (In-browser emulation with WASM was also not built; it's now off the plan for good.)

**Picture effects today** (0.6.0): colour is a CSS filter, and effects that need a shader are drawn on a canvas over the video in the browser (`web/display.ts`). Under the server-only rule they move to the server, before encoding, so every client shows the same picture ([WittyPlay 0.4.11](../modules/WittyPlay.md#0411--picture-filters-on-the-server--e-u)).

**Machine traps** (from memory notes): a new unsigned runner can stall 8 to 31 s on first launch (likely a security scan); other work at 100% CPU starves below-normal priority badly; read `gameFps` against `fps` to tell the game from the encoder.

---

## 3. What the owner feels, and what probably causes it

None of these has been confirmed for this document. Each is a hypothesis with a test that would confirm or kill it, to run in [Phase 0](#phase-0-see-everything).

### "Laggy with audio"

| Likely cause | Why | Test |
|---|---|---|
| Browser audio jitter buffer of 35 to 80 ms | Measured in 0.6.0. The picture's buffer is gone; sound's isn't | Measure A/V offset with a test pattern (a flash and a click on the same frame) |
| Opus frames of 20 ms (FFmpeg's default) | Each packet holds 20 ms; the receiver waits for whole packets | Compare 20, 10 and 5 ms frames |
| Sound through a separate FFmpeg process and pipe | Pipe buffering and FFmpeg's own queues | Timestamp samples in and packets out |
| Core sample rate vs output rate (e.g. 32040 Hz SNES, 44100 vs 48000) resampled in big chunks | Resamplers with large windows add delay | Log rates and chunk sizes per core |
| Drift between the core's clock and real time | A core running at 60.098 Hz against a 60 Hz world either gaps or piles up sound | Count clock jumps per minute |

### "Laggy feel"

| Likely cause | Test |
|---|---|
| Sound being late makes everything feel late (above) | A/V offset |
| Display wait (8 ms median in 0.6.0) on a fixed-refresh screen | Run with `PhaseLock` logs on a real 60 Hz and 120 Hz screen |
| The game's own frames of reaction on 3D consoles, without run-ahead | `wittyplay-probe react` per console |
| Wi-Fi on the client | Same test wired and on Wi-Fi |
| Pads polled at 12 ms in the server's web app vs 4 ms in the doc | Check which is live; measure press timestamps |
| Windows priority (runner at normal, competing with everything) | CPU load during play |

### "Glitchy video and graphics" (the owner's Gran Turismo screenshot, `Screenshot 2026-10-31 191627.png`)

| Likely cause | Test |
|---|---|
| Rendering fault in the core or its settings (PS2 hardware renderer, upscaling, wrong renderer for the GPU) | `wittyplay-probe frames -out` saves frames **before** the encoder: if they're broken, it's the core |
| GPU read-back racing the core's next frame (a torn or half-written frame read) | Same: frames before encoding; checksums of repeated frames |
| Packet loss without recovery: H.264 decoders smear until the next keyframe (up to 1 s) | Impair the network (`run -impair loss=2`) and watch for smears |
| Colour conversion (BT.601 vs BT.709, limited vs full range) | A colour bar test frame through the whole pipe |
| Encoder bitrate too low for a busy 3D scene (blocks) | Bitrate logs against the scene |

### "Hitching"

| Likely cause | Test |
|---|---|
| **Encoder restarts** on every bitrate change (at least 5 s apart) and on every keyframe request (0.25 s picture stop each) | Count restarts per session; correlate with hitches |
| **PS2 shader cache rebuilt every run** under the sandbox: shader compiles mid-game stutter | Hitch log per scene, first visit vs second |
| Go garbage collection in the service with per-frame allocations | `GODEBUG=gctrace=1`, pause times against hitch times |
| Other processes (transcodes, scans) taking the CPU | System load at each hitch |
| The core's frame pacing against the display's refresh (59.94 vs 60) | Frame interval histogram |
| Disc games reading slowly from a network drive or compressed image | Read times in the runner |
| Thermal throttling on laptops | CPU and GPU clock logs |

---

## 4. The target pipeline

```
 runner (sandboxed, no network)                 encoder host                 service                   browser / app
┌─────────────────────────────┐   shared    ┌───────────────────────┐    ┌──────────────┐  WebRTC  ┌───────────────────────┐
│ core runs frame N           │   memory    │ zero-copy where GPU   │    │ packetise,   │  (UDP)   │ decode (hardware)     │
│  (run-ahead if on)          │──frame────► │ hardware encode:      │──► │ pace, FEC,   │────────► │ present at phase      │
│ sound for frame N           │  + stamp    │  intra-refresh,       │    │ NACK/RTX,    │          │  (PhaseLock / direct) │
│ GPU frame stays on GPU when │             │  reference invalid.,  │    │ congestion   │          │ sound: small buffer,  │
│  core and encoder share it  │──sound────► │  live bitrate change  │──► │ control      │          │  rate-matched         │
│ input applied at frame start│             │ Opus 10 ms, low delay │    │              │ ◄─input─ │ pads polled 4 ms,     │
└─────────────────────────────┘             └───────────────────────┘    └──────────────┘ datachan │  stamped              │
                                                                                                    └───────────────────────┘
```

**Per-stage budget, local, 3D console at 60 fps:**

| Stage | 0.6.0 | Target |
|---|---|---|
| Input arrival to frame start | up to a frame, cut by "new press starts next frame" | ≤ 1 ms after arrival |
| Core | game-dependent (4 ms N64, most of a frame on PS2) | unchanged, except run-ahead and settings |
| Frame to encoder | ~2 ms (read-back, pipe) | **≤ 0.5 ms** zero-copy; ≤ 1 ms via shared memory |
| Encode | 5 to 7 ms NVENC at 1280x960 | **≤ 4 ms** (sliced output: first slice on the wire before the last is encoded) |
| Network, LAN | < 1 ms | < 1 ms |
| Decode | ~1 ms | ~1 ms |
| Wait for display | ~8 ms median | **≤ 4 ms** median with phase lock, ~0 with VRR |
| **Total, frame to screen** | 18 to 27 ms | **≤ 10 ms p50** |

---

## 5. Capture and the core

- **Shared memory, not pipes**, for frames and sound between the runner and the encoder host. A ring of three frame buffers in a memory mapping made by the service and handed to both; the runner writes, flips an index, and signals with an event (Windows) or futex/eventfd (Linux). No copies through the kernel. The sandbox already allows a private `/dev/shm` (melonDS fix), so this fits it; **to verify** on Windows job objects and macOS's sandbox profile.
- **Zero-copy for GPU cores** (PS2, N64, PSP, Dreamcast, 3DS, PSX HW): where the core renders with the same graphics card the hardware encoder uses, share the texture instead of reading it back:
  - Windows: D3D11 shared textures (cores rendering through Vulkan or GL use interop: `VK_KHR_external_memory_win32`, `WGL_NV_DX_interop`) into NVENC, AMF or Quick Sync.
  - Linux: DMA-BUF export from Vulkan or EGL into VAAPI or NVENC (via CUDA interop).
  - macOS: IOSurface into VideoToolbox.
  - Fallback: today's read-back, but asynchronous (a pixel buffer object or a staging texture a frame behind, so the core never waits on the GPU at read time; 0.6.0 already avoided waiting on every read for PS2).
- **Frame stamps:** every frame carries the monotonic time it finished, its frame number and the input sequence numbers it included, all the way to the browser, for measuring (see [Measuring](#15-measuring-it)).
- **Frame pacing:** the core runs on its own exact rate (60.0988 Hz NES, 59.94, 50 PAL), and the stream is told so. Phase lock follows the display; sound is resampled by up to ±0.5% to keep the core's clock tied to the display without gaps (the technique emulators use locally, applied end to end).
- **Shader caches that persist:** each GPU core gets a cache folder in WittyPlay's data, allowed by the sandbox as write-only for that core, so a scene compiles once per machine, not once per run. Pre-warm by replaying a short recorded input on first install, as an optional background task (shown in the activity panel).
- **Run-ahead and pre-emptive frames:** keep run-ahead automatic; add **second-instance run-ahead** (a second core instance for cores where state saves are cheap but the main instance has side effects in sound), and a per-game memory of which games react in one frame vs two, gathered by `wittyplay-probe react` and kept in the console tables.
- **Priorities:** on Windows, register the runner's main thread and the encoder host's threads with the Multimedia Class Scheduler (`AvSetMmThreadCharacteristics("Games")`), which gives them time ahead of background work without the starvation that below-normal caused. On Linux, `nice` plus cgroup `cpu.weight` (see [Resource allocation](#12-resource-allocation)); real-time scheduling only if the admin allows it and the kernel grants it.

---

## 6. Encoding

### An encoder host instead of FFmpeg on the command line

The biggest single change. Today each bitrate change and each keyframe request **starts a new FFmpeg**, because FFmpeg's command line can't be told mid-stream. That's two hitch sources by design.

**`wittyplay-encoder`**, a small program in WittyPlay's repository, one per session, sandboxed like the runner (no network, no files outside its folder):

- Links **libavcodec** (from the FFmpeg the server ships, dynamically, so licences stay as they are) and talks to hardware encoders directly where that's lighter: NVENC (Video Codec SDK headers, MIT), AMF (MIT), oneVPL for Quick Sync (MIT), VAAPI, VideoToolbox.
- Takes frames from shared memory or a shared GPU texture.
- **Changes bitrate live** (NVENC, AMF, oneVPL, VAAPI and VideoToolbox all support reconfiguring rate without a restart), so rate control can move every second instead of every five, and never hitches.
- **Heals loss without a keyframe:**
  - **Intra refresh** (a moving column of intra blocks) instead of periodic keyframes: no bitrate spikes, and a lost packet heals within the refresh period (set to about 100 ms on LAN).
  - **Reference picture invalidation** where the encoder supports it (NVENC): on a loss report, the next frame references a frame the client still has, so the picture heals in one frame with no keyframe.
  - **Long-term reference frames** as safe points.
  - Keyframes only on a new client or a decoder reset.
- **Slices:** output in slices, sent as each is done, so the network and decoder start before the encoder finishes the frame. (H.264 and HEVC slices; AV1 tiles.)
- Written in C or C++ (the encoder libraries are C), with a narrow message protocol to the service, fuzzed like the other parsers. Or in Go with cgo **inside WittyPlay only**: the server itself stays cgo-free, and WittyPlay already ships per-platform programs. **To decide** in Phase 2 by prototype.

### Codecs

| Codec | When | Notes |
|---|---|---|
| H.264 | Default; every browser and device decodes it in hardware | Constrained baseline or high profile, no B-frames |
| HEVC | Client decodes it in hardware (most phones, TVs, Apple, recent Windows with the extension) | Better quality per bit; Safari and Apple apps first. Browser support varies: detect, never assume |
| AV1 | Client decodes it in hardware and the server can encode it (RTX 40+, Arc, RX 7000) | Best per bit, screen-content tools; on the existing roadmap |
| VP8 | Fallback only | |
| **4:4:4** H.264/HEVC | Pixel-art consoles on a fast LAN, where the client decodes it | Sharp pixels without doubling; optional |

Chosen per session from what the client reports it decodes in hardware (WebCodecs `isConfigSupported` with `hardwareAcceleration: "prefer-hardware"`, or the app's own query), what the server can encode, and the person's choice.

### Quality

- **Content-aware rate:** 2D consoles need few bits; give the budget to 3D scenes. A per-console starting bitrate table, adjusted by rate control.
- **Pixel art:** keep nearest-neighbour doubling; add integer-scale to the client's size when the client is a TV, to avoid the client's own blurry scale.
- **Colour:** one path, BT.709 limited range tagged correctly, tested with a colour bar frame end to end on every encoder (0.6.0 already routes NVENC through the scaler for this reason).

---

## 7. Transport

WebRTC stays: it's the only low-latency path every browser has, and native apps can speak it too (`TestNativeClientPlaysAGame` already plays a game without a browser).

| Area | Today | Target |
|---|---|---|
| Loss recovery | Keyframe request (new encoder) | **NACK + RTX** retransmission on LAN (a retransmit costs ~1 ms); **FEC** (FlexFEC or ULPFEC) on lossy links, amount set by measured loss; reference invalidation in the encoder |
| Congestion control | Browser receiver reports, steps of 5 s | **Transport-wide congestion control** (transport-cc feedback, a delay-based estimator like Google Congestion Control) with live encoder bitrate; react within 100 ms to a queue building |
| Pacing | Frames sent as made | **Paced sending:** a frame's packets spread over a few ms, so a Wi-Fi access point's queue doesn't spike |
| Jitter buffer | `playout-delay` 0 | Keep; Automatic mode adds a small buffer only when arrival gets uneven (already the design) |
| Packet size | Default MTU | Probe path MTU; smaller packets on Wi-Fi to cut loss impact |
| Low-latency queues | None | **L4S / ECN** marking where the network supports it (later; modern routers and some ISPs) |
| Fallbacks | WebSocket + JPEG | Add **WebTransport (QUIC datagrams)** as a middle option for networks that block WebRTC's UDP handshake but allow QUIC; keep WebSocket+JPEG as the last resort, but upgrade it to H.264 over WebSocket with WebCodecs where possible (the roadmap's "fallback for networks that block UDP") |

---

## 8. Decoding and display

- **Hardware decode, always where available.** Report to the session overlay whether the client decodes in hardware, and warn if it doesn't.
- **Direct mode** (`VideoDecoder` to a canvas, bypassing the compositor's queue) measured with a camera on real screens; if it wins without tearing on most, make it the default where supported, with low-latency canvas (`desynchronized: true`).
- **Variable refresh rate:** where the display supports it (G-Sync / FreeSync, HDMI VRR on TVs), present frames as they arrive; phase lock becomes unnecessary.
- **Game mode on TVs:** the client apps ask the TV for its low-latency mode (HDMI ALLM where the platform exposes it: Android TV and some web views do; **to verify** per platform) and tell the person when the TV isn't in game mode.
- **Native apps:** Android with MediaCodec's low-latency decoding (`KEY_LOW_LATENCY`, Android 11+) and a `SurfaceView`; Apple with VideoToolbox and `AVSampleBufferDisplayLayer` with `preventsDisplaySleep` and minimal buffering. These can beat any browser, and the TV apps are where people sit on the couch.

---

## 9. Sound

Sound is likely the biggest "feel" win, and the cheapest.

| Change | Expected effect |
|---|---|
| **Opus in the encoder host, 10 ms frames** (5 ms on LAN if the client copes), application `lowdelay`, CBR | Removes the FFmpeg pipe; halves packet duration |
| **One clock for picture and sound:** both stamped from the core's frame clock | Offset becomes a fixed, known number |
| **Rate matching instead of jumps:** resample by ±0.5% to stay locked to real time, rather than jumping over gaps | No clicks, no drift |
| **Client buffer target** (playing sound only; nothing is emulated on the client): web: an AudioWorklet player with its own small ring buffer (target 15 to 25 ms) fed from the WebRTC track or from WebCodecs `AudioDecoder` in direct mode, instead of the browser's own jitter buffer; native apps: AAudio low-latency on Android, `AVAudioEngine` with a small IO buffer on Apple | 0.6.0's 35 to 80 ms → target ≤ 20 ms |
| **Sync picture to sound when they disagree:** delay the picture by the measured sound offset only if the offset is steady and the person chose "In sync" over "Fastest picture" | People choose: lowest picture lag, or lips-and-explosions in sync |
| **Bluetooth headphones:** detect (where the platform says) and warn that they add 100 to 200 ms | Honest expectations |

---

## 10. Input

- **Every input packet stamped** with the client's monotonic time and a sequence number; the service records arrival; the runner records the frame that applied it. That gives press-to-frame inside the server, per press.
- **Poll pads at 4 ms** (as the doc says) in every client, and send on change only, which 0.6.0 does. Check the web app's 12 ms timer from games-ui-2 against this: 12 ms was chosen to fix dropped presses on stalled frames, so measure the cost on low-end boxes before changing it.
- **Redundancy:** each packet already carries the full pad state; add the last two states, so a burst of loss still delivers every press and release in order.
- **Touch and keyboard** go through the same path and stamps.
- **Input prediction is not used** for games (it changes outcomes). Run-ahead is the honest version.

---

## 11. Streaming options people choose

The owner asked for "different streaming options". Presets first, with every knob behind **Custom**:

| Preset | For | Settings |
|---|---|---|
| **Automatic** (default) | Everyone | Picks from the measured network and client; moves within limits as conditions change |
| **Competitive** | Fighting, rhythm, racing on a good LAN | Lowest latency: direct mode, no jitter buffer, sound "fastest", run-ahead on, high bitrate, intra refresh fast |
| **Balanced** | Most play | Small adaptive buffer, sound in sync |
| **Quality** | Slow games, big TV | Higher resolution and bitrate, 4:4:4 for pixel art if possible, small buffer allowed |
| **Data saver** | Mobile data, weak Wi-Fi | Bitrate cap, 30 fps for slow games, AV1/HEVC if possible |
| **Custom** | Tinkerers | Every row below |

**Custom knobs:**

| Knob | Choices |
|---|---|
| Resolution sent | Native, 2x, 3x, 4x, 720p, 1080p, 1440p, 4K, Match my screen |
| Internal render scale (3D cores) | 1x to 4x, or Automatic by GPU budget |
| Frame rate | Game's own, 30 cap, 60, 120 (for cores that can) |
| Codec | Automatic, H.264, HEVC, AV1, VP8 |
| Colour | 4:2:0, 4:4:4 where supported |
| Bitrate | Automatic, or a cap from 2 to 100 Mbit/s |
| Loss protection | Automatic, Off (LAN), Light, Strong |
| Latency mode | Fastest, Balanced, Steadiest, Automatic (existing) |
| Sound | Fastest picture, In sync |
| Drawing | Normal, Direct (may tear), VRR if the screen supports it |
| Run-ahead | Automatic, Off, 1, 2 frames (within the admin's limit) |
| Stats overlay | Off, Simple, Detailed |

Kept per profile and per device (a phone and a TV want different things), with per-game overrides, in the same settings levels WittyPlay already has.

**The stats overlay** ("Detailed"): fps sent and shown, frame to screen p50/p99, press to picture (from stamps), bitrate, loss, retransmits, FEC recovered, encoder and decoder in use and whether hardware, sound offset, hitches this session. The same numbers go to the session's log, so a report can include them.

---

## 12. Resource allocation

The owner asked for resource allocation "with limits that are set automatically and can be overrided by admin settings".

### What's limited

| Resource | Why | How it's held |
|---|---|---|
| CPU | Software cores, encoding in software, Opus, the service | Linux: cgroup v2 `cpu.weight` and `cpu.max` per session; Windows: a Job Object with CPU rate control; FreeBSD: `rctl`; macOS: priority only (no hard limits) |
| GPU | 3D cores and hardware encoding | Hardware encoder sessions counted (consumer cards limit concurrent encode sessions: read the limit from the driver where possible, or probe); render scale lowered first when the GPU is busy |
| Memory | Runners are large (956 MB for an N64 session at 0.6.0) | Job Object memory limit on Windows, cgroup `memory.max` on Linux, rlimits elsewhere |
| Network | Upload from the server is often the limit remotely | A total upload budget shared across sessions and other streaming |
| Encoder sessions | Hardware limits | A pool, with software encoding as overflow if CPU allows |

### Automatic limits

On install and after a hardware change, a short **hardware probe** (a background task, shown in activity) measures: CPU threads and a single-thread score, GPU model and encoder capabilities (which codecs, how many sessions, how fast at 1080p), memory, and upload speed (optional, admin-run). From that:

| Tier (examples) | Default limits |
|---|---|
| **Low** (4 threads, no hardware encoder, 4 GB) | 1 session; 2D consoles at 2x; 3D consoles allowed but warned; software H.264 with sliced threads; run-ahead off |
| **Mid** (6 to 8 threads, iGPU encoder, 8 to 16 GB) | 2 sessions (today's default); 3D at native; hardware encode |
| **High** (12+ threads, discrete GPU, 32 GB) | 4 sessions; 3D at 2x to 3x; AV1 if supported |

These are starting points; the probe's numbers decide, not a model name.

### Admin overrides

**Settings → Games → Resources**, admins only:

- Sessions at once (server-wide and per profile)
- Per session: CPU share, memory ceiling, highest render scale, highest resolution and frame rate, highest bitrate
- Hardware encoder: which one, how many sessions
- **Priority against other work:** Games first, Even, Transcodes and scans first. With "Games first", scans pause and transcodes slow while a game runs (the existing transcode limit and the task registry make this possible)
- Everything shows the automatic value beside it, and **Back to automatic**
- Limits are ceilings: players choose within them (as `gamingMaxSpeed`, `gamingRewindMB` and `gamingRunAhead` already work)

### Admission and graceful degradation

When a new session would go over a limit, the scheduler tries, in order: lower a running session's render scale; lower its frame rate for games that tolerate it (never below the game's own rate for action games without asking); use software encoding if CPU allows; then **refuse with a clear message** ("The server is running 2 games, its limit. Ask an admin to raise it in Settings → Games → Resources."). The 0.5.2 audit item "refusing patches early when game sessions are full" belongs here.

---

## 13. Remote play

- **The path:** WebRTC needs UDP. Through the router path in Remote Access, open a UDP port range for games (documented, opt-in), or use a **TURN relay** on the server's own HTTPS port (TURN over TLS: works through most networks at the cost of TCP behaviour). The hosted relay, when it exists, can carry TURN.
- **Adapt fast:** remote links are where congestion control, FEC and live bitrate pay off. Targets in [The bar](#1-the-bar).
- **Honest numbers before playing:** a 3-second probe on start shows the round trip and loss, and what that means ("About 45 ms of extra delay: fine for most games, hard for rhythm games").
- **Mobile networks:** Data saver preset suggested automatically on metered connections (the Android app already caps metered connections at 8000 kbps).

---

## 14. Robustness

"As robust as humanly possible":

- **Watchdogs everywhere** (today: encoder death and hang). Add: runner frame stall (no frame for 2 s while not paused → save state, restart the runner, restore: the person sees a 1-second pause, not a dead game), sound stall, network stall (reconnect with ICE restart without leaving the game).
- **Never lose progress:** battery saves written on a timer and on every pause, plus an automatic state every few minutes (kept as the "resume" point), so any crash costs seconds of play at most.
- **Crash reports, local:** a runner or encoder crash writes a small report (core, game hash, last frames' stats, no file names) to the server's log, and the game page offers "Report this" which builds a sanitised bundle (WittyLog's sanitise switch).
- **Soak tests in CI on Linux** (an hour per tier, nightly), plus the existing 65-minute soak style: memory, handles and goroutines must not grow.
- **Chaos tests:** `wittyplay-probe run -impair` already simulates delay, jitter, loss and bandwidth; add bursts, reordering, a link that drops for 2 s, and CPU contention (a background load generator at 100%).
- **Fuzzing** of every message parser between processes, as the settings files are fuzzed.
- **No per-frame allocation in Go** in the service's hot path (sync.Pool buffers, preallocated rings), checked with an allocation test, so GC can't hitch.

---

## 15. Measuring it

### Automatic, every session

- **Stamps end to end:** frame finish time (runner), encode start and end (encoder host), send (service), receive and decode (client, via `requestVideoFrameCallback` and WebRTC stats), presentation estimate. Clocks aligned with an NTP-style exchange at session start.
- **A frame watermark** for tests: a small block in a corner encodes the frame number (as `wittyplay-probe` could), read back in the browser from the decoded frame to match frames exactly, even through encoder reordering.
- **Hitch counter:** intervals over 1.5× the game's frame time, split by cause (game slow, encoder, network, client).

### By camera, on real screens (the owner's help needed)

- **Press to photon:** a phone filming at 240 fps sees both the controller and the screen; press a button with a visible LED (or a small rig that lights an LED on press), count frames to the screen's reaction. Do it for each preset, on a TV in game mode and a desktop monitor, wired and on Wi-Fi.
- **The same rig against the console running locally in an emulator on the same TV**, as the reference. "On par" means the streamed figure is within a frame or two of local play.

### Reporting

Numbers only in reports, no game names in public files (as `wittyplay-check` does). p50, p95, p99, hitch counts, sound offset, for each console, core, encoder, client and network.

---

## 16. Testing many games

The owner: "Test as many games as you need." Scheduled for this major, not done for this document.

- **Tool:** `wittyplay-check` already loads real games headless through the real sandbox, presses buttons, checks states, resumes and disc changes. Add a **streaming mode**: the same, through the full pipeline to a headless browser, recording the measures above per game.
- **Matrix:**
  - Every console in the table, every offered core.
  - Per console, the most demanding games in the owner's library (heavy 3D scenes, fast scrolling, lots of sound channels) plus a few known-quiet ones as a baseline.
  - Encoders: NVENC, Quick Sync, AMF, VAAPI, VideoToolbox (needs a Mac), libx264, OpenH264.
  - Clients: Edge/Chrome, Firefox, Safari (needs a Mac or iOS), Android phone and TV apps, the desktop app, Apple apps (once compiled).
  - Networks: wired, Wi-Fi (simulated with `-impair` profiles from real Wi-Fi traces), 4G/5G profiles, 30, 60 and 100 ms remote.
  - Server tiers: low (a 4-thread mini PC: the owner's hardware or a VM capped to 4 threads), mid, high (this machine's RTX 3070 laptop).
- **On this machine,** remember the traps: first launch of a new runner can stall; other chats load the CPU; WSL's D3D12 layer makes some GPU cores slow, which a real card won't. A real Linux machine with a real GPU, or Windows directly (with the owner clicking any firewall prompt), gives the true GPU figures.
- **The owner plays** on a real screen for the feel check, and films with the camera rig.

---

## 17. Phases

Each phase is spread over several small WittyPlay versions, each its own milestone with an issue per item ([../GITHUB-PRACTICES.md](../GITHUB-PRACTICES.md)), landing on the alpha channel as it's ready.

| Phase | WittyPlay versions |
|---|---|
| 0. See everything | 0.3.8, 0.3.9 |
| 1. Quick wins | 0.3.6 (faults, shader cache), 0.4.0 (sound), 0.4.1 (smoothness), 0.4.2 (NACK/RTX), 0.4.11 (server-side filters) |
| 2. Encoder host | 0.5.0 to 0.5.10 |
| 3. Transport and sound | 0.6.0 to 0.6.4, plus 0.5.7 (HEVC/AV1) and 0.5.8 (4:4:4) |
| 4. Options and resources | 0.7.5, 0.7.6 (presets, per device), 0.8.0 to 0.8.8 |
| 5. Remote | 0.6.4 to 0.6.10 |
| 6. Native clients | 0.9.0 to 0.9.9 |
| 7. Prove it | Each series' release candidate, and 1.0.0 |

### Phase 0: see everything

Stamps end to end, the watermark, the hitch counter with causes, the stats overlay, and `wittyplay-check` streaming mode. Then run the matrix once to confirm or kill every hypothesis in [section 3](#3-what-the-owner-feels-and-what-probably-causes-it). **Nothing else starts until the numbers say where the time and the hitches go.**

### Phase 1: quick wins (no new architecture)

- Persistent shader caches (PS2 first).
- Opus at 10 ms, `lowdelay`; sound rate matching.
- MMCSS on Windows; cgroup weights on Linux.
- No per-frame allocation in the service.
- NACK + RTX on LAN.
- Fix the known faults: Mesen on Linux, melonDS and glibc's exec-stack rule, Play! on load, Azahar on exit.

### Phase 2: the encoder host

Shared-memory frames, live bitrate, intra refresh, reference invalidation, slices; then zero-copy GPU frames per platform (Windows NVENC first, as that's the owner's machine and the most common gaming PC).

### Phase 3: transport and sound

Transport-wide congestion control, paced sending, FEC by measured loss, AudioWorklet sound player with a small buffer, HEVC and AV1 negotiation, 4:4:4.

### Phase 4: options and resources

Presets and Custom, per device; hardware probe and tiers; the scheduler with admission and degradation; the Resources settings page; priority against transcodes and scans.

### Phase 5: remote

TURN on the HTTPS port, UDP range option, the start-up probe, WebTransport fallback, H.264-over-WebSocket fallback.

### Phase 6: native clients

Low-latency decoding and sound on Android and Apple; TV game mode; direct presentation.

### Phase 7: prove it

The camera rig with the owner; the full matrix again; publish the numbers honestly, with what wasn't tried.

---

## 18. Licences and boundaries

- WittyPlay stays Apache-2.0. Cores (mostly GPL) keep running in the separate runner process, as now. No non-commercial cores offered. No ROMs or BIOS files bundled, downloaded or linked.
- The encoder host links libavcodec dynamically from the FFmpeg the server already ships, and hardware SDK headers with permissive licences (NVENC headers, AMF, oneVPL: MIT; check each when used). Show each in Settings → About.
- pion (MIT) stays the WebRTC stack. A TURN server, if added, from a permissive library (pion/turn, MIT).
- Encrypted games stay refused (the 3DS rule is unchanged).
- WittyCouch and WittyCouchHost (game PCs, GPL-3.0 host) are separate; anything learned here that applies to them is planned in [../modules/WittyCouch.md](../modules/WittyCouch.md), keeping the GPL boundary.

## 19. Questions for the owner

1. **Benchmark hardware:** what low-end machine should "Low" be measured on? A mini PC, an old laptop, a Raspberry Pi 5 (arm64 is a server platform)?
2. **Real screens:** which TV and monitor do you play on most, and do you have a phone that films at 240 fps for the camera rig?
3. **Remote:** is opening a UDP port range on your router acceptable for testing, or should remote testing go through TURN only?
4. **Defaults:** should Automatic lean towards lowest latency or towards steadiness when it can't have both?
5. **Priority:** when a game and a transcode compete, should games win by default?

---

<sub>Future plans, written 2026-11-02. Not built, not measured for this document.</sub>
