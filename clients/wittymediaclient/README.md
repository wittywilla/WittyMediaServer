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

## Logs

Each app keeps a small log of its own, and reports errors to the server it is connected to. Those reports go
to that server and nowhere else: there is no telemetry and no other address. The owner can turn them off with
"App errors" in the server's settings, and reads them in the server's log, under `clients`.

| App | The file | How to reach it |
|---|---|---|
| Desktop | `logs/client.log` in the app's data folder, 2 MB and three files at most | **Help** in the title bar: "Open logs folder" and "Copy diagnostic info". The connect screen has the link too |
| Android | `logs/client.log` in the app's private storage, 1 MB and three files at most | Not yet: no screen for it. Errors reach the server's log |
| Apple | `logs/client.log` in the app's caches, 1 MB and three files at most | Not yet: no screen for it. Errors reach the server's log |

**What is kept and sent:** crashes, what stopped working in the app (with its reason and exit code), failed requests
with their request ID, playback errors with their stream ID, and failed connections, plus the app's version and
the system's name and version. The desktop app's file also has the address that failed, which isn't sent. The server's
page reports its own errors, so the desktop app doesn't repeat them.

**What never is:** tokens, passwords, cookies, stream addresses' secrets and file contents. Text is cleaned before it's
written anywhere, by the rules tested in `shared/data/redaction-cases.json`.

A report that fails is dropped. Repeats are grouped with a count, and nothing is ever tried again in a loop.

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

## Legal

WittyMediaClient is an independent open-source project. It isn't associated with, sponsored by or
endorsed by any device or platform maker, publisher, rights holder or service. It connects to a
server its owner runs. What's on that server is theirs, not this project's: the project, its
contributors and affiliates don't provide, host, endorse or check it. Any third-party names here
only describe what it works with, and belong to their owners. It's provided “as is”, with no
warranty; see [LICENSE](LICENSE). Local law applies to how you use it. This isn't legal advice.
