# WittyCD: update plan

> **Status on 2026-11-04**
>
> **Planned:** [the idea](https://github.com/wittywilla/WittyMediaServer/discussions/60).

**Now:** 0.2.0. Helper, API 3. Audio CD images played as discs: gapless, tracks, indexes, CD-TEXT, in a CD player; its own FLAC encoder; album details from MusicBrainz and the Cover Art Archive by table of contents only. Doesn't read compressed audio other than a single FLAC.

**Channel:** alpha.

| Release | Theme |
|---|---|
| 0.2.x | Fixes |
| **0.3.0** | CD+G from a raw `.sub` file; double-byte CD-TEXT; matching disc icon (if the owner wants the set) |
| Later | More image formats, hidden tracks, audio CDs as extra discs for games |

## 0.3.0

- **CD+G** from a raw `.sub` file (roadmap: not built). Karaoke discs: ties in with the karaoke features added in 0.6.0.
- **Double-byte CD-TEXT** (Japanese and other languages): reported today, names come from elsewhere.
- **Icon:** the same disc as WittyDVD and WittyBluray with **C**, if the owner wants a set ([../design/module-icons.md](../design/module-icons.md#a-set-if-the-owner-wants-it)).

## Later

- **Single-file images in other lossless formats** (WavPack, APE, TTA) with a cue sheet, where a decoder with a usable licence exists.
- **Hidden track one** (pre-gap audio before track 1) played when present.
- **Audio CDs as extra discs for games:** WittyPlay 0.4.9's disc set editor can add an audio CD image; make sure WittyCD's images can be offered there, and that a game core reading the CD gets the image unchanged.
- **Enhanced CD** data tracks shown as files (read-only).
- **HDCD and pre-emphasis** flags honoured (de-emphasis filter when the flag is set).

---

<sub>Future plans, written 2026-11-02. Not built, not tested.</sub>
