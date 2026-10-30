# Games: the API for clients

Everything a client needs to play a game: the Apple TV and Android TV apps, and anything else. The web
player uses exactly these routes and has no private ones. Conventions (sign-in, errors, versions) are those of
the client API, `docs/CLIENT-API.md`. Paths below are written without `/api`.

The server runs the game. A client shows a video stream, plays an audio stream and sends controller
states, so there is no emulator or game logic to write.

## Playing a game

1. List games with `GET /items?library=<id>&kind=game`. A library's `console` and an item's `console` say which
   console it is. A game without a picture answers 404 on its poster: draw a placeholder, as for any item.
2. `GET /games/consoles` says what each console has (`pad`) and whether it can be played now (`ready`,
   `unavailable`).
3. `GET /consoles/{console}/controls?device=<your device id>` gives the controller as data (below).
4. `GET /games/{id}/saves` lists saved states. If slot 0 exists, offer Resume.
5. `POST /games/{id}/play` with `{"resume": true}` or `{"resume": false}` starts it and answers the session,
   including `modes`.
6. Connect with the first mode in `modes` you can use. The game starts once a viewer is connected.
7. Send controller states. Show a menu of your own for saving, loading and quitting.
8. `DELETE /play/{sid}` saves and stops it. The answer comes once everything is written.

An account plays one game at a time: starting another saves and ends the first, on any device. A game
is stopped and saved after 60 seconds with nobody connected, and after 30 minutes with no controller
changes. The picture comes back if you reconnect within the first limit, so a brief network drop or a
trip through the app switcher loses nothing.

## Routes

| Route | Sign-in | What it does |
|---|---|---|
| `GET /games/consoles` | user | Every console, with `pad`, `ready`, `unavailable`, `cores` and `installed`. With `?library=<id>`, also `count`: that library's games for it |
| `GET /games/modes?client=native` | user | What this server can stream, best first |
| `GET /consoles/{console}/controls?device=` | user | The controller as data |
| `PUT /consoles/{console}/controls` | user | Saves one layout, or removes it |
| `POST /games/{id}/play` | user | Starts a game. Body: `{"resume": bool, "client": "native"}` |
| `GET /games/{id}/saves` | user | This account's saved states: `[{slot, updatedAt, size, hasThumb}]` |
| `GET /games/{id}/saves/{slot}/thumb` | user | A state's picture (PNG) |
| `DELETE /games/{id}/saves/{slot}` | user | Deletes a state |
| `GET /play/{sid}` | owner | The session |
| `POST /play/{sid}/webrtc` | owner | Connects over WebRTC. Body: `{"sdp": offer, "prefer": ""}`; answers `{"sdp": answer}` |
| `GET /play/{sid}/ws?video=h264` | owner | Connects over a WebSocket; `video` is `h264` or `jpeg` |
| `POST /play/{sid}/action` | owner | `{"type": "save" or "load", "slot": n}`, `{"type": "reset"}`, `{"type": "pause", "on": bool}` |
| `DELETE /play/{sid}` | owner | Saves and stops |
| `GET /gaming`, `POST /gaming/cores/{id}/install`, `DELETE /gaming/cores/{id}` | admin | Server setup. An app can leave this to the web interface |

A session belongs to the account that started it; to anyone else its routes answer 404.

Starting a game can answer:

| Status | Meaning |
|---|---|
| 201 | Started: the session |
| 404 | No such game for this account, or its file has gone |
| 409 | Can't be played yet. `needCore: true` with `console` means an admin has to install a core; otherwise `error` says why |
| 422 | The file can't be used |
| 503 | The server is running as many games as it allows, or its game runner is missing |

**The session** is `{id, itemId, title, console, core, width, height, aspect, fps, sampleRate, resumed, encoder, modes}`.
Show the picture at `aspect` (width over height), not at the video's own shape: the video is often
doubled, and consoles' pixels often aren't square.

**Save slots:** 0 is the automatic one, written when a game stops, and can only be loaded. 1 to 9 are manual.

## Modes

`GET /games/modes?client=native` and the session's `modes` list what you can use, best first. Use the first one
you support, and move to the next if it doesn't show a picture within about 12 seconds. Ignore ids you don't
know, because more may appear. A mode with `browserOnly` set is never listed for a native client, and leaving
out `client` counts as native.

| `id` | Video | Audio | Input | Needs |
|---|---|---|---|---|
| `webrtc` | `video`: `h264` (Constrained Baseline, `profile-level-id=42e01f`) and/or `vp8` | Opus | Data channel | A WebRTC stack: the Google library on Android, or WebRTC.framework on tvOS |
| `ws-h264` | H.264 Annex B | PCM | WebSocket | A decoder that takes raw frames, such as `MediaCodec` or `VTDecompressionSession` |
| `ws-jpeg` | One JPEG per frame | PCM | WebSocket | Only a JPEG decoder. Works when the server has no video encoder |

### WebRTC

Standard offer and answer with no trickle, over HTTPS or HTTP, using your sign-in:

1. Create a peer connection. Add **receive-only** video and audio transceivers.
2. Create the data channels: `input` (unordered, no retransmits, binary) and `events` (reliable, text).
3. Create an offer, set it, and wait for ICE gathering to finish (a second or two is enough).
4. `POST /play/{sid}/webrtc` with the SDP, then set the answer. The answer holds all of the server's
   candidates.

The server picks H.264 if your offer has it and VP8 otherwise. Send `"prefer": "vp8"` to ask for VP8. Ask your
stack for the smallest receive buffer it allows (`playoutDelayHint`, `jitterBufferTarget`, or the same on
your platform). Latency is the point.

Out of your home network, WebRTC needs a route to the server's UDP port. Admins set that up in Settings, Gaming.
If it fails, the WebSocket modes work anywhere HTTP does.

### WebSocket

Connect to `GET /play/{sid}/ws?video=h264` or `video=jpeg` with your sign-in in the `Authorization` header.
Browsers can't set that header and use `?token=`. The first message is text: `{"type":"hello","video":"h264"}`. It
says what you'll get, which may be `jpeg` if H.264 isn't available. After that:

Binary messages, whose first byte says what they are:

| First byte | Rest |
|---|---|
| 1 | H.264: one byte (1 for a keyframe, else 0), then one frame in Annex B with its parameter sets before each keyframe. The first frame is a keyframe |
| 2 | One JPEG |
| 3 | PCM: u32 little-endian sample rate, then signed 16-bit little-endian stereo. The rate may be anything a console uses, such as 32040 |

Text messages are events, as below. Send controller states as binary messages. If you fall behind, frames are
dropped, and you carry on from the next keyframe.

## Controller states

An 11-byte binary message, little-endian, on the `input` data channel or the WebSocket:

| Bytes | Field | Meaning |
|---|---|---|
| 0 | port | The player, 0 to 3. Others are ignored |
| 1-2 | buttons | u16, a bit per RetroPad button |
| 3-4, 5-6 | left stick | X, Y: i16, -32768 to 32767, with up negative |
| 7-8, 9-10 | right stick | X, Y |

| Bit | Button | Bit | Button |
|---|---|---|---|
| 0 | B (bottom) | 8 | A (right) |
| 1 | Y (left) | 9 | X (top) |
| 2 | Select | 10 | L |
| 3 | Start | 11 | R |
| 4 | Up | 12 | L2 |
| 5 | Down | 13 | R2 |
| 6 | Left | 14 | L3 |
| 7 | Right | 15 | R3 |

Send the whole state when anything changes, and again every 250 ms or so. A lost packet then corrects itself,
which is why the channel doesn't retransmit. Repeats don't count as playing. Release everything when your
app goes to the background or a menu opens.

## Events

JSON text on the `events` data channel or the WebSocket. Ignore types you don't know.

| `type` | Fields | What to do |
|---|---|---|
| `rumble` | `port`, `effect`, `strength` (0 to 65535) | Rumble that player's controller, if it can |
| `message` | `text` | Show it for a few seconds |
| `geometry` | `width`, `height`, `aspect` | Change the picture's shape |
| `rotation` | `turns` | Rotate the picture by this many quarter turns |
| `avinfo` | `width`, `height`, `aspect`, `fps`, `rate` | The game's timing changed |
| `shutdown` | | The game ended itself: leave the player |

## The controller as data

`GET /consoles/{console}/controls?device=<id>` answers:

```json
{
  "console": "snes",
  "pad": { "face": [{ "id": 1, "label": "Y" }], "shoulder": [], "meta": [], "sticks": false },
  "controls": [{ "key": "dpad", "id": -1, "label": "D-pad", "kind": "dpad" }, { "key": "b8", "id": 8, "label": "A", "kind": "face" }],
  "defaults": {
    "gamepad": { "8": { "b": 1 } },
    "keyboard": { "8": "KeyX" },
    "remote": { "8": "Enter" },
    "touch": { "landscape": { "dpad": { "x": 0.13, "y": 0.62, "size": 0.36, "opacity": 0.55 } }, "portrait": {} }
  },
  "saved": { "gamepad": { "Some Pad Model": { "8": { "b": 5 } } }, "keyboard": null, "touch": {} }
}
```

Numbers used as keys are RetroPad buttons, the bit numbers above. Labels are what that console calls them, so
draw `Y` on a Super Nintendo and `□` on a PlayStation from the same data.

- **Gamepads** map each button to `{"b": n}` (a button) or `{"a": n, "d": 1 or -1}` (one direction of an axis).
  The numbers are those of the [W3C standard gamepad](https://www.w3.org/TR/gamepad/#remapping), which is laid out by
  position, so translate from your platform's names. Bottom face button is 0, right 1, left 2, top 3, L1 4, R1 5,
  L2 6, R2 7, View/Select 8, Menu/Start 9, stick clicks 10 and 11, D-pad up, down, left, right 12 to 15. With no
  saved layout, the left stick steers the D-pad as well. Saved layouts are filed under the pad's model name.
- **Keyboards** map to `KeyboardEvent.code` names, such as `KeyX`.
- **Remotes** map to `KeyboardEvent.key` names. Translate from your platform:

  | Name | Means | tvOS | Android TV |
  |---|---|---|---|
  | `ArrowUp`, `ArrowDown`, `ArrowLeft`, `ArrowRight` | D-pad | Swipes or clickpad | `KEYCODE_DPAD_*` |
  | `Enter` | A | Select | `KEYCODE_DPAD_CENTER` |
  | `MediaPlayPause` | B | Play/Pause | `KEYCODE_MEDIA_PLAY_PAUSE` |
  | `MediaRewind` | Select | Not on all remotes | `KEYCODE_MEDIA_REWIND` |
  | `MediaFastForward` | Start | Not on all remotes | `KEYCODE_MEDIA_FAST_FORWARD` |

  **Back opens your game menu; it isn't a game button.** A remote can't reach every button, so the menu should
  offer "Press Start" and "Press Select", which send those bits for about 200 ms. Offer a gamepad for games that
  need more.
- **Touch** layouts place each control by its `key`: `x` and `y` are its centre, as fractions of the play area,
  and `size` is a fraction of the shorter side. A D-pad is a square of that size; `meta` buttons are about half as
  tall as wide.

**Saving:** `PUT /consoles/{console}/controls` with one of these. `"data": null` goes back to the default.

```json
{ "kind": "gamepad", "key": "Some Pad Model", "data": { "8": { "b": 5 } } }
{ "kind": "keyboard", "data": { "8": "KeyK" } }
{ "kind": "touch", "key": "my-device-id", "orientation": "landscape", "data": { "dpad": { "x": 0.2, "y": 0.6, "size": 0.3, "opacity": 0.6 } } }
```

Bad data answers 400 and saves nothing. Layouts follow the account to every device, except touch layouts, which
belong to the `device` that saved them. A device id is yours to choose: up to 64 letters, digits, `.`, `_` or `-`.

## What a TV app must build itself

- The player screen, showing the stream and playing the sound.
- Reading controllers and the remote, and turning them into the states above using the data above.
- The game menu: Resume, Press Start, Press Select, Save, Load, Reset, Quit. Focus handling is the platform's.
  Remember that Back goes up, and that the picture owns the D-pad until the menu is open.
- Choosing a mode from `modes`, and falling back.
- Stopping cleanly with `DELETE /play/{sid}`, including when the app leaves the screen for good.

**Moved to the server for this**, so apps don't have to do it: what every console's controller has, its names
for its buttons, default layouts, saved layouts, sessions, saves, resume and play time. Nothing about a game lives
only in the web player, except this device's own choices: which connection mode to force, and whether to show
touch controls.

**To be added when the shared contract lands:** these routes under `/api/v1`, entries in `docs/openapi.json` and
`docs/CLIENT-API.md`, a `games` feature in `/capabilities`, and a `game` case in `/items/{id}/playback` that points to
`POST /games/{id}/play`.
