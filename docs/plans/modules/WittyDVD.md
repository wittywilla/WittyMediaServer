# WittyDVD: update plan

> **Status on 2026-11-04**
>
> **Planned:** 0.4.0 and its icon, [the idea](https://github.com/wittywilla/WittyMediaServer/discussions/58). Nothing in progress.

**Now:** 0.3.1. Helper, API 3. DVD images with their real menus, chapters, angles and subtitles; insert effect with tray animation and drive sound; `wittydvd walk` and `crawl` diagnostics; DVD main feature and scenes on WittyTV channels. Built on WittyDisc. Its player engine `wittydvdplayer` 0.2.0 is used by WittyMediaPlayer. Encrypted (CSS) discs are refused.

**Channel:** alpha (with Module API 4).

| Release | Theme |
|---|---|
| 0.3.x | Fixes on API 3 |
| **0.4.0** (next major) | Matching disc icon, and the items below |
| Later | See the list at the end |

---

## 0.4.0

### The icon

The owner: both disc modules get disc images that match, with **D** in the centre for WittyDVD and **B** for WittyBluray, and no copyrighted logo. Full design: [../design/module-icons.md](../design/module-icons.md). Lands as a catalogue change at the same time as WittyBluray's.

### Also in 0.4.0, from what's known

- **The insert effect shared with WittyPlay's new animation engine,** so DVDs, Blu-rays and games use one timing engine, one reduced-motion path and one settings model. The DVD tray stays its own drawing. Option "Only while loading" like games.
- **Menu rendering on low-end devices:** measure menu highlight latency and the cost of animated menus on a 2 GB TV box; keep it smooth (performance first).
- **Remote and controller in menus:** the final accessibility pass (0.6.3) checks DVD menus by arrows and controllers; fix what it finds.
- **Real-world checks** listed in the README for the beta channel later: PAL and NTSC, multi-angle, seamless branching, discs with odd menu programs. The owner's Shrek discs are the reference set (Shrek 2 for in-title interactive features).
- **Scenes on channels:** a scene picker that previews the clip (exists: `POST /api/channels/clip-preview`); add marking several scenes in one pass.

## Later

- **WittyMediaPlayer on macOS and Linux** needs `wittydvdplayer` built there.
- **Disc folders (VIDEO_TS) on network shares** read as fast as images: measure and cache.
- **Subtitles:** better VobSub to text (OCR, optional, local) for players that can't show pictures.
- **Bookmarks inside a disc** per profile ("resume this disc at the title and time I left").

## Standing rule

CSS-encrypted discs stay refused, with a clear message. No libdvdcss, no bypass, no hook. WittyMediaPlayer's optional VLC path never opens a disc through VLC's disc modules.

---

<sub>Future plans, written 2026-11-02. Not built, not tested.</sub>
