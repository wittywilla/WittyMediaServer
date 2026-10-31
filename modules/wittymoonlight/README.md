<h1 align="center">WittyMoonlight</h1>

<p align="center">
  <b>Play the games on any PC running Sunshine, in your browser.</b><br>
  A client protocol in Go, a WebRTC relay, and a player with controllers, keyboard, mouse and touch.
</p>

> **Not an official project.** WittyMoonlight isn't part of, or made by, the [Moonlight](https://moonlight-stream.org) project or the
> [Sunshine](https://github.com/LizardByte/Sunshine) project (LizardByte).

> Read-only mirror of `modules/wittymoonlight` in [WittyMediaServer](https://github.com/wittywilla/WittyMediaServer),
> where development happens. Please open issues and pull requests there.

## What's here

| Part | What for |
|---|---|
| `gamestream` | A PC's client API: `Browse` finds PCs, `Rig.Pair` the PIN flow with its certificate pinned, `Apps`, `AppImage`, `Launch`, `Resume`, `Quit`, `Wake`, and the rules on which addresses may be reached. `gamestream/fake` is a stand-in for Sunshine, for tests |
| `relay` | Runs [WittyMoonlight Host](https://github.com/wittywilla/WittyMoonlightHost) and carries its frames to a browser over WebRTC, without decoding them, and input back |
| `web` | The player. Its host app provides the API through `MoonlightHost` |
| `plan.go` | What each device can do when it can't play in a page: handing off to the separately installed Moonlight app |

## Licences

**This module is Apache-2.0**: see [LICENSE](LICENSE) and [NOTICE](NOTICE). It's written from the protocol and the browser's APIs, and contains no Moonlight code.

To stream, it starts **WittyMoonlight Host**, a separate program under the **GPL-3.0**, built on Moonlight's own [moonlight-common-c](https://github.com/moonlight-stream/moonlight-common-c). The two exchange only the messages in that program's `PROTOCOL.md`, over pipes. Without it, everything but playing still works.

## How it has been tested

Automated tests run `gamestream` against `fake`, and `relay` over real WebRTC with a stand-in for the host program. `gamestream/real_test.go` and `relay/real_test.go` run against a real Sunshine, and a real browser has played a real stream end to end. Real games on real gaming PCs haven't been tried yet: see `docs/GAME-RIGS.md` in WittyMediaServer.
