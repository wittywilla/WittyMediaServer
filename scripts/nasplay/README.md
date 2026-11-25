# NAS playback check

`nasplay.py` plays an item from a server running in a container the way a player does, at a
playback rate, and records what a viewer would see: how long it took to start, every stall and
how long it lasted, how long each segment took to arrive, and how far ahead of the player the
server's conversion stayed (by looking in the stream's folder inside the container).

Two players:

- `--client hlsjs` (the default): the web app's hls.js, as `web/src/player/hooks.ts` sets it up:
  a minute per try, six retries, a 30-second buffer.
- `--client exo`: ExoPlayer as the Android app set it up before 0.6.3: 8-second timeouts, three
  retries a second or two apart, then the app's own three new requests for the stream, and a
  `500` ends it. That is how "it stalls and then stops early on" looked on a small NAS. Add
  `--buffer 50` for ExoPlayer's buffer.
- `--client exo-new`: the Android app from 0.6.3: 75-second timeouts, six retries.

```bash
# A small NAS: two processors, 2 GB, media read-only.
docker run -d --name wms-nas --cpuset-cpus 0,1 --memory 2g -p 127.0.0.1:19811:9797 \
  -v wms-nas-config:/config -v ~/nas-media:/media:ro ghcr.io/wittywilla/wittymediaserver:alpha
python3 scripts/nasplay/nasplay.py setup --url http://127.0.0.1:19811 --container wms-nas
python3 scripts/nasplay/nasplay.py play --title "Some Film" --rate 2 -v
```

`setup` makes an administrator from the setup code, and a Movies library on `/media/Movies`. The
player asks as the web app does from a browser that can't play HEVC, so HEVC and 10-bit files are
converted. Each run prints one JSON line: `result` (`end`, or why it stopped), `reached` and `of`,
`stalls` and `stallSec`, `startup`, `waitMax` and `waitAvg` per segment, and `leadMin` and
`leadAvg`, the server's lead in seconds of the film (negative: the player waited for it).

`scripts/test/nas-playback.sh <image>` is the CI check (the `docker` job in `build.yml`): one
processor, 1 GB, a 90-second MPEG-2 clip, played at 2× by the web player and the 0.6.3 Android player, failing on any stall after
the start or an end before the true end.
