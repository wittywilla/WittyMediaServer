# Developer tools

Nothing in this folder ships to people, apart from what a build makes. What does ship is in `cmd/` and `packaging/`.

## Where things are

| Folder | What's in it |
|---|---|
| `cmd/wittymediaserver` | The server and its command line: `bin/wittymediaserver` in a package |
| `cmd/wittymediaserver-tray` | The app people open: `WittyMediaServer` in a package |
| `cmd/wittymediaserver-uninstall` | The uninstaller with windows and buttons, on Windows and macOS |
| `cmd/wms-forwarder` | Ships as `wms` in the `.zip` and `.tar.gz` only, so a copy of 0.5.2-alpha can update. Goes once that's past |
| `packaging/windows` | The `.msi`, made with the WiX Toolset |
| `packaging/linux` | The `.deb` and `.rpm`, made with nFPM. Their service, scripts and repository layout, and the archives' `install.sh` |
| `packaging/macos` | The `.pkg`, made with Apple's tools. Needs a Mac |
| `deploy/k8s` | A Kubernetes setup |
| `Dockerfile`, `docker-compose.yml` | The Docker image, and how to run it |
| `scripts/` | This folder |

What a package holds, and the name of everything in it, is in one place: `internal/layout`. Removing all of it is in `internal/uninstall`.

## How programs are named

- What a person opens is named for the product, with capitals, in the top folder: `WittyMediaServer`.
- The command line is the product's name in lower case, on every system: `wittymediaserver`.
- Everything else goes in `bin/`, named `<product or module>-<job>`, in lower case: `wittyplay-game-runner`.
- On Windows each program also says what it is in its Properties and in Task Manager: `winres` in `build-release.sh` puts that in.
- Developer tools never go in a package.

## Building

| Script | What it does |
|---|---|
| `build-release.sh` | Builds the package folder and archive for each platform. `build-release.ps1` runs it from PowerShell. Unsigned unless `SIGN=1` |
| `build-ffmpeg-static.sh` | A static FFmpeg for FreeBSD, which has no prebuilt one. Run on FreeBSD; the `ffmpeg-freebsd` workflow does |
| `bump-server.sh` | Raises the server's version everywhere it's written. A bump isn't a release |
| `dev-modules.mjs` | Run once in each checkout: makes `go.work` and links the module repositories beside it. See [docs/REPOSITORIES.md](../docs/REPOSITORIES.md) |
| `genicon/` | Draws the icon, and the pictures in the Windows installer's wizard |
| `pack/` | Writes `.zip` and `.tar.gz` with the right permissions from any system |

## Signing

| Script | What it does |
|---|---|
| `sign/` | Makes and uses the key that signs a release's checksum list: `go run ./scripts/sign`. See [docs/PUBLISHING.md](../docs/PUBLISHING.md) |
| `new-module-key.sh` | Makes the separate key that signs modules |
| `set-module-secret.sh` | Gives the module repositories that key on GitHub |

## Checking

| Script | What it does |
|---|---|
| `test/` | Really installs, upgrades and removes each installer on a throwaway machine: [its README](test/README.md). Not run by CI, as they take a while: run them before a release |
| `design-check.py`, `design-check-baseline.json`, `design-tokens.py` | Hold the web app to its design standards. CI runs them. See [docs/DESIGN.md](../docs/DESIGN.md) |
| `theme-contrast.py` | Checks the themes' contrast |
| `ci-test-report.py` | Turns a failed test run into a short report in CI |
