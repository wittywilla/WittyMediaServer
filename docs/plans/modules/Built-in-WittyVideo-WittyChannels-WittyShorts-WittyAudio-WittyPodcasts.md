# The built-in modules: update plans

> **Status on 2026-11-04**
>
> **In progress:** the small UI fixes, sort direction and hearing radio and music ([#95](https://github.com/wittywilla/WittyMediaServer/issues/95)), smaller pictures ([#100](https://github.com/wittywilla/WittyMediaServer/issues/100)), lyrics ([#102](https://github.com/wittywilla/WittyMediaServer/issues/102)). **Planned:** [studios](https://github.com/wittywilla/WittyMediaServer/discussions/26), [series folders](https://github.com/wittywilla/WittyMediaServer/discussions/36).

WittyVideo, WittyChannels, WittyShorts, WittyAudio and WittyPodcasts are part of the server and update with it (`delivery: builtin`). WittyTV (guide and channels) has [its own plan](WittyTV.md). Music and radio sit in WittyAudio.

---

## WittyVideo (films, shows, home videos)

| Release | Theme |
|---|---|
| 0.6.3-alpha | Small UI fixes ([list](../releases/0.6.3-alpha.md#small-ui-fixes)) |
| 0.7.0-alpha | Studios that are one studio; poster picker; the minimum hardware doc |
| 0.8.0-alpha | Right-click menus; library customisation; watch together |
| Later | Skip intro and credits shared across episodes by audio fingerprint (thumbnails and intro detection get tasks of their own first); trailers and extras from the person's own folders; HDR to SDR tone mapping on more hardware encoders (today hardware encoders only for 8-bit) |

Library transcoding (our own FFmpeg engine, optional Tdarr connection): automatic runs exist. Later: worker nodes of our own stay out (a 0.6.0 decision); better reporting of what each run saved.

## WittyChannels

See [WittyTV](WittyTV.md). Channel definitions: later, import and export a channel as a file to share between servers (data only, validated).

## WittyShorts

| Release | Theme |
|---|---|
| Next | Vertical video player polish on phones and TVs; creator pages from folder structure |
| Later | Chapters and timestamps from description files; offline downloads in the native apps |

## WittyAudio (music, radio, lyrics, karaoke)

| Release | Theme |
|---|---|
| 0.6.3-alpha | **Hear it:** 0.6.0 never heard sound in tests (the radio test stream was on 127.0.0.1, which the server refuses). Play a real station and a real album |
| 0.7.0-alpha | Sort direction on CDs, Music and Books (0.5.2 audit) if still open; richer music browsing in the native apps (not built in 0.6.0) |
| 0.8.0-alpha | Background audio and picture in picture on Android (not built in 0.6.0); mini player everywhere |
| Later | Smart playlists by rules; crossfade and gapless across albums; replay gain; synced lyrics editing by the owner; karaoke scoring by pitch (local, off by default) |

## WittyPodcasts

| Release | Theme |
|---|---|
| Next | Feed refresh shown in the activity panel; per-feed limits (keep last N episodes) |
| Later | Chapters, transcripts where the feed provides them (Podcasting 2.0 tags), playback speed with pitch kept, sync of positions in native apps |

---

<sub>Future plans, written 2026-11-02. Not built, not tested.</sub>
