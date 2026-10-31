# WittyMoonlight Host

Moonlight's streaming core as a small program of its own. WittyMediaServer starts it for each game stream and talks to it over
stdin and stdout. It connects to a PC running Sunshine, and passes on whole video frames and audio packets, without decoding them.

> **Not an official project.** It isn't made by the [Moonlight](https://moonlight-stream.org) project or by
> [Sunshine](https://github.com/LizardByte/Sunshine)'s. It's built on Moonlight's code, whose authors did the hard part.

## Licence

**GPL-3.0**: see [LICENSE](LICENSE). This program links [moonlight-common-c](https://github.com/moonlight-stream/moonlight-common-c) (GPL-3.0),
which itself includes [ENet](https://github.com/cgutman/enet) (MIT) and [nanors](https://github.com/sleepybishop/nanors) (MIT), and OpenSSL
(Apache-2.0) or Mbed TLS (Apache-2.0).

Whoever gets a binary of this program can get its source: this repository, at the tag of that binary's release, plus the pinned commit of
moonlight-common-c in `scripts/fetch-core.sh`. Each release carries a source archive with everything in it.

**It's a separate program.** WittyMediaServer, its relay and its browser client are Apache-2.0 and don't contain or link any of this. They only
exchange the messages in [PROTOCOL.md](PROTOCOL.md), which anyone can implement.

## Building

```bash
sh scripts/fetch-core.sh
cmake -S . -B build -DCMAKE_BUILD_TYPE=Release
cmake --build build
```

It needs a C compiler, CMake and OpenSSL's development files. `build/wittymoonlight-host --version` says it runs.

## Safety

A stream comes from someone else's computer, and this program parses it in C, so it's treated as if it might be taken over:

- It's started with an empty environment, and no addresses or keys on its command line.
- What it sends is treated as untrusted by the program that started it, with limits on every message.
- On Linux, it can't gain privileges, it's limited to no files (Landlock where the kernel has it), and it dies with its parent.
