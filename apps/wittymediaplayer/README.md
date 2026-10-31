<h1 align="center">WittyMediaPlayer</h1>

<p align="center">
  <b>Video, audio, DVDs and Blu-rays, on your own computer.</b><br>
  Real disc menus, straight from a drive, a disc image or a folder. No server, no account, and nothing leaves your computer.
</p>

## What it does

- **Plays ordinary files:** MKV, MP4, AVI, MPEG, TS, WebM, WMV, MP3, AAC, FLAC, Ogg, WAV and more, with
  audio and subtitle track choice. Subtitle files beside a video (`Movie.srt`, `Movie.en.srt`) are found by themselves.
- **Plays DVDs and Blu-rays with their real menus:** arrows, Enter and the mouse work the disc's own
  buttons, with title, chapter and angle controls, using the same engines as
  [WittyMediaServer](https://github.com/wittywilla/WittyMediaServer).
- **Plays from a disc drive directly,** as well as from `.iso` and `.img` images and `VIDEO_TS` or `BDMV` folders.
- **Makes disc images:** copies a disc to one `.iso` or `.img` file to keep, play later, or add to a
  WittyMediaServer library.
- **Says no plainly:** a copy-protected disc gets a clear message. Nothing here unlocks copy protection.
- **Runs well on small machines:** the picture is drawn by the graphics card, and hardware decoding is used where
  the card has it.
- **Keeps to itself:** no telemetry, no update checks, and nothing leaves your computer. Its FFmpeg and its disc helper
  are built with no network code. (`Qt6Network.dll` is in the folder only because Qt's sound library won't load without
  it. Nothing calls it.)

## Use it

Unzip the folder anywhere and run `wittymediaplayer.exe`. It needs Windows 10 version 1809 or newer, 64-bit.

Open something with **File, Open**, by dropping it on the window, from the list of drives on the start page, or
from a command line:

```bash
wittymediaplayer.exe "C:\Movies\film.mkv"
```

```bash
wittymediaplayer.exe "C:\Discs\film.iso"
```

```bash
wittymediaplayer.exe D:
```

### Keyboard

| Key | Does |
|---|---|
| Space or K | Play or pause |
| Left, Right | Back and forward 10 seconds |
| Up, Down | Volume |
| F, Esc | Full screen, and out of it |
| A, S | Next audio track, next subtitle track |
| N, P | Next and previous chapter |
| 1 to 9 | Go to that chapter on a disc; that far through a file |
| M (files) | Mute |

On a disc, in its menus:

| Key | Does |
|---|---|
| Arrow keys, Enter | Move along the disc's buttons, and press one |
| M | The disc's menu, or its pop-up menu during a Blu-ray film |
| T | Title menu |
| Backspace | Go back up a menu |

Tab reaches every control, and screen readers can read them, including a disc's menu buttons.

### Making a disc image

Put the disc in a drive, then choose **File, Make disc image** or the button beside the drive on the start page.
To use the image in WittyMediaServer, save it in (or move it to) a folder one of your disc libraries watches, and
scan the library. The same copy runs from a command line, and never overwrites anything:

```bash
wittymediaplayer-engine.exe --image D: "C:\Discs\film.iso"
```

Discs protected with CSS or AACS are refused before anything is written. Audio CDs have no files to image.

## What doesn't work yet

- Blu-ray menus written in Java don't run. The disc plays its titles instead, and the window says so.
- Sound is stereo. Surround tracks are mixed down.
- Windows only for now. Nothing in the design stops other systems: see [DESIGN.md](DESIGN.md).

## Build it

You need Git for Windows, Python 3 and Go. Everything else is fetched for your user only, with no
administrator rights and no accounts. Run these in Git Bash from the repository's top folder:

```bash
bash apps/wittymediaplayer/scripts/setup-toolchain.sh
```

```bash
bash apps/wittymediaplayer/scripts/build-windows.sh
```

The portable folder and zip land in `apps/wittymediaplayer/.build/`.

To run the tests:

```bash
bash apps/wittymediaplayer/scripts/test.sh
```

They need no one at the screen and use Qt's offscreen platform. Set `WITTY_TEST_DVD` and `WITTY_TEST_BD` to
disc images to include real discs; those are only ever read.

## Licence

**Apache-2.0**, the same as WittyMediaServer. © 2026 WittyWilla and contributors. If you redistribute it or build
on it, keep the [NOTICE](NOTICE) attribution, *"WittyMediaPlayer, developed by WittyWilla"*.

Qt and FFmpeg ship as separate libraries under the LGPL, so you can replace them. See
[licenses/THIRD-PARTY.txt](licenses/THIRD-PARTY.txt).
