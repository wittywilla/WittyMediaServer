#!/bin/sh
# Makes the FreeBSD package (.pkg) from a folder scripts/build-release.sh built. Runs on FreeBSD:
# pkg create is FreeBSD's own tool.
#
#   packaging/freebsd/build-pkg.sh <version> <package folder> <amd64|arm64> [out folder]
#
# People install it with: pkg add wittymediaserver-<version>-freebsd-<arch>.pkg
set -eu
cd "$(dirname "$0")/../.."

VERSION="$1"; STAGE="$2"; ARCH="$3"; OUT="${4:-dist}"
[ "$(uname -s)" = FreeBSD ] || { echo "This needs FreeBSD: pkg create is FreeBSD's tool." >&2; exit 1; }
[ -x "$STAGE/bin/wittymediaserver" ] || { echo "$STAGE has no server in bin." >&2; exit 1; }
mkdir -p "$OUT"
OUT="$(cd "$OUT" && pwd)"

case "$ARCH" in
  amd64) abi_arch=amd64 ;;
  arm64) abi_arch=aarch64 ;;
  *) echo "unknown arch $ARCH" >&2; exit 2 ;;
esac
MAJOR="$(uname -r | cut -d. -f1)"
# pkg reads 0.6.0.alpha as coming before 0.6.0.
PKG_VERSION="$(printf '%s' "$VERSION" | tr '-' '.')"

work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT
root="$work/root"
lib="$root/usr/local/lib/wittymediaserver"
mkdir -p "$lib" "$root/usr/local/bin" "$root/usr/local/etc/rc.d" "$root/usr/local/share/doc/wittymediaserver" \
         "$root/usr/local/share/applications" "$root/usr/local/share/icons/hicolor/256x256/apps"

cp -R "$STAGE/bin" "$lib/"
[ -d "$STAGE/modules" ] && cp -R "$STAGE/modules" "$lib/"
if [ -f "$STAGE/WittyMediaServer" ]; then
  cp "$STAGE/WittyMediaServer" "$lib/"
  chmod 755 "$lib/WittyMediaServer"
  sed -e "s|/usr/lib/wittymediaserver|/usr/local/lib/wittymediaserver|g" "$STAGE/share/wittymediaserver.desktop" > "$root/usr/local/share/applications/wittymediaserver.desktop"
  cp "$STAGE/share/wittymediaserver.png" "$root/usr/local/share/icons/hicolor/256x256/apps/"
fi
cp "$STAGE/docs/"* "$root/usr/local/share/doc/wittymediaserver/"
# The settings file beside the program. pre-install carries a port someone set in it over.
cp "$STAGE/settings.ini" "$lib/"
printf '{"product":"WittyMediaServer","kind":"freebsd-pkg"}\n' > "$lib/package-info.json"
ln -s ../lib/wittymediaserver/bin/wittymediaserver "$root/usr/local/bin/wittymediaserver"
sed -e 's|%%USER%%|wms|g' -e 's|%%DATA%%|/var/db/wittymediaserver|g' \
    -e 's|%%PROGRAM%%|/usr/local/lib/wittymediaserver|g' packaging/freebsd/wittymediaserver.rc > "$root/usr/local/etc/rc.d/wittymediaserver"
chmod 755 "$root/usr/local/etc/rc.d/wittymediaserver" "$lib/bin/"*

cat > "$work/+MANIFEST" <<EOF
name: wittymediaserver
version: "$PKG_VERSION"
origin: multimedia/wittymediaserver
comment: "Your own media server"
desc: "Your own media server: films, shows, music, photos, books, discs, TV and games, with a web app for every device on your home network."
maintainer: "noreply@users.noreply.github.com"
www: "https://github.com/wittywilla/WittyMediaServer"
prefix: /usr/local
abi: "FreeBSD:$MAJOR:$abi_arch"
licenselogic: single
licenses: [APACHE20]
message: [
  {
    message: "WittyMediaServer is installed.\n\nStart it now, and at every boot:\n  sysrc wittymediaserver_enable=YES\n  service wittymediaserver start\nThen open http://<this computer>:9797/ to finish setup. Another port: Settings, Network, or /var/db/wittymediaserver/settings.ini. If it says FFmpeg is missing: pkg install ffmpeg\nRemove: wittymediaserver --uninstall (asks about your data)\n",
    type: install
  },
  {
    message: "WittyMediaServer is updated. Start it again: service wittymediaserver start\n",
    type: upgrade
  },
  {
    message: "Your library, accounts and settings are still in /var/db/wittymediaserver. To remove those too, run the uninstall again from a new copy, or delete that folder and then: pw userdel wms\n",
    type: remove
  }
]
scripts: {
  pre-install: "$(sed -e 's/\\/\\\\/g' -e 's/"/\\"/g' packaging/freebsd/pre-install.sh | awk '{printf "%s\\n", $0}')",
  post-install: "$(sed -e 's/\\/\\\\/g' -e 's/"/\\"/g' packaging/freebsd/post-install.sh | awk '{printf "%s\\n", $0}')",
  pre-deinstall: "service wittymediaserver onestop >/dev/null 2>&1 || true\n"
}
EOF

# Everything under the root, and the folders that are ours alone.
( cd "$root" && find . \( -type f -o -type l \) | sed 's|^\./|/|' | sort ) > "$work/plist"
{
  echo "@dir /usr/local/lib/wittymediaserver/bin"
  echo "@dir /usr/local/lib/wittymediaserver"
  echo "@dir /usr/local/share/doc/wittymediaserver"
} >> "$work/plist"

pkg create -M "$work/+MANIFEST" -r "$root" -p "$work/plist" -o "$work/out"
target="$OUT/wittymediaserver-$VERSION-freebsd-$ARCH.pkg"
mv "$work/out/"*.pkg "$target"
echo "   -> $target"
