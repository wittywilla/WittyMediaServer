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
| `packaging/linux` | The `.deb` and `.rpm`, made with nFPM. Their service, scripts and repository layout |
| `packaging/linux/appimage`, `flatpak`, `snap` | The AppImage, the Flatpak and the snap, each made from the same folder by a script of its own. `launcher.sh` is what the first two run. None has been built or run yet |
| `packaging/macos` | The Mac `.pkg`, made with Apple's tools. Needs a Mac |
| `packaging/freebsd` | The FreeBSD `.pkg`, made with `pkg create`, and the rc.d service. Needs FreeBSD |
| `packaging/install.sh` | The service installer in the Linux and FreeBSD `.tar.gz` |
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
| `genicon/` | Draws every logo, icon and background of the family (apps, installers, modules, website), from `internal/brandart`. See [docs/BRAND.md](../docs/BRAND.md) |
| `sync-brand.sh` | Brings `web/public`'s icons up to date with genicon (`--check` only checks) |
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
| `../packaging/linux/make-repository.sh` | Makes an apt and dnf repository from the packages. Proven by hand with `apt-get install` and `dnf install` from what it made |
| `test/` | Really installs, upgrades and removes each installer on a throwaway machine: [its README](test/README.md). Not run by CI, as they take a while: run them before a release |
| `design-check.py`, `design-check-baseline.json`, `design-tokens.py` | Hold the web app to its design standards. CI runs them. See [docs/DESIGN.md](../docs/DESIGN.md) |
| `theme-contrast.py` | Checks the themes' contrast |
| `ci-test-report.py` | Turns a failed test run into a short report in CI |
| `check-executable.sh` | Fails when a script is in git without its executable bit, which commits from Windows lose. CI runs it |
| `soak.py` | A long soak run: keeps a test server busy for hours and fails on slow leaks or slow answers. See [Soak runs](#soak-runs) |

## Soak runs

Every test is minutes long, so a leak that takes hours, a goroutine or timer that piles up, or a lock only reached after a while, is never seen by them. `soak.py` keeps a test server busy the way a household would, for as long as you say (4 hours unless told otherwise), and watches it.

**What it does:** music with online lyrics (from a stand-in lyrics service that answers quickly, slowly, with nothing, or not at all, so nothing goes online), skipping through songs now and then; a 1080p film converted while it plays; a library channel; a DVD menu when the DVD module is installed; a scan of every library every 30 minutes; and the web app's pages from four clients.

**What it records, every minute:** the Go heap and the process's memory, goroutines, open files, FFmpeg processes, and how long `/api/health` and a library page take to answer. They go in `samples.csv`.

**When it fails:** a measure that keeps rising (its last third 25% above its first, with the middle in between, after a 20-minute warm-up); `/api/health` slower than 2 seconds or a library page slower than 5 seconds, even once; FFmpeg still running 2½ minutes after everyone has left; the server stopping, or not stopping within a minute when asked; or its log saying it stopped answering, crashed, or found a data race. It writes `report.md` and `graph.svg` either way.

On Linux, or in WSL, with the demo media:

```bash
go build -o /tmp/wms ./cmd/wittymediaserver
python3 scripts/soak.py --binary /tmp/wms --media /mnt/d/WittyDemoMedia --out /tmp/soak --hours 4
```

Without the demo media, `--make-media` makes a small library with FFmpeg first. `--minutes 10` is a quick try; `--help` lists the limits. The server gets its own data folder in `--out`, so it can't touch another one, and it listens on 127.0.0.1 only.

**On GitHub:** **Actions → soak → Run workflow**, with the hours (up to 5; GitHub stops a job at 6). It uses `--make-media`, and the report is in the run's summary and its `soak-report` download.

Open files and FFmpeg processes are read from `/proc`, so the run is for Linux. On Windows the rest works, and open files are the process's handles.
