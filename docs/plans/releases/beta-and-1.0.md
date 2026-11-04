# The way to beta and 1.0: plan

> **Status on 2026-11-04**
>
> **Planned.** Nothing here has started, and the "Before beta" checklist isn't written yet: where it lives is the owner's choice.

**The project stays in alpha for a while.** The owner, 2026-11-02: "We're only going to beta once every feature I want is at a point where it's at least somewhat implemented." So this isn't a schedule. It's the bar the beta has to clear, and what "proper versioning" should mean once it does.

The owner also said, for 0.6.0-alpha: "starting with this new release, we'll allow limited forms of backwards compatibility in an unofficial capacity. Once we transition to beta waaay down the road, we'll allow proper versioning support for plugins."

---

## The beta gate

The beta starts when **every feature the owner wants is at least somewhat implemented**: it exists, it works in the common case, and it says plainly what it doesn't do yet. Polish, edge cases and real-world testing can carry on during the beta; a missing feature can't.

Keep the list in one place, so "are we there yet" has an answer:

- **A feature checklist** (suggested home: `docs/ROADMAP.md` gains a "Before beta" table, or a private file here if the owner prefers). Each row is a feature the owner wants, with one of: **not started**, **started**, **somewhat implemented**, **done**.
- The beta gate is met when no row is **not started** or **started**, and the owner agrees.
- The owner adds rows whenever they want something new. Adding a row moves the gate; that's expected.

**Rows already known from the roadmap and the owner's lists** (each is somewhere between not started and done today; check the state before relying on it):

| Area | Features |
|---|---|
| Server | Single instance; studios combining; clear Remote Access; poster picker; library order and right-click menus; customisation everywhere; watch together; recordings in a series' folder; local AI for photos; WittyRelay, the free hosted relay (greyed today); photo batch rotate and edit; sandbox on FreeBSD and macOS |
| Modules | Release channels (Module API 4); every module's plan in [../modules](../README.md#module-plans) to its "somewhat implemented" line |
| WittyTV | Live sync; full screen without retuning; guide focus; tuners: Windows BDA, DiSEqC, shared tuner for viewing and recording |
| WittyPlay | Everything on the server and streamed (in-browser and on-device emulation are off the plan); every series in [WittyPlay's timeline](../modules/WittyPlay.md) at least started: signed cores, insert animations, core store, disc sets, server-side filters, encoder host, transport and remote, seamless sessions and hand-off, resources, thin native clients, the dashboard, playing together, more consoles |
| Discs | Matching icons; Blu-ray secondary audio and picture in picture; CD+G and double-byte CD-TEXT |
| Photos | Map rebuilt |
| Apps | WittyMediaClient native parity on Android and Apple (music, books, disc menus, channels, live TV and guide, photos); Apple apps compiled and run; theme picker on native apps; other TVs (webOS, Tizen, Roku) at least started or decided against; WittyMediaPlayer on macOS and Linux; WittyVisualizer in native apps; WittyMediaRemoteMedia |
| Packaging | Combined Windows installer; apt and dnf hosted; Arch, Flatpak, Snap; code signing on Windows and macOS (a purchase: the owner's call) |
| Reliability | The audit in [../design/reliability-audit.md](../design/reliability-audit.md) closed, or each finding accepted in writing; 24-hour soak tests green on every server platform |
| Docs | In-app help complete enough that nobody needs GitHub; minimum hardware document |

---

## What stays true through the long alpha

- Modules work with 0.6.0-alpha and later, and nothing older.
- A later server *may* keep running a module if its range says it fits. Nothing promises it.
- Every module states a real `maxServer`.
- Breaking changes are allowed at any minor alpha, announced in the changelog.
- Release channels exist, but almost everything sits on **alpha**, as the owner asked. Beta and stable channels stay nearly empty until the project itself reaches beta.

---

## What beta adds, when it comes

### For modules: "proper versioning"

| Promise | How |
|---|---|
| **A stable module API within a major server version.** A module built for API N runs on every server of the same major that offers API N | `MinAPI` only rises at a major server version, announced one minor release ahead with a warning in the store |
| **A deprecation window.** A host call marked deprecated keeps working for at least two minor server releases | Deprecations listed in `wittymodule` and printed by `wittymodule check` |
| **Version ranges by API, not only by server.** A manifest can say `api: ">=4 <6"` | `wittymodule` version rules gain an API range; `maxServer` stays as a hard ceiling |
| **Data migrations both ways for one step**, so a module can be rolled back one version without losing data | `dataFormat` and `dataFrom` already exist; beta adds `dataTo` for the step back |
| **Signed, reproducible builds** for every first-party module | Two machines produce the same bytes, checked in CI |
| **The beta channel fills up**, and stable opens for modules that have earned it | See [../design/module-release-channels.md](../design/module-release-channels.md) |

### For apps

- `apiRevision` gets the same treatment: fields are only added within a major; removal needs a deprecation window.
- `docs/openapi.json` becomes the contract, with the docs contract tests required in CI (they skip without `WMS_TEST_CATALOG` today, so CI must set it).

### For the server

- **Database migrations that roll back one release.** A downgrade by one minor version never needs a restore.
- **Long-term data compatibility:** a backup made on beta restores on every later beta and on 1.0.
- **A security process with dates:** a private reporting path (already an owner step), a fix window, and advisories.

---

## Hardening, just before the beta

Only once the gate is met, not before. A few alpha releases spent finishing rather than adding:

| Area | Work |
|---|---|
| Security | An outside-eyes review of the sandbox on every system, the module signing chain, the store and remote access. The runner's brief capability inside its own user namespace (`stream/sandbox_linux.go`, `hostsandbox/apply_linux.go`) was never reviewed by a second reader |
| Real world | The roadmap's "hasn't met the real world" table: real Windows installs, a real Mac, FreeBSD hardware and arm64, real TVs, remotes, phones and pads, Safari and Firefox, screen readers with speech, real tuners, retail Blu-rays with Java menus, a real game PC, a camera on game delay, an unencrypted 3DS dump, real release workflows, the full Docker image |
| Performance | The optimise pass that the owner cut short in 0.6.0, run to the end, with the same measurements as the baseline (idle wakeups, scan SQL counts, route times) |

## 1.0.0

Much later again: a stable server channel; the minimum hardware document true on the hardware it names; every platform in the owner's list run on real hardware at least once, or plainly marked as not.

---

<sub>Future plans, written 2026-11-02. Not built, not tested.</sub>
