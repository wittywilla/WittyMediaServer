# First-time setup, step by step

First-time setup happens **in a web browser only**. Until it's finished, apps can't use the server: they say "This server isn't set up yet", with the address to finish at and a QR code of it ([First run](CLIENT-API.md#first-run) says what an app is sent).

This guide walks through every page, so you know what's coming and what each answer does. It takes a few minutes. Only the account is required: every other page has **Skip for now** under it, and says where to change it later in **Settings**. Your answers are saved as you go, so you can go back, or stop and carry on another day.

There are always fourteen steps, and a list of them sits beside the page: a tick for those you've done, the one you're on highlighted, and the ones coming. A step for a module you haven't added (Live TV, Games, Discs) stays in the list, dimmed and marked **module not added**, and is passed over. "Step 8 of 14" never changes its total.

| | Step | When it asks |
|---|---|---|
| 1 | [Welcome](#1-welcome) | Always |
| 2 | [Account](#2-account) | Always |
| 3 | [Name and address](#3-name-and-address) | Always |
| 4 | [Look](#4-look) | Always |
| 5 | [Theme](#5-theme) | Always |
| 6 | [Modules](#6-modules) | When the server can reach the module list |
| 7 | [Libraries](#7-libraries) | Always |
| 8 | [Details and artwork](#8-details-and-artwork) | Always; four pages, two only where the server can use them |
| 9 | [Live TV](#9-live-tv) | With WittyTuner |
| 10 | [Games](#10-games) | With WittyPlay |
| 11 | [Discs](#11-discs) | With WittyDVD or WittyBluray |
| 12 | [People and rules](#12-people-and-rules) | Always |
| 13 | [Remote access and apps](#13-remote-access-and-apps) | Always |
| 14 | [Ready](#14-ready) | Always |

The pictures show what each page holds. They were taken before the current looks, so the colours and shapes on your screen will differ. The page itself wears the look and theme you're trying, as you choose them.

- [Before you start](#before-you-start)
- [With a remote, a controller or the keyboard](#with-a-remote-a-controller-or-the-keyboard)

## Before you start

Install the server first: [Getting started](GETTING_STARTED.md#install-it-and-start-it) says how for each system. Setup opens in your browser by itself on Windows and on a Mac. Elsewhere, open `http://<this computer>:9797/`.

**If it asks for a setup code,** you're on another device. That's so nobody else on your network can claim the server first. The code is on the server's console when it starts, or in `setup-code.txt` in its data folder. See [Setting up from another device](ACCOUNTS.md#setting-up-from-another-device).

## 1. Welcome

![The welcome page: a button to set up a new server, a button to restore from a backup, and what the server does](images/setup-01-welcome.webp)
*The first page.*

- **Set up a new server** carries on to the next page.
- **Restore from a backup** is for moving from another computer or coming back from a reset. It brings back your accounts, libraries, what you've watched and your settings, and you won't see the rest of this guide. See [Backup and restore](BACKUP.md).

## 2. Account

![The account page: a name, a password typed twice, and a tick to say you understand](images/setup-02-account.webp)
*This account looks after the server: its libraries, people and settings.*

Choose a **name to sign in with**, and optionally the **name people see** (what this profile is called around the app), and a **password** of at least 8 characters, typed twice. **Show what I'm typing** helps on a TV. Tick the box to say that the files you add are your own responsibility, then press **Create account**. On another device, the setup code goes here too.

**Two-step sign-in (recommended)** is a tick under the password. If you tick it, the next page shows a QR code for an authenticator app (or a key to type), asks for the six digits it shows, then shows eight recovery codes, **once**: save them. Each works once in place of a code if you lose your phone. **Skip for now** leaves it off; it can be turned on later in **Settings → Security**. See [Accounts](ACCOUNTS.md).

Keep the password somewhere safe: [forgetting it is hard to undo](ACCOUNTS.md#if-you-forget-your-password).

## 3. Name and address

The name appears in every app. The editor is the same one as **Settings → Appearance → Name and logo**, and the page's corner shows your name as you change it.

- **Name:** type one, or leave it empty for “WittyMediaServer”.
- **Letters or words to colour:** the letters are a row of buttons. Use the arrows to move, **Shift** and the arrows to select a range, **Space** to select a letter, or press one of the words listed under it.
- **Colour for the selection:** a colour that **follows the theme** (text, accent, success, warning, danger), a **fixed colour** that stays the same in every theme (a swatch, **Any colour…** or a `#rrggbb`), or a **gradient** (two ready-made, or **Edit stops…** for two to four colours and a direction). Giving each letter its own colour is allowed.
- **Presets:** accent on every other word, two-tone, one colour, plain, rainbow letters and gradient.
- **Mark for the rail and small spaces:** the first letter, the initials, one or two characters of your own, or a PNG or JPEG logo up to 2 MB, always centred in a square badge. A logo that can't load falls back to the letter.
- A **Check** appears if a colour is under 4.5 to 1 against the side menu or the top bar, with the words affected. The **live preview** shows the name in the side menu, top bar, rail header, sign-in page, and on a light and a black look. Fixed colours and gradients stay as chosen when the theme or look changes; the theme-following ones change with it.

**Network and time:**

- **Time zone:** the computer's own, or any other. The TV guide's days, schedules, recordings and the logs follow it.
- **Find this server automatically** lets apps on your home network find it by name, with no address to type.
- **Encrypted connections** says whether HTTPS is on, and links to **Settings → Remote Access**, where it is set up (a certificate needs a domain name).
- The **port** the server answers on, with **Change the port**.

When the server checks the name's fixed colours against the theme and finds words under 4.5 to 1, it saves them and says **Check:** with the words and ratios; press **Continue** again to keep them, or change the colours.

## 4. Look

A look is the layout, shapes, finish and type: how pages are arranged and how they feel. Fourteen come with the server, each tried on this very page as you pick it:

| Look | In a word | Wears, unless you choose |
|---|---|---|
| Lumen | Quiet cinema | Witty |
| Rail | Library first | Cinema |
| Stage | Streaming | Midnight |
| Aero | Glossy glass | Aero Day |
| Classic | Sidebar | Witty |
| Classic Cable | Blue guide | Cable blue |
| Classic Satellite | Silver bars | Satellite |
| Compact | Dense lists | True black |
| Paper | Editorial | Paper |
| Crossbar | Console style | Evergreen |
| Marquee | Poster carousel | Cinema |
| Big picture | Easy to see | High contrast |
| Dashboard | Widgets | Midnight |
| Retro | Teletext | Teletext |

What you pick is where everyone starts, on every kind of device. People can pick their own later, unless you require one (see [People and rules](#12-people-and-rules)). Each look works on TVs, phones, tablets and computers. See [Themes](THEMES.md#a-look-is-an-experience-and-a-theme).

Beside the choices, a picture of the look you're trying shows how its menu and Home sit on a **TV**, **Desktop**, **Phone** and **Headset**, in its own colours.

**TV mode** lays this device out like the Apple TV and Android TV apps, for a screen across the room, driven by a remote or a controller. It's for this device only.

## 5. Theme

The colours. Any theme goes with any look, and the page changes as you choose. Choosing none keeps each look's own theme. Fifteen come with the server, among them Witty, Midnight, Cinema, Evergreen, True black, Light, Paper, High contrast, the four Aero themes, Cable blue, Satellite and Teletext.

**How people use it** has three choices: **Start with the look's own theme** (on until you choose a theme above, which then stays for everyone), **Let people import their own themes**, and the **WittyTV look** the guide starts in ("Follows the look" by default). **Import a theme…** installs one from a ZIP file, and you can make your own later in **Settings → Appearance**. See [Themes](THEMES.md).

## 6. Modules

Movies, TV shows and music work straight away. Modules add the rest: discs, games, photos, books, live TV and more. Each is a separate program that downloads now, signed, from the official list, so this page needs an internet connection. What you don't tick isn't downloaded.

![The list of modules, each with a tick box and what it lets you add](images/setup-04-modules-chosen.webp)
*Tick what you want. The folder browser is ticked for you, as it's how you choose a folder from a TV or a phone.*

Under the list, choose what happens when a module has a new version: tell you and let you choose, install it by itself, or look only when you ask. Then press **Download and install**.

![Modules being installed, one at a time, each saying how far it has got](images/setup-05-modules-installing.webp)
*Each module downloads, and its signature is checked before it goes in.*

![Every module ticked off as ready](images/setup-06-modules-ready.webp)
*If one fails, you can try again or carry on without it. No internet? Carry on, and add them later from Settings, Modules.*

See [Modules and the store](MODULE-STORE.md).

## 7. Libraries

Pick a kind of media, then the folder it's in. Each one becomes a library you can browse. Your files stay where they are, and nothing is copied.

![Every kind of media to choose from: movies, TV, discs, music, books, photos, games and more](images/setup-07-media.webp)
*Choose a kind. A kind you didn't add the module for isn't offered.*

![The folder for a Movies library: its name, and three ways to give the folder](images/setup-08-media-movies.webp)
*Three ways to give the folder.*

- **Choose folder…**, on the computer running the server, opens your computer's own folder window.
- **Browse folders** shows the server's drives in the page, from any device. Open the folder you want and press **Use this folder**.
- Or type the folder's full path.

Press the button that adds it, such as **Add Movies**, and it starts scanning in the background. Add as many as you like.

![The list of libraries so far, with every kind still there to add another](images/setup-09-media-added.webp)
*Add another kind, or press Continue.*

How to name your files is in [Libraries](LIBRARIES.md#how-to-lay-out-and-name-your-files). A folder on another computer has [its own steps](LIBRARIES.md#network-shares), which this page lays out if you choose one from another device.

## 8. Details and artwork

Four pages share this one step ("Step 8 of 14, part 2 of 4"). Each has **Skip for now**; the last two appear only where the server can use them.

### Posters and details

![The free sources that work with no setup, a tick for saving beside your files, and two optional keys](images/setup-12-details.webp)
*TVmaze, Wikidata, Wikipedia, AniList and libretro work with no key.*

Names taken from your files and folders are sent to these services to look things up. Your files stay on your computer.

- **Save posters and details beside your files** keeps them with your media if you ever move it. Your videos and music aren't changed. Untick it to leave your media folders exactly as they are.
- **Add a key**, for TMDB and TheTVDB, is optional, and each takes about two minutes. It's fine to do this another day.

See [Where details come from](METADATA.md).

### Logos

![What logos are, where they come from and who owns them, with a button for yes and a button for no](images/setup-14-logos.webp)
*Off unless you say yes. Nothing else changes if you say no.*

Real logos for consoles, studios and networks aren't part of this software. They download from Wikimedia Commons one at a time, only if you agree, and they're still trademarks of their owners. Choosing **No thanks, use plain names** shows names in plain text.

### Lyrics

![Two switches, both off: find lyrics online, and artist biographies and photos](images/setup-15-music.webp)
*Both are free, with no account and no key, and both stay off unless you turn them on.*

Lyrics beside your songs or inside their files work anyway, with nothing sent anywhere. These two add what they don't have, and the page says what's sent to each. See [Lyrics and karaoke](LYRICS-KARAOKE.md).

### Streaming lists

![What streaming lists do, and that nothing streams from those services, with a button to set it up and one to leave it off](images/setup-16-listings.webp)
*Off unless you choose it.*

This sorts your own titles by which streaming service lists them right now. Nothing streams from those services, and you don't need an account with any of them. To turn it on, you accept four statements and add a free TMDB key. **Leave it off and continue** is the usual choice.

## 9. Live TV

This page appears if you added WittyTuner.

**Add live TV** walks you through adding a TV tuner on your network, a provider's playlist or a single stream. It finds what it can by itself and takes the rest by hand. Each one you add is listed with how many channels it has, and their channels sit in the guide beside the ones made from your library. **Add another** adds more; **Skip for now** leaves it for later in **Settings → Channels & tuners**.

No channel lists or stream addresses come with the server: you bring your own. Encrypted channels are shown as encrypted and never decrypted. See [Channels and live TV](CHANNELS.md).

## 10. Games

This page appears if you added WittyPlay.

![The games page: a button to add a games folder, and four optional sources of details, each with a place for its key](images/setup-10-games.webp)
*Point it at your games folder and leave it.*

It works out each game's console and name by itself, and lists anything it can't prove for you to look at. No games come with the server, and nothing is downloaded for you. The sources here are optional, and each needs a free key. See [Games](GAMES.md).

## 11. Discs

This page appears if you added a disc module.

![Two choices for pressing play on a disc: show its menu, or play the movie straight away, and a switch for a disc animation](images/setup-11-discs.webp)
*The menu is still one button away if you choose to play straight away.*

Discs with copy protection can't be played. They're listed with a plain reason, and nothing is ever unlocked. See [DVDs](DVD.md) and [Blu-rays](BLURAY.md).

## 12. People and rules

Add profiles (an adult's, or a **Kids** profile that only shows titles for their age), listed with what each is. Then choose what is **required**: a look, a theme or both, of **Everyone**, of **Kids profiles**, or of one profile. A required choice is shown locked, "Required by" your name, and that person can't change it; leave a row on "Free to choose" to let them pick. The most specific requirement wins: a profile's over the kids' over everyone's. You can change all of this in **Settings → Profiles**, and under **Settings → Pages & menu → Looks**.

## 13. Remote access and apps

Says where to turn on **Remote Access** (your server works only at home until you do) and how to **pair an app** with a code or a QR code. See [Remote access](REMOTE.md).

## 14. Ready

![A summary of every answer and where to change it, the address for other devices, and buttons to add a Kids profile and start watching](images/setup-18-done.webp)
*The last page says what you chose, as the server has it now.*

- **What you chose**, each with where to change it later. Your libraries keep scanning in the background, and titles and posters appear as each one is found.
- **Watch on every screen at home:** the address other devices on your home network open, and the port it's on, with **Change the port**. If the server only answers on this computer, it says what to change. Your server works only at home until you choose a way in from away, in **Settings → Remote Access** ([Remote access](REMOTE.md)).
- **Add the family:** **Add a Kids profile** makes one that only shows titles for their age. Add more people in **Settings → Profiles**.
- **Also here when you want them:** library transcoding, TV channels made from your libraries, and live TV.

Press **Start watching** and you're in, or **Open Settings** to carry on there. Requirements for looks and themes are on [People and rules](#12-people-and-rules).

[Where next](GETTING_STARTED.md#where-next) has what to read after this.

## With a remote, a controller or the keyboard

The arrow keys, a TV remote's arrows and a controller's stick or pad reach everything, and OK (or the controller's main button) presses it. Back goes back a page. On a list of choices, the arrows only move: press OK on the one you want.

---

<sub>WittyMediaServer documentation</sub>
