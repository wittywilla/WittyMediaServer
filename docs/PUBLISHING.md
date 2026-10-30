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
   `0.3.0-alpha: a new home, discs as films or series, podcasts, module versions`, and click
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
git commit -m "0.3.0-alpha: a new home, discs as films or series, podcasts, module versions"
git push origin master         # upload
```

The first push asks you to sign in: choose *Sign in with your browser*. (Git no longer accepts
your GitHub password on the command line; the browser sign-in or a personal access token from
*GitHub → Settings → Developer settings → Personal access tokens* is used instead.)

## Publishing a release (the downloadable archives)

The repository's release workflow (`.github/workflows/release.yml`) builds every platform when
you push a **version tag**:

```bash
git tag v0.3.0-alpha
git push origin v0.3.0-alpha
```

Then watch the *Actions* tab on GitHub; when the workflow finishes, the archives for all ten
platforms, the Windows setup programs, and the SHA256 checksum file with its signature (see
[Signing releases](#signing-releases)) appear on the *Releases* page. Tags with a suffix such as
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

**The key was made once** with `go run ./cmd/wms-sign keygen` and is stored outside the repository,
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
3. **Local builds** sign automatically: `scripts/build-release.sh` uses `~/.wms-signing/release.key`
   (or the file in `WMS_SIGNING_KEY`) and checks the signature against the key built into the
   server. Without a key it warns and leaves the release unsigned; `REQUIRE_SIGNING=1` makes that an
   error.

**A release came out unsigned** (for example before the secret was added)? Download its
`…SHA256SUMS.txt` from the release page, sign it, and upload the `.sig` it writes to the same
release (*Edit release* → drag the file in → *Update release*):

```bash
go run ./cmd/wms-sign sign wittymediaserver-0.4.0-alpha-SHA256SUMS.txt
go run ./cmd/wms-sign verify wittymediaserver-0.4.0-alpha-SHA256SUMS.txt   # "good signature"
```

**If the key is ever exposed** (or lost): make a new one (`wms-sign keygen -key new.key`), add its
public key to `trustedKeys` in `internal/update/verify.go` next to the old one, and publish a
release signed with the **old** key, so installed servers learn the new key. Sign later releases
with the new key and remove the old public key a release after that. If the old key was stolen,
also delete the `WMS_SIGNING_KEY` secret and add the new key in its place.

### Building the archives on your own computer

The same script the workflow uses runs locally (Windows: from *Git Bash*, or
`scripts\build-release.ps1` in PowerShell):

```bash
scripts/build-release.sh 0.2.0-alpha            # all ten platforms -> dist/
scripts/build-release.sh 0.3.0-alpha windows/amd64 linux/arm64   # just some
```

On a non-Mac computer the macOS archives contain the server but not the menu-bar launcher (it
needs a Mac to compile), and the FreeBSD archives use FreeBSD's own FFmpeg unless you provide a
static build (`scripts/build-ffmpeg-static.sh`, run on FreeBSD).

### Before tagging

In GitHub Desktop you create the tag via *History → right-click the commit → Create Tag…*, then
*Push origin*. Before tagging, update the version in `cmd/wms/main.go`, `cmd/wms-tray/main.go` and `cmd/wms-setup/main_windows.go` (the default
`version`), and add a section to `CHANGELOG.md`. Installed servers show the release's
description from GitHub as its notes under *Settings → About & legal → Updates*, so give the
release a short summary of what changed.

## Good habits

- Commit small, related changes together with a message that says *why*.
- Run the checks in [CONTRIBUTING.md](../CONTRIBUTING.md) before pushing; GitHub runs them too
  (*Actions* tab) and shows a red ✗ if something fails.
- Never commit disc images, media, API keys or your data folder. The `.gitignore` already
  excludes build output, databases and logs.
- Bigger changes: create a branch (`git checkout -b my-feature`), push it, and open a *pull
  request* on GitHub so you can review the diff before it lands on `master`.

---
<sub>WittyMediaServer 0.3.0-alpha documentation</sub>
