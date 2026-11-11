# DVD disc quirks

This reference lists what the DVD-Video format can do and what odd discs do with it, and how WittyDVD handles each. It's for people working on the engine. To play discs, see [DVDs](DVD.md).

File names in the tables are in the WittyDVD project, unless they start with `internal/` or `web/`, which are in the server. The server's half is `internal/dvdsession` and `web/src/player/DvdPlayer.tsx`. "Test discs" are the discs used for regression runs. Their counts come from scanning every program chain (PGC), cell and NAV pack on them.

- [How this was checked](#how-this-was-checked)
- [Navigation and the virtual machine](#navigation-and-the-virtual-machine)
- [Cells, stills and VOBUs](#cells-stills-and-vobus)
- [Menus and highlights](#menus-and-highlights)
- [Sub-pictures, subtitles and audio](#sub-pictures-subtitles-and-audio)
- [Structure: angles, interleaving, timing, chapters](#structure-angles-interleaving-timing-chapters)
- [Damaged and protected discs](#damaged-and-protected-discs)
- [Regions and parental management](#regions-and-parental-management)
- [Late video: credits on a bonus short](#late-video-credits-on-a-bonus-short)
- [Not handled](#not-handled)
- [Sources](#sources)

## How this was checked

- **Field layouts** come from the public DVD-Video structure references: see [Sources](#sources). Where those references stay silent, the entry says so and explains the choice made.
- **Unit tests** build IFO tables, NAV packs, sub-picture units and VM commands byte by byte (`*_test.go`). The IFO, PCI, DSI and sub-picture parsers are fuzz tested with `go test -fuzz=FuzzParsers`. Seeds include a VMG with a parental table, title sets with parental blocks, and sub-pictures with several control sequences and CHG_COLCON.
- **Regression runs:** `wittydvd info`, `walk` and `crawl` on the test discs give the same output before and after a change. `crawl` and `walk` draw the disc's random numbers from a fixed seed, so two runs, or two builds, can be compared line by line.

## Navigation and the virtual machine

| Feature | What it is | Hardware player | WittyDVD |
|---|---|---|---|
| **Command types 0 to 6** | 8-byte commands: special (Goto, Break, NOP, SetTmpPML), link, jump and call, system set, GPRM set, and three combined set, compare and link forms | Runs pre-commands on entering a PGC, a cell's command when the cell ends, post-commands at the end, and a button's command when it's activated | All seven decoded, with the operand byte positions of each type (`vm.go`). Types 4 to 6 differ in what the compare gates: type 4 sets, then compares and links; type 5 compares, then sets and links; type 6 compares, sets only if true, and always links. Title, program and PGC numbers ignore their reserved top bit, which some discs set. Tests cover every type |
| **Arithmetic** | `+= -= *= /= %= rnd & \| ^ <->` on 16-bit GPRMs | Saturates at 0 and 65535 | The same. Division by zero gives 65535 and modulo by zero gives 0, which the references don't specify. `rnd n` gives 1 to n |
| **GPRM counter mode** | `SetGPRMMD`: a GPRM that counts up once per second, for game timers and "press within 10 s" | Counts wall-clock time | `Registers.Tick` adds whole seconds. The session ticks on every viewer action, with the time that has passed, so counters are right whenever a command reads them |
| **SPRM side effects** | Commands write SPRMs 1 to 3 (audio, sub-picture, angle), 8 (highlighted button), 9 and 10 (timer), 11 (karaoke mix) and 13 (parental level). The player maintains 4 to 7 (title, VTS title, PGC, chapter) | Applies streams at once, highlights the button | SetSTN, SetHL_BTNN and link button numbers set the registers. The navigator updates SPRMs 4 to 7 on every title and cell change (`nav.go`), and the session picks the streams up from SPRM 1 and 2. SPRM 14 says "16:9 display, normal mode", which is what the browser shows |
| **Navigation timer** (SPRM 9 and 10) | `SetNVTMR n, pgc`: after *n* seconds, go to a PGC of the current title, for timed menus inside titles and "auto-play after 30 s" | Counts down only during title playback | The same (`Nav.Tick`). The session tells the player the remaining time (`timerSec`), and the player sends a tick when it runs out |
| **CallSS and RSM** | CallSS calls a menu from a title and saves a resume point. RSM returns to it | RSM restores SPRMs 4 to 8 and the cell. With a resume cell given, that cell restarts from its beginning. Without one, playback resumes at the exact point | The same (`saveResume`, `doResume`). The session remembers the exact position for the no-cell case. CallSS from a menu just jumps, as there's no title to resume. RSM with no resume point is ignored in a menu button, and elsewhere stops with a reason. Tested |
| **LinkTailPGC, LinkPGN, LinkCN, LinkPTTN** | Links inside the current PGC or title | LinkTailPGC runs the post-commands. LinkNextPG past the last program also ends the PGC | The same. A program, cell or chapter that doesn't exist stops playback with a message that names it, instead of guessing. Tested |
| **Next, Prev and GoUp PGC links** | Each PGC names the next, previous and "up" PGC | Used by links and by the RETURN key | `LinkNextPGC`, `PrevPGC` and `GoUpPGC` follow them. RETURN follows GoUp, or resumes the title when there is none. A title PGC with no post-command link continues with its next PGC |
| **Pre, post and cell commands** | Run around a PGC and after each cell | Cell command after the cell's still time, PGC still after the last cell, then post-commands | The same order (`afterCell`). 70 to 80 PGCs per test disc have no cells at all: pure "dispatcher" PGCs that run their commands and move on |
| **Random and shuffle PGCs** | PGC playback mode (byte 0xA3): bit 7 random (0) or shuffle (1), bits 6 to 0 how many programs to play | Draws programs: random may repeat, shuffle plays each once. Then the post-commands | `pickProgram`. A random title starts with a drawn program, and a chosen chapter counts as the first. The PGC still follows whichever program ends the run. The session plays such PGCs cell by cell. None on the test discs, which use `g0 = rnd n` commands instead |
| **Parental management jumps** | `SetTmpPML level, line`: ask to change the parental level, and go to a line if allowed. Discs also compare SPRM 13 | Asks for the PIN when parental control is on | SetTmpPML sets SPRM 13 and jumps. The parental level is a setting, and there's no PIN. One test disc uses it (VMGM PGC 22) with an `s13 >= 3` check (PGC 21). For parental blocks, see [Regions and parental management](#regions-and-parental-management) |

## Cells, stills and VOBUs

| Feature | What it is | Hardware player | WittyDVD |
|---|---|---|---|
| **Cell still time** | 1 to 254 seconds, or 255 for until the viewer acts, after a cell | Holds the last picture. Buttons stay active | `Step.StillSec`. The session holds the last frame, and stills shorter than 1.5 s become a picture. 131 to 358 infinite cell stills per test disc, and no finite ones |
| **PGC still time** | The same, after the last cell | Holds after the last cell's command | Added to the last cell when it has no command. Otherwise the navigator holds after the command (`Step.Hold`). For random PGCs, after the last program played |
| **Infinite still with no buttons** | A picture that waits for the viewer | The Play key ("still off") goes on | The player shows **Continue**. Clips have **Skip** |
| **VOBU stills** | One picture (an I-frame and a sequence end code), then VOBUs with sound only. Used for slide shows, galleries and stills with narration | The decoder keeps the picture until new video comes | Clips are padded with the last frame to the cell's length (`HoldLastFrame`). 20 to 131 such cells per test disc, such as a picture gallery in VTS 29 of one disc. See [Late video](#late-video-credits-on-a-bonus-short) |
| **Zero-length cells** | Cells of 0 s or a single VOBU. Dummy titles of 0.48 s are common: 9 or 10 per test disc | Pass through | A cell shorter than 1.5 s is shown as a still picture for its still time, at least 0.3 s |
| **Cells that can't be played** | Sectors that run backwards, or start past the end of the VOB | Skip them | The navigator passes over them (`playable`), and still runs their cell commands. The VOB sizes come from the IFO, and on the test discs they match the real file sizes exactly. A PGC made only of such cells that loops on itself stops after 1000 skips |

## Menus and highlights

| Feature | What it is | Hardware player | WittyDVD |
|---|---|---|---|
| **Menu language units** | Each title set's menus come in language units, and SPRM 0 picks one | Uses the unit for the menu language, else a default | Unit for SPRM 0, else the first. When that unit lacks the requested menu (root, chapter and so on), a unit that has it is used and remembered, so the menu's own links stay inside that unit. The session asks the navigator which PGC a step plays (`Nav.StepPGC`). The test discs have one unit each |
| **Highlight start** (HLI_S_PTM) | Buttons appear part-way into a clip, after an intro animation | Buttons inactive until then | `buttonsFromSec`. The player shows "Waiting for the menu" |
| **Highlight end and selection end** (HLI_E_PTM, BTN_SL_E_PTM) | Buttons stop working before the clip ends. After selection end, the viewer is ignored | Highlight removed, input ignored | `buttonsUntilSec`, the earlier of the two. The player hides the buttons from then on, and a looping clip brings them back. On the test discs both times fall on the cell's end, or are 0xFFFFFFFF ("never") |
| **Forced select** (FOSL_BTNN) | The button highlighted when the menu appears | Wins over SPRM 8 | Applied when the highlight is new (status 1). Used by a test disc's games |
| **Forced activate** (FOAC_BTNN) | The button the disc presses by itself | At selection end | Pressed at `actionAtSec`, in clips and on stills. The references don't say when, and selection end is the only time that makes sense |
| **Auto-action buttons** | Selecting the button activates it | No Enter needed | Moving onto one presses it, with remote keys and the on-screen remote. Common in a test disc's games |
| **Button groups** | Up to three button layouts for 16:9, letterbox and pan-scan. Group type bits: 1 is 16:9 or normal, 2 is letterbox, 4 is pan-scan, as the test discs' menus confirm | Uses the group for its display mode | The browser always shows the whole 16:9 frame, so the session uses the normal group and the matching wide sub-picture stream. The letterbox group is used only for a viewer who asks for 4:3, and no client does. `wittydvd crawl` and `walk` read the same group as the player. Tested |
| **Highlight status 2 and 3** | "Same highlight as the previous VOBU" | Keeps the buttons | The button table is read from every VOBU whose status isn't 0. A looping cell may never repeat a status 1 VOBU |
| **Highlight colours** | Select and action colour and contrast per button | Draws the selected look | Pre-rendered per button (`menu.go`, `spu.go`) |

## Sub-pictures, subtitles and audio

| Feature | What it is | Hardware player | WittyDVD |
|---|---|---|---|
| **Several control sequences in one unit** | A sub-picture unit is a bitmap plus a chain of display control sequences, each applied at its own delay. They can change colours, area or pixel data, or start and stop display again, for blinking | Applies each at its time | `DecodeSPUFrames` returns one picture per state, each with its own start and stop, and the session makes one subtitle event of each. A start before the display area is known shows from the sequence that completes it. Delays that go back in time are clamped. All 7,299 units of one test disc's feature are single pictures |
| **Changing display areas** | `SET_DAREA` or `SET_DSPXA` in a later sequence: the picture moves, or shows another part of the bitmap | Re-draws | Covered by the above. Each state has its own area and bitmap |
| **CHG_COLCON** | Colour and contrast changes for line ranges and columns of the picture, for karaoke wipes, two-colour subtitles and menu artwork | Applies them | Parsed (`parseColCon`) and applied when a sub-picture is drawn with its own look. A button highlight still wins inside its button, as on players. The parameter area is size-checked and fuzzed |
| **Forced subtitles** | `FSTA_DSP` (forced start): shows even with subtitles off, for signs and alien speech | Shows them | `SPU.Forced`. The player shows forced events of the active stream when subtitles are off (`forcedStream`). Can be turned off in the viewing settings |
| **Audio stream mapping** | SPRM 1 is a logical stream. Each PGC's audio control table maps it to a physical stream, or marks it absent | Plays the mapped stream | `titleAudio` in `internal/dvdsession` maps through `AudioControl`, and streams are pinned by MPEG stream id. Streams absent from a title are hidden from the audio menu |
| **Sub-picture stream mapping** | Each PGC maps a logical sub-picture stream to a physical one per display mode: 4:3, wide, letterbox or pan-scan | Uses the one for its display mode | `subs.go` in `internal/dvdsession` picks wide (16:9 display) or narrow. Menus inside titles use SPRM 2's stream |
| **Multi-channel audio** | AC-3 and DTS 5.1, LPCM | Downmixes or passes through | Streams with more than 2 channels are encoded as 6 channels, and others as stereo |
| **Karaoke** | Audio application mode 1 with guide melody and vocal channels. SPRM 11 (`SetAMXMD`) says which to mix in | Mixes the selected channels | The stream is offered with its vocals, and as a sing-along of the front channels only. SPRM 11 is recorded and not applied. See [Not handled](#not-handled) |

## Structure: angles, interleaving, timing, chapters

| Feature | What it is | Hardware player | WittyDVD |
|---|---|---|---|
| **Angle blocks** | Cells in a block, one per camera angle (cell type 1 to 3, block type 1) | Plays the cell for SPRM 3 | `angleCell`. Titles are flattened per angle (`TitleAngle`), and the Angle picker restarts the stream at the same moment. One test disc has a 2-angle block in its feature |
| **Interleaved blocks and seamless branching** | Cells stored interleaved on disc, in ILVUs | Reads only the VOBUs of the playing cell, jumping ILVU to ILVU | `CellRanges` follows the VOB and cell ids in each VOBU's DSI, so another angle or branch never plays in between. The DSI's seamless (ILVU and PREU) flags and angle jump tables are documented in the references, and walking the VOBUs gives the same result without them |
| **STC discontinuities** | A cell's timestamps start over, as in 444 cells on one test disc | Resets its clock | Every cell is its own entry in the converter's concat list, so timestamps are rebased per cell |
| **Chapter table against PGCs** | A title's chapter table (PTT) names PGC and program pairs, which needn't match how the PGCs link | Chapter search uses the table | Chapters come from the table. A title PGC without a next link continues with the next PGC the table names (`nextTitlePGC`). A table entry naming a PGC that doesn't exist starts the title at its entry PGC. A program that doesn't exist starts at the PGC's first cell |
| **Title types** | TT_SRPT: one-sequential-PGC titles or not, and which commands are present | Restricts some operations | Parsed (`PlayType`), and used only for diagnostics |

## Damaged and protected discs

| Quirk | What it is | Hardware player | WittyDVD |
|---|---|---|---|
| **CSS scrambling** | Encrypted sectors, with PES scrambling control bits set | Decrypts with licensed keys | **Refused.** Scrambled packs are detected in menu scans and subtitle reads (`PackScrambled`), and the disc stops with a message. The server checks for itself, whatever a helper says. Nothing here decrypts anything |
| **Broken IFO, intact backup** | VIDEO_TS.IFO or VTS_xx_0.IFO is unreadable, zeroed or truncated, and the .BUP copy is fine | Reads the backup | The .BUP is used when the IFO file is missing or doesn't parse (`readIFO`). Tested with a zeroed title set IFO and a truncated VMG |
| **Junk titles and PGCs** | Damaged or decoy structures: titles that replay the feature's cells in scrambled order or many times over, cells pointing outside the VOB, unreadable sectors, PGCs that link in circles | Follows only what the menus do, so it never meets the junk | Nothing is decrypted or removed. WittyDVD follows the disc's navigation like a hardware player, and doesn't stop on what it can't play. Unplayable cells are skipped, and **Play movie** chooses the film by ranking every title (see [Finding the film among decoys](#finding-the-film-among-decoys)). Hop and step limits on the navigator and VM (`maxHops`, `maxVMSteps`) stop circular links with a message. Tested with synthetic decoy titles, a shuffled-cell decoy, cell spam and a junk-cell loop |
| **Bad sectors in images** | Zero-filled where the disc couldn't be read | Skips | A cell that doesn't start with a NAV pack plays without buttons instead of stopping. The interleave scan stops at the first sector that isn't a NAV pack and uses what it has. Subtitle reads skip what they can't parse |

### Finding the film among decoys

Discs that guard their film can carry dozens of titles of the same length. **Play movie** scores every title in minutes of distinct video, and adds or takes away points for what it finds. The diagnose report gives each title's score with the reasons for it.

- **Distinct video.** Each cell counts once, however often a title replays it, so padding with repeats gains nothing and loses points for the time it takes.
- **Shuffled and duplicate copies.** A title holding most of another's cells in a scrambled order is a decoy. A copy in the same order is the same film, so it only loses a tie.
- **Broken structure.** Cells outside the video files, cells that claim more than four hours, chains of programs that loop back on themselves, and chapter lists that point nowhere all cost points, and the worst make a title a decoy.
- **Cell spam.** A title made mostly of cells under a second long, or of empty ones, is a decoy.
- **The disc's own program.** WittyDVD dry-runs the disc's first-play program, as a player would when the disc goes in, and a title it reaches by itself gains points. Short titles on the way are taken as intros. Titles that a menu jumps to gain a little, and the title behind the first menu's default button gains most, in the diagnose report.
- **Damaged index files.** Each title set's index is kept twice on the disc. When the two differ, the copy with fewer problems is used, and the report says which.
- **Smaller signals.** Even chapter lengths, more soundtracks and subtitles, and the largest title set each add a little.

TV discs are found the same way: three or more clean titles of similar length become episodes, in the order a play-all title gives, if there is one.

None of this decrypts anything. It reads only the parts of a DVD that are never encrypted, and a CSS-scrambled disc is still refused, whatever its titles score.

The ranking reads structure, never pictures. A disc that only tells the film apart through its menu, or picks it from player settings, can still pick wrongly with **Play movie**. Choosing a title from the disc's menu, or from **Bonus Features**, avoids the guess.

## Regions and parental management

| Feature | What it is | Hardware player | WittyDVD |
|---|---|---|---|
| **Region codes** | VMG region mask, where a set bit means not allowed. SPRM 20 is the player's region as a bitmask | Refuses discs for other regions | SPRM 20 is set to the disc's lowest allowed region, or the one in **Settings → Playback & discs**, so discs play everywhere. Region-coding-enhanced discs, which check that SPRM 20 holds exactly one region, pass. Tested |
| **Multi-region discs** | Several regions allowed. Some discs branch on SPRM 20, for trailers per region | Its own region | The lowest allowed region |
| **Parental blocks** | Alternative versions of a scene as consecutive title PGCs (PGC category block type 1, modes first, middle and last). Each has a parental ID mask. The VMG's parental table (PTL_MAIT) gives, per country and level 1 to 8, the mask of each title set | Plays the PGC whose mask matches the table for its level and country | The table is parsed (`parsePTLMAIT`), and the navigator picks the matching PGC when it enters a block (`parentalPGC`). With parental control off (SPRM 13 is 15), it acts at level 8, which permits everything. The references don't define this case. A country the table doesn't list uses its first entry. Tested |

## Late video: credits on a bonus short

**What the disc does.** The credits of a bonus short on one test disc (VTS 11 PGC 13, cell 1, 173.3 s) are 31 different credit cards, encoded as 31 consecutive frames of a normal I, P and B group of pictures at 25 frames a second. That's 1.24 s of video in the first two VOBUs. The remaining 172 s are VOBUs with sound only: the DSI's first-reference address is 0, and the VOBU search information says no later VOBU has video. There's no sequence end code, no still time and no VOBU still flag (VOBU_SE_E_PTM is 0 throughout), and the one sub-picture unit only clears the screen.

**What a hardware player does.** It shows each frame at its presentation time, so the credits fly by in 1.24 s. Then the decoder keeps showing the last picture, the copyright card, while the music plays. Nothing in the DVD-Video structure can hold each card longer: still times apply to whole cells, and still pictures need a sequence end code, which these frames don't have.

**What WittyDVD does.** The same thing. The cell plays as a clip padded to its length with the last frame (`hls.Spec.HoldLastFrame`), and the clip always converts from its start, so no empty segments appear. Nothing more is changed, because nothing more is supported by the disc or the specification. Viewers who want to read the cards can pause.

**The general case.** Cells whose picture ends before their sound are common: 20 to 131 per test disc, almost all of them still pictures (one I-frame and a sequence end code) followed by sound. NAV packs show them: PCI `VOBU_SE_E_PTM` marks the sequence end, DSI `VOBU_1STREF_EA = 0` marks a VOBU without video, and the VOBU search information's "next VOBU with video" is 0xBFFFFFFF when no more video follows. WittyDVD parses these fields (`PCI.SeqEndPTM`, `DSI.FirstRefEnd`, `DSI.NoVideoAhead`). Checked with the converter:

- Inside a title, a gap between two cells' pictures is filled with the last picture by the converter's constant-frame-rate output: 100 frames per 4-second segment across the gap.
- A gap at the end of a clip is filled by `HoldLastFrame`.
- A gap at the end of a whole title is left as it is, and the browser holds the last frame once the stream has ended.

A frame-rate filter was tried and made no difference, so it wasn't added.

## Not handled

- **Karaoke mixing by the disc.** A karaoke stream is offered twice in the audio picker, with its vocals and as a sing-along of the backing channels only. SPRM 11 is recorded, but the disc's own choice of what to mix isn't applied. No karaoke disc has been available to test this on.
- **User-operation prohibitions in VOBUs** (PCI `VOBU_UOP_CTL`) aren't enforced. Those of the PGC are: the player obeys them for seeking, skipping, pausing and stream changes, unless the viewer turns off **Disc restrictions**. **Skip** still works on warning screens that a player wouldn't let you skip.
- **Seeking into a still with sound** inside a long title can start the picture a little late: after a seek into the gap, the stream's first picture is the next cell's.
- **Pan-scan and letterbox conversion.** The browser always shows the full 16:9 frame, so pan-scan vectors and the letterbox button group aren't used. The player's **Fill screen** setting zooms a wide picture on a narrow one.
- **Parental PIN.** Settings hold a parental level and a country, which pick the right version of a parental block. There's no PIN: `SetTmpPML` always succeeds. Use profiles to restrict what people can open.

## Sources

- DVD-Video structure references by mpucoder, *DVD-Video Information*: [PCI](https://dvd.sourceforge.net/dvdinfo/pci_pkt.html), [DSI](https://dvd.sourceforge.net/dvdinfo/dsi_pkt.html), [PGC](https://dvd.sourceforge.net/dvdinfo/pgc.html), [SPRMs](https://dvd.sourceforge.net/dvdinfo/sprm.html), [VMG IFO](https://dvd.sourceforge.net/dvdinfo/ifo_vmg.html), [VTS IFO](https://dvd.sourceforge.net/dvdinfo/ifo_vts.html), [VM instructions](https://dvd.sourceforge.net/dvdinfo/vmi.html), [sub-pictures](https://dvd.sourceforge.net/dvdinfo/spu.html).
- Still pictures as a sequence end code, and decoders holding the last picture: [FFmpeg development list, 2006](https://ffmpeg.org/pipermail/ffmpeg-devel/2006-December/008283.html).
- VOBU presentation periods and slide shows on DVD-Video: [EP 1665268](https://data.epo.org/gpi/EP1665268A2), as quoted by search results. The page itself couldn't be fetched.

---

<sub>WittyMediaServer documentation</sub>
