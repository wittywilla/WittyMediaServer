# Security policy

WittyMediaServer runs on people's own machines and holds their accounts and their media, so
security reports are welcome and taken seriously.

## Reporting a vulnerability

**Please don't open a public issue.** Report it privately through GitHub:

1. Open the [Security tab](https://github.com/wittywilla/WittyMediaServer/security) of this
   repository.
2. Choose **Report a vulnerability**.

That starts a private conversation with the maintainer, visible to nobody else until a fix is out.

It helps to include which version you tested, how to reproduce the problem, and what someone could
do with it. Please test only on servers you own, and don't touch other people's data or services.

## What to expect

This is a project of one maintainer, so these are aims, not promises:

- a first answer within a week;
- a fix released as soon as it is ready, sooner for anything that exposes accounts or files;
- credit in the changelog, if you'd like it.

## Supported versions

Only the newest release gets security fixes. Servers can update themselves from
*Settings → About*; every update is checked against the project's signing key before it is
installed.

## What counts

Examples: reaching accounts, files or settings you shouldn't; running code on the server; getting
around the limits on Kids, Guest or restricted accounts; a crafted media file, disc image, book or
podcast feed that crashes the server or reads other files; anything that weakens the update chain.

What the administrator can do by design is not a vulnerability: choosing any folder on the machine
as a library, for example. Getting past the limits of that role is: an administrator's account
should not be able to read arbitrary files or run programs through FFmpeg settings.

## Running it safely

- Keep the server on your home network, or put it behind HTTPS (see *docs/INSTALL.md*). Without
  HTTPS, passwords and sessions cross the network unencrypted.
- Finish first-time setup before exposing the server. From another network, setup asks for the
  code the server writes to its log.
- Use a long password for the administrator account.

The most recent review, its findings and what is still open are in
[docs/SECURITY-REVIEW.md](docs/SECURITY-REVIEW.md).
