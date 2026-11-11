# Blu-ray disc quirks

This reference lists what the Blu-ray format can do, what real discs do with it, and what WittyBluray does in each case. It's for people working on the engine. The DVD counterpart is [DVD disc quirks](DVD-QUIRKS.md), and to play discs, see [Blu-rays](BLURAY.md).

**How to read the Tested column**

| Mark | Meaning |
|---|---|
| Disc | Checked on the test images: two custom-authored discs, neither retail |
| Unit | Checked by a Go test built from synthetic data in the real file format |
| Java | Run end to end on real Java 21, in the browser player, with a test application |
| Sample | An open-source application not written for this project ran it |
| None | Written from the format, with no test data available |

No retail BD-J disc, UHD disc or real MVC disc has been available for testing yet, so anything marked Unit or Java may still meet surprises on commercial discs.

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
| Index entries | First play, top menu and titles each name a movie object or BD-J object in 12 bytes | Parsed as such | Unit |
| One level of suspension | CALL_OBJECT, CALL_TITLE and a menu call keep exactly one way back | One slot. A second call replaces it | Unit |
| RESUME | Goes back with the playback position | Restores title, chapter, playlist, item, time, button, page and style. General registers and stream choices stay as they are, so a setup menu's changes survive | Unit |
| Resume intention | A movie object says whether a menu call may come back to it | Honoured: without it, the object ends at the menu call | Unit |
| Menu call mask | A movie object or the playing item can forbid the Menu key | The key does nothing while forbidden | Unit |
| Jumps from buttons | A button can run JUMP_OBJECT or CALL_OBJECT | Followed | Unit |
| JUMP_TITLE | Forgets what was suspended | Done, so Resume can't return to a film the disc left | Unit |
| Running off the end | A movie object that ends without jumping | Playback stops, and nothing resumes by itself | Unit |
| Arithmetic | Saturates at the top and stops at zero | Done. Division or modulo by 0 gives all ones, and huge shifts give 0 | Unit |
| Bit compare | True when the first operand has a bit the second lacks | Done | Unit |
| Bad register references | A reference with stray bits set is no register | Reads as 0, and writes are ignored | Unit |
| Set options | Five bits wide, not four | Read as five, so option 0x11 doesn't look like SET_STREAM | Unit |
| Button-only commands | LINK, TERMINATE_PL, POPUP_OFF, STILL and enable and disable only work in buttons | Ignored in movie objects | Unit |
| SET_BUTTON_PAGE | In a button it ends the button's commands. Everywhere it writes the registers | Done | Unit |
| Navigation timer | Object first, then seconds. At most 300, and 0 cancels | Runs from movie objects | Unit |
| Run limit | A disc's program could loop for ever | Stops after 1,000,000 commands in one run, or 64 jumps without playing | Unit |

## Player registers

| Feature | What WittyBluray does | Tested |
|---|---|---|
| Default values | All registers start at the format's "nothing chosen" values: audio 0xff, subtitles 0x0fff0fff, button 0xffff and so on. A disc's program tells a first visit from a later one by these | Unit |
| Profile and version | PSR 31 uses the real encoding: 0x00030200 for profile 2, 0x00130240 for 3D, 0x00000300 for Ultra HD. This can change what a disc offers | Unit |
| Capabilities | Audio, video and text capability registers say what a transcoding server can in fact deliver | Unit |
| Ultra HD discs | Index version 0300 or above gets an Ultra HD player identity, since such discs check | Unit |
| Menu state | The page and button on show are written to PSR 11 and 10. Commands often read them to learn what was pressed | Disc |
| Stream selection | When an item starts, a stream number it lacks is replaced by the viewer's language, else the first | Unit |
| Chapter register | PSR 5 follows entry marks and counts only those | Unit |
| Region, parental level, country | Settings. Blu-ray regions are checked by the disc's own program, which then shows its own message | Unit |

## Playlists

| Feature | What it is | What WittyBluray does | Tested |
|---|---|---|---|
| Random and shuffle | A playlist can draw its items at random, a set number of times | Played item by item. Shuffle plays each once before any repeat | Unit |
| Still items | An item can hold its last picture for some seconds or until the viewer acts | Held. Menus built on a held picture don't loop | Unit |
| Slideshows | Pictures one at a time, often with music from a sub-path | Item by item, with Next and Previous moving between pictures | Unit |
| Items with different streams | One playlist can change stream PIDs or codec between items | Played one item at a time, since one stream can't carry both | Unit |
| Menu that moves on | When a menu's playlist ends, the program may start the film | The menu loops only if the program would replay it. Otherwise the end is reported | Disc, Unit |
| Stream table order | Picture-in-picture subtitles follow subtitles. Secondary streams carry lists of what they go with | Parsed in that order, including Dolby Vision layers | Unit |
| User operation masks | A playlist or item can forbid seeking, skipping, pausing and stream changes | Honoured in the player, with **Disc restrictions** to ignore them | Unit |
| Mark types | Entry marks are chapters, and link points aren't | Only entry marks show as chapters. Commands can reach both | Unit |
| Finding the feature | Copy protection schemes add decoy playlists | See below | Unit |

### Finding the feature among decoys

Some discs carry hundreds of playlists built to defeat "longest wins". WittyBluray scores every playlist instead, in minutes of distinct video, and adds or takes away points for what it finds. The diagnose report gives each playlist's score with the reasons for it.

- **Distinct video.** Each stretch of a clip counts once, however often a playlist repeats it, so a playlist padded with repeats gains nothing and loses points for the time it spends replaying.
- **Shuffled and duplicate copies.** Playlists that hold the same stretches are compared by how naturally they run: whether each piece starts where the last one ended, in time or on the disc. The most natural one stands, and copies in a scrambled order count as decoys. An exact copy in the same order is the same film, so it only loses a tie.
- **Missing clips.** A playlist that names a clip that isn't on the disc, or an empty one, or times past a clip's end, is a decoy and is never chosen ahead of one that plays.
- **Segment spam.** A playlist made mostly of tiny pieces, under a second or a few seconds each, is a decoy.
- **The disc's own program.** WittyBluray reads the disc's first-play and Title 1 instructions without running them, and follows where they lead. A playlist the disc plays first by itself gains points, and short playlists passed on the way are taken as intros. Playlists any title plays gain a little too.
- **Java (BD-J) discs.** The program of a disc whose first play or Title 1 is a Java application isn't read for this, so the choice comes from the structure alone, plus a small hint when the application lists only a few playlists. The report says so.
- **Smaller signals.** Even chapter lengths, more soundtracks and subtitles, and more data on the disc each add a little. A 3D twin gives way to its 2D playlist, as 3D plays here as 2D.

TV discs are found the same way: several clean playlists of similar length become episodes, in the order a play-all playlist gives, if there is one.

None of this decrypts anything. It reads only the parts of a disc that are never encrypted, and a disc protected with AACS or BD+ is still refused, whatever its playlists score.

The ranking reads structure, never pictures, so a disc whose program chooses the film in ways only a running player can see may still pick wrongly. Choosing a title from the disc's own menu, or from **Bonus Features**, avoids the guess.

## Clips and time

| Feature | What it is | What WittyBluray does | Tested |
|---|---|---|---|
| Several time bases | A clip's clock can restart mid-file, so one time can occur twice | Entry points are looked up within the item's own time base | Unit |
| Clips with no video | Audio-only clips map their audio instead | The clip's own map is used | Unit |
| No time-base table | A damaged clip | Handled | Unit |
| 4:3 on a Blu-ray | Standard-definition extras can be either shape | Shape read from the clip | None |
| Late audio or video | Separately muxed sub-paths have their own clock | Mapped through the sub play item's sync point | Unit |
| Play item ending or starting between keyframes | Its IN and OUT times needn't be entry points, and a picture shown after OUT may be needed to decode one before it | Read past both, decode, and leave out what lies outside by time. The next item carries on in the same read when it continues the same clip | Unit |

## Menus

| Feature | What WittyBluray does | Tested |
|---|---|---|
| Time-outs | Selection end presses the default button, and composition end removes the buttons | Unit |
| Pop-up time-out | A pop-up closes when no key has come for the disc's time | None |
| Button groups | Enabling a button replaces the others in its group | Disc |
| Hidden neighbours | A disabled button can't take the selection | Disc |
| Self-linked buttons | Discs link keys to the button itself to disable them, as one tested disc does with Left and Right. Followed as written | Disc |
| No default selection | Falls back on the disc's last selection, then the first button | Disc |
| Numeric keys | A button's own number, not its id | None |
| Button sounds | `sound.bdmv` clips play on selection and activation, as WAV in the browser | Unit |
| Page transitions | In-effects play frame by frame, unless the disc turned them off for this page change | Unit |
| STILL_ON and STILL_OFF | Freeze and release the picture | None |
| Normal-state animation | Selected-state animation plays. Normal-state animation shows its first frame | None |

## Subtitles

| Feature | What WittyBluray does | Tested |
|---|---|---|
| Off by default | They show when the disc or the viewer says so | Disc |
| Forced text | Taken from the stream in the language being heard. Forced and ordinary objects in one display set are separate pictures, so only the forced part shows with subtitles off | Unit |
| Windows | Objects are cut to their windows, which is how authors wipe text on | Unit |
| Epochs | State is dropped at each epoch start, keeping memory small | Disc |
| Read-ahead | Reading starts five seconds early, since a picture is stored ahead of its time | Disc |
| Sub-path subtitles | Played from their own clip through the sync point | Unit |
| Text subtitles | Words, place and style go to the browser, which draws them in its own type. UTF-8 and UTF-16 work. Older Asian codings show nothing, since those tables aren't included | Unit |
| Ultra HD colours | Palettes use BT.2020 on 2160p, BT.709 on HD and BT.601 on SD | Unit |

## Audio and secondary streams

| Feature | What WittyBluray does | Tested |
|---|---|---|
| Music discs | Discs with no video play over a black picture, at a very low frame rate to spare the machine | Unit |
| Sub-path audio | Played as a second input, looping when the sub-path repeats | Unit |
| Viewer's language | Used when a disc hasn't chosen | Unit |
| Secondary audio and picture-in-picture | Registers and stream tables are complete, so discs can select them. They aren't mixed in | Not handled |

## 3D, Ultra HD and HDR

| Feature | What WittyBluray does | Tested |
|---|---|---|
| MVC 3D | Plays the 2D base view, and tells the disc it's a 2D player. FFmpeg has no MVC decoder | Disc |
| SET_OUTPUT_MODE | Only a 3D player can turn on 3D, as the format says | Unit |
| Ultra HD | 2160p HEVC with HDR10, HDR10+ and the Dolby Vision base layer, tone-mapped. The enhancement layer is parsed and not decoded | None |

## BD-J

Many commercial discs write their menus in Java. See [BD-J menus](BLURAY.md#bd-j-java-menus) for setup, and [Java and licensing](BLURAY.md#java-and-licensing).

Java is the owner's to install, and BD-J is off until an administrator turns it on. Which Java is used is set only in the server's config file (`javaPath`, `-java` or `WMS_JAVA`), never from the web page. The code below is in WittyBluray, in `bdj`, and the server starts it through the helper.

| Feature | What WittyBluray does | Tested |
|---|---|---|
| BD-J objects | Applications, JAR files, playlists, keys and file access, read with or without Java | Unit |
| Xlet life cycle | Both Xlet interfaces, on a thread of their own, with their arguments | Java |
| Drawing | HAVi scenes and AWT components paint to an off-screen plane. Only the changed rectangle crosses to the server, at most 12 times a second | Java |
| Keys | Reserved keys reach the event manager first, then the scene | Java |
| Playlists | JMF players with `bd://` locators play on the server's normal video path, with end-of-media and mark events | Java |
| Registers | `RegisterAccess` shares the navigator's registers, with the same write rules | Java |
| Storage | Each disc has its own persistent area, limited to 64 MB | Java |
| Drawing straight to the screen | What most animated menus do: `getGraphics`, then `Toolkit.sync`. What's drawn stays, like a real graphics plane | Java, Sample |
| HAVi widgets | Buttons, text, lists, entry fields, ranges and animations, with focus and looks | Java, Sample |
| Fonts and images | The disc's font index and fonts, and pictures from JARs and files, with the callback `prepareImage` promises | Java, Sample |
| XML | Parsers read only what they're given, and a DOCTYPE or entity fetches nothing | Java |
| Quitting | An application that ends itself returns the disc to its menu, and a film it started goes on. A menu that only ever quits ends at the feature | Java |
| Falling back | If an application can't start, or Java is off, missing or crashes, the disc plays its main feature | Java |
| BD-Live | No network: applications see what they would with the cable out | Java, Sample |
| Other classes of the published APIs | About 300 written from the specifications, many returning empty answers a disc player has no data for. Tested only where a sample calls them | Partly |

**What has run, and how far.** The published BD-J samples are the closest thing to a real disc that's freely available. They're used in testing only, and none of their code is in this project.

| Application | How far it got |
|---|---|
| HD Cookbook hello xlet, built as discs are (class version 47) | Starts, draws |
| HD Cookbook GRIN engine: tumbling duke, bouncing Reddy | Parses its show, loads pictures, animates through its segments. Scene graph and direct drawing work |
| HD Cookbook storage writer | Its own test passes |
| HD Cookbook progressive playlist | Starts, draws, is told downloaded content isn't available, as designed |
| A test menu with HAVi buttons, on a real disc structure with real video | In the player: keys move focus, a playlist plays under the graphics, its score is saved across sessions, and quitting returns to the disc's menu |
| The two Blu-ray images tried | Neither has Java menus |

Not reached: a retail disc, and the HD Cookbook's full book menu and game, which need a show compiler that wasn't run. How well the API classes match a real disc's calls past these is still unknown. Frame-accurate timing with video hasn't been tried at all.

**The sandbox.** A disc's classes load through a default-deny list, so a name not on it can't be linked. That rules out reflection, class loading, processes, native code, sockets and file systems. File, URL, System, Toolkit and XML parser classes are swapped for checked versions in the class file itself. An XML parser reads only the bytes it's given: it follows no name, external entity or DTD. Behind that, no socket of the process connects, whoever asks.

A hostile test disc (`bdj/java/testxlet/a`) tries each way out, and the test fails if one works, checked from outside as well: a file that appears, a port that answers. Three allowed packages turned out to be doors: `java.rmi` opens sockets, the XML packages fetch what a document names, and `java.awt.color.ICC_Profile` reads and writes any file. So a whole package goes on the list only after reading every class in it.

**What a runaway disc costs.** These are enforced from the server's side of the pipe, where disc code can't reach them.

| It does | What happens |
|---|---|
| Fills the Java heap, direct buffers or class space | Limits of 192, 32 and 96 MB. Running out ends the process |
| Holds native memory the heap doesn't count | Past 768 MB in all, the process is ended |
| Starts thread after thread | Past 512 threads, the process is ended |
| Spins on many cores | Past about 2 cores for a minute, the process is ended. On Linux and macOS it also runs slightly niced |
| Floods the link with messages or printing | Read at a set rate, which holds the disc back. The excess is dropped |
| Asks for playlists or titles as fast as it can | Only the newest is carried out, at most about 7 a second |
| Fills its storage | 64 MB, however it writes: `RandomAccessFile` holes and `setLength` count |
| Hangs in `initXlet`, `paint` or a listener | Only itself. Ending the session ends the process |

When the process ends, the disc plays its main feature, and the log says why. A single core kept busy is tolerated, since some menus do that, and it ends with the session.

This is a strong barrier, not a guarantee. Disc code still runs inside a real JVM and uses its image and font decoders and AWT, so a JVM bug could be reached from a hostile disc. Run the server as an unprivileged user, and only turn this on for discs you trust.

## File layouts and damage

| Feature | What WittyBluray does | Tested |
|---|---|---|
| BACKUP folder | Every structure falls back on its copy, on damage as well as absence, including files only the backup still has | Unit |
| Everything damaged | With no readable program or playlists, each clip plays by itself | Unit |
| AVCHD | 8.3 names (`INDEX.BDM`, `.MPL`, `.CPI`, `.MTS`), under `BDMV` or `PRIVATE/AVCHD/BDMV` | Unit |
| BDAV recordings | Clips are listed and played. Recordings' own playlists aren't read | Unit |
| Names in any case | Copied folders work on case-sensitive file systems | Unit |

## Copy protection

| Feature | What WittyBluray and the server do | Tested |
|---|---|---|
| AACS | Never decrypted. Encrypted discs are refused, detected by what the stream holds: an aligned unit with its copy-permission bits set and no sync bytes after its first packet. It samples the start and middle of several clips, so a clear intro can't hide an encrypted film. The server checks for itself, whatever a helper says | Unit |
| Folder names | A folder named AACS decides nothing, and an encrypted disc without one is refused | Unit |
| BD+ | Never run. Its folder is reported, since the picture can be damaged by design | Unit |

## Not handled

| Feature | Why |
|---|---|
| Secondary audio and picture-in-picture | Would need mixing and overlay in the conversion. The disc's choices are recorded, not applied |
| Frame-accurate BD-J animation | The plane runs at 12 pictures a second, and browsers can't sync an overlay to a video frame through a server |
| MVC 3D output | No decoder in FFmpeg |
| Dolby Vision enhancement layer | No decoder. The HDR10 base layer plays |
| Lossless audio passthrough | A browser can't take it, so audio is converted |
| BD-Live downloads | No network, by design |
| AACS, BD+ | Never, by design |

## Testing a disc

The `wittybluray` tool comes from the WittyBluray project: see [Blu-rays](BLURAY.md#for-developers).

```bash
wittybluray info  THE_DISC.iso              # structure, flags, anything unreadable
wittybluray walk  THE_DISC.iso              # simulate inserting it
wittybluray bdj   THE_DISC.iso              # BD-J titles, and what their code needs that the runtime lacks
wittybluray bdj   THE_DISC.iso 20 menu.png  # with Java: run them for 20 seconds, save the picture
wittybluray bdj   THE_DISC.iso 30 menu.png down enter  # the same, pressing those keys on the way
```

With Java it also lists the Java library classes the disc names that the sandbox refuses, and what the process used in memory, threads and processor time.

If a BD-J disc falls back to its feature, the output of `bdj` usually says why. Attach it to a bug report, as it holds no disc content.

---

<sub>WittyMediaServer documentation</sub>
