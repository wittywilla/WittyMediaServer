<!-- module-kit -->
## Releasing

@@README_WHAT@@

#@if manifest
Versions move in small steps as work lands. `go run @@TOOL_RELEASED@@ bump .` raises it
everywhere it is written (add `-part minor` for a bigger step). A bump isn't a release: nothing is
tagged or published until you tag it. CI fails when code changed since the last tag and the version
didn't, because tags never move.

To release:

1. Check that `version` in @@README_VERSION@@ is the one to publish, with the bump committed.
#@end
#@if app
To release:

1. Raise `version` in @@README_VERSION@@ and commit it.
#@end
2. Tag that commit and push the tag: `git tag vX.Y.Z`, then `git push origin vX.Y.Z`. A tag with a
   suffix, such as `v1.2.0-beta`, is published as a prerelease.
3. Watch the *Actions* tab. @@README_AFTER@@
@@README_WITHDRAW@@
Licensed under Apache-2.0: see [LICENSE](LICENSE) and [NOTICE](NOTICE).
<!-- /module-kit -->
