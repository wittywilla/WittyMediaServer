Checklist for **{{VERSION}}**, from `.github/release-checklist.md`. To change the list, change that file and run the **release tracking** workflow again: ticks made here are kept, lines added here by hand are not. Lines that name a file are ticked by the workflow once the release is published and has that file.

### Before the tag

- [ ] Every issue in the milestone closed, or moved with a comment saying why
- [ ] Soak run of at least 4 hours on Linux passed, or what it found has an issue
- [ ] Full Linux run of `go test ./...` with real modules (`WMS_TEST_CATALOG`), SKIP lines counted
- [ ] Windows `go test ./...`
- [ ] Web build and `npm run test:tv`
- [ ] Design check, light and dark

### Platform builds

- [ ] The `build` workflow is green on the commit to be tagged: Linux (amd64, arm64, armv7, 386), Windows, macOS, FreeBSD, Docker, and the AppImage, Flatpak and snap
- [ ] Windows archives <!-- assets: -windows-amd64.zip -windows-arm64.zip -->
- [ ] macOS archives, signed <!-- assets: -darwin-arm64.tar.gz -darwin-amd64.tar.gz -->
- [ ] Linux archives <!-- assets: -linux-amd64.tar.gz -linux-arm64.tar.gz -linux-armv7.tar.gz -linux-386.tar.gz -->
- [ ] FreeBSD archives <!-- assets: -freebsd-amd64.tar.gz -freebsd-arm64.tar.gz -->
- [ ] Docker image `ghcr.io/wittywilla/wittymediaserver:{{VERSION}}` pushed for amd64, arm64 and armv7

### Installers

- [ ] Windows `.msi` <!-- assets: -windows-amd64.msi -windows-arm64.msi -->
- [ ] The `.msi` updated the last published version in the release workflow, three ways
- [ ] macOS `.pkg` <!-- assets: -macos-arm64.pkg -macos-amd64.pkg -->
- [ ] Linux `.deb` <!-- assets: -linux-amd64.deb -linux-arm64.deb -linux-armv7.deb -linux-386.deb -->
- [ ] Linux `.rpm` <!-- assets: -linux-amd64.rpm -linux-arm64.rpm -linux-armv7.rpm -linux-386.rpm -->
- [ ] FreeBSD `.pkg` <!-- assets: -freebsd-amd64.pkg -freebsd-arm64.pkg -->
- [ ] AppImage <!-- assets: -linux-amd64.AppImage -linux-arm64.AppImage -->
- [ ] Flatpak <!-- assets: -linux-amd64.flatpak -linux-arm64.flatpak -->
- [ ] Snap <!-- assets: -linux-amd64.snap -linux-arm64.snap -->

### Updater feed

- [ ] Checksum list <!-- assets: -SHA256SUMS.txt -->
- [ ] Its signature, made with the release key <!-- assets: -SHA256SUMS.txt.sig -->
- [ ] A server of the version before finds {{VERSION}} under **Settings → About → Updates**, with its notes, and **Update now** or **Download the installer** works
- [ ] `wittymediaserver-withdrawn.json` still says what it should

### Docs

- [ ] `docs/INSTALL.md`, **What has been tested**, says what was run for this version
- [ ] `README.md`, **Where it has been run**, and the release notes in `.github/workflows/release.yml` say the same
- [ ] `docs/openapi.json` and `docs/CLIENT-API.md` are at this version's API revision

### Changelog

- [ ] `scripts/bump-server.sh {{VERSION}}` has been run
- [ ] `CHANGELOG.md` has a `## {{VERSION}}` section, with everything in it

### Module catalogue ranges

- [ ] Every module in the catalogue has a `minServer` and `maxServer` that cover {{VERSION}} (`scripts/bump-server.sh` names the ones that don't)
- [ ] The catalogue is signed and published with them

### Website

- [ ] The website's download links and version say {{VERSION}}

### Publishing

- [ ] Owner's go-ahead
- [ ] Tag `v{{VERSION}}` pushed, and the release workflow watched to the end
- [ ] Release published <!-- published -->
