# WittyPhotos

A gallery of your own pictures and clips: a timeline, albums, favourites, folders, a map, sharing
and uploads. Nothing is looked up online, and your files are never changed.

## Setting it up

Add a library of the kind **Photos** (setup wizard, or Settings → Libraries) and choose the folders
your pictures are in. Everything below them is picked up, in any layout. Scanning reads only the
start of each file, so a large collection scans quickly; thumbnails are made the first time a picture
is looked at, a few at a time, and kept.

| | Formats |
|---|---|
| Pictures | JPEG, PNG, GIF, WebP, BMP, TIFF, HEIC/HEIF, AVIF, JPEG XL |
| Camera RAW | CR2, CR3, CRW, NEF, NRW, ARW, SRF, SR2, DNG, ORF, RW2, RAF, PEF, SRW, 3FR, FFF, ERF, KDC, DCR, MEF, MOS, MRW, IIQ, X3F, RWL |
| Clips | MP4, MOV, MKV, WebM, AVI and the other video formats the server plays |

HEIC and JPEG XL depend on the FFmpeg build the server uses; the builds shipped with releases read
them. A picture that can't be read shows its name instead of a thumbnail. **RAW files are shown by the
preview the camera stored inside them**, not developed: WittyPhotos is for looking through pictures,
not for editing them.

Two things are grouped for you:

- A RAW file with a JPEG of the same name beside it shows once, as the JPEG.
- A short clip (up to 40 MB) with a picture of the same name beside it, as phones save live photos,
  becomes that picture's **Live** button rather than a separate entry.

## Dates and places

A picture's date comes from its EXIF (`DateTimeOriginal`), then from its file name
(`IMG_20230412_153045`, `2023-04-12 15.30.45`), then from the file's date. It is kept as the time on
the clock where the picture was taken, so it stays in its month wherever you view it from.

Places come from EXIF GPS tags, and from the `location` tag of clips. Phones and some cameras write
these.

## The screens

- **Timeline:** by month and day, newest first. A month loads when it comes into view, so a library of
  any size only costs what's on screen. The years down the side jump.
- **Albums:** yours, and those others shared. An album belongs to whoever made it; a shared album is
  visible to every profile that can open the library, and each sees only what its own limits allow.
  **Select** in the timeline adds many pictures at once.
- **Favourites:** the ♡ on a picture. Favourites are per profile.
- **Folders:** your folders, as they are on disk.
- **Map:** pictures with a place, grouped as you zoom. Choosing a group zooms in, or lists its pictures.
- **People & places:** a placeholder, marked as such, for the [local AI](ROADMAP.md) when it exists.
  Nothing looks at your pictures today.

The viewer steps with ← and →, zooms with + and − (or the wheel, or a double-click), shows details with
**I**, favourites with **F**, and runs a slideshow with Enter or Space. With a TV remote, ← and → step,
OK starts or stops the slideshow, ↓ reaches the buttons, and Back closes.

## The map and the internet

The map needs nothing online: a small outline of the world's land ships with the app (from Natural
Earth, which is public domain), and the groups of pictures come from your server.

If you want streets and place names, an administrator can give an online map's address in
Settings → Sharing & photos. It is off by default. When it is on, **each viewer's device asks that
provider for the parts of the map being looked at**, so the provider learns roughly what area is being
viewed and from where. The pictures themselves stay on your server. Use a provider whose terms allow it.

## Uploads

Adults and administrators see **Upload** in the gallery, and can drop files onto it. Files are saved in
`<the library's first folder>/Uploads/<profile>/<year-month>/` and appear at once. Uploads:

- never replace a file: a name already taken gets a number;
- are checked by their first bytes as well as their name, so a web page called `photo.jpg` is refused;
- can be turned off, or limited in size, in Settings → Sharing & photos.

Kids and Guest profiles can't upload.

## Sharing

**Share** on a picture, clip or album gives two ways:

- **A link for other profiles** on this server, who still see only what their limits allow.
- **A link for someone without an account**, with an end date. They see only that, with no way into the
  server. Pictures are re-encoded for them, so camera details and locations stay home, and clips whose
  file says where they were shot go through the converted stream for the same reason. Tick *Allow
  downloading the original files* only if you mean it: an original carries everything in the file.

The server keeps only a fingerprint of a link, so a link is shown once. End one in Settings →
Sharing & photos, or turn all links off there. A link also stops working if the person who made it can
no longer see what it shows. It works wherever your server can be reached: only on your home network
until you set up remote access.

## Profiles

Each library can be limited per account, as everywhere. Pictures have no age rating, so **Kids and Guest
profiles see no photos** unless you allow unrated titles or that library for them in their limits.

## For apps (Apple TV, Android TV and others)

Everything is on the server, in JSON:

| Route | What it gives |
|---|---|
| `GET /api/photos/months?library=` | Months with pictures, for a timeline |
| `GET /api/photos/list?library=&ym=&album=&folder=&favorites=&kind=&search=&bbox=&limit=&cursor=` | One page, newest first, with `nextCursor` |
| `GET /api/photos/{id}` | One picture's details, including EXIF and place |
| `GET /api/photos/folders?library=` | Folders |
| `GET /api/photos/map?library=&zoom=` | Groups of pictures for a map zoom |
| `GET`/`POST /api/photos/albums`, `PUT`/`DELETE /api/photos/albums/{id}`, `POST`/`DELETE /api/photos/albums/{id}/items` | Albums |
| `GET /api/items/{id}/image?w=` | A JPEG at most `w` pixels along its longer side (160, 320, 480, 640, 1280, 1920 or 2560). Without `w`, the original if it is JPEG, PNG, GIF or WebP, and otherwise a large JPEG |
| `GET /api/items/{id}/original` | The file, to download |
| `GET /api/items/{id}/motion` | A live photo's clip |
| `PUT /api/items/{id}/reaction` `{value}` | Favourite (1) or not (0) |
| `POST /api/libraries/{id}/upload` | Multipart upload, field `file` |
| `POST /api/shares`, `GET /api/shares`, `DELETE /api/shares/{id}` | Links |
| `GET /api/share/{token}`, `.../image/{id}`, `.../play/{id}`, `.../original/{id}` | What a link opens, without an account |

Clips are ordinary video items: play them with `GET /api/items/{id}/play`, direct or HLS.
