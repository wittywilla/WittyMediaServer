# Publishing the modules

The steps a person has to take to put every module repository on GitHub, in order. This is for the project's owner, and isn't in the app's help.

Everything is ready in local repositories at `D:\projects\<Repository>`, and nothing has been pushed. Until it is, a server finds no modules to install, and works without them.

`scripts/publish-all.sh` does the publishing. Run on its own it only rehearses, in throwaway copies, and changes nothing on GitHub. `--apply` has never been run, and no GitHub workflow has.

Words you'll see: a *secret* is a value GitHub keeps encrypted and never shows again. An *environment* is a named set of secrets and rules that only the jobs naming it can use.

Run the commands below in **Git Bash**, inside the `WittyMediaServer` folder, except for steps 5 and 6.

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

## 3. Prepare the repositories

Rename the two that have new names, and make the two that don't exist yet. `gh repo create` needs doing here for the secret and variable below to have somewhere to go:

```bash
gh repo rename WittyCouchHost -R wittywilla/WittyMoonlightHost --yes
```

```bash
gh repo rename WittyRigs -R wittywilla/WittySunshine --yes
```

```bash
gh repo create wittywilla/WittyModuleCatalog --public
```

```bash
gh repo create wittywilla/WittyStats --public
```

Then give the repositories the key, with approval and tag rules, and check that it's there. The second command lists names only, and the key's text is never printed:

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

## 4. Delete the repository that's no longer used

`WittyMoonlight` holds only a README, and WittyCouch replaces it. Deleting can't be undone, so it's left to you:

1. Open the repository's **Settings** on GitHub.
2. Scroll to the bottom, and click **Delete this repository**.

## 5. Rehearse

Open **Ubuntu**, and:

```bash
cd /mnt/d/projects/WittyMediaServer && scripts/publish-all.sh --repos /mnt/d/projects
```

It takes about half an hour. It clones every repository, runs each one's tests on its own, builds a catalogue with a throwaway key, and builds and tests a clean clone of the server, with no modules and then with them. Then it lists anything still waiting for you. Nothing leaves your machine.

## 6. Publish

This talks to GitHub from Ubuntu, so Ubuntu needs `gh`, signed in. Once:

```bash
sudo apt install gh
```

```bash
gh auth login
```

Once the rehearsal passes and lists nothing:

```bash
cd /mnt/d/projects/WittyMediaServer && scripts/publish-all.sh --repos /mnt/d/projects --apply
```

Stay nearby: it stops for your **Approve** on each module's release. It stops at the first failure. To carry on after fixing something, add `--from <the repository it stopped at>`: what's already published is left alone, and tags never move.

In order it does these:

1. Renames what needs it, and creates what's missing.
2. WittyModule, then WittyDisc: libraries, with nothing to sign.
3. WittyDVD, WittyBluray, WittyCD, WittyTuner, WittyPhotos, WittyIntegratedFileBrowser, WittyRigs, WittyPlay, WittyCouch, WittyCouchHost. For each: pins it to what was just published, tests it alone, pushes, tags, and waits for its signed release.
4. WittyInk first, as the readers build on it, then WittyEbook, WittyPDF, WittyComics, WittyVisualizer and WittyStats, the same way.
5. WittyReader, WittyMediaPlayer and WittyMediaClient: pushed, and released on their own schedules.
6. WittyModuleCatalog: runs its refresh, and checks the published catalogue's signature.
7. WittyMediaServer: a clean clone builds and passes its tests, is pushed, and is tagged, which starts its own release. Add `--no-server-tag` to stop short of that.

## 7. Afterwards

- Open a server, go to **Settings → Modules**, and see the store list the new versions.
- Build the Docker image, which nothing above covers: `docker build .`. It needs `wittymodule` v0.3.0 on GitHub, and no other module, as the image holds none.
- Try one module's withdraw workflow, on a version made for the purpose.

## What this publishes

The server's `go.mod` names one thing from this family, `github.com/wittywilla/wittymodule` at **v0.3.0**. The modules a running server offers come from the catalogue, and all of them declare module API 3 and `minServer` 0.6.0-alpha.

| Tag | Repositories |
|---|---|
| v0.3.0 | WittyModule, WittyDVD, WittyBluray, WittyPhotos, WittyPlay |
| v0.2.0 | WittyDisc, WittyCD, WittyTuner, WittyCouch, WittyCouchHost, WittyRigs, WittyIntegratedFileBrowser, WittyInk, WittyEbook, WittyPDF, WittyComics, WittyVisualizer |
| v0.1.1 | WittyStats |

## If something goes wrong

| It says | What it means |
|---|---|
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
