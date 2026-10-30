# DVD-Video features and disc quirks

This is a reference for people working on WittyDVD, the DVD engine in `modules/wittydvd`, and on
the DVD player in WittyMediaServer (`internal/dvdsession`, `web/src/player/DvdPlayer.tsx`). For how
to *use* discs, see [DVD.md](DVD.md).

Each entry says what the feature or quirk is, what a hardware player does with it, what WittyDVD
does (checked in the code, with the file named), and what changed in 0.4.0-alpha. "Test discs" are
the four PAL discs used for regression runs: *Shrek*, *Shrek 2*, *Shrek the Third* and *Shrek
Forever After*. Their counts come from scanning every program chain (PGC), cell and NAV pack on
them.

- [How this was checked](#how-this-was-checked)
- [Navigation and the virtual machine](#navigation-and-the-virtual-machine)
- [Cells, stills and VOBUs](#cells-stills-and-vobus)
- [Menus and highlights](#menus-and-highlights)
- [Sub-pictures, subtitles and audio](#sub-pictures-subtitles-and-audio)
- [Structure: angles, interleaving, timing, chapters](#structure-angles-interleaving-timing-chapters)
- [Damaged and protected discs](#damaged-and-protected-discs)
- [Regions and parental management](#regions-and-parental-management)
- [Late video: Shrek 2's Far Far Away Idol credits](#late-video-shrek-2s-far-far-away-idol-credits)
- [Not handled](#not-handled)
- [Sources](#sources)

## How this was checked

- **Field layouts** come from the public DVD-Video structure references (see [Sources](#sources)).
  Where those references stay silent, the entry says so and explains the choice made.
- **Unit tests** build IFO tables, NAV packs, sub-picture units and VM commands byte by byte
  (`*_test.go` in `modules/wittydvd`). The IFO, PCI, DSI and sub-picture parsers are fuzz tested
  (`go test -fuzz=FuzzParsers`). Seeds now include a VMG with a parental table, title sets with
  parental blocks, and sub-pictures with several control sequences and CHG_COLCON.
- **Regression runs**: `wittydvd info`, `walk` and `crawl` on the test discs give the same output
  before and after every change. `crawl` and `walk` now draw the disc's random numbers from a
  fixed seed, so two runs, or two builds, can be compared line by line.

## Navigation and the virtual machine

| Feature | What it is | Hardware player | WittyDVD |
|---|---|---|---|
| **Command types 0–6** | 8-byte commands: special (Goto, Break, NOP, SetTmpPML), link, jump/call, system set, GPRM set, and three combined set/compare/link forms | Runs pre-commands on entering a PGC, a cell's command when the cell ends, post-commands at the end, and a button's command when it is activated | All seven decoded, with the operand byte positions of each type (`vm.go`). Types 4–6 differ in what the compare gates: type 4 sets then compares and links; type 5 compares, then sets and links; type 6 compares, sets only if true, and always links. **Changed:** title, program and PGC numbers ignore their reserved top bit (some discs set it). New tests cover every type |
| **Arithmetic** | `+= -= *= /= %= rnd & \| ^ <->` on 16-bit GPRMs | Saturates at 0 and 65535 | Same. Division by zero gives 65535 and modulo by zero gives 0 (the references do not say). `rnd n` gives 1…n |
| **GPRM counter mode** | `SetGPRMMD`: a GPRM that counts up once per second (game timers, "press within 10 s") | Counts wall-clock time | `Registers.Tick` adds whole seconds. The session ticks on every viewer action, with the time that has passed, so counters are right whenever a command reads them |
| **SPRM side effects** | Commands write SPRMs 1–3 (audio, sub-picture, angle), 8 (highlighted button), 9/10 (timer), 11 (karaoke mix) and 13 (parental level). The player maintains 4–7 (title, VTS title, PGC, chapter) | Applies streams at once, highlights the button | SetSTN, SetHL_BTNN and link button numbers set the registers. The navigator updates SPRMs 4–7 on every title and cell change (`nav.go`), and the session picks the streams up from SPRM 1 and 2. SPRM 14 says "16:9 display, normal mode", which is what the browser shows |
| **Navigation timer** (SPRM 9/10) | `SetNVTMR n, pgc`: after *n* seconds, go to a PGC of the current title (timed menus inside titles, "auto-play after 30 s") | Counts down only during title playback | Same (`Nav.Tick`). The session tells the player the remaining time (`timerSec`), and the player sends a tick when it runs out |
| **CallSS / RSM** | CallSS calls a menu from a title and saves a resume point. RSM returns to it | RSM restores SPRMs 4–8 and the cell. With a resume cell given, that cell restarts from its beginning; without one, playback resumes at the exact point | Same (`saveResume`, `doResume`). The session remembers the exact position for the no-cell case. CallSS from a menu just jumps (there is no title to resume). RSM with no resume point is ignored in a menu button; elsewhere it stops with a reason. Tested |
| **LinkTailPGC, LinkPGN, LinkCN, LinkPTTN** | Links inside the current PGC or title | LinkTailPGC runs the post-commands. LinkNextPG past the last program also ends the PGC | Same. A program, cell or chapter that does not exist stops playback with a message that names it, instead of guessing. Tested |
| **Next / Prev / GoUp PGC links** | Each PGC names the next, previous and "up" PGC | Used by links and by the RETURN key | `LinkNextPGC`/`PrevPGC`/`GoUpPGC` follow them. RETURN follows GoUp, or resumes the title when there is none. A title PGC with no post-command link continues with its next PGC |
| **Pre, post and cell commands** | Run around a PGC and after each cell | Cell command after the cell's still time; PGC still after the last cell; then post-commands | Same order (`afterCell`). 70–80 PGCs per test disc have no cells at all: pure "dispatcher" PGCs that run their commands and move on |
| **Random and shuffle PGCs** | PGC playback mode (byte 0xA3): bit 7 random (0) or shuffle (1), bits 6–0 how many programs to play | Draws programs: random may repeat, shuffle plays each once. Then the post-commands | **New.** Implemented in `pickProgram`. A random title starts with a drawn program; a chosen chapter counts as the first. The PGC still follows whichever program ends the run. The session plays such PGCs cell by cell. None on the test discs; they use `g0 = rnd n` commands instead, which already worked |
| **Parental management jumps** | `SetTmpPML level, line`: ask to change the parental level, and go to a line if allowed. Discs also compare SPRM 13 | Asks for the PIN when parental control is on | SetTmpPML sets SPRM 13 and jumps (the parental level is a setting; there is no PIN). *Shrek 2* uses it (VMGM PGC 22) together with an `s13 >= 3` check (PGC 21). For parental *blocks*, see [Regions and parental management](#regions-and-parental-management) |

## Cells, stills and VOBUs

| Feature | What it is | Hardware player | WittyDVD |
|---|---|---|---|
| **Cell still time** | 1–254 seconds, or 255 = until the viewer acts, after a cell | Holds the last picture. Buttons stay active | `Step.StillSec`. The session holds the last frame (stills shorter than 1.5 s become a picture). 131–358 infinite cell stills per test disc; no finite ones |
| **PGC still time** | The same, after the last cell | Holds after the last cell's command | Added to the last cell when it has no command. Otherwise the navigator holds after the command (`Step.Hold`). For random PGCs, after the last program played |
| **Infinite still with no buttons** | A picture that waits for the viewer | The Play key ("still off") goes on | **New:** the player shows **Continue**. Clips already had **Skip** |
| **VOBU stills** | One picture (an I-frame and a sequence end code), then VOBUs with sound only. Used for slide shows, galleries, stills with narration | The decoder keeps the picture until new video comes | Clips are padded with the last frame to the cell's length (HoldLastFrame). 20–131 such cells per test disc, for example *Shrek the Third* VTS 29 (a picture gallery) and *Shrek* VTS 8. See [Late video](#late-video-shrek-2s-far-far-away-idol-credits) |
| **Zero-length cells** | Cells of 0 s or a single VOBU (dummy titles of 0.48 s are common: 9–10 per test disc) | Pass through | A cell shorter than 1.5 s is shown as a still picture for its still time (at least 0.3 s) |
| **Cells that cannot be played** | Sectors that run backwards, or start past the end of the VOB | Skip them | **New:** the navigator passes over them (`playable`) and still runs their cell commands. The VOB sizes come from the IFO; on the test discs they match the real file sizes exactly. A PGC made only of such cells that loops on itself stops after 1000 skips |

## Menus and highlights

| Feature | What it is | Hardware player | WittyDVD |
|---|---|---|---|
| **Menu language units** | Each title set's menus come in language units; SPRM 0 picks one | Uses the unit for the menu language, else a default | Unit for SPRM 0, else the first. **New:** when that unit lacks the requested menu (root, chapter…), a unit that has it is used and remembered, so the menu's own links stay inside that unit. The session asks the navigator which PGC a step plays (`Nav.StepPGC`) instead of guessing again. The test discs have one unit each |
| **Highlight start** (HLI_S_PTM) | Buttons appear part-way into a clip (after an intro animation) | Buttons inactive until then | `buttonsFromSec`. The player shows "Waiting for the menu…" |
| **Highlight end and selection end** (HLI_E_PTM, BTN_SL_E_PTM) | Buttons stop working before the clip ends; after selection end, the viewer is ignored | Highlight removed; input ignored | **New:** `buttonsUntilSec`, the earlier of the two. The player hides the buttons from then on; a looping clip brings them back. On the test discs both times fall on the cell's end (or are 0xFFFFFFFF, "never"), so nothing changes there |
| **Forced select** (FOSL_BTNN) | The button highlighted when the menu appears | Wins over SPRM 8 | Applied when the highlight is new (status 1). Used by the *Shrek 2* games |
| **Forced activate** (FOAC_BTNN) | The button the disc presses by itself | At selection end | Pressed at `actionAtSec`, in clips and on stills. The references do not say when; selection end is the only time that makes sense |
| **Auto-action buttons** | Selecting the button activates it | No Enter needed | Moving onto one presses it (remote keys and on-screen remote). Common in the *Shrek* games |
| **Button groups** | Up to three button layouts for 16:9, letterbox and pan-scan. Group type bits: 1 = 16:9/normal, 2 = letterbox, 4 = pan-scan (the test discs' menus confirm this) | Uses the group for its display mode | The browser always shows the whole 16:9 frame, so the session uses the normal group and the matching wide sub-picture stream. The letterbox group is used only for a viewer who asks for 4:3 (no client does). **Changed:** `wittydvd crawl` and `walk` now read the same group as the player. Tested |
| **Highlight status 2 and 3** | "Same highlight as the previous VOBU" | Keeps the buttons | The button table is read from every VOBU whose status is not 0. A looping cell may never repeat a status 1 VOBU |
| **Highlight colours** | Select and action colour/contrast per button | Draws the selected look | Pre-rendered per button (`menu.go`, `spu.go`) |

## Sub-pictures, subtitles and audio

| Feature | What it is | Hardware player | WittyDVD |
|---|---|---|---|
| **Several control sequences in one unit** | A sub-picture unit is a bitmap plus a chain of display control sequences, each applied at its own delay. They can change colours, area or pixel data, or start and stop display again (blinking) | Applies each at its time | **New:** `DecodeSPUFrames` returns one picture per state, each with its own start and stop, and the session makes one subtitle event of each. Before, everything folded into one picture (first start, first stop, last colours). A start before the display area is known shows from the sequence that completes it. Delays that go back in time are clamped. All 7,299 units of *Shrek 2*'s feature are single pictures, so normal subtitles are unchanged |
| **Changing display areas** | `SET_DAREA` or `SET_DSPXA` in a later sequence: the picture moves, or shows another part of the bitmap | Re-draws | Covered by the above. Each state has its own area and bitmap |
| **CHG_COLCON** | Colour and contrast changes for line ranges and columns of the picture (karaoke wipes, two-colour subtitles, menu artwork) | Applies them | **New:** parsed (`parseColCon`) and applied when a sub-picture is drawn with its own look. A button highlight still wins inside its button, as on players. The parameter area is size-checked and fuzzed |
| **Forced subtitles** | `FSTA_DSP` (forced start): shows even with subtitles off (signs, alien speech) | Shows them | `SPU.Forced`. The player shows forced events of the active stream when subtitles are off (`forcedStream`). Can be turned off in the viewing settings |
| **Audio stream mapping** | SPRM 1 is a *logical* stream; each PGC's audio control table maps it to a physical stream, or marks it absent | Plays the mapped stream | `titleAudio` maps through `AudioControl`, and streams are pinned by MPEG stream id. Streams that are absent from a title are hidden from the audio menu |
| **Sub-picture stream mapping** | Each PGC maps a logical sub-picture stream to a physical one per display mode (4:3, wide, letterbox, pan-scan) | Uses the one for its display mode | `subs.go` picks wide (16:9 display) or narrow. Menus inside titles use SPRM 2's stream |
| **Multi-channel audio** | AC-3/DTS 5.1, LPCM | Downmixes or passes through | Streams with more than 2 channels are encoded as 6 channels; others as stereo |
| **Karaoke** | Audio application mode 1 with guide melody and vocal channels; SPRM 11 (`SetAMXMD`) says which to mix in | Mixes the selected channels | **New:** the stream is offered with its vocals and as a sing-along of the front channels only. SPRM 11 is recorded and not applied. See [Not handled](#not-handled) |

## Structure: angles, interleaving, timing, chapters

| Feature | What it is | Hardware player | WittyDVD |
|---|---|---|---|
| **Angle blocks** | Cells in a block, one per camera angle (cell type 1–3, block type 1) | Plays the cell for SPRM 3 | `angleCell`. Titles are flattened per angle (`TitleAngle`), and the Angle picker restarts the stream at the same moment. *Shrek* has a 2-angle block in its feature |
| **Interleaved blocks and seamless branching** | Cells stored interleaved on disc, in ILVUs | Reads only the VOBUs of the playing cell, jumping ILVU to ILVU | `CellRanges` follows the VOB and cell ids in each VOBU's DSI, so another angle or branch never plays in between. The DSI's seamless (ILVU and PREU) flags and angle jump tables are documented in the references; walking the VOBUs gives the same result without them |
| **STC discontinuities** | A cell's timestamps start over (444 cells on *Shrek*) | Resets its clock | Every cell is its own entry in the converter's concat list, so timestamps are rebased per cell |
| **Chapter table vs PGCs** | A title's chapter table (PTT) names PGC/program pairs, which need not match how the PGCs link | Chapter search uses the table | Chapters come from the table. A title PGC without a next link continues with the next PGC the table names (`nextTitlePGC`). **New:** a table entry naming a PGC that does not exist starts the title at its entry PGC. A program that does not exist starts at the PGC's first cell |
| **Title types** | TT_SRPT: one-sequential-PGC titles or not, and which commands are present | Restricts some operations | Parsed (`PlayType`); used only for diagnostics |

## Damaged and protected discs

| Quirk | What it is | Hardware player | WittyDVD |
|---|---|---|---|
| **CSS scrambling** | Encrypted sectors (PES scrambling control bits set) | Decrypts with licensed keys | **Refused.** Scrambled packs are detected in menu scans and subtitle reads (`PackScrambled`), and the disc stops with a message. WittyMediaServer never decrypts anything, and never will |
| **Broken IFO, intact backup** | VIDEO_TS.IFO or VTS_xx_0.IFO is unreadable, zeroed or truncated; the .BUP copy is fine | Reads the backup | **Changed:** before, the .BUP was used only when the IFO file was missing. Now it is also used when the IFO does not parse (`readIFO`). Tested with a zeroed title set IFO and a truncated VMG |
| **Junk titles and PGCs** (ARccOS- and RipGuard-style structural protection) | Decoy titles that replay the feature's cells in scrambled order or many times over; cells pointing outside the VOB; unreadable sectors; PGCs that link in circles | Follows only what the menus do, so it never meets the junk | Nothing is decrypted or bypassed; these are structural defences, and WittyDVD simply plays safely through them. **New:** unplayable cells are skipped, and **Play movie** chooses the title with the most *distinct* video, preferring titles that play their cells in disc order over decoys of the same length. Already in place: hop and step limits on the navigator and VM (`maxHops`, `maxVMSteps`), so circular links stop with a message, and the feature guard bypass (see DVD.md). Tested with a synthetic decoy title and a junk-cell loop |
| **Bad sectors in images** | Zero-filled where the disc could not be read | Skips | **New:** a cell that does not start with a NAV pack plays without buttons instead of stopping. The interleave scan stops at the first sector that is not a NAV pack and uses what it has; subtitle reads skip what they cannot parse |

## Regions and parental management

| Feature | What it is | Hardware player | WittyDVD |
|---|---|---|---|
| **Region codes** | VMG region mask (bit set = region not allowed); SPRM 20 is the player's region as a bitmask | Refuses discs for other regions | SPRM 20 is set to the disc's lowest allowed region (or a configured one), so discs play everywhere. Region-coding-enhanced discs, which check that SPRM 20 holds exactly one region, pass. Tested |
| **Multi-region discs** | Several regions allowed; some discs branch on SPRM 20 (trailers per region) | Its own region | The lowest allowed region |
| **Parental blocks** | Alternative versions of a scene as consecutive title PGCs (PGC category block type 1, modes first/middle/last). Each has a parental ID mask; the VMG's parental table (PTL_MAIT) gives, per country and level 1–8, the mask of each title set | Plays the PGC whose mask matches the table for its level and country | **New:** the table is parsed (`parsePTLMAIT`) and the navigator picks the matching PGC when it enters a block (`parentalPGC`). With parental control off (SPRM 13 = 15) it acts at level 8, which permits everything (the references do not define this case). A country the table does not list uses its first entry. Tested |

## Late video: Shrek 2's Far Far Away Idol credits

**What the disc does.** The credits of the *Far Far Away Idol* short (*Shrek 2*, VTS 11 PGC 13,
cell 1, 173.3 s) are 31 different credit cards, encoded as 31 consecutive frames of a normal
I/P/B group of pictures at 25 frames a second. That is 1.24 s of video in the first two VOBUs.
The remaining 172 s are VOBUs with sound only: the DSI's first-reference address is 0, and the
VOBU search information says no later VOBU has video. There is no sequence end code, no still
time, no VOBU still flag (VOBU_SE_E_PTM is 0 throughout), and the one sub-picture unit only
clears the screen.

**What a hardware player does.** It shows each frame at its presentation time, so the credits fly
by in 1.24 s. Then the decoder keeps showing the last picture (the copyright card) while the music
plays. Nothing in the DVD-Video structure can hold each card longer: still times apply to whole
cells, and still pictures need a sequence end code, which these frames do not have. A fan wiki
describes the short's credits as running at full speed, like TV credits.

**What WittyDVD does.** The same thing. The cell plays as a clip padded to its length with the last
frame (`hls.Spec.HoldLastFrame`), and the clip always converts from its start, so no empty
segments appear. Nothing more is changed, because nothing more is supported by the disc or the
specification. Viewers who want to read the cards can pause.

**The general case.** Cells whose picture ends before their sound are common: 20–131 per test
disc, almost all of them still pictures (one I-frame and a sequence end code) followed by sound.
NAV packs show them: PCI `VOBU_SE_E_PTM` marks the sequence end, DSI `VOBU_1STREF_EA = 0` marks
a VOBU without video, and the VOBU search information's "next VOBU with video" is 0xBFFFFFFF
when no more video follows. WittyDVD now parses these fields (`PCI.SeqEndPTM`,
`DSI.FirstRefEnd`, `DSI.NoVideoAhead`). Checked with the converter:

- inside a title, a gap between two cells' pictures is filled with the last picture by the
  converter's constant-frame-rate output (100 frames per 4-second segment across the gap);
- a gap at the end of a clip is filled by HoldLastFrame;
- a gap at the end of a whole title is left as it is, and the browser holds the last frame once
  the stream has ended.

A frame-rate filter was tried and made no difference, so it was not added.

## Not handled

- **Karaoke mixing by the disc.** A karaoke stream is offered twice in the audio picker, with its
  vocals and as a sing-along of the backing channels only. SPRM 11 is recorded, but the disc's own
  choice of what to mix is not applied. No karaoke disc has been available to test this on.
- **User-operation prohibitions in VOBUs** (PCI `VOBU_UOP_CTL`) are not enforced. Those of the PGC
  are: the player obeys them for seeking, skipping, pausing and stream changes, unless the viewer
  turns off **Disc restrictions**. **Skip** still works on warning screens that a player would not
  let you skip.
- **Seeking into a still-with-sound** inside a long title can start the picture a little late:
  after a seek into the gap, the stream's first picture is the next cell's.
- **Pan-scan and letterbox conversion.** The browser always shows the full 16:9 frame, so
  pan-scan vectors and the letterbox button group are not used. Screens today are wide, and the
  player's **Fill** setting zooms a wide picture on a narrow one.
- **Parental PIN.** Settings hold a parental level and a country, which pick the right version of a
  parental block. There is no PIN: `SetTmpPML` always succeeds. Use user accounts to restrict what
  people can open.

## Sources

- DVD-Video structure references by mpucoder, *DVD-Video Information*:
  [PCI](https://dvd.sourceforge.net/dvdinfo/pci_pkt.html),
  [DSI](https://dvd.sourceforge.net/dvdinfo/dsi_pkt.html),
  [PGC](https://dvd.sourceforge.net/dvdinfo/pgc.html),
  [SPRMs](https://dvd.sourceforge.net/dvdinfo/sprm.html),
  [VMG IFO](https://dvd.sourceforge.net/dvdinfo/ifo_vmg.html),
  [VTS IFO](https://dvd.sourceforge.net/dvdinfo/ifo_vts.html),
  [VM instructions](https://dvd.sourceforge.net/dvdinfo/vmi.html),
  [sub-pictures](https://dvd.sourceforge.net/dvdinfo/spu.html).
- Still pictures as a sequence end code and decoders holding the last picture:
  [FFmpeg development list, 2006](https://ffmpeg.org/pipermail/ffmpeg-devel/2006-December/008283.html).
- VOBU presentation periods and slide shows on DVD-Video:
  [EP 1665268](https://data.epo.org/gpi/EP1665268A2).
- The *Far Far Away Idol* credits: [Shrek wiki](https://shrek.fandom.com/wiki/Far_Far_Away_Idol)
  (as quoted by search results; the page could not be fetched).

---
<sub>WittyMediaServer 0.4.0-alpha documentation</sub>