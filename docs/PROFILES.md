# Profiles and parental controls

This guide is for a household sharing one server: a TV everyone uses, a tablet that changes hands, children who should see less and for less time. It covers switching profiles on a shared screen, each profile's picture, what a profile may see and do, screen time, and what a parent can look back on. Making profiles, passwords and signing in are in [Accounts and profiles](ACCOUNTS.md).

- [How profiles work](#how-profiles-work)
- [Switching profile](#switching-profile)
- [A picture for each profile](#a-picture-for-each-profile)
- [What a profile sees](#what-a-profile-sees)
- [What a profile can do](#what-a-profile-can-do)
- [Screen time](#screen-time)
- [Asking a parent for more time](#asking-a-parent-for-more-time)
- [What a parent can see](#what-a-parent-can-see)
- [What limits can't do](#what-limits-cant-do)
- [For app makers](#for-app-makers)
- [Why it is built this way](#why-it-is-built-this-way)

## How profiles work

A profile is an account. Each person on your server has one, with its own watch progress, history, lists, likes, recommendations, game saves, name and picture. The people on one server are one household: there's nothing else to set up to group them.

Limits are set by whoever manages a profile. An administrator manages everyone. An adult manages Kids profiles, unless the administrator keeps that to themselves, and unless that adult has limits of their own. See [Who manages whom](ACCOUNTS.md#who-manages-whom).

**Every limit is applied by the server.** An app only shows what the server gives it, so a limit holds in the web app, the TV and phone apps, and for anything else that talks to the server.

## Switching profile

On a device that's signed in, open the menu under your name and choose **Switch profile**. *Who's watching?* lists everyone. Press a profile:

- **With a password or PIN:** type it, and press **Switch**. An account with two-factor sign-in asks for its code as well.
- **With none:** it switches with a tap. This works on your home network only, as signing in with a tap does.

The device is then signed in to that profile and no longer to the one before. To go back, the first profile's password is needed again, so a child handed the remote can't return to a parent's profile.

Wrong tries are counted as they are at the sign-in screen: after a few, the server makes you wait, and says how long.

Where the server doesn't show its profiles to a device (the **Classic** sign-in screen, or away from home unless you allowed it), **Switch profile** signs out, and you sign in again. Adults see the list anywhere.

## A picture for each profile

Each profile shows its initial on a colour, or a picture chosen from a small set.

- **Yours:** **Settings → Profiles → Your account → Change your picture**.
- **Someone else's:** **Name and picture** beside them, if you manage them.

Kids and Guest profiles keep the name and picture they were given, unless you turn on **Change their own name and picture** under **Limits → What they can do**. Their username and password always stay with whoever manages them. Pictures are drawn by the apps: nothing is uploaded.

## What a profile sees

Press **Limits** beside someone under **Settings → Profiles**, and stay on **What they see**.

| Limit | What it does |
|---|---|
| **What may they watch?** | One age limit for everything, from *Little kids* to *Everything* |
| **Also show titles without a rating** | Home videos and unmatched files, which otherwise stay hidden under an age limit |
| **A different age for some kinds** | A limit of its own for films and videos, TV shows, games, music and audio, or books. Each shows what it allows in each country's ratings |
| **Libraries** | Only the ticked libraries. Leave all unticked for every library |
| **Hide genres** | Titles of these genres are hidden |
| **Show only these genres** | When any is ticked, only titles with one of them |
| **Collections** | **Always allow** or **Never allow** everything in a collection, whatever its rating |
| **Hide videos with these tags** | For video libraries whose videos have tags |
| **Exceptions** | **Always allow** or **Never allow** one film or show |

**Never allow wins.** Then **Always allow**. Then everything else has to pass: the library, the genres, the tags and the age.

### Ratings

A limit is kept as an age, because every country's ratings can be turned into one. A title's rating is read when it's added, whichever board gave it:

| Kind | Ratings understood |
|---|---|
| Films | MPA (G, PG, PG-13, R, NC-17), BBFC (U, PG, 12A, 12, 15, 18), FSK (0, 6, 12, 16, 18), Kijkwijzer (AL, 6, 9, 12, 14, 16, 18), and others written as an age, such as `16+` or `NL:12` |
| TV | TV-Y, TV-G, TV-Y7, TV-PG, TV-14, TV-MA, and the film ratings above |
| Games | ESRB (E, E10+, T, M, AO), PEGI (3, 7, 12, 16, 18), USK, CERO, ACB, GRAC, ClassInd. Where boards disagree, the strictest counts |
| Music | *Explicit* counts as 17 and *Clean* as for everyone, for a song whose rating says so. Explicit podcast episodes are rated TV-MA |
| Books | Any of the above, or a plain age |

A collection made from a rule can't be used as a limit, because it has no list of titles: it holds whatever matches today. Add titles to a collection by hand to use it.

### Hidden means hidden everywhere

A title outside someone's limits doesn't show in lists, search, Home, *Continue watching*, recommendations, collections, counts or *Your year*, even if they watched it before the limit was set. Asking for it by its address answers "not found", as do its pictures, its file and its details. A share link they made to it stops working.

## What a profile can do

Under **Limits → What they can do**:

| Switch | When it's off |
|---|---|
| **Games** | No games, here or on a game PC, and game libraries aren't listed |
| **Recording Live TV** | No recordings or recording rules |
| **Downloads** | No saving a copy of a photo or video |
| **Share links** | No new links, and the ones they made stop working. Only adult accounts can make links at all |
| **Comments** | No comments shown, and none written. Kids and Guest profiles can read comments when this is on, and never write them |
| **Watch parties** | No starting a watch party and no joining one, whatever the server's own setting for who may start one says. A seat they had in one goes the moment this is turned off |
| **Playing on other devices** | No list of devices, no **Play on…**, no steering any device, their own included. Their own devices can still be played on and steered by whoever is allowed to, such as a parent |
| **Use away from home** | The profile works on your home network only. Anywhere else it's signed out, and can't sign in |
| **Change their own name and picture** | Their name and picture are for whoever manages them |
| **Live TV** | No Live TV, or only your own channels, or only tuner and internet channels |

Installing modules, changing settings and managing libraries are always for administrators, so there's no switch for them. Signing in a TV is always for adults.

An adult with any limit at all, these included, can't manage Kids profiles. Otherwise they could make a profile with no limits and use that.

**Share links** and **Comments** say nothing about watch parties. Inviting someone to a watch party isn't a share link: it reaches only people with an account here, and gives them nothing their own limits don't. Watch parties have no chat.

## Limits everywhere else

The same limits hold in every part of the server, because every part asks the same question in the same place. What that means where features meet:

**The apps' screens.** The TV and phone apps draw screens the server sends them ([SDUI.md](SDUI.md)). A screen sent to a profile holds only titles that profile may see, by every kind of limit. What it has been kept from isn't offered either: with games off there's no games library and nothing of WittyPlay; with Live TV off, no channels and nothing of WittyTV; with recordings, watch parties or playing on other devices off, no button for them. When screen time is over, screens are sent without **Play** and **Resume**, and the player's own screen is refused the way playing is.

**Playing on another device.** Two sets of limits apply to every request, and both must allow it:

- *The device's profile decides what plays on it*, whoever asks. Nobody can put on a Kids profile's TV what that profile may not see, an administrator included. When that profile's screen time is over, nobody can start anything there. Pausing, stopping and the volume still work.
- *The person asking is held to their own limits.* They can't ask any device for a title they may not see, and when their own screen time is over they can start nothing anywhere, though they can still pause and stop.

What plays on a device counts as the screen time of the profile that device is signed in as, whoever started it. A device playing something you may not see shows you only that it's playing.

**Watch parties.** Each person's limits are asked for everything in a party's queue. Someone who may not see what's playing can't join. If the party moves on to something a member may not see, or their limits change during it, they keep their place, see no title and can't control anything. When a member's screen time runs out, they sit out: the party doesn't wait for them, and they're back the moment a parent gives more time. Nobody starts or joins a party when their time is up.

**Games in a standalone emulator.** The time counts as screen time while the game runs, and the game is ended when the time is up or games are turned off for the profile.

**Switching profile and signing out.** A device that switches profile, or signs out, stops being the profile before at once: it's no longer listed as that profile's device, it gives up its seat in a watch party, and a game it was running in an emulator is ended. It then appears as the new profile's device, with the new profile's limits. The same happens when a device is signed out from **Sign-in & security**, when a password changes, and when a profile is removed.

**Signing in a TV.** An adult signs a TV in as themselves, and the TV then has their limits. An adult with no limits can sign it in as a Kids profile they manage. An adult with limits can't, and a Kids or Guest profile signs in no device. Signing a device in as an administrator asks for that administrator's password again.

## Screen time

Under **Limits → Screen time**:

| Setting | What it does |
|---|---|
| **School days** | How long they may play each day, Monday to Friday |
| **Weekends** | The same for Saturday and Sunday |
| **Not before** | The earliest time of day. Left empty with a bedtime set, the day starts at 6:00 |
| **Bedtime on school nights** | When playing stops on Sunday to Thursday evenings |
| **Bedtime on Fridays and Saturdays** | The same for the other two |

**How time is counted.** The server counts, by its own clock, while the profile is playing something: video, music, radio, Live TV, discs and games. It goes by the requests that playing makes, not by what an app says, so it can't be reset from a device. Two devices playing at once on one profile count once. A pause of more than a minute and a half stops the count. Clearing their own watch history gives no time back. The day is the server's day, and starts at midnight.

**When it runs out.** Whatever is playing stops, and the profile is told why and when it can watch again. A few minutes before, a line at the foot of the page says how long is left. Browsing still works, and their place in what they were watching is kept.

The server doesn't know school holidays. Change the settings when they start.

## Asking a parent for more time

When time is up, the notice has **Ask a parent**. A parent chooses 15 minutes, 30 minutes or an hour, and types either:

- **the parents' PIN**, which an administrator sets under **Settings → Profiles → Parents' PIN**. It's 4 to 12 digits, and all it can do is give time, so it can be short. It's kept scrambled, as passwords are; or
- **their own username and password**, if they manage that profile.

The extra time is for today. It adds to the day's allowance, and for that long from now, bedtime doesn't apply.

Wrong PINs are counted for the profile and for the device. After five, the server makes everyone wait before another try, the right PIN included, and the wait grows.

A parent can also give time from their own device: **Settings → Profiles**, **Report** beside the profile, then a button under **Today**.

## What a parent can see

**Report** beside a profile you manage shows the last two weeks:

- **Today:** how much time is used, and buttons to give more.
- **What they watched:** each day's titles and for how long, and the day's screen time.
- **What was refused, and extra time:** a hidden title they asked for by its address, playing when time was up, something that's turned off, and each time more time was given and by whom.

This is kept only for profiles with limits, and for 400 days. The profile can't see or clear it. **Clear** forgets it, except today's screen time. An adult nobody has limited has no such record: their history is their own, and nobody else can read it.

## What limits can't do

- **Ratings can be missing or wrong.** They come from your files and from outside services. Limits are a help, not a guarantee.
- **Music is rarely rated.** Song files seldom say whether they're explicit, and the server doesn't read that from a file's tags yet. Under an age limit, unrated music is hidden unless you allow unrated titles, for everything or for **Music and audio** alone.
- **Podcasts, radio stations and Live TV programmes** have no ratings of their own. Limit them by library, or with the Live TV switches.
- **Downloads.** The switch stops the routes that hand over a file as a download. A file that an app plays directly is sent the same way whether the app plays it or keeps it, so an app that keeps files for watching offline has to honour the switch itself (`features.downloads` in `GET /me/limits`).
- **A game played in the browser** runs on the device once it has loaded. The web app checks the time each minute and puts the notice over the game, and the server refuses to start another, but it can't reach into a game already running on a device that has gone offline.
- **Reading** isn't counted as screen time, except a book that's opened as a whole file.
- **A collection used as a limit** is looked up again every 15 seconds. A title added to a hidden collection can be opened by its address for that long. Lists follow at once. A watch party that is already going asks again about every half minute.
- **A game in a standalone emulator** is looked at every 20 seconds, so it can run that long past the end of screen time.
- **A device that is already playing** when its profile's limits change isn't sent a stop. Its next request for more of the video is refused, which ends it within seconds.

## For app makers

Everything is under `/api/v1`. [CLIENT-API.md](CLIENT-API.md#sign-in-and-accounts) lists each route, and `features` in `/capabilities` has `profiles`, `profile-switching`, `profile-avatars`, `parental-controls` and `screen-time`.

| Route | For |
|---|---|
| `GET /profiles/household` | The profiles to offer for switching |
| `POST /profiles/switch` | Switch: `{name, password, code, device}`, as `/login`. A new `token`; the old one ends |
| `GET /me/limits` | What this profile may do, for hiding what's off, and its time rules. `features` includes `syncplay` and `remoteControl` |
| `GET /me/screen-time` | Time left now. Ask once a minute while playing, and at `until` |
| `POST /me/screen-time/extra` | Ask a parent: `{minutes, pin}` or `{minutes, name, password}` |
| `GET /parental/ratings` | Rating systems, kinds, features and avatars, for a settings page |
| `PUT /users/{id}/access` | Save limits. What isn't sent stays as it was, so an older app doesn't clear a newer limit |
| `GET /users/{id}/activity` | The report |
| `POST /users/{id}/screen-time/extra` | Give time from a parent's device |
| `GET`, `PUT /parental/pin` | Whether the parents' PIN is set, and setting it. Administrators |

**What an app has to handle:**

- **403 with `"code": "screen-time"`** from anything that plays: stop, and show `error`. `screenTime` in the answer says why and until when. Every answer to a profile with time rules, from a route that plays, also has `X-WMS-Screen-Time-Left` in seconds.
- **403 with `"code": "feature-off"`** and `feature`: that part is turned off for this profile. Use `/me/limits` to not offer it in the first place.
- **A stream that ends early.** When time runs out, the server stops sending. Ask `/me/screen-time` before treating it as a network fault.
- **401 away from home** for a profile kept to the home network.

Limits in `access` beyond the older ones: `kinds` (`{"game": {"maxAge": 12, "allowUnrated": false}}`, for `film`, `tv`, `game`, `music`, `book`), `onlyGenres`, `blockTags`, `allowCollections`, `blockCollections`, `off` (any of `games`, `recordings`, `downloads`, `sharing`, `comments`, `remote`, `editProfile`), and `time` (`schoolDayMinutes`, `weekendMinutes`, `from`, `schoolNightUntil`, `weekendUntil`, with times as `"20:30"`).

## Why it is built this way

**Profiles are accounts, not something inside an account.** The server already gave each person an account with their own progress, history, lists and saves, and every route already decides what to show by the account asking. Profiles under one shared sign-in would have needed all of that a second time, keyed by a profile inside the account, in every one of those routes, with a second set of rules for who may read whose. So a household is the accounts on a server, and what a shared screen needs is added on top: the list of them, and a way to change from one to another that is signing in as the other and signing out of the first in one step. Sign-ins, the list of signed-in devices, the log, and every "own data" rule keep meaning what they meant.

**One place for each kind of limit.** Which titles an account sees is `library.Access`, which each list applies in its SQL and each single title checks in Go, as before; the newer limits were added to it, not beside it. What an account may do, and when it may play, is decided once, before any handler runs, from the route being called (`classify` in `internal/server/parental.go`). A handler can't forget a check it doesn't make.

**Tests that walk the routes.** `TestLimitedProfilesNeverSeeHiddenTitles` makes a library where everything hidden has one made-up word in its name, genre, studio and file, gives a profile each kind of limit and a past with those titles, and calls every route a signed-in account can reach, with every hidden title's number and with the word searched for. The word must never come back. It takes its routes from the list every route has to be in, so a new route is walked without anyone remembering. `TestEveryRouteIsClassed` fails on a route nobody has classed as playing something, part of a feature, or neither.

---

<sub>WittyMediaServer documentation</sub>
