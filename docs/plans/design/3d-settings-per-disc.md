# Design: 3D viewing settings that stay with the 3D disc

> **Status on 2026-11-04**
>
> **In progress:** [#18](https://github.com/wittywilla/WittyMediaServer/issues/18), in 0.7.0-alpha.

The owner, 2026-11-02, after the stretched and shifted DVD pictures: "I found out what was causing the video issues on the DVD player!!!! The media center saved my 3d blu ray settings and applied it across all videos. This needs to be revamped with the UI so this doesn't happen to anyone else!"

Then: "Don't build anything yet! The current build is fine! Put it in the docs for what we need to work on later!"

Not built. Not tested.

---

## What goes wrong today

From the published code (`web/src/player/viewPrefs.ts`, `ViewSettings.tsx`, `DvdPlayer.tsx`):

- The disc player's **Viewing settings** (the gear) hold everything in one set of preferences: seek bar, remote, picture fit, subtitle size and also **3D video** (`stereo`: Off, Side by side, Over-under) and **Show 3D as** (`stereoOut`: 2D one eye, or red/cyan).
- Those preferences are saved **for this disc** or for **Everything**, chosen by **Apply to** at the top of the panel. When a disc has nothing saved of its own, the panel starts on **Everything**.
- So choosing 3D on a 3D Blu-ray, with **Apply to** left on **Everything**, turns 3D on for every disc and every video in that browser, for good.
- With 3D on, the player hides the real video (`opacity: 0`) and draws a canvas from half of each frame (`StereoLayer`). On a normal 2D disc, that shows half the picture stretched to fill the box: exactly the "stretched", "zoomed" and "shifted" pictures in the owner's screenshots, and menus whose buttons don't line up with what's drawn.
- Nothing on screen says 3D is on. The only place it shows is inside the gear.
- It's saved in the browser's storage, so it follows that browser on every server and survives restarts and updates.

**About 0.6.1-alpha's Firefox fix.** It put the video in the exact box the buttons use. That change is correct and harmless, and stays. But the owner's stretched pictures came from this 3D setting, not from Firefox's layout: the side session's headless Firefox never reproduced the shift with 3D off. Re-check Firefox with 3D off before crediting 0.6.1 with any part of it.

---

## The fix

### 1. 3D is always "this disc only"

- `stereo` and `stereoOut` come out of the shared preferences. They're kept per item, in their own key (`wms.view.3d.<item id>`), whatever **Apply to** says.
- **Apply to** no longer covers them, and the panel says so.

### 2. Clear what's stuck today

When a player opens after the update:

- remove `stereo` and `stereoOut` from the **Everything** preferences (`wms.view`);
- remove them from every per-item preference set too, since switching **Apply to** copied the stuck global value into each disc's own settings.

People who really watch a 3D disc choose 3D once more on that disc. The changelog says so, plainly: "3D settings are now kept for one disc at a time. If you used 3D, turn it on again on your 3D discs."

### 3. 3D starts where the disc says so

- A disc or file the server knows is 3D (the Blu-ray's own 3D flag, or a file's stereo metadata, which the play info already reports as `stereo`) offers 3D on the start screen: "This disc is 3D. Watch in 2D (one eye), with red/cyan glasses, or as is?"
- Anything not known to be 3D starts in 2D, always. 3D can still be chosen by hand for side-by-side or over-under files, for that item only.

### 4. You can always see it, and turn it off in one press

- While 3D is on, the top bar shows a **3D: side by side** chip next to the title, with **Turn off** on it.
- The first time 3D comes on for an item, a short notice says what changed and how to undo it.
- Keyboard and remote: a shortcut (for example **3**) toggles it, and it's in the on-screen remote.
- Screen readers hear "3D on: side by side" when it changes.

### 5. A clearer gear panel

- **Apply to** moves next to the rows it covers, under a heading **For this disc, or for everything**.
- A separate section **3D (this disc only)** holds the two 3D rows, with the note "3D settings never apply to other discs."
- The footnote names exactly what's saved where: "Seek bar, remote and picture: saved for all videos on this device. 3D: saved for this disc."

### 6. Look for the same trap elsewhere

Any other preference that only makes sense for some media is checked for the same problem, in the disc player, the video player, music, books and games: per-item by nature, but saveable for everything. Candidates to check: **Picture: Fill screen** (fine for one film, wrong for a 4:3 cartoon), **Subtitle size**, **Disc restrictions**. Each either stays shared on purpose, or moves to per-item with the same clean-up.

### 7. Apps

WittyMediaClient's desktop app gets this from the web app. Native players that add 3D later must keep it per item from the start.

---

## Where it goes

| Release | Work |
|---|---|
| **0.6.3-alpha** | Sections 1, 2 and 4 (per-disc 3D, clearing stuck values, the on-screen chip), and a changelog line. Small, and it stops the problem for everyone |
| **0.7.0-alpha** | Sections 3, 5 and 6: starting in 3D only when the disc says so, the clearer panel, the check of other preferences |

## Done when

On a browser that had 3D saved for **Everything**: after updating, every 2D disc plays normally with no setting changed by hand; choosing 3D on a 3D Blu-ray affects only that disc; and while 3D is on, the top bar says so with a one-press way off. Checked in Firefox, Chromium and the desktop app, by mouse, keyboard and remote.

---

<sub>Future plans, written 2026-11-02. Not built, not tested.</sub>
