#!/bin/sh
# Builds ../runtime.jar (the platform disc applications link against) and
# ../testdata/00000.jar (the test disc's application). Needs a JDK 11 or newer;
# nothing here is needed to build or run the server, only to change the runtime.
# JDK=/path/to/jdk sh build.sh
set -e
cd "$(dirname "$0")"
B="${JDK:+$JDK/bin/}"
T=$(mktemp -d)
find src -name '*.java' > "$T/files"
"${B}javac" -nowarn --release 8 -Xlint:-options -encoding UTF-8 -d "$T/classes" @"$T/files"
"${B}jar" --create --no-manifest --file ../runtime.jar -C "$T/classes" .
"${B}javac" -nowarn --release 8 -Xlint:-options -encoding UTF-8 -cp "$T/classes" -d "$T/test" testxlet/t/TestXlet.java
cp testxlet/t/hello.txt "$T/test/t/"
mkdir -p ../testdata
"${B}jar" --create --no-manifest --file ../testdata/00000.jar -C "$T/test" .
# The hostile disc. Its probes name what disc code must not link, so it builds against the full JDK.
"${B}javac" -nowarn --release 8 -Xlint:-options -encoding UTF-8 -cp "$T/classes" -d "$T/attack" testxlet/a/*.java
"${B}jar" --create --no-manifest --file ../testdata/attack.jar -C "$T/attack" .
rm -rf "$T"
