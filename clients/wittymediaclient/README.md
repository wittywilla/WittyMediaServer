# WittyMediaClient

Apps that connect to a [WittyMediaServer](../../README.md): TV boxes, phones, tablets and desktops.

**Start with [docs/PLATFORMS.md](docs/PLATFORMS.md)**, which says exactly what works and what has
never been run. Most of this is early, and the native apps have not been compiled yet.

| Folder | What it is |
|---|---|
| [`desktop/`](desktop) | Windows and Linux: a frameless window around the server's web interface |
| [`android/`](android) | Android TV, Google TV, Fire TV, phones and tablets (Kotlin, Compose, Media3) |
| [`apple/`](apple) | Apple TV, iPhone, iPad and Mac (SwiftUI, AVPlayer) |
| [`shared/`](shared) | What every app shares, as plain data: themes, wording, button meanings and test cases |
| [`docs/`](docs) | [Design](docs/DESIGN.md), [status](docs/PLATFORMS.md), [what the owner needs to do](docs/OWNER-STEPS.md) |

The native apps talk to the API described in `docs/CLIENT-API.md` at the top of this repository.

## Trying the desktop app

You need [Node.js](https://nodejs.org) 22.12 or newer, and a WittyMediaServer to connect to.

```bash
cd clients/wittymediaclient/desktop
npm ci
npm start
```

## Tests

```bash
cd clients/wittymediaclient/desktop
npm test
```

```bash
cd clients/wittymediaclient/apple/WittyKit
swift test
```

```bash
cd clients/wittymediaclient/android
gradle testDebugUnitTest
```

The sample server answers used by those tests are checked against a real server with
`node shared/scripts/check-fixtures.mjs <server address> <token>`. After changing any theme, rerun
`node shared/scripts/build-themes.mjs`; CI fails if `shared/data/themes.json` is out of date.
