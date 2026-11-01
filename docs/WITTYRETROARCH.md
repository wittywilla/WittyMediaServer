# WittyRetroArch

For developers: what a RetroArch fork would have to do to run games for this server. It isn't built, and no console needs it. To play games, see [Games](GAMES.md).

## What it would be

A **runner** is the process that runs one game. The server talks to it through a documented protocol over pipes, so any program that speaks it can be a runner. The built-in one, `wittyplay-game-runner`, runs every console, including those drawn on a graphics card. [The design](GAMING.md#two-runners-one-protocol) compares the two.

The server looks for a program named `wittyretroarch` (`gameRunner` in `internal/server/api_games.go`). **Game runner** in **Settings → Consoles & cores → How games play → Advanced** chooses between **Automatic**, **Built in** and **A separate RetroArch-based runner, when installed**. Automatic uses it if it's found. `WMS_WITTYPLAY_RUNNER` names a runner directly and beats that setting.

Unmodified RetroArch can't fill this role: its only way to hand frames to another program is its recording feature, which adds several frames of delay. A fork would add Vulkan and RetroArch's own shaders, if something comes to need them. It would cost a C codebase to build for every platform, and frequent merges from upstream.

## How it stays a separate program

RetroArch is GPLv3, and WittyMediaServer and WittyPlay are Apache-2.0. Keep it that way with these rules:

- Only messages on a pipe. No shared memory, no linking, and no loading one into the other. The protocol is the whole of what they share.
- Never link RetroArch code into `wittymediaserver`, `wittyplay-module` or `wittyplay-game-runner`. Write the fork's side of the protocol from this page, not from Apache-licensed code.
- The server keeps working without the fork.
- Cores are never bundled with it, as they aren't with anything else.

A release carrying the fork must put the exact matching source beside the binary, keep every copyright and licence notice along with `COPYING`, say that the program is modified and when, show its legal notices, add no further restrictions, and be listed in `NOTICE` and `THIRD_PARTY_LICENSES.md` as a separate GPLv3 program. RetroArch's credits stay in. It can't go inside an iOS or tvOS app.

## What the fork adds

One mode, in which it has no window and no menus, and speaks the protocol on stdin and stdout:

```
wittyretroarch --wittyplay -save <dir> -memory-mb <n> [-gpu] [-normal-priority]
```

`-save` is the only folder it may write to, and `-memory-mb` its memory limit. `-gpu` says the game draws with a graphics card, and `-normal-priority` that it can't play at low priority. `-probe-gpu` asks which card games would draw with: the built-in runner prints that and stops.

| RetroArch part | What the fork adds |
|---|---|
| Video | A driver sending `MsgVideo`. For hardware-rendered cores, a headless GL context and a read-back of each frame |
| Audio | A driver sending `MsgAudio`, with no local sound device |
| Input | A driver reading `CmdInput` and `CmdPointer` from the pipe |
| Main loop | Handling every command below |
| Network | Built without netplay, the updater, cloud sync and the HTTP stack, so that a sandboxed process has nothing to reach for |

## The protocol

Each message is one byte of type, a 4-byte little-endian payload length, then the payload. An unknown type or an oversized message ends the connection. The limits are in `ipc/ipc.go` in WittyPlay, which is the authority on this table: everything a runner says is treated as hostile.

**Server to runner**

| Type | Name | Payload |
|---|---|---|
| 64 | `CmdLoad` | JSON: `{"core","rom","system","save","options","preferDevices","assets","probe"}`. Always first. With `"probe": true`, send `options` with no game loaded, then exit |
| 65 | `CmdSaveRAM` | Battery save to restore |
| 66 | `CmdState` | Save state to restore |
| 67 | `CmdInput` | 11, 13 or 17 bytes: port (u8), buttons (u16, a bit per RetroPad id), left X, left Y, right X, right Y (i16), then optionally L2, R2 (u8), then optionally a u32 naming this state. All little-endian |
| 68 | `CmdSaveState` | u32 request id |
| 69 | `CmdReset` | None |
| 70 | `CmdPause` | One byte: 1 paused, 0 running. A runner starts paused |
| 71 | `CmdQuit` | One byte: 1 sends a save state first. The battery save is always sent |
| 72 | `CmdDisc` | One byte: the disc to change to, from 0 |
| 73 | `CmdPointer` | 6 bytes: 255, flags (bit 0 pressed), X, Y (i16, -32767 to 32767 across the picture) |
| 80 | `CmdDevice` | 5 bytes: port (u8), then a u32 device type |
| 90 | `CmdProbe` | JSON, as `CmdLoad`. Test-loads the core, answers `MsgProbe` and exits |
| 96 | `CmdControl` | JSON: `{"speed","frameSkip","rewindBytes","rewindSeconds","rewinding","runAhead","advance","noEarlyInput","quietAhead","shiftMs"}`. Only what's present changes. `speed` 1 is normal, from 0.1 to 16. `frameSkip` -1 is automatic, 0 none, or how many to leave out after each one sent. `rewindBytes` is the memory rewind may hold, 0 for off. `runAhead` is frames, up to 3. `advance` runs that many frames while paused |
| 97 | `CmdOption` | JSON: `{"key","value"}`. A key the core didn't offer or a value it didn't list is ignored |
| 98 | `CmdCheats` | JSON: `[{"code","enabled"}]`. The whole list, replacing what was there |

A runner that ignores `CmdControl`, `CmdOption` and `CmdCheats` still works: those features do nothing.

**Runner to server**

| Type | Name | Payload |
|---|---|---|
| 1 | `MsgReady` | JSON: `{"name","version","width","height","aspect","fps","sampleRate","disc","discs","renderer","devices"}` |
| 2 | `MsgVideo` | 5 bytes: u16 width, u16 height, u8 format (0 0RGB1555, 1 XRGB8888, 2 RGB565), then tightly packed rows |
| 3 | `MsgDupe` | None: the previous frame again |
| 4 | `MsgAudio` | Signed 16-bit little-endian stereo |
| 5 | `MsgState` | u32 request id (0 when sent on quit), then the state; nothing after the id if there is none |
| 6 | `MsgSaveRAM` | Battery save |
| 7 | `MsgEvent` | JSON with a `type`: `rumble` (`port`, `effect`, `strength`), `message`, `geometry`, `rotation`, `avinfo`, `log`, `shutdown`, `statefail` (a state wouldn't load), `disc`, `options` (`"options": [{"key","label","info","category","choices","labels","default","value","hidden"}]`, of which `key`, `label`, `choices` and `value` are required, sent before `MsgReady` and after a change), `stats` (once a second while running) |
| 8 | `MsgError` | Text, then the runner exits |
| 9 | `MsgTiming` | 32 bytes: when the next picture was made. Optional, sent just before it |
| 20 | `MsgProbe` | JSON: `{"name","version","extensions","needFullPath","blockExtract","noGame","hwRender","options"}`, the answer to `CmdProbe` |

---

<sub>WittyMediaServer documentation</sub>
