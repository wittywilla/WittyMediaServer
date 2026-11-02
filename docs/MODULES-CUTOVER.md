# Publishing the modules

The steps a person has to take to put every module repository on GitHub, in order. This is for the project's owner, and isn't in the app's help.

Everything is ready in local repositories at `D:\projects\<Repository>`, and nothing has been pushed. Until it is, a server finds no modules to install, and works without them.

`scripts/publish-all.sh` does the publishing. Run on its own it only rehearses, in throwaway copies, and changes nothing on GitHub. `--apply` has never been run, and no GitHub workflow has.

Words you'll see: a *secret* is a value GitHub keeps encrypted and never shows again. An *environment* is a named set of secrets and rules that only the jobs naming it can use.

Run the commands below in **Git Bash**, except for steps 4 and 5. The `WittyMediaServer` folder itself is on `master`, which has none of this yet, so work in the folder that has the release branch:

```bash
cd /d/projects/WittyMediaServer/.claude/worktrees/release-integration
```

## 1. Make the module key

```bash
scripts/new-module-key.sh ~/.wms-signing/module.key
```

It refuses to replace a file that's already there, won't put the key inside this repository, and never opens your existing key. It prints the public half, which isn't secret. Put that in `moduleKey` in `internal/update/keys.go`, and commit it. It's empty now.

**Why one key for all modules, and not the server's.** A key in the secrets of many repositories is far more exposed than one in the server's alone. A module key can't sign a server, so losing it can't put a bad server on anyone's machine.

## 2. Back up your keys

There are two now: `release.key` signs the server, and `module.key` signs modules and the catalogue. Do this before anything uses them.

1. Copy each file to a password manager, or to a USB stick kept somewhere safe. Label which is which.
2. Never email them, put them in a chat, or keep them in a cloud folder others can open. Never copy them into this project's folder.

- **Module key lost:** make a new one, set it in `keys.go`, and publish a server release. Older servers can't take new module versions until they update, and keep working with what they have.
- **Module key stolen:** the same, and replace the secret in every repository straight away.
- **Release key:** see [Signing releases](PUBLISHING.md#signing-releases). It's the one that matters most.

## 3. Give the repositories the key

Every repository the release needs is already on GitHub, under its final name.

Give them the key, with approval and tag rules, and check that it's there. The second command lists names only, and the key's text is never printed:

```bash
scripts/set-module-secret.sh --require-approval --protect-tags ~/.wms-signing/module.key
```

```bash
scripts/set-module-secret.sh --check
```

- **Approving signatures:** a release stops before it's signed, and waits for you to click **Approve** on its page in the **Actions** tab.
- **Protecting version tags:** only you can create, move or delete a tag starting with `v`.

Both are strongly recommended. All modules share one key, so a compromised build in one repository could otherwise get a release of another module signed, and every server would accept it.

The catalogue also needs the public half of the key:

```bash
gh variable set MODULE_PUBLIC_KEY -R wittywilla/WittyModuleCatalog --body "<the public half from step 1>"
```

## 4. Rehearse

Open **Ubuntu**, and:

```bash
git clone -q --branch release/0.6.0-alpha /mnt/d/projects/WittyMediaServer ~/wms-publish
```

```bash
cd ~/wms-publish && scripts/publish-all.sh --repos /mnt/d/projects
```

It takes about half an hour. It clones every repository, runs each one's tests on its own, builds a catalogue with a throwaway key, and builds and tests a clean clone of the server, with no modules and then with them. Then it lists anything still waiting for you. Nothing leaves your machine.

## 5. Publish

This talks to GitHub from Ubuntu, so Ubuntu needs `gh`, signed in. Once:

```bash
sudo apt install gh
```

```bash
gh auth login
```

```bash
gh auth setup-git
```

Once the rehearsal passes and lists nothing, take a fresh copy, so it has your commit from step 1:

```bash
cd ~/wms-publish && git pull -q && scripts/publish-all.sh --repos /mnt/d/projects --apply
```

Stay nearby: it stops for your **Approve** on each module's release. It stops at the first failure. To carry on after fixing something, add `--from <the repository it stopped at>`: what's already published is left alone, and tags never move.

In order it does these:

1. WittyModule, then WittyDisc: libraries, with nothing to sign.
2. WittyDVD, WittyBluray, WittyCD, WittyTuner, WittyPhotos, WittyIntegratedFileBrowser, WittyRigs, WittyPlay, WittyCouch, WittyCouchHost. For each: pins it to what was just published, tests it alone, pushes, tags, and waits for its signed release.
3. Straight after WittyDVD and WittyBluray, their copies for WittyMediaPlayer, each tagged with its folder in front: `wittydvdplayer/v0.2.0` and `wittyblurayplayer/v0.2.0`.
4. WittyInk first, as the readers build on it, then WittyEbook, WittyPDF, WittyComics, WittyVisualizer and WittyStats, the same way.
5. WittyReader, WittyMediaPlayer and WittyMediaClient: pushed, with WittyMediaPlayer pinned and tested first. They aren't tagged, so they have no release yet.
6. WittyModuleCatalog: runs its refresh, and checks the published catalogue's signature.
7. WittyMediaServer: a clean clone builds and passes its tests, is pushed to `release/0.6.0-alpha`, and is tagged, which starts its own release. Add `--no-server-tag` to stop short of that.

## 6. Afterwards

- **Merge the server into `master`,** which is what people see on GitHub. The script only pushes `release/0.6.0-alpha`:

  ```bash
  git checkout master && git pull && git merge origin/release/0.6.0-alpha && git push
  ```

- **Bring your own copies up to date.** The script pins in throwaway clones, so GitHub has commits your folders don't. In each repository run `git pull`, or your next push is refused. WittyStats has no `origin` yet: `git -C D:/projects/WittyStats remote add origin https://github.com/wittywilla/WittyStats.git`, then `git -C D:/projects/WittyStats fetch origin`, then `git -C D:/projects/WittyStats branch -u origin/main main`.
- **Release WittyMediaClient.** The server's download page sends people to its Releases page, which is empty until you tag it. Its version is in `desktop/package.json`. Do the same for WittyMediaPlayer and WittyReader when you want them out.
- Open a server, go to **Settings → Modules**, and see the store list the new versions.
- Build the Docker image, which nothing above covers: `docker build .`. It needs `wittymodule` v0.3.0 on GitHub, and no other module, as the image holds none.
- Try one module's withdraw workflow, on a version made for the purpose.

## What this publishes

The server's `go.mod` names one thing from this family, `github.com/wittywilla/wittymodule` at **v0.3.0**. The modules a running server offers come from the catalogue, and all of them declare module API 3 and `minServer` 0.6.0-alpha. GitHub only has `v0.1.0` of most of these, `v0.2.0` of WittyPhotos, and none of the rest.

| Tag | Repositories |
|---|---|
| v0.3.1 | WittyPlay |
| v0.3.0 | WittyModule, WittyDVD, WittyBluray, WittyPhotos |
| v0.2.1 | WittyCouch, WittyPDF |
| v0.2.0 | WittyDisc, WittyCD, WittyTuner, WittyCouchHost, WittyRigs, WittyIntegratedFileBrowser, WittyInk, WittyEbook, WittyComics, WittyVisualizer |
| v0.1.2 | WittyStats |
| wittydvdplayer/v0.2.0 | WittyDVD |
| wittyblurayplayer/v0.2.0 | WittyBluray |

WittyMediaPlayer, WittyReader and WittyMediaClient get no tag from this. A version comes from each repository's own `module.json` when it's published, so this table only says what to expect.

## Before anything builds outside this machine

Today only this machine can build these, through a `go.work` that points at the folders beside each other. Anyone else, and any GitHub workflow, downloads what `go.mod` names, and none of it exists yet:

- **Every module's `go.mod` names `wittymodule` v0.3.0,** and the disc modules name `wittydisc` v0.2.0. GitHub has `v0.1.0` of each.
- **Every `go.sum` holds a checksum for a v0.3.0 that was never published.** `main` has moved on since, so tagging it now gives a different checksum, and Go refuses a download that doesn't match. Rewriting the checksums is therefore part of publishing.
- **WittyMediaPlayer's engine has no `go.sum` at all,** and needs the two folder tags above.
- **Module web bundles name WittyInk by its GitHub address,** so they can't install until it's pushed.

Step 5 fixes all of these, in order. Each repository is pinned to what was just published, then tested alone in a clone, then pushed and tagged. So:

1. Don't tag anything by hand. A tag can't be moved once a build has downloaded it.
2. The first moment a stranger's `go build` works is when step 5 has pushed WittyModule's v0.3.0, for the server, and the last of what each repository names, for the others.
3. `docker build .` and the server's own release workflow need only `wittymodule` v0.3.0.
4. Module CI on GitHub stays red until its repository has been through step 5, and WittyMediaPlayer's until the end of it.

## If something goes wrong

| It says | What it means |
|---|---|
| `GitHub has ... which isn't in` | That repository's history on GitHub isn't part of what you're publishing. Merge it in first |
| `hasn't had the module work merged` | A repository's branch is older than module API 3: merge it into its `main` |
| `moduleKey ... is empty` | Step 1, and it needs committing |
| `has no WMS_SIGNING_KEY` or `no MODULE_PUBLIC_KEY` | Step 3 |
| `release didn't appear in an hour` | Its release is waiting for your approval, or failed. Open its **Actions** tab. For a passing problem, click **Re-run failed jobs**, then run the script again with `--from` |
| `names something it mustn't` | A name that isn't allowed in a repository has got in. The first lines of what it found are printed |

## Versions

Versions move in small steps, and a bump isn't a release.

- **A module:** `wittymodule bump [-part minor] [-note text] [-changelog CHANGELOG.md] <folder>` raises the version everywhere it's written. Nothing is tagged.
- **Each module's CI** runs `wittymodule needs-bump .`, which fails when code changed since the last tag and the version didn't. Documentation doesn't count. WittyReader has no manifest, so it has no such check.
- **The server:** `scripts/bump-server.sh <version> ["what changed"]`.

---

<sub>WittyMediaServer documentation</sub>
