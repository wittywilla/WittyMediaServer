#!/usr/bin/env bash
# Makes the .deb and .rpm from a folder scripts/build-release.sh built.
#
#   packaging/linux/build-packages.sh <version> <package folder> <arch> [out folder]
#
#   arch: amd64, arm64, armv7 or 386
#
# Needs nfpm: go install github.com/goreleaser/nfpm/v2/cmd/nfpm@v2.46.3
set -euo pipefail
cd "$(dirname "$0")/../.."

VERSION="${1:?version}"; STAGE_DIR="${2:?package folder}"; ARCH="${3:?arch}"; OUT="${4:-dist}"
NFPM="${NFPM:-nfpm}"
command -v "$NFPM" >/dev/null || { echo "nfpm is not installed: see the top of this script" >&2; exit 1; }
mkdir -p "$OUT"

case "$ARCH" in
  amd64|arm64|386) NFPM_ARCH="$ARCH" ;;
  armv7) NFPM_ARCH=arm7 ;;
  *) echo "unknown arch $ARCH" >&2; exit 2 ;;
esac

# A version like 0.6.0-alpha has to sort before 0.6.0: both systems read ~ as "before".
PKG_VERSION="${VERSION/-/\~}"

work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT
for kind in deb rpm; do
  rm -rf "$work/stage"
  cp -R "$STAGE_DIR/." "$work/stage/" 2>/dev/null || { mkdir -p "$work/stage"; cp -R "$STAGE_DIR/." "$work/stage/"; }
  # Only for archives and for updating from older copies.
  rm -f "$work/stage/wms" "$work/stage/install.sh"
  # Which installer owns this copy.
  printf '{"product":"WittyMediaServer","kind":"%s"}\n' "$kind" > "$work/stage/package-info.json"

  sed -e "s|\${STAGE}|$work/stage|g" -e "s|\${NFPM_ARCH}|$NFPM_ARCH|g" -e "s|\${PKG_VERSION}|$PKG_VERSION|g" \
    packaging/linux/nfpm.yaml > "$work/nfpm.yaml"
  # The modules that come with a package, when this one has any.
  if [ -d "$work/stage/modules" ]; then
    sed -i "s|^  # The command, on everyone's PATH.|  - src: $work/stage/modules/\n    dst: /usr/lib/wittymediaserver/modules/\n    type: tree\n\n&|" "$work/nfpm.yaml"
  fi

  target="$OUT/wittymediaserver-$VERSION-linux-$ARCH.$kind"
  "$NFPM" package --config "$work/nfpm.yaml" --packager "$kind" --target "$target"
  echo "   -> $target"
done
