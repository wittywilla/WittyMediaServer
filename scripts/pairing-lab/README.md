# Pairing lab

Pairing a phone with a server **from away**, the first time, across the networks people really have, by
an invite link (what the QR code holds) and by a code, with every step on both sides written down with its
time. It exists because pairing from another network failed for the owner on 0.8.4-alpha with the app
0.6.2, and the unit tests and smoke tests passed.

Everything runs on one Linux machine (or WSL), in network namespaces, with nothing reaching the real
internet except, for joining by a code, the public address book (DHT) through this machine's NAT.

## What runs

- **The server** (`wittymediaserver`), built from the checkout under test, set up the way the owner sets
  it up: an administrator, then Settings, Remote Access: peer to peer on, the owner's own WittyRelay with its
  key, "Only paired devices" on or off.
- **WittyRelay**, the owner-style relay, on the pretend internet.
- **The phone**: either
  - `labphone` (default): `pkg/wittyconnect/mobile`, the library the Android app embeds, driven in the
    order the Android app drives it (`PeerManager.join`/`joinByCode` and `App.joinedFromAway`). `PL_ORDER`
    picks the app: `0.6.3` (the default: start the connection and show the invite at once) or `0.6.2` (start
    the connection, wait up to 45 s for a path, and only then show the invite). Then: prove the server's identity
    (`POST /api/v1/identity`), redeem the sign-in code (`POST /api/v1/pair/redeem`), use it, then close and
    open again from what was kept (the next launch), up to 25 s.
  - the real app (`PL_PHONE=emulator`): an APK on an Android emulator started **inside** the phone's
    namespace, so all its traffic crosses the phone's router. `PL_APK` is the app; `PL_OLD_APK` the one an
    upgrade starts from.

## The matrix

A case is `network.server.only.phone.way`, for example `symmetric-relay.restart.on.fresh.link`.

| part | values |
|---|---|
| network | `direct` (the server on a public address), `fullcone` (the server's port forwarded), `cone-relay`, `ipv4-only`, `symmetric-relay`, `cgnat-relay` (the server behind a provider's CGNAT), `udp-blocked` (the phone's network lets no UDP out: TCP only), `ipv6-only` (no IPv4 at all; home routers are firewalls without NAT), `docker-home` (the server in a container on Docker's bridge behind its host, web and peer-to-peer ports published, the phone on the same Wi-Fi, and only an ordinary public relay: issue #683) and `docker-away` (the same server, the phone away, the owner's relay). Every other network but `direct` has the owner's own WittyRelay. `net.sh` has the details. |
| server | `fresh`: the invite is made as soon as the server answers, before it holds a place on the relay. `settled`: the server has its relay place. `restart`: the server restarts between the invite (or the code shown) and the claim. |
| only | "Only paired devices" `on` or `off`. |
| phone | `fresh`: a new install. `upgrade`: an older app's leftovers (its key, the local port it used, addresses kept for this server from before it restarted on new ports); on the emulator, the old APK signed in at home, then updated. `previous`: paired with another server first, whose connection is still open when this one starts. |
| way | `link` or `code`. |

`PL_KEEP=library` keeps only what the library remembers (`KnownAddrs`) for next time, without the app's own rules, to test the library itself.

The link is what Connect a device's QR code holds (`qrLink`); `PL_LINK=plain` uses the link to paste instead.

Sets: `fast` (a dozen cases, every pull request), `full` (everything, nightly and before a release tag),
or filters such as `net=symmetric-relay way=link phone=upgrade`.

## Running it

```sh
scripts/pairing-lab/lab.sh build /tmp/plab                       # server from this checkout, WittyRelay main, labphone
sudo -E scripts/pairing-lab/lab.sh run /tmp/plab /tmp/plab-out fast
sudo -E scripts/pairing-lab/lab.sh run /tmp/plab /tmp/plab-out symmetric-relay.restart.on.fresh.link
scripts/pairing-lab/lab.sh list net=ipv6-only                     # what a set or filter holds
```

`lab.sh build <out> <server checkout> <WittyRelay checkout>` builds other versions, for example a release
tag, to see what it does. With an emulator:

```sh
sudo PL_PHONE=emulator PL_APK=app-0.6.2.apk PL_OLD_APK=app-0.6.0.apk PL_AVD=plab-phone PL_EMU_PORT=5770 \
  PL_CASE_TIMEOUT=900 SUDO_USER=$USER scripts/pairing-lab/lab.sh run /tmp/plab /tmp/plab-out cone-relay.settled.on.upgrade.link
```

Cases run one at a time (they share namespace names `pl-*`). They never touch other servers on the
machine: the server runs in its own namespace on ports 19890 to 19893 and is stopped by its own PID.

## Reading the results

Each case leaves a folder: `steps.jsonl` (both sides' steps on one clock: `t` in milliseconds since the case
began, `side` phone, server or prep, `step`, `ok`, `detail`), `result.json` (the outcome, the first step
that failed, why, and when each key step happened), the server's, relay's and phone's logs, and for the
emulator a picture and the screen's words at each step. `summary.md` and `junit.xml` cover the run.

```sh
python3 scripts/pairing-lab/report.py timeline /tmp/plab-out/<case>
```

The server's side is what `/api/p2p` and the device list say, written down whenever it changes: its
addresses, whether it holds a relay place, and each device's name, how it was added and when it was last
seen.
