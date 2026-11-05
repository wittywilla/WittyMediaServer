# Design: WittyTV channels that stay live, in sync, and where you left them

> **Status on 2026-11-04**
>
> **In progress:** full screen without retuning [#67](https://github.com/wittywilla/WittyMediaServer/issues/67), the guide's focus [#68](https://github.com/wittywilla/WittyMediaServer/issues/68). **Planned:** everyone on a channel in sync, [the idea](https://github.com/wittywilla/WittyMediaServer/discussions/47).

For WittyTV's next major update (it ships in the server, so with 0.7.0-alpha, or 0.6.3-alpha for the smallest part). The owner:

> TV Channels stop watching when you full screen or leave full screen. Then it retunes causing a gap in playing, and starts from a slightly earlier time as well. We need to fix this behavior so that if multiple users try to watch a channel at the same time on different accounts, it is synced across it. Also, when exiting a channel, the channel that is selected is not the same as what was just watched.

Not built. Not tested. No test was run to confirm the causes below: they're read from the published code and docs, and each one is marked **to verify** before it's fixed.

---

## Three problems

1. Going full screen, or leaving it, stops the channel and tunes it again: a gap, and it comes back a little earlier than it was.
2. Two people watching the same channel on different accounts aren't at the same moment.
3. Leaving a channel, the guide highlights a different channel from the one just watched.

## What the code does today

From `docs/CHANNELS.md` and `web/src/pages/tv/TvPlayer.tsx`:

- **The server already shares one stream per channel.** Everyone on a channel gets one FFmpeg run, paced to real time, as a rolling HLS timeline (`internal/hls/live.go`). FFmpeg stops about 45 seconds after the last request, and about 2 seconds after the last player says it left.
- **The player tunes in a React effect keyed on the channel.** On mount it calls `tune(key, viewer)`, waits for the first picture (`preflight`), then hands the playlist to hls.js (`useSource` in `web/src/player/hooks.ts`). **On unmount it calls `leaveChannel(key, viewer)`** straight away.
- **hls.js starts at `startPosition: -1`** for a live stream (its default: a few target durations behind the live edge), with `lowLatencyMode: false`.
- Full screen is `toggleFullscreen(root.current)` on the player's own element.
- The previous channel is remembered with `rememberChannel(key)` / `previousChannel()`.

### Likely causes (to verify)

**Problem 1.** `toggleFullscreen` on the player element shouldn't remount anything by itself. A remount happens if the page renders the player in a different place depending on full screen or layout: for example the guide's **preview** and the full **watch** view being two different `TvPlayer` instances, or a layout change that moves the player under a different parent, or a `key` that changes. Any of those runs the cleanup (`leaveChannel`) and a fresh `tune`, with a new hls.js instance that starts at its default position behind the edge: the gap, and the earlier time. The 2-second stop after leaving makes it worse: if the new tune lands after the stream stopped, FFmpeg starts again from the schedule, which takes longer.

To verify, in order: log mounts of `TvPlayer` and calls to `tune` and `leave` while toggling full screen from the watch view, from the preview, with F, with the button and with a double-click; check whether the browser's own full screen (Escape, F11, a TV remote's back) takes a different path from ours.

**Problem 2.** One shared stream doesn't mean one shared moment. Each hls.js starts "a few segments behind the edge" *of what it saw when it joined*, buffers differently, and drifts with stalls. Two viewers can be several seconds apart, more after a stall. Nothing today tells players what wall-clock moment a frame belongs to.

**Problem 3.** The guide's highlighted row comes from the guide's own state, while channel changes made inside the player (Up, Down, dialling, Backspace for the previous channel) update `rememberChannel` but not the guide's focus. Returning to the guide restores the old highlight, or the first row.

---

## 1. Full screen never retunes

### Rule

**One player, one stream, for as long as the person stays on a channel.** Changing size, going full screen, opening the mini guide, showing the remote, or moving from the preview to the watch view are all changes of *where the picture is drawn*, never of *what's playing*.

### How

- **A persistent player host.** One `<video>` and one hls.js instance live in a component mounted once at the TV section's root, outside the routes that change. The preview, the watch view and full screen are slots it's moved into (CSS position and size, or a portal into a stable node). The video element is never detached from the document, as detaching pauses it in some browsers.
- **Full screen on the host's container**, not on a route's element, so the element that goes full screen is the one already playing.
- **Leaving is deferred.** `leaveChannel` is sent 3 seconds after the player stops wanting a channel, and cancelled if the same viewer wants the same channel again in that time. `pagehide` still sends it at once, with `navigator.sendBeacon`, so a closed tab frees the channel promptly.
- **The server's quick stop waits for that.** The "2 seconds after the last player leaves" stays, counted from the leave that actually arrives.

### Done when

On the web, Electron and the Android web view: toggling full screen 20 times by F, button, double-click, Escape and a TV remote's Back makes **zero** `tune` calls after the first, zero `leave` calls, and no gap in `video.currentTime`. Moving from the preview to the watch view is the same. The banner may show again; the picture doesn't stop.

---

## 2. Everyone on a channel sees the same moment

### The idea

A channel is a schedule: `GenerateDay` is a pure function, so the server knows, for any wall-clock instant, which programme and which second of it is "on the air". The stream should say so, and players should keep a fixed distance behind it.

### Server

- **`EXT-X-PROGRAM-DATE-TIME` on every segment** of the live playlist, set from the schedule's wall-clock time (not FFmpeg's start time), so a segment says "this is 20:14:06.000 on the air".
- **A server clock** for players: `GET /api/time` (signed in, tiny) returns the server's time, so players can work out the offset of their own clock with an NTP-style round trip (send time, server time, receive time; best of three). Devices with a wrong clock are common, especially TVs.
- **`EXT-X-SERVER-CONTROL` with `HOLD-BACK`** set to the target latency, so standard players (native apps too) know where "live" sits.
- **A target latency per server**, default **6 seconds** behind the air time, in **Settings → Channels & tuners → Options**. Lower is possible with LL-HLS later (see below); 6 s with today's segment length keeps the shared stream as light as it is now.

### Web player

- hls.js with `liveSyncDuration` set to the target, and program-date-time used to compute **drift**: `drift = (playing PDT) − (server now − target)`.
- **Small drift (under 0.5 s):** nothing.
- **Medium (0.5 s to 3 s):** play at 1.03× or 0.97× until caught up. Audio keeps its pitch (`preservesPitch`, on by default in browsers). Nobody notices 3%.
- **Large (over 3 s), or after a stall:** seek straight to the target.
- Checked every 2 seconds, from timestamps already in hand: no extra requests.

### Pause and rewind still work

Pausing, rewinding within the pause buffer (5 minutes on library channels, 30 on tuners), and Left and Right leave sync **on purpose**. The banner shows "Behind live by 1:24". **L** (Back to live) and the "Live" button return to the synced point, not to the raw edge.

### Native apps

Both platforms have this built in, from the same tags:

- Android (ExoPlayer / Media3): `LiveConfiguration` with `targetOffsetMs`, and its speed control for catching up.
- Apple (AVPlayer): `configuredTimeOffsetFromLive` and `automaticallyPreservesTimeOffsetFromLive`.
- The desktop client runs the web player.

### Lower latency later

LL-HLS (partial segments) can bring the target down to about 2 seconds, for people watching sport across rooms. It costs more requests per viewer, so it's an option, not the default, and comes after this lands.

### Done when

Two accounts on two devices (a browser and the Android app on the emulator; later a real TV) tuned to one channel stay within **0.25 seconds** of each other for 30 minutes, measured by showing a burned-in clock card from the channel (a test channel of the server's own) and comparing frames; after a forced stall on one, they're back together within 10 seconds.

### What this gives "watch together"

The roadmap's watch-together feature needs exactly this clock (server time, offset, drift correction). Build it once here, in `web/src/player/sync.ts` (name to choose), and reuse it.

---

## 3. The guide highlights what you were watching

### Rule

Leaving a channel by any route (Escape, Back, the guide button, a TV remote's Back) lands on the guide with **the last tuned channel highlighted, scrolled into view, and the time column at now**.

### How

- The TV section keeps one `lastTuned` value, updated on every tune, including channel up and down, dialling, the previous-channel key, the mini guide and the on-screen remote.
- The guide takes its initial focus from `lastTuned` whenever it's entered from a channel. Entered any other way (from the top bar), it keeps today's behaviour.
- Screen readers hear the row: "Channel 12, Cartoons, now: …, highlighted".
- Native apps get the same: `lastTuned` is per device, so each app keeps its own, using the tune answer it already has.

This part is small and can be pulled forward into 0.6.3-alpha.

### Done when

On web, Electron and Android TV: tune 5, change to 9 with Up, then 23 by dialling, then Back: the guide shows 23 highlighted and on screen. Same by remote and by mouse.

---

## Tests to add

- A unit test for the drift controller (fed with synthetic PDTs and clocks).
- A browser test: full screen toggles make no network calls to `tune` or `leave`.
- A two-browser test on Linux with a test channel that burns the wall clock into the picture: compare the two at intervals.
- `npm run test:tv` extended for focus after leaving, in every TV theme and size.

## Costs

- Server: one tag per segment, one tiny route. Nothing per viewer.
- Players: one timer every 2 seconds and the occasional speed change. No extra requests.
- The persistent host keeps one video element alive while in the TV section, which it effectively already does while watching.

---

<sub>Future plans, written 2026-11-02. Not built, not tested.</sub>
