<h1 align="center">WittySunshine</h1>

<p align="center">
  <b>Find, pair with and manage a gaming PC running Sunshine, from Go.</b>
</p>

> **Not an official project.** WittySunshine is not part of, or made by, the
> [Sunshine](https://github.com/LizardByte/Sunshine) project (LizardByte) or the
> [Moonlight](https://moonlight-stream.org) project. It contains none of their code. It talks to a separately
> installed Sunshine over its network protocols.

> Read-only mirror of `modules/wittysunshine` in [WittyMediaServer](https://github.com/wittywilla/WittyMediaServer),
> where development happens. Please open issues and pull requests there.

## What it does

| Part | What for |
|---|---|
| `Browse` | Finds PCs on the home network over mDNS (`_nvstream._tcp`) |
| `Rig.Hello`, `Rig.Status` | Who is this PC, and how is it doing. `Status` also says its MAC address, and proves it's still the same PC |
| `Rig.Pair` | The PIN flow. Returns the PC's certificate, ready to pin |
| `Rig.Apps`, `Rig.AppImage` | What a PC offers, and box art |
| `Rig.Launch`, `Rig.Quit` | Starts and ends an app on the PC |
| `Rig.SubmitPIN` | Types a PIN into Sunshine for another device, sent only to the pinned certificate |
| `Wake` | Wake-on-LAN |
| `CleanAddress`, `CheckAddress`, `Policy` | Addresses: what may be typed, and what may be connected to, checked again at connect time |
| `fake` | A stand-in for Sunshine for tests, with ways to misbehave |

Every failure is one a person can act on: `ErrRefused` means it's on but Sunshine isn't running, `ErrNoAnswer` asleep or off,
`ErrNotPaired` it no longer trusts this client, `ErrCertChanged` something else answered.

```go
id, _ := wittysunshine.LoadOrCreateIdentity(dir)
rig := &wittysunshine.Rig{Address: "192.168.1.20", Identity: id}
pin, _ := wittysunshine.NewPIN() // show this to the person, who types it into Sunshine
cert, err := rig.Pair(ctx, pin)  // waits for them
rig.Cert = cert                  // pin it, and keep it
apps, err := rig.Apps(ctx)
```

## How it has been tested

Automated tests run against `fake`. `real_test.go` runs against a real Sunshine when one is named, and has been run against
v2026.914.233613 in a Linux container. Real discovery and waking on a real network haven't been tried yet.

## Licence

Apache-2.0: see [LICENSE](LICENSE) and [NOTICE](NOTICE). Sunshine itself is GPL-3.0, and stays a separate program.
