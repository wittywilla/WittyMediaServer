# Blu-ray features and disc quirks

What the Blu-ray format can do, what real discs do with it, and what WittyBluray does in each
case. The DVD counterpart is [DVD-QUIRKS.md](DVD-QUIRKS.md); for using discs, see
[BLURAY.md](BLURAY.md).

**How to read the Tested column**

| Mark | Meaning |
|---|---|
| Disc | checked on the test images (two custom-authored discs, neither retail) |
| Unit | checked by a Go test built from synthetic data in the real file format |
| Java | run end to end on real Java 21, in the browser player, with a test application |
| None | written from the format, with no test data available |

No retail BD-J disc, UHD disc or real MVC disc has been available for testing yet, so anything
marked Unit or Java may still meet surprises on commercial discs.

- [The disc's program](#the-discs-program)
- [Player registers](#player-registers)
- [Playlists](#playlists)
- [Clips and time](#clips-and-time)
- [Menus](#menus)
- [Subtitles](#subtitles)
- [Audio and secondary streams](#audio-and-secondary-streams)
- [3D, Ultra HD and HDR](#3d-ultra-hd-and-hdr)
- [BD-J](#bd-j)
- [File layouts and damage](#file-layouts-and-damage)
- [Copy protection](#copy-protection)
- [Not handled](#not-handled)
- [Testing a disc](#testing-a-disc)

## The disc's program

| Feature | What it is | What WittyBluray does | Tested |
|---|---|---|---|
| Index entries | First play, top menu and titles each name a movie object or BD-J object in 12 bytes | Parsed as such. Before this pass BD-J entries were read as 16 bytes, which scrambled every title after them | Unit |
| One level of suspension | CALL_OBJECT, CALL_TITLE and a menu call keep exactly one way back | One slot; a second call replaces it | Unit |
| RESUME | Goes back with the playback position | Restores title, chapter, playlist, item, time, button, page and style. General registers and stream choices stay as they are, so a setup menu's changes survive. Before this pass they were undone | Unit |
| Resume intention | A movie object says whether a menu call may come back to it | Honoured: without it, the object ends at the menu call | Unit |
| Menu call mask | A movie object or the playing item can forbid the Menu key | The key does nothing while forbidden | Unit |
| Jumps from buttons | A button can run JUMP_OBJECT or CALL_OBJECT | Followed. They used to be dropped silently | Unit |
| JUMP_TITLE | Forgets what was suspended | Done, so Resume cannot return to a film the disc left | Unit |
| Running off the end | A movie object that ends without jumping | Playback stops; nothing resumes by itself | Unit |
| Arithmetic | Saturates at the top and stops at zero | Done. Division or modulo by 0 gives all ones; huge shifts give 0 | Unit |
| Bit compare | True when the first operand has a bit the second lacks | Done | Unit |
| Bad register references | A reference with stray bits set is no register | Reads as 0, writes are ignored | Unit |
| Set options | Five bits wide, not four | Fixed: option 0x11 no longer looks like SET_STREAM | Unit |
| Button-only commands | LINK, TERMINATE_PL, POPUP_OFF, STILL and enable/disable only work in buttons | Ignored in movie objects | Unit |
| SET_BUTTON_PAGE | In a button it ends the button's commands; everywhere it writes the registers | Done | Unit |
| Navigation timer | Object first, then seconds; at most 300; 0 cancels | Operands were swapped and the timer never ran from movie objects. Both fixed | Unit |
| Run limit | A disc's program could loop for ever | Stops after 1,000,000 commands in one run, or 64 jumps without playing | Unit |

## Player registers

| Feature | What WittyBluray does | Tested |
|---|---|---|
| Default values | All registers start at the format's "nothing chosen" values (audio 0xff, subtitles 0x0fff0fff, button 0xffff and so on). A disc's program tells a first visit from a later one by these | Unit |
| Profile and version | PSR 31 uses the real encoding: 0x00030200 for profile 2, 0x00130240 for 3D, 0x00000300 for Ultra HD. It was wrong before, which can change what a disc offers | Unit |
| Capabilities | Audio, video and text capability registers say what a transcoding server can in fact deliver | Unit |
| Ultra HD discs | Index version 0300 or above gets an Ultra HD player identity, since such discs check | Unit |
| Menu state | The page and button on show are written to PSR 11 and 10. Commands often read them to learn what was pressed, and they were never set before | Disc |
| Stream selection | When an item starts, a stream number it lacks is replaced by the viewer's language, else the first | Unit |
| Chapter register | PSR 5 follows entry marks and counts only those | Unit |
| Region, parental level, country | Settings. Blu-ray regions are checked by the disc's own program, which then shows its own message | Unit |

## Playlists

| Feature | What it is | What WittyBluray does | Tested |
|---|---|---|---|
| Random and shuffle | A playlist can draw its items at random, a set number of times | Played item by item. Shuffle plays each once before any repeat | Unit |
| Still items | An item can hold its last picture for some seconds or until the viewer acts | Held. Menus built on a held picture no longer loop | Unit |
| Slideshows | Pictures one at a time, often with music from a sub-path | Item by item, with Next and Previous moving between pictures | Unit |
| Items with different streams | One playlist can change stream PIDs or codec between items | Played one item at a time, since one stream cannot carry both | Unit |
| Menu that moves on | When a menu's playlist ends, the program may start the film | The menu loops only if the program would replay it; otherwise the end is reported. Everything used to loop for ever | Disc, Unit |
| Stream table order | Picture-in-picture subtitles follow subtitles; secondary streams carry lists of what they go with | Parsed in that order, including Dolby Vision layers | Unit |
| User operation masks | A playlist or item can forbid seeking, skipping, pausing and stream changes | Honoured in the player, with a setting to ignore them. The bit names were wrong before | Unit |
| Mark types | Entry marks are chapters; link points are not | Only entry marks show as chapters; commands can reach both | Unit |
| Finding the feature | Copy protection schemes add decoy playlists | See below | Unit |

### Finding the feature among decoys

Some discs carry hundreds of playlists built to defeat "longest wins": clips repeated many times,
clips that are not on the disc, times past a clip's end, or the real film's clips in the wrong
order. WittyBluray counts each stretch of a clip once and rejects a playlist that names what cannot
be read. Among those within 10% of the longest, it prefers the one the disc's program names, then
the one whose clips meet in time (a shuffled copy has the same length, but its seams do not meet),
then one with chapters.

This uses structure only and does not compare pictures, so a disc whose real clips do not meet in
time and whose program hides the choice can still pick wrongly. Choosing a title from the disc's
own menu, or from the list of extras, avoids the guess.

## Clips and time

| Feature | What it is | What WittyBluray does | Tested |
|---|---|---|---|
| Several time bases | A clip's clock can restart mid-file, so one time can occur twice | Entry points are looked up within the item's own time base | Unit |
| Clips with no video | Audio-only clips map their audio instead | The clip's own map is used | Unit |
| No time-base table | A damaged clip could crash the older code | Handled | Unit |
| 4:3 on a Blu-ray | Standard-definition extras can be either shape | Shape read from the clip | None |
| Late audio or video | Separately muxed sub-paths have their own clock | Mapped through the sub play item's sync point | Unit |

## Menus

| Feature | What WittyBluray does | Tested |
|---|---|---|
| Time-outs | Selection and composition times were misread and unused. Now selection end presses the default button and composition end removes the buttons | Unit |
| Pop-up time-out | A pop-up closes when no key has come for the disc's time | None |
| Button groups | Enabling a button replaces the others in its group | Disc |
| Hidden neighbours | A disabled button cannot take the selection | Disc |
| Self-linked buttons | Discs link keys to the button itself to disable them (The Lorax uses Left and Right only). Followed as written | Disc |
| No default selection | Falls back on the disc's last selection, then the first button | Disc |
| Numeric keys | A button's own number, not its id | None |
| Button sounds | `sound.bdmv` clips play on selection and activation, as WAV in the browser | Unit |
| Page transitions | In-effects play frame by frame, unless the disc turned them off for this page change | Unit |
| STILL_ON and STILL_OFF | Freeze and release the picture | None |
| Normal-state animation | Selected-state animation plays; normal-state animation shows its first frame | None |

## Subtitles

| Feature | What WittyBluray does | Tested |
|---|---|---|
| Off by default | Subtitles no longer switch on by themselves on every disc. They show when the disc or the viewer says so | Disc |
| Forced text | Taken from the stream in the language being heard. Forced and ordinary objects in one display set are separate pictures, so only the forced part shows with subtitles off | Unit |
| Windows | Objects are cut to their windows, which is how authors wipe text on | Unit |
| Epochs | State is dropped at each epoch start, keeping memory small | Disc |
| Read-ahead | Reading starts five seconds early, since a picture is stored ahead of its time | Disc |
| Sub-path subtitles | Played from their own clip through the sync point | Unit |
| Text subtitles | Words, place and style go to the browser, which draws them in its own type. UTF-8 and UTF-16 work; older Asian codings show nothing, since those tables are not included | Unit |
| Ultra HD colours | Palettes use BT.2020 on 2160p, BT.709 on HD and BT.601 on SD | Unit |

## Audio and secondary streams

| Feature | What WittyBluray does | Tested |
|---|---|---|
| Music discs | Discs with no video play over a black picture, at a very low frame rate to spare the machine | Unit |
| Sub-path audio | Played as a second input, looping when the sub-path repeats | Unit |
| Viewer's language | Actually used now: the old default register made it impossible to apply | Unit |
| Secondary audio and picture-in-picture | Registers and stream tables are complete, so discs can select them. They are not mixed in | Not handled |

## 3D, Ultra HD and HDR

| Feature | What WittyBluray does | Tested |
|---|---|---|
| MVC 3D | Plays the 2D base view and tells the disc it is a 2D player. The bundled FFmpeg has no MVC decoder | Disc |
| SET_OUTPUT_MODE | Only a 3D player can turn on 3D, as the format says | Unit |
| Ultra HD | 2160p HEVC with HDR10, HDR10+ and the Dolby Vision base layer, tone-mapped. The enhancement layer is parsed and not decoded | None |

## BD-J

Many commercial discs write their menus in Java. See [BD-J in BLURAY.md](BLURAY.md#bd-j-java-menus)
for setup, and [the licensing notes](BLURAY.md#java-and-licensing).

| Feature | What WittyBluray does | Tested |
|---|---|---|
| BD-J objects | Applications, JAR files, playlists, keys, file access, read with or without Java | Unit |
| Xlet life cycle | Both Xlet interfaces, on a thread of their own, with their arguments | Java |
| Drawing | HAVi scenes and AWT components paint to an off-screen plane; only the changed rectangle crosses to the server, at most 12 times a second | Java |
| Keys | Reserved keys reach the event manager first, then the scene | Java |
| Playlists | JMF players with `bd://` locators play on the server's normal video path, with end-of-media and mark events | Java |
| Registers | `RegisterAccess` shares the navigator's registers, with the same write rules | Java |
| Storage | Each disc has its own persistent area, limited to 64 MB | Java |
| Fonts and images | The disc's font index and fonts; pictures from JARs and files | None |
| Falling back | If an application cannot start, or Java is off, missing or crashes, the disc plays its main feature | Java |
| BD-Live | No network: applications see what they would with the cable out | Java |

**The sandbox.** A disc's classes load through a default-deny list, so a name not on it cannot be
linked. That rules out reflection, class loading, processes, native code, sockets and file systems.
File, URL, System and Toolkit classes are swapped for checked versions in the class file itself.
The process has a 192 MB heap and no copy of the server's environment, and it dies with the
session. BD-J is off until an administrator turns it on. The test application tries each of these
escapes and the test fails if one works.

This is a strong barrier, not a guarantee. Disc code still runs inside a real JVM and uses its image
decoders and AWT, so a JVM bug could be reached from a hostile disc. Run the server as an
unprivileged user, and only turn this on for discs you trust.

## File layouts and damage

| Feature | What WittyBluray does | Tested |
|---|---|---|
| BACKUP folder | Every structure falls back on its copy, on damage as well as absence, including files only the backup still has | Unit |
| Everything damaged | With no readable program or playlists, each clip plays by itself | Unit |
| AVCHD | 8.3 names (`INDEX.BDM`, `.MPL`, `.CPI`, `.MTS`), under `BDMV` or `PRIVATE/AVCHD/BDMV` | Unit |
| BDAV recordings | Clips are listed and played; recordings' own playlists are not read | Unit |
| Names in any case | Copied folders work on case-sensitive file systems | Unit |

## Copy protection

| Feature | What WittyBluray does | Tested |
|---|---|---|
| AACS | Never decrypted. Encrypted discs are refused, detected by what the stream holds: an aligned unit with its copy-permission bits set and no sync bytes after its first packet. It samples the start and middle of several clips, so a clear intro cannot hide an encrypted film | Unit |
| Old detection | A folder named AACS used to mean refusal, and a disc without one passed even if encrypted. Now a decrypted copy that kept the folder plays, and an encrypted disc without it is refused | Unit |
| BD+ | Never run. Its folder is reported, since a copy that was not converted can have damaged picture by design | Unit |

## Not handled

| Feature | Why |
|---|---|
| Secondary audio and picture-in-picture | Would need mixing and overlay in the transcode. The disc's choices are recorded, not applied |
| Frame-accurate BD-J animation | The plane runs at 12 pictures a second; browsers cannot sync an overlay to a video frame through a server |
| MVC 3D output | No decoder in FFmpeg |
| Dolby Vision enhancement layer | No decoder; the HDR10 base layer plays |
| Lossless audio passthrough | A browser cannot take it; audio is transcoded |
| BD-Live downloads | No network, by design |
| AACS, BD+ | Never, by design |

## Testing a disc

```bash
wms bluray info  THE_DISC.iso              # structure, flags, anything unreadable
wms bluray walk  THE_DISC.iso              # simulate inserting it
wms bluray bdj   THE_DISC.iso              # BD-J titles, and what their code needs that the runtime lacks
wms bluray bdj   THE_DISC.iso 20 menu.png  # with Java: run them for 20 seconds, save the picture
```

If a BD-J disc falls back to its feature, the output of `bdj` usually says why. Attach it to a bug
report; it holds no disc content.

---
<sub>WittyMediaServer documentation</sub>
