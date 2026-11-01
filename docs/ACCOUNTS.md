# Accounts and profiles

This guide helps you add the people who use your server, decide what each may do, and look after passwords and sign-ins. Everyone's profile is under **Settings → Profiles**, and their own sign-ins under **Settings → Sign-in & security**.

- [Who can do what](#who-can-do-what)
- [Add a profile](#add-a-profile)
- [Limits](#limits)
- [Profile names and usernames](#profile-names-and-usernames)
- [Signing in](#signing-in)
- [Passwords](#passwords)
- [Two-factor sign-in](#two-factor-sign-in)
- [Your devices](#your-devices)
- [Signing in with a code](#signing-in-with-a-code)
- [If you forget your password](#if-you-forget-your-password)
- [Setting up from another device](#setting-up-from-another-device)
- [Remove a profile](#remove-a-profile)

## Who can do what

Every person has their own watch progress, favourites and game saves. There are four kinds of account.

### The owner, or administrator

The account made during setup is an administrator, and the app labels it **Administrator**. It has no limits and can change every setting: libraries, modules, Remote Access, backups, reset and everyone's profiles. You can make other administrators, but each one can do all of this, so keep it to people who need it.

If you use the server away from home, keep this account for changing settings, and use a second adult account for everyday watching. See [Remote access](REMOTE.md).

### Adults

A full account with its own password. An adult can:

- change their own password and names, and turn on two-factor sign-in;
- add and manage Kids profiles, unless the administrator keeps that to themselves, or has set limits on this adult;
- sign in a TV;
- upload to Photos and Vertical videos libraries where uploads are allowed, comment, make collections and photo albums, and make share links.

An adult can't open the server's settings. An administrator can set limits on an adult with **Limits**.

### Kids

Kids see only titles up to the age you choose, and new Kids profiles start at *Kids 7+*. Titles with no rating stay hidden, home videos included, until you tick **Also show titles without a rating** under **Limits**. A password or PIN is optional. Without one, they sign in with a tap, on your home network only.

Kids can like videos, subscribe to creators, keep a watch-later list and choose which of their names shows. They can't change their own names, password or limits, and they can't upload, comment, make collections, make share links or sign in a TV. Channels made from your libraries are on to begin with. Tuner and internet TV channels are off.

Age limits rely on ratings, which come from your files and from outside services, and can be missing or wrong. They're a help, not a guarantee, so please check what's suitable. Live TV, podcasts, radio, games and web content may have no rating at all.

### Guests

For visitors. A new Guest profile starts at *Teens 13+*, with no Live TV. A password or PIN is optional, and without one it works with a tap on your home network only. Guests can do what Kids can, except like videos, subscribe or keep a watch-later list. Only an administrator can change a Guest's limits.

### Who manages whom

- **Administrators** manage everyone. Nobody can set limits on an administrator.
- **Adults** manage Kids profiles, unless **Settings → Profiles → Signing in → Who may manage Kids profiles** is set to **Only the administrator**. An adult with limits of their own can't, because that would be a way around those limits.

## Add a profile

1. Go to **Settings → Profiles** and press **Add profile**. An adult sees **Add a Kids profile** when they have none yet.
2. Under **Kind of profile**, choose **Adult**, **Kids** or **Guest**. Only an administrator gets this choice.
3. Type a **Username**, and a **Profile name** if you want them called something else.
4. Type a **Password**, or for Kids and Guests, a **Password or PIN** if you want one.
5. For Kids and Guests, choose what they may watch. For an adult, tick **Administrator (can change everything)** only if you mean to, which asks for your own password.
6. Press **Add profile**.

## Limits

Press **Limits** beside someone under **Settings → Profiles** to set their age limit, the libraries they can open, genres to hide, titles to always or never allow, and their Live TV. [Hiding things, and who sees what](LIBRARIES.md#hiding-things-and-who-sees-what) explains each one.

| Age limit | What it allows |
|---|---|
| **Little kids** | G, U, TV-Y, TV-G |
| **Kids 7+** | Adds TV-Y7 |
| **Older kids 10+** | Adds PG, TV-PG |
| **Teens 13+** | Adds PG-13, 12A |
| **Older teens 16+** | Adds TV-14, 15, FSK 16 |
| **Everything** | No age limit |

## Profile names and usernames

Each person has two names.

- **Username:** what they sign in with. It has to be theirs alone, and `Sam` and `sam` count as the same name. Up to 40 characters.
- **Profile name:** what they're called around the app, such as on the *Who's watching?* screen. Anyone can have the same one. Up to 60 characters, and optional. Without one, the username is used.

**To change yours,** go to **Settings → Profiles → Your account** and press **Save names**. Adults and administrators can do this for themselves. **To change someone else's,** press **Names** beside them, if you manage them. Kids and Guests keep the names they were given.

**To choose which shows,** turn on **Show my username at the top right, not my profile name**. It's in the same place, and everyone can choose this for themselves, Kids and Guests too.

## Signing in

On your home network, the server shows *Who's watching?* with a tile for each profile. Press yours, and enter your password or PIN if it has one. **Sign in with another account** gets you a form for a username and password.

Away from home, it shows the form, so that strangers can't see who's on your server.

An administrator can change this under **Settings → Profiles → Signing in**:

| Setting | What it does |
|---|---|
| **Sign-in screen** | **Profiles** shows the tiles. **Classic** always asks for a name and password |
| **Show profiles outside your home** | Off by default. Passwords are always required away from home |
| **Let apps find this server** | Apps on your home network can see this server's name, version and a random ID. They still need a code or a password to connect |

**A profile with no password works only at home,** and only with the **Profiles** sign-in screen. With **Classic**, it can't sign in at all. To use a profile away from home, it needs a password, chosen when it's made.

**Too many wrong tries** makes the server wait before it will let you try again, and the message says how long.

## Passwords

- **Adults:** at least 8 characters, and no more than 72 bytes. Change yours under **Settings → Profiles → Your account**: type your **Current password** and **New password**, then press **Change password**. Your other devices are signed out.
- **Kids and Guests:** a password or PIN of at least 4 characters, chosen when the profile is made. Whoever manages them can tick **Remove the password** under **Limits**, so that it goes back to a tap on your home network.

Long passwords are worth it if you use the server away from home. See [Remote access](REMOTE.md).

Passwords can't be read back, by anyone. The server keeps only a scrambled form of each one, which is why a forgotten one can't be looked up.

## Two-factor sign-in

Two-factor sign-in also asks for a 6-digit code from an app on your phone, so a stolen password isn't enough. It's optional, it's for adults and administrators, and any authenticator app works.

1. Go to **Settings → Sign-in & security → Two-factor sign-in**.
2. Type your password and press **Set up**.
3. In your authenticator app, add an account and scan the code on the screen, or type the key shown.
4. Type the 6 digits it shows, and press **Turn on**. Your other devices are signed out.
5. **Save your 8 recovery codes somewhere that isn't your phone,** then press **I saved them**. They won't be shown again. Each works once in place of a code.

To turn it off, press **Turn off**, then give your password and a code.

**Lost your phone?** Enter a recovery code when it asks for a code. If those are gone as well:

- **For an adult:** an administrator can press **Turn off** beside them at **Settings → Sign-in & security → Other people's two-factor sign-in**. Do this only if you know it's really them.
- **For an administrator:** it can only be done on the server itself. Stop the server, then run this, using the username:

```bash
wittymediaserver reset-2fa -data <your data folder> <username>
```

`wittymediaserver config path` shows your data folder.

## Your devices

**Settings → Sign-in & security → Your devices** lists everywhere you're signed in, with when and where it was last used. Press **Sign out** beside one, or **Sign out the others**. If you don't recognise one, sign it out, then change your password.

## Signing in with a code

A TV or an app can be signed in without typing a password on it. An adult or administrator does this from another device: open the menu under your name and choose **Sign in a TV**. There are two ways, depending on what the TV shows.

**Your TV shows a code:**

1. Type the code under **Your TV shows a code** and press **Continue**.
2. The page says which device is asking, and from what address. Continue only if you're looking at that code on your own TV right now.
3. If you manage other profiles, choose one under **Sign it in as**.
4. Press **Sign it in**, or **Refuse**.

**Your TV asks for a code:**

1. Under **Your TV asks for a code**, choose who to sign in as, and press **Get a code**.
2. Type the code on the TV. It works once, within 5 minutes.

**Approve a code only if you started it.** Approving signs that device in as the person you chose. If someone sends you a code and asks you to approve it, say no.

The page also shows this server's fingerprint. If the app shows one, the two should match. [Apps and devices](APPS.md#signing-in) has what each app shows. The native apps don't yet ask for a two-factor code with a password, so an account with two-factor sign-in uses a code there.

## If you forget your password

There's no reset by email, because the server doesn't have your email address, and your password can't be read back.

- **A Kids or Guest profile:** an adult who manages it can tick **Remove the password** under **Limits**. It then works with a tap at home. The app has no way yet to give an existing profile a new PIN, other than removing it and adding it again.
- **An adult:** the app has no button yet for setting someone else's password. An administrator can remove the profile and add it again with the same username, but [removing it](#remove-a-profile) deletes that person's watch history.
- **The only administrator:** the way back is to reset the server. On the computer running it, run:

```bash
wittymediaserver reset
```

Then restart the server and go through first-time setup. A reset removes **every account, the list of libraries, watch progress, settings and game saves**. Your media files and what's saved beside them stay. It keeps a copy unless you add `-delete`, and `wittymediaserver reset -list`, `-restore <copy>` and `-cancel` work with those. Bringing a copy back, or restoring a backup with its accounts, also brings back the old password. See [Backup and restore](BACKUP.md).

**With a second administrator,** there's no need for that. They can remove your profile and add it again, which deletes only your watch history and game saves. This is a good reason to have one.

## Setting up from another device

Until the first account exists, whoever makes it owns the server. On the computer running the server, setup just works. From any other device, your phone included, setup asks for a **setup code**, because someone else on your network could otherwise get there first.

Find the code in any of these places:

- on the server's console when it starts, in a line that begins *This server has no accounts yet*;
- for Docker, with `docker logs wittymediaserver`;
- for a Linux service, with `journalctl -u wittymediaserver`;
- in `setup-code.txt` in the server's data folder, which is deleted once setup is done.

It's kept out of the log files. Wrong codes count, and too many makes you wait.

**If you trust everyone on your network,** you can turn this off for your home network. Set `setupFromHome` in the config file, or start the server with `-setup-from-home` or `WMS_SETUP_FROM_HOME=1`. Anyone on your network could then claim a fresh server, so the code is the safer choice. See [Config file](CONFIG.md).

Restoring a backup during setup from another device asks for the same code.

## Remove a profile

Press the bin beside someone under **Settings → Profiles**. Their watch history and game saves are deleted too, and this can't be undone. Removing another administrator also asks for your password. You can't remove the account you're signed in to.

---

<sub>WittyMediaServer documentation</sub>
