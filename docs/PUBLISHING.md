# Publishing a release

Put changes on GitHub, publish a release of the server, and look after the key that signs it. This is for the project's owner and developers, and isn't in the app's help.

**Releases are signed only with `SIGN=1`, and only by the owner.** Nobody else signs with the release key, and no test build ever does: for a signing test, make a throwaway key. Nothing leaves the development machine, and nothing is pushed or tagged, until the owner says so.

Modules are published separately: see [Publishing the modules](MODULES-CUTOVER.md).

Words you'll see: a *commit* is a saved snapshot of your changes, *push* uploads commits, a *branch* is a separate line of work, and a *release* is a downloadable version on GitHub's Releases page.

## Putting changes on GitHub

**With GitHub Desktop:**

1. Install [GitHub Desktop](https://desktop.github.com/) and sign in.
2. Choose **File**, then **Add local repository**, and pick the `WittyMediaServer` folder.
3. Write a summary at the bottom left, and click **Commit**.
4. Click **Push origin** at the top.

**On the command line,** inside the folder:

```bash
git status                     # see what changed
git add -A                     # stage everything: .gitignore keeps build output out
git commit -m "What changed"
git push origin master
```

The first push asks you to sign in: choose **Sign in with your browser**.

Never commit disc images, media, API keys or a data folder. Run the checks in `CONTRIBUTING.md` before pushing. GitHub runs them too, in the **Actions** tab.

## Publishing a release

1. Run `scripts/bump-server.sh <version>`, and write the `CHANGELOG.md` section.
2. Tag it and push the tag. In GitHub Desktop: **History**, right-click the commit, **Create Tag**, then **Push origin**. Or:

   ```bash
   git tag v0.6.0-alpha
   git push origin v0.6.0-alpha
   ```

3. Watch the **Actions** tab. `.github/workflows/release.yml` builds everything, stops at the `release` environment for the owner's approval if it requires one, signs the checksum list, and publishes.
4. Check the **Releases** page lists the archives for all ten platforms, the installers (Windows `.msi`, macOS `.pkg`, Linux `.deb` and `.rpm`, FreeBSD `.pkg`), `SHA256SUMS.txt` and `SHA256SUMS.txt.sig`.
5. Give the release a short summary. Installed servers show it as the notes under **Settings → About**.

A tag with a suffix such as `-alpha` is published as a pre-release, and its Docker image is tagged `alpha`. Only a tag with no suffix moves `latest`.

**To retry,** delete the tag on GitHub under **Releases**, then **Tags**, make it again on the new commit, and push.

**This workflow has never run.** The first tag may need fixes. [Publishing the modules](MODULES-CUTOVER.md) has to come first, as the Docker build needs `wittymodule` v0.3.0 on GitHub.

## Signing releases

Installed servers update themselves from Settings, but only to a release whose checksum list has a valid signature from **the release key**. Its public half is built into every server, in `internal/update/keys.go`. Whoever has the private half can make updates that every installed server accepts. Lose it, and installed servers can't update themselves any more.

The key was made once, with `go run ./scripts/sign keygen`, and is at `~/.wms-signing/release.key` on the owner's computer. `.gitignore` refuses `*.key` files, but still never copy it into the project folder.

### Looking after the key

1. **Back it up now.** Copy `release.key` to a password manager, or a USB stick kept somewhere safe. Never email it, or put it in a chat or a shared cloud folder.
2. **Let GitHub sign releases.** On github.com, open **Settings**, then **Environments**, then **New environment**, and name it `release`. Tick **Required reviewers** and add yourself. Under **Environment secrets**, add `WMS_SIGNING_KEY`, with all of `release.key` as its value. Delete any repository secret of that name. A tag then waits for your approval before it's signed.
3. **Protect tags.** Add a ruleset so that only you can create, move or delete a tag starting with `v`.

Without the secret, a release comes out unsigned, and servers can't update to it from Settings.

### Signing on your own computer

`scripts/build-release.sh` doesn't sign unless you ask. Only for a real release:

```bash
SIGN=1 scripts/build-release.sh 0.6.0-alpha
```

It uses `~/.wms-signing/release.key`, or the file in `WMS_SIGNING_KEY`, and checks the signature against the key built into the server. With `SIGN=1`, a missing key warns, and `REQUIRE_SIGNING=1` makes that an error.

### If a release came out unsigned

Download its `SHA256SUMS.txt`, sign it, and upload the `.sig` to the same release:

```bash
go run ./scripts/sign sign wittymediaserver-0.6.0-alpha-SHA256SUMS.txt
go run ./scripts/sign verify wittymediaserver-0.6.0-alpha-SHA256SUMS.txt   # "good signature"
```

### If a release has to be taken back

1. Write `wittymediaserver-withdrawn.json`, raising `serial` each time:

   ```json
   {"module":"wittymediaserver","serial":1,"versions":[{"version":"0.6.0-alpha","severity":"security","reason":"What's wrong."}]}
   ```

2. Sign it with `go run ./scripts/sign sign`.
3. Attach both files to a release tagged `withdrawals`.

Servers stop offering that version, and one already running it says so under **Settings → About**. A server keeps the highest serial it has seen, so hiding or replaying the list changes nothing. Deleting a release from GitHub isn't enough, as a server that has already seen it can still be offered it.

### If the key is exposed or lost

- **Exposed:** make a new one with `go run ./scripts/sign keygen -key new.key`, and add its public key to `trustedKeys` in `internal/update/verify.go`, beside the old one. Publish a release signed with the **old** key, so installed servers learn the new one. Sign later releases with the new key, and remove the old public key a release after that. Replace the `WMS_SIGNING_KEY` secret straight away.
- **Lost, with nothing exposed:** servers can't update themselves any more, as nothing else can sign a release they'll accept. Add a new key the same way, and announce that the next release has to be installed by hand, once.

## Building on your own computer

The same script the workflow uses runs locally, from Git Bash on Windows, or as `scripts\build-release.ps1`. It's unsigned. Build into a folder of your own, and never over one that may be running.

```bash
DIST=dist/mine scripts/build-release.sh 0.6.0-alpha                                # all ten platforms
DIST=dist/mine scripts/build-release.sh 0.6.0-alpha windows/amd64 linux/arm64      # just some
```

On a computer that isn't a Mac, the macOS archives have no menu-bar app, as it needs a Mac to compile. The FreeBSD archives use FreeBSD's own FFmpeg unless you give them a static build, from `scripts/build-ffmpeg-static.sh`.

Each installer is made from a folder that script built. `scripts/test/README.md` says what proves each works.

```bash
packaging/linux/build-packages.sh 0.6.0-alpha dist/mine/wittymediaserver-0.6.0-alpha-linux-amd64 amd64   # .deb and .rpm, needs nfpm
packaging/linux/make-repository.sh dist/mine out                                                         # an apt and dnf repository
packaging/macos/build-pkg.sh 0.6.0-alpha dist/mine/wittymediaserver-0.6.0-alpha-darwin-arm64 arm64       # .pkg, on a Mac
packaging/freebsd/build-pkg.sh 0.6.0-alpha dist/mine/wittymediaserver-0.6.0-alpha-freebsd-amd64 amd64    # .pkg, on FreeBSD
```

```powershell
packaging\windows\build-msi.ps1 -Version 0.6.0-alpha -PackageDir dist\mine\wittymediaserver-0.6.0-alpha-windows-amd64 -Arch amd64
```

## Signing the programs, so nothing warns people

This is a separate thing from the release key. Without these, Windows says "Windows protected your PC", and a Mac refuses the installer until it's right-clicked. Everything works without them. Prices are from memory, so check before buying.

| | What it costs | What you do |
|---|---|---|
| **Windows** | A code-signing certificate, about US$200 to $400 a year. Azure Trusted Signing, about US$10 a month, where you qualify. Or SignPath Foundation, free for open source, if accepted | Add the signing step to the `installers-windows` job in `release.yml`, on the programs before the MSI is built and on the MSI after |
| **macOS** | Apple Developer Program, US$99 a year | Make the Developer ID Application and Developer ID Installer certificates and a notary profile, then set `APPLE_APP_IDENTITY`, `APPLE_INSTALLER_IDENTITY` and `APPLE_NOTARY_PROFILE` for `packaging/macos/build-pkg.sh` |
| **apt and dnf repositories** | Free, plus somewhere to host them. None is hosted yet | Make a GPG key, keep it as carefully as the release key, and give its id to `packaging/linux/make-repository.sh` |

The WiX Toolset that builds the MSI is free for this project. Its maintenance fee applies to organisations earning over US$10,000 a year.

---

<sub>WittyMediaServer documentation</sub>
