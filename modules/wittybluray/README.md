<h1 align="center">WittyBluray</h1>

<p align="center">
  <b>A Blu-ray engine in pure Go.</b><br>
  Plays a Blu-ray image or BDMV folder the way a player does: first-play, interactive and pop-up menus, titles, subtitles, 4K and HDR.
</p>

<p align="center">
  <a href="LICENSE"><img alt="Licence: Apache-2.0" src="https://img.shields.io/badge/licence-Apache--2.0-blue.svg"></a>
  <img alt="Go 1.26+" src="https://img.shields.io/badge/Go-1.26%2B-00ADD8.svg">
  <img alt="No cgo" src="https://img.shields.io/badge/cgo-none-success.svg">
  <img alt="Fuzzed" src="https://img.shields.io/badge/parsers-fuzz%20tested-8b6cff.svg">
</p>

<p align="center">
  <a href="#quick-start">Quick start</a> ·
  <a href="#the-command-line-tool">Command-line tool</a> ·
  <a href="#using-the-library">Library</a> ·
  <a href="#design">Design</a> ·
  <a href="#limits">Limits</a> ·
  <a href="https://github.com/wittywilla/WittyMediaServer/blob/master/docs/BLURAY.md">Blu-ray guide</a>
</p>

> Read-only mirror of `modules/wittybluray` in [WittyMediaServer](https://github.com/wittywilla/WittyMediaServer),
> where development happens. Please open issues and pull requests there.

WittyBluray is the Blu-ray engine of [WittyMediaServer](https://github.com/wittywilla/WittyMediaServer),
and a library and tool for anyone building a player. It is an original implementation of the public
Blu-ray Disc structure (the HDMV navigation model).

## What it does

| | Capability | Details |
|---|---|---|
| 💿 | **Reads discs without mounting** | `.iso`/`.img` images (UDF 2.50 with its metadata partition) and BDMV folders, through [WittyDisc](https://github.com/wittywilla/WittyDisc) |
| 📋 | **Parses the disc's program** | `index.bdmv`, `MovieObject.bdmv`, playlists (`.mpls`: items, angles, streams, sub-paths, chapters) and clip information (`.clpi`: entry-point maps from time to byte) |
| ⚙️ | **Runs the disc's commands** | the HDMV virtual machine: 4096 general and 128 player registers; branch, compare, set and system commands |
| 🧭 | **Navigates like a player** | first play, titles, menus, calls and resumes, chapter marks, the navigation timer, direct playlist play |
| 🔘 | **Decodes menus** | interactive graphics: pages, button groups, buttons with their arrow neighbours and commands, normal/selected/activated pictures, **animated** highlights and **pop-up** menus |
| 💬 | **Decodes subtitles** | presentation graphics, read in windows so a 20 GB film is never scanned whole |
| 🌈 | **4K and HDR** | 2160p HEVC with HDR10, HDR10+ and Dolby Vision flags, so a player can tone-map for SDR screens |
| ☕ | **Runs BD-J menus** | with a Java the server's owner installs (none is shipped), in a default-deny sandbox; without one, the disc plays its titles |
| 🔒 | **Knows its limits** | AACS-encrypted discs are refused and BD+ is not run (nothing is decrypted); 3D (MVC) is reported |
| 🛠️ | **Diagnoses discs** | the `wittybluray` tool reports all of the above, simulates the menus, and renders menu pages |

**What it leaves to the player:** decoding the video and audio. WittyBluray tells a player *which
bytes to play, when, and what to draw over them*.

## Quick start

```bash
go install github.com/wittywilla/wittybluray/cmd/wittybluray@latest
wittybluray info MY_DISC.iso
```

Needs Go 1.26 or newer; the tool lands in `$(go env GOPATH)/bin`. To use the library in your own
program: `go get github.com/wittywilla/wittybluray@latest`.

## The command-line tool

| Command | What it shows |
|---|---|
| `wittybluray info <disc>` | titles, the disc's program, playlists, clips and streams, 3D and HDR flags |
| `wittybluray playlist <disc> <n>` | every item, stream and chapter of one playlist |
| `wittybluray menu <disc> <playlist>` | a playlist's menu: pages, buttons, every command |
| `wittybluray walk <disc> [button…]` | simulates inserting the disc, then pressing menu buttons by id |
| `wittybluray render <disc> <dir>` | the first menu's pages as PNG files |
| `wittybluray bdj <disc> [seconds [picture.png [key...]]]` | BD-J titles and what their code needs; with Java, runs them, pressing the keys named |

The output holds no video or artwork, so it's safe to attach to a bug report. The same commands are
built into WittyMediaServer as `wms bluray …`.

## Using the library

```go
import "github.com/wittywilla/wittybluray"

d, err := wittybluray.Open("MY_DISC.iso")   // an image, or a folder holding BDMV
if err != nil { return err }
defer d.Close()
if d.Encrypted { return errors.New("AACS") }

nav := wittybluray.NewNav(d, wittybluray.NavOptions{})
step := nav.Start()                            // what to play first
for step.Kind == wittybluray.StepPlay {
    if step.Menu {                             // the playlist carries a menu
        menu, _ := d.LoadMenu(step.IGClip, step.IGPID, 0)
        page := &menu.Pages[0]
        img := menu.Render(page, page.Visible(nil, nil), page.DefaultSelected)
        _ = img                                // show the video with img over it; on OK:
        step = nav.PressButton(page.Button(page.DefaultSelected).Commands)
        continue
    }
    // play step.Playlist from step.Item at step.Time (byte ranges from d.ClipExtents
    // and the clip's entry-point map), then:
    step = nav.PlaylistEnded()
}
```

<details>
<summary><b>The navigator and decoders at a glance</b></summary>

| Call | Does |
|---|---|
| `Start`, `PlaylistEnded` | first play; what comes after a playlist |
| `PressButton`, `MenuCall`, `Resume` | menus and returning to the film |
| `PlayTitle`, `PlayPlaylist`, `SetTime`, `SetChapter` | titles, playlists and chapters |
| `SetAudio`, `SetSubtitle`, `SetAngle` | streams |
| `NavTimerFired` | the navigation timer |
| `LoadMenu`, `Menu.Render`, `Menu.DrawFrame`, `Popup` | menus, animation frames, pop-ups |
| `Subtitles`, `ReadPID` | subtitle pictures; any stream's packets |

</details>

Requirements: Go 1.26 or newer, no cgo. Its only dependency is WittyDisc. BD-J also needs Java 8 or newer on the machine, at run time only.

## Design

```mermaid
flowchart LR
  I[Disc image] --> D[WittyDisc<br/>UDF 2.50]
  D --> P[index · mobj · mpls · clpi<br/>parsers]
  P --> N[nav.go + vm.go<br/>navigator + HDMV VM]
  N --> S{Step}
  S -->|menu| M[playlist video + IG pages<br/>ig.go · graphics.go]
  S -->|title| T[playlist video + PG subtitles<br/>pg.go]
  D -.-> X[ts.go<br/>one PID out of a clip]
```

| File | What it does |
|---|---|
| `open.go` | opens images and folders; detects AACS, BD-J and 3D; finds the main feature |
| `index.go`, `mobj.go` | `index.bdmv` and `MovieObject.bdmv` |
| `mpls.go`, `clpi.go` | playlists and clip information, with entry-point lookups |
| `vm.go`, `nav.go` | the HDMV virtual machine and the navigator |
| `ts.go`, `graphics.go` | the transport-stream demuxer; palettes, pictures, windows |
| `ig.go`, `pg.go` | menus (interactive graphics) and subtitles (presentation graphics) |
| `bdjo.go`, `sound.go`, `textst.go` | BD-J objects, button sounds, text subtitles |
| `bdj/` | starts Java, the link to it, and `runtime.jar`, the platform disc applications link against (`bdj/java/src`, built by `bdj/java/build.sh`) |
| `cli/` | the command-line tool |

**Safe with untrusted discs:** every read is bounds-checked, counts are limited, a disc's program
can't loop for ever (100 000 commands per run), and the parsers are fuzz-tested.

## Limits

| Not yet | What happens instead |
|---|---|
| AACS-encrypted discs | refused: encrypted media isn't supported |
| Secondary audio, picture-in-picture | the disc's choice is recorded, not mixed in |
| MVC 3D | the 2D view plays; the second view is reported, not decoded |

Tested on custom-authored discs; retail discs exercise more of the format and may show problems not
seen yet. Reports with `wittybluray info` and `walk` output are very welcome.

## The Witty family

| Project | Role |
|---|---|
| [WittyMediaServer](https://github.com/wittywilla/WittyMediaServer) | the media server these engines power |
| [WittyDVD](https://github.com/wittywilla/WittyDVD) · **WittyBluray** | the DVD and Blu-ray engines |
| [WittyDisc](https://github.com/wittywilla/WittyDisc) | disc image reader |

## Licence

**Apache-2.0**, the same licence as WittyMediaServer. © 2026 WittyWilla and contributors.
Use, change and share it; if you redistribute it or build on it, keep the [NOTICE](NOTICE)
attribution, *“WittyBluray, developed by WittyWilla”*, in your NOTICE file, documentation or About screen.

## Legal

WittyBluray is an independent open-source project. It isn't associated with, sponsored by or
endorsed by the Blu-ray Disc Association, any film studio or any other player project. “Blu-ray” and
“Blu-ray Disc” are trademarks of the Blu-ray Disc Association; WittyBluray isn't a licensed or
certified Blu-ray product and uses no Blu-ray logo. AACS-encrypted discs are refused, BD+ isn't run,
nothing is decrypted, and no media comes with it: you're responsible for having the rights to what
you open. Any third-party names here only describe what it works with, and belong to their owners.
It's provided “as is”, with no warranty; see [LICENSE](LICENSE). Local law applies to how you use
it. This isn't legal advice.

<!-- module-kit -->
## Releasing

A version tag builds a program for every platform, signs the checksum list, and publishes a release that servers can install.

Versions move in small steps as work lands. `go run github.com/wittywilla/wittymodule/cmd/wittymodule@v0.2.1 bump .` raises it
everywhere it is written (add `-part minor` for a bigger step). A bump isn't a release: nothing is
tagged or published until you tag it. CI fails when code changed since the last tag and the version
didn't, because tags never move.

To release:

1. Check that `version` in `module.json` is the one to publish, with the bump committed.
2. Tag that commit and push the tag: `git tag vX.Y.Z`, then `git push origin vX.Y.Z`. A tag with a
   suffix, such as `v1.2.0-beta`, is published as a prerelease.
3. Watch the *Actions* tab. The signing step uses the `WMS_SIGNING_KEY` secret of the `release` environment, and nothing is published without it.

To take a version back, run the *withdraw* workflow from the *Actions* tab. It adds the version to a signed list on the release tagged `withdrawals`, which servers read.

Licensed under Apache-2.0: see [LICENSE](LICENSE) and [NOTICE](NOTICE).
<!-- /module-kit -->
