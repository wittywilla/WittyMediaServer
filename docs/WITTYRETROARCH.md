# Using RetroArch with WittyMediaServer: licensing and WittyRetroArch

This page answers two questions: how a GPL program like RetroArch can work with Apache-licensed
WittyMediaServer, and what a fork named **WittyRetroArch** would need.

This is an engineer's reading of the licences, not legal advice. Before selling anything or
shipping at scale, have a lawyer check it. The whole page is a good thing to hand them.

## The short version

- **GPL and Apache can ship side by side.** They just can't be built into one program in the
  Apache direction. Keep RetroArch a separate program that talks to the server over pipes, and the
  server stays Apache-2.0. This is how the release already ships GPL FFmpeg.
- **The fork stays GPLv3, for good.** Everything in WittyRetroArch is GPLv3, including your own
  changes, and anyone who gets the program must be able to get its source.
- **RetroArch's credits can't be removed.** GPLv3 requires keeping the authors' copyright and
  licence notices.
- **The fork doesn't have to credit you.** Your own additions can be plain GPLv3 with no attribution
  term. The Apache `NOTICE` requirement only applies to Apache code, so don't copy any into the
  fork. Write its side of the protocol from the specification below.

## Why this works

| Rule | Where it comes from | What it means here |
|---|---|---|
| Separate programs shipped together don't affect each other's licences | GPLv3 section 5, "aggregate" | The archive can hold `wms` (Apache) and `wittyretroarch` (GPLv3) |
| Pipes, sockets and command lines are how separate programs normally talk | FSF's GPL FAQ, "mere aggregation" | The protocol in `modules/wittyplay/ipc` keeps them separate |
| Apache-2.0 code can go into a GPLv3 program, but not the other way | FSF's licence list | Never link RetroArch code into `wms` or `wittyplay-host` |
| Running a program is unrestricted; the conditions apply when you give it to others | GPLv3 sections 2 and 5 | A user combining things on their own server owes nothing |

To stay on the safe side of "separate programs":

- No shared memory structures, no linking, no loading one into the other. Only messages on a pipe.
- The server must keep working without the fork. It does: the built-in runner is the default, and
  the fork is used when it's found.
- The protocol stays documented and general. Anything speaking it can be a runner.

## What shipping the fork requires

Every release that includes a WittyRetroArch binary must:

1. **Offer the exact source.** Put the matching source archive next to the binary on the release
   page, including your changes and the build scripts (GPLv3 section 6). A link to a branch isn't
   enough on its own, because it has to match the exact build.
2. **Keep every notice.** Leave all copyright and licence headers in place, and include the `COPYING`
   file.
3. **Mark your changes.** Say the program is modified, and when (section 5a). A line in the README
   and in `--version` does it.
4. **Show legal notices.** A program with a user interface must display them (section 5d). The server
   can do this on its About page.
5. **List it in `NOTICE` and `THIRD_PARTY_LICENSES.md`**, the same way FFmpeg is listed, saying it is
   GPLv3 and a separate program.
6. **Add no extra restrictions.** No terms can be placed on the fork beyond the GPL (section 10).

The Windows, macOS and Linux installers can carry it in `bin/`, like FFmpeg.

## Things to watch

**The name.** The GPL gives rights to the code, not to a name. "RetroArch" belongs to its project, and
a fork using it can look like an official release. Before publishing under "WittyRetroArch", ask the
libretro team or have a lawyer check it. If there's any doubt, the name is easy to change: the server
only looks for a program name, in one place (`gameRunner` in `internal/server/api_games.go`).

**Cores.** These matter more than RetroArch:

- Never bundle cores with the release, even GPL ones. Some are GPLv2-only, which is not compatible
  with GPLv3, so shipping them inside a GPLv3 product is a real problem. Downloading on an admin's
  request keeps each core a separate choice made on their own machine.
- Non-commercial cores (`snes9x`, `genesis_plus_gx`, `picodrive`, `fbneo`, `mame2003*`) are never
  offered. If WittyMediaServer is ever sold, or run as a paid service, these are the first thing to
  check.
- The built-in runner loads cores into an Apache-licensed process. Nothing is distributed in that
  combined form: it only exists on the machine of the person who chose to install the core. That's
  the usual position for libretro frontends, but it's the part most worth a lawyer's look.

**App stores.** GPLv3 is widely considered incompatible with Apple's App Store terms. This doesn't
affect a server, but WittyRetroArch can't go inside an iOS app. Clients only need a browser.

## What the fork adds

RetroArch can't do this job out of the box. It has no way to hand frames straight to another program,
and its recording path adds several frames of delay. The fork adds one mode:

```
wittyretroarch --wittyplay -save <dir> -memory-mb <n>
```

In that mode it has no window and no menus, and it speaks the protocol on stdin and stdout, exactly as
`wittyplay-host` does. The server needs no other changes.

| RetroArch part | What the fork adds |
|---|---|
| Video | A `wittyplay` video driver sending `MsgVideo`. For hardware-rendered cores, a headless GL context (EGL surfaceless on Linux, a hidden window elsewhere) and a read-back of each frame |
| Audio | A `wittyplay` audio driver sending `MsgAudio`, with no local sound device |
| Input | A `wittyplay` input driver reading `CmdInput` from the pipe |
| Main loop | Handling `CmdLoad`, `CmdSaveRAM`, `CmdState`, `CmdSaveState`, `CmdReset`, `CmdPause` and `CmdQuit` |
| Network | Built without netplay, the updater, cloud sync and the HTTP stack, so that a sandboxed process has nothing to reach for |

**What this buys:** little, now that the built-in runner plays consoles that draw with a graphics card
([GAMING.md](GAMING.md)). It would add Vulkan and RetroArch's own shaders, if something comes to need them.

**What it costs:** a C codebase of its own to build for every platform, and frequent merges from
upstream. The built-in runner stays as the small default that works everywhere, including servers with
no GPU, and it's what the tests use.

## The protocol

Everything a runner needs. Each message is one byte of type, a 4-byte little-endian payload length, then
the payload. The limits are in `modules/wittyplay/ipc/ipc.go`, and the server enforces them.

**Server to runner**

| Type | Name | Payload |
|---|---|---|
| 64 | `CmdLoad` | JSON: `{"core","rom","system","save","options"}`. Always first |
| 65 | `CmdSaveRAM` | Battery save to restore |
| 66 | `CmdState` | Save state to restore |
| 67 | `CmdInput` | 11 bytes: port (u8), buttons (u16, a bit per RetroPad id), left X, left Y, right X, right Y (i16). All little-endian |
| 68 | `CmdSaveState` | u32 request id |
| 69 | `CmdReset` | None |
| 70 | `CmdPause` | One byte: 1 paused, 0 running. A runner starts paused |
| 71 | `CmdQuit` | One byte: 1 sends a save state first. The battery save is always sent |

**Runner to server**

| Type | Name | Payload |
|---|---|---|
| 1 | `MsgReady` | JSON: `{"name","version","width","height","aspect","fps","sampleRate"}` |
| 2 | `MsgVideo` | u16 width, u16 height, u8 format (0 0RGB1555, 1 XRGB8888, 2 RGB565), then tightly packed rows |
| 3 | `MsgDupe` | None: the previous frame again |
| 4 | `MsgAudio` | Signed 16-bit little-endian stereo |
| 5 | `MsgState` | u32 request id (0 when sent on quit), then the state; nothing after the id if there is none |
| 6 | `MsgSaveRAM` | Battery save |
| 7 | `MsgEvent` | JSON: `{"type": "rumble", "port", "effect", "strength"}`, or `message`, `geometry`, `rotation`, `avinfo`, `log`, `shutdown` |
| 8 | `MsgError` | Text, then the runner exits |

## What's needed from you to start the fork

1. Decide on the name (see above).
2. Fork <https://github.com/libretro/RetroArch> on GitHub as `wittywilla/WittyRetroArch`.
3. Allow a C toolchain and running RetroArch on the machine doing the work, or build it in GitHub
   Actions. Neither is available in this development session.
