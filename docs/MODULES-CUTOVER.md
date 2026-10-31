# Moving the modules to their own repositories

Today the modules live in this repository, and their own repositories are copies. After the
cutover each repository stands alone, with its own workflows and releases, and the server requires
released versions. `scripts/cutover-modules.sh` does the work. Run on its own, it only rehearses.

There are two lists below: [the owner's steps](#the-owners-steps), which need a person, and
[the coordinating session's checklist](#the-coordinating-sessions-checklist).

## The signing key: one module key, separate from the server's

**Recommendation: make one key for all modules, and keep it apart from the server's release key.
Not one key per module.**

- **Why not the server's key.** A key in the secrets of many repositories is exposed far more than
  one in the server's alone: anyone who gets into any one of them, or any of the workflows that use
  it, has it. A module key can't sign a server, so losing it can't put a bad server on anyone's
  machine.
- **Why not one per module.** Each server has to know each key, so per-module keys mean 13
  rotations and a server update for each, and they buy little: any module reaches an admin's
  session once it is running, so a stolen key for the smallest module is about as bad as one for
  the largest.
- **Forks** have their own keys anyway (`wittymodule keygen`), and people choose to trust them.
- **Only the repositories that sign hold it.** WittyModule and WittyDisc reach a server inside
  others and WittyReader isn't delivered through one, so they sign nothing and never get the
  secret. That leaves 10 repositories.

The code already supports this: `moduleKey` in `internal/update/keys.go`, read by
`update.ModuleKeys`. Until it is set, modules are checked against the release key.

**A server only accepts what it has a key for.** A release signed with the module key is accepted
by servers built with `moduleKey` set, and by no older one. So the public half goes into
`keys.go` before the cutover, and a server release with it follows the cutover.

## The owner's steps

Words you'll see: a *secret* is a value GitHub keeps encrypted and never shows again; an
*environment* is a named set of secrets and rules that only the jobs naming it can use.

Run the commands below in **Git Bash**, inside the `WittyMediaServer` folder.

### 1. The repositories

All 13 already exist, public, with `main` as their default branch (checked on 2026-10-31 with
`gh repo view`, reading only): WittyModule, WittyDisc, WittyDVD, WittyBluray,
WittyIntegratedFileBrowser, WittyPhotos, WittyTuner, WittyPlay, WittyInk, WittyEbook, WittyPDF,
WittyComics, WittyReader. **None is missing, so there is nothing to create.**

If the cutover ever says one is missing, creating one in the browser goes like this:

1. Open <https://github.com/new>.
2. *Owner*: `wittywilla`. *Repository name*: exactly as the script printed it, with its capitals.
3. Choose **Public**.
4. Leave *Add a README*, *.gitignore* and *license* off. Each folder brings its own.
5. Click **Create repository**, then run the script again.

### 2. Make the module key

```bash
scripts/new-module-key.sh ~/.wms-signing/module.key
```

It refuses to replace a file that is already there, won't put the key inside this repository, and
never opens your existing key. It prints the public half, which isn't secret, and where that goes:
`moduleKey` in `internal/update/keys.go`. Give that line to the coordinating session, or put it
in yourself.

### 3. Back up your keys

There are two now: `release.key` signs the server, and `module.key` signs modules. Do this before
anything else uses them.

1. Copy each file to a password manager, as an attached file or a secure note, or to a USB stick
   you keep somewhere safe. Label which is which.
2. Never email them, put them in a chat, or keep them in a cloud folder others can open, and never
   copy them into this project's folder.

What each loss costs:

- **Module key lost:** make a new one, set it in `keys.go`, and publish a server release. Servers
  older than that release can't take new module versions until they update. They keep working
  with what they have, and the built-in copies always work.
- **Module key stolen:** the same, and replace the secret in every repository straight away.
- **Release key:** see "Signing releases" in [PUBLISHING.md](PUBLISHING.md). It is the one that
  matters most.

### 4. Give the repositories the key

**By the script** (one command, for all 10):

```bash
scripts/set-module-secret.sh ~/.wms-signing/module.key
scripts/set-module-secret.sh --check
```

(Step 5 adds two options to the first of these, and running it again with them is fine.)

It creates each repository's `release` environment and sets the secret inside it. The key's text is
never printed: it goes from the file to the GitHub CLI, which encrypts it before sending. `--check`
lists names only. The GitHub CLI can do every part of this, and it is signed in with what it needs,
so nothing here has to be done by hand. It hasn't been run yet, because it changes settings.

**By clicks** (10 times, once per repository: WittyDVD, WittyBluray, WittyIntegratedFileBrowser,
WittyPhotos, WittyTuner, WittyPlay, WittyInk, WittyEbook, WittyPDF, WittyComics):

1. Open the repository on github.com and click **Settings** (the top row, on the right).
2. In the left column, click **Environments**, then **New environment**.
3. Name it `release`, in lower case, and click **Configure environment**.
4. Scroll to *Environment secrets* and click **Add environment secret**.
5. *Name*: `WMS_SIGNING_KEY`. *Value*: open `module.key` in Notepad, copy **all** of it, both lines,
   and paste it. Click **Add secret**.

### 5. Strongly recommended: approve each signature, and protect version tags

Do both. All modules share one key, so a compromised build in any one repository could otherwise
get a release of another module signed, and every server would accept it. The signing step refuses
a checksum list that names another module, but someone who can change a repository's workflow could
remove that check. Your approval and the tag rule are what such a person can't get past without your
sign-in.

- **Approving signatures:** a release or withdrawal stops before it is signed, and waits for you to
  click **Approve** on its page in the *Actions* tab. It costs a click for each one. A cutover makes
  10 of them, one at a time, and sits and waits for each.
- **Protecting version tags:** only you, as the repository's admin, can create, move or delete a tag
  starting with `v`. A release starts from such a tag, so nothing else can start one.

**By the script** (for all 10): add both options to the command in step 4.

```bash
scripts/set-module-secret.sh --require-approval --protect-tags ~/.wms-signing/module.key
```

**By clicks**, in each of the 10 repositories:

1. *Settings* > *Environments* > `release`. Tick **Required reviewers**, add yourself, and click **Save
   protection rules**. Leave *Prevent self-review* off, since you are the one who tags.
2. *Settings* > *Rules* > *Rulesets* > **New ruleset** > **New tag ruleset**.
3. *Ruleset name*: `Version tags`. *Enforcement status*: **Active**.
4. Under *Bypass list*, click **Add bypass**, tick **Repository admin**, and click **Add selected**.
5. Under *Target tags*, click **Add target** > **Include by pattern**, type `v*`, and click **Add
   inclusion pattern**.
6. Under *Rules*, tick **Restrict creations**, **Restrict updates** and **Restrict deletions**, and click
   **Create**.

## The coordinating session's checklist

Before:

- [ ] Every session editing modules has merged. No other session is working in `modules/`,
      `web/src/modules/` or `apps/wittyreader`.
- [ ] The dry run passes on the final branch, naming what had to be merged, so it refuses if one
      wasn't. It needs python3 with PyYAML and `zip`, so run it in Ubuntu, in a clone on Ubuntu's disk:
      `scripts/cutover-modules.sh --require-merged branch-one,branch-two`
- [ ] The owner has done steps 2 to 5, and `--check` shows the secret, the approval rule and the tag
      rule in all 10. With approvals on, the owner has to be at hand during `--apply`.
- [ ] `moduleKey` is set in `internal/update/keys.go` and committed.

The real thing:

- [ ] `scripts/cutover-modules.sh --apply --require-merged branch-one,branch-two`. Use the machine
      where `gh` and `git push` are signed in. It stops at the first failure and can be run again:
      what is already released is left alone, and tags never move.
- [ ] It leaves branch `modules-cutover` with the server's change, unpushed. Open a pull request.

After:

- [ ] CI is green on the pull request. Build the Docker image, which is the one thing a dry run
      can't prove: `docker build .`
- [ ] Try one module's *withdraw* workflow on a version made for the purpose.
- [ ] A server release with `moduleKey` set, so servers can take module releases.
- [ ] Update the docs that name the folders: `docs/MODULES.md` (the coordinator's own),
      `docs/REFERENCE.md`, `docs/DVD.md`, `docs/DVD-QUIRKS.md`, `docs/BLURAY.md`, `docs/CHANNELS.md`,
      `docs/GAMING.md`, `docs/WITTYRETROARCH.md`, `docs/ROADMAP.md`, and
      `clients/wittymediaclient/docs/DESIGN.md`, which plans to use `scripts/mirror.sh`.
- [ ] Delete `scripts/cutover-modules.sh` and `scripts/module-kit/cutover-server.mjs`. Keep
      `scripts/module-kit.sh`, which can still regenerate what is in a clone of a module, such as
      `scripts/module-kit.sh ../WittyPlay`.

### Versions

Versions move in small steps, and a bump isn't a release.

- **A module:** `wittymodule bump [-part minor] [-note text] [-changelog CHANGELOG.md] [-consumer .] <folder>`
  raises the version everywhere it is written, and with `-consumer` in what the server requires.
  Nothing is tagged.
- **Each module's CI** runs `wittymodule needs-bump .`, which fails when code changed since the last
  tag and the version didn't. Documentation doesn't count. WittyReader has no manifest, so it has no
  such check, and its version is raised by hand in `package.json`.
- **The server:** `scripts/bump-server.sh <version> ["what changed"]`.
- **The cutover** raises a version only when it must publish and that version's tag is taken: one
  patch above the newest tag. It bumps nothing else. The coordinating session bumps at merge.

### What the cutover does, in order

1. Refuses to start with uncommitted changes, an unmerged required branch, or a missing repository.
2. For each module, leaves first (WittyModule, WittyDisc, then WittyDVD, WittyBluray,
   WittyIntegratedFileBrowser, WittyPhotos, WittyTuner, WittyPlay, then WittyInk, then WittyEbook,
   WittyPDF, WittyComics, then WittyReader): requires released versions of what it uses (`go get` and
   `go mod tidy` with `GOWORK=off`, or `github:` tags and a new `package-lock.json`), regenerates the
   kit with the released WittyModule, tests it alone, publishes the folder's history without rewriting
   anything, tags it, and waits for that release to finish, with its signature, before moving on.
3. In this repository: removes the module folders, `go.work` and `scripts/mirror.sh`; `go.mod` and
   `web/package.json` name the released versions; the Dockerfile, CI and `scripts/build-release.sh` stop
   naming folders. `scripts/module-kit/cutover-server.mjs` holds these edits, and stops if a file no
   longer looks as it expects.
4. Builds and tests the server that way.

### Developing a server and modules together, afterwards

`scripts/dev-modules.sh WittyPlay WittyEbook` clones those beside this checkout, makes a local `go.work`
(`.gitignore` covers it) and links them into `web/node_modules`. `scripts/dev-modules.sh --off` goes back
to the released versions.

The web half of WittyPlay and of WittyIntegratedFileBrowser is an npm package at the root of each
repository (`package.json`, exposing `web/index.ts`, generated by the kit), since the web build can't
find Go's module cache. `web/package.json` depends on them by tag like the readers. The manifests of
the web modules are copied into `web/module-manifests` by the web build and committed, so building the
server doesn't need Node. CI fails if they differ.

---
<sub>WittyMediaServer 0.4.0-alpha documentation</sub>
