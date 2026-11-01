# Publishing the modules: what's left for a person

Modules no longer live in this repository, and the server has none of their code in it. Everything is
ready in local repositories at `D:\projects\<Repository>`, and nothing has been pushed. This page is what
a person has to do to put it all on GitHub. Until then a server finds no modules to install, and works
without them.

[`scripts/publish-all.sh`](../scripts/publish-all.sh) does the publishing itself. Run on its own it only
rehearses, in throwaway copies, and changes nothing on GitHub.

Words you'll see: a *secret* is a value GitHub keeps encrypted and never shows again; an *environment* is a
named set of secrets and rules that only the jobs naming it can use.

Run the commands below in **Git Bash**, inside the `WittyMediaServer` folder, except for step 6.

## The steps, in order

### 1. Merge what's on the branches

Each module's work is on a branch named `modules-cutoff`, and the server's is on its own branch. They need
merging into each module's `main` and into `release/0.6.0-alpha`. The coordinating session does this. If
`publish-all.sh` says a repository "hasn't had the module work merged", this is what's missing.

### 2. Make the module key

```bash
scripts/new-module-key.sh ~/.wms-signing/module.key
```

It refuses to replace a file that is already there, won't put the key inside this repository, and never
opens your existing key. It prints the public half, which isn't secret, and where that goes: `moduleKey` in
`internal/update/keys.go`. Give that line to the coordinating session, or put it in yourself.

**Why one key for all modules, and not the server's.** A key in the secrets of many repositories is exposed far
more than one in the server's alone. A module key can't sign a server, so losing it can't put a bad server on
anyone's machine. One key per module would mean a server update for each rotation, and buys little: any module
reaches an admin's session once it's running.

### 3. Back up your keys

There are two now: `release.key` signs the server, and `module.key` signs modules and the catalogue. Do this
before anything else uses them.

1. Copy each file to a password manager, as an attached file or a secure note, or to a USB stick you keep
   somewhere safe. Label which is which.
2. Never email them, put them in a chat, or keep them in a cloud folder others can open, and never copy them
   into this project's folder.

What each loss costs:

- **Module key lost:** make a new one, set it in `keys.go`, and publish a server release. Servers older than that
  release can't take new module versions until they update. They keep working with what they have.
- **Module key stolen:** the same, and replace the secret in every repository straight away.
- **Release key:** see "Signing releases" in [PUBLISHING.md](PUBLISHING.md). It is the one that matters most.

### 4. Give the repositories the key, with approval and tag rules

```bash
scripts/set-module-secret.sh --require-approval --protect-tags ~/.wms-signing/module.key
```

```bash
scripts/set-module-secret.sh --check
```

The first creates each repository's `release` environment and sets the secret in it. The key's text is never
printed. The second lists names only, so you can see they're all there. Both options are strongly recommended:
all modules share one key, so a compromised build in one repository could otherwise get a release of another module
signed, and every server would accept it.

- **Approving signatures:** a release stops before it is signed, and waits for you to click **Approve** on its page
  in the *Actions* tab. Publishing everything means one click per module, one at a time.
- **Protecting version tags:** only you, as the repository's admin, can create, move or delete a tag starting with
  `v`.

The catalogue also needs the public half of the key, in a variable. `publish-all.sh` creates the catalogue's repository
if it isn't there, so do this after step 7's dry run says it's what's left, or create the repository first:

```bash
gh repo create wittywilla/WittyModuleCatalog --public
```

```bash
gh variable set MODULE_PUBLIC_KEY -R wittywilla/WittyModuleCatalog --body "<the public half from step 2>"
```

The same two renames as the script does in step 7 have to come before `set-module-secret.sh`, or it won't find the
repositories under their new names:

```bash
gh repo rename WittyCouchHost -R wittywilla/WittyMoonlightHost --yes
```

```bash
gh repo rename WittyRigs -R wittywilla/WittySunshine --yes
```

### 5. Delete a repository that's no longer used

`WittyMoonlight` holds only a README, and WittyCouch replaces it. Deleting can't be undone, so it's left to you:

1. Open <https://github.com/wittywilla/WittyMoonlight/settings>.
2. Scroll to the bottom, and click **Delete this repository**.

### 6. Rehearse

Open **Ubuntu**, and:

```bash
cd /mnt/d/projects/WittyMediaServer && scripts/publish-all.sh --repos /mnt/d/projects
```

It takes about half an hour. It clones every repository, uses each at the versions it's about to publish, runs each one's
tests on its own, builds a catalogue from them with a throwaway key, and builds and tests a clean clone of the server, with
no modules and then with them. Then it lists anything still waiting for you. Nothing leaves your machine.

### 7. Publish

Once the rehearsal passes and lists nothing:

```bash
cd /mnt/d/projects/WittyMediaServer && scripts/publish-all.sh --repos /mnt/d/projects --apply
```

Stay nearby: it stops for your **Approve** on each module's release, in the *Actions* tab of each repository. It goes in the
order they depend on each other, and it stops at the first failure. To carry on after fixing something, add `--from <the
repository it stopped at>`: what's already published is left alone, and tags never move.

In order it:

1. Renames what needs it, and creates what's missing.
2. WittyModule, then WittyDisc: libraries, with nothing to sign.
3. WittyDVD, WittyBluray, WittyCD, WittyTuner, WittyPhotos, WittyIntegratedFileBrowser, WittyRigs, WittyPlay, WittyCouch,
   WittyCouchHost. For each: pins it to what was just published, tests it alone, pushes, tags, and waits for its signed release.
4. WittyInk, then the readers and WittyVisualizer, the same way. WittyInk comes first, as the readers build on it.
5. WittyReader, WittyMediaPlayer and WittyMediaClient: pushed, and released on their own schedules.
6. WittyModuleCatalog: runs its refresh, and checks the published catalogue's signature.
7. WittyMediaServer: a clean clone builds and passes its tests, is pushed, and is tagged, which starts its own release. Add
   `--no-server-tag` to stop short of that.

### 8. Afterwards

- [ ] Open a server, *Settings, Modules*, and see the store list the new versions.
- [ ] Build the Docker image, which nothing above covers: `docker build .`
- [ ] Try one module's *withdraw* workflow on a version made for the purpose.

## If something goes wrong

| It says | What it means |
|---|---|
| `hasn't had the module work merged` | Step 1 |
| `moduleKey ... is empty` | Step 2, and it needs committing |
| `has no WMS_SIGNING_KEY` or `no MODULE_PUBLIC_KEY` | Step 4 |
| `release didn't appear in an hour` | Its release is waiting for your approval, or failed: open its *Actions* tab. For a passing problem, click *Re-run failed jobs*, and then run the script again with `--from` |
| `names something it mustn't` | A name that isn't allowed in a repository has got in. The first lines of what it found are printed |

## Versions

Versions move in small steps, and a bump isn't a release.

- **A module:** `wittymodule bump [-part minor] [-note text] [-changelog CHANGELOG.md] <folder>` raises the version
  everywhere it is written. Nothing is tagged.
- **Each module's CI** runs `wittymodule needs-bump .`, which fails when code changed since the last tag and the version
  didn't. Documentation doesn't count. WittyReader has no manifest, so it has no such check.
- **The server:** `scripts/bump-server.sh <version> ["what changed"]`.

---
<sub>WittyMediaServer 0.6.0-alpha documentation</sub>
