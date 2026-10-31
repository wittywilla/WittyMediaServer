<h1 align="center">WittyMoonlight</h1>

<p align="center">
  <b>Get from "Play" to a game on your own PC, on whatever device you're holding.</b>
</p>

> **Not an official project.** WittyMoonlight is not part of, or made by, the [Moonlight](https://moonlight-stream.org)
> project or the [Sunshine](https://github.com/LizardByte/Sunshine) project (LizardByte). It contains none of their code.
> It hands off to separately installed programs.

> Read-only mirror of `modules/wittymoonlight` in [WittyMediaServer](https://github.com/wittywilla/WittyMediaServer),
> where development happens. Please open issues and pull requests there.

## What it does

| Part | What for |
|---|---|
| `Plan` | What really works on a device, best first: Moonlight's command line, an Android intent, a bridge link, or the steps to take by hand |
| `InstallFor` | Where a device gets Moonlight, from its own project |
| `Bridge` | Downloads [moonlight-web-stream](https://github.com/MrCreativ3001/moonlight-web-stream) at an admin's request, from its own release, checked against a recorded digest, and runs it as a separate program on the loopback address |
| `Proxy` | The only way to that program: a one-time ticket for one game on one PC, an allow-list of requests, and every stream request read before it goes on |

## How it has been tested

Automated tests cover `Plan`, unpacking archives that try to climb out, and everything in `Proxy`, against a stand-in. **The real bridge
program hasn't been run yet, and no real Moonlight app has been handed a game.** See `docs/GAME-RIGS.md` in WittyMediaServer.

## Licence

Apache-2.0: see [LICENSE](LICENSE) and [NOTICE](NOTICE). Moonlight and moonlight-web-stream are GPL-3.0, and stay separate programs that
are never linked, copied or bundled.
