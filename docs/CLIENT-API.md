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
- [Game details](#game-details)
- [Legal notices](#legal-notices)
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

## Audio CDs

An audio CD image is an item of kind `cd-image`. In a Discs library that is all there is. In a Music library each of
its tracks is an ordinary `audio` item as well, with `cdDisc` naming the disc, so albums, songs, search and queues
work with no special case, and `GET /items/{id}/stream` plays one track as plain samples with byte ranges (or AAC with
`?transcode=1`). That is all a simple player needs.

**To play the disc as a disc,** gaplessly, use the whole-disc stream. `POST /items/{id}/playback` on the `cd-image` answers
with `decision: cd`, a `url` for the stream and a `cdUrl` for its map. Or go straight there:

- `GET /cd/{id}` lists `tracks`, each with `pregap`, `start`, `end` and `indexes` in sectors from the start of the disc
  (75 to the second, 588 stereo samples each), plus `formats` you can ask for, the one `suggested` for this person's
  connection, and `positionSec`, their place on the disc.
- `GET /cd/{id}/audio` is the stream. **There is one stream and tracks are places in it,** so nothing needs joining.

**The map of a stream.** Work it out from the track table; the server does the same:

- With no `tracks`: every audio track in order, each from its `pregap` to its `end`. Sample 0 is sector `pregap` of the
  first audio track. Data tracks are left out. To start where a player does, use `start` to skip the first track's pregap.
- With `tracks=3,7,1`: each from its `start` to its `end`, in that order. `0` is the hidden track, what is in track 1's pregap
  (`hiddenTrack` says when there is one).
- `loops=N` plays all of that N times over with no join.

**Formats and seeking:**

| `format` | Seek | Notes |
|---|---|---|
| `flac` (default) | New request with `start=SAMPLE`, then add `start` to the player's clock | Lossless. Its duration is in the stream's header. `X-WMS-Samples` says how long it is |
| `wav` | In the player, by byte range | Lossless, 1.4 Mbit/s, no work for the server |
| `aac` | New request with `start=SAMPLE` | For slow connections. Gapless across tracks too, though not sample-exact after a seek. Counts against the server's audio conversions |

A busy server answers 503 with `Retry-After`. Pre-emphasised tracks (`preEmphasis`) are corrected unless `deemphasis=0`.

**Your place.** Send `POST /items/{id}/progress` on the `cd-image` with `positionSec` counted from sector 0 of the disc, and
`DELETE` it when someone presses stop. Unlike films, any position above 3 seconds is worth resuming.

**A disc that isn't music.** `problem` is `dts` or `noise` when the audio would be loud noise, with a `warnings` entry that
says why. Ask before playing it. Nothing is decrypted or unscrambled: a clean copy is the only way to play one.

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

## Game details

Scans only list a game's file, and details arrive afterwards in the background, so a game's item fills
in over time. `title`, `year`, `overview`, `genres`, `studio` (its developer), `rating` (out of 10: critics
if any, otherwise players), `contentRating` and the art on `GET /items/{id}` are already the merged result,
in the owner's chosen order of sources, and they're what lists can sort and filter by. A hidden game answers
404 here like anywhere, so parental controls need nothing from a client.

`GET /games/{id}/metadata` has the rest:

- `status`: `pending` while it's being looked up, then `done`. Ask again in a while if it's `pending`.
- `matchSource`, `matchMethod` (`checksum`, `serial`, `name` or `manual`) and `confidence` out of 100. No
  `matchSource` means no match. `locked` means automatic work leaves it alone.
- `scores`: `{source, kind, value, votes, url}`, each out of 100. `kind` is `critic` or `user`. Always show
  who gave a score.
- `ages`: `{system, value, age}`, such as ESRB and T, with what that means in years. `contentRating` on the
  item is the strictest of these, written `ESRB T`.
- `art`: `{kind, url}`, with `kind` being `screenshot`, `title` or `artwork`. Pictures are fetched by the server,
  so a client never talks to a source. A picture that can't be found answers 404: leave it out.
- `credits`: `{name, text, site, url, license}`. **Show these wherever the game's details are.** Some sources'
  terms require it, RAWG's on every page using its data, and Wikipedia's text needs its link and licence.

A game with no `poster` should use art made from its title and console, as the web client does. In lists, ask for
`/items/{id}/poster?w=320`: full-size box art is several times larger than a grid needs.

The first pass over a large collection takes a while, on purpose: each source is asked no faster than its
rules allow. `GET /gamemeta/status` says how it's going, for administrators.
## Legal notices

Every client shows the same short notices beside third-party and local content, so a person gets the
same words everywhere. `GET /legal/notices` needs no sign-in and returns them by kind:

```json
{
  "version": 1,
  "ackVersion": 1,
  "linkText": "Legal",
  "ack": "I understand: ...",
  "page": "/legal",
  "owner": { "notice": "", "contact": "" },
  "notices": {
    "webResults": { "text": "These results come from third-party services on the web. ...", "section": "third-party" }
  }
}
```

- Show `text` as it is, small, near the content, with a link to the server's `page` (a web page anyone
  can open) or your own view of `GET /legal`. `section` is the part of that page it's about, as an
  address's `#section`.
- [LEGAL-NOTICES-CHECKLIST.md](LEGAL-NOTICES-CHECKLIST.md) says which kind goes on which screen. An
  unknown kind is a newer server's: ignore it. Don't hard-code the words, except where your app has to
  say something before it has a server: `legalConnect` in the shared strings is the `connect` notice.
- `owner.notice` and `owner.contact` are written by the server's owner. They're plain text: never show
  them as HTML.
- `GET /legal` returns `sections` (each with `id`, `title` and paragraphs in `body`), `services` (the
  privacy list) and the same `owner`. A paragraph starting with `- ` is a bullet. Inline, only
  `**bold**`, `[text](address)` and `[[FILL IN: ...]]` appear; an address starting with `/` is a page of
  the web app.
- `/status` has `legalAck`: false only for an administrator who hasn't seen the first-run notice. Show
  them `ack` once, then call `POST /legal/acknowledge`. `POST /setup` takes `acknowledgedLegal: true`
  when a client's own setup screen has shown it. Neither sends anything off the server.

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
| `GET /licenses` | user | Everything in this server someone else made, with its license: `{groups, components}`. |
| `GET /licenses/{id}` | user | One component's full license text: `{text}`. |
| `GET /legal/notices` | none | The short legal notices, by kind, and what this server's owner wrote. See "Legal notices". |
| `GET /legal` | none | The full Legal page, with the privacy list and what this server's owner wrote. |
| `POST /legal/acknowledge` | admin | Record that an administrator has seen the first-run notice, so `legalAck` in `/status` turns true. |
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
| `PUT /logs` | admin | Change each category's level (`levels`), each module's level and advanced options (`modules`), or the folder, or record everything for a while (`recordMinutes`). Takes effect at once, with no restart. |
| `GET /logs/files/{name}` | admin | Download one log file. |
| `POST /logs/open` | admin | Open the log folder on the server's own screen. |
| `POST /logs/client` | user | Report errors from a client: `{app, version, device, events: [{level, kind, message, stack, page, session, request, count, time}]}`. `session` is the stream, disc or game session in play, and `request` the `X-Request-Id` of a failed request. 64 KB and 25 events a request, 60 events a minute per account; 429 past that. 204 when stored, or when the owner turned app errors off. |
| `GET /logs/entries` | admin | The log viewer: `level`, `cat`, `session`, `module`, `q`, `after` (a seq, for a live tail), `before` (a time, for going back), `limit`, `history`. |
| `GET /logs/summary` | admin | Version, modules, system, GPU and encoders, disk space and settings that matter. No secrets. |
| `GET /logs/crashes` | admin | Crash reports, newest first. |
| `GET /logs/crashes/{id}` | admin | One crash report's text. |
| `DELETE /logs/crashes/{id}` | admin | Delete a crash report. |
| `POST /logs/crashes/{id}/seen` | admin | Stop pointing this report out. |
| `GET /logs/bundle` | admin | The support bundle, a zip made on request: `days`, `redact=1` hides titles and paths. Nothing is uploaded anywhere. |
| `GET /logs/bundle/preview` | admin | What the support bundle would hold. |
| `GET /diagnose/targets` | admin | WittyDiagnoser: the libraries, data folders and known discs that can be checked. |
| `GET /diagnose/browse` | admin | Folders and disc images inside the libraries and data folders only. |
| `POST /diagnose/disc` | admin | Start checking a disc: `{path, crawl, damage}`. 202 with the job; 429 when too many wait. Read-only. |
| `POST /diagnose/storage` | admin | Start checking folders: `{targets, readTest, writeTest, walk}`. 202 with the job. Target IDs only, never paths. |
| `GET /diagnose/jobs` | admin | Checks running or waiting, then saved reports. |
| `GET /diagnose/jobs/{id}` | admin | One check, with progress and findings. |
| `DELETE /diagnose/jobs/{id}` | admin | Cancel a check. |
| `GET /diagnose/jobs/{id}/report` | admin | The report as a file: `format=json` or `txt`, `redact=1` hides the disc name and file names. Nothing copyrighted is in it. |
| `GET /update` | admin | What is installed and what is available (404 in builds that can't update themselves). |
| `POST /update/check` | admin | Look for a new version now. |
| `POST /update/apply` | admin | Download, verify and install the newest release, then restart. |
| `GET /modules` | admin | Every module: version in use, supported range, status, what is available, what it needs, whether it is installed, and every kind of library and what it takes (404 in builds that can't update them). |
| `POST /modules/check` | admin | Look for module updates now. Nothing else here goes online. |
| `GET /modules/{id}/versions` | admin | Every version known, explained, without going online: release notes as plain text, whether each can be chosen, and why not. |
| `POST /modules/{id}/install` | admin | Download, verify and install a version, with no restart: the newest the last check found, or `{"version": "0.2.0", "confirm": true}`. A choice with a warning (older, withdrawn as broken, taken out of use before) is answered 409 with `"code": "confirm"` and the `warning` until the body says confirm. 202; follow it with `GET /modules`. |
| `POST /modules/{id}/rollback` | admin | Stop using this version, and go back to the previous one or the built-in copy. |
| `POST /modules/{id}/builtin` | admin | Use the built-in copy, without blaming the version in use. |
| `POST /modules/{id}/pin` | admin | Keep the version in use; automatic updates leave it alone. A version withdrawn for security reasons goes out of use anyway. |
| `DELETE /modules/{id}/pin` | admin | Let automatic updates move it again. |
| `PUT /modules/{id}/autoupdate` | admin | Whether this module may update by itself while the server's setting is on: `{"on": false}`. It still looks for updates and offers them. Not for modules that update with the server. |
| `POST /modules/{id}/latest` | admin | End a pin and move to the newest version that fits. 202. |
| `POST /modules/{id}/add` | admin | Put a module on this server. Works straight away, offline. |
| `POST /modules/{id}/remove` | admin | Take a module off this server. Answered 409 with `"code": "module-in-use"` and the `libraries` that need it. No data is touched. |
| `PUT /modules/installed` | admin | Make the set of modules on this server what someone chose: `{"installed": ["wittyvideo"]}`, as first-run setup does. Refused whole if libraries need something left out. |
| `PUT /modules/{id}/source` | admin | Take this module's releases from a fork: `{"repo": "someone/WittyDVD", "key": "<public key>", "confirm": true}`. Answered 409 with `"code": "confirm"` and the security warning until confirmed. Show that warning; don't replace it. See [MODULES.md](MODULES.md), "Using someone's fork". |
| `DELETE /modules/{id}/source` | admin | Go back to the module's own repository. |
| `GET /modules/libraries` | user | Every kind of library a module provides, whether it can be made here, and what it takes. A kind not listed is always available. |
| `PUT /modules/libraries/{type}/label` | admin | What this server calls a kind of library, for when two read alike or two modules share one: `{"label": "Blu-rays and DVDs", "taken": {"movies": "Movies"}}`. An empty label goes back to the usual name. `taken` is what the app shows for the other kinds, and a name already in use is refused. |
| `GET /modules/web` | user | The web bundles in use instead of the copies in the app. Usually none. |
| `GET /modules/{id}/web/{version}/{path}` | user | A file of the bundle in use. Only names in its signed list exist. |
| `POST /modules/{id}/failed` | user | A browser saying a bundle wouldn't load or crashed: `{"version": "...", "reason": "..."}`. Three within a day, at most one a minute, take that version out of use. 204. |

`POST /libraries` answers 409 with `"code": "module-missing"` and the `modules` that would make it possible when a kind of library's module isn't installed.

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
| `GET /libraries/{id}/scan` | user | Scan progress. For an administrator, a games library also has `skipped`: files the last scan couldn't use, by extension. |
| `GET /genres` | user | Genres among movies and other items. |
| `GET /items` | user | List and filter items. All filtering and sorting happens here, so clients never filter themselves. Games: `console=<id>` for one console, `played=1` for what this user has played, `genre=`, `decade=1990`, `players=2` (at least this many), `minRating=8` (out of 10), `unmatched=1` (no match found), and `sort=title`, `rating`, `year` or `added`. |
| `GET /items/{id}` | user | One item, with its progress, next episode, intro range and (discs) how it starts. |
| `PUT /items/{id}` | admin | Edit an item's details or refresh them; any field may be left out. |
| `GET /items/{id}/poster` | user, `st` | Poster image. For games, `w=320` (up to 640) asks for a smaller copy, for lists. |
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

### Audio CDs

| Route | Sign-in | What it does |
|---|---|---|
| `GET /cd/{id}` | user, `st` | A disc's tracks and layout, and this person's place on it. |
| `GET /cd/{id}/audio` | user, `st` | The disc as one continuous stream: `format`, `tracks`, `loops`, `start`, `deemphasis`. |
| `GET /cd/{id}/graphics` | user, `st` | CD+G packets for a run of sectors. |
| `GET /cd/{id}/matches` | admin | What this disc could be, from MusicBrainz, asked with only its table of contents. |
| `POST /cd/{id}/identify` | admin | Use a release's names and cover. An empty `releaseId` goes back to the disc's own. |
| `GET /cd-covers/{release}` | admin | A release's cover, small, for choosing between matches. |

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

### Channels and live TV

| Route | Sign-in | What it does |
|---|---|---|
| `GET /guide` | user | Guide. |
| `GET /tv/{key}/tune` | user | Tune. |
| `GET /tv/{key}/live/playlist.m3u8` | user | Live playlist. |
| `GET /tv/{key}/live/seg/{name}` | user | Live segment. |
| `POST /tv/{key}/leave` | user | Live leave. |
| `GET /tv/{key}/logo` | user | A tuner channel's logo, fetched and kept by the server, so the site a playlist names never sees the viewer. The `logo` in `/guide` and `/tune` is this address. |
| `GET /tv/settings` | admin | TV settings. |
| `PUT /tv/settings` | admin | Put TV settings. |
| `PUT /libraries/{id}/main` | admin | Library show main. |
| `GET /channels` | user | Channels. |
| `POST /channels` | admin | Create channel. |
| `POST /channels/preview` | admin | Channel preview. |
| `GET /channels/suggestions` | admin | Channel suggestions. |
| `GET /channels/options` | admin | Channel options. |
| `POST /channels/from-network` | admin | Channel from network. |
| `POST /channels/networks/refresh` | admin | Refresh networks. |
| `GET /channels/networks/refresh` | admin | Refresh networks progress. |
| `GET /channels/{id}` | admin | Channel. |
| `PUT /channels/{id}` | admin | Update channel. |
| `DELETE /channels/{id}` | admin | Delete channel. |
| `POST /channels/{id}/rebuild` | admin | Rebuild channel. |
| `GET /channels/{id}/history` | admin | Channel history. |
| `POST /channels/{id}/restore` | admin | Restore channel. |
| `GET /channels/{id}/logo` | user | Channel logo. |
| `PUT /channels/{id}/logo` | admin | Set channel logo. |
| `DELETE /channels/{id}/logo` | admin | Delete channel logo. |
| `GET /tv/encrypted-help` | user | Encrypted help. |
| `GET /tuners/drivers` | admin | Tuner drivers. |
| `GET /tuners/discover` | admin | Tuner discover. |
| `GET /tuners` | admin | Tuners. |
| `POST /tuners` | admin | Create tuner. |
| `PUT /tuners/{id}` | admin | Update tuner. |
| `DELETE /tuners/{id}` | admin | Delete tuner. |
| `POST /tuners/{id}/refresh` | admin | Refresh tuner. |
| `POST /tuners/{id}/scan` | admin | Scan tuner. |
| `GET /tuners/{id}/scan` | admin | Scan progress2. |
| `GET /tuners/{id}/channels` | admin | Tuner channels. |
| `POST /tuners/{id}/channels/enable` | admin | Enable all channels. |
| `PUT /tuner-channels/{id}` | admin | Edit tuner channel. |
| `GET /epg/sources` | admin | EPG sources. |
| `POST /epg/sources` | admin | Add EPG source. |
| `DELETE /epg/sources/{id}` | admin | Delete EPG source. |
| `POST /epg/sources/{id}/refresh` | admin | Refresh EPG source. |
| `GET /epg/ids` | admin | EPGI ds. |
| `GET /recordings` | user | Recordings. |
| `POST /recordings` | user | Schedule. |
| `DELETE /recordings/{id}` | user | Cancel recording. |

### Games

| Route | Sign-in | What it does |
|---|---|---|
| `GET /games/consoles` | user | Game consoles. |
| `GET /games/modes` | user | Game modes. |
| `GET /consoles/{console}/controls` | user | Game controls. |
| `PUT /consoles/{console}/controls` | user | Put game control. |
| `POST /games/{id}/play` | user | Game play. |
| `POST /games/{id}/prepare` | user | Game prepare. |
| `GET /games/{id}/saves` | user | Game saves. |
| `GET /games/{id}/saves/{slot}/thumb` | user | Game save thumb. |
| `DELETE /games/{id}/saves/{slot}` | user | Game save delete. |
| `GET /games/facets` | user | What a library's games can be filtered by, with how many there are. Query: `library`, `console`. |
| `GET /games/{id}/metadata` | user | A game's match, dates, makers, players, scores, age ratings, pictures and credits. See [Game details](#game-details). |
| `GET /games/{id}/art/{n}` | user | One of a game's pictures. Use the `url` in its metadata's `art` list as given. |
| `GET /games/{id}/metadata/search` | admin | Games this might be, from every source in use. Query: `q`. Answers `results` and `failed`. |
| `PUT /games/{id}/metadata/match` | admin | This is the right game: `{source, id, lock}`. With no source, only sets the lock. |
| `DELETE /games/{id}/metadata/match` | admin | This match is wrong, and there's no right one. |
| `POST /games/{id}/metadata/refresh` | admin | Ask the sources about this game again, in the background. |
| `GET /gamemeta/status` | admin | How the background lookup is going, by console, and whether any source has a key. |
| `POST /gamemeta/refetch` | admin | Forget what every source said about every game and ask again. Locked games stay. |
| `GET /gamemeta/sources` | admin | Every source, in the order they're believed: what it needs, costs and sends. Key values are never included. |
| `PUT /gamemeta/sources` | admin | Set the sources' order and whether each is on: `{sources: [{id, on}]}`. Costs no requests. |
| `POST /gamemeta/sources/{id}/test` | admin | Does this work? `{fields: {setting: value}}`. Saves nothing. Keys themselves are saved through `PUT /settings`, and masked when read back. |
| `GET /game-controllers` | user | Controllers this server knows by vendor and product. |
| `GET /game-profiles` | user | This account's controller profiles. |
| `POST /game-profiles` | user | Makes a controller profile. |
| `GET /game-profiles/resolve` | user | Which profile and mapping apply to a controller here. |
| `GET /game-profiles/export` | user | A file of these profiles, or of all of them. |
| `POST /game-profiles/import` | user | Adds the profiles in an exported file. |
| `GET /game-profiles/{id}` | user | One controller profile. |
| `PUT /game-profiles/{id}` | user | Replaces a controller profile. |
| `DELETE /game-profiles/{id}` | user | Deletes a controller profile. |
| `POST /game-profiles/{id}/copy` | user | Copies a controller profile. |
| `POST /game-profiles/{id}/active` | user | Puts a controller profile in or out of use. |
| `GET /game-background` | user | This account's controller screen background: fit, position and dimming. |
| `PUT /game-background` | user | Changes its fit, position and dimming. |
| `DELETE /game-background` | user | Removes the picture. |
| `GET /game-background/image` | user | This account's own picture. |
| `PUT /game-background/image` | user | Uploads a picture as the body: PNG, JPEG or WebP, up to 10 MB. |
| `GET /games/settings/schema` | user | Describe every game setting: its kind, range, default and caution. |
| `GET /games/settings` | user | A person's settings for a game (?item=) or console (?console=): effective values, where each came from, every level, and the server's limits. |
| `PUT /games/settings` | user | Change one level: a patch (null removes a value), a whole settings file, or a reset. Server levels need an administrator. |
| `POST /games/settings/validate` | user | Check a settings file without saving it: errors and ignored things, by line. |
| `GET /games/recent` | user | A person's most recently played games, or favourites with ?favourites=1, with play counts. |
| `GET /game-bezels` | user | A person's own images for around the picture. |
| `POST /game-bezels` | user | Add an image, as the request body. PNG, JPEG or WebP, decoded and made again. |
| `GET /game-bezels/{bezel}` | user | One of a person's images, as a PNG. |
| `DELETE /game-bezels/{bezel}` | user | Remove one of a person's images. |
| `GET /games/{id}/core-options` | user | What this game's core offers, what's chosen, and what it will ignore. |
| `GET /games/{id}/meta` | user | What a person keeps about a game: favourite, note, play count, last played. |
| `PUT /games/{id}/meta` | user | Change a game's favourite mark or note. |
| `GET /games/{id}/cheats` | user | A person's cheat codes for a game. |
| `PUT /games/{id}/cheats` | user | Replace a person's cheat codes for a game. A running game takes them at once. |
| `POST /games/{id}/cheats/import` | user | Read a cheat file, as the request body, and say what's in it. Nothing is kept. |
| `GET /games/{id}/patches` | user | A person's IPS and BPS patches for a game. |
| `POST /games/{id}/patches` | user | Add a patch, as the request body, with ?name=. Applied to a copy when the game starts. |
| `PUT /games/{id}/patches/{patch}` | user | Turn a patch on or off. |
| `DELETE /games/{id}/patches/{patch}` | user | Remove a patch. |
| `GET /games/{id}/saves/{slot}/file` | user | Download a saved state, with a header that an import checks. |
| `PUT /games/{id}/saves/{slot}/file` | user | Put a state file into a slot, as the request body. Slots 1 to 99. |
| `GET /games/{id}/battery` | user | Download a game's battery save. |
| `PUT /games/{id}/battery` | user | Replace a game's battery save, as the request body. Refused while it's being played. |

### Game PCs

**WittyCouch** plays the games on any PC running Sunshine, through this server. Each account has its own list of PCs, and its own pairing with each.
A client can show the player in a web view (`/couch/{id}/play/{app}`), or start a stream and connect with `POST /couch/sessions/{sid}/webrtc`:
H.264 and Opus, with data channels `control` (reliable) and `input` (unordered), whose messages are in `modules/wittycouchhost/PROTOCOL.md`.
[GAME-RIGS.md](GAME-RIGS.md) is the design.

| Route | Sign-in | What it does |
|---|---|---|
| `GET /couch` | user | Whether this account may connect to game PCs, whether this server can stream them, and what they are built on. |
| `GET /couch/find` | user | Look for PCs running Sunshine on the server's network. |
| `GET /couch/hosts` | user | This account's own game PCs, and how they are doing. |
| `POST /couch/hosts` | user | Add a game PC by address. It has to answer as Sunshine. |
| `PATCH /couch/hosts/{id}` | user | Rename a game PC. |
| `DELETE /couch/hosts/{id}` | user | Forget a game PC. |
| `POST /couch/hosts/{id}/pair` | user | Start pairing, and get the PIN to type into Sunshine. |
| `GET /couch/hosts/{id}/pair` | user | How pairing is going. |
| `DELETE /couch/hosts/{id}/pair` | user | Stop waiting for a PIN. |
| `GET /couch/hosts/{id}/apps` | user | What a paired PC offers. |
| `GET /couch/hosts/{id}/apps/{app}/art` | user | An app's picture. |
| `POST /couch/hosts/{id}/apps/{app}/play` | user | Start an app, and a stream of it. Body: width, height, fps, bitrate. |
| `POST /couch/hosts/{id}/wake` | user | Send a game PC the Wake-on-LAN signal. |
| `POST /couch/hosts/{id}/quit` | user | End what is running on a game PC. |
| `GET /couch/sessions/{sid}` | user | A stream of this account's. |
| `POST /couch/sessions/{sid}/webrtc` | user | Connect to a stream: a WebRTC offer in, the answer out. |
| `DELETE /couch/sessions/{sid}` | user | End a stream. With ?quit=1, close the game too. |

### Shared PCs and WittyRigs

A PC an admin shares has a library of its own (`type: games`, `console: rig`). To play, call `POST /rigs/games/{id}/play` with
`{"client": "browser|desktop|android|apple", "os": "windows|mac|linux|chromeos|android|androidtv|firetv|ios|tvos"}`.
It answers `methods`, best first, each of them something this device can really do: `client` means `POST /rigs/games/{id}/stream` and the player here. `POST /games/{id}/play` on
one of these games answers 409 with `"rig": true`.

| Route | Sign-in | What it does |
|---|---|---|
| `GET /rigs` | user | The gaming PCs this account can see, and how they are doing. |
| `POST /rigs` | admin | Add a gaming PC. It has to answer as Sunshine. |
| `GET /rigs/find` | admin | Look for PCs running Sunshine on the home network. |
| `PATCH /rigs/{id}` | admin | Change a rig name, address, address away from home or MAC address. |
| `DELETE /rigs/{id}` | admin | Forget a rig, with its library. |
| `POST /rigs/{id}/pair` | admin | Start pairing, and get the PIN to type into Sunshine. |
| `GET /rigs/{id}/pair` | admin | How pairing is going. |
| `DELETE /rigs/{id}/pair` | admin | Stop waiting for a PIN. |
| `POST /rigs/{id}/devices` | admin | Type a PIN into Sunshine for a Moonlight app. Sunshine's sign-in is used once and never kept. |
| `POST /rigs/{id}/sync` | admin | Read a rig's games again. |
| `GET /rigs/{id}/apps` | admin | Everything a rig offers, shown in the library or not. |
| `PUT /rigs/{id}/apps/{app}` | admin | Show or hide an app in the library. |
| `POST /rigs/{id}/wake` | user | Send a registered rig the Wake-on-LAN signal. |
| `POST /rigs/{id}/quit` | user | End what is running on a rig: for admins and whoever started it. |
| `POST /rigs/games/{id}/play` | user | What this device can do to play a rig's game. Starts nothing on the PC. |
| `POST /rigs/games/{id}/start` | user | Start a rig's game on the PC, for a Moonlight app to resume. |
| `POST /rigs/games/{id}/stream` | user | Start a stream of a shared PC's game. |
| `GET /wittyrigs` | admin | WittyRigs: whether it is on, and whether Sunshine is on this machine. |
| `PUT /wittyrigs` | admin | Turn WittyRigs on or off. |

### Photos

| Route | Sign-in | What it does |
|---|---|---|
| `GET /photos/months` | user | Photo months. |
| `GET /photos/list` | user | Photo list. |
| `GET /photos/folders` | user | Photo folders. |
| `GET /photos/map` | user | Photo map. |
| `GET /photos/ai` | user | Photo AI. |
| `GET /photos/albums` | user | Photo albums. |
| `POST /photos/albums` | user | Create album. |
| `PUT /photos/albums/{id}` | user | Update album. |
| `DELETE /photos/albums/{id}` | user | Delete album. |
| `POST /photos/albums/{id}/items` | user | Album add. |
| `DELETE /photos/albums/{id}/items` | user | Album remove. |
| `GET /photos/{id}` | user | Photo detail. |
| `GET /items/{id}/image` | user | Item image. |
| `GET /items/{id}/original` | user | Original. |
| `GET /items/{id}/motion` | user | Motion. |
| `POST /libraries/{id}/upload` | user | Upload. |

### Vertical and creator videos

| Route | Sign-in | What it does |
|---|---|---|
| `GET /creators` | user | Creators. |
| `GET /creators/{id}` | user | Creator. |
| `PUT /creators/{id}` | admin | Update creator. |
| `GET /creators/{id}/art/{kind}` | user | Creator art. |
| `PUT /creators/{id}/subscribe` | user | Subscribe2. |
| `GET /videos` | user | Videos. |
| `GET /videos/feed` | user | Video feed. |
| `GET /videos/tags` | user | Video tags. |
| `GET /videos/{id}` | user | Video. |
| `PUT /videos/{id}` | admin | Edit video. |
| `GET /videos/{id}/related` | user | Video related. |
| `PUT /items/{id}/reaction` | user | Reaction. |
| `POST /items/{id}/view` | user | View. |
| `GET /items/{id}/comments` | user | Comments. |
| `POST /items/{id}/comments` | user | Add comment. |
| `PUT /comments/{cid}` | user | Edit comment. |
| `DELETE /comments/{cid}` | user | Delete comment. |

### Collections and your year

| Route | Sign-in | What it does |
|---|---|---|
| `GET /collections` | user | Collections. |
| `POST /collections` | user | Create collection. |
| `POST /collections/preview` | user | Preview rule. |
| `GET /collections/holding` | user | Collections holding. |
| `POST /collections/watch-later/items` | user | Watch later add. |
| `GET /collections/{id}` | user | Collection. |
| `PUT /collections/{id}` | user | Update collection. |
| `DELETE /collections/{id}` | user | Delete collection. |
| `POST /collections/{id}/items` | user | Collection add. |
| `DELETE /collections/{id}/items` | user | Collection remove. |
| `GET /me/review` | user | Review. |
| `DELETE /me/history` | user | Clear history. |
| `GET /me/recommended` | user | Recommended. |

### Internet radio

| Route | Sign-in | What it does |
|---|---|---|
| `GET /radio` | user | The stations of a library (`?library=`): `radio` on each has its logo-less details (`homepage`, `country`, `language`, `codec`, `bitrate`, `hls`), `status` (ok, trouble, dead, unknown) with a `reason`, and this account's `favorite` and `lastPlayed`. `path` is empty unless you're an administrator. |
| `POST /radio` | admin | Adds a station by stream address. |
| `POST /radio/import` | admin | Adds the stations in an M3U or PLS playlist's text. |
| `POST /radio/check` | admin | Tries every station of a library, one at a time in the background (202). |
| `GET /radio/directory/search` | admin | Searches the station directory: `q` (a name or genre), `tag`, `country` (two letters), `language`, `limit`, `offset`, and `library` to mark what's already there. Answers 403 when the owner has turned the directory off, 502 when it can't be reached. |
| `GET /radio/directory/browse` | admin | The countries, genres and languages the directory can be browsed by. |
| `POST /radio/directory/add` | admin | Adds a station by its directory `id`; the details are the directory's, not the request's. |
| `GET /radio/{id}` | user | One station. |
| `DELETE /radio/{id}` | admin | Removes a station. |
| `GET /radio/{id}/now` | user | What the station says it is playing, from its own stream: `title`, and `artist` and `track` when the line reads that way; `live` says someone is listening through this server. Poll at most every 15 seconds while playing. |
| `POST /radio/{id}/check` | admin | Tries a station now, and answers with its status. |
| `POST /radio/{id}/lookup` | admin | Finds a station added by address in the directory, and fills in its details. An address that may hold a private key is refused, never sent. |
| `GET /radio/{id}/stream` | user, st | The sound, relayed: endless audio, or an HLS playlist when `radio.hls` is true. Get the address from `POST /items/{id}/playback`, which answers `decision: radio`, `protocol: stream` or `hls`, `seek: none`, and `nowPlayingUrl`. |
| `GET /radio/{id}/hls` | user, st | A part of an HLS station. These addresses come from this server's playlists and are signed; don't build them. |

Favourites use `PUT /items/{id}/reaction` with `{"value": 1}`, as for anything else. Station logos are at `/items/{id}/poster`, like any other picture, and may be absent or refused: show generated art when they are.

### Share links

| Route | Sign-in | What it does |
|---|---|---|
| `GET /shares` | user | Shares. |
| `POST /shares` | user | Create share. |
| `DELETE /shares/{id}` | user | Delete share. |
| `GET /share/{token}` | none | Share open. |
| `GET /share/{token}/image/{id}` | none | Share image. |
| `GET /share/{token}/original/{id}` | none | Share original. |
| `GET /share/{token}/play/{id}` | none | Share play. |
| `GET /share/{token}/file/{id}` | none | Share file. |
| `GET /share/{token}/hls/{id}/playlist.m3u8` | none | Share HLS playlist. |
| `GET /share/{token}/hls/{id}/seg/{n}` | none | Share HLS segment. |

### Remote access and signed-in devices

| Route | Sign-in | What it does |
|---|---|---|
| `GET /remote` | admin | Remote. |
| `PUT /remote` | admin | Put remote. |
| `POST /remote/test` | admin | Start test. |
| `POST /remote/selfcheck` | admin | Self check. |
| `POST /remote/router` | admin | Look router. |
| `POST /remote/obtain` | admin | Obtain. |
| `DELETE /remote/lockouts` | admin | Clear lockouts. |
| `GET /sessions` | user | Sessions. |
| `DELETE /sessions` | user | End other sessions. |
| `DELETE /sessions/{id}` | user | End session. |
| `GET /me/2fa` | user | Two factor. |
| `POST /me/2fa/setup` | user | Two factor setup. |
| `POST /me/2fa/confirm` | user | Two factor confirm. |
| `POST /me/2fa/disable` | user | Two factor disable. |

### More server routes

| Route | Sign-in | What it does |
|---|---|---|
| `GET /play/{sid}` | user | Play get. |
| `DELETE /play/{sid}` | user | Play stop. |
| `POST /play/{sid}/webrtc` | user | Play web RTC. |
| `GET /play/{sid}/ws` | user | Play WS. |
| `POST /play/{sid}/action` | user | Play action. |
| `GET /play/{sid}/stats` | user | A running game's numbers: frame rates, encode time, bitrate, drops. |
| `GET /play/{sid}/screenshot` | user | The current picture as a PNG. |
| `GET /gaming` | admin | Gaming. |
| `POST /gaming/cores/{id}/install` | admin | Core install. |
| `DELETE /gaming/cores/{id}` | admin | Core remove. |
| `DELETE /gaming/unpacked` | admin | Empty the unpacked games cache. |
| `GET /gaming/custom` | admin | Custom cores and consoles: whether they're on, what's here, the cores folder and the audit log. |
| `PUT /gaming/custom/enabled` | admin | Turn custom cores on (password) or off. |
| `POST /gaming/custom/staged/upload` | admin | Upload a core and test-load it. Multipart, password field first. |
| `POST /gaming/custom/staged/url` | admin | Fetch a core from a public https address and test-load it. |
| `POST /gaming/custom/staged/placed` | admin | Test-load a file from the cores folder. |
| `POST /gaming/custom/staged/{token}/approve` | admin | Approve a core that passed its test-load. |
| `DELETE /gaming/custom/staged/{token}` | admin | Throw away a core waiting for approval. |
| `PUT /gaming/custom/cores/{id}` | admin | Change a custom core's name, licence, note and consoles. |
| `DELETE /gaming/custom/cores/{id}` | admin | Remove a custom core. Saves are never touched. |
| `POST /gaming/custom/cores/{id}/reapprove` | admin | Approve a changed core file again, after testing it. |
| `POST /gaming/custom/cores/{id}/rollback` | admin | Go back to a kept version. |
| `POST /gaming/custom/cores/{id}/files` | admin | Add an info file, or a zip of assets or BIOS-folder files. |
| `POST /gaming/custom/consoles` | admin | Add a custom console. |
| `PUT /gaming/custom/consoles/{id}` | admin | Change a custom console. |
| `DELETE /gaming/custom/consoles/{id}` | admin | Remove a custom console. |
| `PUT /gaming/consoles/{console}/core` | admin | Choose a console's core. |
| `GET /gaming/games/{id}/core` | admin | What plays a game, and how each core has gone on it. |
| `PUT /gaming/games/{id}/core` | admin | Choose a core for one game. |
| `GET /reach/{nonce}` | none | Reach. |
| `DELETE /users/{id}/2fa` | admin | User two factor reset. |
| `GET /recording-rules` | user | Recording rules. |
| `POST /recording-rules` | user | Add recording rule. |
| `DELETE /recording-rules/{id}` | user | Delete recording rule. |
| `GET /dvd/{sid}/menu/{serial}/effect/{n}` | user | DVD effect. |
| `GET /dvd/{sid}/sound/{n}` | user | DVD sound. |
| `GET /dvd/{sid}/bdj` | user | BDJ poll. |
| `GET /dvd/{sid}/bdj/frame/{n}` | user | BDJ frame. |
| `GET /bdj/status` | admin | BDJ status. |

---
<sub>WittyMediaServer 0.5.0-alpha documentation</sub>
