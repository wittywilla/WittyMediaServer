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

<!-- module-kit -->
## Releasing

A version tag builds a program for every platform, signs the checksum list, and publishes a release that servers can install.

Versions move in small steps as work lands. `go run github.com/wittywilla/wittymodule/cmd/wittymodule@v0.2.0 bump .` raises it
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
