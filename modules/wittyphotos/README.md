# WittyPhotos

What a photo says about itself, read without decoding it: EXIF (when, with what camera, where),
the picture's dimensions, and the JPEG preview inside a camera RAW file. Pure Go with no
dependencies. Used by the WittyPhotos library in
[WittyMediaServer](https://github.com/wittywilla/WittyMediaServer).

```go
info, err := wittyphotos.ReadFile("IMG_0001.heic")   // JPEG, TIFF, RAW, HEIC, AVIF, PNG, WebP
w, h := wittyphotos.Dimensions("IMG_0002.png")       // JPEG, PNG, GIF, WebP, BMP
jpeg, err := wittyphotos.RawPreview("DSC_0003.nef")  // the largest embedded JPEG
```

Files are untrusted. Only a bounded part of each file is read, every offset is checked, and a
malformed file gives an error or empty fields, never a panic or a large allocation. The parsers
are fuzz-tested: `go test -fuzz=FuzzParse`.

Developed by WittyWilla. Apache-2.0: see [LICENSE](LICENSE) and [NOTICE](NOTICE).
