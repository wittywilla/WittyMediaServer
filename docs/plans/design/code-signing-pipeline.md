# Design: code signing for every repository

> **Status on 2026-11-04**
>
> **Planned:** [code signing](https://github.com/wittywilla/WittyMediaServer/discussions/38) and [#65](https://github.com/wittywilla/WittyMediaServer/issues/65). No certificate yet, and no signing in CI.

The pipeline behind [windows-smart-app-control.md](windows-smart-app-control.md). It applies to the server, every module, WittyPlayCores (new), WittyMediaClient, WittyMediaPlayer and WittyReader. macOS is covered at the end, because it's the same job on the other system.

Not built. Not tested. Prices, programme names and eligibility rules are **from memory and to verify** before anything is bought: `docs/PUBLISHING.md` already says the same about its figures.

---

## Two signatures, two jobs

| Signature | Who trusts it | Signs | Key lives |
|---|---|---|---|
| **Our release key** (exists) | The WittyMediaServer server | Module archives, the catalogue, release checksum lists | `WMS_SIGNING_KEY`, moving to a `release` environment with a required reviewer (0.6.0 owner step) |
| **Authenticode** (new) | Windows: Smart App Control, SmartScreen, installers' publisher name, UAC prompts | Every Windows `.exe`, `.dll`, `.node`, `.msi`, installer and `.ps1` we ship | Inside the signing service's hardware. **Never** on a disk, a developer machine or WSL |

Both stay. Neither replaces the other.

---

## Getting a certificate

The owner's step, because it involves identity checks and payment. Start it before anything else: validation can take days to weeks.

| Option | What it is | Cost (to verify) | Good | Watch out |
|---|---|---|---|---|
| **Microsoft's managed signing service** (Azure Trusted Signing; Microsoft may have renamed it, **to verify**) | Microsoft holds the key and issues short-lived certificates; signing runs from GitHub Actions with a Microsoft action | About US$10 a month for the basic tier | Cheapest; made for CI; key never handled; Microsoft's own route for Smart App Control | **Eligibility:** individual developers were only accepted in some countries (US and Canada at last memory), organisations need a verifiable history. **The publisher name shown is the verified identity**, so an individual's certificate shows their legal name in every install prompt and file property |
| **Certum open source code signing** | A certificate for open-source developers, with the key in Certum's cloud signing service or on a card | Lower than other commercial certificates (to verify) | Made for individual open-source developers | Cloud signing from CI needs their service's tooling; a card can't be used from GitHub's runners |
| **A commercial OV certificate** (Sectigo, DigiCert, SSL.com and others) | A standard certificate; since 2023 the key must be on hardware, so for CI it's in the CA's cloud signing service | About US$200 to 400 a year, plus the cloud signing service | Works anywhere | Most expensive; still shows the verified name |
| **SignPath Foundation** | Free signing for accepted open-source projects; the publisher shown is the foundation | Free | No cost; no personal name shown | They review the project and set rules on what may be signed. **To verify** whether third-party files we ship (FFmpeg, game cores) are allowed; if not, those need another route |

**EV certificates** used to buy instant SmartScreen reputation. Microsoft changed that (to verify), and Smart App Control only needs a valid trusted signature, so EV isn't needed.

**Recommendation:** Microsoft's managed service if the owner is eligible, and is comfortable with their legal name as the publisher. Otherwise Certum's open-source certificate. Apply to SignPath Foundation in parallel as a free fallback.

**The publisher name** is a real decision for the owner: whatever identity the certificate is issued to appears in Windows' install prompts and every file's Properties. If the owner doesn't want their own name there, they need an organisation (a registered business name) or SignPath. Raise it before buying.

---

## The pipeline

### One reusable workflow, used by every repository

A reusable GitHub Actions workflow, `sign-windows.yml`. It lives in one place, so a change is made once: a `wittywilla/.github` repository, to add to `docs/REPOSITORIES.md` for the owner to create, or WittyModule if a new repository isn't wanted. Every repository's release workflow calls it.

```
build (unsigned)  ──►  sign-windows (environment: signing)  ──►  pack / MSI / installer  ──►  sign-windows (installer)  ──►  release key: checksums, catalogue  ──►  verify gate  ──►  publish
```

**`sign-windows.yml`:**

1. Downloads the unsigned files from the build job as an artefact.
2. **Finds every file to sign by content, not by name:** PE files by their `MZ`/`PE` header, scripts by extension, so a renamed DLL can't slip through.
3. **Skips** files already validly signed by their maker (checked, not assumed), and **refuses** any third-party file not on the pinned list (FFmpeg hashes in `scripts/ffmpeg.lock`, core hashes in WittyPlayCores' list, Qt and MinGW runtime DLLs by hash in the player's lock file).
4. Signs with SHA-256 and an **RFC 3161 timestamp**, so signatures outlive the certificate.
5. Logs every signed file and its hash to the run's summary, so there's a record of exactly what was signed and when.
6. Uploads the signed files as the next job's artefact.

**Access to the signing service:**

- **GitHub's OIDC federation** to the signing service where it supports it (Microsoft's does): no long-lived secret stored in GitHub at all.
- The job runs in a GitHub **environment named `signing`**, which only tags (`v*`) and the test-build workflow can use, with the owner as **required reviewer** for releases. (Same pattern as the release key's `release` environment.)
- Pull requests from forks can never reach it.

### What each repository signs

| Repository | Signed before packing | Signed after |
|---|---|---|
| WittyMediaServer | `WittyMediaServer.exe`, `wms.exe`, any setup helper, `ffmpeg.exe`, `ffprobe.exe` (pinned), `.ps1` scripts still shipped | The `.msi` (amd64 and arm64), any zip's contents (zips themselves can't carry Authenticode) |
| Every helper module (WittyDVD, WittyBluray, WittyCD, WittyPhotos, WittyTuner, WittyIntegratedFileBrowser, WittyRigs, WittyCouch, WittyPlay) | Each Windows program in `cmd/`, before `wittymodule pack` | The archive is signed by our release key, as now |
| WittyCouchHost | Its program | |
| WittyPlayCores (new) | Each core DLL, built from a pinned commit | Core list signed by our release key |
| WittyMediaClient | Through electron-builder's signing hook: the app `.exe`, Electron's DLLs, every `.node` native module, helper executables | The installer and uninstaller |
| WittyMediaPlayer | The player, its updater, every unsigned DLL from `windeployqt` and MinGW (pinned), FFmpeg DLLs, Qt plugins | The `.msi` |
| WittyReader | As the client | As the client |

Web modules (WittyEbook, WittyPDF, WittyComics, WittyInk, WittyStats, WittyVisualizer) ship no Windows programs and need nothing.

### The verify gate

`scripts/verify-signatures.sh`, run on Linux in every release workflow, after signing and before publishing:

- Unpacks every Windows artefact: zips, MSIs (with `msiextract` or similar), module archives, installers (where they can be unpacked), core archives.
- Finds every PE file by content and every `.ps1`.
- Checks each with `osslsigncode verify`: a valid chain to a trusted root, the **expected publisher** (so a file signed by someone else fails), a timestamp, SHA-256.
- **Fails the release, listing every file that isn't right.** No partial publish.

The same script runs on a downloaded release afterwards, as a check that what's on GitHub is what was signed.

### Reproducible builds and signatures

A signature changes a file's bytes. To keep builds reproducible, compare files **with the signature removed** (`osslsigncode remove-signature`), and publish the unsigned hashes alongside.

### Test builds

The owner runs test builds. Locally built, unsigned programs won't run on a PC with Smart App Control On, and the owner's PC shouldn't be relied on to have it Off. So a **"Windows test build" workflow** (run by hand on any branch) builds, signs through the same environment, versions the result `-test.N`, and uploads it as a CI artefact for the owner to download. It's never attached to a release. Every signing run is listed in the run summary, so test signatures are accounted for.

---

## Keeping it safe

- **The key never leaves the service.** Nobody downloads it, and there's no "local signing" path. This is the lesson from 0.6.0, when a test build signed a throwaway checksum list with the real release key on the owner's machine.
- **Only CI signs**, only from protected tags or the test-build workflow, only in the `signing` environment.
- **Only what we built or pinned gets signed.** The pinned lists are reviewed in pull requests like code.
- **If the certificate is ever misused or exposed:** revoke it with the issuer, get a new one, re-sign and re-release the current versions, and publish a security advisory. Timestamps keep older genuine signatures valid only if the revocation date is set correctly: agree that with the issuer. Write this down as a runbook in `docs/PUBLISHING.md` before the first signed release.
- **Renewal reminders** as a GitHub issue in the milestone of the release before expiry.

---

## macOS, the same job

Not required for the Smart App Control fix, but it's the matching problem on Macs. The roadmap lists it as "Not bought", and `docs/PUBLISHING.md` already describes it. Plan it with the same structure:

- Apple Developer Program (about US$99 a year, to verify), Developer ID Application and Installer certificates, a notary profile.
- A reusable `sign-macos.yml`: sign every Mach-O (programs, dylibs, module helpers, cores) with the hardened runtime, notarise, staple.
- **Game cores on macOS:** a hardened runtime refuses to load libraries signed by another team, so cores must be signed with our Team ID (WittyPlayCores does this) rather than disabling library validation.
- The same verify gate, with `codesign --verify --deep --strict` and `spctl` on a Mac runner.
- The owner has no Mac, so macOS signing is built and checked on GitHub's Mac runners only, and marked `not-run` on a real Mac.

## Linux and FreeBSD

No equivalent of Smart App Control. The GPG key for apt and dnf repositories (already planned) is the matching piece.

---

## Order of work

1. **Owner:** choose the option, decide the publisher name, start identity validation. Create the `signing` environment and add themself as reviewer. Create `wittywilla/.github` if wanted.
2. `sign-windows.yml` and `verify-signatures.sh`, tested with a throwaway self-made certificate in a fork-free test repository (never the real one) until the real certificate arrives.
3. The server's release workflow, then the test-build workflow.
4. Every helper module, WittyCouchHost, the client, the player, the reader.
5. WittyPlayCores, after the owner's decision.
6. The runbook in `docs/PUBLISHING.md`, and the price notes updated with what was actually paid.
7. Each step as an issue in its repository's milestone ([../GITHUB-PRACTICES.md](../GITHUB-PRACTICES.md)).

---

<sub>Future plans, written 2026-11-02. Not built, not tested.</sub>
