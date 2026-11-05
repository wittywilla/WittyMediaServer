#!/usr/bin/env bash
# Makes the Mac installer (.pkg) from a folder scripts/build-release.sh built, whose app was
# built on a Mac. Runs on a Mac only: pkgbuild, productbuild and codesign are Apple's.
#
#   packaging/macos/build-pkg.sh <version> <package folder> <arm64|amd64> [out folder]
#
# Signing and notarising happen when these are set (docs/PUBLISHING.md):
#   APPLE_APP_IDENTITY        "Developer ID Application: ..." signs the apps and every helper
#   APPLE_INSTALLER_IDENTITY  "Developer ID Installer: ..." signs the .pkg
#   APPLE_NOTARY_PROFILE      a stored notarytool profile; also staples the ticket on
# Without them the apps are signed ad hoc and the .pkg isn't signed: people have to
# right-click it and choose Open.
set -euo pipefail
cd "$(dirname "$0")/../.."

VERSION="${1:?version}"; STAGE="${2:?package folder}"; ARCH="${3:?arch}"; OUT="${4:-dist}"
[ "$(uname -s)" = Darwin ] || { echo "This needs a Mac: pkgbuild and productbuild are Apple's tools." >&2; exit 1; }
[ -d "$STAGE/WittyMediaServer.app/Contents/Helpers" ] || { echo "$STAGE has no WittyMediaServer.app with its helpers." >&2; exit 1; }
mkdir -p "$OUT"
OUT="$(cd "$OUT" && pwd)"
NUMERIC="$(printf '%s' "$VERSION" | sed -E 's/^([0-9]+\.[0-9]+\.[0-9]+).*/\1/')"

chmod +x packaging/macos/scripts/*/*
work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT
root="$work/root"
mkdir -p "$root/Applications" "$root/usr/local/bin"
cp -R "$STAGE/WittyMediaServer.app" "$STAGE/Uninstall WittyMediaServer.app" "$root/Applications/"
printf '{"product":"WittyMediaServer","kind":"pkg"}\n' > "$root/Applications/WittyMediaServer.app/Contents/Resources/package-info.json"
ln -s /Applications/WittyMediaServer.app/Contents/Helpers/wittymediaserver "$root/usr/local/bin/wittymediaserver"
chmod +x "$root/Applications/WittyMediaServer.app/Contents/Helpers/"* "$root/Applications/WittyMediaServer.app/Contents/MacOS/"*

# Signed again here, as package-info.json changed what the app's signature covers. Ad hoc
# without APPLE_APP_IDENTITY, which Local Network privacy still needs.
packaging/macos/sign-app.sh "$root/Applications" "${APPLE_APP_IDENTITY:--}"

# Two parts, so the service can be left out from the installer's Customize button.
pkgbuild --root "$root" --identifier com.wittywilla.wittymediaserver.app --version "$NUMERIC" \
  --install-location / --scripts packaging/macos/scripts/app "$work/app.pkg"
pkgbuild --nopayload --identifier com.wittywilla.wittymediaserver.service --version "$NUMERIC" \
  --scripts packaging/macos/scripts/service "$work/service.pkg"

sed -e "s|@VERSION@|$NUMERIC|g" -e "s|@DISPLAY_VERSION@|$VERSION|g" packaging/macos/distribution.xml > "$work/distribution.xml"
target="$OUT/wittymediaserver-$VERSION-macos-$ARCH.pkg"
productbuild --distribution "$work/distribution.xml" --package-path "$work" \
  ${APPLE_INSTALLER_IDENTITY:+--sign "$APPLE_INSTALLER_IDENTITY"} "$target"

if [ -n "${APPLE_NOTARY_PROFILE:-}" ]; then
  xcrun notarytool submit "$target" --keychain-profile "$APPLE_NOTARY_PROFILE" --wait
  xcrun stapler staple "$target"
fi
echo "   -> $target"
