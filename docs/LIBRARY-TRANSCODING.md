# Library transcoding

Use this guide to convert the files in a library ahead of time, so they play without the server converting them while someone watches. It's in **Settings → Library transcoding**, for administrators.

This rewrites your files, so it starts switched off. To convert only while someone is watching, and leave your files alone, see [Live transcoding](TRANSCODING.md). To use a separate Tdarr server, see [Using Tdarr](TDARR.md).

- [The short version](#the-short-version)
- [The three presets](#the-three-presets)
- [What it does to your files](#what-it-does-to-your-files)
- [How it affects people watching](#how-it-affects-people-watching)
- [Converting new files by themselves](#converting-new-files-by-themselves)
- [Sound tracks](#sound-tracks)
- [Subtitles](#subtitles)
- [Unusual files, decision by decision](#unusual-files-decision-by-decision)
- [Fixing things](#fixing-things)

## The short version

1. Read the warning at the top of the page, tick **I understand how this affects streaming and my files**, and press **Turn on library transcoding**. Until you do, nothing can be changed.
2. Under **Choose a library**, pick one. Then choose the preset that sounds like what you want.
3. Under **What happens to the old file?**, leave it on **Keep the original** until you trust your recipe.
4. Press **Save changes** in the bar that appears. A run does what's saved.
5. Under **Try it on one file**, choose a film and press **Show what would happen**. Each track is listed with what will become of it.
6. Press **Check this library**. That's a dry run: it looks at every file, changes nothing, and tells you how many files it would change, roughly how much room you'd end up with and how long it would take.
7. Press **Start converting**. It shows here and in the activity panel at the top of every page, where you can pause or cancel it.

## The three presets

| Preset | What you get | The cost |
|---|---|---|
| **Make everything play on any device** | MP4 with H.264 picture and AAC sound, at most 1080p, with no HDR. A stereo track comes first, with surround beside it. Every phone, TV, tablet and browser should play it with no help from the server. | Heavy for the processor. Files may grow if they were very efficient to begin with. |
| **Save space** | H.265 picture, usually 30 to 60% smaller. Lossless sound becomes Dolby Digital Plus. HDR stays HDR and every track stays. A file that doesn't come out at least 10% smaller is left as it was. | The slowest choice. Older devices may need the server to convert these files while they play. |
| **Keep the best quality** | Picture and sound aren't touched. Files are tidied into MKV, with tracks in the order you want, and a stereo track added where there's only surround. | Very little: mostly copying. |

Under each preset, **What this does** says exactly what it does. **Picture details**, **Sound details** and **Subtitle details** let you change any of it. Changing something makes it your own recipe, and choosing a preset again goes back. Whatever you choose, a file that already matches is left alone, and running the same recipe twice does nothing the second time.

Save space needs an FFmpeg with H.265 in it. If yours doesn't have it, the page says so and it uses a stronger H.264 setting, which saves less. HDR files can't fall back on H.264, so those are left alone.

The picture format, file type, quality, speed, largest picture and how HDR, Dolby Vision, 10-bit, interlaced, 3D and varying frame rates are treated are all choices under **Picture details**. [Unusual files](#unusual-files-decision-by-decision) says what each does.

## What it does to your files

Nothing is replaced until the new file has been checked. For each file:

1. It checks there's room, keeping free the amount you chose. That's 10 GB unless you change it.
2. It writes the new file into a hidden folder beside the old one, `.wittymedia-work`, on the same drive.
3. It checks the new file: it opens, it's as long as the old one, the tracks and chapters are all there, and it really does decode at the start, middle and end. If not, the new file is thrown away and the old one is untouched.
4. It checks that the old file hasn't changed meanwhile, that nobody started playing it, and, if you asked for a saving, that there is one.
5. Only then does it swap them, doing what you chose for the old file:

| Choice | What happens to the old file | Good for |
|---|---|---|
| **Keep the original** (the default) | Moves to a hidden folder beside it, `.wittymedia-originals`. **Put the original back** undoes it. **Delete the original** frees the room. | Being sure |
| **Replace the original** | Deleted for good, once the new file is in place and the library knows. Choosing this asks for your password. | When you need the room and you've tested your recipe |
| **Write it beside the original** | Stays where it is, in use. The new file is `Film.wms-converted.mp4`, which a scan ignores. **Use the copy** puts it into use. | Comparing them yourself |

Kept originals and waiting copies are listed under **Originals and copies** at the bottom of the page.

The item in your library stays the same item, even if the file goes from `.avi` to `.mp4`. What people have watched, ratings, your edits and its match all stay. If the server stops at any moment, it tidies up when it starts again: temporary files are removed, and a swap caught halfway is finished or put back.

### Two more safety switches

- **This library is read-only.** Nothing the server does may change this library's files: no conversions, no saved artwork or details beside them, no uploads and no podcast episodes kept in its folder. Previews and dry runs still work. The setting is kept with the library, in the `libraries.read_only` column of the database.
- **Save next to media off.** If you've turned that off in **Settings → Metadata agents** (`saveMetadata=0`), the server keeps out of your media folders, so conversions stop too. A switch appears for each library, **Allow conversions in this library**. With it on, new video files are written, and still no subtitle files or artwork. **Check this library** and **Show what would happen** work either way.

## How it affects people watching

Converting competes with watching for the same processor and the same drive. People may see stutters, or a film that takes longer to start, especially if their device needs the server to convert while it plays. These settings are under **How it runs on this server**:

| Setting | What it does | Unless you change it |
|---|---|---|
| **Pause while people are streaming** | Holds the conversion still, using no processor, while anyone is playing something or a game. It carries on where it was. A library may take much longer to finish. | On |
| **Low priority** | Everything else on the machine gets the processor first. | On |
| **Only at certain hours** | Holds still outside the hours you choose, on the server's clock. They can run past midnight. | Off |
| **How much of the processor** | Threads for each file being converted. | Half of them |
| **Files at once** | More finishes sooner and leaves less for everything else. | 1 |
| **Use hardware encoders** | Uses the ones ticked under [Live transcoding](TRANSCODING.md#hardware-encoders), for ordinary 8-bit pictures. Far faster, with slightly larger files. HDR and 10-bit pictures always use software, which can be trusted to keep them right. | On |
| **Keep this much free on each drive** | A file that wouldn't fit waits. | 10 GB |
| **Wait for new files to settle** | For [automatic conversion](#converting-new-files-by-themselves). | 2 minutes |

**Time a 20-second test** really converts a stretch from the middle of the file you chose, and tells you how fast this machine did it and what the whole film would come to. The figures from **Check this library** are rougher, and say so.

There's a cost after conversion too. A file made smaller with H.265 may need converting back while it plays on an older device, which is more work for the server than the file you started with.

## Converting new files by themselves

Each library can have **Convert new files by themselves** turned on. It's off unless you turn it on, and it needs the recipe saved and library transcoding switched on.

- **What counts as new.** Only files a scan finds after you turned it on, and files that change after that. What's already in the library is left alone: **Start converting** is how to do those.
- **When it starts.** After a scan finds something, it waits until each file has stopped changing, 2 minutes unless you change it, so that a file still being copied or downloaded is left to finish. A file that keeps growing for 24 hours waits for the next scan. Then it starts an ordinary conversion of those files.
- **Its limits.** All the same ones: the hours, how much of the processor, files at once, low priority and pausing while people are streaming. Outside its hours or while people are watching, it doesn't start.
- **Where to see it.** In the activity panel, first as **New files in** and the library's name, saying what it's waiting for, then as **Converting new files in** and the library's name, with Pause and Cancel. Files with nothing to do leave no trace.
- **What it won't do again.** Each file's outcome is remembered against its size and date and the recipe. A file that fails, comes out no smaller, already matches or is put back isn't tried again unless the file or the recipe changes. **A run you cancelled isn't retried either.** **Try again** in the activity panel, or **Start converting**, still does it when you ask.
- **What it does try again.** A file someone was playing and a file that didn't fit on the drive are looked at after the next scan, because neither says anything against the file. A file that changed while it was being converted is looked at again too. If the server stopped partway, what was left is picked up when it starts again.

> **It uses the choice you saved for the old file.** If a library is set to **Replace the original**, automatic runs delete originals with nobody watching. The default stays **Keep the original**, and that's the safer choice until you've seen your recipe work.

## Sound tracks

Every track is either kept as it is, converted, or removed. **Show what would happen** says which, and why.

- **Languages.** Keep them all, or a list, in the order you want. **Also keep the original language** keeps what the film was made in: what the file marks as original, otherwise its default track, otherwise its first.
- **Plays first.** The language chosen when a viewer hasn't said what they want.
- **No language.** Keep or remove them, or label them with a language if you know what they are.
- **Commentary and audio description** are recognised by the file's flags and by words in their titles, and can be kept or removed. They go after the plain tracks of their language.
- **Stereo beside surround.** A language with only surround gets a stereo AAC track, for phones, headphones and TVs without a sound system. **Put stereo ahead of surround** makes plain devices start with it.
- **Most channels.** Wider tracks are converted down to 7.1, 5.1 or stereo.
- **Lossless sound** (TrueHD, DTS-HD MA, FLAC, PCM) can stay or be converted.
- **Object sound** (Atmos, DTS:X) survives only by staying as it is, so converting says plainly that it will be lost.
- **The last track.** A film never ends up silent: if your choices would remove every track, one stays and the preview says so.

## Subtitles

- **Languages.** Keep them all, a list in order, or none.
- **Forced** (only the lines in another language). Keep as a track, remove, burn into the picture, or burn in only if they're stored as pictures and the file type needs it. Burning means converting the picture, and it can't be turned off afterwards.
- **SDH** (with sound descriptions) and **untagged.** Keep or remove.
- **Stored as pictures** (Blu-ray PGS, DVD VobSub, DVB). They can't become text here, which would need character recognition, and an MP4 can't hold them. So you choose: keep them, with the file staying MKV; save Blu-ray ones as a `.sup` file beside the video; or remove them. Anything that would be lost is listed in the preview. With **Save next to media** off, nothing is saved beside the video.
- **Text** stays as it is, or becomes plain SRT. An MP4 holds only its own kind, so going to MP4 loses ASS styling, and the preview says so. Going to MKV turns MP4 text into SRT.
- **Files beside the video** (`Film.en.srt`) keep working if left where they are, because the name stays the same. They can instead be moved inside the new file.

## Unusual files, decision by decision

| The file | What happens |
|---|---|
| Already matches | Skipped, whichever recipe it is. Running a recipe again finds nothing to do. |
| Several picture streams | Kept and each converted if needed, or just the main one. The main one is a later stream marked default, otherwise the largest, and it goes first. |
| Cover pictures | Always kept, as they are, in MKV and MP4. An MP4 only holds JPEG and PNG: anything else is listed as lost. |
| Chapters and metadata | Kept. A new file that lost its chapters is thrown away. |
| Attached fonts | Kept in MKV. An MP4 can't hold them, and the preview says so. |
| HDR10, HLG | Your choice: stay HDR (H.265 or AV1 only, with a software encoder, keeping the mastering information), become an ordinary picture, or leave those files alone. |
| HDR10+ | Kept if the picture isn't converted. Converted, it stays HDR10, and the preview says the scene-by-scene information is lost. |
| Dolby Vision | Kept only if the picture isn't touched. If it would have to be, your choice: leave the file alone, or convert from its ordinary layer, losing Dolby Vision. Profile 5 has no ordinary layer, so it's always left alone rather than come out with the wrong colours. |
| 10-bit | Stays 10-bit in H.265 and AV1. "Any device" makes it 8-bit. |
| Interlaced | Deinterlaced if it's being converted and you asked. |
| Variable frame rate | Stays as it is, so sound stays in step. "Any device" makes it constant. |
| 3D | Keep both eyes as stored, keep one eye, or leave those files alone. The kind that stores its second eye separately (MVC) can't survive conversion, so it's left alone unless you choose one eye. |
| Already small | With Save space, a picture already small for its size is left as it is, since converting it again would lose more than it saves. |
| Broken timestamps | New ones are made. If the file's stated length is wrong, what's really in it decides. If it can't be made right, the old file stays. |
| A split film (`CD1`, `part2`) | Its parts stay separate files, are converted together with the same settings, and swap together. If one part fails, none is replaced. If one part fits and another doesn't, both are converted, so they still join. |
| Fails partway | The temporary file is removed, the old file is untouched, and it's listed as a problem you can retry. |
| Sound-only | Left alone, since these recipes are about pictures. |
| Changed during the job | The new version is thrown away, and it's looked at again next time. |
| Being played | Left until the end of the run. If it's still being played then, it's left for next time. If someone starts playing it just as the new file is ready, the swap waits for them. |
| Disc images and folders | Not converted: they'd lose their menus. They play as they are. |
| Copy-protected or encrypted | Refused, and never decrypted. |
| A link leading out of the library | Refused. Nothing outside a library's folders is ever touched. |

## Fixing things

| What you see | What to do |
|---|---|
| **Start converting** is greyed out | The line under it says why: turned off, read-only, unsaved changes, no recipe saved, or Save next to media. |
| A file failed | Its old file is untouched. Open it in the list to see why. **Try again** in the activity panel's history retries what's left. **Settings → WittyLog** has FFmpeg's own words. |
| "Not enough room" | Free some space, delete kept originals, or choose to replace originals. |
| It says it's waiting | People are watching, or it's outside its hours. The activity panel says which. |
| A folder called `.wittymedia-originals` | Your originals. Delete them from this page, or by hand once you're happy. If you delete them by hand, **Put the original back** will say they've gone. |
| A folder called `.wittymedia-work` | Temporary files. It removes them when it starts up, and it's safe to delete if the server isn't converting anything. |
| You want to go back to a file by hand | Move it out of `.wittymedia-originals` to where it was, delete the converted one, and scan the library. |

---

<sub>WittyMediaServer documentation</sub>
