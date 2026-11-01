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
  filler    between programmes            commercials libraries, bumper rules, how many between shows
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

### Commercials

**Commercials** is a kind of library: the owner's own adverts and bumpers. There can be as many as they
like, one for an era, one per channel. Its videos are never looked up online and never become a
channel's programmes. Each library has a switch, off by default, for whether it also shows in the main
menus, on Home and in search (*Settings → Libraries*). Channels can use it either way.

Each channel chooses which commercials libraries it uses, and from each either all the videos or only
the ones ticked. They play after each programme (0 to 5 of them) and fill the gaps to the time grid,
and never count as watched. They come under a profile's limits like anything else: unrated commercials
hide a channel from a Kids profile, since viewers share one stream.

Breaks inside a programme are not done: commercials come between programmes.

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
| **Network tuner** (HDHomeRun-style) | UDP and SSDP discovery, `discover.json`, `lineup.json`, HTTP MPEG-TS, as the maker's guide documents. The scan call is not in that guide | all | **Against a mock device**; no real hardware here |
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

Never decrypted, never bypassed. There is no plugin, hook, setting or command option for it. Each is
detected and refused with a clear message, the same way the disc engines treat CSS and AACS. The check
sits between every driver and the server (`wittytuner.New`), so a driver, or a custom command, can't
leave it out.

A downloaded version of WittyTuner changes none of this. Its helper only reaches network tuners, IPTV
over HTTP and external backends, and the server wraps what it sends in the server's own guard. A
helper saying a channel is clear means nothing: it can only ever refuse one. HLS and DASH are opened
and checked by the server itself, never from options a helper offers.

| Where | How it is detected |
|---|---|
| Network tuners | A `drm` tag in the lineup (the maker's documented marking) or `DRM: 1`, and error 811 when tuning |
| Licensed devices | A DTCP-IP stream type, or a DASH manifest with `ContentProtection` |
| DVB, SAT>IP | `free_CA_mode` in the SDT, CA descriptors in the PMT |
| Backends | The channel's CA field |
| Any MPEG-TS | `transport_scrambling_control` in the packets, also mid-stream |
| IPTV | DRM licence lines in the playlist, and any `#EXT-X-KEY` in an HLS playlist or any of its variants, on opening and again while it plays or records, as often as the playlist changes (every 5 to 30 seconds). A key that appears stops the stream and marks the channel; a recording keeps what was clear. FFmpeg's `crypto` protocol is left off its whitelist, but FFmpeg has no option that stops it asking for a key, so these checks are what keeps encrypted HLS out |

Refused channels stay listed. They cannot be tuned or recorded.

One limit: the check sees scrambling in MPEG-TS packets. Encryption inside the video itself, as on ATSC
3.0, shows only if the device reports it, as the maker's guide says it does. If a device didn't report it,
FFmpeg would fail to decode the channel and the viewer would see an error rather than the label.

**Status from the API, the same on every client.** The guide, the tune answer and the tuner settings give
each tuner channel `security: {state, label, detail, reason, helpUrl}`:

| State | Means | Label |
|---|---|---|
| `encrypted` | The source or the stream says so | Encrypted: can't be played here |
| `clear` | The source says so, or a stream showed it | Clear |
| `unknown` | This source doesn't say (playlists, mostly); checked at first tune | Not checked yet |

What a stream shows is remembered, so a channel found to be encrypted is labelled from then on, and a
scan reports how many of each it found. **Check again** in the tuner's channel list forgets what a stream
showed, for a station that has changed. An encrypted channel is refused again the next time it is tuned.

When a station is encrypted here and clear elsewhere, `alternative` names the clear channel, and every
screen offers it. Stations match by call sign or name, and by number over the air.

### Help for encrypted channels

The label links to a page at `/tv/encrypted`, with its words from `GET /api/tv/encrypted-help`.

**What it is.** Some broadcasters encrypt their channels, so only a device licensed for that encryption
can turn the signal into a picture. Nothing is wrong with the viewer's antenna, tuner or setup.

**Why this server can't play it.** It isn't a licensed device, and it doesn't decrypt or bypass protection
of any kind. It doesn't extract keys from a device, imitate a certified player, or capture protected
(HDCP) HDMI.

**What someone can do:**

1. Watch a clear version of the station, such as its ATSC 1.0 signal. The guide offers one when it has one.
2. Use a licensed device or app certified for encrypted channels.
3. Bring in what a licensed device itself offers to home media servers (next section).

**What to look for, by capability:** an ATSC 3.0 tuner if the station uses it; certification for encrypted
channels, stated by the maker (in the US, the A3SA scheme); for use with this server, a documented network
stream or export that covers encrypted channels specifically; and any limits on recording.

**Devices checked on 2026-10-30, in their makers' own documents.** None of this is a recommendation, and
support can change. **None of these documents a way to pass encrypted channels to other software.**

| Device | What its maker publishes | Source |
|---|---|---|
| SiliconDust HDHomeRun FLEX 4K | 2 ATSC 3.0 and 4 ATSC 1.0 tuners. Some DRM-encrypted ATSC 3.0 channels will not work; it uses the ATSC 1.0 version. Documents an HTTP interface | [product page](https://shop.silicondust.com/shop/product/hdfx-4k/), [HTTP guide](https://info.hdhomerun.com/info/http_api) |
| ZapperBox | Tuner models work with encrypted channels and record them; encrypted recordings play only on the device that made them. APIs documented for discovery and remote keys. Reaching other devices is planned through its own apps. zApp converts recordings to standard HLS; clear ATSC 3.0 recordings then lack sound in players without AC-4 | [FAQ](https://zapperbox.com/pages/faqs), [gateway post](https://zapperbox.com/blogs/blog/the-unbound-gateway) |
| ADTH NextGen TV USB | Certified to the A3SA standard, online, on Android TV, Google TV and Fire TV hosts, which do the decoding | [support](https://support.adth.com/en/support/solutions/articles/43000754091-how-to-play-encrypted-channels-on-adth-nextgen-tv-usb-) |
| ADTH NextGen TV Box | Supports encrypted channels, offline from 2.0.0; records to a USB drive from 2.0.4 | [support FAQ](https://support.adth.com/en/support/solutions/articles/43000703336-faq) |
| NextGen TV sets | A3SA (April 2026): nearly 40 models from Hisense, Panasonic, Samsung, Sony and TCL can decrypt protected content | [A3SA](https://a3sa.com/news-updates/) |

No maker here endorses this software. No logos and no affiliate links.

### Licensed devices as sources

Some boxes handle the DRM themselves. If one offers its output to home media servers through an interface
it documents, it works here as an ordinary source, with no special code:

| The device offers | Add it as |
|---|---|
| A network tuner protocol | **Network tuner** |
| An HTTP or HLS stream | **IPTV single stream**, or a playlist |
| A folder of exported files | A library folder |

What arrives goes through the same checks. If it is still encrypted, it shows the encrypted label and is not
played. Any recording or copy restriction a device reports is honoured. As far as we could find, none of the
interfaces above reports one today, so a channel can also be marked as not recordable by hand, from what
the device's documentation says.

### ATSC 3.0

Unencrypted ATSC 3.0 channels tune, play and record like any others, where the tuner offers them. Checked
with the bundled FFmpeg (build N-128142, 2026-10-28):

| Part | State |
|---|---|
| **Tuners** | Network tuners that present ATSC 3.0 as MPEG-TS over HTTP, as the FLEX 4K's maker documents. **Not tried on real hardware.** Linux DVB has no ATSC 3.0 delivery system, so DVB cards can't. SAT>IP and capture cards don't apply |
| **Video** | HEVC decodes, and is converted to H.264 for players. This needs real CPU: a 4K or 1080p HEVC channel may be too much for a small machine |
| **Audio** | **The bundled FFmpeg has no AC-4 decoder**, and most US ATSC 3.0 stations use AC-4. Such a channel plays **without sound**, says so, and offers the same station on ATSC 1.0 if there is one. MPEG-H audio has no decoder either |
| **Recording** | The raw stream is saved as it arrives, so the AC-4 sound is kept, but it plays silently here until FFmpeg can decode it |

A channel is treated as ATSC 3.0 when its source names HEVC or AC-4. The server asks FFmpeg what it can
decode, so a build with an AC-4 decoder gets sound with no change here. The fallback when a source doesn't
name the codec reads FFmpeg's error, and has not been tried with a real AC-4 stream.

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

## The look

Everything about how the guide looks is the viewer's own choice, per device (**Look** on the guide).
The owner sets what people start with (*Settings → Channels & tuners*).

- **Layout: Classic box.** Bulky, bevelled and square, in the style of a cable or satellite box of the
  early 2000s, with original artwork and colours (no real provider's name, logo or layout): raised
  cells, half-hour columns, an information panel on top with a clock, and a help strip with the
  coloured A, B and C buttons of a remote.
- **Layout: Streaming.** Clean rows and soft corners, with how long is left on each programme.
- **Colours:** Cable blue, Satellite black and amber, or the viewer's theme. Both fixed palettes are
  checked against WCAG AA.
- **The line at the current time** runs down all channels, and each programme on now fills up to it,
  so it is plain how far in each channel is. Both can be turned off, in either layout.
- **The preview:** a live picture, top right by default, in both layouts. It can show the highlighted
  channel, the last channel watched, or be off; go in any corner or anywhere it is dragged (or nudged
  with the arrow keys on its handle); and come in three sizes.
- Row size, hours across the screen, episode names and logos can be changed too.

The preview waits for the highlight to settle, then shares the channel's ordinary stream, so a channel
someone else is watching costs nothing extra. Each player names itself to the server and says when it
leaves, so a channel only flicked past stops about two seconds later and frees FFmpeg and its tuner.
On a tuner, a preview does use up a tuner, which is why it can be pointed at the last channel or off.

Tuning shows a banner with the channel number, name, times, a progress bar and what is next. Number
keys dial a channel, CH+ and CH− use the arrows or Page keys, and **Last** goes back.

The grid is a real ARIA grid. Arrow keys move between programmes with a roving tab stop, every cell
is read as "Channel 101, Comedy, The Show, 8:00 to 8:30 PM, on now", and it works with TV remotes,
game controllers and on phones (a narrower channel column and sideways scrolling).

The player reuses the on-screen remote and the shared controls, and adds channel keys.

## For the TV apps

Everything here lives on the server behind JSON, so the Apple TV and Android TV apps can draw their own guide:
schedules, guide data, tuning, recording and the encryption wording. See the shared contract in
[CLIENT-API.md](CLIENT-API.md) once it is merged.

- **Playing:** `GET /api/tv/{key}/tune?viewer=<id>` gives `url`, standard live HLS. Add the same `viewer`, and `POST
  /api/tv/{key}/leave?viewer=<id>` on leaving so the channel stops promptly. A token in the playlist's address
  carries on to its segments, for players that can't set headers.
- **Codecs:** H.264 and stereo AAC in MPEG-TS, which every native player handles. Viewers share one stream, so the
  codec isn't chosen per client: it's the one they can all play.
- **Guide:** `GET /api/guide`; pass `list=1` for numbers only. A TV app needs a grid that moves with a D-pad and
  remote focus, as the web guide's does: arrows, Page keys, 0–9 to dial, Enter to watch.
- **Encrypted channels:** show `security.label`, `security.detail` and `alternative` from the API, and the page from
  `GET /api/tv/encrypted-help`, rather than wording of your own.

## API

| Method and path | Who | What |
|---|---|---|
| `GET /api/guide?from&hours&offset&limit&kind&group` | signed in | Channels and programmes |
| `GET /api/tv/{key}/tune` | signed in | What is on, and the stream |
| `GET /api/tv/{key}/live/playlist.m3u8`, `/seg/{name}` | signed in | The shared stream |
| `POST /api/tv/{key}/leave?viewer=` | signed in | A player has gone |
| `GET /api/tv/{key}/logo` | signed in, may watch the channel | A tuner channel's logo, through the server. Internet addresses only |
| `GET /api/tv/encrypted-help` | signed in | The help page behind the encrypted label |
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

## Using it

1. *Settings → Channels & tuners*. Under **Add with one click**, press **Add** on anything suggested,
   such as a network your library has shows from. It is on the air straight away.
2. **TV guide** in the menu. Arrow keys or the remote move around, Enter or OK watches, and number keys
   dial a channel. **Look** changes how the guide looks on this device.
3. For more control: **New channel**, or **Edit** on a channel. Start with Simple. Rules and Full
   timetable are there if you want them.
4. For your own adverts: *Settings → Libraries → Add library → Commercials*, then tick the library under
   **Commercials** when editing a channel.
5. For live TV: **Search for tuners**, or **Add** one under *Tuners and IPTV*. For a guide, add an XMLTV
   address under *Guide data*, if the playlist did not bring its own.

To let the custom command driver run, start the server with `WMS_TUNER_COMMANDS=1`.

## Status

**Tested end to end on a real server with real FFmpeg:** one-click network channels, rule channels, hand
timetables, tuning in mid-show, the guide and its looks, the preview, commercials, DVDs on a channel,
IPTV playlists and streams against a local server, a stand-in network tuner, refusal of encrypted
channels, XMLTV guide data, recording from a stand-in tuner, and profile limits.

**Not tested on real hardware, because none was available:** every physical tuner. See the driver table.

**Not done:**

- Windows BDA digital tuning; DiSEqC dish switches on Linux DVB.
- ATSC 3.0 sound: AC-4 needs a decoder FFmpeg doesn't have.
- ATSC huffman-compressed guide text (those events are skipped).
- Commercial breaks inside a programme (they play between programmes).
- A viewer and a recording sharing one tuner on the same channel.
- Recording disk-space checks and automatic clean-up of old recordings.
- Guide data read over the air reaches only the channel being watched, not the others on its multiplex.
