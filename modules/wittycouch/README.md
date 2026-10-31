<h1 align="center">WittyCouch</h1>

<p align="center">
  <b>Play the games on any PC running Sunshine, in your browser.</b><br>
  A client protocol in Go, a WebRTC relay, and a player with controllers, keyboard, mouse and touch.
</p>

> **Not an official project.** WittyCouch isn't part of, or made by, the [Moonlight](https://moonlight-stream.org) project or the
> [Sunshine](https://github.com/LizardByte/Sunshine) project (LizardByte).

> Read-only mirror of `modules/wittycouch` in [WittyMediaServer](https://github.com/wittywilla/WittyMediaServer),
> where development happens. Please open issues and pull requests there.

## What's here

| Part | What for |
|---|---|
| `gamestream` | A PC's client API: `Browse` finds PCs, `Rig.Pair` the PIN flow with its certificate pinned, `Apps`, `AppImage`, `Launch`, `Resume`, `Quit`, `Wake`, and the rules on which addresses may be reached. `gamestream/fake` is a stand-in for Sunshine, for tests |
| `relay` | Runs [WittyCouch Host](https://github.com/wittywilla/WittyCouchHost) and carries its frames to a browser over WebRTC, without decoding them, and input back |
| `web` | The player. Its host app provides the API through `CouchHost` |
| `plan.go` | What each device can do when it can't play in a page: handing off to the separately installed Moonlight app |

## Licences

**This module is Apache-2.0**: see [LICENSE](LICENSE) and [NOTICE](NOTICE). It's written from the protocol and the browser's APIs, and contains no Moonlight code.

To stream, it starts **WittyCouch Host**, a separate program under the **GPL-3.0**, built on Moonlight's own [moonlight-common-c](https://github.com/moonlight-stream/moonlight-common-c). The two exchange only the messages in that program's `PROTOCOL.md`, over pipes. Without it, everything but playing still works.

## How it has been tested

Automated tests run `gamestream` against `fake`, and `relay` over real WebRTC with a stand-in for the host program. `gamestream/real_test.go` and `relay/real_test.go` run against a real Sunshine, and a real browser has played a real stream end to end. Real games on real gaming PCs haven't been tried yet: see `docs/GAME-RIGS.md` in WittyMediaServer.

## Legal

WittyCouch is an independent open-source project. It isn't associated with, sponsored by or endorsed by
the Moonlight or Sunshine projects, NVIDIA, or any game publisher. It connects to a PC someone else may
run, and what that PC offers isn't provided or checked by this project. No games come with it. Any
third-party names here only describe what it works with, and belong to their owners. It's provided
“as is”, with no warranty; see [LICENSE](LICENSE). Local law applies to how you use it. This isn't legal
advice.

<!-- module-kit -->
## Releasing

WittyCouch reaches a server inside the projects built with it, so a release has no files of its own: a version tag runs the tests and publishes a release that those projects can require.

Versions move in small steps as work lands. `go run github.com/wittywilla/wittymodule/cmd/wittymodule@v0.2.1 bump .` raises it
everywhere it is written (add `-part minor` for a bigger step). A bump isn't a release: nothing is
tagged or published until you tag it. CI fails when code changed since the last tag and the version
didn't, because tags never move.

To release:

1. Check that `version` in `module.json` is the one to publish, with the bump committed.
2. Tag that commit and push the tag: `git tag vX.Y.Z`, then `git push origin vX.Y.Z`. A tag with a
   suffix, such as `v1.2.0-beta`, is published as a prerelease.
3. Watch the *Actions* tab. There is nothing to sign.

Licensed under Apache-2.0: see [LICENSE](LICENSE) and [NOTICE](NOTICE).
<!-- /module-kit -->
