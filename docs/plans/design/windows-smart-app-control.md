# Design: running under Windows Smart App Control

> **Status on 2026-11-04**
>
> **Planned:** [#65](https://github.com/wittywilla/WittyMediaServer/issues/65), in Backlog. It waits for the owner's choice of certificate ([code signing](https://github.com/wittywilla/WittyMediaServer/discussions/38)).

For 0.6.3-alpha (the owner: "a major issue we need to address in the next incremental update... I don't want users to have this issue").

Not built. Not tested. Facts about Windows below come from Microsoft's published descriptions as remembered on 2026-11-02, and each one is marked **to verify** against Microsoft's current documentation before it's relied on.

---

## The problem

On the owner's Windows 11 PC, **Windows Security → App & browser control → Smart App Control** is **On**. WittyMediaServer is flagged, and the only way Windows offers to run it is turning Smart App Control off. The page itself warns: "If Smart App Control is off it can't be turned on without reinstalling Windows."

**The installer fails too** (the owner, 2026-11-02): with Smart App Control On, installing the server fails; with it Off, the server installs and works properly. That confirms the cause, and shows the block starts at the very first step: the `.msi`, and every program or DLL it runs while installing (WiX custom actions, the firewall extension's helpers, any setup program of ours it starts, such as `WittyMediaServer.exe` or `wms.exe` for first-run steps; **to verify** exactly which ran), are all checked before anything is on disk. A beginner sees an install that fails, not a message about signing.

So today, anyone on a fresh Windows 11 PC with Smart App Control on has two choices: don't use WittyMediaServer, or permanently lower their PC's protection. Neither is acceptable, and **telling people to turn it off is not the fix**.

### What Smart App Control does (to verify)

- It's on by default only on **clean installs** of Windows 11 (22H2 and later). It starts in **Evaluation** and then switches itself **On** or **Off**. The owner's machine is **On**.
- When On, Windows checks **every program and DLL as it's loaded** (and installers, and some kinds of script). It lets a file run if either:
  1. Microsoft's cloud reputation service says it's known to be safe, or
  2. it's **signed with a valid code-signing certificate** from a certificate authority Windows trusts.
- Anything else is **blocked**. There is **no per-app exception**: the person can't say "allow this one". That's why Windows only offered turning the whole feature off.
- Turning it off can't be undone without reinstalling Windows. (**To verify** whether newer Windows builds have relaxed this; even if so, people shouldn't have to.)
- It's the consumer form of Windows' application control (App Control for Business, formerly WDAC), built on Microsoft's example policy with the reputation option.

### Why we're blocked

Nothing we ship is code-signed. `docs/PUBLISHING.md` says so: "Signing the programs, so nothing warns people... Everything works without them." That was true before Smart App Control. On a PC where it's On, **nothing works without them**. A new, unsigned program also has no reputation, because nobody else has run it yet.

### Clues we already had

- **Go test binaries blocked on this machine.** The 0.6.0 tracker said "Windows Application Control started blocking Go test binaries". That was very likely Smart App Control blocking freshly built, unsigned test programs.
- **Unsigned helpers stalled on first launch.** A freshly built `wittyplay-host.exe` stalled 8 to 31 seconds on first launch, then started in 0.2 seconds. The memory note guessed a security scan. A reputation lookup is a likely candidate. Signing should remove both. **To verify** once signed builds exist.

---

## What Windows will check: every executable file we ship or load

Smart App Control looks at **every PE file** (`.exe`, `.dll`, `.node`, `.sys`) the moment it's loaded, plus installers (`.msi`) and scripts we run. A full list, so nothing is missed:

| Product | Files | Ours? | Signed today |
|---|---|---|---|
| **Server** (MSI) | `WittyMediaServer.exe` (tray and server), `wms.exe`, any setup or uninstall helpers, the `.msi` itself, WiX custom action DLLs (WiX's own are signed by its maintainers: **to verify**) | Ours | No |
| **Server scripts** | `install.ps1`, `uninstall.ps1` (in the zip), any PowerShell run by the installer or by `wms` | Ours | No |
| **FFmpeg** | `bin\ffmpeg.exe`, `bin\ffprobe.exe` (BtbN's builds, pinned by SHA-256 in `scripts/ffmpeg.lock`) | Third party, shipped by us | No (BtbN doesn't sign) |
| **Modules** (downloaded at run time as signed archives) | Each helper for Windows: WittyDVD, WittyBluray, WittyCD, WittyPhotos, WittyTuner, WittyIntegratedFileBrowser, WittyRigs, WittyCouch, WittyPlay's programs (`wittyplay-module`, `wittyplay-game-runner`, the host, `wittyplay-probe`, `wittyplay-check`), and later the encoder host | Ours | No. Our **own** release signature covers the archive, but Windows doesn't know about it |
| **WittyCouchHost** | Its program (GPL-3.0) | Ours | No |
| **Game cores** | Every libretro core DLL, downloaded from `buildbot.libretro.com/nightly/.../latest/` (`WittyPlay/cores/cores.go`) | Third party, downloaded by us | No. **Every core would be blocked, so no game could start** |
| **Custom cores** | Whatever an admin uploads | Third party | Usually not |
| **WittyMediaClient** (Windows) | The app `.exe`, Electron's DLLs, any native Node modules (`.node`), the installer, the uninstaller | Ours (Electron's files are rebranded into ours) | No |
| **WittyMediaPlayer** | The player `.exe`, its updater, Qt DLLs and plugins, MinGW runtime DLLs, FFmpeg DLLs, `wittydvdplayer` and `wittyblurayplayer` parts, the `.msi` | Mixed | No (Qt's own may be: **to verify**) |
| **WittyReader** (Windows) | As the client | Ours | No |
| **Things the person installs** | Java (for BD-J), Sunshine (for WittyRigs), the Moonlight app, VLC (for the player's option) | Not ours | Up to their makers. Java builds from the big vendors are signed (**to verify** each); Sunshine and some others may not be |

---

## The fix

### 1. Sign everything we ship, in CI, with a trusted certificate

**Every PE file, script and installer we ship** gets an Authenticode signature from a certificate in Microsoft's trusted root programme, with an RFC 3161 timestamp so it stays valid after the certificate expires. Full pipeline: [code-signing-pipeline.md](code-signing-pipeline.md).

- **Our own programs:** signed in each repository's release workflow, before they're packed into archives, MSIs or installers. Then the MSI or installer itself is signed.
- **Modules:** each module repository signs its Windows programs before `wittymodule pack`. The archive is then signed with our release key as now. The two signatures do different jobs: Windows trusts the Authenticode one, and the server trusts the release key.
- **Scripts we ship** (`.ps1`): signed too, or better, removed where the MSI does the job.
- **Third-party files we ship unchanged** (FFmpeg, MinGW and Qt DLLs that aren't signed): **signed by us** only when they're pinned by hash and built from a known source, because our signature says we vouch for that exact file. A file already signed by its maker is left as it is.

### 2. Game cores: build them ourselves, pinned and signed

Without this, WittyPlay can't start a single game on a Smart App Control PC.

Today each core is fetched from the libretro build server's **nightly "latest"** folder. That's unsigned, and it's also a moving target: the file can change from one day to the next. Pinning fixes a supply-chain weakness as well.

**Recommended:** a new repository, **WittyPlayCores**, that:

- builds each **offered** core from a **pinned upstream commit**, in CI, for every platform WittyPlay supports (Windows amd64 and arm64 first, then Linux, macOS and FreeBSD);
- signs the Windows DLLs (and, on macOS, signs them for the hardened runtime; see the pipeline doc);
- publishes them as release files with SHA-256 hashes, listed in a signed core catalogue that WittyPlay reads instead of the nightly folder;
- publishes each core's **exact source** with it (the upstream commit and our build scripts). Most cores are GPL, which requires this when we distribute binaries. The core's licence is shown in the core store, as now.

**Decision for the owner** (their call, not assumed): this means the project **distributes GPL core binaries itself** rather than pointing at libretro's server. That's allowed under the licence policy (third-party licences can be anything, shown clearly in the UI), and WittyPlay stays Apache-2.0 because cores run in the separate runner process. Non-commercial cores stay off the list. BIOS files are never shipped.

**Bonus:** reproducible, pinned builds also give WittyPlay's streaming work stable cores to measure against, and the core store a tested version per core.

**Custom cores** stay possible, but on a PC with Smart App Control On, an unsigned custom core **can't load**. The upload page checks the file's signature before accepting it and says so plainly (see section 4).

### 3. FFmpeg

Two steps:

1. **For 0.6.3:** sign the hash-pinned BtbN builds already listed in `scripts/ffmpeg.lock`, after checking the hash as the build already does.
2. **Later:** build FFmpeg for Windows ourselves, reproducibly, from a pinned source (`scripts/build-ffmpeg-static.sh` exists; extend it to Windows cross-builds), and sign that. This also lets us choose exactly which libraries are inside, for licence clarity.

### 4. When something is still blocked, say exactly what and why

Even with everything signed, people will meet blocks: an unsigned fork of a module, an unsigned custom core, Sunshine, an old cached copy, a signature that failed in a build. The product must never fail silently or say "it crashed".

**Know whether Smart App Control is On.** Read `HKLM\SYSTEM\CurrentControlSet\Control\CI\Policy\VerifiedAndReputablePolicyState` (0 Off, 1 On, 2 Evaluation; **to verify** the key and values). Reading it needs no special rights. Show it in **Settings → About → This computer** and in WittyDiagnoser's report.

**Check before installing.** On Windows, before a module version, a fork, a core or a custom core is installed, verify each PE file's Authenticode signature with `WinVerifyTrust` (available from Go through `golang.org/x/sys/windows`, no cgo).

- Signed by a trusted publisher: install.
- Not signed, and Smart App Control is On or in Evaluation: **refuse**, with "Windows' Smart App Control is on, and <file> isn't signed by a trusted publisher, so Windows would block it. <What to do>." For a fork: ask its author for a signed build. For a custom core: use a core from the core store.
- Not signed, and Smart App Control is Off: install, as now.

**Recognise a block at run time.** When starting a helper, runner or FFmpeg fails with `ERROR_SYSTEM_INTEGRITY_POLICY_VIOLATION` (4551, 0x11C7; **to verify** which codes `CreateProcess` and `LoadLibrary` return under Smart App Control), report it as a block, not a crash: name the file, say Smart App Control blocked it, and link to the help page. Where allowed, read the Code Integrity event log (`Microsoft-Windows-CodeIntegrity/Operational`, block events such as 3077; **to verify** the IDs) to name the exact file, including a DLL a program tried to load.

**Things we can't sign** (Sunshine, the Moonlight app, someone's Java): the page that needs them checks and explains, e.g. WittyRigs: "Sunshine on this PC isn't signed, and Smart App Control is on, so Windows blocks it. This isn't something WittyMediaServer can change."

**The help page** (in-app, **Help → Windows → Smart App Control**): what it is, that our programs are signed so it doesn't need turning off, how to check a file's signature (Properties → Digital Signatures), what each message means. It **never** recommends turning Smart App Control off as the fix, because that can't be undone. If someone chooses to anyway, that's their decision, and the page says what it costs them.

### 5. What we will never do

These would "work" and are off the table, as they are with encryption:

- Loading code in ways meant to avoid the check (mapping a DLL into memory by hand, running code from a data file, shelling out to a signed system program to run our code).
- Asking people to turn Smart App Control off as a step in setup or troubleshooting.
- Signing files we can't vouch for: anything not built by us or pinned by hash from a known source.
- Shipping our signing key anywhere outside the signing service.

---

## Testing under Smart App Control

GitHub's Windows runners are Windows Server, which doesn't have Smart App Control. So:

1. **A CI job that applies the same policy** (to verify it can be done this way): Windows ships Microsoft's example policies in `%windir%\schemas\CodeIntegrity\ExamplePolicies\`, including one for Smart App Control (`SmartAppControl.xml`). On a `windows-2025` runner, apply it in enforced mode with `CiTool --update-policy` (no reboot needed on recent Windows; **to verify**), then: install the MSI, start the server, start every module's helper, load every offered core in the runner for a few frames (`wittyplay-check -quick`), run FFmpeg, start the client, the player and the reader. Any block fails the job, naming the file. A second run with an **unsigned** test program must be blocked, which proves the policy is really on.
2. **A clean Windows 11 virtual machine is the real thing.** Smart App Control is only on after a clean install, and once turned off it can't come back without reinstalling. The owner's PC had it On, but turning it off was how the server was made to install, so it **can't be relied on as the test machine**. Instead: a Hyper-V virtual machine with a clean Windows 11 install (owner's help to set up, since it needs an admin prompt and a Windows licence or evaluation image), switched from Evaluation to **On**, then snapshotted. Each release candidate is downloaded from the GitHub release (not built locally), installed there, and the snapshot reverted afterwards. It's the only check against Microsoft's real reputation service. `needs-owner` issue with step-by-step instructions.
3. **Any other real PC** with Smart App Control On that the owner or a tester has, as a second check.
4. **A release gate** that walks every file of every Windows artefact (zip, MSI contents, module archives, cores, client and player installers) and fails if any PE file or script lacks a valid, timestamped signature: `signtool verify /pa /all` on Windows, or `osslsigncode verify` on Linux. See the pipeline doc.

---

## Development on the owner's machine

Never ask the owner to turn Smart App Control off. If it's already off on their PC (it was turned off to get the server installed), local builds will run there again, but nothing tested there says anything about Smart App Control. Either way:

- **Treat the owner's PC as a PC without Smart App Control.** Locally built programs won't run on any PC that has it On. Development tests keep going to WSL first (as already done). Windows tests run in CI or the virtual machine.
- **Test builds for the owner come from CI, signed**, not from `dist\test-all` on this machine. A "test build" workflow on any branch builds, signs and uploads the Windows package as a CI artefact. The owner downloads and runs that. (The signing step for test builds can use the same certificate. Each signing run is logged, and test builds are clearly versioned `-test`.)
- The rule about never rebuilding over `dist\test-all` still applies to anything left there.

---

## Rollout for 0.6.3-alpha

| Step | Who | Needs |
|---|---|---|
| 1. Choose and get a certificate ([options](code-signing-pipeline.md#getting-a-certificate)) | **Owner**: identity checks and payment are theirs | Days to weeks for identity validation: **start first** |
| 2. Signing pipeline in the server, then every module, client, player and reader repository | Claude | The certificate's CI access |
| 3. Sign FFmpeg (pinned) | Claude | Step 2 |
| 4. Detection, pre-install checks, run-time messages, help page | Claude | Nothing (can start at once) |
| 5. WittyPlayCores: pinned, signed core builds, and WittyPlay reading them | Claude | Owner's decision on distributing core binaries; step 2 |
| 6. CI job under the Smart App Control policy; signature gate on every release | Claude | Step 2 |
| 7. Release candidate in the clean Windows 11 virtual machine with Smart App Control On: install the MSI first, then everything else | **Owner** sets up the machine; Claude writes the steps | Steps 1 to 6 |

**If the certificate takes longer than 0.6.3:** ship steps 4 and 6 anyway, so a blocked PC at least gets a clear message and the problem is measured, and say in the release notes that signed builds follow in 0.6.4.

**If cores take longer:** ship the signed server, modules and apps; WittyPlay on a Smart App Control PC says that game cores aren't signed yet and when they will be.

## Done when

On a Windows 11 PC with Smart App Control **On**: the MSI installs without failing, then starting, scanning a library, playing a film (FFmpeg), opening a DVD and a Blu-ray menu, starting a game on every offered core, using the client, the player and the reader all work, **without any change to Windows settings and with no prompt from Smart App Control**. Proven in the clean virtual machine and in the CI policy job, with every Windows file passing the signature gate.

---

<sub>Future plans, written 2026-11-02. Not built, not tested.</sub>
