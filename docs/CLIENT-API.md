# Client API

For developers writing an app or a tool for WittyMediaServer: how to find a server, sign in, browse and play, with every route listed. A person running a server doesn't need this guide.

Everything here is served at `/api/v1` on the server's own address, which is port 9797 unless its owner changed it. A quick look at a server you can reach:

```bash
curl http://192.168.1.20:9797/api/v1/capabilities
curl -X POST http://192.168.1.20:9797/api/v1/login -d '{"name": "Alex", "password": "...", "device": "My tool"}'
curl -H "Authorization: Bearer <token>" http://192.168.1.20:9797/api/v1/home
```

The first needs no sign-in, and says what the server can do. The second answers with a token. The third uses it. An app should sign in [with a code](#with-a-code) rather than a password where it can.

The same contract is in machine-readable form in `openapi.json` (OpenAPI 3.1), beside this guide in the project's `docs` folder. A test in `internal/server` fails when a route is missing from either file, when what they say about sign-in isn't what the server does, or when a schema no longer matches the Go type behind it.

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
- [Live channels](#live-channels)
- [Discs and their menus](#discs-and-their-menus)
- [Audio CDs](#audio-cds)
- [Themes and design data](#themes-and-design-data)
- [Choices that follow the account](#choices-that-follow-the-account)
- [What a TV app must build itself](#what-a-tv-app-must-build-itself)
- [Game details](#game-details)
- [Legal notices](#legal-notices)
- [Remote control and watch parties](#remote-control-and-watch-parties)
- [Background tasks and server stats](#background-tasks-and-server-stats)
- [Adding to the API](#adding-to-the-api)
- [Every route](#every-route)
- [Backup](#backup)

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
comparing server version numbers. Server 0.8.0-alpha is revision 16 (0.7 was 10): revision 15 for
the release's first part and 16 for its second (modules' own SDUI, experiences, the theme on
server-driven pages with each profile's own colours, the editor's merging publish, the artist page
and the fixes listed under revision 16 in `api_capabilities.go`),
each part with its own feature name. `/api/...` without a version is the same thing, for the web
interface that ships with each server. Every API answer carries `X-WMS-API-Version`. A version this
server doesn't have answers 404 with `apiVersions`.

**Which app works with which server.** Every client checks this at the address step, in this order, and
never by comparing server version numbers:

| Check | If it fails | Says |
|---|---|---|
| `apiVersions` has 1 | The app can't talk to it at all | "This server needs a newer version of the app" |
| `apiRevision` is at least what this app needs (each app holds this as a constant) | The server is older than the app | "Update this server" |
| This app's version is at least `minClientVersions[kind]`, for `android`, `apple` or `desktop` | The server has left this app behind | "This server needs a newer version of the app" |
| Something optional: its name is in `features` | Only that part is hidden | Nothing: the app doesn't offer it |

A missing `minClientVersions` or a kind not listed means fine. Within v1 the server should have no reason
to raise these numbers, so they are a way out and not a habit.

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

**Limits.** Routes that need no sign-in take a body of 16 KB, sent within 15 seconds. JSON elsewhere takes 1 MB.
A game session takes 8 WebRTC offers at once and one more a minute, and 4 WebSocket viewers (429 past that). One
address can hold 128 connections, so reuse them, and a response that a client doesn't read for 2 minutes (15 for
a file being played) is ended: ask again with `Range` to carry on.

**Numbers.** Times are seconds as floating point unless a name says otherwise; `...At` values are Unix
seconds. IDs are integers, except session and stream IDs, which are strings to pass back unchanged.

**Lists** that can be long take `limit` and `offset` and answer with `total` (`/items`), so don't fetch
a whole library for a screen. Filtering and sorting always happen on the server.

**Files on the server** are not for apps. An item's `path` is empty for anyone but an administrator, a
podcast episode's and a radio station's included, as their addresses can hold a key. `poster` and
`backdrop` say little. Use the routes for pictures, and `/playback` for media.

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

**If nothing is found:** a firewall may be blocking UDP ports 5353 and 9797 on the server (the Windows
installer opens them if its firewall option is on), a network may keep devices apart ("guest" or "client isolation" Wi-Fi), or
the server may be in a container without host networking. So never make discovery the only way in.

**Finding it again.** Addresses change, so remember `id`. If a saved address stops answering, browse
for the same `id`, then [check its identity](#server-identity) before sending a token.

## Connecting by address

For someone typing an address or a URL. This is the whole check, in order:

1. **Tidy it up.** Trim it. With no scheme, add `http://`. With neither a scheme nor a port, add port
   9797. Remove any path, query and trailing slash.
2. **Ask.** `GET /api/v1/capabilities`, with a timeout of about 5 seconds, sending no token. From the
   internet, `version` is empty until you sign in: go by `apiVersion` and `features`.
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

- The person presses *Get a code* at `/pair` (`POST /pair/invite {userId}`), and sees 6 digits.
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
  show it), `expiresInSec` (600), `intervalSec` (how often to poll, 3), and `approveUrl` (a path with
  the code in it: put the server's address in front for a QR code; the page still asks before doing
  anything). `approvePath` is the same page without the code, to show as text to type.
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
- An administrator signing a device in as themselves must send their own `password` again (and `otp`,
  their two-factor code, when `/pair/info` says `twoFactor: true`) with `/pair/approve`,
  `/auth/pair/{session}/approve` and `/pair/invite`. Without it the answer is 403 with
  `reason: "password-required"`; a wrong one is 403 too. The profile in `profiles` says
  `needsPassword: true`. A session left open on a phone can then sign a device in as a Kids profile at
  most, never as the administrator. The PIN's tries aren't used up by a refused password.
- An owner can turn code-only pairing off (the `pairRequirePin` setting). `POST /pair/start` then
  answers 426 with `reason: "pin-required"` and `use: "/api/v1/auth/pair"`: start a secure pairing
  instead. Apps should try `/auth/pair` first wherever the `pair-totp` feature is listed.
- Nothing is stored: a restart of the server means asking for a new code. The new session
  appears as `device`.

### With a QR code and a PIN (secure pairing)

Servers with the `pair-totp` and `pair-qr` features also offer a stronger version of the
code shown on the device. The device shows a QR code and a **6-digit PIN that changes every 30 seconds**. The
person approving scans the QR code, or opens the page, and types the PIN they see. The PIN proves they are
looking at that screen right now, so a code someone read over a shoulder, or saw in a photo, is useless a minute later.

```
TV                                     server                        phone
│ POST /auth/pair ──────────────────► │
│ ◄── sessionId, deviceSecret, pin,    │
│     pinExpiresAt, qr, approveUrl     │
│ shows the QR code and the PIN        │ ◄── scans qr (or opens approveUrl)
│ GET /auth/pair/{id}/pin ──────────► │     types the PIN shown now
│ ◄── the new pin, every 30 s          │ ◄── POST /auth/pair/{id}/approve {pin, check}
│ GET /auth/pair/{id} ──────────────► │
│ ◄── status: approved, token          │
```

- `POST /auth/pair {device, platform}` (no sign-in) answers `sessionId`, `deviceSecret` (keep it, never show
  it), `pin`, `pinExpiresAt` (Unix seconds), `period` (30), `digits` (6), `qr`, `approveUrl`, `expiresAt`, and,
  as `/pair/start` does, `code`, `approvePath`, `expiresInSec` (300) and `intervalSec`. It shares
  `/pair/start`'s limits on open sessions per address.
- `qr` is `wms://pair?v=1&s=<serverId>&u=<base url>&c=<sessionId>&k=<check>`, with each value URL-escaped.
  `s` is the server's ID from `/capabilities`, so an app can tell whether it's a server it knows. `u` is
  where the device reached the server. `k` is an HMAC the server made over the other fields. An app sends
  it back as `check`, and the server refuses a code that was altered. A phone without the app can use
  `approveUrl` instead, which is an ordinary `http(s)://…/pair?code=…` address. The server doesn't draw
  the QR code: put either string in one, and draw it on the device (never by sending it to a QR
  service: it is a sign-in in progress). The web client does this in the page, from *Sign in with your
  phone* on its sign-in screen, and shows `approveUrl` so that any phone's camera opens the right page.
- `GET /auth/pair/{session}/info?k=<check>` (an adult signed in) is `/pair/info` by session, for an app
  that scanned `qr`: the `device`, `platform` and `ip` to show the person before they approve, and the
  `profiles` they may put on it. A `k` that isn't this session's is 400 `bad-check`. It never answers
  the PIN.
- `GET /auth/pair/{session}/pin`, with the secret in the `X-WMS-Pair-Secret` header, answers the current
  `pin` and `pinExpiresAt`. Ask again then. 404 for an unknown session or the wrong secret; 410 once
  it is no longer waiting.
- `GET /auth/pair/{session}`, with the same header, every `intervalSec`, answers the same way as
  `/pair/poll`. One more 410 `status` is `locked`: too many wrong PINs, so start again.
- `POST /auth/pair/{session}/approve {pin, userId, check, password, otp}` (an adult signed in) approves.
  `userId` and `check` are optional; `password` and `otp` are for an administrator signing the device
  in as themselves (see above). The PIN shown now and the one shown before it are accepted, so someone
  still typing when it changes isn't refused; one not yet shown never is. On the `/pair` page, the device's `code` with `pin` at `POST /pair/approve` does
  the same; `/pair/info` says `needsPin: true` for such a device. The code alone is refused for it.
  Failures:

  | Status | `reason` | Meaning |
  |---|---|---|
  | 400 | `wrong-pin` | Not the PIN shown now or just before, or it was already used. `triesLeft` says how many remain. |
  | 403 | `password-required` | The device would be signed in as an administrator: send `password` (and `otp` when `twoFactor` is true). |
  | 400 | `bad-check` | The QR code's `check` isn't this session's. Counts as a wrong PIN. |
  | 404 | `not-found` | No such session. |
  | 410 | `expired` | Its five minutes are up. |
  | 410 | `used` | Already approved or refused. |
  | 410 | `locked` | Five wrong PINs ended it. |
  | 429 | | Too many wrong codes from this account and address. Wait `Retry-After`. |

- `POST /auth/pair/{session}/deny` refuses the request, and the device is told.
- A PIN is accepted for its own 30 seconds and for one step either side, to allow for clocks that differ.
  Each step's PIN works only once. Five wrong PINs end the session, and the session lasts 5 minutes. All of it is
  under `/api/v1/auth/pair` as well.

### Name and password

`POST /login {name, password, device}` answers `{token, user}`, 401 for wrong details, 429 after
repeated failures. Keep this as the fallback for devices without a phone nearby. On the home network
`GET /profiles` may list a "Who's watching?" screen (`style: "profiles"`); a profile with
`hasPassword: false` signs in with an empty password, only where it is listed. Otherwise
(`style: "classic"`) ask for both. To switch profiles, sign in again.

## Stream tokens

AVPlayer can't reliably add headers to the requests it makes, and a session token in an address is
gives away far too much to put there. A stream token is the short-lived replacement.

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
(for heavily styled ASS). `streaming` and `measuredKbps` are for [robust streaming](#robust-streaming).

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
| `cd` | `file` | A whole audio CD image as one stream: see [Audio CDs](#audio-cds). | Depends on the format |
| `radio` | `stream` or `hls` | An internet radio station. | Little |

The server always tries them in this order. `reasons` are stable names, worth showing in a
"playback info" screen, and `explain` puts the same thing in plain words to show as they are. The names: `container`, `video-codec`, `video-tag`, `video-height`, `video-bitrate`,
`video-depth`, `video-range`, `interlaced`, `audio-codec`, `audio-channels`, `audio-track`, `subtitle`,
`burn-subtitle`, `forced`, `direct-play-off`, `remux-unavailable`, `hls-video-codec`, `no-video`, `split-file`, `video-track`, `flatten-3d`, `remote-limit`, `robust-stream`.

**Files, not just tracks.** Subtitle files beside a video come after the file's own tracks in `subtitles`, and work the
same. A film split across files (`CD1`, `CD2`) is one item, with `split-file` among its reasons. `videos` lists the
streams of a file with several (ask with `video`), and `stereo` says a 3D picture is stored in the frame (ask with
`flatten3d` to see one eye).

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

### Robust streaming

Every video answer has `streaming`, which says how it is sent and what the player should do for that:

```json
"streaming": {
  "mode": "robust",
  "from": "device",
  "segmentSec": 2,
  "measuredKbps": 4000,
  "ladder": [{"height": 480, "kbps": 1400}, {"height": 720, "kbps": 2800}, {"height": 360, "kbps": 800}, {"height": 240, "kbps": 400}],
  "player": {"bufferSec": 60, "maxBufferSec": 120, "retries": 8, "firstRetryMs": 500, "maxRetryMs": 8000,
             "segmentTimeoutMs": 30000, "resumeOnDrop": true, "stepDownOnStall": true}
}
```

`mode` is `standard`, how every stream has always been sent, or `robust`, which favours staying smooth
on a poor connection over the sharpest picture. `from` says who chose it: `default`, `server` (its
setting, `playback.remoteStreaming` in `/capabilities`, which applies to viewers away from home) or
`device`. **Let the person choose** Standard, Robust or the server's choice on the device, and send it as
`streaming` (`standard`, `robust`, or `auto` or nothing to follow the server). A device's choice wins
wherever it is, so robust helps on poor Wi-Fi at home too.

**Measure first.** When the device chose robust, or follows a server whose `remoteStreaming` is `robust`,
time `GET /playback/speedtest` (up to 4 MB of bytes that don't compress, sent within the limits for
viewers away from home) for about a second, counting from the first bytes, and send the result as
`measuredKbps`. Keep it for a few minutes, and keep the player's own estimate when a video ends. Without
one, the server assumes a connection that carries 480p.

**What the server does in robust mode.** A file whose bitrate is no more than half of `measuredKbps` is
still sent as it is (or rewrapped); anything else is converted (reason `robust-stream`) to a ladder of
steady rates, listed in `ladder` in the order of the master playlist in `url`. The first is where it
starts, the best rung that fits in 60% of `measuredKbps`; one rung above it and every rung below follow.
Segments are two seconds long and each starts with a keyframe, aligned across rungs, so a player can
change rung, or pick up after a drop, at any of them. Only the rung being fetched is converted: one the
player has left rests after a few seconds. `DELETE /hls/{streamId}` ends every rung.

**What the player does**, with the numbers in `player`:

- Keep `bufferSec` ahead (`maxBufferSec` at most), and wait up to `segmentTimeoutMs` for a segment to start
  arriving: the server may be starting a conversion for it.
- Retry a failed segment or playlist `retries` times, waiting `firstRetryMs`, then twice that each time, up
  to `maxRetryMs`.
- `stepDownOnStall`: when the picture has stalled for 3 seconds and there's a lower rung, switch to it at
  once and hold the player there for a minute of smooth playing, rather than wait for its bandwidth
  estimate to catch up. Pick a lower rung by `RESOLUTION` or `BANDWIDTH` in the master playlist.
- `resumeOnDrop`: when the connection is lost, keep the position and keep retrying, sooner when the device
  says it's back online. When the stream answers 404, or the stream token has run out (401, 403), or
  the picture has been stalled for 25 seconds with nothing arriving, or the connection has been gone for
  about a minute, call `/playback` again with `startSec` at the position and the same choices. If that
  fails because the connection is still gone, keep asking, waiting longer each time.
- A file sent as it is can't step down: when it stalls for 6 seconds or its connection fails, call
  `/playback` again at the position, with `forceTranscode` after a stall.
- Say what it did, briefly ("Picture lowered to 480p to keep playing", "Reconnected after 12 s, at the
  same place"). `explain.streaming` puts the mode in words for an information screen.

In `standard` mode `player` holds the numbers the web player has always used, and nothing changes.

## What each TV plays

These are the defaults of `GET /playback/profiles`. They come from Apple's *HLS Authoring Specification
for Apple Devices* and the Media3 *Supported formats* tables, and **haven't been tried on a real device
yet**: the server side has been checked with FFmpeg as the client, and the Android client on emulators.
The Apple clients haven't been run. Expect to adjust these as the apps meet real hardware.

| | `appletv` (Apple TV 4K) | `appletv-hd` | `androidtv` | `web` |
|---|---|---|---|---|
| Direct containers | MP4, M4V, MOV | MP4, M4V, MOV | MP4, M4V, MOV, MKV, WebM, TS | MP4, M4V, MOV, WebM |
| Direct video | H.264, HEVC (`hvc1` only) | H.264 | H.264, HEVC, VP9 | H.264, VP8, VP9, AV1 |
| Direct sound | AAC, AC-3, E-AC-3, ALAC, MP3 | AAC, AC-3, E-AC-3, ALAC, MP3 | AAC, MP3, FLAC, Opus, Vorbis | AAC, MP3, Opus, Vorbis, FLAC, PCM |
| Most lines | 2160 | 1080 | 2160 | any |
| HDR | HDR10, HLG, Dolby Vision 5 | none | none (add with capabilities) | none (the web app reports its screen) |
| HLS, copied video | H.264 (TS), HEVC (fMP4) | H.264 (TS) | H.264 (TS), HEVC (fMP4) | H.264 (TS), AV1 and VP9 (fMP4), HEVC when reported |
| HLS, copied sound | AAC, AC-3, E-AC-3 | AAC, AC-3, E-AC-3 | AAC, MP3 | AAC, MP3 |
| Subtitles drawn by the player | `mov_text` in the file; WebVTT renditions | the same | SRT, ASS, WebVTT, `mov_text`, PGS, VobSub, DVB in the file; WebVTT sidecar or rendition | WebVTT sidecar; pictures are burned in |

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

## Live channels

Channels and tuners aren't items, so they don't go through `/playback`. `GET /guide` lists them and their
programmes, with a `key` for each channel.

`GET /tv/{key}/tune?streamToken=1` starts the shared stream and answers what is on (`now`, `next`,
`positionSec`), the `url` of its playlist, and for a player that can't set headers, `streamUrl`: the same
address with a stream token for that channel alone. AVPlayer always needs that one. Add `?viewer=<a random
name>` to the address so the server can tell viewers apart, and send `POST /tv/{key}/leave?viewer=<name>`
when leaving. A 503 means the channel is still starting: wait and ask again. Without `streamToken=1` no ticket
is made, which is what the web player does.

**Keeping to the same moment.** Every segment of a channel's playlist carries `EXT-X-PROGRAM-DATE-TIME`, the moment
on the air it starts at, and the playlist says where players sit behind the newest segment with
`EXT-X-SERVER-CONTROL:HOLD-BACK`. Every player on a channel that keeps to the same point by the server's clock
shows the same moment: for a library channel (`kind: library`, made ahead of the clock) the server's time now,
and for a tuner channel (`kind: tuner`, which arrives as it's broadcast) the server's time less three target
durations. The hold-back starts a player at that point. `GET /time` answers `{"now": <milliseconds>}`: time a few
requests and take the one with the shortest round trip to work out how far this device's clock is off. ExoPlayer
(`LiveConfiguration`) and AVPlayer read the same tags.

On a channel with `countProgress`, report watch progress for `now.itemId` as `positionSec` plus the seconds
since `serverTime`, but only up to `partEnd`, when this stretch of the programme ends; a commercial break may
follow. Ask `tune` again at `partEnd`: during a break `countProgress` is false, and after it `positionSec`
carries on from where the programme stopped.

Everything about a channel's ticket works as in [Stream tokens](#stream-tokens), and every request checks again
that the account may watch that channel. An encrypted broadcast is refused with `security` saying why; this
server never decrypts one.

### Channel sets

A **set** is which channels the guide shows, and in what order. `GET /tv-sets` lists every set this profile can
choose, each with `id`, `kind` (`all`, `favourites`, `custom`, `source`, `group`), `name` and `count` (channels this
profile may watch), and `canShare` for an administrator. Show them in that order. Pass the chosen `id` as
`GET /guide?set=<id>`: a custom set (`set:7`) lists its channels in the order its owner gave, every other set by
number. Remember the choice on the device, per server and account, and fall back to `all` when the guide answers
404 for a set that has gone. Feature `tv-sets`.

A custom set is a person's own (`POST /tv-sets` with `name` and `channels`, the channel keys in order;
`PUT /tv-sets/{id}` to rename, share or replace them; `POST /tv-sets/{id}/channels` with `add` and `remove`;
`DELETE /tv-sets/{id}`; `PUT /tv-sets/order` with `ids` for this account's order). An administrator can make one
`shared`: everyone then sees it, and only an administrator changes it (`editable` says whether the caller may).
Channels a profile may not watch stay in a set and are never shown to that profile.

**Paging and finding channels** (feature `tv-guide-paging`). Ask for a page of rows with `offset` and `limit`
(25, 50, 75 or 100 suit a remote; `limit` is at most 200 with programmes). `q` keeps the channels whose number
starts with it or whose name contains it. `locate=<number or key>` finds a channel in the set: the answer's
`located` is its place (`-1` when it isn't there) and `offset` moves to the start of the page that holds it. For a
channel picker or dialling, `list=1&lite=1` (with `limit` up to 20000) gives only `key`, `kind`, `id`, `number`,
`name`, `group`, `logo`, `brand`, `favourite` and `encrypted` per channel: about a tenth of the full rows.

### Catch-up

A tuner channel whose source keeps what it showed has `catchupDays` in the guide (feature `tv-catchup`). A programme
that has started, within that many days, can be watched from its start: `GET /tv/{key}/catchup?start=<the
programme's start>&end=<its end>&viewer=<name>` answers like `/tune`, with `url` for its own stream and `now` for the
programme. It plays from the start at the speed it aired, and can be paused and rewound within `bufferSec`, like a
live channel. Leave with `POST /tv/{key}/leave?viewer=<name>&start=<the same start>`. A 404 means the programme
isn't in the archive. Offer **Watch from the start** only where `catchupDays` covers the programme's start.

Tuner rows also carry `health` (`ok` or `failing`) from the last check or tune, for showing a channel that has
stopped working. A channel can have backups, the same station from other sources, which the server tries by itself
when it won't open; they don't appear in the guide.

### More info

The Design 2 **More info** screen for a programme in the guide (feature `tv-programme-info`) reads
`GET /tv/{key}/programme?start=<a moment in it, Unix seconds>`. It answers:

- `channel`: `key`, `kind`, `number`, `name`, `logo`, `brand`, `catchupDays`.
- `programme`: `start`, `end`, `kind`, `title`, `sub`, `desc`, `genres`, `rating`, `year`, `season`, `episode`,
  `lengthMin`, `scores` (`value`, `system`), `studio`, `language`, `origLanguage`, `country`, `subtitles`,
  `signLanguage`, `audio`, `hd`, `premiere`, `new`, `repeat`, `keywords`. Each field is there only when the guide or
  the library says it.
- `actions`: which buttons to show: `watch`, `record`, `remind` (and `reminded`), `fromStart`, `watchLater` (and
  `inList`), `open`.
- `library`: `itemId` or `showId` when the title is in the library and the profile may see it. **Open item page**
  and **Watchlist** use these.
- `credits`: the same shape as the people/credits model (`cast`, `guests`, `crew`). It comes from the library's
  credits for that title, or else the guide's own (`source: "guide"`, no `personId`).
- `trailers`: the library item's trailers and clips (`itemId`, `title`, `type`, `durationSec`).
- `schedule`: the title's other airings in the next 14 days on channels the profile may watch, with `this` marking
  the one asked about.
- `ids`: `imdb` and `tmdb`, when the guide gives them, for `GET /discover/where-to-watch`.

A 404 means the channel or the programme isn't there for this profile. A 403 means a library channel is showing
something the profile may not see. In SDUI, the `credits` source takes `programme: "<key>@<start>"` and draws the same
cast and crew.

### Recording

**`canRecord` is the only signal for offering Record.** Show **Record**, **Record every airing** (a series), and any
key or menu item that does either, only where `canRecord` is `true`. Where it is `false`, hide them: don't grey them
out, and don't work it out yourself from `kind`, `encrypted`, `noRecord` or the profile. The server has already, and
every client then offers the same thing.

- **Only media from a tuner can be recorded.** A channel made from the library never has `canRecord: true`: what it
  plays is already in the library.
- A tuner channel has it when it is clear (not encrypted), isn't marked by its device or its owner as not to be
  recorded (`noRecord` says why), and the profile asking may use recordings.

The server always sends `canRecord`, `true` or `false`, wherever it describes a channel:

| Where | Field |
|---|---|
| `GET /guide`, with any of `list=1`, `channel`, `kind`, `group`, `favourites`: the guide, the mini guide, the On now row, the channel list for dialling | `channels[].canRecord`, and `channels[].alternative.canRecord` |
| `GET /tv/{key}/tune`: a live channel's details | `canRecord`, and `alternative.canRecord` |
| A refused tune: 403 for an encrypted channel, 502 for HEVC the server can't decode | `alternative.canRecord` |
| `GET /guide/search` | `[].canRecord`, for the hit's channel |
| `GET /tv/reminders`, `POST /tv/reminders` | `canRecord`, for the reminder's channel |
| `GET /channels`, and for an administrator `POST /channels`, `GET` and `PUT /channels/{id}`, and `POST /channels/{id}/restore` | `canRecord`: always `false` |
| `GET /tuners/{id}/channels`, `PUT /tuner-channels/{id}` | `canRecord` |

For one programme, offer Record when its channel's `canRecord` is `true`, the programme's `kind` is `broadcast`, and
its `end` is still to come.

`POST /recordings` (body: `channel`, `title`, `sub`, `start`, `stop`, and with feature `tv-dvr-rules-2`
`padBefore` and `padAfter`, minutes more either side) and `POST /recording-rules` (body: `channel`, `title`, and with
that feature `anyChannel`, `newOnly`, `keep` and the two paddings, which `PUT /recording-rules/{id}` changes) use the
same rule, so they take what `canRecord` offered and refuse anything else, with `{"error": "…"}` in
words to show as they are:

| Status | When |
|---|---|
| 400 | A library channel: "library channels can't be recorded: what they play is already in your library". Also a `channel` that isn't a channel key, or a programme that has already finished, has no title, or is already being recorded. |
| 403 | The profile may not use recordings ("this profile cannot use recordings"), the channel is encrypted, or it is marked not to be recorded ("{name} can't be recorded: {why}"). |
| 404 | A tuner channel that is turned off, or that this profile can't see. |
| 409 | 50 recordings already waiting, or 20 series rules, for an account that isn't an administrator. |

### Playing in the background

A channel keeps playing, sound and all, when its tab, window or app goes to the background, for as long as
it would in front: people leave it on while they work. Every client offers the same choice, **Keep playing when
this tab is in the background** ("this app" in an app), in WittyTV's settings and in the player's settings.

- **It's the device's own.** Keep it on the device, not with the account: the website keeps it in
  `localStorage` under `wms.tv.backgroundPlay` (`"1"` or `"0"`). A device that hasn't chosen goes by
  `backgroundPlay` (boolean), which `GET /guide` and `GET /tv/{key}/tune` always send: `true` unless the
  owner turned off `tvBackgroundPlay` in `PUT /tv/settings` for new devices.
- **On** (the default): don't stop, pause, mute or leave the channel because the client is in the
  background. The server counts a viewer as gone after **30 seconds** without a request from it, and stops a
  channel **45 seconds** after its last request, so a player that may go quiet in the background (muted,
  paused, or slowed down by the system) asks for its playlist, `GET /tv/{key}/live/playlist.m3u8?viewer=<name>`,
  at least every **15 seconds** while it's there. A browser keeps that timer in a worker, which background
  tabs don't slow down. A pause the system made (a browser pausing a silent video in a background tab) isn't
  the viewer's: play again when the client comes back. A pause the viewer asked for stays.
- **Off:** going to the background leaves the channel at once, `POST /tv/{key}/leave?viewer=<name>`, which
  frees a tuner, and stops the player. Coming back tunes in again with `GET /tv/{key}/tune` and plays at
  what's on now.
- While it's off, the website marks its page `<html data-background-play="off">`, for a desktop app that keeps
  its window from being slowed down in the background (and so always looks visible): seeing that, the app lets
  the page be hidden as a browser would.
- The server never ends a channel because a client is in the background. It only stops one that nobody has
  asked for in 45 seconds, or that its last viewer left.

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

**Keeping it smooth.** Switching clips shouldn't flash black. Keep the last frame on screen until the
next clip has drawn its first one: hold a snapshot (`AVPlayerItemVideoOutput`, or `TextureView.getBitmap`
on Android), and drop it on `isReadyForDisplay` or `onRenderedFirstFrame`. The current clip's `url` is the
same if a new presentation only moved within the same stream: then seek instead of reloading.

**Stops.** `kind: "stop"` has a `reason`; `scrambled` means the image is copy-protected and can't play.
Offer Title menu, Play movie and Back. This server never decrypts discs.

**Java menus.** BD-J (Java) Blu-ray menus run on the server only when its owner has installed Java and
turned on **Run Blu-ray Java menus**: see [Blu-rays](BLURAY.md#bd-j-java-menus). Otherwise such a disc plays its
feature directly. When they run, `GET /dvd/{sid}/bdj` waits for the disc's applications to change something,
and `GET /dvd/{sid}/bdj/frame/{n}` serves their graphics.

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

**The resolved theme.** With `theme-engine` in `features`, `GET /theme` is the one answer to "what
colours does this account see": the server's default theme with the owner's colour overrides, or the
theme the account chose for itself (`theme` in `/me/ui`, which replaces the overrides too), and under
`modules` the colours for the parts of the interface that belong to a module (a games shelf in the
games module's accent). Token names are those in `themes/*/theme.json` `colors` (`bg`, `bg2`, `fg`,
`accent`, `accentFg`, …), colours are `#RRGGBB` or `#RRGGBBAA`. Keep the `ETag` and send it back as
`If-None-Match`; a 304 means nothing changed. `warnings` lists text colours an override made hard to
read (below WCAG AA, 4.5:1), for the theme settings page to show; apps can ignore it.

`scheme` is `dark` or `light`, or `auto` when the theme follows the device. `variants` holds each look
resolved the same way (`{id, tokens, modules}` by scheme): one for an ordinary theme, `dark` and `light`
for the automatic one, where `tokens` and `modules` at the top are the dark look's. A client that
follows the device's setting takes its colours from `variants`. A warning about one look of the
automatic theme says which in `variant`.

```json
{
  "id": "midnight", "source": "server", "revision": 4,
  "tokens": { "bg": "#0B1120", "fg": "#E6ECF7", "accent": "#FF8800", "accentFg": "#FFFFFF" },
  "overrides": { "accent": "#FF8800" },
  "modules": { "wittyplay": { "tokens": { "bg": "#0B1120", "accent": "#3AD29F" }, "overrides": { "accent": "#3AD29F" } } },
  "warnings": [],
  "etag": "W/\"theme-…\""
}
```

**Layers, shape, type and the profile's own colours.** The order, lowest first, is the theme, the
administrator's global colours, a module scope's own, the administrator's for the scope, the
profile's own (`personal`), and the profile's own for the scope (`modules.<scope>.personal`); each
layer brings the colours worked out from it (`theme-derived`: an accent alone also sets `accentHover`,
`accentFg`, `accentText`, `focus`, `brandAccent`, `accentSoft` and `accentLine`). With
`theme-shape-type`, the answer and each of `variants` have `shape` (corner radii and the focus ring, in
points: `card`, `button`, `field`, `row`, `thumb`, `chip`, `badge`, `image`, `sheet`, `dialog`, `pill`,
`focusWidth`, `focusOffset`, `focusScale`) and `type` (`display`, `body` and `mono` families, best
first, and `weights`), and `brand` is the server's name (`parts`, as `brandParts`) with `color`, the
`brandAccent` token resolved. With `theme-personal`, `GET`, `PUT` and `DELETE /theme/personal` read
and set the signed-in profile's own colours: `{"tokens": {...}, "modules": {"<scope>": {...}}}`. A
`PUT` that would put text under 4.5:1 on the theme the profile sees is refused (400) with the pairs in
`contrast`. With `sdui-tokens`, every component and card style in `GET /sdui/catalog` has `tokens`:
which colour token each part is drawn in, which text role each text takes, which `shape` token rounds
each part ([Themes](THEMES.md#sdui-tokens)).

`GET /themes` lists the themes, with four preview colours and `scheme` (`dark` or `light`), and which
is the server's `default`. `GET /themes/{id}/tokens.json` is a theme as data, read from its CSS
variables, so theme authors write one file and the apps get the same colours:

```json
{
  "id": "default", "name": "Witty", "scheme": "dark",
  "colors": { "bg": "#0F1115", "bg-2": "#171A21", "fg": "#E8EAF0", "accent": "#7955FF", "overlay": "#00000099" },
  "lengths": { "radius": 10, "sidebar": 240 },
  "fonts": ["Inter", "Segoe UI Variable Text", "system-ui", "-apple-system", "Segoe UI", "Roboto", "Helvetica", "Arial", "sans-serif"],
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
- How the interface is laid out and what the look is meant to be: see [Design standards](DESIGN.md). This
  guide only covers the data.
- `GET /branding/logo` and `brandName` in `/status` replace the name and icon, where the owner set them.

### Drawing the server's name

The name is drawn with every other word in the theme colour, the way WittyMediaServer is drawn as
Witty**Media**Server. `/status` has it ready to draw:

```json
"brandName": "Willow's Movies",
"brandColouring": "auto",
"brandParts": [{ "text": "Willow's " }, { "text": "Movies", "accent": true }]
```

- **Draw `brandParts`** in order, in one line of text, each in its own `color`: `{kind: "fg"}` in the
  theme's text colour, `accent` in `brandAccent` (they change with the theme), `token` in the theme colour
  `token` names, `static` in the fixed `hex` (the same in every theme) and `gradient` across `stops` (two to
  four fixed colours) at `angle` degrees; `bold` parts in the heavy weight. `accent: true` is the same as the
  accent kind. It is there for the default name too
  (`brandName` is then `""`), so an app needs no rule of its own. Joined, the parts are the name.
- **Screen readers get the name once, whole**: give the text one accessibility label, `brandName` (or
  WittyMediaServer when it's empty), and keep the coloured pieces out of the accessibility tree
  (`accessibilityElement` with a label on Apple, `contentDescription` on the view holding the spans on
  Android).
- **Long names wrap** to a second or third line in a side menu rather than being cut off; a top bar
  shortens it with an ellipsis. The parts can break anywhere a normal line can.

The theme engine's `brand` (`GET /theme`) carries the parts with their colours, the **mark** for small
spaces and `warnings`: `mark` is `{kind: letter|initials|text|image, text?, imageUrl?, shown}`, a square badge
with `shown` centred in it (or the logo at `imageUrl`, fitted inside; the letter when it can't load), drawn
in the rail's header, the tab bar's and the sign-in; `warnings` list the parts in a fixed colour that fall
under 4.5:1 on a surface of this theme. The owner's colouring is `brandColouring` (the quick modes below) or
`brandSpans`, the editor's `[{from, to, color, bold}]` over code points (at most 40, in order, not
overlapping), which wins when set; the mark is `brandMark`, both in `PUT /settings`.

`brandColouring` is how the owner chose it, kept with the name in `PUT /settings`:

| Value | Meaning |
|---|---|
| `auto` (or empty) | Every other word, starting with the second. Words split at spaces, and inside joined words at a capital after a small letter (MyHome), before the last capital of a run followed by two small letters (TVRoom, but TVs stays whole), between a small letter and a digit (Movies4K), and before a capital after a dash, `_`, `/`, `&`, `+`, `\|` or `:`. Scripts without capital letters split at spaces only. |
| `one` | The whole name in one colour. |
| `pick:5-10,16-22` | The ranges in the theme colour, as start-end offsets in characters (Unicode code points, not UTF-16 units), the end not included, in order and not overlapping, up to 80. `pick:` alone colours nothing. |

An app that renames the server sends both `brandName` and `brandColouring`. If it sends only a new
name, chosen ranges no longer fit it, so the server goes back to `auto`. Anything else is refused with
400. An app showing a rename window of its own can offer the same three choices; the rule and its cases
are in `internal/brandname` (`testdata/cases.json`).

## First run

First run is fourteen steps in a web browser, and only there: `GET /setup/state` says where it is
(`complete`, `steps`, `step`, `done`, `url`). Until it is complete the server answers **every route an app
uses** with **503** and

```json
{ "error": "setup_required", "message": "This server isn't set up yet. Finish setup in a web browser.", "setupURL": "http://192.168.1.20:9797/setup" }
```

and a `Retry-After`. Only `/status`, `/health`, the setup routes (`/setup`, `/setup/state`,
`/setup/complete`), the themes and `/sdui/experiences`, `/sdui/preview`, a restore of a backup and the web
app's own files answer; the administrator's browser session also carries on for the setup's own calls.
An app shows "This server isn't set up yet" with `setupURL` and a QR code of it, offers Check again and
Choose another server, and never tries to sign in. `GET /sdui/preview?experience=<id>&theme=<id>` answers a
sample Home and menu from generated demo data with the resolved theme before anyone has signed in, so the
chooser draws every look with the client's own renderer.

## Choices that follow the account

`GET` and `PUT /me/ui` hold one JSON object of up to 16 KB for the account, so choices follow the person
to every device. `PUT` replaces all of it, so read first and keep keys you don't know. Known keys:

| Key | Who uses it | Shape |
|---|---|---|
| `home` | The home page | `{v: 2, order: [widget id...], hidden: [widget id...]}`. Widget ids are hub keys, plus `hero`, `libraries`, `jump` (the three continue rows as one) and `shuffle`. |
| `autoSkip` | The video player | `{intro, recap, credits, adverts}`, each `true` to skip that part without a press; anything else, or no key, is off. The rules the web player follows are in [Playing](PLAYING.md#skipping-without-pressing-anything). |

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

**The server already decides these**, so the apps don't have to: the resume rule, how a disc starts and its
insert effect, default track choice, language names, and the direct/remux/transcode decision. The web
interface reads the first two from the server too.

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

## People

Gate on the feature `credits` (and `sdui-credits` for the SDUI side). A title's people come in three
parts: `GET /items/{id}/credits` for a film or an episode, `GET /shows/{id}/credits` for a show (with
`season=<n>` for a season's page) answer `{cast, guests, crew}`:

- `cast` and `guests`: `{personId, name, hasPhoto, role, character, order, episodes, source}`, in billing
  order (a season's guests: most episodes first, `episodes` saying how many). `character` may hold two
  parts joined by " / ".
- `crew`: one entry per department that has someone, in this order: `directing`, `writing`,
  `production`, `camera`, `lighting`, `editing`, `sound`, `art`, `costume`, `effects`, `crew`; each
  `{department, label, people}`, where each person has a `job` (two joined by ", ").
- An episode's `cast` is its show's regulars; its `guests` and `crew` are its own (the show's crew for
  a department the episode names nobody in).

A person's picture is `GET /people/{personId}/photo?w=<pixels>` when `hasPhoto` is true; otherwise draw
their initials. Open a person with `GET /people/{id}`: `person` (`name`, `sortName`, `birthday` and
`deathday` as `YYYY-MM-DD`, `YYYY-MM` or `YYYY`, `birthplace`, `biography`, `knownFor`: `acting` or a
department, `ids`) and `filmography.groups`: `{key, label, titles}` with `acting` first, then each
department; a title is `{target, id, title, year, character, job, episodes, guest}`, `target` `item`
(open `/items/{id}`, poster `/items/{id}/poster`) or `show` (`/shows/{id}`, `/shows/{id}/art/poster`).
`GET /people?search=` finds people by name. A library filtered to one person is `GET /browse?person=<id>`.

Everything follows the profile's limits: a hidden title's credits answer 404, a person's filmography
names only titles the profile may see, and a person credited on nothing it may see answers 404 and
isn't found, so an app needs no checks of its own. The first time a person is opened the server may
look them up at TMDB for a few seconds; show what arrives.

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

## Remote control and watch parties

Two things that make several clients act as one, both through the server and both optional for an app:

- **Remote control.** Any signed-in client can announce itself as a device, and any other can play something on it, pause it, seek, change its volume or tracks, press its arrow keys, or hand over what it's playing. [REMOTE-PROTOCOL.md](REMOTE-PROTOCOL.md) is the whole wire format, with every message. A server that has it lists `remote-protocol` in `features`, and `GET /remote-control` says whether its owner has it on.
- **Peer-to-peer connections.** An app away from home reaches a server with nothing set up on its router: both are libp2p nodes, the app dials the server's connection ID, and this same HTTP API rides inside. [P2P.md](P2P.md) has the protocol, the invite link and the ready-made connector (a Go package with a desktop helper and mobile bindings). The feature name is `p2p`; `GET /p2p` says whether the owner has it on.
- **Watch parties.** People in different places watch the same thing in step. The server keeps one state for the room and every player follows it. [SYNCPLAY.md](SYNCPLAY.md#for-developers) has the routes, the clock arithmetic and the drift rules. The feature name is `syncplay`.

**Profiles with limits.** Both follow parental controls ([PROFILES.md](PROFILES.md#limits-everywhere-else)), on the server. What an app has to do:

- Ask `GET /remote-control` and `GET /syncplay`, which are never refused, and go by `canControl`, `canCreate` and `canJoin` as well as `enabled`. A profile can be kept from either (`features.remoteControl`, `features.syncplay` in `GET /me/limits`); the other routes then answer 403 with `code: "feature-off"`. A profile kept from controlling still announces its own device.
- Treat 403 with code `screen-time` like the same refusal from a playback route: show the time-up notice. `screen-time-there` means it's the other device's profile whose time is up: say so.
- In a watch party, `timeUp: true` in the room means stop the player and show the notice; the seat is kept.
- When a stream ends with `reason: "signed-out"` or `"feature-off"`, don't reconnect. After switching profile, start again with the new sign-in: announce the device again, join the party again.

A server that does all of this lists `parental-feature-switches` and `session-end-streams` in `features`.

Both use server-sent events for what the server pushes and ordinary requests for the rest, like the activity stream below. Neither carries media: a client told to play something asks for it through [`POST /items/{id}/playback`](#playing-something) with its own sign-in, so every limit there applies as usual. Both refuse a stream token.

## Background tasks and server stats

What the server is doing in the background: library scans, metadata, module installs, transcodes and more, each with its
progress, speed, time left and errors. `GET /system/tasks` gives the state of things and `GET /system/tasks/stream` keeps it
up to date, with no polling. [TASKS.md](TASKS.md#the-api) has the fields, the stream's format and who sees what. The server
decides what each account may see: follow `access` in the answer. If `access.visible` is false, show nothing. If
`access.readOnly` is true, show no controls.

The server's statistics are in [STATS.md](STATS.md): the processor, memory, disks, network, streams, libraries and more, with
history for charts. `GET /system/metrics` says what this server can show and what this account may see, and a section a system
can't provide is left out rather than sent as zero. Keep `GET /system/metrics/stream` open only while it's on screen, as that's
what keeps the sampling going.

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

Paths are relative to `/api/v1`. Full request and response shapes are in `openapi.json`.
"Sign-in" is what the route needs: none, user, or admin; `st` means a
[stream token](#stream-tokens) for the item works too.

### Server

| Route | Sign-in | What it does |
|---|---|---|
| `GET /capabilities` | none | What this server is and can do: API versions, features, playback profiles and shared rules. The first thing a client asks. |
| `POST /identity` | none | Prove this is the server an app pinned: signs a fresh challenge with the server's key. See "Server identity". |
| `GET /status` | none | Server name, version, branding, default theme, and the signed-in user if any. |
| `GET /health` | none | Liveness check. |
| `GET /about` | user | Server version and the modules in use. |
| `GET /licenses` | user | Everything in this server someone else made, with its license: `{groups, components}`. |
| `GET /licenses/{id}` | user | One component's full license text: `{text}`. |
| `GET /legal/notices` | none | The short legal notices, by kind, and what this server's owner wrote. See "Legal notices". |
| `GET /legal` | none | The full Legal page, with the privacy list and what this server's owner wrote. |
| `POST /legal/acknowledge` | admin | Record that an administrator has seen the first-run notice, so `legalAck` in `/status` turns true. |
| `GET /prefs` | user | Server-wide defaults every viewer needs: playback, and `navDefault`, the owner's arrangement of the main menu as JSON. |
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
| `PUT /logs` | admin | Change each category's level (`levels`), each module's level and advanced options (`modules`), or the folder, or record everything for a while (`recordMinutes`), or keep the last 10 minutes in memory (`flight`) and Go's execution trace of the last seconds (`trace`). Takes effect at once, with no restart. |
| `GET /logs/files/{name}` | admin | Download one log file. |
| `POST /logs/open` | admin | Open the log folder on the server's own screen. |
| `POST /logs/client` | user | Report errors from a client: `{app, version, device, events: [{level, kind, message, stack, page, session, request, count, time}]}`. `session` is the stream, disc or game session in play, and `request` the `X-Request-Id` of a failed request. 64 KB and 25 events a request, 60 events a minute per account; 429 past that. 204 when stored, or when the owner turned app errors off. |
| `GET /logs/entries` | admin | The log viewer: `level`, `cat`, `session`, `module`, `q`, `after` (a seq, for a live tail), `before` (a time, for going back), `limit`, `history`. |
| `GET /logs/summary` | admin | Version, modules, system, GPU and encoders, disk space and settings that matter. No secrets. |
| `GET /logs/crashes` | admin | Crash reports, newest first. |
| `GET /logs/crashes/{id}` | admin | One crash report's text. |
| `DELETE /logs/crashes/{id}` | admin | Delete a crash report. |
| `POST /logs/crashes/{id}/seen` | admin | Stop pointing this report out. |
| `GET /logs/crashes/{id}/{part}` | admin | What was kept beside a report, listed in its `parts`: `flight.log` (the last 10 minutes at the most detailed level), `goroutines.txt` (every goroutine's stack) or `trace` (an execution trace for `go tool trace`). |
| `POST /logs/save` | admin | Save what just happened: `{note}` is optional. Writes a report of kind `saved` with those three parts beside it, and returns it. |
| `GET /logs/bundle` | admin | The logs as a zip, made on request: `days`, and `sanitize`, on unless it's `0`, which replaces names, addresses, titles and paths with placeholders (`redact` is its earlier name). Nothing is uploaded anywhere. |
| `GET /logs/bundle/preview` | admin | What that zip would hold, with the same `days` and `sanitize`: `{parts, total, sample, sanitize, days, scanned, replaced, changed}`. `replaced` counts the places changed by kind, in the newest lines; `changed` is a few lines before and after. |
| `GET /diagnose/targets` | admin | WittyDiagnoser: the libraries, data folders and known discs that can be checked. |
| `GET /diagnose/browse` | admin | Folders and disc images inside the libraries and data folders only. |
| `POST /diagnose/disc` | admin | Start checking a disc: `{path, crawl, damage}`. 202 with the job; 429 when too many wait. Read-only. |
| `POST /diagnose/storage` | admin | Start checking folders: `{targets, readTest, writeTest, walk}`. 202 with the job. Target IDs only, never paths. |
| `GET /diagnose/jobs` | admin | Checks running or waiting, then saved reports. |
| `GET /diagnose/jobs/{id}` | admin | One check, with progress and findings. |
| `DELETE /diagnose/jobs/{id}` | admin | Cancel a check. |
| `GET /diagnose/jobs/{id}/report` | admin | The report as a file: `format=json` or `txt`, `redact=1` hides the disc name and file names. Nothing copyrighted is in it. |
| `GET /reset` | admin | What a reset would do here: the data folder, folders set elsewhere, why it can't be done, copies kept, one waiting, how the last went. |
| `POST /reset` | admin | Back to a new install: `{password, code, confirm: "RESET", delete}`. 202, then the server restarts and does it. Keeps a copy unless `delete`. |
| `POST /reset/restore` | admin | Bring a kept copy back: `{password, code, copy}`. 202, then the server restarts. What's there now becomes another copy. |
| `DELETE /reset/request` | admin | Take back a reset waiting for the next start. |
| `POST /reset/copies/delete` | admin | Delete a kept copy for good: `{password, code, copy}`. |
| `GET /repair` | admin | [Repair](REPAIR.md): what's running, the last report with its issues, what fixes did, and the safety copies kept. |
| `POST /repair/check` | admin | Look, and change nothing: `{only}`, a list of checks or areas, or nothing for all. 202, then ask `GET /repair`. 409 if one is running. |
| `POST /repair/fix` | admin | Fix from the last report: `{keys, allSafe, option, password, code}`. A fix that isn't `safe`, or any `option`, needs the password and any second step again. 202, then ask `GET /repair`. |
| `POST /repair/remap` | admin | A library's folder moved: `{from, to}` says what would change, and adds `{apply: true, password, code}` to do it. |
| `POST /repair/restart` | admin | Restart, for a fix that waits for the next start. 409 if none is waiting. |
| `GET /network` | admin | The port: `{port, host, from, settingsFile, saveFile, locked, docker, next, canRestart, problems, addresses, discoveryPort, httpsPort}`. `from` is `flag`, `env`, `config`, `settings` or `default`; `locked` says where the port is set when this can't change it; `next` is the port after a restart. See [The port](CONFIG.md#the-port). |
| `POST /network/check` | admin | `{port}`: whether the server could move there, `{port, ok, current, problems, warnings}`. In range, not a port browsers refuse, not Remote Access's HTTPS port, and free here, tried by listening for a moment. Saves nothing. |
| `PUT /network` | admin | `{port}`: save it in settings.ini for the next start, after the same checks; 400 with the first problem. 409 when `-listen`, `WMS_LISTEN`, `wms.json` or Docker set the port. Answers as `GET /network`. |
| `POST /network/restart` | admin | Restart, on the saved port. 202, then nothing answers until it's back, at the new port. 409 when it can't restart itself. |
| `GET /update` | admin | What is installed and what is available (404 in builds that can't update themselves). |
| `POST /update/check` | admin | Look for a new version now. |
| `POST /update/apply` | admin | Download, verify and install the newest release, then restart. |
| `POST /update/installer` | admin | Download the installer for this system, checked against the release's signed checksums, for a copy that can't replace its own files. |
| `GET /update/installer` | admin | The checked installer, as a file to save. |
| `GET /modules` | admin | Every module: version in use, supported range, status, what is available, what it needs, whether it is installed, and every kind of library and what it takes (404 in builds that can't update them). |
| `POST /modules/check` | admin | Look for module updates now. Nothing else here goes online. |
| `GET /modules/{id}/versions` | admin | Every version known, explained, without going online: release notes as plain text, whether each can be chosen, and why not. |
| `POST /modules/{id}/install` | admin | Download, verify and install a version, with no restart: the newest the last check found, or `{"version": "0.2.0", "confirm": true}`. A choice with a warning (older, withdrawn as broken, taken out of use before) is answered 409 with `"code": "confirm"` and the `warning` until the body says confirm. A version older than the one in use also takes `password`, and `code` with two-factor sign-in, and is answered 403 with `"code": "password-needed"` without them. A version the server would refuse for a judgement rather than an impossibility answers 409 `"code": "needs-override"` with `needsOverride` (`kinds`, `reasons`, `severity`, `warning`, `permissions`, `pending`), and installs with `override: {accept, typed, grants}` and the password (a go-ahead that doesn't fit is 400 `bad-override`); one whose need shows only once downloaded is held, and the module's `pending` says so (feature `module-overrides`; [Installing past a rule](MODULE-STORE.md#installing-past-a-rule)). 202; follow it with `GET /modules`. |
| `POST /module-uploads` | admin | Install a module from a file: multipart `archive` (a release's .tar.gz or .zip, at most 512 MiB), with `sums`, `sig` and `key` when it came with them. Answers what it is and what installing it needs: `{token, expiresAt, id, name, version, delivery, api, releaseStatus, statusNote, signed, trusted, fingerprint, permissions, problems: [{kind, text}], needsOverride: {kinds, reasons, severity, warning, permissions}, refusal, known}`. A `refusal` can't be overridden, and nothing is kept. Feature `module-upload`. |
| `POST /module-uploads/{token}/install` | admin | Install a held upload: `{password, code, override: {accept: [kinds], typed, grants: [permissions]}}`. 400 `bad-override` (with `needsOverride`) when the go-ahead doesn't name exactly what's needed or the name isn't typed, 403 `password-needed` without the password, 404 once it has gone. 202; follow it with `GET /modules`. |
| `DELETE /module-uploads/{token}` | admin | Forget a held upload. 204. |
| `DELETE /modules/{id}/pending` | admin | Forget a download held for a go-ahead (a module's `pending`). |
| `DELETE /modules/{id}/override` | admin | Take an override back, with `password` (and `code`): the module goes back to the version before when that needed none, or off this server; a module this server doesn't know goes altogether. |
| `GET /modules/audit` | admin | What was done to which modules, newest first: `[{at, user, action, module, detail}]`; `limit` up to 1000. Feature `module-audit`. |
| `POST /modules/{id}/rollback` | admin | Stop using this version, and go back to the previous one. Takes `{"password": "...", "code": "..."}`, and is answered 403 with `"code": "password-needed"` without them. |
| `POST /modules/{id}/add` | admin | Install the newest version that fits, and what it's built on. Answers 202, and progress shows in `phase`, `done` and `total`. |
| `POST /modules/{id}/enable` | admin | Turn it back on, with no restart. |
| `POST /modules/{id}/disable` | admin | Turn it off, with no restart. Refused, with names, while libraries need it. |
| `GET /modules/{id}/icon` | user | Its picture, kept by the server and checked against the signed catalogue. |
| `GET /module-store/{id}/{key}` | user | A little JSON a module's page kept for this person, or null. |
| `PUT /module-store/{id}/{key}` | user | Keeps it: at most 64 KB a key and 32 keys a module. |
| `GET /modules/{id}/notices` | user | The licenses of the libraries in its programs, as plain text. |
| `POST /modules/{id}/pin` | admin | Keep the version in use; automatic updates leave it alone. A version withdrawn for security reasons goes out of use anyway. |
| `DELETE /modules/{id}/pin` | admin | Let automatic updates move it again. |
| `PUT /modules/{id}/autoupdate` | admin | Whether this module may update by itself while the server's setting is on: `{"on": false}`. It still looks for updates and offers them. Not for modules that update with the server. |
| `POST /modules/{id}/latest` | admin | End a pin and move to the newest version that fits. 202. |
| `POST /modules/{id}/remove` | admin | Take a module off this server. Answered 409 with `"code": "module-in-use"` and the `libraries` that need it. No data is touched. |
| `PUT /modules/installed` | admin | Make the set of modules on this server what someone chose: `{"installed": ["wittyvideo"]}`, as first-run setup does. Refused whole if libraries need something left out. |
| `PUT /modules/{id}/source` | admin | Take this module's releases from a fork: `{"repo": "someone/WittyDVD", "key": "<public key>", "confirm": true, "password": "...", "code": "..."}`. Answered 409 with `"code": "confirm"` and the security warning until confirmed. Show that warning; don't replace it. 403 where forks are refused. See [Using someone's fork](MODULE-STORE.md#using-someones-fork). |
| `DELETE /modules/{id}/source` | admin | Go back to the module's own repository. |
| `GET /modules/libraries` | user | Every kind of library a module provides, whether it can be made here, and what it takes. A kind not listed is always available. |
| `PUT /modules/libraries/{type}/label` | admin | What this server calls a kind of library, for when two read alike or two modules share one: `{"label": "Blu-rays and DVDs", "taken": {"movies": "Movies"}}`. An empty label goes back to the usual name. `taken` is what the app shows for the other kinds, and a name already in use is refused. |
| `GET /modules/web` | user | The web bundles in use. |
| `GET /modules/{id}/web/{version}/{path}` | user | A file of the bundle in use. Only names in its signed list exist. |
| `POST /modules/{id}/failed` | user | A browser saying a bundle wouldn't load or crashed: `{"version": "...", "reason": "..."}`. Three within a day, at most one a minute, from an administrator's browser take that version out of use, and anyone else's goes in the module's log. 204. |

`POST /libraries` answers 409 with `"code": "module-missing"` and the `modules` that would make it possible when a kind of library's module isn't installed.

A folder on another machine (`\\nas\media`), named from anywhere but the server's own desktop, is refused by `POST /libraries`, `PUT /libraries/{id}` and `GET /fs` with `"code": "other-machine"`, unless the owner set `allowNetworkPaths`. A client should lay out the ways forward: add it on the server's own desktop, use a drive letter or mount on the server, or that setting.

`GET /status` says `onboarding` while first-time setup is under way, for an administrator. A client that takes someone through setup itself sets `setupDone` to `"0"` once the account exists and `"1"` at the end. A server that has never said either, and has libraries, doesn't count as being in setup.

#### The module store

What the store's pages use. Everything a catalogue says is plain text, to be shown as text. Whatever goes wrong comes with `failure`: `{kind, title, message, try, detail, repair}`, the same words on every client. See [Modules and the store](MODULE-STORE.md).

| Route | Who | What |
|---|---|---|
| `GET /modulestore` | admin | Every module from every store that's on, with what this server makes of it (`kind`, `fit`, `status`, `failure`), what kind of module it is (`moduleKind`: `helper`, `web`, `program`, `library`, or `builtin`), how finished the offered version says it is (`releaseStatus`, `statusNote`), the SDUI it adds (`ui`: `{pages: [{id, title}], navigation, settings, sources, inserts}`) and any `override` it was installed with; `moduleKind=web,helper` keeps only those kinds (feature `module-store-kinds`). The stores, the compatibility `policy`, how many `updates` Update All would make, and the current or last `run`. |
| `POST /modulestore/check` | admin | Look at every store now. Answers with the same as `GET /modulestore`. |
| `GET /modulestore/item/{store}/{id}` | admin | A module's page: `item`, `versions`, `plan` (what installing it would bring), `permissions` in words, `builtOn`, and for a store's version `forkWarning` and `fingerprint`. `{store}` is `official` for this project's. |
| `POST /modulestore/install` | admin | Install a module and what it's built on: `{"id": "...", "confirm": true}`. If it would bring anything else it answers 409 with `"code": "plan"` and the `plan` until confirmed. 202, with the run. |
| `POST /modulestore/update-all` | admin | Update everything with an update, what others are built on first. One failure doesn't stop the rest. 202, with the run. |
| `GET /modulestore/run` | admin | `{"run": ...}`: the run going on, or the summary of the last, kept across a restart. Each item has `state` (`queued`, `working`, `done`, `failed`, `skipped`, `undone`), and `failure` if it failed. |
| `POST /modulestore/run/undo` | admin | Take the last Update All back. `{"password": "...", "code": "..."}`: your password, and a code with two-factor sign-in. |
| `DELETE /modulestore/run` | admin | Put the last run's summary away. |
| `GET /modulestore/modules/{id}/log` | admin | What this module has logged lately: `lines`, and `text` to copy. |
| `GET /modulestore/media/{store}/{sha}` | admin | A picture a catalogue lists, fetched, checked against its hash and served from here. Only those a catalogue lists, and only PNG or JPEG. Send the token as a header: this route doesn't take one in its address. |
| `POST /modulestore/stores/preview` | admin | Fetch a store and say what adding it would mean: `{"url": "...", "key": "optional", "private": false}`. Answers with its `name`, `fingerprint`, what it offers and the `warning`. Show that warning; don't replace it. Nothing is kept. |
| `POST /modulestore/stores` | admin | Add a store, pinning the key: `{"url": "...", "key": "...", "private": false, "agreed": true, "password": "...", "code": "..."}`: your password, and a code with two-factor sign-in. |
| `DELETE /modulestore/stores/{id}` | admin | Forget a store. Anything using it goes back to the official version. |
| `PUT /modulestore/stores/{id}/enabled` | admin | Turn a store on or off: `{"on": false}`. |
| `POST /modulestore/stores/{id}/check` | admin | Look at one store now. Answers with how it stands, including a `problem`. |
| `POST /modulestore/stores/{id}/fork/{module}` | admin | Use a store's version of a module this server knows: `{"confirm": true, "password": "...", "code": "..."}`. Answered 409 with `"code": "confirm"` and the warning until confirmed, and confirming asks for your password, and a code with two-factor sign-in. |

### Sign-in and accounts

| Route | Sign-in | What it does |
|---|---|---|
| `POST /setup` | none | Create the first administrator (409 once there is an account). |
| `POST /login` | none | Sign in. 429 with Retry-After after repeated failures. Password-free Kids and Guest profiles only sign in where `/profiles` lists them. |
| `POST /logout` | none | End the session the request carries. |
| `GET /profiles` | none | The "Who's watching?" list where the server offers it: `style` is `profiles` (with `profiles`) or `classic` (ask for name and password). |
| `POST /pair/invite` | user | Code typed on the device: get 6 digits to type there, for this account or a Kids or Guest profile it manages. Adults only. One open at a time per person; lasts 5 minutes. |
| `POST /pair/redeem` | none | Code typed on the device: use the 6 digits and get a sign-in. Works once. Failures say why in `reason`: `wrong-code` (400), `expired` or `used` (410), `too-many-tries` (429). |
| `POST /pair/start` | none | TV sign-in: get a code to show. Rate limited per address (429). 426 `pin-required` when the owner allows only secure pairing. |
| `POST /pair/poll` | none | TV sign-in: has someone answered? `status` is `pending`; `approved` with `token` and `user` (once only); 403 `denied`; 410 `expired`; 429 `slow-down`. |
| `POST /pair/info` | user | TV sign-in, on the phone or computer: what is asking, and which profiles this person may put on it. Adults only. Wrong codes are limited. |
| `POST /pair/approve` | user | TV sign-in: approve, as this account or a Kids or Guest profile it manages. Works once. As an administrator: with `password` (and `otp`). |
| `POST /pair/deny` | user | TV sign-in: refuse; the TV is told. |
| `POST /auth/pair` | none | Secure pairing: start a session; answers a rotating `pin`, a `qr` string and `approveUrl`. See [With a QR code and a PIN](#with-a-qr-code-and-a-pin-secure-pairing). |
| `GET /auth/pair/{session}` | none | Secure pairing: has someone answered? `X-WMS-Pair-Secret` header. As `/pair/poll`, plus 410 `locked`. |
| `GET /auth/pair/{session}/pin` | none | Secure pairing: the PIN to show now. `X-WMS-Pair-Secret` header. |
| `GET /auth/pair/{session}/info` | user | Secure pairing: what is asking, by session (`?k=` the QR code's check), before approving. Adults only. |
| `POST /auth/pair/{session}/approve` | user | Secure pairing: approve with `{pin, userId, check, password, otp}`. Adults only. Five wrong PINs end the session. As an administrator: with the password again. |
| `POST /auth/pair/{session}/deny` | user | Secure pairing: refuse; the device is told. Adults only. |
| `GET /me` | user | The signed-in account. |
| `GET /me/ui` | user | The account's interface choices: any JSON object the clients agree on (home layout, for one). See "Choices that follow the account". |
| `PUT /me/ui` | user | Replace them: a JSON object of 16 KB at most. |
| `GET /users` | user | Accounts this one may manage: everyone for the admin, Kids profiles for adults allowed to. |
| `POST /users` | user | Add an account or profile (adults may add Kids profiles unless the admin locked that). |
| `DELETE /users/{id}` | user | Remove an account this one manages. |
| `PUT /users/{id}/access` | user | Set an account's limits, or clear a Kids or Guest password. |
| `PUT /users/{id}/password` | user | Change a password. Your own needs `current`; other sessions of that account end. |
| `PUT /users/{id}/profile` | user | Change `displayName`, `username`, `showUsername` or `avatar` (`name:colour` from `/parental/ratings`, or empty). Which name the corner shows is anyone's own choice; the names are for an adult's own account and whoever manages the account. |
| `GET /profiles/household` | user | The profiles this device can switch to, as `GET /profiles` lists them, with `avatar` and `current`. Empty where the sign-in screen hides profiles, unless an adult asks. |
| `POST /profiles/switch` | user | Switch this device to another profile: `{name, password, code, device}` as `/login`. Answers a new `token` and `user`, and ends the session it was called with. A wrong password is 403 and the device stays signed in. |
| `GET /me/limits` | user | What this profile may do: `features` (games, recordings, downloads, sharing, comments, writeComments, syncplay, remoteControl, remote, editProfile, editLayout, liveTv), its age limits (`maxAge`, `kinds`), its `time` rules and `screenTime` now. For hiding what is off. |
| `GET /me/screen-time` | user | How long this profile may still play today: `allowed`, `reason` (allowance, bedtime, early), `remainingSec` (-1 = no limit), `until` and `nextAt` in Unix seconds, and a `message` to show. |
| `POST /me/screen-time/extra` | user | Ask a parent: `{minutes, pin}` or `{minutes, name, password}` gives this profile more time today. 403 for a wrong PIN or password, 429 after too many. |
| `POST /users/{id}/screen-time/extra` | user | Give a profile you manage more time today: `{minutes}`. |
| `GET /users/{id}/activity` | user | What a profile you manage watched and when (`watched`), its screen time by day (`days`), and what it asked for and was refused (`events`: blocked, time, feature, extra). `days` in the query, 14 unless given. |
| `DELETE /users/{id}/activity` | user | Forget what was written down about a profile you manage. Today's screen time stays. |
| `GET /parental/ratings` | user | The rating systems a limit can be chosen from (`systems`, each rating with its age), the `kinds` of media a limit can be set for, the `features` that can be turned off, and the `avatars` and `avatarColours` a profile can have. |
| `GET /parental/pin` | admin | Whether the parents' PIN is set. |
| `PUT /parental/pin` | admin | Set the parents' PIN (`{pin, current, code}`), which gives extra screen time on a child's device, or remove it with `pin` empty. |

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
| `GET /artwork/credits` | Signed in | Each poster or backdrop from Wikimedia Commons on a title the account may see: its page, licence and author, as the licences ask. |
| `GET /artwork/preview` | admin | One artwork candidate as an image. |
| `GET /items/{id}/recommend` | user | What to watch when this ends and there is no next episode: an `item`, with its `show` for a series. |
| `GET /items/{id}/credits` | user | A film's or an episode's cast, guest stars and crew by department (`cast`, `guests`, `crew`); see [People](#people). |
| `GET /shows` | user | List and filter shows. |
| `GET /shows/genres` | user | Genres among shows. |
| `GET /shows/networks` | user | TV networks with their shows and genres. |
| `GET /shows/{id}` | user | A show with its seasons, its episodes in order, and `nextUpId`: the episode the Play button means. |
| `PUT /shows/{id}` | admin | Edit a show, set its intro range or match it again. |
| `PUT /shows/{id}/view` | admin | A show's episode layout: `cards`, `list`, or empty for the default. |
| `GET /shows/{id}/artwork` | admin | Artwork to choose from. |
| `GET /shows/{id}/art/{kind}` | user | A show's picture: `poster`, `backdrop` or `season` (with `n`). |
| `GET /shows/{id}/credits` | user | A show's cast and crew; `season=<n>` adds that season's guest stars, most episodes first. |
| `GET /people` | user | People credited on titles the account may see: `search` (two letters or more), `limit`. Each with `titles`, how many of theirs the account may see. |
| `GET /people/{id}` | user | A `person` (name, dates, birthplace, biography, ids) and their `filmography` here, grouped by what they did. 404 for someone credited only on titles the account may not see. |
| `GET /people/{id}/photo` | user, `st` | A person's picture; `w` asks for a smaller copy. |
| `POST /items/{id}/progress` | user | Save the viewer's position. Past 92% counts as watched. Discs send `discTitle` too. `{id}` `show-<n>` with `{"watched": true}` marks every episode of the show this profile may see watched. |
| `DELETE /items/{id}/progress` | user | Forget the position (mark as not started). `{id}` `show-<n>`: in every episode of the show. |

### Metadata, matches and browsing

`METADATA.md` says how these fit together. `target` is `item` or `show`.

| Route | Who | What it does |
|---|---|---|
| `GET /logos/status` | Signed in | Whether logo downloads are agreed to, and for an administrator how far they have got. |
| `PUT /logos/consent` | Admin | Agree or decline logo downloads, and optionally remove what was downloaded. |
| `POST /logos/lookup` | Signed in | What to show for up to 200 names: a logo, or the name. |
| `GET /logos` | Admin | Every name the logo service knows, to find one: kind, q, limit, offset. `{logos, total}`. |
| `GET /logos/credits` | Signed in | Where every logo came from: source, licence and author. An account with limits gets the studios and networks of what it may see. |
| `GET /logos/preview` | Admin | A candidate logo, through this server. Wikimedia addresses only, and 409 until logo downloads are agreed to. |
| `GET /logos/{id}/image` | Signed in | A logo as a PNG. variant=white, dark or original. |
| `GET /logos/{id}/candidates` | Admin | What the source offers for a logo, to choose from. |
| `PUT /logos/{id}` | Admin | Use a candidate, the name alone, or look again. |
| `PUT /logos/{id}/upload` | Admin | Use your own picture as a logo. The picture is the body. |
| `GET /brands` | Signed in | The studios or networks of a library, most titles first. |
| `GET /browse` | Signed in | Films and shows as one list, by any filter and sort. |
| `GET /browse/facets` | Signed in | Everything a library can be filtered by, with counts. |
| `GET /browse/shelves` | Signed in | A library page in one answer: categories, studios, networks, collections and streaming services. |
| `PUT /browse/categories` | Admin | Rename, hide, merge or reorder a library's categories, or start again. |
| `GET /views` | Signed in | This account's saved views. |
| `POST /views` | Signed in | Save a view. |
| `PUT /views/{id}` | Signed in | Change a saved view, or pin it to Home. |
| `DELETE /views/{id}` | Signed in | Remove a saved view. |
| `GET /views/{id}/titles` | Signed in | What a saved view finds now. |
| `GET /listings/status` | Signed in | Whether streaming listings are on, for which region, and how old they are. Counts and progress are for an administrator. |
| `PUT /listings/settings` | Admin | Turn streaming listings on, after accepting every statement, or off, which forgets them. |
| `POST /listings/refresh` | Admin | Check listings again now. |
| `GET /meta/removed` | Admin | Titles taken out of libraries. |
| `POST /meta/removed/restore` | Admin | Bring a removed title back. |
| `POST /meta/undo` | Admin | Undo a group of changes. |
| `GET /meta/history` | Admin | Recent changes, each of which can be undone. |
| `POST /meta/bulk` | Admin | Edit up to 500 titles together. A picture must be a web address or a file in a library. |
| `GET /meta/show/{id}/episodes` | Admin | A show's files beside what its source lists. |
| `PUT /meta/show/{id}/episodes` | Admin | Renumber a show's episodes. |
| `GET /meta/{target}/{id}` | Admin | Everything the edit dialog needs. target is item or show. |
| `PUT /meta/{target}/{id}` | Admin | Change fields and locks. |
| `DELETE /meta/{target}/{id}` | Admin | Take a title out of its library. Its files stay. |
| `POST /meta/{target}/{id}/revert` | Admin | Put a field back to what its source said, and unlock it. |
| `POST /meta/{target}/{id}/search` | Admin | Search every source, or find an id or address. |
| `POST /meta/{target}/{id}/preview` | Admin | What choosing a result, or refreshing, would change. Changes nothing. |
| `POST /meta/{target}/{id}/match` | Admin | Choose a result. It stays chosen through scans and refreshes. |
| `DELETE /meta/{target}/{id}/match` | Admin | Remove a match and keep only what was typed. |
| `POST /meta/{target}/{id}/refresh` | Admin | Look a title up again, keeping its match and its locks. |
| `POST /meta/{target}/{id}/suggest` | Admin | Search for a title and keep the best results. |
| `POST /meta/{target}/{id}/leave` | Admin | It has no match, or is right as it is: stop asking. |
| `GET /meta/{target}/{id}/artwork` | Admin | Every picture every source has for a title. |
| `PUT /meta/{target}/{id}/art/{kind}` | Admin | Upload a poster, backdrop, banner or logo. The picture is the body. It's decoded and written again from its pixels. With `?stage=1` it's only kept, and the answer is `{url}` to apply later with `PUT /meta/{target}/{id}`. |
| `POST /meta/{target}/{id}/frame` | Admin | A frame from the video (items only), kept in the image cache: `{url}`. Nothing changes until it's applied. |
| `POST /meta/{target}/{id}/titleposter` | Admin | A poster made from words: `{title, line, hue, frame}` (`hue` 0 to 359, or left out for the title's own colour; `frame: true` sets the words over a frame from the video). Answers `{url}`. Nothing changes until it's applied. A poster made here is the owner's choice once applied; one the scan makes for a title with no poster (setting `noPoster`) is named `cache:auto-…` and replaced by any real poster. |
| `GET /meta/{target}/{id}/art/{kind}` | Signed in | A title's banner or logo. |
| `POST /meta/{target}/{id}/link` | Admin | Make an item a bonus feature or another version of something. |
| `DELETE /meta/{target}/{id}/link` | Admin | Make an attached item a title of its own again. |
| `POST /meta/{target}/{id}/move` | Admin | Put a title in another library. Its files stay. |
| `GET /meta/{target}/{id}/listings` | Signed in | Which streaming services list a title, with its date and credit. |
| `GET /meta/{target}/{id}/attached` | Signed in | A title's extras, versions, studios and other details beyond its row. |
| `GET /match/queue` | Admin | Titles worst first, with how sure each match is and why. |
| `POST /match/accept` | Admin | Choose results for up to 100 titles at once. |
| `POST /match/suggest-all` | Admin | Find suggestions for everything that needs a look, in the background. |
| `GET /match/suggest-all` | Admin | How that is going. |
| `DELETE /match/suggest-all` | Admin | Stop it. |
| `GET /match/rules` | Admin | A library's naming rules. |
| `PUT /match/rules` | Admin | Replace a library's naming rules. |
| `POST /match/rules/try` | Admin | What some rules would do to a library's files. Changes nothing. |
| `GET /metadata-folder` | admin | The [editable metadata folder](METADATA-FOLDER.md): `{path, files, pictures, layout, job}`. `job` is the export's progress and result. No route here takes a path. |
| `POST /metadata-folder/export` | admin | `{library}` (0 or left out for all): write the files, in the background. 202 with the job; 409 if the folder is busy. Never deletes, and never writes over edits not applied. |
| `GET /metadata-folder/preview` | admin | What applying would change: `{files, unchanged, total, listed, changes: [{file, target, id, name, fields: [{field, label, old, new, line}]}], problems: [{file, line, field, msg, severity}]}`. Writes nothing. |
| `POST /metadata-folder/apply` | admin | Makes those changes in one batch: `{batch, applied, fields, skipped, changes, problems}`. A bad file is listed and left out. `POST /meta/undo` with the batch takes it back. |

### Playback

| Route | Sign-in | What it does |
|---|---|---|
| `POST /items/{id}/playback` | user | The way to play anything: say what this client can play, get a decision, ready-to-use addresses, tracks, where to start and what comes next. |
| `GET /playback/profiles` | user | The built-in client profiles. |
| `GET /playback/speedtest` | user | `kb` kilobytes (64 to 4096, 512 by default) that don't compress, to time this connection before asking for a [robust stream](#robust-streaming). |
| `GET /media-types` | user | Every file type, codec and container the server knows: extensions, how each plays, which libraries take it, what it needs, and whether this machine has that (`available`). Read this rather than keep a list. |
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
| `GET /items/{id}/disc/analysis` | user | Which title (DVD) or playlist (Blu-ray) is the film and why: `main` (what Play plays), `engineMain`, `chosen`, `rank` (confidence 0–1, `level`, `why`, `reasons`, `candidates`, `versions`, `episodes`, `feature`, `repairs`, `issues`, `paddedSectors`, `secondOpinion`), `canChoose`, `protected`. `pending: true` on a low-power server while it is worked out: ask again. `ranked: false` with a module from before rankings. |
| `POST /items/{id}/disc/analysis` | admin | Work the ranking out again. `{"secondOpinion": true}` also asks a libbluray on this machine (Blu-ray only, when the setting `blurayLibbluray` is on and one is installed; never for an encrypted disc). |
| `PUT /items/{id}/disc/choice` | admin | `{"ttn": n}`: remember this title or playlist as the disc's film, for everyone. It beats the engine's pick for Play, the disc page, the item's length and languages, and survives rescans while the disc has that title. |
| `DELETE /items/{id}/disc/choice` | admin | Forget the choice: the engine's pick is the film again. |
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
| `GET /music/albums` | user | Albums of a library; an audiobook library's books, a books library's series. |
| `GET /music/artists` | user | Artists; an audiobook or books library's authors. |
| `GET /music/tracks` | user | Tracks of one album, or all songs; an empty `album` or `artist` means none or unknown. |
| `GET /music/home` | user | A music library's front page in one request: `library`. |
| `GET /music/songs` | user | Songs: `library`, `view` (`liked`, `played`, `recent`, `added`), `artist`, `genre`, `search`, `sort`, `desc`, `limit`, `offset`. |
| `GET /music/likes` | user | The ids of this person's liked songs: `library`. |
| `GET /music/radio/{id}` | user | A station made from this song, from this library: `limit`. |
| `GET /music/loudness/{id}` | user | The volume correction a file is tagged with (ReplayGain or R128), in dB. |
| `GET /music/extras/{id}` | user | Booklets and text beside a song's album. |
| `GET /music/extras/{id}/file` | user | One of those, by the `name` the list gave. |
| `GET /music/artist` | user | An artist's page: albums, songs, and a biography if online information is on: `library`, `name`. |
| `GET /music/artist-photo` | user | An artist's picture, or a cover of theirs: `library`, `name`. |
| `GET /music/lyrics/{id}` | user | A song's lyrics. Looks online only with `fetch=1`, and only if the owner has turned it on. For a music video tied to a song, the song's lyrics, with `songId`, and `startMs` (where the song starts in the video) as the starting `offsetMs`. One song is looked up at a time: `retry: true` with `failed` says another song's look-up is under way, so ask again in a few seconds. A source that couldn't be reached is left alone for 15 minutes, and `failed` says so; `again=1` asks it anyway. Give the request up (close it) when the listener moves on. |
| `PUT /music/lyrics/{id}/offset` | user | This person's timing correction: `{offsetMs}`, up to two minutes either way of where it starts. A music video's is its own. |
| `POST /music/lyrics/{id}/search` | admin | Online lyrics to choose from: `{q}`. |
| `PUT /music/lyrics/{id}` | admin | Corrects a song's lyrics on this server: `{text}`, `{instrumental}` or `{provider, id}`. |
| `DELETE /music/lyrics/{id}` | admin | Forgets the correction and any online answer. |
| `DELETE /music/lyrics-cache` | admin | Forgets all lyrics fetched from online. |
| `GET /music/links/{id}` | user | What a song or a music video is tied to (`tied`), with what's at both ends; admins also get `suggested`. Only items this person may see are listed. |
| `GET /music/links/{id}/candidates` | admin | Videos for a song, or songs for a video, to tie by hand: `q` (the title when empty), `limit`. |
| `GET /music/videos` | user | The **Music videos** section: videos from every music video library this person can see, by artist, each with the `song` it's tied to and `syncMs`: `search`, `sort` (`latest`), `limit`, `offset`. |
| `GET /music/links` | admin | Suggestions waiting to be looked at: `limit`. |
| `PUT /music/links` | admin | Ties a song to a music video for good: `{songId, videoId}`, with `syncMs` (where the song starts in the video) or `noSync`. A video has one song, so its old one is untied. |
| `DELETE /music/links` | admin | Unties them: `song`, `video`. No match ties them again. |
| `POST /music/links/match` | admin | Matches every song with every music video again now. Scans of music and music video libraries do this by themselves. |
| `GET /podcasts` | user | Podcast subscriptions. |
| `GET /podcasts/search` | admin | Search a podcast directory. |
| `POST /podcasts` | admin | Subscribe. |
| `PUT /podcasts/{id}` | admin | How many new episodes to download by themselves. |
| `POST /podcasts/{id}/refresh` | admin | Check the feed now. |
| `DELETE /podcasts/{id}` | admin | Unsubscribe; downloaded episodes stay. |
| `POST /items/{id}/download` | admin | Keep a podcast episode on disk. |

### Media extras

A song's or title's looping video, a song's separated parts for karaoke, where to skip in a
video, and where else a title can be watched. Feature names: `canvas`, `karaoke-stems`,
`skip-markers`, `where-to-watch-xref`, and `canvas-generate`, `stems-approximate`,
`stems-separate`, `intro-detection` for what the server can make itself, and `karaoke-auto` for karaoke an app
asks for in one call. `{id}` is an item, as in
`/items/{id}`. The routes that make something answer 202 with `task`: follow it at `/system/tasks`.

| Route | Sign-in | What it does |
|---|---|---|
| `GET /media/{id}/canvas` | user | `{available, url, mime, width, height, durationMs, loop: true, source, reason, task, canGenerate, limited, error}`: `error` is why the last try at cutting one failed (administrators only); `limited` is true for a viewer away from home whose streams the owner limits (leave the canvas out then: it is decoration); `source` is `upload`, `generated`, `sidecar` or `metadata-folder`; a file beside the media over 64 MB or 60 seconds isn't a canvas (`reason`: `too-big`, `too-long`); `task` is one being made now. In full: a short silent video to loop behind the player. Width, height and length need FFprobe; they're left out without it. Play `url` muted, looping. |
| `GET /items/{id}/canvas` | user | The same, under the item's usual address. |
| `GET /media/{id}/canvas/file` | user, `st` | The canvas itself, with byte ranges, an `ETag` and a day's caching (the `v` in `url` changes when the file does). |
| `PUT /media/{id}/canvas` | admin | Upload a canvas: an MP4 or WebM of at most 30 MB as the body. It replaces any upload, and wins over a file beside the media. |
| `DELETE /media/{id}/canvas` | admin | Forget the uploaded canvas. One beside the media stays. |
| `POST /media/{id}/canvas/generate` | admin | Cut a canvas from a video file: `{startSec?, durationSec?}` (2 to 15 s; by default a quarter of the way in, 5 to 8 s by the size of the machine). Silent H.264 MP4, at most 720p (480p or 360p on a small machine), its end cross-faded into its start so it loops. 202 `{task}`. 400 for a song or a disc, 503 when FFmpeg has no H.264 encoder. |
| `GET /media/{id}/stems` | user | `{stems: [{name, url, mime, size, multitrack?, source, approximate?}], names, separation}`: `source` is `upload`, `sidecar`, `separated` (the owner's separator made it) or `approximate`; `separation.approximate` and `separation.external` each say `{available, reason?, task?, note?, error?}`: `error` is why the last try failed, in words to show, until it is tried again (the separator's own words go to administrators only). `separation.best` is `separator`, `approximate` or empty: the best this server can make; `separation.separator` names the separator in use (`Demucs`), whether found on the machine or named in its configuration. In full: a song's parts, from `vocals`, `instrumental`, `drums`, `bass`, `other`, and `multitrack` for a Native Instruments `.stem.mp4` holding all of them as audio tracks. |
| `GET /media/{id}/stems/{stem}` | user, `st` | One part, with byte ranges. `accompaniment` is another name for `instrumental`. |
| `PUT /media/{id}/stems/{stem}` | admin | Upload a part: FLAC, WAV, Ogg, M4A, AAC, MP3 or WebM of at most 200 MB as the body (`multitrack`: an MP4). The server doesn't separate songs itself. |
| `DELETE /media/{id}/stems/{stem}` | admin | Forget an uploaded part. |
| `POST /media/{id}/stems/approximate` | user | Make an approximate `instrumental` with FFmpeg alone (what the two channels share is cancelled). 202 `{task}`; 200 `{exists: true}` when there is an instrumental already. It is listed with `approximate: true` and gives way to a real one. Say so to the person: the voice is quieter, not gone, and the music is mono. Mono songs fail. 503 without FFmpeg. 403 `{reason: "admin-only" | "off"}` for anyone but an administrator when the server's karaoke switch is "Only when an admin asks" or "Never"; 429 `{reason: "rate-limited"}` past 40 an hour for one account. |
| `POST /media/{id}/stems/separate` | admin | Run the separator the owner installed on the song. 202 `{task}`; 409 when none is set up (`separation.external.reason` is `not-configured`). The parts it writes are listed with `source: "separated"`. |
| `GET /media/{id}/karaoke` | user | Feature `karaoke-auto`. The one answer an app needs to play karaoke: `{state, quality, mix, stems: [{id, label, role, url, mime, defaultGain}], version, improving, progress, canImprove, reason?, message?, pollSec}`. See [Karaoke that sets itself up](#karaoke-that-sets-itself-up). Starts nothing. |
| `POST /media/{id}/karaoke/prepare` | user | The same answer, after starting what is missing: the quick version, and behind it the real one if the server has a separator. No body. Idempotent: call it once when someone opens karaoke on a song. |
| `GET /karaoke/status` | admin | For Settings: `{separator: {available, usable, name, version, source}, approximate, machine, auto, cacheMB, usedBytes, songs}`. `source` is `found` (installed on the machine and found by the server) or `config`. `look=1` looks again. |
| `GET /media/{id}/markers` | user | `{intro, recap, credits, commercials: [...], source, sources}`, each range `{startSec, endSec}`. `source` is where the most trusted came from: `manual`, `skip.json`, `edl`, `chapters`, `detected`, `show` or `none`. |
| `PUT /media/{id}/markers` | admin | Set them by hand: `{intro: {start, end}, recap, credits: {start, end?}, commercials: [...]}`. A credits range without an end runs to the end. No scan replaces this; `{"reset": true}` goes back to what the files say. |
| `POST /shows/{id}/detect-intros` | admin | Compare the sound of a season's episodes and keep the stretch they share as `intro` (15 s to 3 min, in the first 10 minutes) and `credits` (in the last 7) markers, `source: "detected"`. `{season?}`. 202 `{task, episodes, seasons}`; 409 when a run is under way or there aren't two episodes to compare. Trusted less than chapters, more than the show's own intro. |
| `GET /media/{id}/where-to-watch` | user | This title here (`inLibrary`, `local.quality`, `local.versions`) next to the services that offer it in `region` (the listings region, or `US`): `groups` of `stream`, `free`, `ads`, `rent`, `buy`, each provider with `name`, `logo` and `priority`, and `link` to the offers page. With no TMDB key: `providers: []`, `reason: "no-api-key"`. Show `attribution` wherever this is shown. |
| `GET /discover/where-to-watch` | user | The same for a TMDB title that may not be here: `tmdb`, `type` (`movie` or `tv`), `region`. `inLibrary`, and `itemId` or `showId` when it is. |

`POST /items/{id}/playback` also gives `recap`, `credits` and `commercials` beside `intro` for a
video (an intro set by hand, or in a `.skip.json` or `.edl`, replaces the chapter's), and `stems`
for a song, with the stream token already in each `url`.


#### Karaoke that sets itself up

With `karaoke-auto`, an app doesn't choose between stems, separators and approximations. When
someone opens karaoke on a song, `POST /media/{id}/karaoke/prepare` once, then play what the answer
says, and `GET /media/{id}/karaoke` again every `pollSec` seconds while it isn't 0.

| Field | What it means |
|---|---|
| `state` | `ready`: play `stems`. `preparing`: nothing to play yet, keep the normal track and say "Preparing karaoke…" with `progress`. `unavailable`: keep the normal track and show `message`. |
| `quality` | `stems` (really separated: "Separated") or `approximate` (FFmpeg's quick version: "Quick version"). |
| `mix` | `parts`: one stem with `role: "vocals"` and one or more with `role: "music"` (`instrumental`, or `drums`, `bass`, `other`); play all of them in step, each at its own gain, and the Vocals control is the vocals stem's gain. `blend`: exactly two stems, `full` (the whole song) and `instrumental`; for a voice level `v` and a music level `m`, both 0 to 1, play `full` at `m·v` and `instrumental` at `m·(1−v)`, linearly. |
| `stems` | What to play. `url` is relative to the server and takes byte ranges; what the server made is AAC in an MP4, the same length and delay for every stem of a song. A stem the owner put beside the song can be any audio type: if one can't be played, go back to the normal track. `defaultGain` is where its control starts. |
| `version` | Changes when the stems change. When it does (the real version replacing the quick one), load the new ones and carry on from the same place. |
| `improving`, `progress` | A real version is being made while the quick one plays: "Preparing a better version… 40%". `progress` is -1 when the separator doesn't say. |
| `reason` | `not-a-song`, `not-a-file`, `off` (the owner turned it off), `admin-only`, `mono`, `no-ffmpeg`, `busy`, `rate-limited`, `failed`, `not-made` (nobody has asked yet: `prepare` will). |

The stems must start together and stay together to the sample: decode them and mix them on one
clock, not as separate players. Fetching a stem counts as playing for screen time, and follows the
account's limits, as `GET /media/{id}/stems/{stem}` does; `full` is one more name that route takes.

### Books

| Route | Sign-in | What it does |
|---|---|---|
| `GET /items/{id}/book` | user | What a reader needs: `format` (the book's own: mobi, cbr, docx…), `reader` (which reader opens it: cbz, epub or pdf, also on the item as `reader`), `reading` direction, then `pages` for page by page or `spine`, `toc` and `words` for the rest. Kindle, FictionBook, Word, Rich Text, Markdown, web pages and plain text read as EPUB. A book with copy protection answers 422 and says so. |
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
| `POST /themes/custom` | admin | Add a theme from the theme maker: `name`, `scheme`, `colors` and `radius`. Colours must be exactly `#rrggbb`. |
| `DELETE /themes/{id}` | admin | Remove a theme. |
| `GET /theme` | user | The theme for this account, resolved: theme id, every colour token, each module's colours, contrast `warnings`. `ETag`; 304 with `If-None-Match`. See "The resolved theme". |
| `PUT /theme/global` | admin | The server's default theme (`id`) and global colour overrides (`tokens`). Unknown tokens or anything not a colour: 400. |
| `GET /theme/modules/{module}` | admin | A module's colour overrides, with contrast `warnings`. |
| `PUT /theme/modules/{module}` | admin | Replace a module's colour overrides: `{"tokens": {"accent": "#3ad29f"}}`. Unknown tokens or anything not a colour: 400. |
| `DELETE /theme/modules/{module}` | admin | The module goes back to the theme's colours. |
| `GET /theme/personal` | user | This profile's own colours: `tokens` over the theme it sees and `modules` over each module scope, with contrast `warnings`. Only its own. See "The profile's own colours". |
| `PUT /theme/personal` | user | Replace them: `{"tokens": {"accent": "#d02060"}, "modules": {"wittyplay": {"accent": "#3ad29f"}}}`. Unknown tokens, anything not a colour, or text that would fall below 4.5:1: 400, with the failing pairs in `contrast`. |
| `DELETE /theme/personal` | user | Back to the theme's colours. |

### Server-driven UI

The contract is [SDUI.md](SDUI.md): the model, every component and data source, what a client sends and how it draws what it gets. The routes:

| Route | Sign-in | What it does |
|---|---|---|
| `GET /sdui/catalog` | user | The catalogue: every component, card style, data source, condition, page kind, route, command, icon and device profile, described as data (labels, help, types, defaults, choices, examples), so an editor builds its palette and forms from it and a renderer knows what it must draw. 304 with If-None-Match. |
| `GET /sdui/navigation` | user | The main menu for this account on this device: entries in order with label, icon, route, placement (primary, more, end), each an SDUI page or a native screen. Settings is always there and always native. Libraries the account may not open are not listed. 304 with If-None-Match. |
| `GET /sdui/navigation/personal` | user | This profile's own arrangement of the main menu (order, hidden), and whether it may change it. |
| `PUT /sdui/navigation/personal` | user | Replace this profile's arrangement of the main menu. 403 when the profile may not rearrange pages (the editLayout switch) or the server has it off. |
| `DELETE /sdui/navigation/personal` | user | Back to the main menu as everyone has it. |
| `GET /sdui/pages/{page}` | user | A page, rendered for this account and device: an SDUI document (docs/SDUI.md, docs/sdui.schema.json). {page} is a kind of page (home, libraries, library?library=, detail?item= or ?show= or ?collection= or ?library=&artist=&album=, search?q=, livetv, music, photos, books, games, collections), chosen among the pages of that kind by what the request names, or one page's ID. Filled with what the account may see, cut to the device's budget, with components the client can't draw replaced by their fallbacks. 304 with If-None-Match. |
| `GET /sdui/pages/{page}/diff` | user | ?since=<etag>: patches from the render the client has, or the whole document when the server no longer has it. 304 when nothing changed. |
| `GET /sdui/pages/{page}/sections/{section}/items` | user | One page of a section's items: the next cards of a row or grid (cursor), a deferred row's first page, another season of a SeasonList (season). The address is given by the page itself (props.more.url, a season's url); send the same device parameters with it. |
| `GET /sdui/pages/{page}/personal` | user | This profile's own arrangement of a page (order, hidden, pinned, density, cardSize, defaultTab), and whether it may change it. |
| `PUT /sdui/pages/{page}/personal` | user | Replace this profile's arrangement of a page. Only its own. 403 when the profile may not rearrange pages (the editLayout switch) or the server has it off; 400 lists what is wrong. |
| `DELETE /sdui/pages/{page}/personal` | user | Back to the page as everyone has it. |
| `GET /sdui/topshelf` | user | Content for a tvOS Top Shelf extension: what this account was watching, up to two rows its modules offer, and what is new, within its limits, at most 4 sections of 10 pictures, each sized for the device. 304 with If-None-Match when unchanged. |
| `GET /sdui/experiences` | user | Every experience as data (`{experiences, navStyles, themes, default}`): the catalogue a chooser draws, with each experience's shell per form factor, default theme, WittyTV look, effects, shape, type set, density, motion and home. Which a person may pick is `GET /sdui/experience`. |
| `GET /setup/state` | none | First run: `{complete, steps, step, done, url}`. The steps are welcome, account, name, look, theme, modules, libraries, details, livetv, games, discs, people, remote, ready. Until `complete`, every route an app uses answers 503 `setup_required` (below). |
| `PUT /setup/state` | none, until first run is over | `{step, done}`. Before the first account: the setup code (`X-WMS-Setup-Code` from another device). After: an administrator. 409 once first run is over. |
| `POST /setup/complete` | admin | First run is over: apps may connect. |
| `GET /sdui/preview` | none | `experience=<id>` (and `theme`, `platform`, `formFactor`, `viewport`): `{experience, document, navigation, theme}`, a sample Home and menu made from generated demo data with the resolved theme (tokens, shape, type, finish, scene, tv, elevation, brand), so the first-run chooser draws every look with the client's own renderer before anyone has signed in. Rate limited. |
| `POST /themes/mine` | user | Import a theme ZIP (the body, at most 26 MB) for this profile when the administrator allows it (`profileThemes`; administrators always may, and theirs is the server's). 403 when not allowed, 400 with the offending rule named when `theme.css` holds anything but variables, 409 for an id someone else has or after five themes. A profile's theme is offered to that profile only. |
| `GET /sdui/placements` | user | `{experience, regions, placements, own, editable}`: the shell's regions (brand, nav, search, profile, clock, dock, legend, hero; the menu and You are locked), where each is on this device (the experience's, the administrator's and the profile's merged: also `layout.shell.placements`), the profile's own and whether it may change them. `device=` as for `/sdui/experience`. |
| `PUT /sdui/placements` | user | `{placements: {<region>: {position, visible, style}}}`, replacing the profile's earlier ones on this device. 400 for a region, position or style that isn't there and for hiding the menu or You; 403 when the profile may not rearrange. The administrator's are `placements` in `PUT /sdui/config/experience`. |
| `DELETE /sdui/placements` | user | Back to the experience's and the administrator's. |
| `GET /sdui/experience` | user | The experience in force on this device (`device=` names the app's own device, else the form factor): `{experience, shell, choice: {current, chosen, options, url, required}, editable}`. `required` (`{value, by, at, scope}`) is set when an administrator requires the look: `editable` is then false. |
| `PUT /sdui/experience` | user | This profile's experience on this device: `{experience}`. 403 when the profile may not rearrange, the server doesn't offer it, or an administrator requires the look. |
| `DELETE /sdui/experience` | user | This device follows the server's experience again. |
| `GET /sdui/modules/settings` | user | The modules with settings this profile sees: `{modules: [{module, label, icon, url}]}`, for the list under Settings → Modules. |
| `GET /sdui/modules/{module}/images/{name}` | user | A picture a module's cards show (module sources put these addresses in `images`). PNG, JPEG, WebP or GIF. |
| `GET /sdui/modules/{module}/settings` | user | A module's settings, described and with their values, for the app's own Settings to draw under Settings → Modules ([SDUI-MODULES.md](SDUI-MODULES.md#settings)). |
| `PUT /sdui/modules/{module}/settings/personal` | user | This profile's own values of a module's settings: `{values}`. |
| `GET /sdui/config` | admin | All of ui_config.json: the pages and navigation an administrator changed, the theme overrides, with its ETag. |
| `PUT /sdui/config` | admin | Replace ui_config.json. ?target=draft writes to the draft; otherwise If-Match is required (428 without, 412 when stale). Everything is validated; 400 lists the issues, each with a path, a message and a fix. |
| `GET /sdui/config/history` | admin | The last 10 revisions, newest first. |
| `GET /sdui/config/diff` | admin | What differs between two revisions (?from= and ?to=: a revision number, current or draft): pages added, removed and changed, and inside a changed page the sections added, removed, changed and whether they were reordered. `{from, to, fromDraft, toDraft, changes}`: for the draft, the revision it was begun from, with fromDraft/toDraft true. |
| `POST /sdui/config/rollback/{rev}` | admin | Make a kept revision current again, as a new revision. |
| `GET /sdui/config/pages` | admin | Every page in force, built-in ones included: id, kind, title, icon, match, and whether it is builtin, changed or custom. |
| `GET /sdui/config/pages/{page}` | admin | One page as authored, where it comes from, the built-in page it would reset to, and what the validator says of it. |
| `PUT /sdui/config/pages/{page}` | admin | Save one page. ?target=draft writes to the draft; otherwise If-Match is required (428 without, 412 when stale). 400 lists the issues. |
| `DELETE /sdui/config/pages/{page}` | admin | Remove the configuration's page: a built-in page goes back to how it ships (reset to default), a page of the administrator's own is gone; 404 for a page that is in neither. ?target=draft writes to the draft; otherwise If-Match is required (428 without, 412 when stale). |
| `GET /sdui/config/navigation` | admin | The navigation as authored, and the built-in one. |
| `PUT /sdui/config/navigation` | admin | Save the navigation. ?target=draft writes to the draft; otherwise If-Match is required (428 without, 412 when stale). Settings can be moved but not hidden or given a condition. |
| `DELETE /sdui/config/navigation` | admin | Back to the built-in navigation. ?target=draft writes to the draft; otherwise If-Match is required (428 without, 412 when stale). |
| `POST /sdui/validate` | admin | Check {"page": ...}, {"navigation": ...} or {"config": ...} without saving: {ok, issues: [{path, severity, message, fix}]}. ?target=draft: a navigation may name the draft's pages. |
| `GET /sdui/presets` | admin | The gallery of ready-made pages and sections: {presets: [{id, label, description, icon, kind, pageKinds, page or section}]}. |
| `GET /sdui/config/experience` | admin | The server's experience policy: `{etag, experiences: {default, allowed, required, profileThemes}, options, builtin}`. |
| `PUT /sdui/config/experience` | admin | Saves `{default: {formFactor: experience}, allowed: [experience], required: {everyone, kids, profiles: {id: {experience, theme}}}, profileThemes}`; the server stamps each requirement with who set it and when (400 for an experience or theme that isn't there); If-Match, or `target=draft`. |
| `GET /sdui/modules` | admin | Every module in use with its SDUI file: `{modules: [ModuleUI]}`. |
| `PUT /sdui/modules/{module}/settings` | admin | The server's values of a module's settings: `{values}`; null resets one. |
| `POST /sdui/presets/{id}/apply` | admin | Put a page preset in the configuration under the preset's own page ID. ?target=draft writes to the draft; otherwise If-Match is required (428 without, 412 when stale). |
| `GET /sdui/draft` | admin | The unpublished draft; 404 when there is none. |
| `PUT /sdui/draft` | admin | Keep a whole configuration as the draft. If-Match, when sent, is the draft's ETag (the published configuration's when there is no draft yet). |
| `DELETE /sdui/draft` | admin | Forget the draft and every preview of it. |
| `POST /sdui/draft/publish` | admin | Apply what the draft changed since the revision it was begun from to the configuration everyone gets, as a new revision; what was published meanwhile is kept. 409 {conflicts, baseRevision, currentRevision} when both changed the same page, navigation, theme or personalisation; ?prefer=draft or ?prefer=published settles them. If-Match, when sent, is the draft's ETag. |
| `POST /sdui/draft/preview` | admin | A token that shows the draft to this administrator alone, on any device they are signed in on, for two hours: {token, expiresAt}. Sent as preview=<token> or X-WMS-SDUI-Preview with the page and navigation routes. |
| `GET /sdui/export` | admin | One page (?page=<id>) or the whole configuration as a file: {format: "wittymedia-ui", formatVersion, kind, page or config}. |
| `POST /sdui/import` | admin | Bring in a file made by export: a page (saved under its own ID) or a configuration (its pages and navigation replace the target's; its theme colours too with theme=1). Into the draft unless target=published, which needs If-Match. dryRun=1 only reports the issues and the changes. |

### Channels and live TV

| Route | Sign-in | What it does |
|---|---|---|
| `GET /guide` | user | Every channel this account may watch, with its programmes. Each can name `logo`, a picture of its own, or `brand`, for `POST /logos/lookup`, and always has `canRecord`, the only signal for offering Record (see "Recording"). Beside them, `theme`: the owner's WittyTV look for anyone whose own choice and UI suggest none, and `backgroundPlay`: whether a device that hasn't chosen keeps playing in the background. |
| `GET /tv/{key}/tune` | user | What is on a channel, and starts its stream, with `canRecord`. `streamToken=1` adds `streamUrl` for players that can't set headers. See "Live channels". |
| `GET /tv/{key}/live/playlist.m3u8` | user, `st` | A channel's live HLS playlist. |
| `GET /tv/{key}/live/seg/{name}` | user, `st` | One segment of it. |
| `POST /tv/{key}/leave` | user | This viewer has gone: `viewer`. |
| `GET /time` | user | The server's clock, `{"now": <milliseconds>}`, for keeping to the same moment on a channel. See "Live channels". |
| `GET /tv/{key}/logo` | user | A tuner channel's logo, fetched and kept by the server, so the site a playlist names never sees the viewer. The `logo` in `/guide` and `/tune` is this address. |
| `GET /tv/settings` | admin | TV settings. |
| `PUT /tv/settings` | admin | Put TV settings. `tvBackgroundPlay` (`"1"` or `"0"`) is what a device that hasn't chosen does in the background; see "Playing in the background". |
| `PUT /libraries/{id}/main` | admin | Library show main. |
| `GET /channels` | user | Library channels, each with `canRecord: false`. |
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
| `POST /channels/{id}/reset` | admin | Reset channel: throws away the schedule, what is on now included, and makes it again from the channel's current settings and matching titles. Body `{"startOver": true}` also starts every show from its first episode and shuffles again. Viewers move to the new schedule; reminders are kept, moved to the programme's new time, or removed. Answers `{ok, watching, reminders: {kept, moved, dropped, droppedTitles}}`. |
| `GET /channels/{id}/history` | admin | Channel history. |
| `POST /channels/{id}/restore` | admin | Restore channel. |
| `GET /channels/{id}/logo` | user | Channel logo. |
| `PUT /channels/{id}/logo` | admin | Set channel logo. |
| `DELETE /channels/{id}/logo` | admin | Delete channel logo. |
| `GET /tv/encrypted-help` | user | Encrypted help. |
| `GET /guide/search` | user | What is coming up that matches, soonest first, each with its channel's `canRecord`. q (2 to 100 letters), days (1 to 14), limit. 429 with `Retry-After` when several searches are running. |
| `GET /tv/favourites` | user | This account's favourite channels, as channel keys. The guide marks them too. |
| `PUT /tv/favourites/{key}` | user | Star a channel. |
| `DELETE /tv/favourites/{key}` | user | Unstar a channel. |
| `GET /tv-sets` | user | Every channel set this profile can choose: all, favourites, its own and shared sets, each source, each group, with counts. See "Channel sets". |
| `POST /tv-sets` | user | Make a custom set: `name`, `channels` (keys, in order), `shared` (administrators). |
| `PUT /tv-sets/order` | user | This account's order of the custom sets it sees: `ids`. |
| `GET /tv-sets/{id}` | user | One custom set with its channels in order. |
| `PUT /tv-sets/{id}` | user | Rename, share or replace the channels of a set. |
| `POST /tv-sets/{id}/channels` | user | Add and remove channels: `add`, `remove`. |
| `DELETE /tv-sets/{id}` | user | Delete a set. |
| `GET /tv/{key}/programme` | user | WittyTV More info: one programme in full, `start=<unix>`. See "More info". |
| `GET /tv/{key}/catchup` | user | Watch a programme from its start, from the source's archive: `start`, `end`, `viewer`. Answers like `/tune`. See "Catch-up". |
| `GET /tv/{key}/catchup/{start}/playlist.m3u8` | user | The catch-up stream. |
| `GET /tv/{key}/catchup/{start}/seg/{name}` | user | A segment of the catch-up stream. |
| `GET /tv/reminders` | user | This account's reminders still to come, each with its channel's `canRecord`. |
| `POST /tv/reminders` | user | Body: channel, title, sub, start, end. The same programme twice is one reminder. |
| `DELETE /tv/reminders/{id}` | user | One of this account's reminders. |
| `GET /channels/bumpers` | admin | Videos uploaded for the purpose, chosen from any library, or in a commercials library. |
| `POST /channels/bumpers` | admin | Body: itemId. Any video in any library. |
| `POST /channels/bumpers/upload` | admin | The body is the file; ?name= is its name. A video of up to 30 minutes and 2 GB. 507 when the disk is nearly full. |
| `DELETE /channels/bumpers/item/{id}` | admin | An upload goes altogether, its file too. A chosen video only comes off the list. |
| `GET /channels/discs` | admin | Every disc, whether it is encrypted, and the words for refusing those that are. |
| `POST /channels/clip-preview` | admin | Body: item, title. An HLS stream of the title, to try a scene before adding it. |
| `GET /tuners/drivers` | admin | Tuner drivers. |
| `GET /tuners/discover` | admin | Tuner discover. |
| `GET /tuners` | admin | Tuners. |
| `POST /tuners` | admin | Create tuner. |
| `POST /tuners/check` | admin | Check tuner: what a source says before it's saved. Answers `{ok, result}` or `{ok: false, problem: {kind, message, hint}}`, and keeps nothing. |
| `PUT /tuners/{id}` | admin | Update tuner. |
| `DELETE /tuners/{id}` | admin | Delete tuner. |
| `POST /tuners/{id}/refresh` | admin | Refresh tuner. |
| `POST /tuners/{id}/scan` | admin | Scan tuner. |
| `GET /tuners/{id}/scan` | admin | Scan progress2. |
| `POST /tuners/{id}/test` | admin | Test tuner: tunes one channel (`{channel}`, or 0 to let it choose) for a few seconds, as a viewer would. Encrypted channels are refused as everywhere else. |
| `GET /tuners/{id}/channels` | admin | Tuner channels. |
| `POST /tuners/{id}/channels/enable` | admin | Enable all channels. |
| `PUT /tuner-channels/{id}` | admin | Edit tuner channel. `logo` is `""` to find it by name, `none`, or `kind\|name` in the logo service. |
| `GET /epg/sources` | admin | EPG sources. |
| `POST /epg/sources` | admin | Add EPG source. |
| `DELETE /epg/sources/{id}` | admin | Delete EPG source. |
| `POST /epg/sources/{id}/refresh` | admin | Refresh EPG source. |
| `GET /epg/ids` | admin | EPGI ds. |
| `GET /recordings` | user | This account's recordings, and an administrator's sees everyone's. |
| `POST /recordings` | user | Schedule. Only a channel whose `canRecord` is true: 400 for a library channel, 403 for an encrypted one, one marked not to be recorded, or a profile without recordings, 404 for a channel turned off, 409 at 50 waiting (not for an administrator). See "Recording". |
| `DELETE /recordings/{id}` | user | Cancel a recording of this account's; an administrator, anyone's. |

### Games

| Route | Sign-in | What it does |
|---|---|---|
| `GET /games/home` | user | The games home screen in one answer, for `?library=`. |
| `GET /games/{id}/check` | user | What a game needs in order to play, and whether this server has it. |
| `GET /games/consoles` | user | Game consoles. |
| `GET /games/modes` | user | Game modes. |
| `GET /consoles/{console}/controls` | user | Game controls. |
| `PUT /consoles/{console}/controls` | user | Put game control. |
| `POST /games/{id}/play` | user | Game play. |
| `GET /games/{id}/emulators` | user | The standalone emulators set up to play this game: {emulators: [{id, name, version, stream, ready, why}]}. Empty when WittyPlay has none. 403 `feature-off` with games off for the profile. |
| `POST /games/{id}/play/emulator` | user | Start this game in a standalone emulator: {emulator} in; 201 with the session and where to connect, stop, pause and resume it. Refused with 403 `screen-time` when the profile's time is up. The game belongs to the sign-in that started it: it is ended when that sign-in ends (signing out, switching profile), when the profile's screen time runs out, and when games are turned off for the profile. |
| `POST /games/{id}/prepare` | user | Game prepare. |
| `GET /games/{id}/versions` | user | A game's other versions and its discs. See [GAMING-API.md](GAMING-API.md). |
| `PUT /games/{id}/version` | admin | Which file in its archive to play. |
| `POST /games/{id}/split` | admin | A version becomes a game of its own. |
| `PUT /games/{id}/console` | admin | This file is for another console. |
| `POST /games/{id}/not-a-game` | admin | Takes it off the list; its file stays. |
| `GET /libraries/{id}/games/review` | admin | Files that didn't become games, or can't play. Query: `kind`, `limit`, `offset`. |
| `POST /libraries/{id}/games/review/apply` | admin | Says what those files are. |
| `GET /libraries/{id}/games/import` | admin | How an import is going. |
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
| `GET /games/playing` | user | The game this account is playing now, the device it started on, and who joined it; 204 when none. For "Continue here". |
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
| `GET /modules/wittyplay/docker/status` | admin | Whether WittyPlay's Docker backend is on, whether Docker answers, and how many containers run. |
| `GET /modules/wittyplay/docker/settings` | admin | The Docker backend's settings, their schema and the backends. |
| `PUT /modules/wittyplay/docker/settings` | admin | Replace the Docker backend's settings. WittyPlay checks them. |
| `GET /modules/wittyplay/docker/emulators` | admin | The standalone emulators WittyPlay lists, with what's installed. |
| `GET /modules/wittyplay/docker/installed` | admin | The standalone emulators installed. |
| `POST /modules/wittyplay/docker/emulators/{id}/install` | admin | Start installing a standalone emulator. 202 and the job. |
| `GET /modules/wittyplay/docker/emulators/{id}/install` | admin | How an emulator's install is going. |
| `DELETE /modules/wittyplay/docker/emulators/{id}` | admin | Uninstall a standalone emulator. Refused while someone plays with it. |
| `POST /modules/wittyplay/docker/sessions/{sid}/connect` | user | WebRTC for a game in a container: {hello, offer} in, the same answer a core's game gives. |
| `DELETE /modules/wittyplay/docker/sessions/{sid}` | user | Stop a game in a container: the player's own, or any for an administrator. |
| `GET /modules/wittyplay/docker/diagnostics` | admin | A look over everything standalone emulators depend on here, in plain words: {host, checks}. |
| `DELETE /modules/wittyplay/docker/emulators/{id}/install` | admin | Cancel an emulator's install. What had been downloaded is kept for the next try. |
| `GET /modules/wittyplay/docker/emulators/{id}/settings` | user | An emulator's settings as this account sees them: the schema, each layer and what they come to. `?game=` adds one game's. |
| `PUT /modules/wittyplay/docker/emulators/{id}/settings` | user | Set an emulator's settings: {scope: admin, user or game, game, values}. Checked against the schema. |
| `GET /modules/wittyplay/docker/emulators/{id}/manual` | admin | An emulator that's never downloaded (Ryujinx, Lichii): its project's page, the program names it's known by on the system it runs on, the drop folder, `manual.json`'s path, and what was found where it's usually installed. |
| `POST /modules/wittyplay/docker/emulators/{id}/manual` | admin | Add it: {program: a full path, or "" for what was found}. Checked to be that emulator; answers as the GET, with `installed`. |
| `GET /modules/wittyplay/docker/own` | admin | The administrator's own emulator programs, and how many may be kept. |
| `PUT /modules/wittyplay/docker/own` | admin | Replace them: a list of {name, systems, program, args, configDir, like}. Each is checked to be a program on this server. |
| `GET /modules/wittyplay/docker/sessions` | user | Games running in standalone emulators: the caller's own, or everyone's for an administrator. |
| `POST /modules/wittyplay/docker/sessions/{sid}/pause` | user | Freeze a game in a standalone emulator. The player's own only. |
| `POST /modules/wittyplay/docker/sessions/{sid}/resume` | user | Let a paused game in a standalone emulator go on. The player's own only. |
| `GET /modules/wittyplay/kasm/{sid}/{ticket}/{rest}` | public | The KasmVNC desktop of a game in a container, and its WebSocket. Open the `kasmUrl` a start gives, as it is, in a frame sandboxed with `allow-scripts allow-pointer-lock` and never `allow-same-origin`: the ticket in it is the whole of the way in. 404 unless this server started that game, it is still going, and the profile that started it may still play. |

### Game PCs

**WittyCouch** plays the games on any PC running Sunshine, through this server. Each account has its own list of PCs, and its own pairing with each.
A client can show the player in a web view (`/couch/{id}/play/{app}`), or start a stream and connect with `POST /couch/sessions/{sid}/webrtc`:
H.264 and Opus, with data channels `control` (reliable) and `input` (unordered), whose messages are in `PROTOCOL.md` in WittyCouchHost.
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
| `POST /collections/watch-later/items` | user | Watch later add. `{items, shows}`: each a number, or a string `"5"` or `"show-5"` (a show, wherever it is listed). |
| `DELETE /collections/watch-later/items` | user | Watch later remove, with the same body; titles that weren't on it are no error. A title page's DetailHeader offers it as `watchLater.remove` ("In Watch later", `pressed: true`) when the title is on the list. |
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
| `GET /radio/{id}/hls` | user, st | A part of an HLS station. These addresses come from this server's playlists and are sealed: only this server can read them, so they can't be built. |

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
| `GET /remote/status` | none | How remote access stands, in the settings page's own words: `level` (good, waiting or fault), `subject` (such as "Home network only" or "Reachable from away: yes"), `summary`, `home`, `away`, `checked`, `state` and `publicUrl`, and `warning` while the owner allows unencrypted connections from away: show it as it is, and prominently. Show them as they are, on a connection screen. Needs a sign-in, except from the server's own computer. |
| `PUT /remote` | admin | Put remote. |
| `POST /remote/test` | admin | Start test. |
| `POST /remote/selfcheck` | admin | Self check. |
| `POST /remote/router` | admin | Look router. |
| `POST /remote/obtain` | admin | Obtain. |
| `DELETE /remote/lockouts` | admin | Clear lockouts. |
| `DELETE /remote/plain-sessions` | admin | Sign out every session whose sign-in crossed the internet unencrypted (`plainAway` in `GET /sessions`). Answers `{ended}`. |
| `GET /p2p` | user | Whether peer-to-peer connections are on, and what an app needs for them: `serverId` (what to dial), `addrs` (where the server was last reachable, to keep as hints), `devicesOnly`, `layers`. `?device=<id>` adds `enrolled`. See P2P.md. |
| `GET /p2p/devices` | user | The devices that may connect peer to peer: yours, or everyone's with `?all=1` for an admin. Each has `id`, `name`, `platform`, `lastSeen`, and `connected` with its `path` (direct or relayed). |
| `POST /p2p/devices` | user | Put this device's key on the list, under your account: `{id, name, platform}`. An app does this once, while it reaches the server some other way. |
| `DELETE /p2p/devices/{id}` | user | Take a device off the list. Its open connections close at once. |
| `POST /p2p/invite` | user | `{userId, password, otp}`: an invite link (`wittymedia://p2p/...`) that puts one device on the list and signs it in, for this account or a Kids or Guest profile it manages. Adults only; an invite that signs a device in as an administrator takes the administrator's password again (403 with `reason: "password-required"` without it), as `/pair/approve` and `/p2p/join` do. Works once, for 15 minutes. |
| `POST /p2p/join` | user | `{code, userId, password, otp}`: the code a device away from home is showing, typed in by someone signed in. The device finds the server by it, is put on the list and signed in. Adults only; signing a device in as an administrator takes the administrator's password again (403 with `reason: "password-required"` without it). Works once, for 15 minutes. 409 when peer-to-peer connections or their address book are off. Feature `p2p-join`. |
| `GET /p2p/settings` | admin | The owner's choices, how every layer is doing in plain words (`steps`), who is contacted (`thirdParties`) and every device. |
| `PUT /p2p/settings` | admin | Change them. Turning the feature on needs `accept: true`. |
| `GET /p2p/status` | admin | Only how every layer is doing, for a page that stays open. |
| `POST /p2p/test` | admin | Connect to this server as an app away from home would, from a second node, and say how far it got. |
| `POST /p2p/rotate` | admin | Give the server a new connection key and ID: `{password, code}`. Every device must learn the new ID. |
| `GET /sessions` | user | Sessions. |
| `DELETE /sessions` | user | End other sessions. |
| `DELETE /sessions/{id}` | user | End session. |
| `GET /me/2fa` | user | Two factor. |
| `POST /me/2fa/setup` | user | Two factor setup. |
| `POST /me/2fa/confirm` | user | Two factor confirm. |
| `POST /me/2fa/disable` | user | Two factor disable. |

### Remote control

One client playing on and steering another: [REMOTE-PROTOCOL.md](REMOTE-PROTOCOL.md) has every message.

| Route | Sign-in | What it does |
|---|---|---|
| `GET /remote-control` | user | The Remote Protocol's versions, limits and vocabulary, and whether this account may control devices signed in as other accounts. See REMOTE-PROTOCOL.md. |
| `GET /remote-control/devices` | user | The devices this account may control, each with what it last said it was doing. |
| `POST /remote-control/devices` | user | Announce this client as a device that can be controlled. The answer is the id and capabilities agreed. |
| `DELETE /remote-control/devices/{id}` | user | This client stops being controllable. |
| `GET /remote-control/devices/{id}/channel` | user | The device's channel, as server-sent events: hello, then a command for each thing it is asked to do. |
| `POST /remote-control/devices/{id}/state` | user | The device says what it is playing. Also its heartbeat. |
| `POST /remote-control/devices/{id}/ack` | user | The device answers a command: done, or not and why. |
| `POST /remote-control/devices/{id}/commands` | user | Ask a device to do something and wait for its answer: play, pause, resume, stop, seek, next, previous, volume, mute, audio, subtitle, rate, navigate or cast. |
| `GET /remote-control/events` | user | Server-sent events for controllers: a snapshot of devices, then each one as it appears, changes or goes. |
| `GET /remote-control/accounts` | admin | Every account, and whether it may control devices signed in as other accounts. |
| `PUT /remote-control/accounts/{id}` | admin | Let an account control devices signed in as other accounts, or stop it. |

### Watch parties

Watching together, in step: [SYNCPLAY.md](SYNCPLAY.md) has the rules every player keeps.

| Route | Sign-in | What it does |
|---|---|---|
| `GET /syncplay` | user | Whether watch parties are on, the drift rules and limits, and this account's rooms and invitations. See SYNCPLAY.md. |
| `POST /syncplay/rooms` | user | Start a watch party for an item or a queue, as its host. |
| `POST /syncplay/join` | user | Join a watch party by its code, or come back to a seat. |
| `GET /syncplay/rooms/{id}` | user | The room, as one of its members sees it. |
| `PUT /syncplay/rooms/{id}` | user | The host changes the room's name, who may control it, or whether it waits for everyone. |
| `DELETE /syncplay/rooms/{id}` | user | End the watch party for everyone. |
| `GET /syncplay/rooms/{id}/events` | user | Server-sent events: the room, whole, whenever anything about it changes. |
| `POST /syncplay/rooms/{id}/leave` | user | Give up a seat. |
| `POST /syncplay/rooms/{id}/host` | user | Hand the room to another member. |
| `POST /syncplay/rooms/{id}/kick` | user | Take a member's seat away. |
| `POST /syncplay/rooms/{id}/invite` | user | Invite accounts, who then find the room in their list. |
| `POST /syncplay/rooms/{id}/command` | user | Play, pause, seek, next, previous, select, rate or queue. The answer is the room after it. |
| `POST /syncplay/rooms/{id}/report` | user | A member's player says it is ready, or buffering. |
| `POST /syncplay/rooms/{id}/ping` | user | The heartbeat and the clock: answers with the server's time on arrival and departure. |

### Background tasks

| Route | Sign-in | What it does |
|---|---|---|
| `GET /system/tasks` | user | What's going on now and what finished lately, as this account may see it |
| `GET /system/tasks/stream` | user | Server-sent events: a snapshot, then what changes |
| `GET /system/tasks/history` | user | Finished tasks, newest first |
| `DELETE /system/tasks/history` | admin | Forgets the history |
| `DELETE /system/tasks/history/{id}` | admin | Forgets one finished task |
| `POST /system/tasks/{id}/pause` | admin | Asks a task to pause at its next step |
| `POST /system/tasks/{id}/resume` | admin | Lets a paused task carry on |
| `POST /system/tasks/{id}/cancel` | admin | Asks a task to stop |
| `POST /system/tasks/{id}/retry` | admin | Starts a failed or cancelled task again |

### Library transcoding

See [LIBRARY-TRANSCODING.md](LIBRARY-TRANSCODING.md).

| Route | Sign-in | What it does |
|---|---|---|
| `GET /libtranscode/options` | admin | Presets, and what this machine's FFmpeg can do |
| `GET /libtranscode/settings` | admin | How conversions run on this server |
| `PUT /libtranscode/settings` | admin | Changes that. Turning it on needs the warning accepted |
| `GET /libtranscode/libraries` | admin | Libraries that hold video files, each with its recipe and anything stopping it |
| `PUT /libtranscode/libraries/{id}` | admin | Saves a library's recipe, what happens to old files, and whether it's read-only. Starting to replace originals also takes `{current, code}`: the account's own password, and its two-factor code if it has one |
| `GET /libtranscode/libraries/{id}/items` | admin | Finds files to try a recipe on |
| `POST /libtranscode/libraries/{id}/preview` | admin | What a recipe would do to one file, track by track. Changes nothing |
| `POST /libtranscode/libraries/{id}/sample` | admin | Really converts 20 seconds into the server's cache, and measures it |
| `POST /libtranscode/libraries/{id}/runs` | admin | Starts a run. `dry: true` looks at every file and changes nothing |
| `GET /libtranscode/runs` | admin | Recent runs, newest first |
| `GET /libtranscode/runs/{id}` | admin | One run |
| `GET /libtranscode/runs/{id}/files` | admin | A run's files and what became of each |
| `GET /libtranscode/leftovers` | admin | Kept originals and converted copies waiting for a decision |
| `POST /libtranscode/files/{id}/undo` | admin | Puts the original back |
| `POST /libtranscode/files/{id}/use-copy` | admin | Puts a converted copy into use, keeping the original |
| `DELETE /libtranscode/files/{id}/leftover` | admin | Deletes a kept original or an unused copy, for good |
| `GET /libtranscode/tdarr` | admin | The connection to a separately installed Tdarr. The API key is never sent back |
| `PUT /libtranscode/tdarr` | admin | Changes that |
| `POST /libtranscode/tdarr/test` | admin | Asks a Tdarr server about itself, without saving anything |
| `POST /libtranscode/tdarr/send` | admin | Hands a library's files to Tdarr |

### Server statistics

| Route | Sign-in | What it does |
|---|---|---|
| `GET /system/metrics` | user | What statistics this server can show, and what this account may see |
| `GET /system/metrics/now` | user | One reading |
| `GET /system/metrics/stream` | user | Server-sent events: a reading every two seconds, while it's open |
| `GET /system/metrics/history` | user | Charts' worth of numbers, for administrators |
| `GET /system/metrics/report` | user | Libraries, what's in them, and what's gone on |
| `GET /system/metrics/drives` | admin | Each disk's health, where the system will say |

### More server routes

| Route | Sign-in | What it does |
|---|---|---|
| `GET /play/{sid}` | user | Play get. |
| `DELETE /play/{sid}` | user | Play stop. |
| `POST /play/{sid}/webrtc` | user | Play web RTC. |
| `GET /play/{sid}/ws` | user | Play WS. |
| `POST /play/{sid}/stream` | user | Connects a device over WebRTC, version 2: `{hello, sdp}` in, `{stream, sdp}` out. See [the stream](GAMING-STREAM.md). |
| `GET /play/{sid}/stream` | user | Version 2's WebSocket: the same messages as over WebRTC, the hello first. |
| `POST /play/join` | user | Join someone's game with their code, `{code}`, or a friend's open room with none, `{room: sid}` (not for a Kids or guest profile). Answers `{sid, seat, itemId, title, console, host}`. Wrong codes are limited. |
| `POST /play/{sid}/invite` | user | The host makes a four-letter code others can join with, for ten minutes. |
| `GET /play/{sid}/seats` | user | The host and everyone who joined; the host also sees the code. |
| `DELETE /play/{sid}/seats/{user}` | user | Frees a seat: the host frees anyone's, and anyone may leave. |
| `POST /play/{sid}/room` | user | The host opens its game to the friends who may see it, so they join with no code (`{open}`, default true), or closes it. Answers `{sid, open, code, expiresAt}`: the code is for a phone's QR join link. Kids and guest profiles can't open a game. |
| `PUT /play/{sid}/seats/{user}` | user | The host moves someone who joined to another player, `{seat}` from 1, or -1 to watch; a seat someone else holds is swapped. Answers the seats. |
| `POST /play/{sid}/screenshot` | user | Keeps the current picture in the host's memories. Answers `{name, itemId, url}`. |
| `GET /games/{id}/shots/{name}` | user | One kept screenshot, as a PNG. |
| `DELETE /games/{id}/shots/{name}` | user | Deletes a kept screenshot. |
| `GET /games/online/me` | user | What this profile lets its friends know: `{sharePlaying, appearOnline, canShare}`. Not sharing what it plays shows it online with no game; not appearing online shows it offline. |
| `PUT /games/online/me` | user | Sets either or both: `{sharePlaying, appearOnline}`. A guest has no friends to share with. |
| `GET /games/online/kids/{user}` | admin | The profiles a Kids profile may see, and be seen by, as friends: `{user, name, friends, candidates: [{id, name, kind, approved}]}`, every profile that could be one. |
| `PUT /games/online/kids/{user}` | admin | Sets them: `{friends: [id…]}`. |
| `POST /play/{sid}/action` | user | Play action. |
| `GET /play/{sid}/stats` | user | A running game's numbers: frame rates, encode time, bitrate, drops. |
| `GET /play/{sid}/screenshot` | user | The current picture as a PNG. |
| `GET /gaming` | admin | Gaming. |
| `POST /gaming/cores/{id}/install` | admin | Core install. |
| `GET /gaming/cores/{id}/install` | admin | How far an install under way has got: `{installing, received, total}` in bytes, `total` 0 when the download server gave no size. An older WittyPlay always answers `installing: false`. |
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
| `GET /gaming/consoles/{console}/core` | admin | A console's cores for the core picker, installed or not, and its default: see [Choosing a core](GAMING-API.md#choosing-a-core). |
| `PUT /gaming/consoles/{console}/core` | admin | Choose a console's core. |
| `GET /gaming/games/{id}/core` | admin | What plays a game, and how each core has gone on it: the picker's `choices`, and `options`, the installed ones, for older clients. |
| `PUT /gaming/games/{id}/core` | admin | Choose a core for one game. |
| `GET /reach/{nonce}` | none | Reach. |
| `DELETE /users/{id}/2fa` | admin | User two factor reset. |
| `GET /recording-rules` | user | This account's recording rules, and an administrator's sees everyone's. |
| `POST /recording-rules` | user | Record every airing of a title. Only a channel whose `canRecord` is true, refused as `POST /recordings` is; 409 at 20 (not for an administrator). See "Recording". |
| `PUT /recording-rules/{id}` | user | Change how a recording rule records: `anyChannel`, `newOnly`, `keep`, `padBefore`, `padAfter`. |
| `DELETE /recording-rules/{id}` | user | Delete a recording rule of this account's; an administrator, anyone's. |
| `POST /tuner-channels/bulk` | admin | Many tuner channels at once: `ids`, `action` (enable, disable, renumber with `start`, merge with `into`, unmerge, check). |
| `GET /tuners/duplicates` | admin | Channels across every tuner that look like the same station, in groups. |
| `POST /tuners/{id}/health` | admin | Check every enabled channel of a tuner, in the background. |
| `GET /tuners/{id}/health` | admin | How a tuner's channel check is going. |
| `DELETE /tuners/{id}/health` | admin | Stop a tuner's channel check. |
| `GET /dvd/{sid}/menu/{serial}/effect/{n}` | user | DVD effect. |
| `GET /dvd/{sid}/sound/{n}` | user | DVD sound. |
| `GET /dvd/{sid}/bdj` | user | BDJ poll. |
| `GET /dvd/{sid}/bdj/frame/{n}` | user | BDJ frame. |
| `GET /bdj/status` | admin | BDJ status. |
| `GET /bluray/capabilities` | admin | What optional help WittyBluray has here: `libbluray` (`enabled`, `available`, `version`, `reason`) and `libblurayPath`. |


## Backup

The page is [Backup and restore](BACKUP.md). An archive is treated as hostile: a refused one answers with the reason in the job's `error`, and `refused` is true. While a server has no accounts, the routes marked for first-time setup follow the rule for making the first account: on the home network, or with the setup code in `X-Setup-Code`.

| Route | |
|---|---|
| `GET /backup` | What's kept, the schedule, what's running, and how the last restore went. An administrator's, or anyone allowed to set up a server with no accounts yet. |
| `GET /backup/estimate` | About how large each part of a backup is, before compression. |
| `POST /backup/create` | Start a backup: `{parts, password}`. With no password, secrets are left out. |
| `POST /backup/cancel` | Stop what's running. An administrator's, or first-time setup. |
| `PUT /backup/schedule` | Set the schedule: `{frequency, hour, weekday, keep, parts, password, clearPassword}`. Keeping fewer than before also takes `{current, code}`: the account's own password, and its two-factor code if it has one. |
| `DELETE /backup/files/{name}` | Delete a backup kept on this server. Takes `{password, code}`: the account's own password, and its two-factor code if it has one. |
| `POST /backup/files/{name}/link` | A short-lived address to download a backup. Takes `{password, code}`: the account's own password, and its two-factor code if it has one. |
| `GET /backup/download` | Download a backup, with the ticket from `/backup/files/{name}/link` as `?st=`. The ticket is the permission. |
| `POST /backup/restore/upload` | An archive as the body. It's kept under a new name, and nothing is done with it yet. An administrator's, or first-time setup. |
| `POST /backup/restore/inspect` | Check an archive from end to end: `{file, password}`. Nothing changes. An administrator's, or first-time setup. |
| `POST /backup/restore/start` | Bring a checked archive back: `{parts, confirm: "RESTORE", password, code}`. An administrator's password is asked for again; first-time setup has none to ask for. |
| `DELETE /backup/restore/request` | Take back a restore that's waiting for the next start. |
| `POST /backup/undo` | Put back what the last restore replaced: `{copy, password, code}`. The server restarts. |

---

<sub>WittyMediaServer documentation</sub>
