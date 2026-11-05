# WittyPlay: the road to 1.0

> **Status on 2026-11-04**
>
> **Faults open:** [WittyPlay#2](https://github.com/wittywilla/WittyPlay/issues/2) to [WittyPlay#8](https://github.com/wittywilla/WittyPlay/issues/8). **Planned:** the series below are ideas ([insert animations](https://github.com/wittywilla/WittyMediaServer/discussions/51), [core store](https://github.com/wittywilla/WittyMediaServer/discussions/52), [disc sets](https://github.com/wittywilla/WittyMediaServer/discussions/53), [streaming](https://github.com/wittywilla/WittyMediaServer/discussions/54), [seamless sessions](https://github.com/wittywilla/WittyMediaServer/discussions/55), [playing together](https://github.com/wittywilla/WittyMediaServer/discussions/56), [more consoles](https://github.com/wittywilla/WittyMediaServer/discussions/57)). WittyPlay has no milestones on GitHub yet.

The owner, 2026-11-02:

> I want to build an industry leading app, not a wimpy patch-job of an emulation engine for web browser and mobile app / tv app usage. This needs to be all ran on the server and streamed, nothing in browser. Include granular updates and versions to accomplish the goal of being as seamless as [the leading commercial cloud gaming service] but with a better interface. You need to make a ton more incremental versions with incremental updates. Make it expansive please.

This is that plan: about seventy small, numbered releases from **0.3.5** to **1.0.0**. Each one has a goal, what's in it, and a line saying when it's done. Each ships on its own, on the alpha channel, so progress shows every few weeks instead of once a year.

Nothing here has been built or tested. Figures marked **(0.6.0)** were measured during 0.6.0-alpha; everything else is a target. The detailed engineering is in [the streaming design](../design/wittyplay-streaming-performance.md), the animations in [the insert animation design](../design/console-insert-animation.md), and signing in [Smart App Control](../design/windows-smart-app-control.md).

**Now:** WittyPlay 0.3.4 (published 2026-11-02). Helper and service, module API 3, `maxServer: 0.6.x`.

---

## Contents

- [Principles](#principles)
- [The bar: a seamlessness scorecard](#the-bar-a-seamlessness-scorecard)
- [Tracks](#tracks)
- [The timeline at a glance](#the-timeline-at-a-glance)
- [0.3.x: stabilise and measure](#03x-stabilise-and-measure)
- [0.4.x: quick wins and the play experience](#04x-quick-wins-and-the-play-experience)
- [0.5.x: the encoder host](#05x-the-encoder-host)
- [0.6.x: transport and remote play](#06x-transport-and-remote-play)
- [0.7.x: seamless sessions](#07x-seamless-sessions)
- [0.8.x: resources](#08x-resources)
- [0.9.x: thin native clients](#09x-thin-native-clients)
- [0.10.x: the dashboard](#010x-the-dashboard)
- [0.11.x: together](#011x-together)
- [0.12.x: more consoles](#012x-more-consoles)
- [1.0.0: proven](#100--proven)
- [Server and module API needed](#server-and-module-api-needed)
- [WittyPlayCores timeline](#wittyplaycores-timeline)
- [Removed from the plan](#removed-from-the-plan)
- [How each version is run](#how-each-version-is-run)

---

## Principles

1. **The server runs everything.** Every game, every core, every picture filter, every save, every rewind runs on the server. Clients do four things only: **decode the picture, show it, play the sound, send input.** Nothing is emulated in a browser, on a phone or on a TV.
2. **Every screen gets the same game.** Because the server makes the picture (filters included), a TV app, a phone and a browser show exactly the same thing. A new client only needs a decoder and a controller.
3. **Latency is felt as consistency.** A steady 30 ms beats a 15 ms that sometimes jumps to 80. Plans weigh p99 and hitch counts above medians, and sound as much as picture.
4. **Seamless means nobody thinks about it.** No setup screen before playing, start in about two seconds, pick up on any device where you left off, survive a dropped network without losing the game, adapt to the network without asking.
5. **The interface is the product.** The leading cloud service streams well, and its dashboard is a store front. Ours is the person's own collection, shown like a console dashboard: their saves, their discs, their history, their controllers, their screens.
6. **Your hardware, measured honestly.** Automatic limits from a hardware probe, admin overrides, and numbers in plain words. No marketing claims until a camera proves them.
7. **The standing rules hold.** Encrypted games refused. No ROMs or BIOS bundled, downloaded or linked. No non-commercial cores. Cores run in the sandboxed runner, apart from WittyPlay's Apache-2.0 code. Everything signed so Windows' Smart App Control runs it. Every platform in the owner's list counts.

---

## The bar: a seamlessness scorecard

What "as seamless as the leading commercial cloud gaming service" means, as things to measure. Every version that touches a row says which number it moves.

| # | Measure | 0.6.0 | Target by 1.0 |
|---|---|---|---|
| 1 | **Play to first frame**, cartridge, warm server | not measured | ≤ 2 s; **≤ 1 s** when resuming a suspended game |
| 2 | **Play to first frame**, disc game | not measured | ≤ 4 s |
| 3 | **Frame to screen**, LAN, p50 / p99 | 18 to 27 ms (N64, headless) | **≤ 10 / ≤ 20 ms** |
| 4 | **Press to picture**, beyond the game's own reaction, LAN | ~10 to 14 ms (NES, headless) | ≤ 12 ms (8/16-bit), ≤ 20 ms (3D) |
| 5 | **Remote, 30 ms round trip** | not measured | round trip + ≤ 25 ms over LAN |
| 6 | **Hitches** not caused by the game | not counted | < 1 per 10 minutes on LAN |
| 7 | **Sound behind picture** | 35 to 80 ms | ≤ 20 ms |
| 8 | **Recovery** after loss or encoder fault | 0.25 s (encoder death), 3.5 s (hang) | ≤ 100 ms after loss, ≤ 250 ms after a fault |
| 9 | **Network drop** of 10 s | session lost (not tested) | game paused and resumed, nothing lost |
| 10 | **Move to another device** mid-game | not possible | ≤ 3 s, same moment of play |
| 11 | **Questions asked before playing** | some (cores, BIOS) | none, after the folder is chosen once |
| 12 | **Controller just works** | most pads mapped | every common pad, prompts matching the pad, on every client |
| 13 | **Game list opens** with 10,000 games | not measured | ≤ 300 ms |
| 14 | **Picture faults** on LAN | seen (Gran Turismo screenshot) | none; heal within 100 ms after loss |

---

## Tracks

Every version belongs to one or more tracks. The letter appears beside each version.

| Track | Letter | Covers |
|---|---|---|
| Engine | **E** | Runner, capture, encoder host, zero-copy, codecs |
| Transport | **T** | WebRTC, congestion control, loss recovery, fallbacks, remote |
| Audio | **A** | Sound capture, Opus, sync, client buffers |
| Input | **I** | Pads, keyboards, touch, rumble, gyro, mapping |
| Cores | **C** | WittyPlayCores, the core store, overrides, consoles |
| Library | **L** | Import, matching, metadata, discs, BIOS checks |
| Interface | **U** | Every screen, menus, presets, accessibility |
| Sessions | **S** | Start, suspend, resume, hand-off, reconnect, saves, queues |
| Resources | **R** | Probe, tiers, limits, scheduler, priorities |
| Native clients | **N** | Android, Android TV, Apple, desktop, other TVs |
| Together | **G** | Co-op, spectating, clips, recordings |
| Quality | **Q** | Measuring, testing, soak, chaos, the camera |
| Platform | **P** | Signing, packaging, the module API, licences |

---

## The timeline at a glance

```
0.3.5 ─ 0.3.9    stabilise and measure          signed cores, known faults, stamps, baseline
0.4.0 ─ 0.4.12   quick wins, play experience    sound, smoothness, animations, core store, discs, server filters
0.5.0 ─ 0.5.10   encoder host                   shared memory, live bitrate, intra refresh, zero-copy, HEVC/AV1
0.6.0 ─ 0.6.10   transport and remote           congestion control, FEC, TURN, fallbacks, reconnect
0.7.0 ─ 0.7.9    seamless sessions              instant start, suspend, hand-off, queue, presets, saves
0.8.0 ─ 0.8.8    resources                      probe, limits, scheduler, degradation, priorities, multi-GPU
0.9.0 ─ 0.9.9    thin native clients            protocol 2, Android, TV, Apple, desktop, other TVs
0.10.0 ─ 0.10.9  the dashboard                  home, game pages, quick menu, profiles, onboarding, accessibility
0.11.0 ─ 0.11.6  together                       co-op, spectate, clips, recordings, game nights
0.12.0 ─ 0.12.5  more consoles                  every console a permissible core allows
1.0.0            proven                         camera-measured, every client on real hardware
```

Tracks overlap: Quality runs through every version, and the 0.10.x interface work can start beside 0.5.x if hands allow, since they touch different code. The order above is the order of dependency, not of calendar.

---

## 0.3.x: stabilise and measure

Server 0.6.x, module API 3. The goal of the whole series: **nothing broken, everything signed, and numbers for everything**, so every later version is judged against a baseline instead of a feeling.

### 0.3.5 · Signed and pinned cores · C P

- **Goal:** games start on a Windows PC with Smart App Control On.
- WittyPlay reads the core list from **WittyPlayCores** (pinned upstream commits, built in CI, signed, hashes in a list signed by our release key) instead of the libretro build server's nightly "latest" folder. Needs the owner's decision on distributing core binaries.
- Every Windows program of WittyPlay signed in CI ([pipeline](../design/code-signing-pipeline.md)).
- Before installing any core or custom core on Windows, check its Authenticode signature; if Smart App Control is On and the file isn't signed by a trusted publisher, refuse with a plain explanation.
- A run-time block (`ERROR_SYSTEM_INTEGRITY_POLICY_VIOLATION`) is reported as "Windows blocked <file>", not as a crash.
- **Done when:** in the CI job under Microsoft's Smart App Control policy, every offered core loads and runs a few frames; an unsigned test core is refused before install.

### 0.3.6 · Known faults · E C

- Mesen's NES core aborting on Linux.
- melonDS refused by glibc 2.43's executable-stack rule.
- Play! aborting on load.
- Azahar's runner crashing on exit (about 1 in 4 under WSL; check on a real GPU).
- **PS2 shader cache** persisted per core in WittyPlay's data, write-only for that core under the sandbox, so scenes compile once per machine, not once per run.
- A real PS2 save on Linux under the sandbox; Play! and Dreamcast saves.
- **Done when:** `wittyplay-check` passes every offered core on Linux and Windows, and a PS2 scene visited twice hitches only the first time ever.

### 0.3.7 · Finish what 0.6.0 sent back · U L

- PlayStation, PS2 and NES logos (logo service, or our own plain console shapes when logos are off).
- Controller mapping and game settings screens.
- PS2 memory cards, typed title kept on upgrade, region setting, activity panel for game scans (one bar with named phases).
- Refusing patches early when game sessions are full.
- **Remove "games in the browser" and "on-device play"** from `docs/GAMING.md`, `docs/ROADMAP.md` and the game page, which says on-device play isn't built. They're no longer planned.
- **Done when:** the games-ui and games-library "sent back" lists are empty, each item closed by an issue.

### 0.3.8 · See everything, part 1 · Q

- **End-to-end stamps:** every frame carries its finish time, frame number and the input sequence numbers it included, through the encoder and network to the client. Clocks aligned at session start.
- **Frame watermark** for tests: a small coded block in a corner, read back from the decoded frame, so frames match exactly end to end.
- **Hitch counter with causes:** game slow, encoder, network or client.
- **Stats overlay** (Off, Simple, Detailed): fps sent and shown, frame to screen p50/p99, press to picture, bitrate, loss, encoder, decoder and whether hardware, sound offset, hitches.
- Same numbers in the session log, so a report can include them.
- **Done when:** a session on any client shows the Detailed overlay, and its log has every stage's time for every frame.

### 0.3.9 · See everything, part 2: the baseline · Q

- **`wittyplay-check` streaming mode:** games through the full pipeline into a headless browser, recording the scorecard's measures per game.
- **The baseline matrix:** every console, every offered core, the heaviest and quietest games in the owner's library, every encoder this machine has, LAN and impaired network profiles.
- Each hypothesis in [the streaming design's section 3](../design/wittyplay-streaming-performance.md#3-what-the-owner-feels-and-what-probably-causes-it) confirmed or ruled out, in writing.
- A baseline report (numbers only, no game names) kept in WittyPlay's repository.
- **Done when:** every scorecard row with "not measured" has a number, and the causes of the owner's lag, glitches and hitches are known.

---

## 0.4.x: quick wins and the play experience

Server 0.7.x, **module API 4** (release channels). Goal: **feel better at once** with changes that need no new architecture, and give the play experience its first big upgrades.

### 0.4.0 · Sound you can't feel · A

- Opus at **10 ms frames**, `lowdelay`, constant bitrate.
- **One clock** for picture and sound, from the core's frame clock.
- **Rate matching:** resample by up to ±0.5% to stay tied to real time, instead of jumping over gaps.
- **A small client buffer:** in the web client, an AudioWorklet player targeting 15 to 25 ms instead of the browser's own buffer. (It only plays sound; nothing is emulated.)
- A choice: **Fastest picture** or **In sync** (delay the picture by a steady sound offset).
- **Done when:** sound behind picture ≤ 30 ms on LAN (scorecard 7), no gaps in a 30-minute soak.

### 0.4.1 · Smoothness · E R

- Windows: runner and encoder threads registered with the Multimedia Class Scheduler ("Games").
- Linux: cgroup `cpu.weight` for game sessions, so transcodes and scans can't starve them.
- **No allocation per frame** in the service's hot path, enforced by a test.
- **Done when:** hitches per 10 minutes halve against the 0.3.9 baseline with a transcode running beside the game.

### 0.4.2 · Loss recovery on the LAN · T

- **NACK and retransmission (RTX)** for video: on a LAN a resend costs about 1 ms, far cheaper than a keyframe.
- **Done when:** 1% loss on LAN causes no visible smear in the watermark test.

### 0.4.3 · Insert animations: the engine · U

- The animation types (front loader, top slot, back slot, bottom slot, card slot, lid, tray, door), timing tied to the session's first frame ("Only while loading": never slower to start), skip with any button, reduced motion.
- Stock packs for the most-played consoles first: NES, SNES, N64, PlayStation, PS2, GBA, DS.
- It's interface drawn by the client while the server loads the game, like a loading screen. No game code runs on the client.
- **Done when:** from Play to first frame is no slower with the animation on than off.

### 0.4.4 · Insert animations: every console · U

- Packs for every console in the table, our own drawings and sounds (no logos, no traced photos, no recorded console sounds).
- Settings per profile, per console and per game.
- **Done when:** every console has a stock pack, checked in both themes, at phone and TV sizes.

### 0.4.5 · Insert animations: make your own · U P

- Custom packs (data only, SVG cleaned, sounds re-encoded, size limits), upload for admins.
- `wittyplay-pack new | check | preview | build`, the template repository, the makers' guide and the in-app help page.
- **Done when:** a pack made with `wittyplay-pack new` passes `check`, uploads and plays; a pack with a script or an outside link is refused with the line.

### 0.4.6 · Core store · C U

- Every core for every console, recommended or not, with honest notes (accuracy, speed, needs a GPU, BIOS), licence, our test result, size.
- Install, remove, set as the console's default, set for one game. Filters by console, installed, GPU, licence.
- Non-commercial cores never listed.
- **Done when:** the store lists every core WittyPlayCores builds, with notes, and the 0.3.9 matrix results beside each.

### 0.4.7 · Cores across consoles · C

- Per console or per game: **use a core made for another console**, with a plain warning.
- **Saves kept per core** for a game, so an experiment can't damage the usual save.
- Falls back to the usual core if it fails to start.
- **Done when:** a multi-system core runs another console's game, and its saves stay apart from the usual core's.

### 0.4.8 · Your own cores, findable · C P

- "Add your own core" card in the store, with the existing guards (two switches, admins, password, sandbox, SHA-256 pinned), the first test shown live, core-info read, licence shown.
- The Smart App Control check from 0.3.5 runs before the upload is accepted.
- **Done when:** an admin finds, adds and plays a custom core without reading the docs.

### 0.4.9 · Disc sets · L

- Disc set editor on the game's page: order, names ("Disc 1", "Data disc", "Audio CD"), add and remove.
- Combine games as discs of one game by hand, and split them.
- **Extra discs** that aren't the game's own, including audio CD images from WittyCD's library.
- **Done when:** a dual-disc custom set and a game with an audio CD added both play.

### 0.4.10 · Changing discs while playing · S I

- The game menu's **Discs** list: open tray, choose, close, in the steps the console expects; one button "Change to disc 2" for people who don't want to think about trays.
- A controller shortcut to open the list.
- **Done when:** a swap works by controller, keyboard and remote on every disc console.

### 0.4.11 · Picture filters on the server · E U

- Scanlines, CRT looks, colour correction, sharpening and smoothing run on the server, on the GPU when there is one and on the CPU otherwise, **before encoding**. The browser's canvas shader path is removed.
- Every client, native apps included, gets the same picture.
- Filters cost is shown in the overlay; low-end tiers offer only cheap ones.
- **Done when:** a filter chosen on the web shows identically on the Android app, and the client does nothing but decode.

### 0.4.12 · Release candidate for the play experience · Q

- The baseline matrix again; the scorecard rows this series touched.
- **Done when:** every row moved by 0.4.x is better than the 0.3.9 baseline, and nothing else got worse.

---

## 0.5.x: the encoder host

Goal: **a new engine**, replacing FFmpeg on the command line for game video, so the encoder never restarts, loss heals in a frame, and GPU frames never leave the GPU. Engineering detail: [the streaming design, sections 5 and 6](../design/wittyplay-streaming-performance.md#5-capture-and-the-core).

### 0.5.0 · Shared memory · E

- A ring of frame buffers shared by the runner and the service, signalled by events, no copies through pipes.
- Sound the same way.
- **Done when:** frame to encoder ≤ 1 ms on every core, measured by stamps.

### 0.5.1 · The encoder host, in parallel · E

- `wittyplay-encoder`, one per session, sandboxed like the runner, linking libavcodec from the server's FFmpeg and talking to hardware encoders directly where lighter.
- Runs behind a switch, beside the old path, for side-by-side comparison in the matrix. Prototype decides C/C++ or Go with cgo (inside WittyPlay only; the server stays cgo-free).
- **Done when:** both paths produce the same picture quality at the same bitrate on every encoder.

### 0.5.2 · Live bitrate · E T

- Bitrate changed without restarting the encoder, as often as every second.
- **Done when:** a bitrate change causes no hitch (it caused a new encoder before).

### 0.5.3 · Healing without keyframes · E T

- **Intra refresh** instead of periodic keyframes, **reference picture invalidation** where the encoder supports it, long-term reference frames as safe points.
- Keyframes only for a new client or a decoder reset.
- **Done when:** after a lost packet the picture heals within 100 ms with no stall (scorecard 8, 14).

### 0.5.4 · Slices · E

- Output in slices (tiles for AV1), sent as each is ready.
- **Done when:** encode-plus-send time per frame drops measurably at 1080p.

### 0.5.5 · Zero-copy on Windows · E

- GPU cores' frames shared as textures straight into NVENC, AMF or Quick Sync, no read-back.
- **Done when:** frame to encoder ≤ 0.5 ms for GPU cores on Windows, and GPU-to-CPU traffic gone in a profiler.

### 0.5.6 · Zero-copy on Linux and macOS · E

- Linux: DMA-BUF into VAAPI, or CUDA interop into NVENC.
- macOS: IOSurface into VideoToolbox (built and checked on GitHub's Mac runners; marked not run on a real Mac).
- **Done when:** Linux matches Windows' figure from 0.5.5.

### 0.5.7 · HEVC and AV1 · E T

- Chosen per session from what the client decodes in hardware and what the server encodes.
- **Done when:** the same quality at about a third less bitrate on clients that support them.

### 0.5.8 · Sharp pixels · E

- 4:4:4 for pixel-art consoles on clients that decode it; integer scaling to the client's size.
- A colour bar test through every encoder: BT.709, limited range, tagged correctly.
- **Done when:** pixel art shows no colour smearing, and colours match the core's frames exactly.

### 0.5.9 · The old path retired · E Q

- Game video moves to the encoder host for good. FFmpeg on the command line stays only as a software fallback where no encoder host build exists.
- One-hour soaks on every tier; chaos tests (bursts, reordering, 2-second drops, CPU at 100%).
- **Done when:** soaks show no growth and the chaos suite passes.

### 0.5.10 · Engine release candidate · Q

- **Done when:** frame to screen on LAN meets **≤ 10 ms p50, ≤ 20 ms p99** (scorecard 3) in the matrix.

---

## 0.6.x: transport and remote play

Goal: **the network disappears**, at home and away. Detail: [the streaming design, sections 7 and 13](../design/wittyplay-streaming-performance.md#7-transport).

### 0.6.0 · Congestion control · T

- Transport-wide feedback and a delay-based estimator driving the encoder's live bitrate, reacting within 100 ms to a queue building.
- **Done when:** on a link capped at 4 Mbit/s, delay stays flat when the cap is hit.

### 0.6.1 · Pacing · T

- A frame's packets spread over a few milliseconds, so Wi-Fi access points don't spike.
- **Done when:** p99 frame to screen on Wi-Fi improves against 0.6.0.

### 0.6.2 · Forward error correction · T

- FlexFEC or ULPFEC, amount set by measured loss; NACK still used where round trips are short.
- MTU probing, smaller packets on Wi-Fi.
- **Done when:** 3% loss at 30 ms round trip shows no smear.

### 0.6.3 · Showing frames at the right moment · T E

- Direct presentation (decode straight to a low-latency canvas) made the default where camera measurements favour it.
- Variable refresh rate detected and used; phase lock where it isn't.
- **Done when:** waiting for the display ≤ 4 ms median on fixed-refresh screens.

### 0.6.4 · Honest start · T U

- A 3-second probe at session start shows round trip and loss, and what that means in words.
- Data saver offered on metered connections.
- **Done when:** every remote session starts with a sentence saying how it'll feel.

### 0.6.5 · Away from home: TURN · T

- A TURN relay on the server's own HTTPS port (permissive library), joined to Remote Access. The opt-in UDP port range documented as the faster option.
- **Done when:** a phone on mobile data plays through the relay, with no router change.

### 0.6.6 · Networks that block UDP · T

- WebTransport (QUIC datagrams) as a middle fallback.
- **H.264 over WebSocket**, decoded by WebCodecs, replacing the JPEG-per-frame fallback.
- **Done when:** a network that blocks UDP still plays at full quality, a little later.

### 0.6.7 · Reconnect without losing the game · S T

- A dropped network pauses the game on the server; the session survives for a set time (default 10 minutes); the client reconnects (ICE restart) and resumes where it was.
- **Done when:** pulling the network for 10 seconds and plugging it back resumes the same moment (scorecard 9).

### 0.6.8 · Low-latency networks · T

- L4S and ECN where routers and providers support them. Experimental, off by default.
- **Done when:** measured on one L4S-capable path, or marked not run.

### 0.6.9 · Data saver and quality caps · T U

- A cap per profile and per device, AV1/HEVC preferred, 30 fps offered for slow games.
- **Done when:** a session on Data saver stays under its cap for an hour.

### 0.6.10 · Remote release candidate · Q

- **Done when:** at 30 ms round trip and 1% loss, round trip + ≤ 25 ms over LAN (scorecard 5).

---

## 0.7.x: seamless sessions

Goal: **nobody waits and nothing is lost.** This series is where WittyPlay matches the leading service's feel of "just playing".

### 0.7.0 · Warm runners · S R

- A small pool of runners already started and sandboxed per core family, ready to load a game.
- **Done when:** Play to first frame ≤ 2 s for cartridges on a warm server (scorecard 1).

### 0.7.1 · Predictive preload · S

- The core and game of "Continue playing" are loaded into memory ahead, when the machine is idle and memory allows.
- **Done when:** the most recent game starts in ≤ 1 s.

### 0.7.2 · Suspend and resume · S

- Leaving a game suspends it: a state is saved, and the runner is kept warm for a while if resources allow, or frozen to disk.
- **Done when:** resuming a suspended game takes ≤ 1 s warm, ≤ 3 s from disk, at the same frame.

### 0.7.3 · Hand-off between devices · S N

- Start on the TV, pick up on the phone: "Continue on this device" moves the stream, keeps the session, and switches the controller layout.
- **Done when:** moving mid-game takes ≤ 3 s and loses nothing (scorecard 10).

### 0.7.4 · A queue instead of a refusal · S R

- When the server is full: "You're next, about 3 minutes", with a notice when it's ready. Admins can reserve a session.
- **Done when:** a full server never answers with only an error.

### 0.7.5 · Presets · U T

- **Automatic** (default), **Competitive**, **Balanced**, **Quality**, **Data saver**, **Custom** with every knob (resolution, render scale, frame rate, codec, colour, bitrate, loss protection, latency mode, sound, drawing, run-ahead, overlay).
- **Done when:** each preset is one choice on every client, and Custom shows the live effect in the overlay.

### 0.7.6 · Per-device settings and calibration · U T

- Settings remembered per profile and per device.
- A five-second calibration the first time a device plays: measures its decoder, display and network, and sets Automatic accordingly.
- **Done when:** a new TV plays well the first time with no settings touched (scorecard 11).

### 0.7.7 · Save history · S U

- Every save, automatic state and manual state with a picture and time; restore any; slots per profile; export and import saves as data.
- **Done when:** a person can go back to any point from today's play from the game page.

### 0.7.8 · Crash-proof sessions · S Q

- A runner that stops producing frames for 2 seconds is restarted from the last automatic state; the person sees a pause, not an error.
- Automatic states every few minutes, so a crash costs seconds at most.
- **Done when:** killing the runner mid-game loses under 10 seconds of play and needs no click.

### 0.7.9 · Seamless release candidate · Q

- **Done when:** scorecard rows 1, 2, 9, 10 and 11 all meet their targets.

---

## 0.8.x: resources

Goal: **the server shares itself well**, with limits set automatically and overridable by admins. Detail: [the streaming design, section 12](../design/wittyplay-streaming-performance.md#12-resource-allocation).

### 0.8.0 · Hardware probe · R

- Threads, single-thread speed, GPU and its encoders (codecs, sessions, speed), memory, optionally upload speed. Run on install and after hardware changes, as a background task.
- **Done when:** the probe's report matches the machine on the three tiers tested.

### 0.8.1 · Tiers and automatic limits · R

- Low, Mid and High defaults from the probe's numbers, not model names.
- **Done when:** a fresh install on each tier picks limits that hold the scorecard targets for its default session count.

### 0.8.2 · Enforced limits · R P

- Per-session CPU and memory limits: cgroup v2 on Linux, Job Objects on Windows, `rctl` on FreeBSD, priorities only on macOS (said plainly).
- **Done when:** a runaway core is held at its limit and the other sessions don't notice.

### 0.8.3 · GPU-aware scheduling · R E

- Hardware encoder sessions pooled and counted; render scale managed by GPU load.
- **Done when:** a GPU at its limit lowers render scale before dropping frames.

### 0.8.4 · The degradation ladder · R

- Lower render scale, then frame rate where the game allows, then software encoding if the CPU allows, then queue (0.7.4).
- **Done when:** adding sessions past the limit degrades in that order and is told in words.

### 0.8.5 · Priorities with the rest of the server · R

- Games first, Even, or Media first, against transcodes and scans, through the task registry: scans pause and transcodes slow while a game runs when "Games first".
- **Done when:** a library scan beside a game causes no hitch on "Games first".

### 0.8.6 · The Resources page · U R

- **Settings → Games → Resources** for admins: sessions, per-session ceilings, encoder choice, priority, each with its automatic value and **Back to automatic**. WittyStats widgets for game sessions.
- **Done when:** every limit is visible, changeable and resettable in one place.

### 0.8.7 · Several GPUs · R E

- Sessions placed on the GPU with room; a core and its encoder kept on one GPU.
- **Done when:** a two-GPU server runs twice the GPU sessions.

### 0.8.8 · Shader pre-warming · E R

- An optional background task that plays a short recorded input per GPU game to fill its shader cache, shown in the activity panel.
- **Done when:** first visits to pre-warmed games show no compile hitches.

---

## 0.9.x: thin native clients

Goal: **every screen is a thin window**, and native apps beat any browser for latency. Clients only decode, display, play sound and send input.

### 0.9.0 · Client protocol 2 · N P

- The session API, WebRTC offer and answer, data channel messages and fallbacks, documented and versioned in `docs/GAMING-API.md`, with a test client that plays a game with no browser (as `TestNativeClientPlaysAGame` does today).
- **Done when:** a client can be written from the document alone.

### 0.9.1 · Android, low latency · N

- MediaCodec low-latency decoding, SurfaceView, AAudio sound, controller mapping, stats overlay.
- **Done when:** frame to screen on an Android phone is within 5 ms of the best browser on the same network.

### 0.9.2 · Android TV · N

- Ask the TV for its low-latency game mode where Android exposes it; 4K; the remote as a fallback controller; Android TV home screen row for "Continue playing".
- **Done when:** a TV stick plays with game mode on automatically.

### 0.9.3 · Apple: iOS and tvOS · N

- VideoToolbox, AVSampleBufferDisplayLayer, the GameController framework, a small sound buffer.
- Needs a Mac to build: `needs-owner`.
- **Done when:** built, signed and run on a real iPhone and Apple TV.

### 0.9.4 · Desktop, native presentation · N

- A native presenter in the desktop client for direct presentation and variable refresh, beyond what the browser engine allows.
- **Done when:** the desktop client beats the browser on the same PC.

### 0.9.5 · More TVs and handhelds · N

- Plasma Bigscreen; webOS and Tizen (web client, thin); Linux handhelds through the desktop client.
- **Done when:** each runs on real hardware, or is marked not run.

### 0.9.6 · Rumble and haptics · I

- Force feedback from the core to the pad, on every client that can drive it.
- **Done when:** rumble works on XInput, DualShock and DualSense, and Switch Pro pads.

### 0.9.7 · Motion and touch · I

- Gyro passthrough for games that use motion; touch screens for DS and 3DS polished on phones and tablets.
- **Done when:** a DS game is fully playable by touch on a phone.

### 0.9.8 · Mouse and keyboard · I

- Mapping for consoles with mouse peripherals, and keyboard play with remapping.
- **Done when:** a mouse-driven game is playable on desktop.

### 0.9.9 · Every-client release candidate · Q

- **Done when:** every client in the owner's platform list plays the matrix's reference games, or is marked not run with the reason.

---

## 0.10.x: the dashboard

Goal: **an interface better than the leading service's.** Theirs is a store; ours is the person's own collection, shown like a console dashboard. This series can start beside 0.5.x, since it touches different code.

### 0.10.0 · Home · U

- A hero "Continue playing" with the live frame where you stopped; rows for recently played, favourites, by console, collections, most played, new to the library. Big picture mode by default on TVs.
- **Done when:** with 10,000 games, Home opens in ≤ 300 ms (scorecard 13).

### 0.10.1 · Game pages · U L

- Art, play time, save history, the core in use and its notes, compatibility from our sweeps, per-game settings, discs.
- **Done when:** everything about a game is on its page, by mouse, touch, keyboard, remote and pad.

### 0.10.2 · The quick menu · U

- One button opens cards: Resume, Save, Load (with pictures), Rewind, Speed, Discs, Controls, Picture, Sound, Stats, Hand off, Quit. Driven by the server's session API, so every client shows the same choices.
- **Done when:** every action is two presses or fewer from in-game.

### 0.10.3 · Search and collections · U L

- Search as you type with typo tolerance; filters (console, players, genre, year, played); smart collections by rules.
- **Done when:** any game is three keystrokes or one voice search away.

### 0.10.4 · Controller-first everywhere · U I

- Every screen navigable by pad, prompts matching the pad (Nintendo A/B swap, PlayStation shapes), sound and haptics for focus.
- **Done when:** a person never needs a mouse or keyboard from sign-in to playing.

### 0.10.5 · Profiles and families · U S

- Per-profile game visibility, ratings limits, play time limits and bedtime for kids' profiles, a PIN to leave a kids' profile.
- **Done when:** a parent sets a daily limit and the game saves and pauses when it's reached.

### 0.10.6 · Onboarding · L U

- Point at the games folder and leave it; "Needs a look" queue polished; a **BIOS checker** that says which consoles need a BIOS, checks the person's own files by hash, and explains where it goes. It never downloads, links or bundles a BIOS.
- **Done when:** a new person goes from empty server to playing without reading the docs.

### 0.10.7 · Accessibility · U I

- Screen reader for every menu; one-handed remapping, hold-to-toggle and turbo; colour filters for colour blindness, made on the server like the other filters.
- **Done when:** the accessibility pass (screen reader, keyboard, every pad) is clean on every game screen.

### 0.10.8 · Look and themes · U

- Game screens follow the server's themes; console-themed accents optional; motion reduced where asked.
- **Done when:** every game screen passes the design check in both themes.

### 0.10.9 · Dashboard release candidate · Q

- Watched by people who didn't build it, the way the setup wizard was.
- **Done when:** first-time users find and play a game, resume it on another device, and change a preset without help.

---

## 0.11.x: together

Goal: **play with others on one server**, which a server-side design makes natural.

### 0.11.0 · Join a game from your own device · G S

- A second player joins a running session from their own device by invite or profile, each with their own stream; up to four players (`MaxPlayers`).
- **Done when:** two people on two TVs play one game together on LAN.

### 0.11.1 · Spectate · G

- Watch a session live. Cheap: the already-encoded stream is forwarded.
- **Done when:** five spectators add almost nothing to the server's load.

### 0.11.2 · Clips and screenshots · G

- The server keeps the last minutes of the already-encoded stream; "Save the last 30 seconds" needs no re-encode. Screenshots at full resolution from the core's frame.
- **Done when:** a clip is saved in under a second and appears in the library.

### 0.11.3 · Recording sessions · G

- Record a whole session into the library as a video item, with the same privacy as any library item.
- **Done when:** a recorded session plays back in the ordinary video player.

### 0.11.4 · Game nights · G S

- Scheduled sessions with invites, a lobby, and a reminder in the app.
- **Done when:** four profiles join a scheduled game from a reminder.

### 0.11.5 · Achievements, opt-in · G

- From the person's own account on an open achievements service, only if its terms allow it (to research), off by default.
- **Done when:** decided, and built only if the terms are acceptable.

### 0.11.6 · Together release candidate · Q

- **Done when:** co-op on LAN feels the same for both players (scorecard 3 and 4 for each).

---

## 0.12.x: more consoles

Goal: **every console a permissible core allows**, all on the server. Each console needs a core with a licence we can use (no non-commercial cores), no circumvention keys, and an encrypted-media check where the format has one.

### 0.12.0 · WittyPlayCores everywhere · C P

- Every offered core built, pinned and signed for every server platform it compiles on.
- **Done when:** the core store is the same on Windows, Linux, macOS and FreeBSD, except where a core can't build (said plainly).

### 0.12.1 · Arcade · C

- Arcade cores whose licences allow it, with set checking and per-game settings. Cores under non-commercial licences stay off.
- **Done when:** arcade sets import and play, and the store shows each core's licence.

### 0.12.2 · More disc consoles · C

- Further disc-based consoles with permissively usable cores (each checked for licence and circumvention needs before work starts).
- **Done when:** each added console passes `wittyplay-check`.

### 0.12.3 · Heavier 3D consoles · C E R

- Later 3D consoles whose open cores run on a server GPU, with their own tier rules, render scaling and pointer controls mapped from pads.
- Never a console whose games or system need keys taken from hardware, or decrypting.
- **Done when:** each added console holds its frame rate on the High tier.

### 0.12.4 · Compatibility database · C Q

- Per game: the best core, settings and known problems, from our sweeps, shown on the game page and used by Automatic.
- **Done when:** every game in the owner's library has a tested recommendation or a "not tested" mark.

### 0.12.5 · Consoles release candidate · Q

- **Done when:** the full matrix, every console, passes.

---

## 1.0.0 · Proven

- **Camera-measured:** press to photon filmed at 240 fps on a TV in game mode and a desktop monitor, wired and on Wi-Fi, against the same games run locally on the same screen. "On par" means within a frame or two of local play.
- **Every scorecard row met**, or the gap said plainly.
- **Every client** in the owner's platform list run on real hardware.
- **Signed everywhere:** Windows, macOS; Smart App Control On in a clean Windows 11 machine.
- **Security:** an outside review of the runner sandbox, the encoder host and the session API.
- **Done when:** the owner plays on their own screens and says it feels like local play.

---

## Server and module API needed

| WittyPlay | Server | Module API | Why |
|---|---|---|---|
| 0.3.5 to 0.3.9 | 0.6.x | 3 | Fixes and measurement only |
| 0.4.0 to 0.4.12 | 0.7.0-alpha and later | **4** | Release channels; core store and pack routes |
| 0.5.x | 0.7.x | 4 | Encoder host is inside WittyPlay; no server change |
| 0.6.x | 0.7.x; **0.8.0-alpha** for TURN in Remote Access | 4 | TURN and the start probe touch Remote Access |
| 0.7.x | 0.8.x | 4 | Hand-off and queue notices use server notifications |
| 0.8.x | 0.8.x | 4 (5 if the task registry's priority hooks need a new host call) | Priorities with transcodes and scans |
| 0.9.x | 0.8.x; client apps' own versions | 4 or 5 | Protocol 2 is WittyPlay's own |
| 0.10.x to 0.12.x | later 0.x alphas | current | Interface and cores |
| 1.0.0 | whatever the server is then | current | WittyPlay 1.0 doesn't require the server to be out of alpha |

Every release states its `minServer` and `maxServer`, as now.

## WittyPlayCores timeline

A new repository (to add to `docs/REPOSITORIES.md` for the owner): offered cores built from pinned upstream commits, signed, with their exact source published beside them.

| Version | Adds |
|---|---|
| 0.1.0 | Windows amd64, every offered core, signed; core list signed by our release key (for WittyPlay 0.3.5) |
| 0.2.0 | Linux amd64 |
| 0.3.0 | Windows and Linux arm64 |
| 0.4.0 | macOS (Intel and Apple silicon), signed with our Team ID for the hardened runtime, notarised |
| 0.5.0 | FreeBSD amd64 and arm64, where cores build |
| Ongoing | Updating a core is a pull request that moves its pinned commit, rebuilds and re-runs `wittyplay-check` |

## Removed from the plan

The owner ruled these out on 2026-11-02 ("all ran on the server and streamed, nothing in browser"):

| Removed | Was | Instead |
|---|---|---|
| **In-browser emulation** | "Designed, not built" in `docs/GAMING.md` and the roadmap | Server-side only; the docs say so from 0.3.7 |
| **WASM cores** | Part of that design | None |
| **On-device play** on phones and TVs | Mentioned on the game page as not built | Thin native clients (0.9.x) |
| **Client-side picture shaders** | `web/display.ts` drew effects on a canvas | Server-side filters (0.4.11) |
| **The JPEG-per-frame fallback** | Last resort over WebSocket | H.264 over WebSocket with WebCodecs (0.6.6) |

## How each version is run

- A milestone per version in WittyPlay, an issue per item, a release issue with the checklist ([../GITHUB-PRACTICES.md](../GITHUB-PRACTICES.md)).
- Every version re-runs the parts of the matrix it touches, and the scorecard rows it claims.
- Small version bumps, each on the alpha channel. A bump isn't a release; tags never move.
- Real-world checks the owner must do (screens, pads, the camera, a Mac) are `needs-owner` issues with step-by-step instructions.
- Test on Linux first; remember this machine's traps (first-launch stalls on new programs, other chats loading the CPU, WSL's D3D12 layer).

---

<sub>Future plans, written 2026-11-02. Not built, not tested.</sub>
