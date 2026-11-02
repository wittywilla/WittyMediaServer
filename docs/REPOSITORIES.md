# Repositories

Every repository the project has or needs, what's left to do for each on GitHub, and how to work on them. This is for the project's owner and developers, and isn't in the app's help.

Each is a project of its own, at `D:\projects\<Repository>` on the development machine, with `https://github.com/wittywilla/<Repository>.git` as its remote. The server's repository holds none of their source.

Every repository's README ends with a **Legal** section: no affiliation, no warranty, no media included, and what its names mean. For a new one, copy the wording from [Legal notices](LEGAL-NOTICES-CHECKLIST.md#module-readmes).

## The list

Every one is on GitHub under its final name, so nothing needs creating, renaming or deleting. All but WittyMediaRemoteMedia have local history that hasn't been pushed: [Publishing the modules](MODULES-CUTOVER.md) has the steps. WittyStats and WittyModuleCatalog are still empty there.

| Repository | What it's for |
|---|---|
| WittyMediaServer | The server |
| WittyModule | The shared tools and manifest format modules are built with, and the kit that writes a module's workflows |
| WittyModuleCatalog | The signed list of modules and versions that a server's store reads. Holds no module code |
| WittyDisc | Reading disc images, used by the disc engines |
| WittyDVD | The DVD engine, and in `wittydvdplayer/` its copy for WittyMediaPlayer |
| WittyBluray | The Blu-ray engine, and in `wittyblurayplayer/` its copy for WittyMediaPlayer |
| WittyCD | The CD image engine and on-screen CD player |
| WittyPlay | Console games streamed from the server |
| WittyCouch | A client in the page for playing on a game PC: its relay, protocol code and player. **Apache-2.0** |
| WittyCouchHost | The separate program built on Moonlight's own code, which speaks to the PC. **GPL-3.0**, with source offered beside each release |
| WittyRigs | An advanced option: hosting demanding games on the server's own hardware, using a separately installed Sunshine. **Apache-2.0** |
| WittyIntegratedFileBrowser | The built-in file browser |
| WittyPhotos | Photo processing |
| WittyTuner | TV tuners and IPTV |
| WittyInk | Reader plugin: notes and pen |
| WittyEbook | Reader plugin: EPUB |
| WittyPDF | Reader plugin: PDF |
| WittyComics | Reader plugin: comics and manga |
| WittyVisualizer | The audio visualizer plugin, for the server and the desktop player |
| WittyStats | A statistics screen you arrange yourself |
| WittyReader | The standalone reader app |
| WittyMediaPlayer | The standalone desktop player |
| WittyMediaClient | The client apps for desktop, Android, Apple and others |
| WittyMediaRemoteMedia | Connecting libraries on a NAS through a native or Docker app. Empty, and has no local copy: work not started. The roadmap is the only thing that names it |

**A new one on GitHub:** make it under `wittywilla`, as **Public**, with no README, `.gitignore` or licence.

Each desktop product makes its own installers, with the same rules for names and for removing everything: `packaging/` in the server's and the player's repository, `desktop/` in the client's and the reader's.

## Working on them

### The server needs almost none of them

A build of the server needs only WittyModule. Until its new version is published, point at a checkout of it, once in each checkout of the server. This writes a `go.work` that git ignores:

```bash
node scripts/dev-modules.mjs WittyModule=D:/projects/WittyModule
```

Once it's published, nothing needs setting up. To try real modules on a server, see [Developing a server and a module together](MODULES.md#developing-a-server-and-a-module-together).

A module's own checkout that builds on another, such as WittyDVD on WittyDisc, uses the copy outside every repository:

```bash
node D:/projects/dev-modules.mjs
```

`D:\projects\go.work` covers the main checkout of every module repository, which Go finds by walking up. It can't cover worktrees: Go refuses the same module listed twice, and a checkout outside its `use` list won't build. So each worktree has its own `go.work`. **Never run `go work use`**: remake it with the command above.

### Changing a module

Use a `git worktree` under its repository, on a branch of its own. Never work on `main`:

```bash
git -C D:/projects/WittyPlay worktree add .claude/worktrees/my-branch -b my-branch main
```

```bash
cd D:/projects/WittyPlay/.claude/worktrees/my-branch && node D:/projects/dev-modules.mjs
```

The module's tests run in its worktree with `go test ./...`. Commit there, on your branch, and report the branch.

A change to what the server calls, in WittyPlay and WittyCouch, means rerunning the generator, which writes the stubs on both sides. See [Developing a server and a module together](MODULES.md#developing-a-server-and-a-module-together).

### From WSL

A worktree's git link is a Windows path that WSL can't follow, so WSL uses clones, on its own disk. Module repositories are read through `/mnt/d/projects`:

```bash
git clone /mnt/d/projects/WittyMediaServer ~/wms-my-branch
```

```bash
cd ~/wms-my-branch && git fetch origin my-branch && git checkout FETCH_HEAD && node scripts/dev-modules.mjs --repos /mnt/d/projects
```

To test a module's branch from WSL, clone it the same way, and build a catalogue from the clones with `scripts/make-test-catalog.sh`.

---

<sub>WittyMediaServer documentation</sub>
