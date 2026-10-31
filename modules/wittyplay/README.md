# WittyPlay

Plays console games in a browser, from a server. Each game runs in its own sandboxed process through a
[libretro](https://www.libretro.com) core, and its picture and sound stream to the browser over WebRTC,
with fallbacks for networks and browsers where that can't work.

WittyPlay is developed as part of [WittyMediaServer](https://github.com/wittywilla/WittyMediaServer).
`docs/GAMING.md` there explains the design, `docs/GAMING-API.md` is the API for clients, and
`docs/WITTYRETROARCH.md` covers licensing.

## What's in here

| Package | What it does |
|---|---|
| `wittyplay` | The consoles: extensions, recommended cores and their licences, sorting a file into a console, and each controller as data. No dependencies |
| `ipc` | The protocol between a server and a game runner, over pipes |
| `host`, `cmd/wittyplay-host` | The runner: a separate program that runs one game |
| `libretro` | The libretro frontend inside the runner, with no cgo |
| `hostsandbox` | What the runner does to box itself in before it loads a core |
| `testcore` | A tiny game written in Go, so everything can be tested with no third-party code |
| `cmd/wittyplay-check` | Plays real games with no browser, on the real sandboxed runner, and reports what happened to each. `docs/GAMING.md` explains it |
| `stream` | Server side: starts runners, encodes with FFmpeg, serves WebRTC and WebSocket viewers |
| `cores` | Finds cores on the machine and installs them on request |
| `web/` | The browser half: the player, controllers, remapping and the touch editor (React) |

## Playing a game from Go

```go
m := stream.NewManager(stream.Config{Runner: "wittyplay-host", FFmpeg: "ffmpeg"})
s, err := m.Start(stream.Options{
    UserID: 1, ItemID: 7, Title: "My game",
    Core: "/path/to/some_libretro.so", ROM: "/path/to/game", SystemDir: "/path/to/bios", SaveDir: "/path/to/saves",
})
// Give the browser's WebRTC offer to s.Answer, or hand a request to s.ServeWS.
```

## Licences

WittyPlay is Apache-2.0. **Cores, RetroArch, ROMs and BIOS files are not part of it, and are never
bundled.** A core is a separate file with its own licence, loaded only by the separate runner.
`cores.Install` only offers cores whose licences allow it, so non-commercial ones are never offered.
Bring your own games.

## Testing

```bash
go test ./...
```

Set `WMS_TEST_FFMPEG` to an FFmpeg program if it isn't on your `PATH`. The streaming tests run the real
runner with the test core, a real FFmpeg and a real WebRTC peer, and are skipped without FFmpeg.

To build the runner for FreeBSD without cgo:

```bash
GOOS=freebsd CGO_ENABLED=0 go build -gcflags="github.com/ebitengine/purego/internal/fakecgo=-std" ./cmd/wittyplay-host
```
