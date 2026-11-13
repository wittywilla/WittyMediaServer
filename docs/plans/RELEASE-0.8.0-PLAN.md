# Release plan: WittyMediaServer 0.8.0-alpha and its 24 sibling repositories

Written 2026-11-12 by S13 (release engineering) for the owner. **Agents never push, tag, publish or delete anything on
GitHub: every command in "Owner steps" is yours to run.** The per-file version checklist (where the server's, the kit's
and the clients' versions are written, and the checks before tagging) is W7's
[Release checklist](DESIGN-2-RELEASE.md#release-checklist); this plan does not repeat it. It adds what is released as what,
in which order, what has to be raised after each publish, which CI has to be green first, and the commands.

Read from the local `v0.8.0-alpha` branches on 2026-11-12. GitHub was read with `gh` (read-only). Every repository's
`main`/`master` on GitHub is an ancestor of its local `v0.8.0-alpha`, so every push below fast-forwards.

## 1. What each repository releases as

"Already published" means a tag of that number exists on GitHub and in the Go module proxy: **a published version number
is never reused**, even after its tag or release is deleted (proxy.golang.org and sum.golang.org keep it for ever).

| # | Repository | Releases as | Last published | State on `v0.8.0-alpha` | Before it can be tagged |
|---|---|---|---|---|---|
| 1 | WittyModule | **0.4.0** | v0.3.0 (module API 3) | 0.4.0 (module API 4, kit at revision 8) | none (G1 fixed) |
| 2 | WittyDisc | 0.2.1 | v0.2.0 | 0.2.1 | none |
| 3 | WittyDVD | 0.4.0, and Go folder tag `wittydvdplayer/v0.3.0` | v0.3.1, wittydvdplayer/v0.2.0 | 0.4.0, copy 0.3.0 | none |
| 4 | WittyBluray | 0.4.0, and `wittyblurayplayer/v0.3.0` | v0.3.0, wittyblurayplayer/v0.2.0 | 0.4.0, copy 0.3.0 | none |
| 5 | WittyCD | 0.2.1 | v0.2.0 | 0.2.1 | none |
| 6 | WittyTuner | 0.3.0 | v0.2.0 | 0.3.0 | none |
| 7 | WittyPhotos | 0.4.0 | v0.3.0 | 0.4.0 | none |
| 8 | WittyIntegratedFileBrowser | 0.3.0 | v0.2.0 | 0.3.0 | none |
| 9 | WittyRigs | 0.3.0 | v0.2.0 | 0.3.0 | none |
| 10 | WittyPlay | 0.4.0 | v0.3.6 | 0.4.0 | none |
| 11 | WittyCouch | 0.3.0 | v0.2.1 | 0.3.0 | none |
| 12 | WittyCouchHost | 0.3.0 | v0.2.0 | 0.3.0 | none |
| 13 | WittyInk | 0.3.0 | v0.2.0 | 0.3.0 | none |
| 14 | WittyEbook | 0.3.0 | v0.2.0 | 0.3.0 | none: its WittyInk pin is raised during publishing (G4) |
| 15 | WittyPDF | 0.3.0 | v0.2.1 | 0.3.0 | none: as WittyEbook |
| 16 | WittyComics | 0.3.0 | v0.2.0 | 0.3.0 | none: as WittyEbook |
| 17 | WittyVisualizer | 0.3.0 | v0.2.1 | 0.3.0 | none |
| 18 | WittyStats | 0.2.0 | v0.1.2 | 0.2.0 | none |
| 19 | WittyModuleCatalog | no version: `serial` rises with each refresh (22 now) | the moving `catalog` release | lists only API 3 versions | after every module above |
| 20 | **WittyMediaServer** | **0.8.0-alpha** | v0.7.2-alpha | consistent (main.go, web package, README) | modules and catalogue first |
| 21 | WittyMediaClient | **0.5.0** | v0.4.1 | 0.5.0 (Apple build 2, Android code 9) | none (G2 fixed) |
| 22 | WittyReader | 0.2.0 | v0.1.1 | 0.2.0 ("not released yet" heading) | its lock is raised to the 0.3.0 plugins during publishing (G4); date the heading |
| 23 | WittyMediaPlayer | 0.1.3 (CMake `VERSION`) | tag v0.1.2 (its build failed; no release) | 0.1.3, `VISUALIZER_TAG` v0.3.0 | none: engine pins are raised during publishing |
| 24 | WittyMediaServer.com | not versioned: a push to `main` deploys it | n/a | builds hourly from GitHub releases | after the server and clients are released |
| 25 | WittyMediaRemoteMedia | **nothing to release** ("not started, on purpose") | none | README, design | push `main` only |

Changelogs: most modules keep their changes under `## Unreleased` with inline "**X 0.y.z:**" markers. Rename that heading
to the version and date at release (`wittymodule bump -changelog` does it for a module). WittyStats has no 0.1.2 section
although 0.1.2 was released. WittyMediaRemoteMedia and WittyMediaServer.com have no CHANGELOG (gate 6 finding).

## 2. Gate bugs found by this plan, and what became of them

| | What | Done |
|---|---|---|
| G1 | WittyModule said 0.3.0, a number already published with module API 3, and every module's `ci.yml`, `release.yml`, `withdraw.yml`, WittyDisc's workflows and the catalogue's `ci.yml`, `publish.yml` and `scripts/refresh.sh` ran `wittymodule@v0.3.0`: `publish-all.sh` would have skipped the existing tag and pinned everything to the old kit | **Fixed** 2026-11-12: WittyModule 0.4.0 (`maxServer` 0.8.x, a 0.4.0 changelog section), every `@v0.3.0` raised to `@v0.4.0` in 19 repositories and in `docs/MODULES.md`. WittyModule's tests pass |
| G2 | WittyMediaClient said 0.4.1 (already released) | **Fixed**: 0.5.0 in `apple/project.yml` (build 2), `android/app/build.gradle.kts` (code 9), `desktop/package.json` and its lock, and a 0.5.0 changelog section |
| G3 | The checklist asked every module's `sdui.json` to say `catalog: 8` | **Decided** (design lead): `catalog` is the lowest revision a module needs; the server takes 1 to 8. The checklist line is changed; no module is raised |
| G4 | npm and workflow pins: WittyComics, WittyEbook, WittyPDF pinned `WittyInk#v0.2.0` in their locks; WittyReader's lock held the 0.2.x reader plugins; WittyMediaPlayer built with `VISUALIZER_TAG v0.2.1`; WittyInk's `repository` named `#v0.2.0` | **Fixed**: `publish-all.sh`'s `lock()` now raises the WittyInk pin and regenerates the lock in each module that has one, right after WittyInk is published, and re-resolves WittyReader's four plugins after they are; `VISUALIZER_TAG` is v0.3.0 (the player's CI checks out that tag, so its first green run is after WittyVisualizer 0.3.0 is out); WittyInk's `repository` has no ref |
| G5 | The player copies (`wittydvdplayer`, `wittyblurayplayer`) have no downloadable release: the player's updater looks for `wittydvdplayer-v*` release tags; only the Go folder tags are made | Open, issue candidate (S7). No effect now: the player has the copies built in |
| G6 | `docs/MODULES-CUTOVER.md`'s "What this publishes" table and "all of them declare module API 3" are out of date | Open (S1). This plan supersedes it for 0.8.0-alpha |

**Before WittyModule is published (pre-tag check):** if the server's `pkg/sdui` changed after W7's last kit sync, run
`kit/sync-sdui-core.sh <server checkout>` in WittyModule again and its checks ([Release checklist](DESIGN-2-RELEASE.md#release-checklist),
part 3), and merge that before step 3. WittyModule is published first, and a module built against an older kit can't be
rebuilt under the same number.

**Parity audits** (`DESIGN-2-PARITY-*.md`, auditors W21 to W23) were paused by the owner until after the alpha: they are an
after-release task, with what they find filed as GitHub issues.

**The branch and the tag share a name.** The server's work is on branch `v0.8.0-alpha`, and its release tag is
`v0.8.0-alpha`. `publish-all.sh` uses full refs, so it is safe; anything typed by hand must say `refs/heads/v0.8.0-alpha`
or `refs/tags/v0.8.0-alpha`. After the release, `master` is fast-forwarded to the release (step 6).

## 3. CI that must be green first

Nothing on GitHub has ever run these workflows for this branch. Green locally first, then on GitHub:

- **Locally (S11 and S13 before the owner starts):** W7's checklist section 5; `scripts/publish-all.sh` dry run (step 2 below),
  which runs each module's own tests in a clean clone, builds a throwaway-signed catalogue, and builds and tests the server
  with no modules and then with them.
- **On GitHub, before any tag:** the server's `ci.yml` and `build.yml` (they run on a push of `v0.8.0-alpha`), the
  client's `build.yml` (runs on a push of `v0.8.0-alpha`), each module's `ci.yml` (runs on its push to `main`, inside
  publish-all, before the tag: watch the Actions tab if a tag's release fails).
- **The release workflows** themselves are the first runs: the server's `release.yml` (signs in environment `release`
  with `WMS_SIGNING_KEY`; its MSI upgrade test downloads the **previous** server release's `.msi`, so old releases are
  deleted only after this one is out), each module's `release.yml` (signs in `release`), the catalogue's `publish.yml`,
  the client's `release.yml`, WittyReader's and WittyMediaPlayer's `release.yml` (unsigned).

## 4. Owner steps, in order

Run in a terminal on the Mac (or WSL), with `gh` signed in (`gh auth status`). `~/projects` stands for the folder holding
the 25 repositories (here: `/Users/willowpoortenga/Documents/Github/WittyMediaServer`).

### Step 0. Once: keys and secrets (docs/MODULES-CUTOVER.md sections 1 to 3)

```bash
scripts/set-module-secret.sh --require-approval --protect-tags ~/.wms-signing/module.key
```

```bash
scripts/set-module-secret.sh --check
```

```bash
gh variable set MODULE_PUBLIC_KEY -R wittywilla/WittyModuleCatalog --body "oITeMfo/k0yM4l5saoL1ypTfXxdacZTfa0kg5PNmf6s="
```

and the server's `release` environment with `WMS_SIGNING_KEY` = `~/.wms-signing/release.key` (docs/PUBLISHING.md,
"Looking after the key"). Back both keys up first.

### Step 1. Push the server and client branches, no tags, and let CI run

```bash
git -C ~/projects/WittyMediaServer push origin refs/heads/v0.8.0-alpha:refs/heads/v0.8.0-alpha
```

```bash
git -C ~/projects/WittyMediaClient push origin refs/heads/v0.8.0-alpha:refs/heads/v0.8.0-alpha
```

Wait for `ci.yml` and `build.yml` to be green on both (Actions tab). The push also opens the "Release 0.8.0-alpha
tracking" issue.

### Step 2. Rehearse everything (about half an hour; nothing leaves the machine)

```bash
git clone -q --branch v0.8.0-alpha ~/projects/WittyMediaServer ~/wms-publish
```

```bash
cd ~/wms-publish && scripts/publish-all.sh --repos ~/projects --branch v0.8.0-alpha
```

`--branch v0.8.0-alpha` matters: without it the modules publish their old `main`. It must end with "The dry run passed".

### Step 3. Publish the libraries, modules, catalogue and server

```bash
cd ~/wms-publish && scripts/publish-all.sh --repos ~/projects --branch v0.8.0-alpha --apply
```

What it does, in this order, stopping at the first failure (rerun with `--from <Repository>` to continue):

a. **WittyModule v0.4.0**, then **WittyDisc v0.2.1** (tagged, unsigned).
b. The program modules, each pinned to the new WittyModule and WittyDisc, tested, pushed to `main`, tagged, and waited for
   until its signed release appears (approve each in the Actions tab): WittyDVD v0.4.0 (then `wittydvdplayer/v0.3.0`),
   WittyBluray v0.4.0 (then `wittyblurayplayer/v0.3.0`), WittyCD v0.2.1, WittyTuner v0.3.0, WittyPhotos v0.4.0,
   WittyIntegratedFileBrowser v0.3.0, WittyRigs v0.3.0, WittyPlay v0.4.0, WittyCouch v0.3.0, WittyCouchHost v0.3.0.
c. The web-bundle modules: WittyInk v0.3.0, WittyEbook v0.3.0, WittyPDF v0.3.0, WittyComics v0.3.0, WittyVisualizer v0.3.0,
   WittyStats v0.2.0.
d. WittyReader, WittyMediaPlayer and WittyMediaClient: **pushed to `main`, not tagged** (step 4 tags them). WittyMediaPlayer's
   engine is pinned to the new DVD, Bluray, CD and player copies on the way.
e. **WittyModuleCatalog:** pushed, then `refresh.yml` runs, adds every release above, and `publish.yml` signs and uploads
   the catalogue to the `catalog` release (approve it). The script checks the published signature.
f. **WittyMediaServer:** pinned to WittyModule 0.4.0 and WittyDisc 0.2.1, built with no module compiled in, tested, pushed to
   `refs/heads/v0.8.0-alpha`, tagged `v0.8.0-alpha`; `release.yml` builds every platform, waits for approval, signs, publishes
   a pre-release and pushes the Docker image `ghcr.io/wittywilla/wittymediaserver:alpha`.

Pins raised by the script on the way (each committed as "Uses the versions this release publishes"): every module's
`go.mod` to `wittymodule v0.4.0` (and `wittydisc v0.2.1` for DVD and Bluray); `wittydvdplayer/go.mod` to `wittydvd v0.4.0`,
`wittyblurayplayer/go.mod` to `wittybluray v0.4.0`; WittyMediaPlayer `engine/go.mod` to wittydvd 0.4.0, wittybluray
0.4.0, wittycd 0.2.1, wittydisc 0.2.1, wittydvdplayer 0.3.0, wittyblurayplayer 0.3.0, wittymodule 0.4.0; the server's
`go.mod` to wittymodule 0.4.0 and wittydisc 0.2.1.

`lock()` also raises, during the same run: the WittyInk pin in WittyEbook, WittyPDF and WittyComics (package.json and
lock, committed before each is tagged), and WittyReader's lock to the plugins just published (before it is pushed).

### Step 4. Tag the apps (each has its own `release.yml`; none is signed with a key)

```bash
git -C ~/projects/WittyMediaClient fetch origin && git -C ~/projects/WittyMediaClient tag -a v0.5.0 -m "WittyMediaClient 0.5.0" origin/main && git -C ~/projects/WittyMediaClient push origin refs/tags/v0.5.0
```

Wait for its release (desktop for every OS, Flatpak, Snap, FreeBSD, Android APKs, unsigned iOS and tvOS IPAs and the
AltStore source), then:

```bash
git -C ~/projects/WittyReader fetch origin && git -C ~/projects/WittyReader tag -a v0.2.0 -m "WittyReader 0.2.0" origin/main && git -C ~/projects/WittyReader push origin refs/tags/v0.2.0
```

```bash
git -C ~/projects/WittyMediaPlayer fetch origin && git -C ~/projects/WittyMediaPlayer tag -a v0.1.3 -m "WittyMediaPlayer 0.1.3" origin/main && git -C ~/projects/WittyMediaPlayer push origin refs/tags/v0.1.3
```

(WittyMediaPlayer is "not a blocker" under gate 5: if its build fails again, leave the tag and file it.)

### Step 5. Website and WittyMediaRemoteMedia

```bash
git -C ~/projects/WittyMediaServer.com push origin refs/heads/v0.8.0-alpha:refs/heads/main
```

```bash
git -C ~/projects/WittyMediaRemoteMedia push origin refs/heads/v0.8.0-alpha:refs/heads/main
```

The website deploys on that push (the `live` branch, Hostinger) and rebuilds hourly from the releases.

### Step 6. Bring `master` up to the release, and local copies up to date

```bash
git -C ~/projects/WittyMediaServer push origin refs/tags/v0.8.0-alpha^{commit}:refs/heads/master
```

```bash
for r in ~/projects/*/; do git -C "$r" fetch -q origin --tags; done
```

Then docs/MODULES-CUTOVER.md step 6 (local copies drop their `go.work`).

### Step 7. Delete the old GitHub releases (the owner asked for this originally)

**Only after steps 3 to 6 are done and the new releases are out.** Before then, the server's MSI upgrade test has no previous
release, and servers' module stores read the catalogue's versions from these releases. Keep the tags: deleting a tag frees
nothing (the Go proxy keeps every published version), and the catalogue says what was published.

First withdraw the old module versions in the catalogue, so no server offers a version whose files are gone: in
WittyModuleCatalog set `"withdrawn"` on each version listed below and raise `serial` (versions are never removed, per its
README), commit, push `main` (`publish.yml` signs it). Then:

```bash
gh release delete v0.7.2-alpha -R wittywilla/WittyMediaServer --yes
```

and the same for each line of this list (34 releases; **not** the catalogue's `catalog` release, which every server reads):

| Repository | Releases to delete |
|---|---|
| WittyMediaServer | v0.7.2-alpha, v0.7.1-alpha, v0.7.0-alpha, v0.6.2-alpha, v0.6.1-alpha, v0.6.0-alpha, v0.5.2-alpha, v0.4.0-alpha, v0.2.0-alpha |
| WittyMediaClient | v0.4.1, v0.4.0, v0.2.3 |
| WittyPlay | v0.3.6, v0.3.5, v0.3.4 |
| WittyModule | v0.3.0 |
| WittyDisc | v0.2.0 |
| WittyDVD | v0.3.1 |
| WittyBluray | v0.3.0 |
| WittyCD | v0.2.0 |
| WittyTuner | v0.2.0 |
| WittyPhotos | v0.3.0 |
| WittyIntegratedFileBrowser | v0.2.0 |
| WittyRigs | v0.2.0 |
| WittyCouch | v0.2.1 |
| WittyCouchHost | v0.2.0 |
| WittyInk | v0.2.0 |
| WittyEbook | v0.2.0 |
| WittyPDF | v0.2.1 |
| WittyComics | v0.2.0 |
| WittyVisualizer | v0.2.1 |
| WittyStats | v0.1.2 |
| WittyReader | v0.1.1 |
| WittyModuleCatalog | none (keep `catalog`) |

Or all at once, after reading it:

```bash
while read -r repo tag; do gh release delete "$tag" -R "wittywilla/$repo" --yes; done <<'EOF'
WittyMediaServer v0.7.2-alpha
WittyMediaServer v0.7.1-alpha
WittyMediaServer v0.7.0-alpha
WittyMediaServer v0.6.2-alpha
WittyMediaServer v0.6.1-alpha
WittyMediaServer v0.6.0-alpha
WittyMediaServer v0.5.2-alpha
WittyMediaServer v0.4.0-alpha
WittyMediaServer v0.2.0-alpha
WittyMediaClient v0.4.1
WittyMediaClient v0.4.0
WittyMediaClient v0.2.3
WittyPlay v0.3.6
WittyPlay v0.3.5
WittyPlay v0.3.4
WittyModule v0.3.0
WittyDisc v0.2.0
WittyDVD v0.3.1
WittyBluray v0.3.0
WittyCD v0.2.0
WittyTuner v0.2.0
WittyPhotos v0.3.0
WittyIntegratedFileBrowser v0.2.0
WittyRigs v0.2.0
WittyCouch v0.2.1
WittyCouchHost v0.2.0
WittyInk v0.2.0
WittyEbook v0.2.0
WittyPDF v0.2.1
WittyComics v0.2.0
WittyVisualizer v0.2.1
WittyStats v0.1.2
WittyReader v0.1.1
EOF
```

What deleting them changes: servers on 0.7.x see only 0.8.0-alpha in Settings → About (wanted); the website lists only the
new releases within the hour; Docker images already on ghcr.io stay (delete them under the package's settings if wanted).

### Step 8. Issues

After you approve the list in `HANDOVER-0.8.0.md` (top section) and `docs/plans/RELEASE-0.8.0-ISSUES.md`, file them;
`scripts/release-tracking.sh` keeps the tracking issue in step.
