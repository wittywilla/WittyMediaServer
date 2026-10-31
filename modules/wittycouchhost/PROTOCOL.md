# The protocol

Everything between WittyCouch Host and the program that starts it, which is all that connects them. Each message is one byte of type,
a 4-byte little-endian length, then that many bytes. Numbers are little-endian. A message in either direction is at most 1 MiB, but for video, which can
be up to 16 MiB.

The program ends when stdin closes.

## Sent to the program

| Type | Name | Body |
|---|---|---|
| 64 | Start | `key=value` lines: `address`, `appversion`, `gfeversion`, `rtsp`, `scm`, `width`, `height`, `fps`, `bitrate` (kbps), `packetsize`, `remote` (0 or 1), `formats` (a mask of Moonlight's video formats, 1 being H.264), `rikey` and `riiv` (32 hex characters each). Comes first, once |
| 65 | Controller | u8 controller, u8 reserved, u16 active controllers mask, u32 buttons, u8 left trigger, u8 right trigger, then i16 left X, left Y, right X, right Y. 18 bytes |
| 66 | Mouse move | i16 dx, i16 dy |
| 67 | Mouse button | u8 action (7 press, 8 release), u8 button (1 left, 2 middle, 3 right, 4 and 5 side) |
| 68 | Key | u16 Windows virtual key code, with the high bit set; u8 action (3 press, 4 release); u8 modifiers (1 shift, 2 ctrl, 4 alt, 8 meta) |
| 69 | Scroll | i16, with 120 being a notch |
| 70 | Keyframe | None: asks for a keyframe |
| 71 | Quit | None |
| 72 | Mouse position | i16 x, i16 y, i16 reference width, i16 reference height |
| 73 | Text | UTF-8, up to 256 bytes |
| 74 | Horizontal scroll | i16 |

## Sent by the program

| Type | Name | Body |
|---|---|---|
| 1 | Event | JSON, with a `type`: `hello` (`version`, `protocol`), `started`, `failed` (`stage`, `code`), `terminated` (`code`: 0 means the PC ended it normally), `status` (`poor`), `video` (`format`, `width`, `height`, `fps`), `audio` (`rate`, `channels`, `streams`, `coupled`, `samples`) |
| 2 | Video | u8 flags (bit 0: keyframe), u32 the PC's 90 kHz timestamp, then one whole frame in Annex B form |
| 3 | Audio | One Opus packet |
| 4 | Rumble | u16 controller, u16 low-frequency motor, u16 high-frequency motor |

`program --version` prints `wittycouch-host <version> protocol <number>`, and exits.
