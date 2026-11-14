# Games: one stream for every device

How a game's picture, sound and controls travel between the server and every client: browsers on computers and phones, the
Apple app on Apple TV, iPhone, iPad and Vision Pro, and the Android app on phones, tablets and Android TV, at home and away.
This is the decision, the numbers behind it, and the exact wire protocol, version 2. For everything else about playing a
game (starting it, saves, controller layouts, settings), see [the games API](GAMING-API.md).

## The decision

**Every client uses WebRTC, through one route, with the same messages.** A browser has WebRTC built in. The Apple and Android
apps use Google's libwebrtc, as prebuilt packages: LiveKit's `webrtc-xcframework` (iOS, iPadOS, tvOS 17 and later, visionOS
and macOS) and its Android package `io.github.webrtc-sdk:android`. Both are BSD-licensed builds of the same code a browser
runs, with H.264 decoded in hardware through VideoToolbox and MediaCodec.

**The one fallback is the same messages over a WebSocket**, for a network that won't carry UDP to the server: a reverse proxy,
a rented server in front, or a guest Wi-Fi that blocks it. It's a property of the network, not the platform, so every client
has it. JPEG stays for a server with no H.264 encoder at all.

This is the only stream: `/play/{sid}/stream`, version 2. The routes of version 1 (`/play/{sid}/webrtc` and
`/play/{sid}/ws`) are gone.

### Why

Measured on this Mac (M3 Pro, 12 cores, macOS 27.0.1) with `wittyplay-probe`, which plays a game through the real runner,
encoder and transport into a real browser (Chrome 156, headless) and reads frame numbers back off the picture it shows.
Every server part was a private copy on 127.0.0.1; nothing reached another machine. Each line is 20 to 30 seconds of play.
The Mac was busy with other work during these runs (load average 7 to 80 on 12 cores), so 95th percentiles are noisier than
an idle server's; the medians agree run to run. The test share has no Nintendo DS game, so the DS wasn't measured: its
picture, 256x384, is about the SNES's doubled, which the SNES lines cover.

**At home, on a clean network, the transports are equal.** Press to picture, in milliseconds, median and 95th percentile:

| Game | WebRTC | WebSocket + H.264 | Bitrate | Server CPU, % of one core |
|---|---|---|---|---|
| NES (FCEUmm, Thwaite), 512x448 sent | 4.5 / 14.5 | 2.5 / 3.6 | 5.9 Mbit/s | 18 (encoder 9, runner 4, rest 5) |
| SNES (Mesen-S, SNES_00), 512x478 sent | 9.3 / 17.9 | 6.5 / 10.0 | 4.1 Mbit/s | 43 (runner 30, encoder 9) |
| NES, JPEG | | not measured by the probe | | 13 (no FFmpeg) |

WebRTC's figure includes the wait for the browser's compositor, which the WebSocket's canvas path skips; the browser's own
jitter buffer held frames for 0.5 ms (the `playout-delay` extension at work). Both are well under a frame.

**Away, the transports part.** The same NES game through the probe's simulated networks, which add delay, jitter, loss and
a speed limit to everything between the server and the browser (UDP for WebRTC; TCP for the WebSocket, where loss is
modelled as TCP recovers from it: the lost segment comes a round trip late, or three after a burst's tail, and everything
behind it waits):

Press to picture, in milliseconds, median and 95th percentile. It includes the round trip itself.

| Network: round trip, jitter, loss, speed | WebRTC | WebSocket that only queues (version 1, removed) | WebSocket, version 2 |
|---|---|---|---|
| Home Wi-Fi: 6 ms, 4 ms, 0.5% | 16 / 42 | 14 / 38 | 17 / 56 |
| Away: 40 ms, 8 ms, 1% | 58 / 106 | 53 / 171 | 51 / 170 |
| A bad connection away: 60 ms, 15 ms, 3% | 92 / 173 | 235 / 419 | 236 / 396 |
| A slow upload at home: 40 ms, 3 Mbit/s | 63 / 147 | 2,356 / 4,650 | 58 / 231 |

Version 1's WebSocket on a slow link is a queue that only grows: it sends 6 Mbit/s into a 3 Mbit/s link with nothing to
slow it down, and at over 2 seconds behind (2.0 to 2.8 s in three runs) the game can't be played. WebRTC comes down to
fit within seconds. Version 2's WebSocket now does too, from the client's reports of how late frames arrive: its median
matches WebRTC's, and its 95th percentile (231 and 567 ms in two runs) is mostly the first seconds, before the rate has
found the link. What it can't fix is loss: its third line is TCP waiting for every lost packet, as version 1's is, and
there WebRTC is two and a half times quicker. A frame's gap at the 95th percentile, which is what a stutter looks like,
was 30 to 90 ms over WebRTC on these networks.

Over a WebSocket, a lost packet's wait looks to the server like a queue, so on a lossy link the bitrate comes down too
(four steps on the 1% and 3% lines). The picture is softer there; it isn't later.

**What decided it:**

- **Loss.** TCP hands data over only in order, so one lost packet holds up every frame behind it until it's resent. WebRTC
  resends just that packet (NACK), and a frame that can't be finished is skipped while the next keyframe is fetched. At 1%
  loss that's the 95th percentile above; at 3%, the median too. Mobile networks and busy Wi-Fi lose that much in bursts.
- **Rate.** WebRTC tells the server what's arriving, every second, and the server matches the video to it (`rateControl`).
  Over a WebSocket the system's buffers hide a full link from the server for a second or more, so version 2 has the client
  report how late frames arrive. That works (the last line above), but settles a few seconds later than WebRTC.
- **Sound.** WebRTC carries Opus at about 100 kbit/s. The WebSocket carried raw PCM, 1 to 1.5 Mbit/s, which is a large
  share of an upload away from home. Version 2 sends Opus over the WebSocket as well.
- **Getting through.** Over Tailscale or NetBird, the recommended remote access, WebRTC reaches the server directly, as at
  home. Through a reverse proxy or a forwarded HTTPS port only, it can't, and the WebSocket can. So both stay, in that order.
- **Native apps.** tvOS, iOS and Android have no WebRTC of their own, so the apps add libwebrtc: about 10 to 20 MB. That's
  the whole cost, and it's paid once; in return the apps get loss recovery, congestion control, Opus, NAT traversal and the
  same jitter-buffer behaviour as the browser, none of which they'd otherwise have to write.

### What was weighed and set aside

| Option | Verdict |
|---|---|
| **A tuned WebSocket for everyone** | Simplest for the apps, and as fast as WebRTC at home. Away, TCP's in-order delivery turns every loss into a stall for every frame behind it, as measured above. Kept as the fallback, tuned |
| **WebTransport (QUIC) for browsers, raw QUIC in the apps** | Every browser has WebTransport since Safari 26.4 (March 2026), and QUIC's datagrams don't stall on loss. But it brings none of what WebRTC gives for free: no jitter buffer, no resends, no congestion control tuned for video, no NAT traversal, and it needs a trusted certificate or a pinned one renewed every 14 days. Android has no QUIC of its own and Apple's has no WebTransport, so the apps would each write a client from scratch. It doesn't pass a reverse proxy either, so the WebSocket would still be needed. Worth a second look only if WebRTC's jitter buffer ever becomes the problem |
| **A UDP protocol of our own for the apps** | The apps can open UDP sockets, but browsers can't, so browsers would still need WebRTC: two protocols to keep in step, not one |
| **HEVC or AV1** | Not for these games. A retro console's picture, doubled, is 512 pixels wide, and H.264 carries it at 4 to 6 Mbit/s with every device decoding it in hardware. HEVC is arriving in browsers' WebRTC (Chrome's from about version 136) and most devices decode it in hardware; AV1 only recent ones. The `video` list in the hello lets a client say what it decodes, so HEVC can be offered later for the large pictures (Nintendo 64, PlayStation 2 and PSP, upscaled) on servers with a hardware HEVC encoder. Nothing here encodes it yet |
| **Hardware encoding on this Mac** | FFmpeg's `h264_videotoolbox` holds frames back before it gives one out, so with WittyPlay's one-frame-at-a-time feeding it stalls, is restarted, and falls back to libx264 (seen in every run). libx264 at 512x448 costs 8 to 10% of one core here, so a Mac server encodes these consoles in software. Larger pictures need the graphics card work in progress elsewhere |

## The protocol, version 2

### Finding it

`GET /games/modes` and a session's `modes` list what the server can stream, best first, as before. Every mode says
`"stream": 2`, and is reached through `/play/{sid}/stream`. A client that meets a higher number it doesn't know shows
"update this app" rather than connecting.

```json
{"id": "webrtc", "video": ["h264", "vp8"], "audio": "opus", "input": "datachannel", "stream": 2}
```

Try WebRTC first. If ICE hasn't connected within 5 seconds, or no picture has come within 8, connect the WebSocket. The game
waits, paused, for a minute with nobody connected, so trying costs nothing. Keep the old picture up until the new one shows.

### The hello

Everything a client says about itself, in one JSON object. It's the same over both transports.

```json
{
  "v": 2,
  "app": "WittyMedia tvOS 1.4",
  "device": "appletv-livingroom",
  "name": "Living room",
  "watch": true,
  "video": ["h264"],
  "audio": ["opus", "pcm"],
  "seats": [0],
  "display": {"hz": 60, "width": 3840, "height": 2160},
  "latency": "auto",
  "maxKbps": 0
}
```

| Field | Meaning |
|---|---|
| `v` | 2. Anything else answers `bad-hello` |
| `app` | The app and its version, for the server's log. Up to 64 characters |
| `device` | This device's id, as used for touch layouts: up to 64 letters, digits, `.`, `_` or `-` |
| `name` | What other players see this device as, up to 40 characters. Empty is fine |
| `watch` | `false` for a controller with no picture or sound, such as a phone held as a pad beside a TV. Default `true` |
| `video` | Codecs this device decodes in hardware, best first: `h264`, `vp8`, and for later `hevc`, `av1`. Over WebRTC the offer decides, and this breaks ties. Over the WebSocket: H.264 when the server has it, unless the list puts `jpeg` before `h264`; an empty list, or none, is H.264. Send `["jpeg"]` for JPEG |
| `audio` | `opus` and/or `pcm`, best first. WebRTC is always Opus. Over the WebSocket, leave out `opus` if you can't decode it |
| `seats` | The players this device controls, 0 to 7, as below. Left out: any player nobody has taken |
| `display.hz` | The screen's refresh rate, as exactly as you know it (59.94, 60, 50, 120). The server may run the game at it, below |
| `display.width`, `height` | The screen's size in pixels. For the log, and later for choosing a picture size |
| `latency` | `auto`, `fastest`, `balanced` or `steady`, as `emulation.latency` in [the games API](GAMING-API.md#settings) |
| `maxKbps` | A limit the person chose, or 0 |

### WebRTC

1. Make a peer connection with no ICE servers: the server's answer carries every candidate it has.
2. If watching, add a **receive-only** video transceiver and a **receive-only** audio transceiver.
3. Make the data channels: `input` (unordered, no retransmits, binary) and `events` (ordered, reliable, text). `timing`
   (unordered, no retransmits) is still accepted, but timing messages can now go on `input` as well.
4. Make the offer, set it, and wait for ICE gathering, up to 1.5 seconds.
5. `POST /play/{sid}/stream` with `{"hello": {...}, "sdp": "<offer>"}`. It answers `{"stream": {...}, "sdp": "<answer>"}`.
6. Set the answer. The stream's description is also sent as the first message on `events` once it opens.

The video is H.264 Constrained Baseline (`profile-level-id=42e01f`, `packetization-mode=1`), or VP8 when the offer has no
H.264 or the hello asks for it first. Offer the `http://www.webrtc.org/experiments/rtp-hdrext/playout-delay` header extension:
the server sets it to "don't wait" (or "up to 80 ms" for `balanced`), so a frame reaches the decoder as soon as it's whole.
libwebrtc honours it on every platform. NACK and PLI work as usual: a lost packet is resent, and a decoder that's lost track
asks for a keyframe. Sound is Opus, 48 kHz stereo, 10 ms packets, in its own MediaStream so its buffer never holds the
picture back.

### WebSocket

`GET /play/{sid}/stream`, upgraded to a WebSocket, with the sign-in as for any route: the `Authorization` header from an app,
the cookie or `?token=` from a browser. The first message must be the hello, as text, within 5 seconds:

```json
{"type": "hello", "v": 2, "watch": true, "video": ["h264"], "audio": ["opus", "pcm"], "seats": [0]}
```

The server answers with the stream's description, `{"type": "stream", ...}`, and then the picture, starting with a keyframe.
A hello that's missing, late or wrong is answered with `{"type": "error", "code": "bad-hello", "text": "..."}`, and the
connection closes.
Everything else is the same as over WebRTC: binary messages are what would go on `input`, and text is what would go on
`events`. The server keeps at most two frames waiting for a slow connection. Past that it drops the frame, waits for a
keyframe, and lowers the bitrate, so delay can't build up. Sound and events are never dropped.

### The stream

What the server says back, in the answer and as the first event:

```json
{
  "type": "stream", "v": 2, "conn": "k3", "transport": "webrtc",
  "video": "h264", "audio": "opus",
  "width": 512, "height": 448, "aspect": 1.3333, "fps": 60.0988, "rate": 48000, "turns": 0,
  "ports": 4, "motion": true, "motionOn": [{"port": 0, "accel": true, "gyro": true, "hz": 60}],
  "leds": [{"index": 0, "state": 1}], "pace": 60,
  "seats": [{"port": 0, "conn": "k3", "name": "Living room", "you": true}],
  "kbps": 5800, "away": false
}
```

| Field | Meaning |
|---|---|
| `conn` | This connection's id, as seats name it |
| `transport` | `webrtc` or `websocket` |
| `video`, `audio` | What this connection gets: `h264`, `vp8`, `jpeg` or `""` when not watching; `opus`, `pcm` or `""` |
| `width`, `height` | The encoded picture. It's often doubled: show it at `aspect`, not at its own shape |
| `aspect`, `fps`, `turns` | As in the session. `fps` is the console's own rate, such as 60.0988 or 50.007 |
| `rate` | The sound's sample rate: 48000 for Opus, the console's own for PCM |
| `ports` | How many controller ports the core describes: at least 4, at most 8. Not every port is a player's: FCEUmm's fifth is the Famicom's expansion port (Arkanoid, Hyper Shot, Family Trainer), where Mesen-S's fifth is a fifth player through its multitap. The session's `devices` ([the games API](GAMING-API.md#playing-a-game)) says what each port takes; offer a port as a player when it takes a controller |
| `motion` | Whether the core can read motion at all, below |
| `motionOn` | The players whose motion it wants now, and how often. Send motion for these, if you hold them |
| `leds` | The game's lights as they are now, below |
| `pace` | The refresh rate the game is running at, when it's been matched to a screen (below), or `0` |
| `seats` | Who has which player, below |
| `kbps` | What the video starts at |
| `away` | The server counts this device as away from home, and started lower |

### Seats: several players on several devices

A game has up to 8 players (`ports`). Each device can hold some of them:

- Players are 0 to 7; the stream's `ports` says how many ports this game has, and `devices` what each takes.
- A hello with `"seats": [1]` takes player 2 for this device. To change, send `{"type": "seats", "ports": [0, 1]}`. Add
  `"take": true` to take a player another device has; otherwise that one is refused with an `error`. A device that
  connects again with the same `device` id gets its own players back.
- Input for a player another device holds is ignored. A device that names no seats can play any
  player nobody holds.
- Whenever seats change, everyone gets `{"type": "seats", "seats": [{"port", "conn", "name", "you"}]}`.
- When a device lets a player go, or leaves, that player's buttons are released.
- Rumble for a held player goes only to the device holding it. Rumble for a player nobody holds goes to everyone.

Every device of the account that started the game can join it this way, up to 8 at once, including a phone with
`"watch": false` used only as a controller. Other accounts can't join a game.

### Input

Binary messages, little-endian, on `input` or the WebSocket. The first byte says what each is.

**Controller state:** 13 bytes, or 11 without triggers, or 17 with a number of your own on the end:

| Bytes | Field | Meaning |
|---|---|---|
| 0 | port | The player, 0 to 7 |
| 1-2 | buttons | u16, a bit per RetroPad button, below |
| 3-4, 5-6 | left stick | X, Y: i16, -32768 to 32767, with up negative |
| 7-8, 9-10 | right stick | X, Y |
| 11, 12 | L2, R2 | How far each trigger is held, 0 to 255. Set their button bits too, past your press point |
| 13-16 | number | Optional u32 of your choosing. It comes back in frame records (`0xF4`), to follow a press to the screen |

| Bit | Button | Bit | Button |
|---|---|---|---|
| 0 | B (bottom face) | 8 | A (right face) |
| 1 | Y (left face) | 9 | X (top face) |
| 2 | Select | 10 | L |
| 3 | Start | 11 | R |
| 4 | Up | 12 | L2 |
| 5 | Down | 13 | R2 |
| 6 | Left | 14 | L3 |
| 7 | Right | 15 | R3 |

Send the whole state as soon as anything changes, and again every 250 ms. The channel doesn't resend, so a lost state is put
right by the next. Release everything when the app goes to the background or a menu opens. Which button of a real pad is
which bit comes from [the controller as data](GAMING-API.md#the-controller-as-data) and the account's profiles.

**Touch screen:** 6 bytes, for a console whose `pad` has `"pointer": true`:

| Bytes | Field | Meaning |
|---|---|---|
| 0 | mark | 0xFF |
| 1 | flags | Bit 0: pressed |
| 2-3 | X | i16, -32767 at the picture's left edge to 32767 at its right |
| 4-5 | Y | i16, -32767 at the top to 32767 at the bottom |

Send it when anything changes, every 250 ms while pressed, and a release three times. It's one finger; for several, use:

**Touches,** several fingers at once: 2 bytes, then 6 for each finger, up to 10.

| Bytes | Field | Meaning |
|---|---|---|
| 0 | mark | 0xFE |
| 1 | count | How many fingers follow, 0 to 10 |
| then, for each finger: | | |
| +0 | id | Yours to choose, so a finger keeps its place in the list while it moves |
| +1 | flags | Bit 0: pressed |
| +2-3, +4-5 | X, Y | i16, across the whole picture, as for one touch |

Send the whole list when anything changes, and every 250 ms while a finger is down. When the last one lifts, send an empty
list (count 0) three times. The core sees the fingers that are pressed, in the list's order: libretro's pointer 0 is the
first, and its count is how many. The touch screen is the first player's, so either message needs player 0 (seats).

**Motion:** 14 bytes, for a game that reads a controller's or a phone's movement:

| Bytes | Field | Meaning |
|---|---|---|
| 0 | mark | 0xE1 |
| 1 | port | The player |
| 2-3, 4-5, 6-7 | accelerometer | X, Y, Z: i16 in 1/4096 g (±8 g), gravity included, the device held as the console's controller: X right, Y up, Z towards the player |
| 8-9, 10-11, 12-13 | gyroscope | X, Y, Z: i16 in 1/16 degree a second (±2048), turning about the same axes, anticlockwise positive |

Send it for a player you hold while the game wants it: the stream's `motionOn`, and then a
`{"type": "motion", "offered": true, "port": 0, "accel": true, "gyro": false, "hz": 60}` event whenever that changes
(`"offered": true` with no `port` says only that the core can read motion). Send it `hz` times a second (60 at most), and
stop when neither `accel` nor `gyro` is on. The core reads it through libretro's sensor interface, in metres a second
squared and radians a second, as RetroArch gives cores a phone's sensors. A core that never asks gets nothing. Servers on
Windows or a 32-bit system don't offer the sensor interface yet: there the stream's `motion` stays `false`.

**Rumble** comes back as an event:

```json
{"type": "rumble", "port": 0, "effect": 0, "motor": "strong", "strength": 40000, "ms": 300}
```

| Field | Meaning |
|---|---|
| `port` | The player. Only the device holding it gets it; with nobody holding it, every device |
| `motor` | `strong`, the heavy, low motor (the left grip on most pads), or `weak`, the light, high one (the right). `effect` is the same as libretro numbers it: 0 strong, 1 weak |
| `strength` | 0 to 65535. 0 stops that motor at once |
| `ms` | How long to keep that motor at that strength if nothing else comes. While it's on, the server says it again every 200 ms, so it carries on smoothly; a connection that drops stops within `ms` |

Play it on that player's controller, or the device itself, unless the account's profile says `noRumble`. A paused game's
motors stop, and start again when it carries on. libretro has no trigger motors, so neither does this.

**Lights:** `{"type": "led", "index": 0, "state": 1}` when the game sets one of its lights, and the stream's `leds` lists them
as they are. `state` is 0 for off, otherwise on, or a brightness up to 255 for a core that has one. They're the console's
own lights (a drive light, a lamp on an arcade cabinet), through libretro's LED interface. A client shows them where it has
a light to spare. libretro has no colour light bar: light a controller's player number, or its light bar's colour, from
`seats`.

**Timing and control,** optional, as in [the games API](GAMING-API.md#measuring-and-timing):

| First byte | From | Rest |
|---|---|---|
| `0xF1` | Client | A ping: an f64 of your own clock |
| `0xF2` | Server | The answer: that f64, then an i64 of the server's Unix nanoseconds |
| `0xF3` | Client | One byte: 1 to get a record of every frame, 0 to stop |
| `0xF4` | Server | One frame's record, as in [the games API](GAMING-API.md#measuring-and-timing) |
| `0xF5` | Client | One byte: 0 fastest, 1 balanced, 2 steadiest |
| `0xF6` | Client | An i16 of tenths of a millisecond to move when frames are made, once. Later is positive |
| `0xF7` | Client | You need a keyframe |
| `0xF8` | Client | You decode frames yourself, so your stack's own keyframe requests mean nothing |

### The picture and sound over the WebSocket

| First byte | Rest |
|---|---|
| `0x11` | Video: u8 flags (bit 0: keyframe), u32 frame number, u32 timestamp in 90 kHz, then one frame of H.264 in Annex B, with SPS and PPS before each keyframe |
| `0x12` | Opus: u32 timestamp in 48 kHz, then one packet: 10 ms, 48 kHz, stereo |
| `0x03` | PCM: u32 sample rate, then signed 16-bit stereo. For a client that left out `opus` |
| `0x02` | One JPEG, when `video` is `jpeg` |

Both timestamps count from the same moment, so a client that wants to can line sound up with the picture. Show each frame as
soon as it's decoded; don't hold frames to their timestamps.

### Frame pacing

A console doesn't run at 60 frames a second exactly: the NES and Super Nintendo run at 60.0988, the DS at 59.83, PAL
consoles at 50.007. On a 60 Hz screen, a 60.0988 game drops a frame every ten seconds, which shows as a hitch in anything
that scrolls.

- **The server runs the game at the screen's rate** when they're within 1% of each other. It takes `display.hz` from the
  device holding player 1 (or the first that's watching), and the sound is resampled to match, so nothing is audibly
  different: 60.0988 to 60 is 0.16% slower. `pace` in the stream says when it's doing this.
- **Apps show each frame at the next refresh,** and line frames up with it using `0xF6`: when frames land just after a
  refresh, ask for them earlier. `PhaseLock` in WittyPlay's `web/latency.ts` is the browser's version. An app knows its
  refresh exactly (`CADisplayLink`, `Choreographer`), which makes this simpler than in a browser.
- **A TV app switches the display to the game's rate** when the TV has it: 50 Hz for a PAL game, through
  `AVDisplayManager.preferredDisplayCriteria` on tvOS and `Surface.setFrameRate` on Android TV. Then say the new rate with a
  `report` (below), and the server matches it.

### Sound and picture together

Sound and picture are separate streams, so that sound's buffer never delays the picture. Sound is behind by whatever the
device holds back to play smoothly: 35 to 80 ms in a browser. Apps can do better: hold 20 to 40 ms and follow
`audio.latency` from the person's settings.

### Stats

`{"type": "stats", "on": true}` on `events` (or as text on the WebSocket) asks for stats once a second; `"on": false` stops.

```json
{
  "type": "stats", "gameFps": 60.1, "sentFps": 60.1, "encodedFps": 60.1, "encodeMs": 0.7, "kbps": 5800,
  "width": 512, "height": 448, "encoder": "libx264", "speed": 1, "paused": false, "pace": 60,
  "conn": {"transport": "webrtc", "video": "h264", "audio": "opus", "kbps": 5800, "share": 1, "rttMs": 2, "lost": 0, "queueMs": 0, "dropped": 0, "seats": [0]}
}
```

`share` is how much of the full bitrate is being sent after matching the network. `rttMs` and `lost` come from WebRTC's
reports; `queueMs` and `dropped` are what a WebSocket connection is waiting on and has lost to waiting.

A client can report what it sees: `{"type": "report", "hz": 60, "delayMs": 12, "rttMs": 24, "jitterMs": 3, "decodeMs": 1.1,
"displayMs": 6, "dropped": 0}`. Every field is optional. `hz` is the screen's refresh rate, for pacing; send it when it
changes. `delayMs` matters over a WebSocket: how much later than the quickest lately the frames are arriving, from each
`0x11` frame's timestamp against when it arrived (the web player's `ArrivalDelay`). Send the worst of each half second,
twice a second. The server lowers the bitrate when it keeps growing, as it does for WebRTC's round trips. The rest is
for the server's log.

### Errors

`POST /play/{sid}/stream` and the WebSocket's upgrade answer as any route does, with a `code` beside `error`:

| Status | `code` | Meaning |
|---|---|---|
| 400 | `bad-hello` | The hello is missing or wrong. `error` says which field |
| 401, 403 | | Sign in again, as for any route |
| 404 | `no-session` | No such game for this account, or it has ended |
| 413 | `too-large` | The offer is over 64 KB |
| 415 | `no-codec` | Nothing in common: no H.264 or VP8 in the offer, or none this server can encode |
| 424 | `no-module` | WittyPlay isn't installed on this server |
| 429 | `busy` | Too many connections at once. Wait for `Retry-After` |
| 500 | `failed` | Something went wrong starting it. Try again, then fall back |

While connected, refusals come as `{"type": "error", "code": "...", "text": "..."}`: `seat-taken` (with `port`), `bad-seat`
and `bad-message`. The game ending comes as `{"type": "ended", "code": "...", "text": "..."}`, and the connection then closes:

| `code` | Meaning |
|---|---|
| `stopped` | It was saved and stopped: by `DELETE /play/{sid}`, or because the account started another game somewhere |
| `quit` | The game shut itself down |
| `crashed` | The emulator stopped or stopped answering. What was saved stays |
| `idle` | Nobody pressed anything for 30 minutes |
| `evicted` | Too many devices connected: this, the oldest, went |

`text` is written for a person and can be shown as it is.

### Rate

The server starts at a bitrate for the picture's size, at half that for a device it counts as away from home, and never
above the lowest `maxKbps` of the devices watching. It then follows the network: from WebRTC's receiver reports (loss and
round trips), and over a WebSocket from the client's `delayMs` reports, how long frames wait to be written, and frames
dropped. It comes down quickly and goes back up slowly. One encoder serves every device watching, so the device with the
worst network sets the rate for all; a game is usually watched on one screen, and a controller-only device costs nothing.

## What each platform builds

**Browsers** (WittyPlay's player): done. It sends a hello through `/stream` on both transports, holds seats for the players
its controllers are on, decodes Opus over the WebSocket with WebCodecs, reports the screen's refresh rate, and keeps rumble
to its `ms`. Not yet: several fingers, motion and lights, which browsers give pages little of.

**Apple** (Apple TV, iPhone, iPad, Vision Pro):

1. Add LiveKit's `webrtc-xcframework` (Swift Package, tvOS 17 or later). Take decoded frames as `CVPixelBuffer`s through an
   `RTCVideoRenderer` and draw them with the existing Metal path, so `display.*` effects still apply.
2. Connect as above: data channels `input` and `events`, the hello with `video: ["h264"]`, the seats of the controllers
   connected, and `display.hz` from the screen.
3. Keep the current `ws-h264` code as the fallback, moved to `/stream` with a hello, and decode Opus with `AVAudioConverter`
   (`kAudioFormatOpus`), falling back to PCM by leaving `opus` out. Send `delayMs` reports twice a second from the `0x11`
   timestamps, or a slow network fills with video.
4. Rumble from the `rumble` event through Core Haptics: `motor` `strong` on the controller's `.leftHandle` locality and
   `weak` on `.rightHandle` (or both on `.default` for one motor), each for `ms` unless renewed.
5. Motion from `GCController.motion` (gravity plus user acceleration in g, rotation rate in radians a second), or Core Motion
   on an iPhone or iPad held as a pad, while `motionOn` lists a player this device holds.
6. Several fingers with `0xFE` on iPhone and iPad touch play; the Siri Remote stays one pointer.
7. `led` events and `seats` on the controller's lights: `GCController.playerIndex`, and `GCDeviceLight` for a DualSense's
   light bar.
8. On tvOS, switch the display to the game's `fps` with `AVDisplayCriteria` where the TV has it, and send a `report` with the
   new `hz`.

**Android** (phones, tablets, Android TV):

1. Add `io.github.webrtc-sdk:android`. Render frames through a `VideoSink` into the player's own surface, so effects still
   apply; libwebrtc decodes H.264 with MediaCodec.
2. Connect as above, with the same hello. Phones with `"watch": false` can be controllers for a game on a TV.
3. The WebSocket fallback through `/stream`, with Opus decoded by `MediaCodec` (`audio/opus`), or PCM, and `delayMs`
   reports twice a second from the `0x11` timestamps.
4. Rumble through the controller's `VibratorManager`: with two vibrators, `strong` on the first and `weak` on the second,
   each for `ms` unless renewed; the phone's own for touch play.
5. Motion from the controller's own sensors (`InputDevice.getSensorManager()`, Android 12 and later) or the phone's
   `SensorManager` when it's the pad, converting m/s² and rad/s to the wire's units, while `motionOn` lists a player this
   device holds.
6. Several fingers with `0xFE` for touch play.
7. `led` events and `seats` on the controller's lights (`InputDevice.getLightsManager()`, Android 12 and later).
8. On Android TV, call `Surface.setFrameRate(fps, FRAME_RATE_COMPATIBILITY_FIXED_SOURCE)` and report the new `hz`.

**Every app:** `/play/{sid}/stream` is the only way to connect to a game.

## Games in a standalone emulator

A game started with `POST /games/{id}/play/emulator` is not a session of this protocol, though it borrows its WebRTC half. WittyPlay relays RTP that an FFmpeg beside the emulator made, so there is no WebSocket mode, no `timing` channel, no seats, no rewind and no stats message; the hello is checked the same way, the answer is the same `Connected`, and the first `events` message is the same stream description. Its bitrate follows what the browser reports losing, in a few large steps. See [the API](GAMING-API.md#playing-with-a-standalone-emulator).

## Where it lives

- WittyPlay: `stream/connect.go` (the hello, the stream and seats), `stream/viewer.go` (both transports), `stream/wsv2.go` (the
  WebSocket), `stream/feedback.go` (rate), `stream/haptics.go` (rumble, lights, motion), `libretro/libretro.go` and
  `libretro/sensor_*.go` (the sensor and LED interfaces, several fingers), `host/` (pacing, and carrying them to the core), `web/transport.ts`, `web/arrival.ts` and `web/refresh.ts` (the
  browser's side), `cmd/wittyplay-probe` (measuring, with `-v2`, and `-impair` on TCP as well as UDP).
- WittyMediaServer: `internal/server/api_games.go` (the two routes, and which devices count as away), `internal/play/stream`
  (the module's interface, generated).

---

<sub>WittyMediaServer documentation</sub>
