# Remote Protocol

How one WittyMedia client plays on, and steers, another: a phone choosing what the TV plays, a tablet pausing the browser in the next room, a film handed from one screen to the next. This page is the whole wire format. A client written from it alone, in any language, works with every other.

- [The idea](#the-idea)
- [Transport](#transport)
- [Versions and unknown things](#versions-and-unknown-things)
- [Being a device](#being-a-device)
- [Being a controller](#being-a-controller)
- [Commands](#commands)
- [State](#state)
- [Life of a device and of a command](#life-of-a-device-and-of-a-command)
- [Who may control what](#who-may-control-what)
- [Limits](#limits)
- [Errors](#errors)
- [Settings](#settings)
- [A device in forty lines](#a-device-in-forty-lines)

For watching the same thing at the same moment on several screens, see [Watch parties](SYNCPLAY.md). For signing in and playing, see the [Client API](CLIENT-API.md).

## The idea

Everything goes through the server. No client ever connects to another, so it works the same at home and across the internet, and the server decides every time who may do what.

A client can take either part, or both at once:

- A **device** is a client that can be controlled. It announces itself, keeps one channel open to the server to receive commands, and reports what it is playing.
- A **controller** is a client that lists devices, watches their state and sends them commands.

The server keeps devices in memory only. Nothing about them is written to disk, and a restart forgets them: each device notices its channel has closed and announces itself again.

## Transport

Ordinary requests to the server's API under `/api/v1`, signed in the way every other request is (`Authorization: Bearer <token>`), with JSON bodies. The two streams are server-sent events (`text/event-stream`), the same as [`/system/tasks/stream`](TASKS.md#the-api): each event is an `event:` line, a `data:` line holding one JSON object, and a blank line. The server writes a comment line (`: still here`) every 25 seconds so that proxies keep the stream open; a client that has heard nothing for 60 seconds should take the stream to be dead and open it again.

A stream token (`st_…`) is for fetching media and is refused here. The device routes need a real sign-in, because a device belongs to its sign-in.

Away from home, use HTTPS as for everything else. Nothing in this protocol carries media: a device told to play something fetches it itself through [`POST /items/{id}/playback`](CLIENT-API.md#playing-something), so the server's limits on quality away from home apply to it as they always do.

## Versions and unknown things

Every message carries `v`, the protocol version. This is version `1`. `GET /remote-control` says which versions a server speaks:

```json
{ "enabled": true, "v": 1, "minV": 1, "controlOthers": false,
  "kinds": ["web", "tv", "phone", "tablet", "desktop", "other"],
  "features": ["seek", "volume", "queue", "audio", "subtitles", "navigate", "rate"],
  "mediaKinds": ["video", "audio", "dvd-iso", "dvd-folder", "bluray-iso", "bluray-folder", "cd-image", "book", "radio", "game", "photo"],
  "navigateKeys": ["up", "down", "left", "right", "select", "back", "home", "menu", "playpause"],
  "limits": { "devicesPerAccount": 16, "watchersPerAccount": 8, "commandQueue": 16, "stateBytes": 32768, "commandBytes": 8192,
              "stateEveryMs": 500, "commandEveryMs": 200, "heartbeatSec": 30, "idleSec": 75, "reconnectSec": 30,
              "ackTimeoutMs": 8000, "queueItems": 200 } }
```

The rules that let old and new clients work together:

- **Ignore what you don't know.** A field you've never heard of, an event name you've never heard of, a feature name in a list: skip it. Within a version, messages only ever gain fields.
- **A device announces the version it speaks.** A server that speaks a lower one answers with its own, and the device uses that from then on. A device older than `minV` is refused with code `version`, and should say it needs updating.
- **Capabilities are negotiated, not assumed.** A device lists what it can do. The server answers with the part of that list it knows, and never sends the device a command needing a feature outside it: the controller is told `unsupported` instead. A controller reads each device's `capabilities` and offers only what's there.
- **A device that receives a command type it doesn't know** answers it with `ok: false` and an `error`. It never ignores a command: every command gets an answer.

A server that has this at all lists `remote-protocol` in the `features` of [`GET /capabilities`](CLIENT-API.md#start-here).

## Being a device

### 1. Announce

`POST /remote-control/devices`

```json
{ "v": 1, "name": "Living room TV", "kind": "tv",
  "capabilities": { "mediaKinds": ["video", "audio"], "features": ["seek", "volume", "queue", "audio", "subtitles", "navigate"] } }
```

| Field | Meaning |
|---|---|
| `v` | The protocol version this client speaks |
| `name` | What people call it, up to 60 characters. Left out, the server names it after the app or browser |
| `kind` | `web`, `tv`, `phone`, `tablet`, `desktop` or `other`. Anything else becomes `other` |
| `capabilities.mediaKinds` | The kinds of item it plays: the `kind` of an item in the library |
| `capabilities.features` | What else it can do: see [commands](#commands) for which command needs which |
| `deviceId` | Only when coming back: the id it had before |

The answer is what was agreed:

```json
{ "v": 1, "minV": 1, "deviceId": "5b0c9d1e7a3f4c2288aa41d0", "name": "Living room TV", "kind": "tv",
  "capabilities": { "mediaKinds": ["video", "audio"], "features": ["seek", "volume", "queue", "audio", "subtitles", "navigate"] },
  "heartbeatSec": 30, "openWithinSec": 30, "serverTimeMs": 1794000000000 }
```

Keep `deviceId`. The device now has `openWithinSec` seconds to open its channel, or it is forgotten.

Announcing again with the same `deviceId`, from the same sign-in, updates the name and capabilities and keeps the id. An id that isn't this sign-in's, or that the server has forgotten, simply gets a new device with a new id: use whatever id comes back.

### 2. Open the channel

`GET /remote-control/devices/{deviceId}/channel` is the device's live channel. The first event is always `hello`:

```
event: hello
data: {"v":1,"deviceId":"5b0c9d1e7a3f4c2288aa41d0","capabilities":{"mediaKinds":["video","audio"],"features":["seek","volume","queue","audio","subtitles","navigate"]},"heartbeatSec":30,"serverTimeMs":1794000000000}
```

From then on the device is **online** and controllers can see it. Each thing it is asked to do arrives as a `command`:

```
event: command
data: {"v":1,"id":"c41f0a9e2b7d4e118c35a0f2","type":"seek","bySec":-10,"sender":{"userName":"Sam"},"sentAt":1794000004200}
```

When the server ends the channel on purpose it says why first:

```
event: bye
data: {"reason":"replaced"}
```

| `reason` | What happened | What to do |
|---|---|---|
| `replaced` | The same device opened another channel | Nothing: the newer channel is the one |
| `removed` | The device said goodbye | Nothing |
| `idle` | No state was reported for `idleSec` | Announce again |
| `signed-out` | The sign-in ended, or remote control was turned off | Stop |
| `server-stopping` | The server is shutting down or restarting | Try again after a few seconds, announcing again |

One device has one channel. Opening a second for the same `deviceId` ends the first.

### 3. Answer every command

`POST /remote-control/devices/{deviceId}/ack`

```json
{ "id": "c41f0a9e2b7d4e118c35a0f2", "ok": true }
```

or, when it can't:

```json
{ "id": "c41f0a9e2b7d4e118c35a0f2", "ok": false, "error": "Nothing is playing." }
```

`error` is shown to the person holding the controller, so write it for them: a short sentence, up to 200 characters. Answer when the command has been **accepted**, not when it has finished: for `play`, when the player has been told to open the item, not when the first frame shows. The state reports say how it's going after that. An answer must arrive within `ackTimeoutMs` (8 seconds), or the controller is told the device didn't answer.

A `404` with code `no-command` means the command is no longer waiting: it timed out, or the channel was reopened meanwhile. Carry on.

### 4. Report state

`POST /remote-control/devices/{deviceId}/state` with a [state](#state). Send one:

- whenever something changes that a controller would show: play, pause, a seek, another item, another track, the volume;
- every 5 seconds or so while playing, so controllers stay close;
- at least every `heartbeatSec` (30 seconds) whatever happens, playing or not. **This is the heartbeat.** A device whose channel is open but which has reported nothing for `idleSec` (75 seconds) is taken to have gone.

Reports closer together than `stateEveryMs` are refused with `429` after a short burst. Nothing is lost by that: send the latest state a moment later.

### 5. Coming back, and leaving

If the channel drops, the device is **offline** at once: commands to it fail straight away with code `offline`. It stays listed, greyed, for `reconnectSec` (30 seconds). Opening the channel again within that time, with the same `deviceId`, brings it back as the same device. After that, announce again (sending the old `deviceId` does no harm).

`DELETE /remote-control/devices/{deviceId}` when the app closes, or the person turns this off.

## Being a controller

### List devices

`GET /remote-control/devices`

```json
{ "serverTimeMs": 1794000010000,
  "devices": [
    { "id": "5b0c9d1e7a3f4c2288aa41d0", "name": "Living room TV", "kind": "tv", "userId": 2, "userName": "Sam",
      "v": 1, "capabilities": { "mediaKinds": ["video", "audio"], "features": ["seek", "volume", "queue"] },
      "online": true, "mine": true, "thisSession": false,
      "state": { "v": 1, "status": "playing", "itemId": 812, "positionSec": 1312.4, "durationSec": 5400, "rate": 1, "volume": 0.6, "muted": false,
                 "audio": "1", "subtitle": "off", "audioTracks": [{ "id": "1", "label": "English · 5.1", "lang": "en" }] },
      "stateAt": 1794000008200, "title": "Puddle Day", "itemKind": "video" } ] }
```

| Field | Meaning |
|---|---|
| `online` | Its channel is open. Commands only reach a device that is online |
| `mine` | It is signed in as your account |
| `thisSession` | It was announced by the very sign-in asking: this is how an app finds itself in the list, to leave itself out |
| `state`, `stateAt` | What it last reported, and when, in the server's milliseconds |
| `title`, `itemKind` | What it is playing, when you may see that |
| `hidden` | It is playing something your account may not see. `state` then holds only `status`, `volume`, `muted` and `screen` |

Where a playing device is **now**: `positionSec + (serverNow - stateAt) / 1000 × rate`, where `serverNow` is your clock corrected by the difference between `serverTimeMs` and your clock when the answer arrived. Half a second out is fine for a progress bar.

### Watch them live

`GET /remote-control/events` is a stream for controllers:

| Event | Data | Meaning |
|---|---|---|
| `snapshot` | `{ "devices": [...], "serverTimeMs": ... }` | Everything you may control, as in the list. Sent first, and again every 25 seconds: replace what you had |
| `device` | one device | It appeared, or something about it changed |
| `gone` | `{ "id": "..." }` | It left, or you may no longer control it |

A last event, `bye` with `{ "reason": ... }`, says why the server is ending the stream: `signed-out` (the sign-in ended), `feature-off` (remote control was turned off, for the server or for this profile) or `server-stopping`. After `signed-out` or `feature-off`, don't reconnect.

Each `device` event is whole, never a difference. The server sends at most a few a second, and a controller that reads slowly gets fewer of them, each the latest: it never falls behind into a backlog. Keep this stream open only while a list of devices or a remote is on screen.

### Send a command

`POST /remote-control/devices/{deviceId}/commands` with a [command](#commands). The request waits for the device's answer, so allow it ten seconds:

```json
{ "ok": true }
```

`200` means the device said it has done it. Anything else is an [error](#errors) with a `code`, and nothing happened on the device.

## Commands

A command is one JSON object with `v` and `type`. The server adds `id`, `sender` and `sentAt` on the way to the device, and ignores those if a controller sends them.

| `type` | Fields | Needs | What the device does |
|---|---|---|---|
| `play` | `itemId`, or `queue` with `queueIndex`; optional `positionSec`, `audio`, `subtitle` | the item's kind in `mediaKinds`; `queue` for a queue of more than one | Opens the item and plays it from `positionSec` (from the account's own resume point when left out) |
| `pause` | | | Pauses |
| `resume` | | | Plays again from where it paused |
| `stop` | | | Stops and leaves the player |
| `seek` | `positionSec` (from the start), or `bySec` (from here, negative to go back) | `seek` | Moves there |
| `next`, `previous` | | `queue` | Moves along its queue |
| `volume` | `level`, from 0 to 1 | `volume` | Sets its volume |
| `mute` | `muted` | `volume` | Mutes or unmutes |
| `audio` | `audio`: a track id | `audio` | Switches sound track |
| `subtitle` | `subtitle`: a track id, or `"off"` | `subtitles` | Switches subtitles |
| `rate` | `rate`, from 0.25 to 4 | `rate` | Sets the playback speed |
| `navigate` | `key`: `up`, `down`, `left`, `right`, `select`, `back`, `home`, `menu` or `playpause` | `navigate` | Presses that key in its interface, as a TV remote would |
| `cast` | `from`: another device's id | as `play` | Controllers only: see [below](#cast) |

Examples, as a controller sends them:

```json
{ "v": 1, "type": "play", "itemId": 812, "positionSec": 0 }
{ "v": 1, "type": "play", "queue": [812, 813, 814], "queueIndex": 0, "audio": "2", "subtitle": "off" }
{ "v": 1, "type": "pause" }
{ "v": 1, "type": "resume" }
{ "v": 1, "type": "stop" }
{ "v": 1, "type": "seek", "positionSec": 754.5 }
{ "v": 1, "type": "seek", "bySec": -10 }
{ "v": 1, "type": "next" }
{ "v": 1, "type": "previous" }
{ "v": 1, "type": "volume", "level": 0.4 }
{ "v": 1, "type": "mute", "muted": true }
{ "v": 1, "type": "audio", "audio": "2" }
{ "v": 1, "type": "subtitle", "subtitle": "off" }
{ "v": 1, "type": "rate", "rate": 1.25 }
{ "v": 1, "type": "navigate", "key": "select" }
{ "v": 1, "type": "cast", "from": "9e2d7c11aa0b4f6e83c1d5a7" }
```

And as the device receives one:

```json
{ "v": 1, "id": "c41f0a9e2b7d4e118c35a0f2", "type": "play", "itemId": 812, "positionSec": 0,
  "sender": { "userName": "Sam" }, "sentAt": 1794000004200 }
```

**Queues.** With `queue`, `queueIndex` says which entry plays first, and `itemId`, if given too, must be that entry. A queue holds up to 200 items. The device keeps the queue and reports it in its state; `next` and `previous` move along it.

**Track ids** are the ids the device lists in its own state (`audioTracks`, `subtitleTracks`), as text. So that a controller can choose a track before anything is playing, a device that plays through [`POST /items/{id}/playback`](CLIENT-API.md#playing-something) uses the `index` of that answer's `audio` and `subtitles` entries, written as text (`"1"`), and `"off"` for no subtitles.

**`play` is checked by the server** before the device hears of it: see [who may control what](#who-may-control-what). A device may trust that an item it is told to play is one its own account may see. It still plays it through the ordinary playback routes, which check again.

### Cast

Casting hands over what one device is playing to another, at the same place. A controller sends `cast` to the device that should take over, naming the device to take over from:

```
POST /remote-control/devices/{target}/commands
{ "v": 1, "type": "cast", "from": "<source device id>" }
```

The server reads the source's last state and works out where it has got to since. It sends the target a `play` for the same item (and queue, if the target has `queue`), at that position, with the same sound track and subtitles. When the target has accepted, the server sends the source a `stop`. The answer says how that went:

```json
{ "ok": true, "sourceStopped": true }
```

`sourceStopped: false` means the new device is playing and the old one didn't stop when asked. The devices never see `cast`: one gets a `play`, the other a `stop`.

A client that wants to hand over **its own** playback sends `cast` with its own `deviceId` as `from`. A client that isn't announced as a device sends an ordinary `play` with `positionSec`, and stops itself when that succeeds.

## State

What a device reports, and what controllers are shown:

```json
{ "v": 1, "status": "playing", "itemId": 812, "positionSec": 1312.4, "durationSec": 5400, "rate": 1,
  "volume": 0.6, "muted": false, "audio": "1", "subtitle": "off",
  "audioTracks": [{ "id": "1", "label": "English · 5.1", "lang": "en" }, { "id": "2", "label": "Commentary", "lang": "en" }],
  "subtitleTracks": [{ "id": "3", "label": "English", "lang": "en" }],
  "queue": [812, 813, 814], "queueIndex": 0, "screen": "player" }
```

| Field | Meaning |
|---|---|
| `status` | `idle` (nothing open), `playing`, `paused` or `buffering` |
| `itemId` | What is open. Leave it out when idle |
| `positionSec`, `durationSec` | Where it is and how long the item is, in seconds |
| `rate` | Playback speed: 1 is normal |
| `volume`, `muted` | 0 to 1. Leave `volume` out if the device has no volume of its own |
| `audio`, `subtitle` | The ids of the tracks in use. `"off"` for no subtitles |
| `audioTracks`, `subtitleTracks` | What can be chosen: `id`, and `label` and `lang` for showing. Up to 64 each |
| `queue`, `queueIndex` | Its queue, if it has one |
| `screen` | Where the device is in its own interface, in a word (`home`, `player`): for a remote that shows it |

An idle device sends `{ "v": 1, "status": "idle" }`, with `volume` and `screen` if it likes. That is also the heartbeat.

The server tidies what it's sent rather than refusing it: a status it doesn't know becomes `idle`, numbers that aren't finite become 0, long text is cut, long lists are cut. A report over `stateBytes` (32 KB) is refused.

## Life of a device and of a command

A device:

| State | How it gets there | What controllers see |
|---|---|---|
| Announced | `POST /remote-control/devices` | Listed, `online: false` |
| Online | Its channel opens | `online: true`. Commands are delivered |
| Offline | Its channel closes, for any reason | `online: false` at once. Commands fail with `offline` |
| Gone | It says goodbye; or 30 seconds pass announced or offline with no channel; or 75 seconds pass online with no state report; or its sign-in ends | A `gone` event |

A command:

| Step | What happens |
|---|---|
| Checked | Size, shape, values in range, the caller's rate. Then whether the caller may control this device, whether the device is online, whether it has the feature, and for `play` whose limits allow the items. A failure here answers at once, and the device never hears of it |
| Queued | Placed on the device's channel. Up to 16 can wait. One more is refused at once with `busy`: **a command is never dropped silently** |
| Delivered | Written to the device as a `command` event |
| Answered | The device posts its `ack`. The controller's request returns `200`, or `refused` with the device's reason |
| Timed out | No answer within 8 seconds: the controller is told `timeout`. The device may still act on it; its state reports say what really happened |
| Failed | The channel closed or was reopened first: every command still waiting fails at once with `offline` |

State updates are different on purpose. They are **latest-wins**: a controller that can't keep up is sent fewer of them, each complete, and nothing waits on it.

## Who may control what

- **A device belongs to the sign-in that announced it.** Only that sign-in opens its channel, reports its state, answers its commands or removes it. Any other sign-in, even of the same account, even an administrator's, gets `404` for those.
- **An account controls its own devices**: every device signed in as that account.
- **Other accounts' devices are off limits by default**, to see or to control, for administrators too. An administrator can allow particular accounts to control devices signed in as *any* account: in the web app under **Settings → Sharing & photos → Play on other devices**, or with `PUT /remote-control/accounts/{id}`. This is for households: a parent's phone steering the TV that is signed in as a Kids profile.
- **Limits are never widened.** A `play` (or `cast`) is refused unless every item named is one the **caller** may see *and* one the **device's own account** may see. A Kids profile allowed to control the living room TV can't put on a film it couldn't open itself: to it the item doesn't exist (`404`, code `no-item`). A parent can't make a Kids profile's tablet play what that profile is limited from: `403`, code `not-allowed-there`. Queues are checked item by item, and one refusal refuses the whole command.
- **What a device is playing is shown only to those who may see it.** A controller whose account may not see the item gets `hidden: true` and a state with no item, position, tracks or queue. Entries in a queue that the controller may not see are sent as `0`.
- **A sign-in that ends takes its device with it, at once.** Signing out, switching to another profile on that device, being signed out from another device or by an administrator, a new password, the account being removed: the channel gets `bye` with `reason: "signed-out"`, the device leaves every controller's list, and a controller's own stream from that sign-in gets `bye` too. An app that switches profile announces itself again with its new sign-in, and is then a new device of the new profile.
- **A change to who may control what shows at once.** When an administrator gives or takes away an account's leave to control other accounts' devices, or anyone's limits change, open event streams are sent a fresh `snapshot` straight away.
- **Everything is looked at again as it runs** as well: a stream checks its sign-in and the server's switch every 25 seconds, which catches a sign-in that simply ran out.
- **A stream token is refused**, since it is for media and not for acting as a person.

## Parental controls

Profiles can have limits ([PROFILES.md](PROFILES.md)). Here is what they mean for remote control. The rule throughout: **the limits of the account a device is signed in as decide what plays on it, and the limits of the caller decide what the caller may ask for.** Both are checked on the server for every command; neither side's app is trusted with it.

- **Every kind of limit on titles counts**, on both sides: libraries, an age for each kind of media, genres hidden or the only ones allowed, tags, collections, single titles, and games turned off. It's the same check the item routes make.
- **Screen time.** `play`, `cast`, `resume`, `next` and `previous` start something, so they're refused when time is up: `403` with code `screen-time` when it's the caller's time, and `screen-time-there` when it's the time of the profile the device is signed in as. That holds for every caller, an administrator included. `pause`, `stop`, `seek`, volume, tracks and `navigate` are never refused for time. What a device plays is fetched by the device with its own sign-in, so it counts as its own profile's screen time whoever started it, and stops when that time runs out.
- **The "Playing on other devices" switch** (`remoteControl` in a profile's limits). With it off, the controller's routes (`GET /remote-control/devices`, `GET /remote-control/events`, `POST …/commands`) answer `403` with `code: "feature-off"` and `feature: "remoteControl"`, for the profile's own devices too. The device's routes are not affected: the profile's devices announce themselves, report and obey as before, so a parent with leave can still play on and pause a child's TV. `GET /remote-control` stays open and says `canControl: false`; an app hides its remote and its **Play on…** then, and still announces itself.
- **What a device is playing** is hidden from a controller who may not see it by any of those limits (`hidden: true`).
- **Turning the switch off** while a controller's stream is open ends it at once with `bye`, `reason: "feature-off"`.

## Limits

Chosen so that a small server stays small. `GET /remote-control` reports them, so a client needn't copy the numbers.

| What | Limit | When it's reached |
|---|---|---|
| Devices announced by one account | 16 | `429`, code `too-many` |
| Devices in all | 128 | `429`, code `too-many` |
| Controllers watching `events`, one account | 8 | `429`, code `too-many` |
| Controllers watching, in all | 64 | `429`, code `too-many` |
| Open streams of both kinds, one account | 16 | `429`, code `too-many` |
| Commands waiting for one device | 16 | `503`, code `busy`, at once |
| A command or an announcement | 8 KB | `413` |
| A state report | 32 KB | `413` |
| State reports from one device | one every 500 ms, after a burst of 6 | `429`, code `rate` |
| Commands from one account | one every 200 ms, after a burst of 20 | `429`, code `rate` |
| A queue | 200 items | `400`, code `bad-command` |
| Waiting for a device to answer | 8 seconds | `504`, code `timeout` |
| A device with no channel | 30 seconds | Forgotten |
| An online device reporting nothing | 75 seconds | `bye` with `idle`, and forgotten |
| Keep-alive on streams | every 25 seconds | |

Each open channel costs the server one connection, one small queue and a timer. A device that isn't being controlled costs nothing more than its heartbeat.

## Errors

Every refusal is `{ "error": "<a sentence fit to show>", "code": "<for programs>" }`.

| Status | `code` | Meaning |
|---|---|---|
| 400 | `bad-command` | Unknown `type`, a missing field, a value out of range |
| 400 | `version` | This app speaks a protocol older than the server accepts |
| 403 | `disabled` | Remote control is turned off on this server |
| 403 | `no-session` | A device route was called with something that isn't a sign-in |
| 403 | `not-allowed-there` | The account that device is signed in as may not play that |
| 403 | `screen-time` | The caller's screen time is over: nothing can be started. The answer has `screenTime`, as from a playback route |
| 403 | `screen-time-there` | Screen time is over for the profile the device is signed in as |
| 403 | `feature-off` | Playing on other devices is turned off for this profile (`feature: "remoteControl"`) |
| 404 | `no-device` | No such device, or not one you may control, or (on device routes) not yours |
| 404 | `no-item` | No such item, or not one you may see |
| 404 | `no-command` | The command being answered is no longer waiting |
| 409 | `offline` | The device isn't connected |
| 409 | `unsupported` | The device never said it can do that, or doesn't play that kind of thing |
| 409 | `refused` | The device answered `ok: false`; `error` is its reason |
| 409 | `nothing-playing` | `cast` from a device that is idle |
| 413 | | The message is too long |
| 429 | `rate`, `too-many` | Slow down (`Retry-After` says how long), or too many devices or streams |
| 503 | `busy` | Too many commands are already waiting for that device |
| 504 | `timeout` | The device didn't answer in time |

## Settings

| Setting | What it does | Default |
|---|---|---|
| `remoteControl` | The owner's switch for all of this. Off: every route here answers `403` with code `disabled`, open channels end, and `GET /remote-control` says `enabled: false` | On |
| Control of other accounts' devices | Per account, set by an administrator with `PUT /remote-control/accounts/{id}` `{ "controlOthers": true }`. `GET /remote-control/accounts` lists every account with its setting | Off for everyone |

## A device in forty lines

The whole of a device, in JavaScript, for a player object with `play`, `pause` and so on:

```js
const api = (path, method = 'GET', body) => fetch(server + '/api/v1' + path, {
  method, headers: { Authorization: 'Bearer ' + token, 'Content-Type': 'application/json' },
  body: body && JSON.stringify(body) })

const caps = { mediaKinds: ['video'], features: ['seek', 'volume'] }
let id = null

async function run() {
  const a = await (await api('/remote-control/devices', 'POST', { v: 1, name: 'Den', kind: 'tv', capabilities: caps, deviceId: id })).json()
  id = a.deviceId
  const beat = setInterval(report, 25000)
  try {
    const res = await api(`/remote-control/devices/${id}/channel`)
    const reader = res.body.pipeThrough(new TextDecoderStream()).getReader()
    for (let buf = ''; ;) {
      const { value, done } = await reader.read()
      if (done) break
      buf += value
      for (let end; (end = buf.indexOf('\n\n')) >= 0; buf = buf.slice(end + 2)) {
        const frame = buf.slice(0, end)
        const event = /^event: (.*)$/m.exec(frame)?.[1], data = /^data: (.*)$/m.exec(frame)?.[1]
        if (event === 'command') obey(JSON.parse(data))
        if (event === 'bye' && JSON.parse(data).reason === 'signed-out') return
      }
    }
  } finally { clearInterval(beat) }
  setTimeout(run, 3000) // the channel dropped: come back as the same device
}

async function obey(cmd) {
  let ok = true, error
  try {
    if (cmd.type === 'play') await player.open(cmd.itemId, cmd.positionSec)
    else if (cmd.type === 'pause') player.pause()
    else if (cmd.type === 'resume') player.resume()
    else if (cmd.type === 'stop') player.close()
    else if (cmd.type === 'seek') player.seek(cmd.positionSec ?? player.position + cmd.bySec)
    else if (cmd.type === 'volume') player.volume = cmd.level
    else if (cmd.type === 'mute') player.muted = cmd.muted
    else throw new Error('This device can’t do that.')
  } catch (e) { ok = false; error = e.message }
  await api(`/remote-control/devices/${id}/ack`, 'POST', { id: cmd.id, ok, error })
  report()
}

const report = () => api(`/remote-control/devices/${id}/state`, 'POST', player.item
  ? { v: 1, status: player.paused ? 'paused' : 'playing', itemId: player.item, positionSec: player.position, durationSec: player.duration, rate: 1, volume: player.volume, muted: player.muted }
  : { v: 1, status: 'idle' })
```

---

<sub>WittyMediaServer documentation</sub>
