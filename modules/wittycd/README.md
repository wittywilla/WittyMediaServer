<h1 align="center">WittyCD</h1>

<p align="center">
  <b>An audio CD engine in pure Go.</b><br>
  Plays an image of a CD the way a CD player does: one continuous program, with tracks and indexes as places in it, so there is no gap to hide.
</p>

<p align="center">
  <a href="LICENSE"><img alt="Licence: Apache-2.0" src="https://img.shields.io/badge/licence-Apache--2.0-blue.svg"></a>
  <img alt="Go 1.26+" src="https://img.shields.io/badge/Go-1.26%2B-00ADD8.svg">
  <img alt="No cgo" src="https://img.shields.io/badge/cgo-none-success.svg">
  <img alt="No dependencies" src="https://img.shields.io/badge/dependencies-none-success.svg">
  <img alt="Fuzzed" src="https://img.shields.io/badge/parsers-fuzz%20tested-8b6cff.svg">
</p>

WittyCD is the audio CD engine of [WittyMediaServer](https://github.com/wittywilla/WittyMediaServer), and a
library and tool for anyone building a player. It is an original implementation written from the publicly
described formats, under a permissive licence, with no code from GPL libraries.

## What it does

| | Capability | Details |
|---|---|---|
| 💿 | **Reads images** | `.cue` with `.bin`, `.wav`, `.aiff` or a single `.flac`; cdrdao `.toc`; CloneCD `.ccd` with `.img`; Nero `.nrg`; MAME `.chd` (version 5); and a bare `.iso`, `.img` or `.bin` that is really audio |
| 📋 | **Knows the layout** | tracks, index points, pregaps, generated silence, multiple files, sessions, data tracks, pre-emphasis and copy flags, ISRCs, the barcode |
| 🎵 | **Plays without gaps** | the whole disc as one run of samples, or tracks in any order, any number of times, readable at any sample |
| 🗜️ | **Streams FLAC or WAV** | its own small FLAC encoder (about 200 times real time on a desktop), and a WAV view with byte ranges that takes no work at all |
| 🎚️ | **Corrects pre-emphasis** | flagged tracks are de-emphasised as a real player does in hardware: within 0.1 dB of the 50/15 µs curve |
| 🔎 | **Finds what's hidden** | audio before track 1 (HTOA), byte order that a sheet got wrong, CD+G pictures, CD-TEXT |
| 🆔 | **Identifies discs** | MusicBrainz disc ID and table of contents, and the older CDDB ID, including the rules for CD-Extra |
| 🩹 | **Copes with damage** | every file is hostile input: size limits, sheets can't reach outside their folder, truncated images give a shorter disc and a message, and all readers are fuzz tested |
| 🛠️ | **Diagnoses** | `wittycd info` for structure, `wittycd check` to read the whole image and say what is wrong |

**What it leaves to the player:** the buttons and the screen. WittyCD says what is on the disc and hands over
samples.

**Copy protection:** nothing here decrypts or bypasses anything. Plain CD audio needs nothing of the kind. An
image whose audio is scrambled shows up as noise, and is reported as such. A DTS surround disc is reported too, as
it would otherwise play as loud noise.

## The design, and why

**A disc is one stream, not a list of files.** A browser's audio element can't join two files without a gap, and
no amount of timing makes it sample-accurate. A CD never had that problem, because it is one continuous spiral: so
the listener gets one continuous stream, and track changes are just places in it. That is why the player is gapless
by construction. It also makes a position an exact sample, and leaves nothing for clients to get wrong.

**The server keeps nothing it has to work out twice.** A `Stream` holds no position and no state, so one disc serves
any number of listeners, and `ReadAt` works at any offset. Pre-emphasis correction starts 128 samples early and is
fully settled by then, so seeking gives the very same samples as playing through (a test checks this).

**Chosen orders are streams too.** A program (`NewStream([]int{3, 7, 1}, 2, true)`) is a list of ranges played as
one stream, so even a repeat is seamless. Client and server work out the same map from the track table.

**WAV costs nothing, FLAC costs little.** WAV is a 44-byte header in front of the image: no encoding, and browsers
seek by byte range. FLAC saves about a third of the bandwidth for a little CPU. It uses fixed predictors rather than
searching for the best filter, because this runs live on small machines, and because a sized-down FLAC that plays
now beats a slightly smaller one that makes a Raspberry Pi hot.

**The two halves are apart.** Reading an image gives a plain structure, and only opening audio touches the files
again. So parsing can move into a helper process (see WittyMediaServer's `docs/MODULES.md`) with no change to how
samples reach a player.

## Quick start

```bash
go install github.com/wittywilla/wittycd/cmd/wittycd@latest
wittycd info "Album.cue"
wittycd check "Album.cue"
```

To use the library:

```go
d, err := wittycd.Open("Album.cue")
if err != nil {
    return err
}
defer d.Close()

toc := d.TOC()
fmt.Println(toc.MusicBrainzID(), toc.LookupString())

// The whole disc, as laid out, with pre-emphasis corrected.
s, err := d.NewStream(nil, 1, true)
if err != nil {
    return err
}
err = s.WriteFLAC(w, 0)          // to any io.Writer, starting at a sample
wav := s.WAV(0)                  // or a file you can read at any offset
_ = io.NewSectionReader(wav, 0, wav.Size())
```

## Limits

- A sheet may name only files in or under its own folder. A path that climbs out is looked for by its last part
  beside the sheet, which also rescues rips that recorded the ripper's own folders.
- Compressed audio other than a single FLAC (`.mp3`, `.ape`, and so on) isn't read: a sheet beside them isn't treated as
  an image. A sheet beside per-track FLAC files isn't either, since those are tagged songs already.
- CD+G comes from a `.cdg` file beside the image or from `CDG` sectors. Raw `.sub` files are interleaved in a way
  that isn't undone yet.
- CD-TEXT in double-byte character sets isn't decoded, as that would need large tables, and is reported.
- CCD and NRG are tested against images built to their published descriptions, not against dumps from the original
  programs. CHD and TOC are checked against the real tools.

## Credits

Developed by WittyWilla as part of [WittyMediaServer](https://github.com/wittywilla/WittyMediaServer).
Apache-2.0: see [LICENSE](LICENSE) and [NOTICE](NOTICE). Disc IDs follow the method published by
[MusicBrainz](https://musicbrainz.org/doc/Disc_ID_Calculation); this project isn't connected with them.
