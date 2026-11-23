# Smoke tests

End-to-end checks that a real server, built from the change being tested, does what people use it for. They run on every pull request into a release branch (`v0.*`) in `.github/workflows/smoke.yml`, and each failure leaves a log and a picture per step in the run's artifacts (`smoke-web`, `smoke-remote`).

They exist because broken features reached people while every unit test passed. Each test below names what it would have caught.

## What runs here (server repository)

| Job | What it does | Would have caught |
|---|---|---|
| `build` | The server with its web app (built with the `updatetest` tag, so it trusts a throwaway catalogue key), Wittyconnect, WittyRelay, and a catalogue holding WittyPlay and WittyTuner from their release branches | A server, module or relay that doesn't build together |
| `web` | Sets up a new server the way an owner would (administrator, a film, an album, a Game Boy game, WittyPlay, WittyTuner's demo tuner), then in a headless browser: **sign in** with the form, **play the film** (its clock moves and a frame has a picture), **play music**, **watch a demo tuner channel**, **start a WittyPlay game and see its frame fill the screen**, **open settings** | Any of those pages or players broken by a change |
| `remote` | Two homes behind routers that change ports for every connection, and a WittyRelay on a pretend internet between them, all in network namespaces. The server uses its own WittyRelay; Connect a device makes an invite link; the real connector (as the apps run it) enrols with the link at home, the cable is unplugged, and the server must answer **through the relay**: sign-in, the API, and a web page with the connector's cookie (what a web view such as WittyPlay's games frame sends). Then the device is removed, and must be shut out | Remote access that only works at home; the relay not carrying a listed device; a web page refused from away; a removed device still getting in |

The client apps' repository runs the Android phone and TV, and desktop app, smoke tests against this branch's server: see its `scripts/smoke/README.md`.

## Tiny media

`media.sh` makes everything with FFmpeg except the game: a 20-second film (padded to 6.5 MB, because a film library leaves anything under 5 MiB out as a sample), and three 4-second tracks. The game is **Adjustris** by tbsp (CC0 1.0), downloaded from its own release and checked against its SHA-256. The Game Boy core (Gambatte) comes from the libretro build server once, then from the job's cache.

## Running them on your own machine

On Linux or WSL, with Go, Node, FFmpeg and a Chromium-based browser:

```bash
scripts/smoke/build.sh /tmp/smoke-build            # clones the module repositories it needs beside it
scripts/smoke/media.sh /tmp/smoke-media
SMOKE_OUT=/tmp/smoke-out scripts/smoke/server.sh /tmp/smoke-build /tmp/smoke-media 19780
set -a; . /tmp/smoke-out/server.env; set +a
node scripts/smoke/web.mjs /tmp/smoke-out          # or name steps: ... /tmp/smoke-out film game
SMOKE_OUT=/tmp/smoke-out scripts/smoke/stop.sh
go build -o /tmp/smoke-build/bin/p2p-netsim ./scripts/p2p-netsim
sudo env "PATH=$PATH" SMOKE_OUT=/tmp/smoke-remote scripts/smoke/remote.sh /tmp/smoke-build
```

`SMOKE_MODULE_REF=v0.8.3-alpha` picks the modules' branch; `WV_BROWSER` names the browser. The web test only needs `WMS_URL`, so it can run on Windows against a server in WSL. `stop.sh` stops only the server and catalogue that `server.sh` started, by their PIDs.

## Reading a failure

The job's summary names the step that failed. In the artifact, `NN-<step>.log` has what the step did and, for the web, the page's console and any server error; `NN-web-<step>.png` is the screen when it ended; `data/logs` is the server's own log.
