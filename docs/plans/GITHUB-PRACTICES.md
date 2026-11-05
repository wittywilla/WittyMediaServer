# GitHub practices for every future release

> **Status on 2026-11-04**
>
> **Done:** Issues and Discussions are on in every repository; features are planned ideas in Discussions; the server has the `0.7.0-alpha` and `Backlog` milestones and most of the labels below; private vulnerability reporting is on for the server.
>
> **In progress:** the server's issue forms (pull request [#75](https://github.com/wittywilla/WittyMediaServer/pull/75)); rulesets, a `wittywilla/.github` repository for every other repository's forms, and the project board are owner steps in [#114](https://github.com/wittywilla/WittyMediaServer/issues/114).
>
> **Not yet:** labels and milestones in the module and app repositories (the catch-up script waits for the owner's go-ahead), so their issues have no milestone.

The owner, 2026-11-02: "ensure our future plans also have github best practices. Opening issues, ensuring that it's done properly. Recently we have not been doing that. Ensure that is engrained in all future development."

During 0.6.0-alpha the work was tracked in local notes (`D:\alpha6-todo\TRACKER.md`, chat notes) and local branches, and went to GitHub only at publishing. That kept things moving, but left GitHub with no record of *why* anything changed, nothing for others to follow or report against, and no link from a release back to the work in it. From here on, **GitHub is where work is tracked**, and the local notes only hold what must stay private.

This applies to every repository: the server, every module, WittyModuleCatalog, WittyMediaClient, WittyMediaPlayer, WittyReader and any new one.

---

## Issues are for faults; features live in Discussions

The owner, 2026-11-02: "Add the plans I have as planned features with no direct media server version YET! Mark issues as genuine issues, but find a way to allow feature requests that don't bog down our issues." And: "Make sure discussions and issues are available on all repositories we've been working on, and the feature ideas and bugs are tied to their associated repositories."

Set up on 2026-11-02:

- **Issues and Discussions are on in all 23 Witty repositories.**
- **Issues are only for things that are broken**, labelled `bug`, in the repository whose code is at fault.
- **Planned features and feature requests are discussions in that repository's Ideas category.** The maintainer's plans carry the **planned** label and no version or milestone. Anyone can upvote them and say what they need. A planned feature becomes an issue (for a task) or a pull request when work on it starts, and only then gets a milestone.
- **WittyMediaServer's issue forms** (pull request #75 when written) allow no blank issues, offer a bug report form, and send ideas to Discussions, questions to Q&A and security problems to private reporting. **The other 22 repositories don't have forms yet:** the simplest way is a `wittywilla/.github` repository holding shared defaults (issue forms and `config.yml`), which GitHub applies to every repository without its own. Creating it is an owner step.
- The 2026-11-02 backlog: 43 planned features posted, 19 of them moved to their module's repository; 18 bugs filed (10 server, 7 WittyPlay, 1 WittyMediaPlayer); #19, #20 and #21, which were features, closed with links to their discussions.

So where this document says "open an issue" for a feature below, read "open or update its discussion in Ideas"; for faults and for tasks that work has started on, issues stay as described.

## The rules, in short

1. **No work without an issue.** Every bug, feature, task and release has an issue before the work starts, in the repository the code lives in.
2. **Every issue is in a milestone** named after the release it's planned for (`0.6.3-alpha`, `WittyPlay 0.4.0`), or in `Backlog`.
3. **One branch per issue, one pull request per branch**, and the pull request says `Closes wittywilla/<Repo>#<n>`.
4. **Nothing reaches `master` or `main` except through a pull request** with green CI.
5. **A release is a closed milestone.** Its release notes link the issues and pull requests in it.
6. **Security problems are never public issues.** They go through private vulnerability reporting.
7. **The standing rules still hold:** pushing, merging, tagging and publishing need the owner's go-ahead unless they've given it for that release. Opening and updating issues is part of doing the work.

---

## Issues

### When

- **Before starting work**, open one. If the work came from a plan in this folder, link the plan's file and section.
- **When something is found during other work** and isn't fixed there, open one, instead of only noting it in a chat's notes. The 0.6.0 notes' "left open", "not checked" and "sent back for" lists are exactly what issues are for.
- **When a test is flaky**, open one with the failing output. (For example `TestRealSubtitles/embedding_files`, which the tracker said not to dismiss.)
- **When something can't be tried here** (a real TV, a Mac, a real tuner), open one labelled `needs-owner` or `not-run`, so it's visible until someone does it.

### What a good issue has

Each repository gets templates in `.github/ISSUE_TEMPLATE/`:

| Template | Sections |
|---|---|
| **Bug** | What happened; what should have happened; steps; version and platform; logs (sanitised with WittyLog's sanitise switch); screenshots |
| **Feature** | The need, in the person's words; what done looks like; what it must not do (the standing rules that apply: encryption, performance, licences); platforms; accessibility (keyboard, screen reader, remote, controllers) |
| **Task** | What and why; done when; links |
| **Release** | Checklist: issues in the milestone closed or moved; full Linux run with real modules (`WMS_TEST_CATALOG`, SKIP lines counted); Windows `go test`; web build; design check; every platform built; changelog; version bumps; owner's go-ahead; tags; release workflows watched to the end |
| **Owner step** | What the owner needs to do, step by step (the owner is new to GitHub: give clicks and commands), and why it can't be done for them |

Plus a `config.yml` that points questions to Discussions and **security reports to private reporting**.

**Writing them:**

- Plain words, the way the docs are written.
- **No competitor product names**, as in code and docs.
- **Nothing from the private legal review.** An issue about a legal question says only what a reader needs, if anything.
- No personal data, real media file names from the owner's library, or addresses. `wittyplay-check` already avoids game names in its output for this reason.

### Labels

The same set in every repository, made once with a script (`gh label create`, run with the owner's go-ahead):

| Group | Labels |
|---|---|
| Kind | `bug`, `feature`, `task`, `docs`, `release`, `owner-step` |
| Area | `area:ui`, `area:api`, `area:modules`, `area:tv`, `area:games`, `area:discs`, `area:music`, `area:photos`, `area:books`, `area:security`, `area:performance`, `area:accessibility`, `area:packaging`, `area:ci` |
| Platform | `platform:windows`, `platform:linux`, `platform:macos`, `platform:freebsd`, `platform:android`, `platform:android-tv`, `platform:ios`, `platform:tvos`, `platform:bigscreen`, `platform:web` |
| State | `needs-owner`, `not-run` (written and tested as far as possible, not tried for real), `blocked`, `flaky`, `good first issue`, `help wanted` |
| Priority | `priority:high`, `priority:normal`, `priority:low` |

### Milestones

- Server: `0.6.3-alpha`, `0.7.0-alpha`, `0.8.0-alpha`, … and `Backlog`.
- Each module: its own versions (`0.4.0`), plus `Backlog`.
- Apps: their own versions.
- A milestone's description links its plan in this folder (`releases/0.6.3-alpha.md`) or, once the plan moves into a repository, there.

### A project board across repositories

One GitHub Project (owned by the `wittywilla` account) shows every open issue from every repository, with fields for **Release** (the server release it ships alongside), **Status** (Todo, In progress, In review, Done) and **Needs owner**. That replaces `TRACKER.md` as the place to see where a release stands. Creating it is an owner step.

---

## Branches and pull requests

- **Branch name:** `<issue number>-<short-name>`, for example `412-single-instance`. A chat's worktree branch follows the same rule.
- **Commits:** keep the project's style (a plain sentence saying what changed, from the reader's side), with `Refs #412` where useful.
- **Pull request** per issue, opened early as a draft so the work is visible. The template (`.github/pull_request_template.md`) asks for:
  - `Closes #412` (or `Refs` when it's partial)
  - What changed and why, in a few lines
  - **How it was tested**, with the commands and the result, and the SKIP count for tests that skip without real modules
  - **What wasn't tried** (platforms, devices, real hardware), plainly, which then gets `not-run` issues
  - Screenshots for UI, light and dark
  - Version bumps and changelog line
  - The standing rules it touches (encryption boundary, licences, security)
- **Review:** a code review pass on every pull request before asking the owner (the `/code-review` skill, at a level that fits the change). The owner approves merges, unless they've said to keep merging for that release.
- **CI must be green.** No merging over red. A flaky test gets an issue, not a re-run until green.
- **Cross-repository changes:** one pull request per repository, each linking the others ("Needs wittywilla/WittyModule#57"), merged in the order the dependency needs. The module catalogue's change comes last.

## Protecting the default branches

Owner steps, most already listed after 0.6.0's security audit:

- A ruleset on `master` / `main` in every repository: pull request required, CI required, no force pushes, no deletion.
- A tag ruleset so release tags (`v*`) can't be moved or deleted (tags never move is already the rule).
- `WMS_SIGNING_KEY` in a `release` environment with a required reviewer, the repository copy deleted.
- Private vulnerability reporting on.
- Dependabot security updates on, and alerts looked at each release.
- Actions pinned to commit SHAs.

## Releases

1. The release issue's checklist is worked through.
2. Every issue in the milestone is closed, or moved to the next milestone with a comment saying why.
3. The changelog entry links the issues (`#412`) and is written for users.
4. The GitHub release's notes come from the changelog, with a "Not tried for real" section from the milestone's `not-run` issues.
5. The milestone is closed when the release is published and its workflows have finished green.

## Security

- `SECURITY.md` in every repository says how to report privately, and what's in scope (encryption refusal included: a way to make the server decrypt something is a security report).
- Fixes for private reports are made in a private fork (GitHub's security advisory workflow), released, then the advisory is published.
- Never discuss an unfixed vulnerability in a public issue, pull request, commit message or changelog.

---

## Catching up after 0.6.0-alpha

Since the last release wasn't tracked this way, the first job of 0.6.3-alpha is to put what's already known onto GitHub:

1. Labels, templates and milestones in every repository (owner's go-ahead for the script, since it changes all of them).
2. One issue for each open item in `D:\alpha6-todo\TRACKER.md` and the chats' notes that's still true: leftovers, "not checked", "sent back for", flaky tests, owner steps. Skip anything private (legal) or security-sensitive (that goes to private reporting).
3. One issue for each item in the [0.6.3-alpha plan](releases/0.6.3-alpha.md), in the `0.6.3-alpha` milestone.
4. Issues for the rest of the plans in this folder, in their milestones or `Backlog`, each linking its plan.
5. The project board.

After that, `TRACKER.md`-style files are only for what can't be public.

## How the plans in this folder use this

Every release and module plan here ends with an **order of work**. Its first step is always: **open the milestone and an issue per item**, linking the plan. Its last step is always the release issue's checklist.

---

<sub>Future plans, written 2026-11-02.</sub>
