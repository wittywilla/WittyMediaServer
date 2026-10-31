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

<!-- module-kit -->
## Releasing

WittyModule reaches a server inside the projects built with it, so a release has no files of its own: a version tag runs the tests and publishes a release that those projects can require.

Versions move in small steps as work lands. `go run github.com/wittywilla/wittymodule/cmd/wittymodule@v0.2.0 bump .` raises it
everywhere it is written (add `-part minor` for a bigger step). A bump isn't a release: nothing is
tagged or published until you tag it. CI fails when code changed since the last tag and the version
didn't, because tags never move.

To release:

1. Check that `version` in `module.json` is the one to publish, with the bump committed.
2. Tag that commit and push the tag: `git tag vX.Y.Z`, then `git push origin vX.Y.Z`. A tag with a
   suffix, such as `v1.2.0-beta`, is published as a prerelease.
3. Watch the *Actions* tab. There is nothing to sign.

Licensed under Apache-2.0: see [LICENSE](LICENSE) and [NOTICE](NOTICE).
<!-- /module-kit -->
