# WittyModule

What a WittyMediaServer plugin needs to be a project of its own: the manifest, the rules for version
ranges, the protocol a helper process speaks, and the tool that builds and signs a release. Go, no
dependencies.

```bash
go run github.com/wittywilla/wittymodule/cmd/wittymodule check .
go run github.com/wittywilla/wittymodule/cmd/wittymodule pack -out release .
go run github.com/wittywilla/wittymodule/cmd/wittymodule sign release/<id>-<version>-SHA256SUMS.txt
```

A helper is a small program:

```go
func main() {
	wittymodule.Main(manifestJSON, func(method string, params json.RawMessage) (any, []byte, error) {
		// answer one call
	})
}
```

The design, including what each version range means, is in
[WittyMediaServer's docs/MODULES.md](https://github.com/wittywilla/WittyMediaServer/blob/master/docs/MODULES.md).

WittyModule is developed by WittyWilla. Apache-2.0.
