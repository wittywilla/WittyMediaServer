# WittyTV: update plan

> **Status on 2026-11-04**
>
> **In progress** in 0.7.0-alpha: [#67](https://github.com/wittywilla/WittyMediaServer/issues/67), [#68](https://github.com/wittywilla/WittyMediaServer/issues/68), commercials [#76](https://github.com/wittywilla/WittyMediaServer/issues/76), resetting a channel [#77](https://github.com/wittywilla/WittyMediaServer/issues/77), custom channels' own page [#78](https://github.com/wittywilla/WittyMediaServer/issues/78). **Done:** the live TV playlist proxy (`internal/livetv/hlsgate.go`). **Planned:** [sync](https://github.com/wittywilla/WittyMediaServer/discussions/47), [keep off channels](https://github.com/wittywilla/WittyMediaServer/discussions/48), [languages](https://github.com/wittywilla/WittyMediaServer/discussions/49), [tuners](https://github.com/wittywilla/WittyMediaServer/discussions/50), [series folders](https://github.com/wittywilla/WittyMediaServer/discussions/36).

**What it is:** the TV guide, channels, preview and watching, across library channels (built-in WittyChannels) and tuner and IPTV channels (WittyTuner). The guide and player live in the server (`web/src/pages/tv`, `internal/livetv`, `internal/hls/live.go`, `internal/channels`), so WittyTV's updates ship with server releases.

**Now (0.6.0-alpha):** nine themes (modern, classic and more, with colours from the overall theme and overrides), scalable preview, mini guide, on-screen remote, bumpers, commercials and "cut commercials short", DVD main features and scenes on channels, one-click channels, logos with notices, reminders, favourites, recordings, encrypted-channel refusal.

| Release | Theme |
|---|---|
| 0.6.3-alpha | Guide highlights the channel you were watching; the final accessibility pass in every theme |
| **0.7.0-alpha** (next major for WittyTV) | Full screen never retunes; everyone on a channel in sync |
| 0.8.0-alpha | Watch together on top of the same clock; live TV and guide in native apps |
| Later | Low latency mode; series-folder recordings; tuner sharing; more |

---

## 0.6.3-alpha

- **Guide focus after leaving a channel** (the owner's third WittyTV point): small enough to come forward. [Design](../design/wittytv-live-sync.md#3-the-guide-highlights-what-you-were-watching).
- **The vital verification** the owner asked for in 0.6.0: screen readers, keyboard only, every kind of controller, in every TV theme, on every system (`npm run test:tv` in a browser, Electron and the Android web view).
- Side menu's brand name cut off in big screen mode (reported again after a fix).
- Not every new dialog seen in both light and dark (wittytv's note): check.
- Real logo downloads, never tried.

## 0.7.0-alpha: live, in sync, and no retuning

Full design: [../design/wittytv-live-sync.md](../design/wittytv-live-sync.md).

1. **Full screen never retunes:** one persistent player in the TV section, moved between preview, watch view and full screen; leaving deferred 3 seconds and cancelled by coming back.
2. **Everyone on a channel sees the same moment:** program date-time from the schedule's own clock on every segment, a server clock route, a target latency (default 6 s), drift correction by tiny speed changes, standard hold-back for native players.
3. Pause and rewind still leave sync on purpose; **Back to live** returns to the synced point.

**Keep a title off TV channels** (the owner, 2026-11-02): a switch on any title, with 3D titles left out by default. [../design/keep-off-channels.md](../design/keep-off-channels.md).

**Also for 0.7.0-alpha, from the existing plans:**

- **Recordings in a series' own folder** (roadmap: not started).
- **Live TV playlist proxy** (decided after 0.5.2): **to verify** whether it was built in 0.6.0's security work; if not, here.
- The Modern guide's preview "no bar" style (the reference look), keeping the resize corner.

## 0.8.0-alpha

- **Watch together** on the clock built in 0.7.0: a shared session any channel or film can start, with a host, invite by profile, shared pause (if the host allows), chat-free by default.
- **Live TV and the guide in WittyMediaClient's native apps** (not built in 0.6.0): a native guide grid with D-pad focus, using `GET /api/guide`, and the same sync from ExoPlayer and AVPlayer's live settings.
- **Android TV channels:** the app's channels on the Android TV home screen (left out of 0.6.0 until tested).

## Also planned (owner, 2026-11-02)

- **Language and subtitles while watching** (0.8.0-alpha, tuner captions after): [../design/channel-audio-and-subtitles.md](../design/channel-audio-and-subtitles.md).
- **Channel rules that require an audio language:** [../design/audio-language-preferences.md](../design/audio-language-preferences.md).

## Later

- **Low-latency mode** (LL-HLS, ~2 s) as an option.
- **A viewer and a recording sharing one tuner**, Windows BDA, DiSEqC (with WittyTuner).
- **Automatic clean-up of old recordings** with rules (keep last N, keep for N days, keep unwatched).
- **Channel surfing preview:** a small picture of what's on other channels in the guide, from tuner thumbnails taken when the stream is already running (never tuning a tuner just for a thumbnail).
- **Parental controls by rating on channels**, using the existing profile limits.
- **Recording from library channels** isn't needed (the files are there), but "Start this programme from the beginning" on a library channel is: jump to the start of the current programme for one viewer, which then leaves sync until **Back to live**.

## Standing rule

Encrypted broadcasts stay refused. No bypass, no hook. Every new source goes through `wittytuner.New`.

---

<sub>Future plans, written 2026-11-02. Not built, not tested.</sub>
