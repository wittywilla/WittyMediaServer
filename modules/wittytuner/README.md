<h1 align="center">WittyTuner</h1>

<p align="center">
  <b>One way to reach every kind of TV tuner, from Go.</b><br>
  Network tuners, IPTV, Linux DVB, capture cards, SAT&gt;IP, external backends and a custom command.
</p>

<p align="center">
  <a href="LICENSE"><img alt="Licence: Apache-2.0" src="https://img.shields.io/badge/licence-Apache--2.0-blue.svg"></a>
  <img alt="Go 1.26+" src="https://img.shields.io/badge/Go-1.26%2B-00ADD8.svg">
  <img alt="No dependencies" src="https://img.shields.io/badge/dependencies-none-success.svg">
</p>

> Read-only mirror of `modules/wittytuner` in [WittyMediaServer](https://github.com/wittywilla/WittyMediaServer),
> where development happens. Please open issues and pull requests there.

## What it does

A driver lists channels and opens one, as raw MPEG-TS or as FFmpeg input options.

| Driver | Reaches | Platforms | How it has been tested |
|---|---|---|---|
| `iptv` | M3U and M3U8 playlists (`tvg-*`, `group-title`, `url-tvg`) | all | Automated tests, and FFmpeg against local streams |
| `stream` | One HTTP, HLS, RTSP, RTP, UDP, RTMP or SRT address | all | Automated tests, and FFmpeg against local streams |
| `network` | HDHomeRun-style boxes: UDP and SSDP discovery, `discover.json`, `lineup.json`, the scan API, MPEG-TS over HTTP | all | Against a mock device. **Not yet on real hardware** |
| `backend` | TV servers over open protocols: an HTTP API with M3U and XMLTV, or SVDRP | all | Against stand-in servers. **Not yet on a real server** |
| `satip` | SAT>IP servers: SSDP, RTSP tuning | all | Protocol tests. **Untested on hardware** |
| `dvb` | Linux DVB-T/T2, DVB-C, DVB-S/S2, ATSC, ClearQAM, ISDB-T; band-plan scans; `channels.conf` import | Linux | Parsers and band plans tested. **Untested on hardware.** No DiSEqC yet |
| `v4l2` | Analogue tuner and capture cards | Linux | Argument tests. **Untested on hardware** |
| `dshow` | Windows capture cards through DirectShow | Windows | Listing tested. **Untested with a card** |
| `avfoundation` | macOS capture devices | macOS | Parser tests. **Untested** |
| `command` | Your own program writing MPEG-TS | all | Automated tests with a test program |
| `bda` | Windows digital tuners through BDA | | **Not built.** Listed so the gap is visible |

It also reads what is in a stream: PAT, PMT, SDT, ATSC VCT, and guide data from DVB EIT and ATSC PSIP
(`mpegts`), plus XMLTV files as they stream by (`xmltv`).

## Encrypted and copy-protected channels

**Nothing here decrypts or bypasses anything.** Protected channels are detected and refused with a
clear message:

- a `drm` tag or `DRM: 1` in a network tuner's lineup, and its "content protection required" error;
- DTCP-IP streams, and DASH manifests with `ContentProtection`;
- `free_CA_mode` in the SDT, CA descriptors in the PMT, `access_controlled` in the ATSC VCT;
- `transport_scrambling_control` in the packets themselves, also mid-stream (`mpegts.Guard`);
- DRM licence lines in a playlist, and any `#EXT-X-KEY` in an HLS playlist. FFmpeg's `crypto`
  protocol is left off its whitelist as a second lock.

`wittytuner.New` applies the check to every driver, so none can leave it out, and there is no hook for
changing it. Channels are `Checked` when their source says whether they are protected, and ATSC 3.0 is an
ordinary channel when it isn't encrypted (the host decides what it can decode).

No channel lists or stream addresses are included. Scans use the standard channel-to-frequency
formulas, which say where a channel could be, not what is there.

## Use

```go
env := wittytuner.Env{FFmpeg: "ffmpeg"}
src, err := wittytuner.New("network", wittytuner.Config{
    Name:    "Living room",
    Options: map[string]string{"address": "192.168.1.50"},
}, env)
if err != nil { return err }

channels, err := src.Channels(ctx)
if err != nil { return err }

stream, err := src.Open(ctx, channels[0])
if err != nil { return err } // wittytuner.ErrBusy, *wittytuner.EncryptedError
defer stream.Close()
// stream.Reader is MPEG-TS, or stream.Input are options for FFmpeg.
```

`wittytuner.Drivers(env)` describes every driver and its settings, so a form can be drawn from it.
`wittytuner.Discover(ctx, env)` looks for tuners on the network and on this machine.

The custom command driver runs a program, so it stays off unless the host sets
`Env.AllowCommands` (WittyMediaServer: the `WMS_TUNER_COMMANDS=1` environment variable).

## Licence

Apache-2.0. See [LICENSE](LICENSE) and [NOTICE](NOTICE). Developed by WittyWilla as part of
[WittyMediaServer](https://github.com/wittywilla/WittyMediaServer).

## Legal

WittyTuner is an independent open-source project. It isn't associated with, sponsored by or endorsed
by any tuner maker, broadcaster, IPTV provider or guide service. No channel lists or stream
addresses come with it. Encrypted channels are refused and nothing is decrypted. What a tuner or a
provider offers belongs to them, and you're responsible for having the rights to what you receive
and record. Any third-party names here only describe what it works with, and belong to their owners.
It's provided “as is”, with no warranty; see [LICENSE](LICENSE). Local law applies to how you use
it. This isn't legal advice.

<!-- module-kit -->
## Releasing

A version tag builds a program for every platform, signs the checksum list, and publishes a release that servers can install.

Versions move in small steps as work lands. `go run github.com/wittywilla/wittymodule/cmd/wittymodule@v0.2.1 bump .` raises it
everywhere it is written (add `-part minor` for a bigger step). A bump isn't a release: nothing is
tagged or published until you tag it. CI fails when code changed since the last tag and the version
didn't, because tags never move.

To release:

1. Check that `version` in `module.json` is the one to publish, with the bump committed.
2. Tag that commit and push the tag: `git tag vX.Y.Z`, then `git push origin vX.Y.Z`. A tag with a
   suffix, such as `v1.2.0-beta`, is published as a prerelease.
3. Watch the *Actions* tab. The signing step uses the `WMS_SIGNING_KEY` secret of the `release` environment, and nothing is published without it.

To take a version back, run the *withdraw* workflow from the *Actions* tab. It adds the version to a signed list on the release tagged `withdrawals`, which servers read.

Licensed under Apache-2.0: see [LICENSE](LICENSE) and [NOTICE](NOTICE).
<!-- /module-kit -->
