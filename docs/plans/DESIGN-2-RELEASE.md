# Design 2: the release cut

Written 2026-11-12 by the design lead on the owner's instruction: the priority is to get themes, UI and SDUI
implemented, then work toward a release with working client apps. Be as feature complete as the end goal allows, but
do not work forever: the first release is a starting point. Everything past the cut becomes a GitHub issue and is
worked from there.

## The freeze (in force now)

- No new catalogue revision. Revision 8 is final for this release. Fixes to it are allowed; new components, fields and
  commands are not.
- No new canvas screens, looks or themes. The 17 look boards, 15 themes and the 128 gap screens are the design.
- No new modules, routes or features. Only: finishing what is in flight, fixing bugs, and verifying.
- Every agent merges what works into `v0.8.0-alpha` under the lock, then stops adding. Unfinished work that is not
  needed for the gates below is written up as an issue candidate (see "Issue candidates") instead of continued.

## The release gates (must work, seen running)

0. **Parity first.** The release has the same features as the previous release (server v0.7.2-alpha; each client's last
   tag) and then some, under the new design. Auditors (W21 web and server, W22 Apple, W23 Android) write
   `DESIGN-2-PARITY-*.md`; every missing or regressed core feature is a gate bug. Core means: playing, libraries, search,
   profiles, settings, admin, WittyTV, music, readers, photos, games, downloads, sharing, remote access, backup.
0b. **Peer to peer is vital.** Connect from anywhere (invite by link, code or QR; From away; remote access) works end to
   end on the web, iOS, Apple TV, Android and Android TV, with the native library built and bundled by default.
0c. **The apps look and work as intended**, proven by simulator or emulator screenshots against the design pictures in
   `docs/design-2/images`, not only by builds and unit tests.

1. **Server:** first-run gate (`setup_required`) and the 14-step browser setup; 14 experiences with per-device choice and
   admin policy; 15 themes with import; server name colours and mark; catalogue revision 8 and the server pages;
   conformance fixtures pass; `go test ./...` passes.
2. **Web app:** every look wears every theme; setup wizard; Settings Appearance with layout previews; layout and name
   editors; rail and drawer; player with WittyRemote and the control bar; WittyTV guide and More info; music with
   visualizers and karaoke; module pages; keyboard and controller reach.
3. **Apple (iPhone, iPad, Apple TV):** builds, runs, connects (including peer to peer), first-connect chooser with layout
   previews, Settings Appearance, the SDUI renderer at revision 8 (focus-grow, regions, layers, forms, QR),
   WittyTV More info on tvOS, play a video.
4. **Android (phone, Android TV):** the same list.
5. **Standalone apps (WittyReader, WittyMediaPlayer):** use SDUI and wear the themes where already built; the rest are
   issues. Not a blocker for the server and client release.
6. **Docs and housekeeping:** CHANGELOG, FIRST-TIME-SETUP, THEMES, SDUI docs current; licence headers; no secrets; the
   design README and canvas in `docs/design-2/`.

## Known gaps accepted for the first release (issue candidates)

Each becomes a GitHub issue after the owner says to file them:

- Admin pages: bulk edit, edit match, fix matches and library management as SDUI forms (the web has its own pages).
- Smart playlist builder live match count; download chooser (needs a server API); shorts and concerts pages.
- Language (no interface translation); Visualizer Speed/Colour measured visually; karaoke Mix page as rows.
- Standalone apps: remote provider polish, WittyMediaPlayer pieces not done by the cut, installers.
- Apple: Mac frame and visionOS ornament if not finished; Android: headset frames.
- Every item in `DESIGN-2-QA-*.md` marked open and not a gate.

## Phases

- **A. Finish and freeze (now):** merge what is in flight; fix only gate bugs; each agent posts a final report with its
  "Known issues / not verified" list written as issue candidates.
- **B. Release candidate:** build the server, web, iOS, tvOS, Android and Android TV from the `v0.8.0-alpha` heads;
  run the end-to-end path (fresh server, first-run in a browser, connect from each app, choose look and theme, play,
  music, WittyTV, peer to peer); fix what breaks the path; nothing else.
- **C. Issues:** the owner approves; the open items are filed on GitHub and worked from there.

Nothing is pushed by agents. The owner pushes.

## Release checklist

Written by W7 (2026-11-12). Nothing here is pushed or tagged by an agent: the owner does it. A bump is not a release.

**1. The server's version (0.8.0-alpha is already in the tree).** `scripts/bump-server.sh <version> ["note"]` raises it
everywhere it is written and lists the modules whose `maxServer` no longer covers it. Written in:

| File | What |
|---|---|
| `cmd/wittymediaserver/main.go` | `var version = "0.8.0-alpha"`; release builds overwrite it with `-X main.version=<tag without v>` (`.github/workflows/build.yml`, from `GITHUB_REF_NAME`), so a tag build says the tag |
| `web/package.json` and `web/package-lock.json` (twice) | `"version"` |
| `README.md` | the version badge (`version-0.8.0--alpha`) and the footer line |
| `CHANGELOG.md` | rename `## Unreleased` content into the version's section, leave an empty `## Unreleased` above it |
| `internal/server/api_capabilities.go` | `apiRevision` (16): raise it only when a client can need something new; the comments name what each revision added |
| `packaging/linux/flatpak/*.metainfo.xml` | filled in at build time by `metainfo.sh <version>` (a `-` becomes `~`; a suffix is a development release): nothing to edit |

Derived from the tag and not edited: the `.msi`, `.pkg`, `.deb`, `.rpm`, AppImage, Flatpak, snap, FreeBSD package, Docker tags.

**2. The catalogue.** `sdui.CatalogRevision` in `pkg/sdui/catalog.go` is 8 and final for this release. Documents say
`catalog: 8` (`docs/SDUI.md` says the revision in Versioning and in the document table). If it ever changes: regenerate
the fixtures, schema and docs (`go test ./pkg/sdui/conformance -update`, `go test ./pkg/sdui -run
TestJSONSchemaAndDocsAreCurrent -update`), resync the module kit, and tell every client (the rule is in `docs/SDUI.md`,
Versioning).

**3. The module kit (WittyModule).** `kit/sync-sdui-core.sh <server checkout>` copies `pkg/sdui` and its `data/` and
`experiences/` tables into `sdui/internal/core`; then regenerate `catalog.json` (`json.Marshal(core.Catalog())`), copy it
and `docs/sdui-module.schema.json` into `sdui/` and `kit/sdui/`, set `sdui.CatalogRevision` in `sdui/check.go`, and update
the lists in `kit/sdui/wittymodule-sdui.ts`. Check: `WMS_SERVER=<server checkout> go test ./...`, `node --test
kit/sdui/tests/*.test.mjs`, `tsc --strict` on the kit file. WittyModule's own version is its `module.json`-less release
(a Go module: tag `v0.x.y`); every module's `module.json` carries its own `version`, `minServer` and `maxServer`
(`wittymodule bump` raises one and its changelog). Each module's `sdui.json` says `catalog: 8`.

**4. The clients keep their own versions.** Raise them only when they ship:

| Repo | Where |
|---|---|
| WittyMediaClient, Android | `android/app/build.gradle.kts`: `versionCode`, `versionName` |
| WittyMediaClient, Apple | `apple/project.yml` and the project's `MARKETING_VERSION` / `CURRENT_PROJECT_VERSION` (regenerate with XcodeGen) |
| WittyMediaClient, desktop | `desktop/package.json` |
| WittyReader, WittyMediaPlayer | `module.json` / `package.json` |

Each client's built-in catalogue revision and card-style table must be 8 (conformance proves it).

**5. Before tagging.**
- `gofmt -l .` empty, `go vet ./...`, `go test -p 2 ./...` (one session, heavy slot), web `tsc`, design checks, `npm run
  build`, `python3 scripts/theme-contrast.py`, `python3 scripts/license-headers.py --repo . --license AGPL-3.0-or-later`
  for each repo with its licence (server, modules, WittyModule, catalogue, website AGPL; the clients GPL).
- `go test ./internal/docscheck` (links, guides in the app, no one sent to GitHub for help), `go test ./internal/server -run
  TestDocs`, openapi and SECURITY-ROUTES current (the route tests say what a new route needs).
- `CHANGELOG.md`: the 0.8.0-alpha section has its Highlights and everything since 0.7.2-alpha; `docs/FIRST-TIME-SETUP.md`
  matches the wizard; `docs/plans/README.md` and the README link the design documents.
- No secrets (`git grep` for keys and tokens, `.env`, signing keys), nothing from `/Volumes/STUFF` in any repo, test, doc
  or picture.
- Build each artefact from the `v0.8.0-alpha` heads (server, web, iOS, tvOS, Android, Android TV), run the end-to-end
  path in "Phases, B", and note what was not run.

**6. Publish (the owner).** Tag `v0.8.0-alpha` on the server, let the build workflow make the assets and the signed
checksum list, publish the module catalogue (`scripts/publish-all.sh`), then file the issue candidates once the owner
says so. `scripts/release-tracking.sh` and `.github/workflows/release-tracking.yml` keep the milestone in step.
