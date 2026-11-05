# Working on a Mac

How to set up a Mac from nothing to build, run, test and package WittyMediaServer, where it keeps its files on a Mac, and what hasn't worked or hasn't been tried there yet.

This is for whoever develops on a Mac. To just install the server on one, see [Installing: macOS](INSTALL.md#macos). The plan for the port itself, and the state of every repository, is in the [macOS port hand-over](handover/MACOS-PORT.md).

- [Where macOS stands](#where-macos-stands)
- [Setting up a Mac from nothing](#setting-up-a-mac-from-nothing)
- [Building and running the server](#building-and-running-the-server)
- [Tests](#tests)
- [Packaging](#packaging)
- [Signing and notarising](#signing-and-notarising)
- [Where everything lives](#where-everything-lives)
- [The background service (launchd)](#the-background-service-launchd)
- [Firewall, privacy and sleep](#firewall-privacy-and-sleep)
- [Known not to work, or not tried yet](#known-not-to-work-or-not-tried-yet)

## Where macOS stands

As of 0.7.0-alpha, 2026-11-04. "CI" means GitHub's Mac runners: virtual Macs, Apple silicon (`macos-latest`) and Intel (`macos-15-intel`), with no screen, no person signed in at a desktop, and the firewall off.

| Part | Built | Run in CI | Run on a real Mac |
|---|---|---|---|
| Server (`wittymediaserver`) | Every pull request, both chips | `go vet` and every test, on Apple silicon; `version` on both | No |
| Menu-bar app (`WittyMediaServer.app`) | Every pull request and release, on a Mac (it needs CGO for Cocoa) | No: there's no menu bar to show it in | No |
| Uninstaller app | Every pull request and release | Its command-line form, `wittymediaserver --uninstall` | No |
| `.pkg` installer | Every pull request and release, unsigned | Installed, the service answers on port 9797, uninstalled, on both chips | No |
| Bundled FFmpeg | Downloaded and checked against `scripts/ffmpeg.lock` | Runs, and lists VideoToolbox's encoders | No |
| Module helpers (every module) | Every module's CI cross-compiles darwin/amd64 and darwin/arm64 | No | No |
| Helper sandbox (`sandbox-exec`) | Yes | `TestMacHelperSandbox`: writing, removing, the network, Unix sockets and starting programs were all refused inside it, and a helper allowed more kept it | No |
| Games (WittyPlay) | Yes | No | No |
| WittyMediaClient desktop (`.dmg`) | Every push, on a Mac | No | No |
| WittyMediaClient for Mac, iPhone, iPad, Apple TV | On a Mac in CI, unsigned | WittyKit's tests | No |
| WittyReader (`.dmg`) | Every release, on a Mac | No | No |
| WittyCouch Host | Every push, both chips | `--version`, both chips | No |
| WittyMediaPlayer | Not for macOS: Windows only | | |

So nothing has been seen working on a real Mac, with a screen, a person and a real library. That's the port's first job.

## Setting up a Mac from nothing

Everything below is free. It needs an administrator's password a few times.

### 1. Apple's command line tools

Open **Terminal** (in Applications, Utilities) and run:

```bash
xcode-select --install
```

Choose **Install** in the window that opens. This gives `git`, `clang`, `make` and `python3`, which the server and its scripts need. CGO, which the menu-bar app uses, needs `clang` from here.

**The whole of Xcode** (from the App Store, about 15 GB) is only needed for the Apple apps in WittyMediaClient (Mac, iPhone, iPad, Apple TV). Open it once after installing, and accept its licence, before building them.

### 2. Homebrew, or not

[Homebrew](https://brew.sh) is the easiest way to get the rest, and what this page assumes. Install it with the one command on its front page, then do what it prints at the end ("Next steps") so `brew` is on your path. On Apple silicon it lives in `/opt/homebrew`, on Intel in `/usr/local`.

```bash
brew install go node ffmpeg gh
```

**Without Homebrew** everything has an installer of its own: Go from [go.dev/dl](https://go.dev/dl/) (the `.pkg` for your chip), Node.js from [nodejs.org](https://nodejs.org/) (the LTS `.pkg`), GitHub's CLI from [cli.github.com](https://cli.github.com/), and FFmpeg as a static build: the same one a release bundles, whose address and checksum are in `scripts/ffmpeg.lock` (the `mac-arm64-*` or `mac-amd64-*` lines). Put `ffmpeg` and `ffprobe` somewhere on your path, or pass them with `-ffmpeg` and `-ffprobe`.

### 3. The versions

| Tool | Version | Check with |
|---|---|---|
| Go | 1.27 (`go.mod` asks for 1.26 at least, and names `go1.27.3` as its toolchain, which Go fetches itself if yours is older) | `go version` |
| Node.js | 24, as CI uses; 22 works | `node --version` |
| FFmpeg | Any recent build with VideoToolbox (Homebrew's has it) | see below |
| Python | 3, from Apple's command line tools | `python3 --version` |
| Java | Only for Blu-ray Java menus: see below | `/usr/libexec/java_home -V` |

### 4. FFmpeg with VideoToolbox

VideoToolbox is the Mac's own hardware video encoder and decoder. Check FFmpeg has it:

```bash
ffmpeg -hide_banner -encoders | grep videotoolbox    # h264_videotoolbox, hevc_videotoolbox, ...
ffmpeg -hide_banner -hwaccels                         # videotoolbox
```

The server finds FFmpeg next to its own program first (in a release, `WittyMediaServer.app/Contents/Helpers`), then on your path. It then lists **Apple VideoToolbox** under **Settings → Transcoding** when FFmpeg has `h264_videotoolbox`. GitHub's Mac runners are virtual machines, so whether VideoToolbox really encodes there says nothing about a real Mac: try it on one.

### 5. Java, for Blu-ray Java menus only

WittyBluray runs a disc's Java menus (BD-J) on a Java you install; none is ever shipped. It's off by default. Any current JDK works: 21 and 25 have been tested on Linux.

```bash
brew install --cask temurin@21
/usr/libexec/java_home -V
```

Without Homebrew, use the macOS `.pkg` from [adoptium.net](https://adoptium.net/). Either puts it in `/Library/Java/JavaVirtualMachines`, which is where WittyBluray looks on a Mac. Then turn on **Java menus** in WittyBluray's settings (`bdjEnabled`), and set `javaPath` only if it doesn't find it. See [Blu-ray](BLURAY.md).

### 6. GitHub and the repositories

```bash
gh auth login            # GitHub.com, HTTPS, log in with a web browser
mkdir -p ~/projects && cd ~/projects
for r in WittyModule WittyMediaServer; do gh repo clone wittywilla/$r; done
cd WittyMediaServer
node scripts/dev-modules.mjs
```

`~/projects` plays the part `D:\projects` does on the Windows machine: every repository side by side, one folder each. Clone any module you're working on beside them too, and run `node scripts/dev-modules.mjs` again. [Repositories](REPOSITORIES.md) lists them all.

## Building and running the server

The same as on any system ([Contributing](../CONTRIBUTING.md#build-and-run)):

```bash
cd web && npm install && npm run build && cd ..
go build -o .build/wittymediaserver ./cmd/wittymediaserver
.build/wittymediaserver serve -dev -data ./devdata -listen 127.0.0.1:9797
```

Open `http://localhost:9797`. Always use a `-data` folder of its own while developing, so the library in `~/Library/Application Support/WittyMediaServer` is never touched. Binding to `127.0.0.1` keeps the Mac's firewall from asking (see [below](#firewall-privacy-and-sleep)).

**The menu-bar app** needs CGO, which is on by default on a Mac:

```bash
go build -o .build/WittyMediaServer ./cmd/wittymediaserver-tray
```

It looks for `wittymediaserver` beside itself, or in `Contents/Helpers` inside an app. The easiest way to try it as people get it is a whole app, built by the release script (next section), and opened with `open dist/mine/wittymediaserver-dev-darwin-arm64/WittyMediaServer.app`.

**For an Intel Mac from an Apple silicon one** (or the other way), the server cross-compiles with `GOARCH=amd64 CGO_ENABLED=0`. The menu-bar app needs `CGO_ENABLED=1 GOARCH=amd64`, which works on a Mac because Apple's `clang` builds for both chips. An Apple silicon Mac runs Intel programs through Rosetta 2, which it offers to install the first time.

## Tests

```bash
gofmt -l .
go vet ./...
go test ./... -count=1 -timeout 45m
cd web && npx tsc --noEmit -p . && npm run build && cd ..
python3 scripts/theme-contrast.py && python3 scripts/design-tokens.py --check && python3 scripts/design-check.py
```

- **`WMS_TEST_LAN=1`** runs the tests that listen on the network, as CI does. On a real Mac with the firewall on, each new test program makes it ask, and the test waits for the answer. Either leave it unset, or turn the firewall off while testing (System Settings, Network, Firewall) and back on afterwards.
- **The race detector** (`CGO_ENABLED=1 go test -race ./internal/...`) works on a Mac; CI runs it on Linux only.
- **Tests with real modules** need a catalogue: `scripts/make-test-catalog.sh <out> <folder with the module repositories>`, then `WMS_TEST_CATALOG=<out>`. Tests skip without it; count the SKIP lines before calling a run green.
- **The Apple apps** have their own check: `bash apple/scripts/check-on-mac.sh` in WittyMediaClient, which needs Xcode and `brew install xcodegen`.

CI runs the first block on Apple silicon for every pull request (the `macos` job in `.github/workflows/ci.yml`).

## Packaging

On a Mac, `scripts/build-release.sh` builds the menu-bar app itself, so its archives are complete:

```bash
DIST=dist/mine scripts/build-release.sh 0.7.0-dev darwin/arm64 darwin/amd64
packaging/macos/build-pkg.sh 0.7.0-dev dist/mine/wittymediaserver-0.7.0-dev-darwin-arm64 arm64 dist/mine
```

Each archive holds `WittyMediaServer.app` (the menu-bar app in `Contents/MacOS`, the server, FFmpeg and the rest in `Contents/Helpers`) and `Uninstall WittyMediaServer.app`. The `.pkg` puts both in Applications, links `/usr/local/bin/wittymediaserver`, and, unless **Customize** leaves it out, sets up the background service. Build into a folder of your own, and never over one that may be running.

To try a `.pkg` and take it away again:

```bash
sudo installer -pkg dist/mine/wittymediaserver-0.7.0-dev-macos-arm64.pkg -target /
curl http://127.0.0.1:9797/api/health
sudo wittymediaserver --uninstall --dry-run        # what it would remove
sudo wittymediaserver --uninstall --remove-data --yes
```

`--remove-data` deletes the library in `~/Library/Application Support/WittyMediaServer`. Leave it out on a Mac whose library you want.

The release workflow does the same: `tray-macos` builds the app on a Mac for both chips, `archives` packs everything on Linux, and `installers-macos` makes the `.pkg` files on a Mac.

## Signing and notarising

Without signing, macOS warns about the app and refuses to open the `.pkg` with a double-click: people have to right-click it and choose **Open**, or use **Open Anyway** in **System Settings → Privacy & Security**. Everything works once opened.

Signing and notarising need **the Apple Developer Program, US$99 a year**, which only the owner can join. The steps, once:

1. **Join.** Go to [developer.apple.com/programs/enroll](https://developer.apple.com/programs/enroll/), sign in with your Apple Account (two-factor authentication must be on), and enrol as an **Individual**. Apple checks who you are, which can take a day or two. The yearly fee is paid there.
2. **Make the two certificates**, on the Mac you'll sign with. Open **Xcode → Settings → Accounts**, add your Apple Account, select your team, choose **Manage Certificates…**, then **+** and **Developer ID Application**, and again for **Developer ID Installer**. They go into your login keychain with their private keys. **Back them up**: in **Keychain Access**, select both, **File → Export Items…**, as a `.p12` with a strong password, kept somewhere safe and never in a repository.
3. **Find their names:**
   ```bash
   security find-identity -v -p basic | grep "Developer ID"
   ```
   They look like `Developer ID Application: Your Name (TEAMID1234)`.
4. **An app-specific password for notarising.** At [account.apple.com](https://account.apple.com), **Sign-In and Security → App-Specific Passwords**, make one called `notarytool`.
5. **Store it for `notarytool`:**
   ```bash
   xcrun notarytool store-credentials wittymediaserver-notary --apple-id <your Apple Account email> --team-id <TEAMID1234>
   ```
   It asks for the app-specific password, and keeps it in the keychain under that profile name.
6. **Build signed:**
   ```bash
   export APPLE_APP_IDENTITY="Developer ID Application: Your Name (TEAMID1234)"
   export APPLE_INSTALLER_IDENTITY="Developer ID Installer: Your Name (TEAMID1234)"
   export APPLE_NOTARY_PROFILE=wittymediaserver-notary
   packaging/macos/build-pkg.sh 0.7.0-dev dist/mine/wittymediaserver-0.7.0-dev-darwin-arm64 arm64 dist/mine
   ```
   It signs every program inside out with the hardened runtime, signs the `.pkg`, sends it to Apple, waits, and staples the ticket on.
7. **Check it:**
   ```bash
   pkgutil --check-signature dist/mine/wittymediaserver-0.7.0-dev-macos-arm64.pkg
   spctl --assess --type install -v dist/mine/wittymediaserver-0.7.0-dev-macos-arm64.pkg
   ```

**Not built yet:** signing in the release workflow. That needs the `.p12`, its password, and an App Store Connect API key for `notarytool` as secrets in a protected `release` environment, and a step that makes a throwaway keychain on the runner. The code signing plan covers it, together with WittyMediaClient, WittyReader and the modules' helpers, which have to be signed with the same Team ID. Signing also changes what can load what: a hardened program refuses libraries signed by another team, which is why game cores need signing by us.

## Where everything lives

| What | Where |
|---|---|
| The apps | `/Applications/WittyMediaServer.app`, `/Applications/Uninstall WittyMediaServer.app` |
| The server, FFmpeg and the other programs | `/Applications/WittyMediaServer.app/Contents/Helpers` |
| The command | `/usr/local/bin/wittymediaserver`, a link to the one in `Helpers` |
| Library, accounts and settings | `~/Library/Application Support/WittyMediaServer` |
| Settings file | `~/Library/Application Support/WittyMediaServer/wms.json` |
| Logs | `~/Library/Application Support/WittyMediaServer/logs` |
| Themes you add | `~/Library/Application Support/WittyMediaServer/themes` |
| Modules | `~/Library/Application Support/WittyMediaServer/modules`; a starter set in `WittyMediaServer.app/Contents/Resources/modules` |
| The background service | `/Library/LaunchDaemons/com.wittywilla.wittymediaserver.plist` |
| Starting the menu-bar app at sign-in | `~/Library/LaunchAgents/com.wittywilla.wittymediaserver.tray.plist` |
| The installer's record | `pkgutil --pkgs | grep wittywilla` |

The service runs as the person who installed it, so `~` above is their home folder. Its data folder is whatever its property list's `WMS_DATA_DIR` says. [Uninstalling: macOS](UNINSTALL.md#macos) lists everything removal touches.

## The background service (launchd)

The `.pkg`'s **Run in the background** part writes a LaunchDaemon: launchd starts the server when the Mac starts, before anyone signs in, as the person who installed it, and starts it again if it stops.

```bash
sudo launchctl print system/com.wittywilla.wittymediaserver          # is it running, and its last exit
sudo launchctl kickstart -k system/com.wittywilla.wittymediaserver   # restart it
sudo launchctl bootout system /Library/LaunchDaemons/com.wittywilla.wittymediaserver.plist     # stop it
sudo launchctl bootstrap system /Library/LaunchDaemons/com.wittywilla.wittymediaserver.plist   # start it
```

**To change the port or the data folder,** edit `WMS_LISTEN` or `WMS_DATA_DIR` in the property list (it's root's: `sudo nano`), then stop and start it as above. Installing a newer `.pkg` keeps both.

**Its output** goes to the server's own logs (see the table above) and to the system log: `log show --last 1h --predicate 'process == "wittymediaserver"'`.

Without the background part, the server runs while the menu-bar app is open, and the app's **Start at login** adds a LaunchAgent for that person only.

## Firewall, privacy and sleep

A Mac asks before a program does several things a media server needs. None of these happen on GitHub's runners, so none has been seen yet.

- **The firewall** (System Settings, Network, Firewall), when it's on, asks "Do you want the application to accept incoming network connections?" the first time a program listens on the network. An unsigned program that changes, such as a fresh build, may be asked about again after each build. Allow it, or bind to `127.0.0.1` while developing. Signing fixes the repeat questions for releases. A LaunchDaemon can't show the question at all: whether the service is blocked silently is one of the first things to try.
- **Local network** (macOS 15 and later, System Settings, Privacy & Security, Local Network) is asked for when a program reaches other devices on the home network: the client's server discovery, HDHomeRun tuners, game PCs, casting. A program started from Terminal is covered by Terminal's answer. To check: whether the service and the menu-bar app are asked, and what happens when someone says no.
- **Protected folders.** Desktop, Documents, Downloads, iCloud Drive, removable drives, network volumes and the Photos library each need the person's permission. The menu-bar app can be asked; the background service can't, so a library there may scan as empty with no explanation. The way round today: keep media in `~/Movies`, `~/Music` or a folder you made, or give **Full Disk Access** to `/Applications/WittyMediaServer.app/Contents/Helpers/wittymediaserver` (System Settings, Privacy & Security, Full Disk Access, **+**, then Command-Shift-G to type the path). The server doesn't yet say when this is the reason.
- **Sleep.** A Mac that sleeps stops serving. Nothing in the server keeps it awake yet. For a Mac that serves all day: System Settings, Energy (or Battery, Options on a laptop), turn on **Prevent automatic sleeping when the display is off** and **Wake for network access**. A laptop with its lid closed sleeps regardless, unless it has power and an external display.

## Known not to work, or not tried yet

Each of these has an issue labelled `platform:macos`. Those labelled `not-run` were written and checked as far as possible, and need a real Mac.

- [Everything listed as `platform:macos` on the server](https://github.com/wittywilla/WittyMediaServer/issues?q=is%3Aopen+label%3Aplatform%3Amacos), and in [every repository](https://github.com/search?q=org%3Awittywilla+label%3Aplatform%3Amacos+is%3Aopen&type=issues).
- **Never run on a real Mac:** the menu-bar app, the `.pkg` with a person at the screen, the uninstaller app, the service after a restart, the firewall, local network and privacy questions above, VideoToolbox transcoding, the helper sandbox, games, disc modules, WittyTuner's capture devices, the Mac's drive health and system figures.
- **Module helpers run without a sandbox on a Mac** for now: the `sandbox-exec` profile is written and compiled but not trusted until it's seen working, so a fork from another publisher can't be chosen. See [Modules](MODULES.md) and [Module store](MODULE-STORE.md#using-someones-fork).
- **Games:** the consoles drawn on a graphics card don't play on a Mac ([Games](GAMES.md)), there's no per-game CPU limit (priority only), and game cores downloaded from the libretro build server aren't signed by us, which a signed, hardened runner will refuse.
- **WittyMediaPlayer** has no Mac build ([its design notes](https://github.com/wittywilla/WittyMediaPlayer/blob/main/DESIGN.md) list what a port needs).
- **No signing**, so every program warns, until the owner joins the Apple Developer Program.
- **Updating from the server's own page** isn't offered for a `.pkg` install yet.

---

<sub>WittyMediaServer documentation</sub>
