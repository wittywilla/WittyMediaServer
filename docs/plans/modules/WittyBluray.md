# WittyBluray: update plan

> **Status on 2026-11-04**
>
> **Planned:** [icons](https://github.com/wittywilla/WittyMediaServer/discussions/58) and [secondary audio and picture in picture](https://github.com/wittywilla/WittyMediaServer/discussions/59). Nothing in progress.

**Now:** 0.3.0. Helper, API 3. Blu-ray images with their menus, including 4K, HDR and 3D discs (3D plays the 2D view); BD-J menus on a Java the owner installs, with our own AGPL-3.0-or-later `runtime.jar` and a default-deny sandbox, off by default; insert effect; `wittybluray walk` and `bdj` diagnostics. Built on WittyDisc. `wittyblurayplayer` 0.2.0 is used by WittyMediaPlayer. AACS-encrypted discs are refused.

**Channel:** alpha (with Module API 4).

| Release | Theme |
|---|---|
| 0.3.x | Fixes on API 3 |
| **0.4.0** (next major) | Matching disc icon, and the items below |
| Later | Secondary audio, picture in picture, MVC 3D, retail BD-J |

---

## 0.4.0

### The icon

A disc with **B** in the centre, matching WittyDVD's **D**, with no format logo: no "BD", no oval, no blue wordmark styling. [../design/module-icons.md](../design/module-icons.md).

### Also in 0.4.0

- **Shared insert animation engine** with WittyDVD and WittyPlay (see WittyDVD's plan).
- **BD-J graphics smoother:** today the disc's graphics reach the player as pictures at up to 12 a second. Measure, then raise the rate where the machine allows (dirty-rectangle updates rather than whole frames), with the cap kept for low-end machines.
- **BD-J security:** every allow-listed package has been read class by class (the rule after java.rmi, XML and ICC_Profile holes); re-run that review for anything added, and fuzz the class-file rewriter.
- **Java detection** clearer in Settings: which Java was found, its version, and a link to the help page on installing one (no Java is ever shipped).

## Later

- **Secondary audio and picture-in-picture mixed in** (roadmap: not built).
- **MVC 3D:** FFmpeg has no MVC decoder. Watch for one with a usable licence; until then 2D, and say so.
- **A retail BD-J disc:** never run (the owner's two Blu-ray images have none). Don't describe BD-J as retail-proven until one runs. This is the gate for the beta channel.
- **UHD HDR on HDR screens**, tone mapping on others: real screens needed.
- **WittyMediaPlayer: Java menus** (roadmap: not started).

## Standing rule

AACS discs stay refused. No keys, no bypass, no hook.

---

<sub>Future plans, written 2026-11-02. Not built, not tested.</sub>
