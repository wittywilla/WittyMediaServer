<h1 align="center">WittyDisc</h1>

<p align="center">
  <b>Reads DVD and Blu-ray disc images without mounting them.</b><br>
  Every file in an <code>.iso</code> or <code>.img</code>, as byte ranges a player can read or hand straight to a decoder.
</p>

<p align="center">
  <a href="LICENSE"><img alt="Licence: Apache-2.0" src="https://img.shields.io/badge/licence-Apache--2.0-blue.svg"></a>
  <img alt="Go 1.26+" src="https://img.shields.io/badge/Go-1.26%2B-00ADD8.svg">
  <img alt="No dependencies" src="https://img.shields.io/badge/dependencies-none-success.svg">
</p>

> Read-only mirror of `modules/wittydisc` in [WittyMediaServer](https://github.com/wittywilla/WittyMediaServer),
> where development happens. Please open issues and pull requests there.

WittyDisc is the reader under [WittyDVD](https://github.com/wittywilla/WittyDVD) and
[WittyBluray](https://github.com/wittywilla/WittyBluray), the disc engines of
[WittyMediaServer](https://github.com/wittywilla/WittyMediaServer).

## What it reads

| Format | Used by | Details |
|---|---|---|
| **UDF 1.02** | DVD-Video | the file system DVD players read |
| **UDF 2.50** | Blu-ray | metadata partitions (where Blu-ray keeps its folders), extended file entries, chained allocation descriptors for files over a gigabyte, and the mirror copy of the metadata when the main one can't be read |
| **ISO 9660** | DVD “bridge” discs | a fallback, including multi-extent files over 4 GiB |

- **Files as extents:** each file's size and the byte ranges it occupies, so a 20 GB stream is read
  in place, or passed to FFmpeg's `subfile` protocol, never copied.
- **Safe with damaged images:** sizes and depths are bounded, descriptor chains are limited, and
  nothing is trusted to be where it claims.

## Use

```bash
go get github.com/wittywilla/wittydisc@latest
```

```go
import "github.com/wittywilla/wittydisc/isofs"

d, err := isofs.Open("MY_DISC.ISO")
if err != nil { return err }
defer d.Close()

fmt.Println(d.Format, d.Label)                    // "udf" or "iso9660", and the volume label
for _, f := range d.Files() {                     // every file with its size and extents
    fmt.Println(f.Path, f.Size, f.Extents)
}
b, err := d.ReadFile("BDMV/index.bdmv", 1<<20)    // a whole file, size-capped
f, ok := d.Find("VIDEO_TS/VTS_01_0.IFO")          // case-insensitive lookup
r := d.Reader(f)                                  // an io.ReaderAt over the file
```

| Call | Does |
|---|---|
| `Open(path)` | opens an image (UDF first, then ISO 9660) |
| `Files()`, `Find(path)` | every file, or one by path, case-insensitively |
| `ReadFile(path, max)` | a whole file, with a size cap |
| `Reader(file)` | an `io.ReaderAt` over a file's extents |

No cgo and no dependencies beyond the standard library. Go 1.26 or newer.

## The Witty family

| Project | Role |
|---|---|
| [WittyMediaServer](https://github.com/wittywilla/WittyMediaServer) | the media server these engines power |
| [WittyDVD](https://github.com/wittywilla/WittyDVD) · [WittyBluray](https://github.com/wittywilla/WittyBluray) | the DVD and Blu-ray engines |
| **WittyDisc** | disc image reader |

## Licence

**Apache-2.0**, the same licence as WittyMediaServer. © 2026 WittyWilla and contributors.
Use, change and share it; if you redistribute it or build on it, keep the [NOTICE](NOTICE)
attribution, *“WittyDisc, developed by WittyWilla”*, in your NOTICE file, documentation or About screen.

## Legal

WittyDisc is an independent open-source project. It isn't associated with, sponsored by or endorsed
by the owners of the DVD or Blu-ray formats, any film studio or any other player project. It reads
disc images you already have. It doesn't decrypt anything and no media comes with it: you're
responsible for having the rights to what you open. Any third-party names here only describe what it
works with, and belong to their owners. It's provided “as is”, with no warranty; see
[LICENSE](LICENSE). Local law applies to how you use it. This isn't legal advice.
