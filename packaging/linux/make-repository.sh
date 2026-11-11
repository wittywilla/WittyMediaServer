#!/usr/bin/env bash
# SPDX-License-Identifier: AGPL-3.0-or-later
# Copyright (C) 2026 WittyWilla and contributors
# Turns a folder of .deb and .rpm files into a package repository that apt and dnf can use,
# ready to put on any web server. Run in a container or on a machine with dpkg-dev,
# createrepo-c and gpg.
#
#   packaging/linux/make-repository.sh <folder with packages> <out folder> [gpg key id]
#
# Without a key the repository works only with [trusted=yes] or gpgcheck=0, as for testing.
# For real use, make a key and give its id here: docs/PUBLISHING.md.
#
# What people then do:
#   apt:  deb [signed-by=/etc/apt/keyrings/wittymediaserver.asc] https://<host>/apt stable main
#   dnf:  baseurl=https://<host>/rpm/$basearch
set -euo pipefail

IN="${1:?folder with packages}"; OUT="${2:?out folder}"; KEY="${3:-}"
APT="$OUT/apt"; RPM="$OUT/rpm"
mkdir -p "$APT/pool/main" "$RPM"

shopt -s nullglob
debs=("$IN"/*.deb)
rpms=("$IN"/*.rpm)

if [ ${#debs[@]} -gt 0 ]; then
  command -v apt-ftparchive >/dev/null || { echo "apt-ftparchive is missing (apt install apt-utils)" >&2; exit 1; }
  # A folder for each architecture: apt-ftparchive's own --arch goes by a file name's pattern,
  # which these names don't follow.
  for f in "${debs[@]}"; do
    case "$f" in
      *-amd64.deb) a=amd64 ;;
      *-arm64.deb) a=arm64 ;;
      *-armv7.deb) a=armhf ;;
      *-386.deb) a=i386 ;;
      *) echo "can't tell the architecture of $f" >&2; exit 1 ;;
    esac
    mkdir -p "$APT/pool/main/$a"
    cp "$f" "$APT/pool/main/$a/"
  done
  (
    cd "$APT"
    for arch in amd64 arm64 armhf i386; do
      dir="dists/stable/main/binary-$arch"
      mkdir -p "$dir"
      if [ -d "pool/main/$arch" ]; then
        apt-ftparchive packages "pool/main/$arch" > "$dir/Packages"
      else
        : > "$dir/Packages"
      fi
      gzip -9fk "$dir/Packages"
    done
    apt-ftparchive -o APT::FTPArchive::Release::Suite=stable -o APT::FTPArchive::Release::Codename=stable \
      -o APT::FTPArchive::Release::Components=main -o "APT::FTPArchive::Release::Architectures=amd64 arm64 armhf i386" \
      -o APT::FTPArchive::Release::Origin=WittyMediaServer -o APT::FTPArchive::Release::Label=WittyMediaServer \
      release dists/stable > dists/stable/Release
    if [ -n "$KEY" ]; then
      gpg --batch --yes --local-user "$KEY" --armor --detach-sign -o dists/stable/Release.gpg dists/stable/Release
      gpg --batch --yes --local-user "$KEY" --clearsign -o dists/stable/InRelease dists/stable/Release
      gpg --armor --export "$KEY" > "$OUT/wittymediaserver.asc"
    fi
  )
fi

if [ ${#rpms[@]} -gt 0 ]; then
  command -v createrepo_c >/dev/null || { echo "createrepo_c is missing" >&2; exit 1; }
  for f in "${rpms[@]}"; do
    case "$f" in
      *-amd64.rpm) a=x86_64 ;;
      *-arm64.rpm) a=aarch64 ;;
      *-armv7.rpm) a=armv7hl ;;
      *-386.rpm) a=i386 ;;
      *) echo "can't tell the architecture of $f" >&2; exit 1 ;;
    esac
    mkdir -p "$RPM/$a"
    cp "$f" "$RPM/$a/"
    [ -n "$KEY" ] && command -v rpmsign >/dev/null && rpmsign --define "_gpg_name $KEY" --addsign "$RPM/$a/$(basename "$f")"
  done
  for d in "$RPM"/*/; do
    createrepo_c --quiet "$d"
    [ -n "$KEY" ] && gpg --batch --yes --local-user "$KEY" --armor --detach-sign "$d/repodata/repomd.xml"
  done
fi
echo "Repository in $OUT"
