# WittyMediaPlayer: update plan

> **Status on 2026-11-04**
>
> **Open:** the release build, [WittyMediaPlayer#1](https://github.com/wittywilla/WittyMediaPlayer/issues/1). **Planned:** [macOS and Linux](https://github.com/wittywilla/WittyMediaServer/discussions/63).

**Now:** 0.1.2 tagged, **not released**. Qt desktop player for Windows only, with FFmpeg built with no network code, DVD and Blu-ray menus through `wittydvdplayer` and `wittyblurayplayer` 0.2.0 (API 3), CD look-ups, the visualizer, and an optional VLC path loaded from the user's own VLC (credited, off by default, never used to open discs). Its release build crashes in Qt's `mingw32-make` on GitHub's Windows runners; the MSYS2 fallback didn't take effect.

| Release | Theme |
|---|---|
| **0.1.3** (first, with 0.6.3) | A release build that works on GitHub: pin a runner image known to work, or switch the build to Ninja (CMake's `-G Ninja`), or cache a known-good toolchain; then publish (a new number, as tags never move) |
| 0.2.0 | Single instance (a second launch passes the file to the running player); built-in surround sound (roadmap); the shared insert animation for discs |
| 0.3.0 | **macOS and Linux** builds (roadmap: not started; nothing in the design stops it); installers and uninstallers for each |
| Later | Java (BD-J) menus (roadmap: not started); playing from a WittyMediaServer library as well as files |

**Rules kept:** VLC stays optional, loaded at run time from the user's VLC, not shipped, never chosen for discs; protected discs refused before playback.

---

<sub>Future plans, written 2026-11-02. Not built, not tested.</sub>
