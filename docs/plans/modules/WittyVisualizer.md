# WittyVisualizer: update plan

> **Status on 2026-11-04**
>
> **Planned.** Nothing in progress.

**Now:** 0.2.1. Web module, API 3. Light shows that follow the music, full screen: bars, scopes, plasma, tunnels, starfields, feedback, fire; shuffle; calm mode; a flash limiter (per-tile brightness steps, beat rate caps) with an honest notice; custom effects as untrusted data with a GLSL subset and a cost limit. Appears in the server's audio player and WittyMediaPlayer.

**Channel:** alpha.

| Release | Theme |
|---|---|
| 0.2.x | Owner's checks: the visualizer button easy to see, no auto transition when not wanted, full-screen music with it in the background (all 0.6.0 asks; verify by eye with sound playing, which 0.6.0 never heard) |
| **0.3.0** | WittyVisualizer in the native apps (roadmap: not started), via a web view first; effects that react to lyrics timing (from the lyrics feature) |
| Later | A small in-browser editor for custom effects (writes the same data file); sharing effects between profiles on one server (admin approves, the same checks) |

**Rule kept:** the flash limiter stays on and can't be turned off for profiles marked as kids; the notice stays honest that no limiter makes moving light safe for everyone.

---

<sub>Future plans, written 2026-11-02. Not built, not tested.</sub>
