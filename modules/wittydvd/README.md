# WittyDVD

**A DVD-Video engine in pure Go.** WittyDVD reads a DVD image or `VIDEO_TS` folder and plays it
the way a hardware player does: the first-play program, the menus with their highlighted buttons,
the titles, chapters, angles, audio and subtitle streams, and every command the disc's program
runs in between. It is an original implementation of the public DVD-Video navigation model; no
code from libdvdnav or libdvdread is used.

WittyDVD is the disc engine of [WittyMediaServer](https://github.com/wittywilla/WittyMediaServer)
and the coming WittyMediaPlayer, and a library and command-line tool for anyone building a
player.

> This repository is a read-only mirror of `modules/wittydvd` in
> [WittyMediaServer](https://github.com/wittywilla/WittyMediaServer), where development happens.
> Please open issues and pull requests there.

## What it does

- **Reads discs without mounting them:** `.iso` and `.img` images (UDF and ISO 9660 "bridge"
  discs) and `VIDEO_TS` folders. The `isofs` package finds each file's byte extents in the image,
  so a player can hand the raw VOB ranges straight to a decoder such as FFmpeg.
- **Parses the disc's program:** the VMG and VTS IFO files: titles, program chains (PGCs),
  programs and cells, menu language units, audio and sub-picture attributes, palettes, parental
  levels and region codes.
- **Runs the disc's commands:** a virtual machine with the 16 general and 24 system registers,
  links, jumps and calls, comparisons, arithmetic and the navigation timer.
- **Navigates like a player:** first play, menus and titles, resume points, still frames, menu
  loops, up buttons, chapter skipping, and the title, root, subtitle, audio, angle and chapter
  menu keys.
- **Reads NAV packs:** button positions, colours and links (PCI) and the cell's timing and
  seamless-branching information (DSI), including interleaved multi-angle cells.
- **Decodes sub-pictures:** the run-length-coded sub-picture units used for menu highlights and
  subtitles, rendered to RGBA with the disc's palette and highlight colours, cropped to the
  button or line.
- **Refuses scrambled discs** (CSS) and never decrypts anything.
- **A command-line tool,** `wittydvd`, that reports everything above and simulates a run through
  a disc's menus, for diagnosing discs that misbehave (its output contains no video or artwork,
  so it is safe to attach to a bug report).

What it leaves to the player: decoding and showing the video and audio. WittyDVD tells a player
*which bytes to play, when, and what to draw over them*; WittyMediaServer feeds those bytes to
FFmpeg and shows the result in the browser with the button and subtitle layers on top.

## The command-line tool

```bash
wittydvd info   SHREK_2.ISO            # titles, menus, languages, first-play program
wittydvd walk   SHREK_2.ISO 1          # simulate inserting the disc and pressing button 1
wittydvd crawl  SHREK_2.ISO            # press every button of every menu; report dead ends
wittydvd pgc    SHREK_2.ISO vtsm 12 26 # the commands of one program chain
wittydvd render SHREK_2.ISO out/       # save the first menu's highlight images
wittydvd disc ls SHREK_2.ISO           # list the files inside the image
```

Build it with `go build ./cmd/wittydvd`. The same commands are built into WittyMediaServer as
`wms dvd …`.

## Using the library

```bash
go get github.com/wittywilla/wittydvd
```

```go
import "github.com/wittywilla/wittydvd"

o, err := wittydvd.Open("SHREK_2.ISO")   // an image or a VIDEO_TS folder
if err != nil { return err }
defer o.Close()

for _, t := range o.Disc.Titles() {        // titles with their length, chapters, angles, streams
    fmt.Println(t.Number, t.Duration, len(t.Chapters))
}

nav := wittydvd.NewNav(o.Disc, wittydvd.NavOptions{})
step := nav.Start()                        // what to show first: a menu, a title cell, or stop
for step.Kind != wittydvd.StepStop {
    switch step.Kind {
    case wittydvd.StepMenu:
        rd, _ := o.Reader(wittydvd.SpaceFor(step.Domain, step.VTS))
        pic, _ := wittydvd.ScanMenu(rd, step.FirstSector, step.LastSector, wittydvd.MenuScanOptions{Palette: step.Palette})
        // pic has the buttons, their rectangles and highlight artwork; show the cell's video
        // under it, then:
        step = nav.PressButton(1, nil)
    case wittydvd.StepTitle:
        ext, _ := o.CellExtents(wittydvd.SpaceFor(step.Domain, step.VTS), step.Cell)
        // play these byte ranges, then:
        step = nav.CellEnded()
    }
}
```

The navigator also offers `MenuKey`, `GoUp`, `OpenMenu`, `Resume`, `PlayTitle`, `NextChapter`,
`PrevChapter`, `SetAudio`, `SetSubpicture`, `SetAngle`, `Tick` (for the navigation timer) and
`Clone` (to explore a disc without disturbing the live session). `CellSubpictures` returns a
cell's subtitle units, and `DecodeSPU` and `SPU.Render` turn them into images.

Requirements: Go 1.26 or newer, no cgo, no other dependencies.

## Design

```
disc image ──► isofs (UDF / ISO 9660) ──► ifo.go (IFO parser) ──► nav.go + vm.go ──► Step: what to show
                                                                  │                   ├─ menu: cell video + buttons
                                                                  │                   └─ title: cell video + subtitles
                                                                  └─ navpack.go (PCI/DSI), spu.go, menu.go, subtitles.go
```

| File | What it does |
|---|---|
| `isofs/` | ISO 9660 and UDF reader: files as byte extents, no mounting |
| `open.go` | Opens images and folders; title summaries; sector readers for each VOB address space |
| `ifo.go` | VMG and VTS IFO parsing: titles, PGCs, cells, menus, attributes |
| `vm.go` | Registers, instruction decoding and the command interpreter |
| `nav.go` | The navigator: first play, menus, titles, resume, stills, keys |
| `navpack.go` | NAV packs (PCI and DSI) and a pack scanner over VOB sectors |
| `interleave.go` | Cell sector ranges, including interleaved multi-angle blocks |
| `spu.go` | Sub-picture decoding and rendering |
| `menu.go` | Menu scanning: buttons, highlight pictures, default button |
| `subtitles.go` | A cell's subtitle sub-pictures |
| `cli/` | The `info`, `walk`, `crawl`, `pgc`, `render` and `disc ls` commands |

Everything is written from the public DVD-Video structure and is careful with untrusted input:
IFO reads are size-bounded, and malformed tables, commands and sub-pictures are refused rather
than trusted.

## License

Apache-2.0, the same licence as [WittyMediaServer](https://github.com/wittywilla/WittyMediaServer).
Copyright 2026 WittyWilla and contributors.

You're welcome to use, change and share WittyDVD. If you redistribute it or build on it, the licence
asks you to keep the [NOTICE](NOTICE) file's attribution, *"WittyDVD, developed by WittyWilla"*, in
your own NOTICE file, documentation or About screen.
