# WittyTuner: update plan

> **Status on 2026-11-04**
>
> **Planned:** [the idea](https://github.com/wittywilla/WittyMediaServer/discussions/50). No real tuner has been tried yet: that needs the owner's device.

**Now:** 0.2.0. Helper, API 3. Network tuners, IPTV playlists, capture cards and guide data; drivers describe their own settings so the web form is generated. Encrypted channels refused (the server's own `mpegts.Guard` reads scrambling bits too). **Can't be forked**, by policy. No real tuner or capture card has been tried on any system.

**Channel:** alpha.

| Release | Theme |
|---|---|
| 0.2.x | Fixes; real-hardware findings once the owner tries a tuner |
| **0.3.0** | Windows BDA; DiSEqC on Linux DVB; a viewer and a recording sharing one tuner |
| 0.4.0 | ATSC 3.0 sound; ATSC huffman guide text; licensed devices through their own interfaces |
| Later | More capture devices, smarter scanning |

## 0.2.x

- **Real hardware:** the owner has a licensed device and said they'd give details during real-world testing. Open a `needs-owner` issue with what to try: discovery, scan, tune, record, guide, encrypted label.
- macOS `avfoundation` capture: parser tests only. Try on a Mac when one's available.

## 0.3.0

- **Windows BDA** digital tuning (listed as "Not built" in the README so the gap is visible).
- **DiSEqC dish switches** on Linux DVB.
- **Shared tuner:** a viewer and a recording on the same multiplex share one tuner (needs the server's live stream to take a tuner stream it didn't start).
- **Channel scans** that resume after an interruption, and show progress in the activity panel.

## 0.4.0

- **ATSC 3.0 sound** (AC-4 decoding only if a decoder with a usable licence exists; otherwise say so).
- **ATSC huffman-compressed guide text.**
- **Licensed devices:** streams from licensed boxes through their own documented interfaces, as the encryption policy allows. A stream still encrypted on arrival shows the encrypted label.

## Later

- Signal quality graphs in WittyStats.
- Guide data: more XMLTV sources and mapping help.
- IPTV: playlist health checks (dead streams marked), catch-up where the provider's own interface offers it.

## Standing rule

Never decrypt. No bypass plugin, no hook, no device key extraction, no imitating certified clients, no HDCP capture. Device facts in help come from makers' or standards bodies' documents, with the date checked.

---

<sub>Future plans, written 2026-11-02. Not built, not tested.</sub>
