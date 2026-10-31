# WittyCouch Host

Moonlight's streaming core as a small program of its own. WittyMediaServer starts it for each game stream and talks to it over
stdin and stdout. It connects to a PC running Sunshine, and passes on whole video frames and audio packets, without decoding them.

> **Not an official project.** It isn't made by the [Moonlight](https://moonlight-stream.org) project or by
> [Sunshine](https://github.com/LizardByte/Sunshine)'s. It's built on Moonlight's code, whose authors did the hard part.

## Licence

**GPL-3.0**: see [LICENSE](LICENSE). What's built into it, each checked against its own licence file at the commit pinned in
[`scripts/fetch-core.sh`](scripts/fetch-core.sh):

| What | Licence | In which builds |
|---|---|---|
| [moonlight-common-c](https://github.com/moonlight-stream/moonlight-common-c) | GPL-3.0 | All |
| [ENet](https://github.com/cgutman/enet), Moonlight's version of it | MIT | All |
| [nanors](https://github.com/sleepybishop/nanors), with its obl library | MIT | All |
| [Mbed TLS](https://github.com/Mbed-TLS/mbedtls) 3.6.7 | Apache-2.0, chosen from its dual Apache-2.0 / GPL-2.0-or-later | All, unless built with `-DUSE_MBEDTLS=OFF` |
| [musl](https://musl.libc.org) | MIT | Linux releases |
| [mingw-w64](https://www.mingw-w64.org)'s runtime | ZPL-2.1, with some parts public domain and some under the permissive notices in NOTICE | Windows |
| GCC's runtime libraries (libgcc, libssp) | GPL-3.0 with the GCC Runtime Library Exception | Linux releases and Windows |

Mbed TLS's own third-party code (Everest, p256-m) isn't used: this program's Mbed TLS settings leave it out. Each of these licences can be combined
with GPL-3.0. [NOTICE](NOTICE) credits their authors, and carries the licence texts they ask to have kept with a binary.

Whoever gets a binary of this program can get its source. Each release carries `wittycouch-host-<version>-source.tar.gz` right beside its
binaries, with this program and every dependency at its exact commit, and the commits listed in `third_party/COMMITS.txt`. It builds with no network.

**It's a separate program.** WittyMediaServer, its relay and its browser client are Apache-2.0 and don't contain or link any of this. They only
exchange the messages in [PROTOCOL.md](PROTOCOL.md), which anyone can implement.

## Building

One command, which fetches everything at its pinned commit and leaves a single file in `dist/`:

```bash
sh scripts/build.sh [windows-x64 | linux-x64 | linux-arm64 | macos-x64 | macos-arm64]
```

With no target, it builds for the computer it's on. `dist/wittycouch-host-<target> --version` says it runs. It needs CMake, git, and a C and C++ compiler
(ENet's build asks for both, though nothing in it is C++).

| Target | Build on | Needs | Result |
|---|---|---|---|
| `windows-x64` | Linux, cross-compiling | `g++-mingw-w64-x86-64` (Debian, Ubuntu) | One `.exe`, needing only DLLs that are part of Windows |
| `linux-x64`, `linux-arm64` | Alpine | `build-base cmake linux-headers git` | Static with musl, so it runs on any Linux, the server's Docker image included |
| `linux-x64`, `linux-arm64` | Another Linux | `g++`, or `g++-aarch64-linux-gnu` to cross-compile for arm64 | Links that system's glibc, and needs that version or newer |
| `macos-x64`, `macos-arm64` | macOS | Xcode's command line tools, CMake | Links only what's part of macOS, version 11 or newer |

To build on Alpine from any computer with Docker, in this folder:

```bash
docker run --rm -v "$PWD:/src" -w /src alpine:3.24 sh -ec 'apk add --no-cache build-base cmake linux-headers git; git config --global --add safe.directory "*"; sh scripts/build.sh linux-x64'
```

Windows, Linux x64 (both kinds) have taken real streams from a real Sunshine. Linux arm64 has been built and run with `--version`, under emulation. macOS is only
built by the release workflow, on GitHub's computers, and hasn't been tried by hand.

Two CMake options, for building some other way. `-DUSE_MBEDTLS=OFF` links the system's OpenSSL (Apache-2.0, from version 3) instead of Mbed TLS, and needs its
development files. `-DWITTY_STATIC=ON` builds the C library in, and is for musl: built into a binary, glibc's licence (LGPL-2.1) would need its source offered with it too.

[`.github/workflows/release.yml`](.github/workflows/release.yml) builds all of them on every push, and publishes a release for a `v*` tag. GitHub only reads it once
this folder is the root of [its own repository](https://github.com/wittywilla/WittyCouchHost).

## Safety

A stream comes from someone else's computer, and this program parses it in C, so it's treated as if it might be taken over:

- It's started with an empty environment, and no addresses or keys on its command line.
- What it sends is treated as untrusted by the program that started it, with limits on every message.
- On Linux, it can't gain privileges, it's limited to no files (Landlock where the kernel has it), and it dies with its parent.
- On Windows, WittyMediaServer runs it at low integrity, and it loads DLLs only from Windows' own folder.
- Everything is built with stack protection and for address randomisation. Mbed TLS is cut down to the AES it needs, with no certificates, parsers or file access.

## Legal

WittyCouch Host is an independent open-source project built on Moonlight's code under the GPL-3.0. It
isn't associated with, sponsored by or endorsed by the Moonlight or Sunshine projects, NVIDIA, or any
game publisher. No games come with it. Any third-party names here only describe what it works with, and
belong to their owners. It's provided “as is”, with no warranty; see [LICENSE](LICENSE). Local law
applies to how you use it. This isn't legal advice.
