# WittyPhotos

What a photo says about itself, read without decoding it: EXIF (when, with what camera, where),
the picture's dimensions, and the JPEG preview inside a camera RAW file. Pure Go, and the library
itself has no dependencies. Used by the WittyPhotos library in
[WittyMediaServer](https://github.com/wittywilla/WittyMediaServer).

`cmd/wittyphotos-module` is the same code as a helper process, so a new version can reach a server
without a new server. Its `module.json` says which servers it works with, and a `v*` tag publishes a
signed release of it. See WittyMediaServer's `docs/MODULES.md`.

```go
info, err := wittyphotos.ReadFile("IMG_0001.heic")   // JPEG, TIFF, RAW, HEIC, AVIF, PNG, WebP
w, h := wittyphotos.Dimensions("IMG_0002.png")       // JPEG, PNG, GIF, WebP, BMP
jpeg, err := wittyphotos.RawPreview("DSC_0003.nef")  // the largest embedded JPEG
```

Files are untrusted. Only a bounded part of each file is read, every offset is checked, and a
malformed file gives an error or empty fields, never a panic or a large allocation. The parsers
are fuzz-tested: `go test -fuzz=FuzzParse`.

Developed by WittyWilla. Apache-2.0: see [LICENSE](LICENSE) and [NOTICE](NOTICE).

## Legal

WittyPhotos is an independent open-source project. It isn't associated with, sponsored by or
endorsed by any publisher, rights holder, service or other media software project. It reads the
files you give it, and doesn't provide or check them. Any third-party names here only describe what
it works with, and belong to their owners. It's provided “as is”, with no warranty; see
[LICENSE](LICENSE). Local law applies to how you use it. This isn't legal advice.
