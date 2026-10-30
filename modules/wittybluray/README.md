# WittyBluray

**A Blu-ray engine in pure Go.** WittyBluray reads a Blu-ray image or BDMV folder and plays it
the way a player does: the first-play program, the menus with their buttons, the titles,
chapters, angles, audio and subtitle streams, and every command the disc's program runs in
between. It is an original implementation of the public Blu-ray Disc structure (the HDMV
navigation model); no code from other players is used.

WittyBluray is the Blu-ray engine of
[WittyMediaServer](https://github.com/wittywilla/WittyMediaServer) and the coming
WittyMediaPlayer, and a library and command-line tool for anyone building a player.

> This repository is a read-only mirror of `modules/wittybluray` in
> [WittyMediaServer](https://github.com/wittywilla/WittyMediaServer), where development happens.
> Please open issues and pull requests there.

## What it does

- **Reads discs without mounting them:** `.iso` and `.img` images (UDF 2.50, with their
  metadata partition) and BDMV folders, through
  [WittyDisc](https://github.com/wittywilla/WittyDisc), which finds each stream file's byte
  extents so a player can hand them straight to a decoder such as FFmpeg.
- **Parses the disc's program:** `index.bdmv` (first play, top menu, titles),
  `MovieObject.bdmv` (the navigation programs), playlists (`.mpls`: play items, angles,
  stream tables, sub-paths, chapter marks) and clip information (`.clpi`: programs, time bases,
  entry-point maps that turn a time into a byte position).
- **Runs the disc's commands:** the HDMV virtual machine with its 4096 general-purpose and 128
  player status registers; branch, compare and set commands; SET_SYSTEM effects (stream
  selection, the navigation timer, button pages, 3D output mode).
- **Navigates like a player:** first play, titles, menus, calls and resumes, chapter marks,
  menu call and resume, and direct playlist playback for "play the movie".
- **Decodes the menus:** the interactive graphics (IG) stream: pages, button groups, buttons
  with their arrow-key neighbours and commands, pictures for the normal, selected and
  activated states, palettes. Menus are rendered to images and driven with a remote.
- **Decodes subtitles:** presentation graphics (PG) pictures with their times, read in windows
  so a 20 GB film is never scanned whole.
- **Knows its limits and says so:** encrypted (AACS) discs are detected and refused (nothing is
  decrypted); BD-J (Java) menus are detected and the disc falls back to its titles; 3D (MVC)
  content is reported.
- **4K and HDR:** 2160p HEVC streams and their HDR10, HDR10+ and Dolby Vision flags are read
  and reported, so a player can choose its output and tone-map for SDR screens.
- **A command-line tool,** `wittybluray`, that reports everything above, simulates a run
  through a disc's menus, and renders its menu pages, for diagnosing discs that misbehave. Its
  output holds no video or artwork, so it is safe to attach to a bug report.

What it leaves to the player: decoding and showing the video and audio. WittyBluray tells a
player *which bytes to play, when, and what to draw over them*; WittyMediaServer feeds those
bytes to FFmpeg and shows the result in the browser with the menu and subtitle layers on top.

## The command-line tool

```bash
wittybluray info     THE_LORAX.iso           # titles, the disc's program, playlists, clips, streams
wittybluray playlist THE_LORAX.iso 2         # every item, stream and chapter of one playlist
wittybluray walk     THE_LORAX.iso           # simulate inserting the disc; show the first menu's buttons
wittybluray walk     THE_LORAX.iso 2 5       # ...then press buttons 2 and 5
wittybluray render   THE_LORAX.iso out/      # save the menu's pages as PNG files
```

Build it with `go build ./cmd/wittybluray`. The same commands are built into WittyMediaServer
as `wms bluray …`.

## Using the library

```bash
go get github.com/wittywilla/wittybluray
```

```go
import "github.com/wittywilla/wittybluray"

d, err := wittybluray.Open("THE_LORAX.iso")   // an image, or a folder that holds BDMV
if err != nil { return err }
defer d.Close()
if d.Encrypted { return errors.New("AACS") }

main := d.MainPlaylist()                        // the feature: the longest playlist
for _, it := range main.Items {                 // clips, times, audio/subtitle streams
    fmt.Println(it.Clip, it.Duration(), len(it.Audio), len(it.PG))
}

nav := wittybluray.NewNav(d, wittybluray.NavOptions{})
step := nav.Start()                             // what to play first
for step.Kind == wittybluray.StepPlay {
    if step.Menu {                              // the playlist has a menu stream
        menu, _ := d.LoadMenu(step.IGClip, step.IGPID, 0)
        page := &menu.Pages[0]
        img := menu.Render(page, page.Visible(nil, nil), page.DefaultSelected)
        // show the playlist's video with img over it; on OK:
        step = nav.PressButton(page.Button(page.DefaultSelected).Commands)
        continue
    }
    // play step.Playlist from step.Item at step.Time; byte ranges come from
    // d.ClipExtents(clip) and the clip's entry-point map (Clip.SPNAt); then:
    step = nav.PlaylistEnded()
}
```

The navigator also offers `MenuCall`, `Resume`, `PlayTitle`, `PlayPlaylist`, `SetAudio`,
`SetSubtitle`, `SetAngle`, `SetTime` and `NavTimerFired`. `Subtitles` decodes a window of a
clip's subtitle pictures; `ReadPID` hands out any stream's packets.

Requirements: Go 1.26 or newer, no cgo. Its only dependency is WittyDisc.

## Design

```
disc ──► WittyDisc ──► index/mobj/mpls/clpi parsers ──► nav.go + vm.go ──► Step: what to play
                                                        │                   ├─ menu: playlist video + IG pages (ig.go)
                                                        │                   └─ title: playlist video + PG subtitles (pg.go)
                                                        └─ ts.go (one PID out of a clip), graphics.go (palettes, pictures)
```

| File | What it does |
|---|---|
| `open.go` | Opens images and folders; loads the disc; detects AACS, BD-J and 3D; the main feature |
| `index.go`, `mobj.go` | `index.bdmv` and `MovieObject.bdmv` |
| `mpls.go`, `clpi.go` | Playlists and clip information, with entry-point lookups |
| `vm.go` | Registers, command decoding and the interpreter |
| `nav.go` | The navigator: first play, titles, menus, calls, resume, timers |
| `ts.go` | Transport-stream demuxer for one PID (PES packets with their times) |
| `graphics.go` | Palettes, run-length pictures, windows and composition objects |
| `ig.go` | Menus: pages, button groups, buttons, rendering |
| `pg.go` | Subtitles: display sets to timed pictures |
| `cli/` | The `info`, `playlist`, `walk` and `render` commands |

Everything is careful with untrusted input: every read is bounds-checked, counts are limited,
and a disc's program cannot loop for ever (a run is capped at 100 000 commands).

## Limits

- Encrypted (AACS) discs are refused; make decrypted images of discs you own with a tool that
  is legal where you live.
- BD-J (Java) menus cannot run; such discs play their titles without the menu.
- 3D: the base (2D) view plays; the MVC dependent view is reported, not decoded.
- Menu animations and page transition effects are not played; the first picture of each
  button state is shown.
- Pop-up menus during the film are not shown yet (chapters, audio and subtitles are available
  through the player's own controls).

## License

Apache-2.0, the same licence as [WittyMediaServer](https://github.com/wittywilla/WittyMediaServer).
Copyright 2026 WittyWilla and contributors.

You're welcome to use, change and share WittyBluray. If you redistribute it or build on it, the licence
asks you to keep the [NOTICE](NOTICE) file's attribution, *"WittyBluray, developed by WittyWilla"*, in
your own NOTICE file, documentation or About screen.
