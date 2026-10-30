# Client API

Everything an app needs to work with WittyMediaServer: signing in, browsing, playing, discs and
themes. It is written for the Apple TV and Android TV apps (WittyMediaClient, in their own
repository), and works for any other client or tool.

The machine-readable version is [openapi.json](openapi.json) (OpenAPI 3.1). A test in
`internal/server` fails when a route is missing from either file, when what they say about sign-in
isn't what the server does, or when a schema no longer matches the Go type behind it.

- [Start here](#start-here)
- [Conventions](#conventions)
- [Finding a server](#finding-a-server)
- [Connecting by address](#connecting-by-address)
- [Server identity](#server-identity)
- [Signing in](#signing-in)
- [Stream tokens](#stream-tokens)
- [Playing something](#playing-something)
- [What each TV plays](#what-each-tv-plays)
- [Subtitles](#subtitles)
- [Rules every client shares](#rules-every-client-shares)
- [Discs and their menus](#discs-and-their-menus)
- [Themes and design data](#themes-and-design-data)
- [Choices that follow the account](#choices-that-follow-the-account)
- [What a TV app must build itself](#what-a-tv-app-must-build-itself)
- [Adding to the API](#adding-to-the-api)
- [Every route](#every-route)

## Start here

1. [Find servers on the network](#finding-a-server), and always offer
   [typing an address](#connecting-by-address) beside them. `GET /api/v1/capabilities` confirms it is
   a WittyMediaServer and says what it can do. `setupRequired: true` means nobody has set it up: say
   so, and point them to a browser.
2. [Pin the server's identity](#server-identity), and [sign in](#signing-in) with a code. Keep the
   token in the platform's secure storage (Keychain, Android Keystore).
3. `GET /api/v1/home` gives the first screen; `/items`, `/shows` and `/music/...` the libraries.
4. To play anything, `POST /api/v1/items/{id}/playback` with the [profile](#what-each-tv-plays)
   of this device, and hand the `url` it answers with to AVPlayer or ExoPlayer.
5. Save the position with `POST /api/v1/items/{id}/progress` every 10 seconds and when stopping.

## Conventions

**Versions.** `/api/v1/...` is the contract. Within v1 changes only add things: new routes, new
fields, new values in lists. Ignore what you don't recognise; never assume a list of values is
complete. Removing or changing anything means a new version, with v1 kept alongside. `apiRevision`
in `/capabilities` counts additions, and `features` names them: look for a feature rather than
comparing server version numbers. `/api/...` without a version is the same thing, for the web
interface that ships with each server. Every API answer carries `X-WMS-API-Version`. A version this
server doesn't have answers 404 with `apiVersions`.

**Addresses in answers** (`url`, `overlayUrl`, `openUrl`...) start with `/api/`: put the server's
address in front and don't change them otherwise. They are not part of what a version promises to
keep the same; use them as given, and don't build your own from pieces.

**Sign-in.** Send `Authorization: Bearer <token>`, on every API call: never put a session token in an
address. Tokens last 90 days and end when the user signs out or changes their password, so a 401 at any
time means: sign in again. Each route's sign-in is in
[the table below](#every-route): none, user or admin (403 for everyone else).

**Errors** are JSON, `{"error": "..."}`, in plain words that can be shown as they are. Status codes
mean what they usually do: 400 bad input, 401 not signed in, 403 not allowed, 404 not found **or not
for this account** (titles an account may not see don't exist as far as it can tell), 409 conflict,
429 slow down (`Retry-After` says for how long), 503 busy (for example, every transcode slot is in use: a
stream's segments may answer this, so wait `Retry-After` and try again), other 5xx a problem on the server.

**Numbers.** Times are seconds as floating point unless a name says otherwise; `...At` values are Unix
seconds. IDs are integers, except session and stream IDs, which are strings to pass back unchanged.

**Lists** that can be long take `limit` and `offset` and answer with `total` (`/items`), so don't fetch
a whole library for a screen. Filtering and sorting always happen on the server.

**Files on the server** are not for apps. An item's `path`, `poster` and `backdrop` fields are empty or
say little for anyone but an administrator: use the routes for pictures, and `/playback` for media.

**Artwork** is a plain image route. Add `v=<updatedAt>` to the address so it can be cached until the
item changes. Items and shows without a picture answer 404: draw a placeholder.

**CORS and cookies.** Apps use the bearer token and never need cookies. Cross-origin requests with a
token are allowed; cookies are not honoured across origins.

## Finding a server

The server announces itself on the home network, so an app can list it without anyone typing an
address. Being found opens nothing: connecting still takes a code or a password. An administrator can
turn it off (*Settings → Profiles → Signing in → Let apps find this server*; the setting is
`lanDiscovery`). It never runs when the server only listens on `127.0.0.1`, since nobody else could
connect, and it only uses private addresses.

**What is announced, and nothing more:** the server's name (the one its owner gave it, never the
computer's), a random ID, its version, and how to connect. No accounts, no paths, no hostname.

**mDNS/DNS-SD is the way to find it** (`_wittymedia._tcp`, in the `local` domain, IPv4):

| Record | Holds |
|---|---|
| PTR | The instance: `<name> (<first 4 of id>)._wittymedia._tcp.local`. Show `name` from TXT, not this. |
| SRV | The port of the server's web address, and a host name `wms-<first 8 of id>.local` |
| A | Its private IPv4 address on that network |
| TXT | `id` (32 hex, stable), `name`, `v` (server version), `api` (API version), `tls` (`1` for `https://`, otherwise `http://`) |

| Platform | How to browse |
|---|---|
| tvOS, iOS | `NWBrowser` for `_wittymedia._tcp`. List it under `NSBonjourServices`, and give an `NSLocalNetworkUsageDescription`; the system asks the person once. No special entitlement is needed. |
| Android, Android TV | `NsdManager`, type `_wittymedia._tcp` |
| Desktop | Any mDNS library, or a socket on `224.0.0.251:5353` |
| Web apps (webOS, Tizen, browsers) | Not possible: offer the address box only, and say so |

**An extra for clients with raw sockets**, where multicast is blocked but broadcast isn't: send the ASCII
bytes `WMS-DISCOVER/1` by UDP to `255.255.255.255` port 9797. Each server answers directly with:

```json
{"service": "wittymediaserver", "id": "…", "name": "…", "version": "…", "api": 1, "port": 9797, "tls": false}
```

Apple platforms need an entitlement Apple must grant for this, so don't rely on it there. Answers go to
private addresses only. Ignore answers whose `service` differs. Wait a second or two and listen for
several: there may be more than one server.

**If nothing is found:** a firewall may be blocking UDP ports 5353 and 9797 on the server (the install
script for Windows opens them), a network may keep devices apart ("guest" or "client isolation" Wi-Fi), or
the server may be in a container without host networking. So never make discovery the only way in.

**Finding it again.** Addresses change, so remember `id`. If a saved address stops answering, browse
for the same `id`, then [check its identity](#server-identity) before sending a token.

## Connecting by address

For someone typing an address or a URL. This is the whole check, in order:

1. **Tidy it up.** Trim it. With no scheme, add `http://`. With neither a scheme nor a port, add port
   9797. Remove any path, query and trailing slash.
2. **Ask.** `GET /api/v1/capabilities`, with a timeout of about 5 seconds, sending no token.
3. **Is it a WittyMediaServer?**

   | Answer | Means | Say |
   |---|---|---|
   | 200, JSON, `server` is `WittyMediaServer`, `apiVersions` has 1 | Yes | Carry on |
   | 200, JSON, but `apiVersions` lacks 1 | A far newer or older server | "This server needs a newer version of the app" |
   | 404, and `GET /api/status` has `name` of `WittyMediaServer` | A server older than this API | "Update this server" |
   | Anything else with an answer (a web page, other JSON) | Something else is at that address | "That isn't a WittyMediaServer" |
   | Connection refused, or a timeout | Nothing there, or it's not running | "Can't reach it. Check the address, and that the server is running." |
   | A certificate error | Wrong or self-signed certificate | Say so. Never accept it silently. |

4. **Redirects.** Don't follow one automatically to another host. `http://` to `https://` on the same host
   is fine: keep the new address. Never send a token across a redirect to a different host.
5. **Plain HTTP?** On a private address (`10.x`, `172.16-31.x`, `192.168.x`, `169.254.x`, `fd00::/8`,
   loopback, a `.local` name) it's how home servers normally run: allow it. On anything else, a password
   or token would cross the internet readable by anyone on the way: refuse, or at the very least warn
   clearly, and suggest HTTPS. `100.64.0.0/10` and VPN names are encrypted by the VPN itself, so count as
   private.
6. **Identity.** [Pin or check it](#server-identity), then [sign in](#signing-in). If `setupRequired` is
   true, send the person to a browser first.

Save the address, `serverId`, `publicKey` and the token, all together.

## Server identity

Anyone on the network can announce a server's name and ID, and answer at its old address. A key can't
be copied that way. Each server has an Ed25519 key, made the first time it runs and moved with its database,
and proves it holds it:

```
POST /api/v1/identity  {"nonce": "<base64 of 16 to 64 random bytes>"}
→ {"serverId": "…", "publicKey": "<base64>", "fingerprint": "AB12-CD34-EF56", "algorithm": "ed25519", "signature": "<base64>"}
```

The signature is over these bytes, the nonce being the string exactly as sent:

```
WittyMediaServer identity v1\n<serverId>\n<nonce>
```

- **First connection:** check the signature, then keep `publicKey` (the pin). Show the `fingerprint`, as `/pair`
  shows it too, so careful people can compare.
- **Every connection after that, and before sending the token to any address not just used:** make a new
  nonce and check that the signature is good **and** `publicKey` is the pinned one. `serverId` alone proves
  nothing, because it's announced openly.
- **If it doesn't match:** stop, don't send the token, and tell the person that a different machine is answering
  at that address, which may be a look-alike or a reinstalled server. Only they can decide; offer "This is
  my server, trust it" and then sign in again.

**What this doesn't do.** It notices a different machine once a key is pinned. On plain HTTP it can't stop someone
relaying traffic on the very first connection or reading what is sent: comparing the fingerprint helps with
the first, and HTTPS is the real answer to both.

## Signing in

### With a code

A device on the network can't attach by itself: someone already signed in on a phone or computer has to agree,
at `/pair` in the web interface (*their name → Sign in a TV*). Only adult accounts can. There are two ways
round, so use whichever end is easier to type on. They end the same: the device has a token and `user`.

**The code is typed on the device.** Good when the device has a number pad, or a remote can act as one.

- The person presses *Get a number* at `/pair` (`POST /pair/invite {userId}`), and sees 6 digits.
- The device sends `POST /pair/redeem {code, device, platform}` and gets
  `{status: "approved", token, user}` at once.
- Digits only; spaces and dashes are ignored. A code lasts 5 minutes, works once, and a person has one open
  at a time. The sign-in is for whoever pressed the button, or a Kids or Guest profile they chose.
- Each failure has `error`, in plain words to show, and `reason` if you want your own:

  | Status | `reason` | Meaning |
  |---|---|---|
  | 400 | `wrong-code` | Not a code that exists. |
  | 410 | `expired` | Its five minutes are up. |
  | 410 | `used` | It has already been used. |
  | 429 | `too-many-tries` | Too many wrong codes. `retryAfterSec` and `Retry-After` say how long to wait. |

- Six digits are few, so guessing is limited: 5 wrong from one address means a wait, which grows; 30 wrong
  anywhere on the server within 5 minutes stops all attempts for a while and cancels every open code.

**The code is shown on the device.** Good for a TV with only a remote: nothing to type there. The TV shows a
short code and someone approves it on a phone or computer. It follows the same pattern as the OAuth device flow.

```
TV                              server                         phone or computer
│ POST /pair/start ───────────► │
│ ◄── code, deviceSecret,       │
│     intervalSec, expiresInSec │
│ shows ABCD-EFGH (and a QR     │
│ code of approveUrl)           │ ◄── /pair page: types the code
│                               │ ◄── POST /pair/info   (shows which device is asking)
│ POST /pair/poll ────────────► │
│ ◄── status: pending           │ ◄── POST /pair/approve
│ POST /pair/poll ────────────► │
│ ◄── status: approved, token   │
```

- `POST /pair/start {device, platform}`: `device` is a name people will recognise ("Living room Apple
  TV"), `platform` is `tvOS` or `Android TV`. It answers `code` to show, `deviceSecret` to keep (never
  show it), `expiresInSec` (600), `intervalSec` (how often to poll, 3), and `approveUrl` (a path: put
  the server's address in front for a QR code; the page still asks before doing anything).
- `POST /pair/poll {deviceSecret}`, every `intervalSec`:

  | Status | `status` | Meaning |
  |---|---|---|
  | 200 | `pending` | Keep waiting. |
  | 200 | `approved` | `token` and `user` are here. This answer comes once. |
  | 403 | `denied` | Someone refused. Show that, and offer a new code. |
  | 410 | `expired` | Ten minutes passed, or it was already collected. Ask for a new code. |
  | 429 | `slow-down` | Polling too fast. Wait `Retry-After`; the code is still good. |

- Codes last 10 minutes and work once. They are 8 letters from 20 consonants (no vowels or look-alike
  letters), and are accepted in lower case, with or without the dash or spaces.
- Limits: 4 codes open at once and 12 started in 10 minutes per address, 256 open on the server;
  wrong codes count against the signed-in account like wrong passwords.
- Only adult accounts can approve (a password-free Kids profile must not be able to hand out lasting
  sign-ins). They may sign the device in as themselves or as a Kids or Guest profile they manage, by
  `userId` from the `profiles` in `/pair/info`.
- Nothing is stored: a restart of the server simply means asking for a new code. The new session
  appears as `device`.

### Name and password

`POST /login {name, password, device}` answers `{token, user}`, 401 for wrong details, 429 after
repeated failures. Keep this as the fallback for devices without a phone nearby. On the home network
`GET /profiles` may list a "Who's watching?" screen (`style: "profiles"`); a profile with
`hasPassword: false` signs in with an empty password, only where it is listed. Otherwise
(`style: "classic"`) ask for both. To switch profiles, sign in again.

## Stream tokens

AVPlayer can't reliably add headers to the requests it makes, and a session token in an address is
far too powerful to put there. A stream token is the short-lived replacement.

- You get one in every `/playback` answer (`streamToken`), or from
  `POST /items/{id}/stream-token`. Addresses from `/playback` already carry it.
- Add it as `?st=<token>` (or `&st=`) to any address it should open. It belongs on: the file, audio
  stream, HLS playlists and segments (the server passes it on inside the playlists it writes), WebVTT
  subtitles, artwork of that item, and the media of a disc session for that item (stills, overlays,
  highlights, sub-pictures, thumbnails). Routes it opens are marked `st` in [the tables](#every-route).
- It is **read-only and for one item**. It opens nothing else: not another item, not the library, not
  an account, and it can't be used to get another token.
- It expires 30 minutes after it was last used, and after 12 hours at most. It ends at once when the
  sign-in it came from ends, and the account's limits are checked on every use.
- A 401 on a media address means: ask for a new one (and open the stream again).
- If your player can set headers (ExoPlayer can, with a `DataSource.Factory`), a bearer token works on
  all these routes too, but a stream token is still the safer choice to give a player: playlists get
  logged and shared in ways API calls don't.

Logs hide these tokens, like session tokens.

This is the one way for apps. The web interface has its own older way of putting a credential in media
addresses (`?token=`), which is limited to media routes and may change with it: don't use it.

## Playing something

`POST /api/v1/items/{id}/playback` is the one door. It takes what this device can play, and answers
what to do.

```json
{
  "profile": "appletv",
  "capabilities": { "audioCodecs": ["aac", "ac3", "eac3"], "maxHeight": 1080 },
  "audio": 1,
  "subtitle": -1,
  "startSec": 0,
  "maxBitrateKbps": 20000
}
```

Only `profile` is needed. Leave out `audio` and `subtitle` and the server picks them
(`audioLanguage` and `subtitleLanguage` say what the viewer prefers; see
[the rules](#rules-every-client-shares)). Leave out `startSec` and it resumes where the viewer stopped.
`forceTranscode` asks for a conversion, `burnSubtitle` draws the chosen subtitle into the picture
(for heavily styled ASS).

```json
{
  "itemId": 42,
  "decision": "remux",
  "reasons": ["container"],
  "protocol": "hls",
  "url": "/api/hls/v-4395...-n1b1.../master.m3u8?st=st_8cd1...",
  "streamToken": "st_8cd1...",
  "tokenExpiresInSec": 1800,
  "streamId": "v-4395...-n1b1...",
  "durationSec": 9141.4,
  "seek": "restart",
  "startAtSec": 0,
  "timeOffsetSec": 600,
  "resumeSec": 600,
  "audio": [{"index": 0, "language": "eng", "languageName": "English", "codec": "ac3", "channels": 6}],
  "subtitles": [{"index": 0, "language": "eng", "codec": "subrip", "text": true, "delivery": "hls", "url": "..."}],
  "audioIndex": 0,
  "subtitleIndex": -1,
  "subtitleDelivery": "none",
  "intro": {"startSec": 30, "endSec": 95},
  "nextId": 43,
  "progressUrl": "/api/items/42/progress"
}
```

**The decision** (`decision`, why in `reasons`):

| `decision` | `protocol` | What it is | Cost on the server |
|---|---|---|---|
| `direct` | `file` | The file itself, with byte ranges. | Almost none |
| `remux` | `hls` | HLS with the picture copied unchanged; the sound is copied or converted to AAC. | Little: no video encoding |
| `transcode` | `hls` | HLS converted to H.264 and AAC, with subtitles burned in if needed. | A lot |
| `disc` | `disc-session` | A disc: see [Discs](#discs-and-their-menus). `disc` says how it starts. | Depends on the title |
| `audio-direct` | `file` | A sound file as it is. | Almost none |
| `audio-transcode` | `adts` | Sound converted to AAC while streaming. | Little |

The server always tries them in this order. `reasons` are stable names, worth showing in a
"playback info" screen: `container`, `video-codec`, `video-tag`, `video-height`, `video-bitrate`,
`video-depth`, `video-range`, `interlaced`, `audio-codec`, `audio-channels`, `audio-track`, `subtitle`,
`burn-subtitle`, `forced`, `direct-play-off`, `remux-unavailable`, `hls-video-codec`, `no-video`.

**Time.** `seek` says how seeking works:

- `native`: seek in the player, anywhere. The playlist of a `transcode` covers the whole film from the
  start, and the server converts whichever part is asked for.
- `restart`: the stream is a growing `remux`. A copied picture can only be cut at the file's own
  keyframes, so the server can't know segment lengths until it has read them; the playlist is an HLS
  `EVENT` playlist that grows as it goes (at about four times real time). Seek freely within what the
  player reports as seekable. To go further, stop the stream and call `/playback` again with `startSec`.

Before starting, seek the player to `startAtSec`. The film position is always
`player time + timeOffsetSec`: use that for progress reports, the on-screen clock and subtitles you
draw yourself. A remux starts at the keyframe just before `timeOffsetSec`, so it can be a few seconds
early.

**Changing tracks.** Call `/playback` again with the new `audio` or `subtitle` and `startSec` set to the
current position, and if `url` changed, switch to it. It stays the same when the player can do it
alone: a `direct` Android or Apple player switches embedded sound tracks itself
(`AVMediaSelectionGroup`, `TrackSelectionParameters`), and subtitle renditions in an HLS master
playlist are switched in the player.

**Stopping.** `DELETE /hls/{streamId}` when the viewer leaves: it stops the conversion straight away.
Streams also pause after 90 seconds with no requests, and go after 10 minutes. A long pause can
therefore end in a 404: call `/playback` again.

## What each TV plays

These are the defaults of `GET /playback/profiles`. They come from Apple's *HLS Authoring Specification
for Apple Devices* and the Media3 *Supported formats* tables, and **haven't been tried on a real device
yet**: the server side has been checked with FFmpeg as the client. Expect to adjust them as the apps
meet real hardware.

| | `appletv` (Apple TV 4K) | `appletv-hd` | `androidtv` | `web` |
|---|---|---|---|---|
| Direct containers | MP4, M4V, MOV | MP4, M4V, MOV | MP4, M4V, MOV, MKV, WebM, TS | MP4, M4V, MOV, WebM |
| Direct video | H.264, HEVC (`hvc1` only) | H.264 | H.264, HEVC, VP9 | H.264, VP9, AV1 |
| Direct sound | AAC, AC-3, E-AC-3, ALAC, MP3 | AAC, AC-3, E-AC-3, ALAC, MP3 | AAC, MP3, FLAC, Opus, Vorbis | AAC, MP3, Opus, Vorbis |
| Most lines | 2160 | 1080 | 2160 | any |
| HDR | HDR10, HLG, Dolby Vision 5 | none | none (add with capabilities) | not checked |
| HLS, copied video | H.264 (TS), HEVC (fMP4) | H.264 (TS) | H.264 (TS), HEVC (fMP4) | never remuxed |
| HLS, copied sound | AAC, AC-3, E-AC-3 | AAC, AC-3, E-AC-3 | AAC, MP3 | |
| Subtitles drawn by the player | `mov_text` in the file; WebVTT renditions | the same | SRT, ASS, WebVTT, `mov_text`, PGS, VobSub, DVB in the file; WebVTT sidecar or rendition | none: burned in |

Why they differ:

- **AVPlayer doesn't open MKV**, and Apple requires HEVC in HLS to be fMP4 and tagged `hvc1` (files
  tagged `hev1` are rewrapped). So an MKV goes to Apple TV as `remux`, nearly free for the server.
- **ExoPlayer reads MKV itself**, so most MKV files play directly. What decodes depends on the device
  (`MediaCodec`), so the default lists only what every device has.

**Send what the device really does.** Each profile is a safe floor. An app that asks its hardware gets
fewer conversions: pass `capabilities` with what it found, and those lists replace the profile's.

- Android: `MediaCodecList` for codecs, `Display.getHdrCapabilities()` for HDR,
  `AudioManager`/`AudioCapabilities` for AC-3, E-AC-3 and DTS passthrough. FFmpeg-based extensions
  add more sound codecs, if the app includes them.
- tvOS: `AVPlayer.availableHDRModes` for HDR, `VTIsHardwareDecodeSupported` for HEVC and AV1,
  `AVAudioSession.maximumOutputNumberOfChannels` for channels.

Names are FFmpeg's: `h264`, `hevc`, `vp9`, `av1`, `mpeg2video`, `vc1`; `aac`, `ac3`, `eac3`, `dts`,
`truehd`, `flac`, `opus`, `vorbis`, `mp3`, `alac`; `subrip`, `ass`, `webvtt`, `mov_text`,
`hdmv_pgs_subtitle`, `dvd_subtitle`, `dvb_subtitle`. HDR kinds: `hdr10`, `hlg`, `dv5`. Dolby Vision
with a base layer others can show (profiles 7 and 8) counts as that base layer; profile 5 has none and
needs `dv5`, or it is converted. Anything HDR the display can't show is converted to SDR with tone
mapping, where the server's FFmpeg has the filters.

## Subtitles

Each track in `subtitles` says how it would reach the screen if chosen now (`delivery`), and text
tracks have a `url` to their WebVTT.

| `delivery` | What the app does | Used for |
|---|---|---|
| `embedded` | Nothing special: pick the track in the player. | PGS or SRT in an MKV on Android; `mov_text` in an MP4 |
| `hls` | Pick the rendition in the player (`AVMediaSelectionGroup`, `TrackSelectionParameters`). Its name and language are in the master playlist. | Text tracks on Apple TV |
| `sidecar` | Load `url`: `MediaItem.SubtitleConfiguration` on Android (MIME type `text/vtt`), or draw it yourself. | Text tracks the player can't read from the file |
| `burn` | Nothing: they are in the picture. Costs a full conversion. | Picture subtitles (PGS, VobSub) on Apple TV |

If the chosen track needs a different stream, the answer already is that stream: on Apple TV a text
subtitle on an MP4 that was playing directly turns into a `remux` with a master playlist, since AVPlayer
can't add a separate file to a progressive download. Offer text tracks from `subtitles` without
worrying about this, and call `/playback` again when one is chosen.

Details for HLS: each rendition's playlist holds the whole track as one WebVTT segment, with the
`X-TIMESTAMP-MAP` line HLS requires. The first request for a track reads through the file, which can
take a minute or more on a large film and a slow disk; it is then cached, and the chosen track starts
converting as soon as `/playback` is called. Styled ASS keeps only basic styling in WebVTT; offer
`burnSubtitle: true` for those.

## Rules every client shares

So that a TV and a phone never disagree, the server applies these and answers with the result. The
numbers are also in `/capabilities`, under `playback.rules`.

| Rule | How it works | Where it comes from |
|---|---|---|
| Resume | Saved position past 60 s and not finished (books: any position) | `progress.resumeSec`, `resumeSec` in `/playback`; 0 means start over |
| Watched | Past 92% of the duration | Saved by `/progress`; `progress.watched` |
| Continue watching and Next up | The rows of `/home` | `hubs`, by `key`: `continue`, `continue-reading`, `continue-listening`, `nextup`, `recent-<library>` |
| Which episode Play means | The one in progress, else the one after the last watched | `nextUpId` in `/shows/{id}` |
| Next episode | The following one in the show | `nextId` in `/items/{id}` and `/playback` |
| After the last one, or a film | A similar title or show | `/items/{id}/recommend` |
| Skip intro | From a chapter in the file, or the show's own setting | `intro` in `/playback`, `introStart`/`introEnd` on items |
| How a disc starts | Its own setting, else the server's; a disc without menus plays the feature | `startMode`, `insertEffect` on `/items/{id}`, `disc` in `/playback` |
| Which sound track | Wanted language, else marked default, else the first; never commentary by itself | `audioIndex` |
| Which subtitle | Wanted language; else forced subtitles in the language being heard; else off | `subtitleIndex` |
| Who sees what | Per-account limits; hidden titles answer 404 | Applied to every list and route |

A few are for the app to carry out, with these values, so it feels the same everywhere: show **Watch
next** 30 seconds before the end; count down **8 seconds** into the next episode; report progress every
**10 seconds** and on stopping, but not in the first **5 seconds**.

## Discs and their menus

Discs don't need a browser: the disc engines run on the server, and a client only draws what it is told.
The player is a small loop. Open a session, draw the `Presentation` it gets, and send what happens back.

```
POST /items/{id}/dvd {"mode": "menu"}        → {sessionId, presentation}
POST /dvd/{sessionId}/action {"type": "press", "button": 3, "positionSec": 0}
                                             → {presentation}  (and so on)
DELETE /dvd/{sessionId}                      when finished
```

The same routes serve DVDs and Blu-rays. Open with `mode` from `disc.startMode`, or `mode: "title"` with
`ttn` and `chapter` to skip the menus (the disc page, `/items/{id}/disc`, lists them), or `menu` and
`menuVts` to open one menu. Send remembered choices along as `audioLanguage`, `subtitleLanguage` and
`angle`; they are the app's to remember, per disc.

A `Presentation` has a `kind` (`menu`, `title` or `stop`) and a `serial`. If `serial` and `media.url`
are both unchanged, only the highlight moved: don't touch the player.

**The picture (`media`).** `type` is `hls` or `still`. Add the item's [stream token](#stream-tokens) to
every media address. These streams are always converted (H.264, AAC), so they play on anything.

- `hls`: play it. `loop: true`: repeat it (`AVPlayerLooper`, `REPEAT_MODE_ONE`).
- `still`: show the image.
- `stillSec`: how long to hold after a clip ends or a still appears, then send `{"type": "ended"}`. -1
  means hold until the viewer acts; show a Continue button if there are no menu buttons to press.
- `timerSec` on the presentation: send `{"type": "tick"}` that many seconds later (the disc's own timer).
- Players don't always report the end of a short clip: playback that stops moving within 2.5 seconds of
  the end counts as ended.

**Menu buttons (`menu`).** Coordinates are in `width` × `height`; scale them to the rectangle the video
is drawn in (`AVPlayerLayer.videoRect`; on Android, from the aspect ratio in `aspect`, which is the
picture's real shape). Draw in this order, in views on top of the video:

1. `overlayUrl`, stretched over the video, if there is one.
2. The selected button's highlight: `highlightUrl` with `{n}` replaced by the button number, if the
   button is `lit`. Load them all at once and show or hide them, so moving never waits for an image.
   `frames` and `fps` mean it's animated: add `?f=N` (`&f=N` with a token).
3. If `hasHighlights` is false or the button isn't `lit`, draw a focus frame yourself at its rectangle.

Remote input:

| Remote | Action |
|---|---|
| D-pad | Move to the button in `up`, `down`, `left`, `right` of the selected one (0 or itself: nowhere; go to the app's own controls instead). If the target has `auto`, press it at once. Otherwise show it straight away and send `{"type": "select", "button": n}` without waiting for the answer. |
| OK | `{"type": "press", "button": n, "positionSec": t}` with the current position when in a title |
| Menu | `{"type": "menu", "menu": "root", "positionSec": t}`; `"title"` for the top menu |
| Back | `{"type": "key", "key": "return", "positionSec": t}` |
| Play | `{"type": "resume"}`, back to the film from a menu |
| Numbers, if any | `press` on that button number |
| Next and previous in a menu | `{"type": "chapter", "chapter": 1}` or `-1` |

Timing:

- Buttons only work between `buttonsFromSec` and `buttonsUntilSec` of the clip (0 means no limit). A
  looping clip brings them back when it starts over.
- `forcedAction` and `actionAtSec`: at that moment of the clip, press that button by yourself.
- `inTitle`: this is an interactive part of the feature (a game, a quiz, a branching story), not a menu.
- `popup` (Blu-ray pop-up menus during the film): don't show the buttons until the Menu key opens them;
  hide them when an answer has `closed: true`. Send `positionSec` with every press, because buttons on
  these menus read the current time.

**The film (`title`).** Seek to `startSec` on starting. Chapter jumps are seeks in the player, using
`chapters`; past the last chapter, send `ended`, since a disc moves on to whatever it plays next. Report
progress with `discTitle` set to `ttn`, and pick up from `progress.resumeSec` when `discTitle` matches.

- Sound: `{"type": "audio", "stream": n, "positionSec": t}`; angle:
  `{"type": "angle", "stream": n, "positionSec": t}`. Both answer with a new stream that restarts at
  `positionSec`.
- Subtitles are pictures drawn by the client, so they switch instantly and work on any player.
  `{"type": "subtitle", "stream": n, "on": true}` chooses; `subtitleStream` is the track on (-1 off), and
  `forcedStream` is the track whose `forced` events still show with subtitles off. Fetch
  `subtitlesUrl` + `&stream=N&from=F&to=T` in windows of 120 seconds around the playhead, and draw each
  event's `url` at `x, y, w, h` (in `frameW` × `frameH`) between `start` and `end`.
- 3D Blu-rays (`stereo3d` on opening): the web player shows side-by-side and over-under as one eye or
  red/cyan. A TV app can leave that out, or hand the picture to the display's own 3D mode.

**Keeping it seamless.** Switching clips shouldn't flash black. Keep the last frame on screen until the
next clip has drawn its first one: hold a snapshot (`AVPlayerItemVideoOutput`, or `TextureView.getBitmap`
on Android), and drop it on `isReadyForDisplay` or `onRenderedFirstFrame`. The current clip's `url` is the
same if a new presentation only moved within the same stream: then seek instead of reloading.

**Stops.** `kind: "stop"` has a `reason`; `scrambled` means the image is copy-protected and can't play.
Offer Title menu, Play movie and Back. This server never decrypts discs.

**What's left out.** BD-J (Java) Blu-ray menus aren't run by the server: such a disc plays its feature
directly, on TV apps as in the browser.

## Themes and design data

`GET /themes` lists the themes, with four preview colours and `scheme` (`dark` or `light`), and which
is the server's `default`. `GET /themes/{id}/tokens.json` is a theme as data, read from its CSS
variables, so theme authors write one file and the apps get the same colours:

```json
{
  "id": "default", "name": "WittyMediaServer", "scheme": "dark",
  "colors": { "bg": "#0F1115", "bg-2": "#171A21", "fg": "#E8EAF0", "accent": "#7955FF", "overlay": "#00000099" },
  "lengths": { "radius": 10, "sidebar": 240 },
  "fonts": ["Inter", "system-ui", "-apple-system", "Segoe UI", "Roboto", "Helvetica", "Arial", "sans-serif"],
  "other": { "shadow": "0 10px 30px rgba(0, 0, 0, 0.45)" },
  "webOnly": false
}
```

- Colours are `#RRGGBB` or `#RRGGBBAA`, by variable name (`--bg-2` is `bg-2`). [THEMES.md](THEMES.md) says
  what each one is for. Lengths are pixels. Every built-in theme has `bg`, `bg-2`, `bg-3`, `bg-4`,
  `border`, `fg`, `fg-2`, `fg-3`, `accent`, `accent-2`, `accent-fg`, `accent-text`, `danger`, `ok`,
  `overlay`, `radius` and `fonts` (a test checks), but a theme someone uploaded may lack some or use a
  form of colour that isn't recognised: fall back on the `preview` colours and the app's own defaults.
- `webOnly: true` means the theme also has its own CSS rules or images: it will look plainer in an app.
  CSS itself can't be used in a native app.
- Which theme a person uses on a device is chosen on that device in the web interface too (the browser
  remembers it). Apps should offer the list, default to the server's, and remember the choice themselves.
- How the interface is laid out and what the look is meant to be: `docs/DESIGN.md` (from the interface
  work) is the place for that. This file only covers the data.
- `GET /branding/logo` and `brandName` in `/status` replace the name and icon, where the owner set them.

## Choices that follow the account

`GET` and `PUT /me/ui` hold one JSON object of up to 16 KB for the account, so choices follow the person
to every device. `PUT` replaces all of it, so read first and keep keys you don't know. Known keys:

| Key | Who uses it | Shape |
|---|---|---|
| `home` | The home page | `{v: 2, order: [widget id...], hidden: [widget id...]}`. Widget ids are hub keys, plus `hero`, `libraries`, `jump` (the three continue rows as one) and `shuffle`. |

Add keys of your own under a name that says whose they are (`appletv: {...}`).

## What a TV app must build itself

The server does everything that can be decided without the screen. This is what is left. It is
deliberately a real list: anything here has to be written once for tvOS and once for Android TV.

**Playing**

- The player screen and its controls, using AVPlayer and ExoPlayer/Media3. Seek bar, chapters, picture
  fit, speed, the track pickers.
- Resuming the right way for the `seek` value: seeking for `native`, a new `/playback` call for `restart`.
- Reporting progress, and switching streams without losing the place when a track changes.
- Watch next and the 8-second countdown, Skip intro (the data comes from the server, the moment on screen
  doesn't).
- Handing the system a now-playing title, artwork and remote commands (`MPNowPlayingInfoCenter`,
  `MediaSession`).
- Asking the device what it can play, and passing that as `capabilities`.
- Recovering: 401 on media means a new token, 404 on a stream means a new `/playback`, a stall means nudge
  and retry.
- Drawing WebVTT over the picture in the rare case of `sidecar` on tvOS, if a profile ever needs that.

**Discs**

- The presentation loop above: the overlays and highlights in views over the video, button movement,
  timers (`stillSec`, `timerSec`, `forcedAction`, `buttonsFromSec`, `buttonsUntilSec`), the watchdog for
  clip ends, keeping the last frame between clips.
- Drawing disc subtitles from their events. The windows and the forced-only rule are the client's.
- Remembering per-disc choices (sound and subtitle language, angle) and sending them when opening. The web
  player keeps these in the browser, so they don't carry over; the app could keep them in `/me/ui`.
- The insert animation, if wanted (`insertEffect`, `insertSound` say whether).
- The on-screen remote for discs: map the real remote instead.

**Browsing**

- Every screen: home, libraries, shows, the item page, search, profiles, settings if any.
- Arranging the home rows from `hubs`, and honouring `home` in `/me/ui` (joining the three continue rows
  into "Jump back in" is the web page's choice).
- Focus handling by the platform: tvOS focus engine, Android's D-pad focus, and the controller keys.
  `web/src/tv.ts` shows which keys do what. Keep the same habits: Back goes up, OK on the picture brings
  up the controls, the picture owns the arrows until then.
- Screen reader support (VoiceOver, TalkBack) and the announcements the web player makes.
- Words: episode labels ("S2 · E15"), run times, dates, codec names ("H.264"). Language names come as
  `languageName` where the server knows them; fill the rest from `Locale`.
- Placeholders for items without pictures.
- Settings for this device only: playback speed choice, subtitle size, picture fit, which theme.

**Music, audiobooks, podcasts**

- The audio player and queue. Which track of an album to go on from (the first unfinished one) is decided
  by the client, as is keeping audiobooks and podcasts playing after the screen changes.

**Books**

- Readers. The server hands over pages (`/page/{n}`), EPUB files (`/epub/...`), PDFs (`/file`), annotations
  and search; the web readers (WittyEbook, WittyPDF, WittyComics, WittyInk) are browser code. On a TV it may
  make sense to leave books out, or support comics only. Treat EPUB content as untrusted: no scripts.

**Connecting and signing in**

- Browsing for servers (`NWBrowser`, `NsdManager`), the address box with its checks, and the wording of
  its failures.
- Pinning `publicKey`, the signature check, and what to say when it doesn't match. Ed25519 verification is
  in CryptoKit (`Curve25519.Signing`) and, from Android 13, the platform; older Android needs a small library.
- The code screens: entering digits with a remote, or showing a code and polling, with a QR code of
  `approveUrl`. Saving the token securely.

**Administration**

- Nothing has to be built. Library, account and server settings are fine in the web interface; every route
  for them is here for tools, but a TV app can link to the web address instead.

**Moved to the server for this work**, so the apps don't have to: the resume rule, how a disc starts and its
insert effect, default track choice, language names, and the direct/remux/transcode decision. The web
interface now reads the first two from the server too.

## Adding to the API

For anyone adding a feature (a new library, channels, games):

1. **Register routes as usual**, with `m.Handle` in `internal/server`. Use `a(...)` or `s.requireAdmin(...)`,
   since the tests read sign-in from those. Anything that changes data must not be a `GET`.
2. **Document them**, or `go test ./internal/server` fails, and its message has a stub to paste: a block in
   `docs/openapi.json` and a row in the right table here.
3. **Schemas come from Go types.** Add the response type to `schemaTypes` in
   `internal/server/contract_test.go`, then run
   `WMS_UPDATE_OPENAPI=1 go test ./internal/server -run TestSchemasMatchGoTypes`.
4. **Add a name to `serverFeatures`**, so apps can tell a server has it, and raise `apiRevision` (and
   `info.version` in `openapi.json`, which a test compares).
5. **Only add.** Don't rename or remove a field or route in v1, or change what a value means. New fields
   must be safe to ignore. Clients are told to ignore what they don't know.
6. **Media and native players.** Anything a player fetches goes in `streamRoutes`
   (`internal/server/stream_tokens.go`) with a check that the request is for the token's item, and only
   read-only routes go there. Name HLS streams `<kind>-<hls.Key(path)>-...`, as the token check reads the item
   from that. Give playback back through `/items/{id}/playback`: a new item kind gets a case there, not a
   route of its own the apps have to discover. Don't assume a browser.
7. **Keep rules on the server.** If two clients would have to agree on it (what counts as finished, what's
   hidden, what comes next), compute it here and send the answer; if it's arranging a screen, it's the
   client's. Long lists take `limit` and `offset`. Errors are `{"error": "..."}` in plain words.
8. **Think of a remote.** Web pages: everything clickable must be reachable with arrow keys. Use real buttons
   and links; `web/src/tv.ts` does the rest.

## Every route

Paths are relative to `/api/v1`. Full request and response shapes are in [openapi.json](openapi.json).
"Sign-in" is what the route needs: none, user, or admin; `st` means a
[stream token](#stream-tokens) for the item works too.

### Server

| Route | Sign-in | What it does |
|---|---|---|
| `GET /capabilities` | none | What this server is and can do: API versions, features, playback profiles and shared rules. The first thing a client asks. |
| `POST /identity` | none | Prove this is the server an app pinned: signs a fresh challenge with the server's key. See "Server identity". |
| `GET /status` | none | Server name, version, branding, default theme, and the signed-in user if any. |
| `GET /health` | none | Liveness check. |
| `GET /about` | user | Server version and the engines built into it. |
| `GET /prefs` | user | Server-wide playback defaults every viewer needs. |
| `GET /branding/logo` | none | The custom logo, if the server has one (404 otherwise). The `brandLogo` value in `/status` changes when it does. |
| `PUT /branding/logo` | admin | Replace the logo: the PNG or JPEG itself as the body, 2 MB at most. |
| `DELETE /branding/logo` | admin | Go back to the app icon. |
| `GET /system` | admin | Host, FFmpeg and hardware encoder details. |
| `GET /settings` | admin | All server settings as strings; secrets are masked. |
| `PUT /settings` | admin | Change settings; unknown keys are ignored. |
| `GET /fs` | admin | Browse the server's folders, to choose library folders. |
| `POST /fs/pick` | admin | Open the system folder picker on the server's own screen (only from that machine; 501 elsewhere). |
| `POST /metadata/test` | admin | Check a metadata API key before saving it. |
| `GET /agents` | admin | Custom metadata agents, with header values masked. |
| `PUT /agents` | admin | Replace the list of agents, 20 at most. |
| `POST /agents/test` | admin | Ask an agent about a title without changing anything. |
| `GET /logs` | admin | What is logged, where, and the log files. |
| `PUT /logs` | admin | Change log categories, level or folder. |
| `GET /logs/files/{name}` | admin | Download one log file. |
| `POST /logs/open` | admin | Open the log folder on the server's own screen. |
| `POST /logs/client` | user | Report an error from a client. Only stored when the owner turned on app errors; 204 otherwise; rate limited. |
| `GET /update` | admin | What is installed and what is available (404 in builds that can't update themselves). |
| `POST /update/check` | admin | Look for a new version now. |
| `POST /update/apply` | admin | Download, verify and install the newest release, then restart. |
| `GET /modules` | admin | Every module: version in use, supported range, status and what is available (404 in builds that can't update them). |
| `POST /modules/check` | admin | Look for module updates now. |
| `POST /modules/{id}/install` | admin | Download, verify and install the version the last check found, with no restart. 202; follow it with `GET /modules`. |
| `POST /modules/{id}/rollback` | admin | Go back to the previous version or the built-in copy. |

### Sign-in and accounts

| Route | Sign-in | What it does |
|---|---|---|
| `POST /setup` | none | Create the first administrator (409 once there is an account). |
| `POST /login` | none | Sign in. 429 with Retry-After after repeated failures. Password-free Kids and Guest profiles only sign in where `/profiles` lists them. |
| `POST /logout` | none | End the session the request carries. |
| `GET /profiles` | none | The "Who's watching?" list where the server offers it: `style` is `profiles` (with `profiles`) or `classic` (ask for name and password). |
| `POST /pair/invite` | user | Code typed on the device: get 6 digits to type there, for this account or a Kids or Guest profile it manages. Adults only. One open at a time per person; lasts 5 minutes. |
| `POST /pair/redeem` | none | Code typed on the device: use the 6 digits and get a sign-in. Works once. Failures say why in `reason`: `wrong-code` (400), `expired` or `used` (410), `too-many-tries` (429). |
| `POST /pair/start` | none | TV sign-in: get a code to show. Rate limited per address (429). |
| `POST /pair/poll` | none | TV sign-in: has someone answered? `status` is `pending`; `approved` with `token` and `user` (once only); 403 `denied`; 410 `expired`; 429 `slow-down`. |
| `POST /pair/info` | user | TV sign-in, on the phone or computer: what is asking, and which profiles this person may put on it. Adults only. Wrong codes are limited. |
| `POST /pair/approve` | user | TV sign-in: approve, as this account or a Kids or Guest profile it manages. Works once. |
| `POST /pair/deny` | user | TV sign-in: refuse; the TV is told. |
| `GET /me` | user | The signed-in account. |
| `GET /me/ui` | user | The account's interface choices: any JSON object the clients agree on (home layout, for one). See "Choices that follow the account". |
| `PUT /me/ui` | user | Replace them: a JSON object of 16 KB at most. |
| `GET /users` | user | Accounts this one may manage: everyone for the admin, Kids profiles for adults allowed to. |
| `POST /users` | user | Add an account or profile (adults may add Kids profiles unless the admin locked that). |
| `DELETE /users/{id}` | user | Remove an account this one manages. |
| `PUT /users/{id}/access` | user | Set an account's limits, or clear a Kids or Guest password. |
| `PUT /users/{id}/password` | user | Change a password. Your own needs `current`; other sessions of that account end. |

### Browsing

| Route | Sign-in | What it does |
|---|---|---|
| `GET /home` | user | The home page: rows (`hubs`) of continue watching, next up and recently added, featured items, and the libraries. Rows carry a stable `key`. |
| `GET /libraries` | user | The libraries this account may open. |
| `POST /libraries` | admin | Add a library; scanning starts. |
| `PUT /libraries/{id}` | admin | Change a library; scanning starts. |
| `DELETE /libraries/{id}` | admin | Remove a library (the files stay). |
| `POST /libraries/{id}/scan` | admin | Scan now. |
| `GET /libraries/{id}/scan` | user | Scan progress. |
| `GET /genres` | user | Genres among movies and other items. |
| `GET /items` | user | List and filter items. All filtering and sorting happens here, so clients never filter themselves. |
| `GET /items/{id}` | user | One item, with its progress, next episode, intro range and (discs) how it starts. |
| `PUT /items/{id}` | admin | Edit an item's details or refresh them; any field may be left out. |
| `GET /items/{id}/poster` | user, `st` | Poster image. |
| `GET /items/{id}/backdrop` | user, `st` | Backdrop image (the poster if it has none). |
| `GET /items/{id}/artwork` | admin | Artwork to choose from. |
| `GET /artwork/preview` | admin | One artwork candidate as an image. |
| `GET /items/{id}/recommend` | user | What to watch when this ends and there is no next episode: an `item`, with its `show` for a series. |
| `GET /shows` | user | List and filter shows. |
| `GET /shows/genres` | user | Genres among shows. |
| `GET /shows/networks` | user | TV networks with their shows and genres. |
| `GET /shows/{id}` | user | A show with its seasons, its episodes in order, and `nextUpId`: the episode the Play button means. |
| `PUT /shows/{id}` | admin | Edit a show, set its intro range or match it again. |
| `PUT /shows/{id}/view` | admin | A show's episode layout: `cards`, `list`, or empty for the default. |
| `GET /shows/{id}/artwork` | admin | Artwork to choose from. |
| `GET /shows/{id}/art/{kind}` | user | A show's picture: `poster`, `backdrop` or `season` (with `n`). |
| `POST /items/{id}/progress` | user | Save the viewer's position. Past 92% counts as watched. Discs send `discTitle` too. |
| `DELETE /items/{id}/progress` | user | Forget the position (mark as not started). |

### Playback

| Route | Sign-in | What it does |
|---|---|---|
| `POST /items/{id}/playback` | user | The way to play anything: say what this client can play, get a decision, ready-to-use addresses, tracks, where to start and what comes next. |
| `GET /playback/profiles` | user | The built-in client profiles. |
| `POST /items/{id}/stream-token` | user | A stream token for this item's media (see "Stream tokens"). Needs a real sign-in, not another stream token. |
| `GET /items/{id}/play` | user | The web player's older way to ask: decides for a browser only. Apps use `/items/{id}/playback`. |
| `GET /items/{id}/file` | user, `st` | The file itself, with byte ranges (videos and books). |
| `GET /items/{id}/stream` | user, `st` | An audio file: as it is, or converted to AAC while streaming with `transcode=1` (`start` seeks). |
| `GET /items/{id}/subtitles/{file}` | user, `st` | A text subtitle track as WebVTT: `{n}.vtt`, n being its index. The first request reads the whole file, so it can take a while. |
| `GET /hls/{id}/master.m3u8` | user, `st` | HLS master playlist: the stream and its WebVTT subtitle renditions. |
| `GET /hls/{id}/playlist.m3u8` | user, `st` | HLS media playlist. |
| `GET /hls/{id}/seg/{n}` | user, `st` | One HLS segment (`{n}.ts`, `{n}.m4s`) or the fMP4 header (`init.mp4`). |
| `GET /hls/{id}/subs/{file}` | user, `st` | One subtitle rendition: `{n}.m3u8` and its `{n}.vtt`. |
| `DELETE /hls/{id}` | user | Stop a stream when finished, freeing the transcoder. Streams nobody asks for also stop by themselves. |

### Discs

| Route | Sign-in | What it does |
|---|---|---|
| `POST /items/{id}/dvd` | user | Open a disc session (DVD or Blu-ray). Answers with the first Presentation. |
| `GET /dvd/{sid}` | user | The current Presentation of a session. |
| `POST /dvd/{sid}/action` | user | Send viewer input or a player event; answers with the next Presentation. |
| `DELETE /dvd/{sid}` | user | Close a session. |
| `GET /dvd/{sid}/menu/{serial}/overlay.png` | user, `st` | The menu's picture layer, to draw over the video. |
| `GET /dvd/{sid}/menu/{serial}/highlight/{n}` | user, `st` | A button's highlight, to draw over that. `f` picks a frame of an animated one. |
| `GET /dvd/{sid}/still/{name}` | user, `st` | A still picture (a menu or slide without video). |
| `GET /dvd/{sid}/subs` | user | Sub-picture subtitle events for a time window. Normally reached through `title.subtitlesUrl`. |
| `GET /dvd/{sid}/sub/{name}` | user, `st` | One sub-picture as a PNG. |
| `GET /items/{id}/disc` | user | The disc page: the feature, extras and menus to jump into. |
| `PUT /items/{id}/disc/labels` | admin | Name a disc's titles and chapters. |
| `GET /items/{id}/disc/thumb/{ttn}/{n}` | user, `st` | A picture of a title (n = 0) or chapter; the first request makes it. |

### Music, audiobooks and podcasts

| Route | Sign-in | What it does |
|---|---|---|
| `GET /music/albums` | user | Albums or books of a library. |
| `GET /music/artists` | user | Artists or authors. |
| `GET /music/tracks` | user | Tracks of one album, or all songs; an empty `album` or `artist` means none or unknown. |
| `GET /podcasts` | user | Podcast subscriptions. |
| `GET /podcasts/search` | admin | Search a podcast directory. |
| `POST /podcasts` | admin | Subscribe. |
| `PUT /podcasts/{id}` | admin | How many new episodes to download by themselves. |
| `POST /podcasts/{id}/refresh` | admin | Check the feed now. |
| `DELETE /podcasts/{id}` | admin | Unsubscribe; downloaded episodes stay. |
| `POST /items/{id}/download` | admin | Keep a podcast episode on disk. |

### Books

| Route | Sign-in | What it does |
|---|---|---|
| `GET /items/{id}/book` | user | What a reader needs: `format` (cbz, epub, pdf), `reading` direction, then `pages` for comics or `spine`, `toc` and `words` for EPUB. |
| `GET /items/{id}/page/{n}` | user | One comic page, n from 0. |
| `GET /items/{id}/epub/{file}` | user | A file inside an EPUB. Treat its HTML as untrusted: no scripts. |
| `GET /items/{id}/annotations` | user | The account's bookmarks and highlights in this book. |
| `POST /items/{id}/annotations` | user | Add a bookmark or highlight. |
| `PUT /annotations/{aid}` | user | Change a highlight's colour or note. |
| `DELETE /annotations/{aid}` | user | Remove one. |
| `GET /items/{id}/search` | user | Search inside an EPUB. |
| `GET /dictionary` | user | Definitions of one word, fetched by the server so devices stay private. |

### Themes

| Route | Sign-in | What it does |
|---|---|---|
| `GET /themes` | none | All themes, and the server's default. |
| `GET /themes/{id}/tokens.json` | none | A theme as data for native apps: colours, lengths and fonts. |
| `GET /themes/{id}/download` | none | The theme as a ZIP. |
| `GET /themes/{id}/{file}` | none | A file of a theme: theme.css and the images and fonts it uses. |
| `POST /themes` | admin | Install a theme: the ZIP as the body. |
| `DELETE /themes/{id}` | admin | Remove a theme. |

---
<sub>WittyMediaServer 0.4.0-alpha documentation</sub>
