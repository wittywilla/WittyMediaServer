# Design: arranging libraries, right-click menus and deep customisation

> **Status on 2026-11-04**
>
> **Planned** for after 0.7.0-alpha: [library order](https://github.com/wittywilla/WittyMediaServer/discussions/27), [right-click menus](https://github.com/wittywilla/WittyMediaServer/discussions/28), [deep customisation](https://github.com/wittywilla/WittyMediaServer/discussions/29).

For the next major server version (written as 0.8.0-alpha). The owner: "easily customize the order of libraries across the top and side in a customization menu. Allow the ability to right click on those library listings in a web browser and have easy access to settings and the ability to edit metadata / refresh metadata. Make this as extensive as all the customization options available on other providers."

Not built. Not tested. Not a priority yet, by the owner's word.

---

## What 0.6.0-alpha already has

From the 0.6.0 notes: the top bar has customising and a **More** overflow (the owner's line about a big top bar), moving posters with an off switch, eight themes, a TV look per account (`tvLook` in `GET /api/me/ui`), per-user choice of profile name or username at the top right, and an activity button. This design builds on those.

---

## 1. The customisation menu

**Where:** **Settings → Appearance → Navigation**, and **Customise…** at the end of the top bar's More menu and the side menu.

**What it arranges:**

| | Choices |
|---|---|
| **Order** | Drag, or select and use Move up / Move down (keyboard, remote, screen reader). Same order for the top bar and side menu, or a separate order for each |
| **Show or hide** | Each library and each fixed page (Home, Search, Channels, Games…). Hidden ones stay reachable from **All libraries** |
| **Group** | Put libraries in a named group ("Kids", "Music & radio") that opens as a menu |
| **How many in the top bar** | Automatic (fits the window, the rest go to More), or a fixed number |
| **Labels** | Icon and name, name only, icon only (phone default) |
| **Rename for this profile** | A local name, without renaming the library for everyone |
| **Icon** | From the built-in set |
| **Start page** | Which page opens after signing in |

**Who:** each profile arranges its own. Admins set the **default** for new profiles and can **apply to everyone**, and can lock it for kids' profiles.

**Reset:** "Back to the default" per profile, and "Back to stock" for the admin's default.

**Where it's kept:** on the server, per profile, in `GET /api/me/ui` under `navigation`, so every app follows the same order. An app that doesn't know the field shows the server's default order.

## 2. The context menu

**How it opens:** right-click on the web and desktop app; long press on touch; the **menu** button on a TV remote or the **options/view** button on a controller; the keyboard's context-menu key and Shift+F10. Every action in it is also reachable another way (the library page's own menu), so the context menu is a shortcut, never the only path.

**On a library listing (top bar, side menu, Home row title):**

- Open, Open in new tab (web)
- **Scan for new files**, **Refresh metadata** (choice: missing only, or everything), **Check matches**
- **Library settings…**
- **Edit details of every item…** (bulk edit, which exists)
- Move left / right, Hide, Pin to Home, Customise navigation…
- Mark all as watched or unwatched (with a confirmation)
- For admins only, by role, from the server

**On an item card (poster, episode, album, game, photo):**

- Play, Play from start, Resume, Add to queue, Shuffle (where it fits)
- Mark watched or unwatched
- **Edit details…**, **Change poster…** (the new picker), **Fix match…**, **Refresh metadata**
- Add to collection or playlist, Add to channel
- Show in folder (admins: the file path, copyable), Media info, Download (if allowed)
- Delete (admins, if deleting is turned on; with a confirmation that names the file)

**Actions come from the server:** `GET /api/libraries/{id}/actions` and `GET /api/items/{id}/actions` return the actions this profile may take, with labels and the route each one calls. The web app and the native apps draw the same menu without hard-coding permissions. Every action's route checks permissions again on its own (the menu is a convenience, not a gate).

**Accessible:** a real menu (`role="menu"`, items with names, arrows move, Escape closes and returns focus). Big enough targets on touch.

## 3. Customisation as deep as the best media servers, and more

The owner asked for "as extensive as all the customization options available on other providers". A list to work from, grouped. Each is small on its own; the work is keeping them coherent and fast.

### Home

- Choose, order and hide rows: Continue watching, Up next, Recently added (per library or combined), Recently released, Collections, Genres, Studios, Networks, Streaming listings, Channels on now, Random picks, Favourites, Watchlist, "Because you watched".
- Rows per profile, with the admin's default.
- Card size per row (small, medium, large), poster or wide.
- Hide spoilers: blur episode thumbnails and descriptions for unwatched episodes.

### Libraries

- Default view per library: posters, wide, list, table, by folder.
- Default sort and direction per library, remembered per profile.
- Filters saved as named views ("4K films", "Unwatched comedies"), which can be pinned to the navigation or Home.
- What a card shows: title, year, rating, length, watched mark, resolution badge, progress bar.
- Group a TV library by show, or flatten; collapse single-season shows.
- Per-library look: backdrop on or off, theme colour.

### Item pages

- Which sections show and in what order: cast, extras, similar, collections, reviews, chapters.
- Trailers: play automatically, muted, or not.
- Theme music on show pages: on or off, volume.

### Playback

- Per profile: preferred audio and subtitle languages, subtitle style, skip intro and credits automatically, next-episode countdown length, default quality on each network.

### Everything else

- Saved layouts: export and import a profile's whole customisation as a file (data only, validated like the other settings files).
- Kids' profiles: the admin can lock any of this.

## Performance

All of this is a few small JSON objects per profile, read once on sign-in and cached. Rows on Home load lazily as they come into view (as Home already does; **to verify**). No setting may add a query per card.

## Order of work

1. Data model and `GET`/`PUT /api/me/ui` fields, with a migration from today's top bar setting.
2. The customisation menu, keyboard and remote first.
3. Actions routes, then the context menu on the web.
4. The deep customisation list, a group at a time, starting with Home rows and per-library default views.
5. WittyMediaClient: desktop comes free; Android and Apple read the same fields.

---

<sub>Future plans, written 2026-11-02. Not built, not tested.</sub>
