# Channels and live TV (WittyTV)

Always-on TV channels made from the libraries, and live TV from tuners and IPTV, side by side in
one programme guide with the look of an early-2000s cable or satellite box.

This page is the design. [What works and what is left](#status) is at the end.

## The ideas it is built on

1. **One channel model, three levels of detail.** Simple, Rules and Full timetable are three views
   of the same saved definition. Moving between them never discards anything.
2. **A day at a time, only when asked.** A day's schedule is worked out the first time something
   needs that day (the guide or someone tuning in), then kept, so the guide never changes under the
   viewer. An idle channel costs nothing.
3. **Deterministic.** The same definition, seed and library always give the same schedule.
4. **FFmpeg runs only while someone is watching,** and everyone on a channel shares one stream.
5. **Encrypted broadcasts are refused, never decrypted.** The same rule as CSS and AACS on discs.

## Library channels

### The definition

```
Definition
  mode      simple | rules | timetable    how much of the editor the owner has opened
  rules     what may play by default      library, genre, network, decade, rating, show, season,
                                          watched state, length, kind, hand-picked items…
  order     how it plays                  rotate | sequential | aired | shuffle, keep episodes in order
  blocks[]  day parts and timetable slots days, start, end, own rules, own order, fit
  filler    between programmes            rules for bumpers, how many between shows
  alignMin  start times snap to a grid    0, 5, 15, 30 or 60 minutes
```

Day parts and hand-built timetable slots are both **blocks**. "Weekdays 18:00–18:30, The Show, next
episode" is one block with `rules.shows=[id]`; "Saturday 07:00–12:00, Animation" is another. Time no
block covers plays the channel's own rules, or a "We'll be right back" slate if there are none.
In `simple` mode the blocks are ignored but kept, so going back to Rules finds them again.

Each save also keeps the previous definition (the last 20, restorable) and the editor keeps an
unsaved draft in the browser, so a closed tab does not lose work.

### How a day is scheduled

`GenerateDay` is a pure function of the definition, the seed, the day and the state left by the
previous day (where each pool had got to, and when the last programme ended).

- Each block's pool is an SQL query over items, in a stable order.
- A cursor per pool (shared by every slot with the same rules and order) remembers the last item by
  id, so a strip of weekday slots carries on from yesterday's episode, and a newly scanned episode
  slots in without disturbing it.
- **rotate** takes the next episode of each show in turn. **shuffle** uses a seeded permutation per
  cycle, so nothing repeats until the pool is used up.
- When a programme will not fit before the block ends, the block's **fit** decides: `fill` (the
  default) looks for one that fits, or fills the gap so the next block starts on time; `overrun`
  lets it finish; `cut` stops it at the block's end. A programme longer than the whole block runs
  over, so a block can never get stuck.
- Gaps (from the time grid or a block end) are filled with filler that fits, then a slate.

Days skipped because nobody watched are simulated, not stored, so an unwatched channel still moves
on as real TV would. A definition change applies right after whatever is on now.

### Tune in mid-show

`At(now)` finds the programme and how far into it we are. Library channels use a shared live HLS
session (below) that starts FFmpeg at that position. When the run ends the feeder asks the schedule
again, so a slow machine catches up rather than drifting.

Discs play their main title through the same engines as the disc player, with no menus or remote
features. Scrambled discs are left out of every pool.

### Known TV networks

`shows.network` already comes from TVmaze, TheTVDB, AniList and NFO files. Two things were added:

- `shows.air_days` and `shows.air_time`: the slot a show aired in, where TVmaze or TheTVDB say.
- *Fill in missing network facts*, a background job that asks the metadata services about shows
  already in the library without rescanning anything.

One click builds a channel in one of these styles:

| Style | What it does | Needs |
|---|---|---|
| All day | The network's shows in rotation | a network on the show |
| Era | The same, limited to a decade or years, by each episode's air date | air dates |
| Day parts | Morning, afternoon, early evening, prime time and late night, each from the genres that suit it | genres |
| As it aired | Each show in its original day and time, the network's other shows around it | air slots |

A style is offered only when the library's data supports it. Network names are plain text facts
from metadata: nothing is bundled, no logos, and owners can upload their own channel logo.

### Who sees what

A channel is shown to an account only when all of these hold:

- the account may use library channels (`Access.LiveTV`, `LibraryChannels`);
- the channel's visibility lets them in (everyone, adults only, or chosen profiles);
- **every** title the channel could play is one that account may see. This is one SQL query:
  "is there an item matching the channel's rules that the account's limits exclude?".

Everyone on a channel shares one stream, so a mixed channel cannot show a Kids profile only its
child-friendly half. It is hidden from that profile instead. Tuning in checks the current programme
again, and every playlist and segment request checks the channel.

Counting towards watch progress is optional per channel and off by default. Bumpers never count.

## The shared live stream

`internal/hls/live.go` adds live sessions beside the existing on-demand ones.

```
feed(at) -> LiveInput          what to play now: a file and offset, a tuner stream, a slate
  run FFmpeg (paced to real time) -> rN-00000.ts, rN.m3u8
  read rN.m3u8 -> a rolling timeline of segments, with a discontinuity between runs
  drop segments older than the window
```

- **Light:** paced to real time with a short start-up burst; video is copied rather than converted
  when the source is progressive H.264 (setting). Audio is always stereo AAC at 48 kHz, so a stream
  stays playable across programmes.
- **Shared:** one session per channel. Viewers only read files.
- **Idle stop:** about 45 seconds after the last playlist request FFmpeg stops, segments are deleted
  and any tuner is released.
- **Limit:** live sessions that convert count against `max-transcodes`, and the viewer is told
  plainly when the server is full.
- **Pause and rewind:** the window is the time-shift buffer, on disk in the transcode folder: 5
  minutes for library channels and 30 for tuners by default (*Settings → Channels & tuners*).

## Tuners (`modules/wittytuner`)

A Go module of its own with no dependencies: `go.work` and a `replace` directive wire it in, like
the disc engines. A driver implements one small interface:

```go
type Source interface {
    Info() Info
    Channels(ctx) ([]Channel, error)                  // what the source knows now
    Scan(ctx, ScanOptions, progress) ([]Channel, error) // ask it to look for channels
    Open(ctx, Channel) (*Stream, error)               // raw MPEG-TS, or FFmpeg input options
    Close() error
}
```

Raw MPEG-TS passes through `mpegts.Guard` on its way to FFmpeg, which reads the scrambling bits (and
collects guide data), so an encrypted stream stops with a clear message. Drivers describe their own
settings, so the web form is generated from them and a new driver needs no UI work.

### Drivers

| Driver | How it works | Platforms | Tested |
|---|---|---|---|
| **IPTV: M3U playlist** | Downloads or reads the list, with `tvg-*` and `group-title` | all | **Real FFmpeg against local streams**, parser unit-tested |
| **IPTV: single stream** | One HTTP, HLS, RTP, UDP or RTSP address | all | **Real FFmpeg against local streams** |
| **Network tuner** (HDHomeRun-style) | UDP and SSDP discovery, `discover.json`, `lineup.json`, scan API, HTTP MPEG-TS | all | **Against a mock device**; no real hardware here |
| **SAT>IP** | SSDP discovery, RTSP tuning with the standard query, optional M3U from the server | all | Protocol tests only, **untested on hardware** |
| **Linux DVB** (DVB-T/T2/C/S/S2, ATSC, ISDB-T) | `/dev/dvb` ioctls, band-plan scan, PAT/SDT/VCT reading, `channels.conf` import | Linux | Parsers tested, **untested on hardware**. DiSEqC is not done |
| **V4L2 and analogue capture** | `/sys/class/video4linux`, frequency and input via ioctl, FFmpeg capture | Linux | Argument tests only, **untested on hardware** |
| **Windows capture** (DirectShow) | FFmpeg `dshow` device listing and capture | Windows | Listing tested on this machine, **untested with a card** |
| **macOS capture** (AVFoundation) | FFmpeg `avfoundation` listing and capture | macOS | Parser tests only, **untested** |
| **External backend** | Open protocols: an HTTP M3U and XMLTV API, or SVDRP with an HTTP TS streamer | all | **Against mock servers** |
| **Custom command** | Your own program writes MPEG-TS to standard output | all | Tested with a small test program |

Listed, not built:

- **Windows BDA digital tuning.** It needs a COM graph. Windows capture cards work through
  DirectShow, and a BDA tuner can be used today through an external backend.
- **ATSC huffman-compressed guide text.** Such events are skipped.

The custom command runs a program on the server, so a web session alone must not be able to set one
up. It stays off until the owner starts the server with `WMS_TUNER_COMMANDS=1`.

### Encrypted and copy-protected channels

Never decrypted, never bypassed. Each is detected and refused with a clear message, the same way
the disc engines treat CSS and AACS:

| Where | How it is detected |
|---|---|
| Network tuners | `DRM: 1` in the lineup (copy-protected CableCARD channels) |
| DVB, SAT>IP | `free_CA_mode` in the SDT, CA descriptors in the PMT |
| Backends | The channel's CA field |
| Any MPEG-TS | `transport_scrambling_control` in the packets, also mid-stream |
| IPTV | DRM licence lines in the playlist, and any `#EXT-X-KEY` in an HLS playlist. FFmpeg's `crypto` protocol is left off its whitelist as a second lock |

Refused channels stay listed, marked with a padlock and the reason. They cannot be tuned or recorded.

No channel lists or stream addresses ship with the server. Scanning uses computed band plans (the
standard channel-to-frequency formulas), not lists of stations.

## Guide data

- **XMLTV:** from an address or a file, on a schedule, gzip included. Matched on `tvg-id`, then on
  the channel's name, with a manual override per channel. A playlist's `url-tvg` and a backend's
  guide address are offered as sources automatically.
- **Over the air:** DVB EIT and ATSC PSIP, read by `mpegts.Guard` while a channel is being watched,
  recorded or scanned. It costs nothing when idle, and only channels recently tuned have this data.
- **Library channels:** their own schedule.

All of it is one guide. `GET /api/guide` returns any window of any channels in one format, so the
page does not care where a programme came from. It loads only the rows on screen, so a playlist of
thousands of channels stays quick.

## DVR

- Record one programme from the guide, or every airing of a title on a channel.
- **Tuner channels only.** Library channels cannot be recorded, because their programmes are already
  your files (the guide says so).
- A raw MPEG-TS stream is written straight to disk, using almost no CPU. Other sources use FFmpeg
  in copy mode.
- Recordings go to their own folder by default (`<data>/recordings`), or into a chosen library's
  folder, which is then rescanned.
- Recordings come first: when no tuner is free, a live viewer on that tuner is stopped, with a
  message. A viewer and a recording do not yet share one tuner.

## The classic look

The guide and the channel banner are styled after set-top boxes of the early 2000s, with original
artwork and colours (no real provider's name, logo or layout):

- a royal-blue grid with bevelled cells, a highlighted cell in yellow, and half-hour columns;
- an information panel on top (the title, times, rating and description of the highlighted
  programme) with a clock;
- a help strip along the bottom with the coloured A, B and C buttons of a remote;
- when tuning: a banner with the channel number, name, times, a progress bar and what is next;
- number keys to dial a channel, CH+ and CH− with the arrows or Page keys, **Last** to go back.

Three skins: **Cable** (blue), **Satellite** (near-black with amber) and **Theme** (follows the
server's theme). All are checked against WCAG AA.

The grid is a real ARIA grid. Arrow keys move between programmes with a roving tab stop, every cell
is read as "Channel 101, Comedy, The Show, 8:00 to 8:30 PM, on now", and it works with TV remotes,
game controllers and on phones (a narrower channel column and sideways scrolling).

The player reuses the on-screen remote and the shared controls, and adds channel keys.

## API

| Method and path | Who | What |
|---|---|---|
| `GET /api/guide?from&hours&offset&limit&kind&group` | signed in | Channels and programmes |
| `GET /api/tv/{key}/tune` | signed in | What is on, and the stream |
| `GET /api/tv/{key}/live/playlist.m3u8`, `/seg/{name}` | signed in | The shared stream |
| `GET /api/channels` | signed in | Library channels this account may see |
| `POST /api/channels`, `PUT`/`DELETE /api/channels/{id}` | admin | Manage |
| `GET /api/channels/{id}`, `/history`, `POST /restore` | admin | Definition and earlier versions |
| `POST /api/channels/preview` | admin | Size and first day of a draft, without saving |
| `GET /api/channels/suggestions`, `/options` | admin | One-click channels; choices for the editor |
| `POST /api/channels/{id}/rebuild` | admin | Regenerate after what is on now |
| `POST /api/channels/networks/refresh` | admin | Fill in missing network facts |
| `PUT`/`DELETE /api/channels/{id}/logo`, `GET` (signed in) | admin | The owner's own logo |
| `GET /api/tuners/drivers`, `/discover` | admin | What can be added; what is on the network |
| `GET`/`POST /api/tuners`, `PUT`/`DELETE /api/tuners/{id}` | admin | Manage |
| `POST /api/tuners/{id}/scan`, `GET` for progress | admin | Find channels |
| `GET /api/tuners/{id}/channels`, `PUT /api/tuner-channels/{id}` | admin | Turn on or off, renumber, map guide data |
| `GET`/`POST /api/epg/sources`, `DELETE`, `POST …/refresh` | admin | XMLTV sources |
| `GET`/`POST /api/recordings`, `DELETE /api/recordings/{id}` | signed in, may use tuner channels | DVR |
| `GET`/`PUT /api/tv/settings` | admin | Buffers, DVR folder, conversion limits |

## Status

See [TV.md](TV.md) for the viewer's guide, and the driver table above for what has been tested on
real hardware (nothing yet: no tuner was available while this was built).
