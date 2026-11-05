# Game plan: how the future work gets done

> **Status on 2026-11-04**
>
> **In use:** 0.7.0-alpha runs this way, as parallel chats with one issue each, a worktree and branch named after it, and a draft pull request. Owner steps: [#114](https://github.com/wittywilla/WittyMediaServer/issues/114) (protecting `master`, shared issue forms, the project board) and [#105](https://github.com/wittywilla/WittyMediaServer/issues/105) (two new repositories). The code-signing certificate hasn't been started ([#65](https://github.com/wittywilla/WittyMediaServer/issues/65)).

How to turn the plans in this folder into releases, based on what worked and what didn't in 0.6.0-alpha. Not a schedule: the owner decides what starts when.

---

## What 0.6.0-alpha taught us

| Worked | Didn't |
|---|---|
| Many parallel chats, each with one clear brief, its own worktree and branch | Tracking only in local notes: GitHub had no record (now fixed by [GITHUB-PRACTICES.md](GITHUB-PRACTICES.md)) |
| A coordinator merging into one integration branch | The final accessibility pass skipped to make the publish date |
| Linux first for tests (WSL), real modules in the test catalogue | Tests that skip silently without `WMS_TEST_CATALOG` looked green |
| Writing down "not tried for real" plainly | Workers touching shared state: a reflog wiped, a `pkill` that could hit others, a test build that opened the owner's real data folder |
| Owner steps collected in one list | A release build that only fails on GitHub's runners, found at publish |

## The shape of every release

1. **Plan → issues.** Open the milestone in each repository the release touches, an issue per item linking its plan here, and a release issue with the checklist. Put the cross-repository picture on the project board.
2. **Split into briefs.** One brief per piece that can run on its own (different files, no shared result). Each brief: the issues it closes, the files and area, what done looks like, what not to touch, the standing rules. Pieces that touch the same files go one after another.
3. **Run them** as separate chats, each in its own worktree and branch named after its issue, each opening a draft pull request early.
4. **Review** each pull request (`/code-review`), then merge into the release branch. CI green, SKIP lines counted.
5. **Verify together:** full Linux run with real modules, Windows `go test`, every platform build, the release workflows dry-run (to catch runner-only failures before publish day), accessibility and controller pass on every changed screen.
6. **Owner checks:** the `needs-owner` issues (real devices, sound, a Mac, admin prompts).
7. **Publish** with the owner's go-ahead, watching every workflow to the end, fixing publish failures without asking (the owner's standing word), and closing the milestone.

## Suggested sequence

```
0.6.3-alpha ─ owner starts the certificate ─► Smart App Control: detection, CI policy job, verify gate ─► signing in every repository ─► WittyPlayCores
            ─ GitHub catch-up ─► single instance ─► accessibility pass ─► ~15 UI fixes ─► leftovers ─► publish
                                                                      └► WittyMediaPlayer release build fix
                                                                      └► WittyTV guide focus

0.7.0-alpha ─ Module API 4 + channels (WittyModule ► catalogue ► server ► every module re-tagged alpha)
              ├► studios        ├► Remote Access page     ├► poster picker (web ► client)
              ├► WittyTV live sync (server)
              ├► WittyDVD 0.4.0 + WittyBluray 0.4.0 (icons, shared insert engine)
              ├► WittyPlay 0.4.x (sound ► smoothness ► animations ► core store ► disc sets ► server-side filters)
              └► WittyPhotos 0.4.0 (map: investigate ► rebuild)

0.8.0-alpha ─ navigation and customisation ─► context menus ─► deep customisation
              ├► watch together (on WittyTV's clock)
              └► native client parity (live TV, music, books, photos)

WittyPlay, its own long train (everything on the server, nothing emulated on clients; modules/WittyPlay.md):
  0.3.5–0.3.9 signed cores, faults, measure ─► 0.4.x play experience ─► 0.5.x encoder host ─► 0.6.x transport and remote
  ─► 0.7.x seamless sessions ─► 0.8.x resources ─► 0.9.x thin native clients ─► 0.12.x more consoles ─► 1.0.0 proven
  0.10.x dashboard and 0.11.x together can run beside the engine work, as they touch different code
  (0.3.5 starts with server 0.6.3's signing work; it's the longest piece of work here)

… more alphas, until every feature the owner wants is at least somewhat implemented ─► beta
```

**What can run side by side** in 0.7.0: studios, Remote Access, the poster picker, WittyTV sync and each module major touch different files. Module API 4 must land first because the module majors need it to publish on the alpha channel.

## Where the owner is needed

Collected so each can be one `owner-step` or `needs-owner` issue, with step-by-step instructions:

- **First of all: a code-signing certificate** (choice, publisher name, identity validation, payment) and the `signing` environment, for the Smart App Control fix. Then a clean Windows 11 virtual machine with Smart App Control On, for testing each release.

- Approving the GitHub catch-up script (labels, templates, milestones across every repository) and creating the project board.
- Rulesets, the `release` environment for the signing key, private vulnerability reporting, Dependabot (from 0.6.0's security notes).
- Clicking through the single-instance question on Windows and macOS.
- Real devices: the APK on a phone and a TV stick; a TV in game mode; controllers.
- Listening: radio, music, game sound, screen readers.
- A Mac, when one's available (Apple apps, macOS builds, Safari).
- A gaming PC for game PC streaming; a real tuner; a retail BD-J disc; an unencrypted 3DS dump.
- Filming with a 240 fps camera for game latency.
- Answers to the questions in the [streaming design](design/wittyplay-streaming-performance.md#19-questions-for-the-owner), the [studios line](releases/0.7.0-alpha.md#studios-that-are-one-studio), the [single instance port choice](design/single-instance.md#a-decision-for-the-owner) and the [icon set](design/module-icons.md#a-set-if-the-owner-wants-it).
- Legal review questions (kept private): console drawings in insert animations, disc icons.

## Rules for every chat

The standing rules in [ROADMAP.md](ROADMAP.md#standing-rules-every-plan-here-keeps), plus the worker rules from 0.6.0 that prevented or would have prevented accidents:

- Never touch another chat's worktree, processes or test servers. No `pkill` by name, no `git reflog expire`, no `git gc --prune=now` on the shared repository.
- Never run a build from `dist\test-all` or anything that opens `%LOCALAPPDATA%\WittyMediaServer`.
- Test servers on `<chat>.localhost`, with their own data folder and `saveMetadata=0` against real media.
- Delete worktree junctions before deleting a worktree.
- Report "not tried" plainly; turn each into an issue.

---

<sub>Future plans, written 2026-11-02.</sub>
