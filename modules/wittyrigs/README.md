<h1 align="center">WittyRigs</h1>

<p align="center">
  <b>An advanced option: host demanding PC games on your server's own hardware.</b>
</p>

> **Not an official project.** WittyRigs isn't part of, or made by, the [Sunshine](https://github.com/LizardByte/Sunshine)
> project (LizardByte). It contains none of its code.

> Read-only mirror of `modules/wittyrigs` in [WittyMediaServer](https://github.com/wittywilla/WittyMediaServer),
> where development happens. Please open issues and pull requests there.

## What it does

Most servers don't need this. To play on other PCs, a friend's included, that's [WittyCouch](https://github.com/wittywilla/WittyCouch).

For a server with a good graphics card and PC games installed on it, this finds a separately installed Sunshine on that machine, so its games can
become a library. It's off until an admin turns it on. People play through WittyCouch, as with any other PC.

## Licence

Apache-2.0: see [LICENSE](LICENSE) and [NOTICE](NOTICE). Sunshine is GPL-3.0 and stays a separate program, which this only looks for and talks to over the network.

## Legal

WittyRigs is an independent open-source project. It isn't associated with, sponsored by or endorsed by
the Sunshine or Moonlight projects, or any game publisher. Sunshine is a separate program you install
yourself, under its own license, and no games come with this. You're responsible for what you host and
who you let play it. Any third-party names here only describe what it works with, and belong to their
owners. It's provided “as is”, with no warranty; see [LICENSE](LICENSE). Local law applies to how you use
it. This isn't legal advice.

<!-- module-kit -->
## Releasing

WittyRigs reaches a server inside the projects built with it, so a release has no files of its own: a version tag runs the tests and publishes a release that those projects can require.

Versions move in small steps as work lands. `go run github.com/wittywilla/wittymodule/cmd/wittymodule@v0.2.1 bump .` raises it
everywhere it is written (add `-part minor` for a bigger step). A bump isn't a release: nothing is
tagged or published until you tag it. CI fails when code changed since the last tag and the version
didn't, because tags never move.

To release:

1. Check that `version` in `module.json` is the one to publish, with the bump committed.
2. Tag that commit and push the tag: `git tag vX.Y.Z`, then `git push origin vX.Y.Z`. A tag with a
   suffix, such as `v1.2.0-beta`, is published as a prerelease.
3. Watch the *Actions* tab. There is nothing to sign.

Licensed under Apache-2.0: see [LICENSE](LICENSE) and [NOTICE](NOTICE).
<!-- /module-kit -->
