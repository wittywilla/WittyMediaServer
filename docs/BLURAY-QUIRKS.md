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

Some discs carry hundreds or thousands of playlists built to defeat "longest wins". WittyBluray scores every playlist, in minutes of distinct video, and adds or takes away points for what it finds. Each answer comes with its reasons, a confidence (high, medium or low) and the sentences behind it; the diagnose report and `wittybluray rank` show them.

| Pattern | What a disc does | What WittyBluray does | Tested |
|---|---|---|---|
| Permuted decoys | Hundreds or thousands of playlists of the film's clips in other orders, all the same length | Playlists with the same segments form a cluster. Each join between two clips is weighed: the clock carrying on, the clip's packet map, where the clips lie on an image, and last the clip numbers. The best order stands; the rest are decoys | Unit (1,000 decoys, each kind of evidence alone) |
| Nothing tells the orders apart | The same clips in orders that follow on equally well, and no program that can be read | Says so: the playlists are marked ambiguous, the lowest-numbered is picked, the confidence is low and the report warns | Unit |
| The real order only in the HDMV program | First play computes a number into a register; a title compares it against every playlist number | The program is read without running it: registers are followed as sets of constants through arithmetic, comparisons, jumps and calls. The playlist the register selects is the one found | Unit |
| The real order only in a Java program | A BD-J title picks the playlist | Nothing is run. The JAR files' class constants are read for playlist locators, five-digit names and numbers. A program that computes or scrambles them shows nothing, and the structure decides with lower confidence | Unit |
| Repeated clips | A decoy plays clips again to run longer than the film | Each stretch of a clip counts once | Unit |
| Fake short clips | Clips of a fraction of a second between the real ones | A copy padded this way is a scrambled copy by playing time; a playlist mostly of such pieces is segment spam | Unit |
| Shuffled subsets | Some of the film's clips, out of order | Decoys: they go back on the disc and mostly play another playlist's video | Unit |
| Missing or empty clips | A playlist names a clip with no information or no stream file, or times past a clip's end | Never chosen ahead of one that plays | Unit |
| Two cuts of a film | Theatrical and extended cuts share most clips (seamless branching) | Each cut is listed as an edition with its length and what only it has. The one the disc's program plays is the main one; without a program, the longer | Unit |
| A cut that is only a subset | A playlist of some of the film's clips in order | Cannot be told from a shorter cut by structure: listed as an edition, never picked over the full film unless the program plays it | Unit |
| Multi-angle | A play item with several clips | The first angle is scored; the count is reported | Unit |
| TV discs | A playlist per episode, often numbered out of order, and one that plays them all | Episodes are the largest run of clean playlists of similar length; their order is where each starts in the play-all playlist, which is split at those points | Unit |
| TV disc with only a play-all | One playlist of several whole clips of even length | Its parts are offered as a guess, with a warning | Unit |
| Forced trailers | First play runs trailers before the film | Short playlists passed on the way are intros; the first long one reached is the feature | Unit |
| 3D twin | The same film as a 2D and a 3D playlist | The 2D one is preferred, as 3D plays here as 2D. The 3D stream table and the extra sub-paths in the playlist's extension data are read | Unit |
| Ultra HD | HEVC, HDR10, HDR10+, a Dolby Vision layer; clips of tens of gigabytes | Reported per playlist; sizes do not wrap | Unit |

None of this decrypts anything. It reads only the parts of a disc that are never encrypted, and a disc protected with AACS or BD+ is still refused, whatever its playlists score.

What is not read for ranking: the commands of menu buttons (they are in the stream files), and BD-J programs as they run. A film that only a menu button starts is found by title programs and structure.

### Chapters

| Feature | What WittyBluray does | Tested |
|---|---|---|
| Marks across play items | A mark names a play item and a time in its clip. A chapter's start is the play items before it plus how far into its own item it lies | Unit |
| Marks out of order, twice at one instant, outside their item, or naming an item that isn't there | Sorted; counted once; moved to the item's nearer end; left out | Unit |
| Link points | Not chapters. Commands can still reach them | Unit |

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

| Feature | What WittyBluray does | Tested |
|---|---|---|
| A structure that parses but is wrong | A playlist naming a clip that isn't there, or an index naming movie objects that aren't there, is taken from `BACKUP` when the backup doesn't have the fault. The report says which files came from where | Unit |
| A backup that differs | Remarked on. The first copy is used when it reads well | Unit |
| A file damaged part of the way | A stream table, mark table, sub-path, extension block, program table or entry-point map that is cut off or runs past its length is left out and named; the rest of the file is kept | Unit, fuzzed |
| Movie-only copies | A folder with playlists, clip information and streams but no `index.bdmv` or `MovieObject.bdmv` plays from its playlists, and isn't called damaged | Unit |
| A copy cut short | A stream file shorter than its clip information counts is flagged, with how much of the clip the bytes there can play | Unit |
| Hostile sizes | No count or length from a file is trusted: an entry-point map that claims billions of entries, or a movie object file that claims gigabytes of commands, reads what the file holds. A disc has a budget of what it may keep in memory | Unit, fuzzed |
| UDF 2.50 images | Metadata partition and its mirror, extended file entries, files in many extents past 4 GB, chained descriptors, images of Ultra HD size, small files kept inside their entry | Unit (images written by the tests) |
| Burned and rewritable discs | WittyDisc reads sparable partitions (BD-RE) through their sparing table and virtual partitions (BD-R written in sessions) through their allocation table, and a rewritten file's newest entry. Parts of a file the disc doesn't record read as zeros and are never streamed | Unit (images written by WittyDisc's tests) |
| A folder of only `.m2ts` files | Not opened as a disc: there is no clip information to tell lengths by | None |

## Copy protection

| Feature | What WittyBluray and the server do | Tested |
|---|---|---|
| AACS | Never decrypted. Encrypted discs are refused, detected by what the stream holds: an aligned unit with its copy-permission bits set and no sync bytes after its first packet. It samples the start and middle of several clips, so a clear intro can't hide an encrypted film. The server checks for itself, whatever a helper says | Unit |
| Folder names | A folder named AACS decides nothing, and an encrypted disc without one is refused | Unit |
| BD+ | Never run. Its folder is reported, since the picture can be damaged by design | Unit |

## libbluray as a second opinion

| Feature | What WittyBluray does | Tested |
|---|---|---|
| A libbluray the user installed | Loaded at run time when asked (never needed, never shipped, no cgo). Its playlists, lengths, chapters, angles and choice of feature are compared with WittyBluray's; differences are listed and lower the confidence, and WittyBluray's answer stands | Unit, with a stand-in library. Not run against a real libbluray |
| Encrypted discs | Never handed to it: a disc must read as clear and have no AACS and no BD+ folder. No key file is passed, and it cannot be asked to read streams | Unit |
| Where | 64-bit macOS and Windows. On Linux only in a build with the `libbluray` tag, because the released Linux program is a static binary | Unit (macOS) |

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
