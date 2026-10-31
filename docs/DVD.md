# DVDs and the WittyDVD engine

WittyMediaServer plays DVD images (`.iso`, `.img`) and `VIDEO_TS` folders **with their real
menus**, in a browser. The part that does this is **WittyDVD**: an original implementation of
the DVD-Video navigation model — the same logic a hardware DVD player runs.

- [Playing a disc](#playing-a-disc)
- [Disc settings](#disc-settings)
- [The remote and keyboard](#the-remote-and-keyboard)
- [Audio and subtitles](#audio-and-subtitles)
- [Interactive extras and games](#interactive-extras-and-games)
- [Performance on small machines](#performance-on-small-machines)
- [How WittyDVD works](#how-wittydvd-works)
- [Diagnosing a disc](#diagnosing-a-disc)
- [Limits](#limits)

Working on the engine? [DVD-QUIRKS.md](DVD-QUIRKS.md) surveys the DVD-Video features and the
quirks of real discs, and how WittyDVD handles each.

## Playing a disc

On a DVD's page:

| Button | What happens |
|---|---|
| **Insert DVD** | starts the disc like a player: first-play program, warnings, studio intros, trailers the disc plays, then the menu. Optionally with a tray animation and drive sound |
| **Play movie** / **Resume** | straight into the main feature (the longest title), or back to where you stopped |
| **Main menu** | the disc's menu, skipping the intros |
| Disc titles table | play any title (extras, trailers…) or chapter directly |

"Play movie" also works on discs that guard their feature (many discs refuse a direct jump unless
their own *Play* button was used); WittyDVD enters the film anyway and picks the audio/subtitles
from your preferred languages.

## Disc settings

*Disc settings* on a DVD's page (administrators):

- **When this disc is inserted**: start with the disc menu, or skip menus and play the feature.
  The server-wide default is under *Settings → Playback & discs → DVDs and Blu-rays*.
- **Insert DVD effect**: the tray animation and synthesized drive sound, on or off for this disc;
  the default is also under *Settings → Playback & discs*. Click the animation to skip it.

## The remote and keyboard

Press **Remote** (or `r`) for an on-screen DVD remote:

| Key | Action |
|---|---|
| Eject | leave the player |
| Top menu / `t` | the disc's title menu |
| Menu / `m` | the current title set's menu (remembers your exact position) |
| ▲ ▼ ◀ ▶ / arrow keys | move the highlight along the disc's own button links |
| Enter / `Enter` | activate the highlighted button |
| Return | go up one menu level, or resume the film |
| Audio, Subtitle | cycle through the tracks |
| ⏮ ⏭ / `p` `n` | previous / next chapter |
| ⏪ ⏩ | back 10 s / forward 30 s |
| ▶ ⏸ ⏹ / `Space` | play, pause, stop |
| 0–9 | press menu button *n*; during the film, jump to chapter *n* |

You can also hover and click buttons with the mouse. The highlight moves instantly; the server
is told in the background.

A picture the disc holds until you act, with nothing to press, shows **Continue** (the Play key
of a DVD player).

The remote can be **moved** (drag its top bar), **resized** (drag the corner grip; double-click it
to reset) and made **see-through** (the ◐ button cycles four levels). It remembers all three, and
it fades while you're not moving the mouse.

## Audio and subtitles

- Audio and subtitle tracks are labelled with their purpose from the disc, e.g. *English ·
  Commentary · AC3 2ch* or *English · Captions*.
- Changing the audio track restarts the stream **at the same moment** of the film.
- Titles filmed from several **camera angles** (often the same scene with on-screen text in
  another language) offer an *Angle* choice; switching also continues at the same moment.
- Subtitles are drawn by the player from the disc's own subtitle pictures, so switching them is
  instant and they look exactly like on a DVD player. **Forced** subtitles (for alien languages,
  signs…) show even when subtitles are off.
- Choices made in the disc's own menus (language / setup menus) are honoured.
- Preferred languages for discs that don't choose: *Settings → Playback & discs*.

## Interactive extras and games

Many discs put buttons *inside* titles: games, quizzes, "vote for your favourite", branching
featurettes (Shrek 2's *Far Far Away Idol*, for example). WittyDVD detects these and plays them
cell by cell with their buttons, timers and stills, exactly as authored. The top bar shows
*Interactive feature* while one is running.

## Performance on small machines

- Menu scans, button artwork and subtitle pictures are cached per disc, so returning to a menu
  or moving the highlight never re-reads the disc.
- The next menu clip (e.g. intro → looping menu) is prepared while the current one plays.
- *Settings → Transcoding → Low-power mode for DVDs* uses cheaper deinterlacing and the fastest
  encoder preset — enough for a Raspberry Pi 4/5 or an Intel N100 to play discs in real time.
- A hardware encoder (*Settings → Transcoding*) helps most. See [Transcoding](TRANSCODING.md).

## How WittyDVD works

```
disc image ──► ISO 9660 / UDF reader ──► IFO parser ──► navigator + command VM ──► what to show
                                                         │                         ├─ menu: video clip + button layer
                                                         │                         └─ title: video stream + subtitles
                                                         └─ NAV packs, sub-pictures (buttons, subtitles)
```

WittyDVD is its own Go module, `modules/wittydvd` here and
[github.com/wittywilla/WittyDVD](https://github.com/wittywilla/WittyDVD) on its own, so other
players can use it too. The steps below name its files.

1. The image is read directly (no mounting): the `isofs` package.
2. The IFO files (the disc's "program") are parsed: titles, program chains, cells, menus,
   audio/subtitle tables — `ifo.go`.
3. The **virtual machine** runs the disc's commands (16 general and 24 system registers,
   links, jumps, calls, comparisons, arithmetic, timers) — `vm.go`.
4. The **navigator** follows first-play → menus → titles exactly as the commands dictate,
   including resume points, still frames and looping menus — `nav.go`.
5. For each menu or interactive cell it reads the NAV packs for button positions and links, and
   decodes the sub-picture for the highlight artwork — `navpack.go`, `menu.go`, `spu.go`.
6. The video of each cell is converted to HLS by FFmpeg and shown in the browser with the button
   and subtitle layers on top — `internal/dvdsession`. A title is read in runs (`runs.go`): cells
   that follow each other on the disc are one recording and are read as one, so nothing happens at
   their chapters. Where a join is real, the next run's first picture is put exactly where it
   belongs. `internal/seamcheck` measures this; see `TestRealDiscChapterSeams`.

Everything is written from the public DVD-Video structure; no libdvdnav/libdvdread code is used,
and scrambled (CSS) discs are refused — WittyMediaServer never decrypts anything.

Real discs bend the rules: still pictures that hold for minutes of sound, random and shuffled
programs, decoy titles, broken IFOs with an intact backup, menus in only some languages.
[DVD-QUIRKS.md](DVD-QUIRKS.md) lists what WittyDVD does with each.

## Diagnosing a disc

The `wms` command-line tool inspects a disc without the web interface:

```bash
wms dvd info  SHREK_2.ISO               # titles, menus, languages, first-play program
wms dvd walk  SHREK_2.ISO 1             # simulate inserting the disc and press button 1
wms dvd crawl SHREK_2.ISO               # press every button of every menu; reports dead ends
wms dvd pgc   SHREK_2.ISO vtsm 12 26    # show the commands of one program chain
wms dvd render SHREK_2.ISO out/         # save the first menu's highlight images
```

On Windows run these in a terminal in the WittyMediaServer folder (`.\wms.exe dvd info …`).
The same commands exist as a standalone tool, `wittydvd`, from the
[WittyDVD repository](https://github.com/wittywilla/WittyDVD) (`wittydvd info SHREK_2.ISO`).
When reporting a disc problem, attach the output of `info` and `crawl` — never the disc image.
`walk` and `crawl` give the same output every run (the disc's random numbers come from a fixed
seed), so two versions can be compared line by line.

If playback stutters at a chapter, this check plays a title through the real stream and compares it with
a straight read of the disc, frame by frame and sound by sound. It takes as long as FFmpeg needs to convert
the title, and needs a developer's copy of the source and FFmpeg:

```bash
WMS_SEAM_DVD=SHREK_2.ISO WMS_SEAM_OUT=seams.txt go test ./internal/dvdsession -run RealDiscChapterSeams -v -timeout 60m
```

`WMS_SEAM_PACED=1` paces it like someone watching, so the server's rests and restarts are included.
In the table, `shift ms` and `mark ms` should be 0 at every boundary. A steady 21 ms of sound is the audio
encoder's own start. A low `conf` means that moment's sound was too quiet or too repetitive to compare, so its
figures mean nothing, and a dark or still scene can show a few frames as lost and repeated.

## Limits

- CSS-scrambled images are refused; make decrypted images of discs you own with a tool that is
  legal where you live.
- Karaoke discs play all their channels mixed: guide vocals cannot be switched off.
- The disc's prohibitions of skipping and seeking (on warning screens, for example) are not
  enforced.
- Blu-ray discs have their own engine and page: [BLURAY.md](BLURAY.md).

---
<sub>WittyMediaServer 0.5.0-alpha documentation</sub>
