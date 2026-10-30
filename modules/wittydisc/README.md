# WittyDisc

**Reads optical disc images without mounting them.** WittyDisc opens `.iso` and `.img` images
of DVDs and Blu-rays and lists their files as byte ranges inside the image, so a player can read
a disc's structures directly and hand the raw video ranges to a decoder.

It is the reader under [WittyDVD](https://github.com/wittywilla/WittyDVD) and
[WittyBluray](https://github.com/wittywilla/WittyBluray), the disc engines of
[WittyMediaServer](https://github.com/wittywilla/WittyMediaServer) and WittyMediaPlayer.

> This repository is a read-only mirror of `modules/wittydisc` in
> [WittyMediaServer](https://github.com/wittywilla/WittyMediaServer), where development happens.
> Please open issues and pull requests there.

## What it does

- **UDF** 1.02 (DVD-Video) and **UDF 2.50** (Blu-ray), including:
  - metadata partitions, where Blu-ray keeps its directory structures;
  - extended file entries;
  - chained allocation descriptors, which files over a gigabyte need;
  - the mirror copy of the metadata when the main copy can't be read.
- **ISO 9660**, as a fallback for DVD "bridge" discs, including multi-extent files over 4 GiB.
- **Files as extents:** every file's size and the byte ranges it occupies in the image, so
  callers can read a 20 GB stream without copying it, or pass the ranges to FFmpeg's `subfile`
  protocol.
- **Safe with damaged images:** sizes and depths are bounded, descriptor chains are limited,
  and nothing is trusted to be where it claims.

## Use

```bash
go get github.com/wittywilla/wittydisc
```

```go
import "github.com/wittywilla/wittydisc/isofs"

d, err := isofs.Open("SHREK_2.ISO")
if err != nil { return err }
defer d.Close()

fmt.Println(d.Format, d.Label)              // "udf" or "iso9660", and the volume label
for _, f := range d.Files() {               // every file, with its size and extents
    fmt.Println(f.Path, f.Size, f.Extents)
}
b, err := d.ReadFile("BDMV/index.bdmv", 1<<20)   // a whole file, size-capped
f, ok := d.Find("VIDEO_TS/VTS_01_0.IFO")         // case-insensitive lookup
r := d.Reader(f)                                 // an io.ReaderAt over the file
```

No cgo, no dependencies beyond the standard library. Go 1.26 or newer.

## License

Apache-2.0, the same licence as [WittyMediaServer](https://github.com/wittywilla/WittyMediaServer).
Copyright 2026 WittyWilla and contributors.

You're welcome to use, change and share WittyDisc. If you redistribute it or build on it, the licence
asks you to keep the [NOTICE](NOTICE) file's attribution, *"WittyDisc, developed by WittyWilla"*, in
your own NOTICE file, documentation or About screen.
