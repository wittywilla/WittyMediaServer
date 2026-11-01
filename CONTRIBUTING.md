# Contributing to WittyMediaServer

For developers changing the server. This page covers how to build it, test it and send a change, even if you haven't worked on a Go or React project before.

## What you need

| Tool | Version | Get it |
|---|---|---|
| Go | 1.26 or newer | [go.dev](https://go.dev/dl/) |
| Node.js | 22 or newer | [nodejs.org](https://nodejs.org/) |
| FFmpeg | any recent build | [ffmpeg.org](https://ffmpeg.org/download.html), or the copy inside a release archive |
| Git | any | [git-scm.com](https://git-scm.com/downloads) |

FFmpeg is needed to run the server. Tests that stream through it skip without it.

## How the code is split

The server and its modules are separate projects. **Discs, games, photos, readers, tuners and the rest each live in a repository of their own**, and a server installs them at run time. The server's source holds none of their code. [Repositories](docs/REPOSITORIES.md) lists them all, and [Modules](docs/MODULES.md) explains how they fit together.

To change a module, work in its repository. A change that crosses the line is two changes in two repositories: see [Developing a server and a module together](docs/MODULES.md#developing-a-server-and-a-module-together).

## Build and run

The server needs only one other repository to build, WittyModule. Clone it beside this one, and run `dev-modules.mjs` once in each checkout:

```bash
git clone https://github.com/wittywilla/WittyModule
git clone https://github.com/wittywilla/WittyMediaServer
cd WittyMediaServer
node scripts/dev-modules.mjs
cd web && npm install && npm run build && cd ..
go build -o wittymediaserver ./cmd/wittymediaserver
./wittymediaserver serve -ffmpeg /path/to/ffmpeg -data ./devdata -listen 127.0.0.1:9797
```

Open `http://localhost:9797`. Always use a `-data` folder of its own while developing, so your real library is never touched. If you're testing against media you care about, also turn off **Settings → Metadata agents → Save information and artwork next to your media**, which is on by default.

**What `dev-modules.mjs` does.** It finds the repositories beside this checkout, and writes a `go.work` and `node_modules` links that git ignores. Clone any other module repositories beside it first if you want them too. If Go says a directory "does not contain modules listed in go.work", run it again. **Never run `go work use`**, because this script remakes that file. `node scripts/dev-modules.mjs WittyPlay=<folder>` points at another checkout of one module, and `--off` removes what it made. [Working on them](docs/REPOSITORIES.md#working-on-them) has the rest.

**Trying real modules on your build.** A build tagged `updatetest` reads a catalogue you make yourself, signed with a throwaway key. See [Developing a server and a module together](docs/MODULES.md#developing-a-server-and-a-module-together).

**Working on the web interface?** Run the server with `-dev`, which serves `web/dist` from disk, and rebuild with `npm run build` after each change. Or run `npm run dev` in `web/` for hot reload, which proxies `/api` to `127.0.0.1:9797`.

## Tests and checks

**Run the full tests on Linux.** On Windows, that means WSL. CI runs on Linux, and a test that listens on a network address makes Windows' firewall prompt for it, which looks like a hang. If you test on Windows, bind only to `127.0.0.1`.

In WSL, use a clone on WSL's own disk, because a worktree's git link is a Windows path that WSL can't follow:

```bash
git clone /mnt/d/projects/WittyMediaServer ~/wms-my-branch
cd ~/wms-my-branch
git fetch origin my-branch && git checkout FETCH_HEAD
node scripts/dev-modules.mjs --repos /mnt/d/projects
```

Change the paths to wherever your repositories are. Then, before you send a change:

```bash
gofmt -l .
go vet ./...
go test ./... -count=1 -timeout 30m
cd web && npx tsc --noEmit -p . && npm run build
```

`gofmt -l .` must print nothing. The tests need more than Go's default 10 minutes: `internal/server` alone is slow, and takes longer on a busy machine.

CI also runs these, which you can run yourself from the top of the repository:

```bash
python3 scripts/theme-contrast.py && python3 scripts/design-tokens.py --check && python3 scripts/design-check.py
node --experimental-strip-types --test web/src/ui/spatial.test.mjs web/src/ui/themeMaker.test.mjs web/src/shell/navModel.test.mjs web/src/safeLink.test.mjs
```

**Tests of what the server does with a real module** need a catalogue, and are skipped without one. Make it with `scripts/make-test-catalog.sh`, then set `WMS_TEST_CATALOG` to its folder.

## Where things are

| Folder | What it is |
|---|---|
| `cmd/wittymediaserver` | The server and its command line |
| `cmd/wittymediaserver-tray` | The app people open: the tray or menu-bar icon |
| `cmd/wittymediaserver-uninstall` | The uninstaller with windows and buttons, on Windows and macOS |
| `cmd/wms-forwarder` | Ships as `wms` in archives only, so a copy of 0.5.2-alpha can update |
| `internal/server` | The HTTP API |
| `internal/library` | Libraries, the scanner, TV parsing, metadata agents, NFO reading and writing |
| `internal/modules` | The module manager, the catalogue and the helpers' sandbox |
| `internal/hls`, `internal/ffmpeg` | On-demand transcoding, and choosing an encoder |
| `internal/dvdsession`, `internal/bdsession` | Turning a disc engine's steps into what the browser shows |
| `internal/play` | The server's side of games: generated stubs for WittyPlay |
| `internal/mediatypes` | Every file type. [Supported formats](docs/SUPPORTED-FORMATS.md) is generated from it |
| `internal/layout` | What a package holds, and what each program is called |
| `internal/uninstall` | Removing everything, on every system |
| `internal/themes`, `themes/` | Theme ZIP handling, and the built-in themes |
| `packaging/` | The installers: MSI, `.deb`, `.rpm`, `.pkg` |
| `scripts/` | Developer tools, none of which ship: [its README](scripts/README.md) |
| `web/src` | The React and TypeScript interface |
| `docs/` | The guides. The web app bundles them as its Help |

## Guidelines

- **Docs.** The guides in `docs/` are what people read inside the app. New settings need a sensible default, a tooltip in the UI and a line in the guides. Some docs are read by tests, such as [the client API](docs/CLIENT-API.md) and [the route table](docs/SECURITY-ROUTES.md).
- **Routes.** Every route checks who's asking, and gets a row in [the route table](docs/SECURITY-ROUTES.md). A test fails for a new route with none.
- **Modules.** Don't import module code into `internal/` or `cmd/`: the server has to work with none of them there.
- **Disc engines.** They live in their own repositories, and stay free of other players' code, because they're written from the public disc formats.
- **Disc bugs.** Attach a report from **Settings → WittyDiagnoser**, or the output of `wittydvd crawl` or `wittybluray walk` from those repositories. Never attach disc images.
- **Style.** Match the code around you, and explain *why* in comments, not *what*. Interface work follows [the design guide](docs/DESIGN.md).
- **Legal notes.** A new screen that shows third-party or web content, or where people add or share media, takes a `<LegalNote>`: [the checklist](docs/LEGAL-NOTICES-CHECKLIST.md) says which. A new outside service goes on the privacy list in `internal/legal/legal.json`, and a test fails otherwise.
- **Examples.** No real films, shows, music, games or people in screenshots, examples or test files. Make something up, or use openly licensed media.
- **Copy protection.** Encrypted media is detected and refused. Never add a way around it, or a plugin hook for one, and never link to ROMs or BIOS files.
- **Licences.** Contributions are under the Apache License 2.0 (see `LICENSE`). A new third-party component goes in `THIRD_PARTY_LICENSES.md`.
- **Versions and changes.** Add what users will notice to `CHANGELOG.md`. `scripts/bump-server.sh` raises the server's version everywhere it's written, and a bump isn't a release.

## Sending a change

1. Fork the repository on GitHub and clone your fork.
2. Create a branch: `git checkout -b fix-menu-highlight`.
3. Commit with a clear message, push, and open a pull request against `master`.
4. CI runs the checks above.

New to Git and GitHub? [Publishing changes](docs/PUBLISHING.md) walks through it.

---

<sub>WittyMediaServer documentation</sub>
