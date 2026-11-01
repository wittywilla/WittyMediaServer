# Publishing changes to GitHub

A step-by-step guide for getting local changes onto
<https://github.com/wittywilla/WittyMediaServer>, written for people new to Git.

**Words you'll see:** a *commit* is a saved snapshot of your changes with a message; *push*
uploads commits to GitHub; a *branch* is a separate line of work; a *release* is a downloadable
version (with the built archives) on GitHub's Releases page.

## Option A — GitHub Desktop (easiest)

1. Install [GitHub Desktop](https://desktop.github.com/) and sign in with your GitHub account.
2. *File → Add local repository…* → choose the `WittyMediaServer` folder.
3. The left panel lists every changed file. Write a summary at the bottom left, e.g.
   `0.4.0-alpha: one-click updates, a folder browser, accessibility and DVD fixes`, and click
   **Commit to master**.
4. Click **Push origin** at the top. Your changes are now on GitHub.

## Option B — the command line

Run these in a terminal inside the `WittyMediaServer` folder.

One-time setup (use the email of your GitHub account, or your GitHub "noreply" address from
*GitHub → Settings → Emails*):

```bash
git config --global user.name "WittyWilla"
git config --global user.email "you@example.com"
```

Every time you want to publish:

```bash
git status                     # see what changed
git add -A                     # stage everything (the .gitignore keeps build output out)
git commit -m "0.4.0-alpha: one-click updates, a folder browser, accessibility and DVD fixes"
git push origin master         # upload
```

The first push asks you to sign in: choose *Sign in with your browser*. (Git no longer accepts
your GitHub password on the command line; the browser sign-in or a personal access token from
*GitHub → Settings → Developer settings → Personal access tokens* is used instead.)

## Publishing a release (the downloadable archives)

The repository's release workflow (`.github/workflows/release.yml`) builds every platform when
you push a **version tag**:

```bash
git tag v0.4.0-alpha
git push origin v0.4.0-alpha
```

Then watch the *Actions* tab on GitHub; when the workflow finishes, the archives for all ten
platforms, the installers (Windows `.msi`, macOS `.pkg`, Linux `.deb` and `.rpm`, FreeBSD `.pkg`) and the SHA256
checksum file with its signature (see [Signing releases](#signing-releases)) appear on the
*Releases* page. Tags with a suffix such as
`-alpha` or `-beta` are published as *pre-releases* automatically.

To retry a release after fixing something, delete the tag (GitHub Desktop: *History* →
right-click the tag → *Delete Tag…*, which also offers to delete it on GitHub; or on the website:
*Releases → Tags*), then create it again on the new commit and push.

### Signing releases

Installed servers update themselves from Settings, but only to a release whose checksum list
(`wittymediaserver-<version>-SHA256SUMS.txt`) has a valid signature (`…SHA256SUMS.txt.sig`) made
with **the release signing key**. Its public half is built into every server
(`internal/update/keys.go`); the private half must stay secret and safe. Whoever has it can make
updates that every installed server accepts. Lose it, and installed servers can no longer update
from Settings (people would have to download the next version by hand once).

**The key was made once** with `go run ./scripts/sign keygen` and is stored outside the repository,
on the computer that made it, at `C:\Users\<you>\.wms-signing\release.key` (`~/.wms-signing/release.key`).
`.gitignore` refuses `*.key` files, but still never copy it into the project folder.

1. **Back it up now**: copy `release.key` to a password manager (as an attached file or secure
   note) or a USB stick you keep somewhere safe. Never email it or put it in a chat or cloud
   folder others can open.
2. **Let GitHub sign releases**: on github.com open the repository → *Settings* → *Secrets and
   variables* → *Actions* → **New repository secret**. Name it `WMS_SIGNING_KEY`, open
   `release.key` in Notepad, copy **all** of it (both lines) into *Secret*, and click *Add secret*.
   GitHub keeps secrets encrypted and never shows them again, even to you. The release workflow
   then signs every release it builds; check that each release lists a `…SHA256SUMS.txt.sig`.
   **Better:** keep it in an environment so only the signing job can read it, and approve each
   signature yourself: *Settings* → *Environments* → **New environment**, name it `release`, tick
   *Required reviewers* and add yourself, then add the secret there under *Environment secrets*
   and delete the repository secret. A tag then waits for your approval before it is signed.
3. **Local builds** are unsigned unless you ask: `SIGN=1 scripts/build-release.sh <version>` uses
   `~/.wms-signing/release.key` (or the file in `WMS_SIGNING_KEY`) and checks the signature against the
   key built into the server. A signature says "this is a real release", so test builds must not have
   one. With `SIGN=1`, a missing key warns; `REQUIRE_SIGNING=1` makes that an error.

**A release came out unsigned** (for example before the secret was added)? Download its
`…SHA256SUMS.txt` from the release page, sign it, and upload the `.sig` it writes to the same
release (*Edit release* → drag the file in → *Update release*):

```bash
go run ./scripts/sign sign wittymediaserver-0.4.0-alpha-SHA256SUMS.txt
go run ./scripts/sign verify wittymediaserver-0.4.0-alpha-SHA256SUMS.txt   # "good signature"
```

**If the key is ever exposed** (or lost): make a new one (`go run ./scripts/sign keygen -key new.key`), add its
**If a server release has to be taken back:** make `wittymediaserver-withdrawn.json` (the same form
modules use: `{"module":"wittymediaserver","serial":N,"versions":[{"version":"x.y.z","severity":"security","reason":"..."}]}`,
raising `serial` each time), sign it with `go run ./scripts/sign sign`, and attach both files to a release tagged
`withdrawals`. Servers stop offering that version, and one already running it says so under *Settings → About*.
A server keeps the highest serial it has seen, so hiding or replaying the list changes nothing. Deleting a
release from GitHub isn't enough: a server that has already seen it can still be offered it.

**If the key is lost and nothing was exposed:** servers can't update themselves any more, because nothing else
can sign a release they will accept. Add a new key as below and announce that the next release has to be
installed by hand, once.

**If the key is ever exposed** (or lost): make a new one (`go run ./scripts/sign keygen -key new.key`), add its
public key to `trustedKeys` in `internal/update/verify.go` next to the old one, and publish a
release signed with the **old** key, so installed servers learn the new key. Sign later releases
with the new key and remove the old public key a release after that. If the old key was stolen,
also delete the `WMS_SIGNING_KEY` secret and add the new key in its place.

### Building the archives on your own computer

The same script the workflow uses runs locally (Windows: from *Git Bash*, or
`scripts\build-release.ps1` in PowerShell):

```bash
scripts/build-release.sh 0.2.0-alpha            # all ten platforms -> dist/
scripts/build-release.sh 0.4.0-alpha windows/amd64 linux/arm64   # just some
```

On a non-Mac computer the macOS archives contain the server but not the menu-bar app (it needs a
Mac to compile), and the FreeBSD archives use FreeBSD's own FFmpeg unless you provide a
static build (`scripts/build-ffmpeg-static.sh`, run on FreeBSD).

### Before tagging

In GitHub Desktop you create the tag via *History → right-click the commit → Create Tag…*, then
*Push origin*. Before tagging, run `scripts/bump-server.sh <version>`, and add a section to `CHANGELOG.md`. Installed servers show the release's
description from GitHub as its notes under *Settings → About → Updates*, so give the
release a short summary of what changed.

### Building the installers on your own computer

Each is made from a folder that `scripts/build-release.sh` built, and [`scripts/README.md`](../scripts/README.md) says what proves it works.

```bash
packaging/linux/build-packages.sh 0.6.0-alpha dist/wittymediaserver-0.6.0-alpha-linux-amd64 amd64   # .deb and .rpm, needs nfpm
packaging/linux/make-repository.sh dist out                                                         # an apt and dnf repository
packaging/macos/build-pkg.sh 0.6.0-alpha dist/wittymediaserver-0.6.0-alpha-darwin-arm64 arm64       # .pkg, on a Mac
packaging/freebsd/build-pkg.sh 0.6.0-alpha dist/wittymediaserver-0.6.0-alpha-freebsd-amd64 amd64    # .pkg, on FreeBSD
```

```powershell
packaging\windows\build-msi.ps1 -Version 0.6.0-alpha -PackageDir dist\wittymediaserver-0.6.0-alpha-windows-amd64 -Arch amd64
```

### Signing the programs, so nothing warns people

Without these, Windows says "Windows protected your PC" and a Mac refuses to open the installer until it's right-clicked. Everything works without them.

| | What it costs | What you do |
|---|---|---|
| **Windows** | A code-signing certificate, about US$200 to $400 a year; [Azure Trusted Signing](https://azure.microsoft.com/products/trusted-signing) is about US$10 a month, where you qualify. Or [SignPath Foundation](https://signpath.org), free for open source, if accepted | Add the signing step to the `installers-windows` job in `.github/workflows/release.yml`, on the programs before the MSI is built and on the MSI after |
| **macOS** | Apple Developer Program, US$99 a year | Make the *Developer ID Application* and *Developer ID Installer* certificates and a notary profile, then set `APPLE_APP_IDENTITY`, `APPLE_INSTALLER_IDENTITY` and `APPLE_NOTARY_PROFILE` for `packaging/macos/build-pkg.sh` |
| **apt and dnf repositories** | Free, plus somewhere to host them | Make a GPG key, keep it as carefully as the release key, and give its id to `packaging/linux/make-repository.sh` |

The WiX Toolset that builds the MSI is free for this project: its maintenance fee applies to organisations earning over US$10,000 a year, so check again if that's ever you.

## Good habits

- Commit small, related changes together with a message that says *why*.
- Run the checks in [CONTRIBUTING.md](../CONTRIBUTING.md) before pushing; GitHub runs them too
  (*Actions* tab) and shows a red ✗ if something fails.
- Never commit disc images, media, API keys or your data folder. The `.gitignore` already
  excludes build output, databases and logs.
- Bigger changes: create a branch (`git checkout -b my-feature`), push it, and open a *pull
  request* on GitHub so you can review the diff before it lands on `master`.

---
<sub>WittyMediaServer 0.5.0-alpha documentation</sub>
