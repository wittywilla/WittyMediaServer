# Game PCs: WittyCouch, WittyCouchHost and WittyRigs: update plans

> **Status on 2026-11-04**
>
> **Planned.** The first real test on a game PC still needs the owner. How much these lean on the server is part of [#98](https://github.com/wittywilla/WittyMediaServer/issues/98).

| Module | Now | Licence | Does |
|---|---|---|---|
| WittyCouch | 0.2.1 | Apache-2.0 | Play the games on any PC running Sunshine, in your browser; hands off to the separately installed Moonlight app where a page can't play |
| WittyCouchHost | 0.2.0 | **GPL-3.0** (built on moonlight-common-c) | The separate program that carries the streams; talks to WittyCouch only over pipes in its `PROTOCOL.md` |
| WittyRigs | 0.2.0 | Apache-2.0 | Advanced: host demanding PC games on the server's own hardware (Sunshine stays a separate GPL program it only looks for) |

All on the **alpha** channel. The GPL boundary stays exactly as the licence policy set it: no GPL code compiled into the server, the client or other projects.

**Never exercised for real:** game PC streaming against a real PC (0.5.2 and 0.6.0 both said so). WittyRigs has never hosted a real game. WittyCouchHost has no FreeBSD build; its macOS build has never run.

---

## Next (0.2.x / 0.3.0)

1. **The first real test** ([the checklist](../../GAME-RIGS.md#a-checklist-for-the-first-real-test) in `docs/GAME-RIGS.md`): a gaming PC with Sunshine, on the owner's network. `needs-owner` issue with step-by-step instructions.
2. **Fallback for networks that block UDP**, HEVC and AV1, and adaptive bitrate (roadmap: not built).
3. **WittyCouchHost on FreeBSD** (follow-up from modules-cutoff) and a first real run on macOS.
4. **Share the measuring tools** from WittyPlay's streaming work (stamps, overlay, camera rig) so game PCs are measured the same way.

## Later

- **Latency work from WittyPlay 0.4.x to 0.9.x** applied where the protocol allows: client-side sound buffer, direct drawing, VRR, native low-latency decoders. Anything that would need GPL code stays inside WittyCouchHost.
- **WittyRigs:** a guided setup that checks the GPU, driver and encoder, and says plainly what the machine can host.
- **Wake-on-LAN** for game PCs that sleep (the PC's own setting, sent from the server).

---

<sub>Future plans, written 2026-11-02. Not built, not tested.</sub>
