# Security policy

How to report a security problem in WittyMediaServer, and what to expect. It runs on people's own machines and holds their accounts and their media, so reports are welcome and taken seriously.

## Reporting a vulnerability

**Please don't put the details in a public issue.** Report it privately through GitHub:

1. Open the [Security tab](https://github.com/wittywilla/WittyMediaServer/security) of this repository.
2. Choose **Report a vulnerability**.

That starts a private conversation with the maintainer, visible to nobody else until a fix is out. If you don't see that button, open an issue that says only that you have a security report, with no details, and ask for a private way to send it.

It helps to include which version you tested, how to reproduce the problem, and what someone could do with it. Please test only on servers you own, and don't touch other people's data or services.

**A problem in a module** can be reported here too, if you aren't sure which repository it belongs in. A module from another publisher or another store is that publisher's to fix, though a way for one to break out of its limits is a problem here.

## What to expect

This is a project of one maintainer, so these are aims, not promises:

- a first answer within a week;
- a fix released as soon as it is ready, sooner for anything that exposes accounts or files;
- credit in the changelog, if you'd like it.

## Supported versions

Only the newest release gets security fixes. This is alpha software.

Servers can update themselves from **Settings → About**, and every update is checked against the project's signing key before it's installed. A release found to be unsafe is withdrawn: servers stop offering it, and one running it says so on that page.

Modules update on their own, from a signed catalogue, under **Settings → Modules**. A version can be withdrawn in the same way.

## What counts

Examples:

- reaching accounts, files or settings you shouldn't;
- running code on the server;
- getting around the limits on Kids, Guest or limited accounts;
- a crafted media file, disc image, book, game file, backup or podcast feed that crashes the server or reads other files;
- anything that weakens the update chain, for the server or for modules;
- a module, helper or game core getting out of its sandbox.

What an administrator can do by design is not a vulnerability: choosing any folder on the machine as a library, for example. Getting past the limits of that role is. An administrator's web session should not be able to read arbitrary files or run programs on the machine, or make itself a second administrator without the password.

What the owner chooses to trust is also outside this: a store run by someone else, a fork of a module, a custom game core or a reverse proxy listed as trusted.

## Running it safely

- Keep the server on your home network, or use [a private network](docs/REMOTE.md#the-recommended-way-a-private-network) to reach it from elsewhere. Without HTTPS or a VPN, passwords and sessions cross the network unencrypted.
- Finish first-time setup before exposing the server. From anywhere but the computer it runs on, setup asks for a code found on the server's console and in `setup-code.txt` in its data folder.
- Use a long password for the administrator account, with [two-factor sign-in](docs/ACCOUNTS.md#two-factor-sign-in).
- Add only modules you need. On Linux and Windows a module's helper runs in a sandbox. On FreeBSD it has limits rather than a sandbox, and macOS's hasn't been run on a real Mac yet. Settings, Modules says what yours has.

Who the server is defended against, and how that gets checked, is in the [security model](docs/SECURITY-REVIEW.md).
