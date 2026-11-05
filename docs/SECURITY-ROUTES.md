# HTTP routes

Every route of the server: who may call it and what it validates. `TestRouteTableMatchesTheCode` keeps this table and the code
together. [SECURITY-REVIEW.md](SECURITY-REVIEW.md) describes the model behind it.

Nothing else listens for HTTP: the HTTPS port serves this same handler. One route is a WebSocket: `GET /api/play/{sid}/ws`.

Every `/api/...` route is also served as `/api/v1/...`. `HEAD` is served wherever `GET` is.

## What applies to every request

- Requests from outside the home network are refused until the owner turns remote access on, and then need HTTPS or a VPN.
- Requests with a body are limited to 32 MB and 2 minutes, except the routes that set their own limit. A route that takes a body from
  someone not signed in is limited to 16 KB and 15 seconds, and JSON to 1 MB unless the table says otherwise.
- State-changing requests signed in by cookie are refused when they come from another origin.
- `/api/` responses carry a script-free content policy unless the table says otherwise.

## Words used in the table

| Word | Meaning |
|---|---|
| Public | No sign-in |
| Signed-in | Any account: administrator, adult, Kids or Guest |
| Adult | An administrator or an adult account |
| Tuner channels | An account the owner lets use tuner channels |
| Signed-in with game PC access | An account the owner lets use game PCs |
| Admin | An administrator |
| ticket | Also accepts a stream token (`?st=`) made for that item, for reading only |
| address token | Also accepts a token in the address (`?token=`), for `GET` and `HEAD` |
| limits | The account's limits are applied: libraries, age rating, blocked genres, allowed and blocked titles. A title outside them answers "not found" |
| own | Looked up together with the signed-in account's ID. Another account's answers "not found" |
| Nothing | The route takes no input to check |

## Routes

| Method + route | Who may call it | What it validates |
|---|---|---|
| `GET /api/status` | Public | Reports setup state, branding and the caller's own account if signed in. Version left out for signed-out callers from the internet |
| `POST /api/setup` | Public, until the first account exists | Refused once an account exists. 16 KB body. From outside the home network: setup code, with an attempt limit. Name and password rules |
| `POST /api/login` | Public | 16 KB body. Attempts counted per name and per address. Password, and a code with two-factor sign-in. Password-free profiles only at home with the profile screen on |
| `POST /api/logout` | Public | Ends the sessions of the tokens the request carries |
| `GET /api/profiles` | Public | Lists profiles only when the profile screen is on and the caller is at home, or the owner allows it from away |
| `GET /api/themes` | Public | Installed themes and the current one |
| `GET /api/themes/{id}/download` | Public | Theme id resolved by the theme manager |
| `GET /api/themes/{id}/{file...}` | Public | Theme id and file name checked by the theme manager. Inert content policy |
| `GET /api/themes/{id}/tokens.json` | Public | Theme id resolved by the theme manager |
| `GET /api/health` | Public | Nothing: answers `ok`, and this run's random `instance` id, so that a second start on the same data folder knows the server answering is the one using it. The id is made fresh at every start and says nothing about the server |
| `GET /api/capabilities` | Public | Describes the server to apps. Version left out for signed-out callers from the internet |
| `POST /api/identity` | Public | Nonce format. Answers with a signature over it |
| `POST /api/pair/start` | Public | Server is set up. Limits on open codes per address and for the server |
| `POST /api/pair/poll` | Public | 64-character device secret. Polling rate |
| `POST /api/pair/redeem` | Public | Six digits. Wrong-code limits per address and for the server. Code used once |
| `GET /api/me` | Signed-in | The caller's own account |
| `GET /api/me/ui` | Signed-in | own |
| `PUT /api/me/ui` | Signed-in | own. A JSON object of at most 16 KB |
| `GET /api/home` | Signed-in | own progress. limits |
| `GET /api/libraries` | Signed-in | limits. Folder paths for admins only |
| `POST /api/libraries` | Admin | Name, known type, full paths to existing folders, known console. Module for the type is installed |
| `PUT /api/libraries/{id}` | Admin | As creating. Refused for a library owned by a game PC |
| `DELETE /api/libraries/{id}` | Admin | Numeric id |
| `POST /api/libraries/{id}/scan` | Admin | Library exists |
| `GET /api/libraries/{id}/scan` | Signed-in | Library is within the account's allowed libraries. File names and skipped files for admins only |
| `GET /api/genres` | Signed-in | limits |
| `GET /api/items` | Signed-in | limits, in the query. Paths for admins only |
| `GET /api/logos/status` | Signed-in | Whether downloads are agreed to, and the source. Counts for admins only |
| `PUT /api/logos/consent` | Admin | Body says yes or no |
| `POST /api/logos/lookup` | Signed-in | Up to 200 names. Only an admin's request can add a name or ask a source |
| `GET /api/logos` | Admin | Kind known. Bounded page |
| `GET /api/logos/credits` | Signed-in | limits, on studios and networks: those of titles and channels the account may see |
| `GET /api/logos/preview` | Admin | Downloads agreed to. An address on the source's own host. Internet-only client, pictures only |
| `GET /api/logos/{id}/image` | Signed-in | Numeric id. One of three variants, from the server's own files. Pictures only |
| `GET /api/logos/{id}/candidates` | Admin | Logo exists. Downloads agreed to |
| `PUT /api/logos/{id}` | Admin | Logo exists. Downloads agreed to, for a picture the source offers, fetched again by the server. Known status |
| `PUT /api/logos/{id}/upload` | Admin | Logo exists. Picture of at most 8 MB, drawn again as new PNGs |
| `GET /api/brands` | Signed-in | limits, in the query. Kind is studio or network |
| `GET /api/browse` | Signed-in | limits, in the query. Library within the account's allowed libraries. Filter values passed as data |
| `GET /api/browse/facets` | Signed-in | limits, in the query. Library within the account's allowed libraries |
| `GET /api/browse/shelves` | Signed-in | limits, in the query. Library within the account's allowed libraries. Hidden categories for admins only |
| `PUT /api/browse/categories` | Admin | Library exists. Fields |
| `GET /api/views` | Signed-in | own |
| `POST /api/views` | Signed-in | own. 100 per account. Name size, known filters only, library within the account's allowed libraries |
| `PUT /api/views/{id}` | Signed-in | own. Name size, known filters only, library within the account's allowed libraries |
| `DELETE /api/views/{id}` | Signed-in | own |
| `GET /api/views/{id}/titles` | Signed-in | own, then limits, in the query |
| `GET /api/listings/status` | Signed-in | Whether listings are on, the region and the credit. Counts and progress for admins only |
| `PUT /api/listings/settings` | Admin | Two-letter region. Every statement accepted before turning on |
| `POST /api/listings/refresh` | Admin | Nothing further |
| `GET /api/meta/removed` | Admin | Nothing further |
| `POST /api/meta/removed/restore` | Admin | Title was removed |
| `POST /api/meta/undo` | Admin | Batch exists and can still be undone |
| `GET /api/meta/history` | Admin | Nothing further |
| `POST /api/meta/bulk` | Admin | 1 to 500 titles. Field values as for one title. A picture is a web address or a file in a library |
| `GET /api/meta/show/{id}/episodes` | Admin | Numeric id |
| `PUT /api/meta/show/{id}/episodes` | Admin | Numeric id. Numbering checked |
| `GET /api/meta/{target}/{id}` | Admin | Item or show. Title exists |
| `PUT /api/meta/{target}/{id}` | Admin | Title exists. Field values checked. A picture is a web address or a file in a library |
| `DELETE /api/meta/{target}/{id}` | Admin | Title exists |
| `POST /api/meta/{target}/{id}/revert` | Admin | Title exists. Field names |
| `POST /api/meta/{target}/{id}/search` | Admin | Title exists. Query length |
| `POST /api/meta/{target}/{id}/preview` | Admin | Title exists. Source and id |
| `POST /api/meta/{target}/{id}/match` | Admin | Title exists. Source and id |
| `DELETE /api/meta/{target}/{id}/match` | Admin | Title exists |
| `POST /api/meta/{target}/{id}/refresh` | Admin | Title exists |
| `POST /api/meta/{target}/{id}/suggest` | Admin | Title exists |
| `POST /api/meta/{target}/{id}/leave` | Admin | Title exists |
| `GET /api/meta/{target}/{id}/artwork` | Admin | Title exists. Known kind of artwork |
| `PUT /api/meta/{target}/{id}/art/{kind}` | Admin | Title exists and has that artwork. JPEG, PNG, WebP or GIF of at most 20 MB. Stored under a name the server makes |
| `GET /api/meta/{target}/{id}/art/{kind}` | Signed-in | limits. Banner or logo. Pictures only |
| `POST /api/meta/{target}/{id}/link` | Admin | Not a show. Attached to one film or show that exists, not to itself. Known kind of link |
| `DELETE /api/meta/{target}/{id}/link` | Admin | Title exists |
| `POST /api/meta/{target}/{id}/move` | Admin | Title exists. Library checked |
| `GET /api/meta/{target}/{id}/listings` | Signed-in | limits |
| `GET /api/meta/{target}/{id}/attached` | Signed-in | limits, on the title and each extra and version. Paths for admins only |
| `GET /api/match/queue` | Admin | Nothing further |
| `POST /api/match/accept` | Admin | 1 to 100 titles |
| `POST /api/match/suggest-all` | Admin | Library |
| `GET /api/match/suggest-all` | Admin | Nothing further |
| `DELETE /api/match/suggest-all` | Admin | Nothing further |
| `GET /api/match/rules` | Admin | Nothing further |
| `PUT /api/match/rules` | Admin | Library exists. Each pattern is of a fixed set of words, up to 300 characters |
| `POST /api/match/rules/try` | Admin | Number of rules. Each pattern as above. Nothing saved |
| `GET /api/items/{id}` | Signed-in | limits. own progress. Paths for admins only |
| `PUT /api/items/{id}` | Admin | Item exists. Field values for disc settings |
| `GET /api/items/{id}/poster` | Signed-in; ticket; address token | limits. Pictures only, from the cache, the internet or inside a library |
| `GET /api/items/{id}/artwork` | Admin | Item exists |
| `GET /api/items/{id}/recommend` | Signed-in | limits, on the item and on what is suggested |
| `GET /api/items/{id}/disc` | Signed-in | limits. Item is a disc |
| `PUT /api/items/{id}/disc/labels` | Admin | Labels trimmed |
| `GET /api/items/{id}/disc/thumb/{ttn}/{n}` | Signed-in; ticket; address token | limits. Item is a disc and not scrambled. Title and chapter exist |
| `GET /api/artwork/preview` | Admin; address token | Web address, cache name, or a picture inside a library |
| `GET /api/prefs` | Signed-in | Nothing: server-wide playback defaults |
| `POST /api/themes` | Admin | ZIP of at most 26 MB, checked by the theme manager |
| `DELETE /api/themes/{id}` | Admin | Theme exists and may be removed |
| `GET /api/shows` | Signed-in | limits, in the query. Paths for admins only |
| `GET /api/shows/genres` | Signed-in | limits |
| `GET /api/shows/networks` | Signed-in | limits |
| `GET /api/shows/{id}` | Signed-in | limits, on the show and its episodes. Paths for admins only |
| `PUT /api/shows/{id}` | Admin | Show exists |
| `PUT /api/shows/{id}/view` | Admin | View is cards, list or empty |
| `GET /api/shows/{id}/artwork` | Admin | Show exists |
| `GET /api/shows/{id}/art/{kind}` | Signed-in; address token | limits. Pictures only |
| `GET /api/items/{id}/backdrop` | Signed-in; ticket; address token | limits. Pictures only |
| `POST /api/items/{id}/progress` | Signed-in | limits. own |
| `DELETE /api/items/{id}/progress` | Signed-in | own |
| `POST /api/pair/info` | Adult | Code, with a limit on wrong codes |
| `POST /api/pair/approve` | Adult | Code, with a limit on wrong codes. The device signs in as the caller or a profile the caller manages |
| `POST /api/pair/deny` | Adult | Code, with a limit on wrong codes |
| `POST /api/pair/invite` | Adult | Target is the caller or a profile the caller manages. One open code per person, limit on open codes |
| `GET /api/playback/profiles` | Signed-in | Nothing: the built-in client profiles |
| `GET /api/playback/speedtest` | Signed-in | Nothing: random bytes, 64 KB to 4 MB, sent within the limits for viewers away from home, as any stream is |
| `POST /api/items/{id}/playback` | Signed-in | limits. Known profile, start time, track numbers, sizes |
| `POST /api/items/{id}/stream-token` | Signed-in | limits. Needs a real sign-in |
| `GET /api/items/{id}/subtitles/{file}` | Signed-in; ticket | limits. Item is a video. Name is 'n.vtt' and a text track |
| `GET /api/items/{id}/play` | Signed-in | limits. Item is a video inside a library |
| `GET /api/items/{id}/file` | Signed-in; ticket; address token | limits. Video or book. Plain file inside a library once links are followed. Type from a fixed list. Limit for viewers away from home |
| `GET /api/items/{id}/stream` | Signed-in; ticket; address token | limits. Audio item. Plain file inside a library, or a feed address through the internet-only client. Limit for viewers away from home |
| `GET /api/cd/{id}` | Signed-in; ticket | limits. Disc image inside a library |
| `GET /api/cd/{id}/audio` | Signed-in; ticket | limits. Disc image inside a library. Format, tracks, start. Limit for viewers away from home |
| `GET /api/cd/{id}/graphics` | Signed-in; ticket | limits. Disc image inside a library. Sector range |
| `GET /api/cd/{id}/matches` | Admin | Disc image inside a library |
| `POST /api/cd/{id}/identify` | Admin | Disc image inside a library. Release id |
| `GET /api/cd-covers/{release}` | Admin | Release id looked up by the scanner |
| `GET /api/items/{id}/book` | Signed-in | limits. Book, plain file inside a library |
| `GET /api/items/{id}/page/{n}` | Signed-in; address token | limits. Book inside a library. Page exists. Restrictive content policy |
| `GET /api/items/{id}/epub/{file...}` | Signed-in; address token | limits. EPUB inside a library. Name cleaned and looked up in the archive. Restrictive content policy |
| `GET /api/items/{id}/annotations` | Signed-in | limits. Book inside a library. own |
| `POST /api/items/{id}/annotations` | Signed-in | limits. Book inside a library. Kind, colour, sizes, per-book count |
| `PUT /api/annotations/{aid}` | Signed-in | own. Colour and note only |
| `DELETE /api/annotations/{aid}` | Signed-in | own |
| `GET /api/items/{id}/search` | Signed-in | limits. Book inside a library |
| `GET /api/dictionary` | Signed-in | One word of up to 40 characters. Internet-only client |
| `GET /api/music/albums` | Signed-in | limits, in the query |
| `GET /api/music/artists` | Signed-in | limits, in the query |
| `GET /api/music/tracks` | Signed-in | limits, in the query. Paths for admins only |
| `GET /api/music/home` | Signed-in | limits, in the query. own plays. Paths for admins only |
| `GET /api/music/songs` | Signed-in | limits, in the query. own plays. Search of up to 200 characters, at most 5000 songs. Paths for admins only |
| `GET /api/music/likes` | Signed-in | own. limits, in the query |
| `GET /api/music/radio/{id}` | Signed-in | limits, on the song and what is suggested. 2 to 200 songs. Paths for admins only |
| `GET /api/music/loudness/{id}` | Signed-in | limits. An audio item inside a library. Values held to a range |
| `GET /api/music/extras/{id}` | Signed-in | limits. An audio item inside a library. PDFs and text files, plain files only, 50 at most |
| `GET /api/music/extras/{id}/file` | Signed-in | limits. Name is one the listing gave. Opened inside a library, plain files only. Text up to 512 KB, as JSON. A PDF is sent as a PDF for the browser's own viewer, without the script-free policy, within the limit for viewers away from home |
| `GET /api/music/artist` | Signed-in | limits, in the query. Name of up to 300 characters. Online biography only when the owner turned it on, its credit a Wikipedia address |
| `GET /api/music/artist-photo` | Signed-in | limits: an artist with a song the account may see. Name of up to 300 characters. Opened inside a library. Pictures only |
| `GET /api/music/lyrics/{id}` | Signed-in | limits. An audio item. own timing. Asks online only when the owner turned it on |
| `PUT /api/music/lyrics/{id}/offset` | Signed-in | limits. own. Up to two minutes either way |
| `POST /api/music/lyrics/{id}/search` | Admin | limits. An audio item. Online lyrics turned on. Query length |
| `PUT /api/music/lyrics/{id}` | Admin | limits. An audio item. At most 256 KB, or a search result from a source that is turned on. Nothing is written beside the music |
| `DELETE /api/music/lyrics/{id}` | Admin | limits. An audio item |
| `DELETE /api/music/lyrics-cache` | Admin | Nothing further |
| `GET /api/podcasts` | Signed-in | Library within the account's allowed libraries. Feed addresses for admins only |
| `GET /api/podcasts/search` | Admin | Query length |
| `POST /api/podcasts` | Admin | A podcasts library. Feed address checked by the scanner |
| `PUT /api/podcasts/{id}` | Admin | Podcast exists. Count held to 0 to 50 |
| `POST /api/podcasts/{id}/refresh` | Admin | Podcast exists |
| `DELETE /api/podcasts/{id}` | Admin | Podcast exists |
| `POST /api/items/{id}/download` | Admin | Audio item |
| `POST /api/items/{id}/dvd` | Signed-in | limits. Item is a disc. Opens a session with a random id |
| `GET /api/dvd/{sid}` | Signed-in | Session exists. An account with limits is checked against what it plays |
| `POST /api/dvd/{sid}/action` | Signed-in | Session exists. An account with limits is checked against what it plays |
| `DELETE /api/dvd/{sid}` | Signed-in | Session id. An account with limits is checked against what it plays |
| `GET /api/dvd/{sid}/menu/{serial}/overlay.png` | Signed-in; ticket; address token | Session exists, and for a ticket is of the ticket's item. Menu picture exists. An account with limits is checked against what it plays |
| `GET /api/dvd/{sid}/menu/{serial}/highlight/{n}` | Signed-in; ticket; address token | Session exists, and for a ticket is of the ticket's item. Picture exists. An account with limits is checked against what it plays |
| `GET /api/dvd/{sid}/menu/{serial}/effect/{n}` | Signed-in; address token | Session exists. Picture exists. An account with limits is checked against what it plays |
| `GET /api/dvd/{sid}/sound/{n}` | Signed-in | Session exists. Sound exists. An account with limits is checked against what it plays |
| `GET /api/dvd/{sid}/bdj` | Signed-in | Blu-ray session exists. An account with limits is checked against what it plays |
| `GET /api/dvd/{sid}/bdj/frame/{n}` | Signed-in | Blu-ray session exists. An account with limits is checked against what it plays |
| `GET /api/bdj/status` | Admin | Nothing further |
| `GET /api/dvd/{sid}/still/{name}` | Signed-in; ticket; address token | Session exists, and for a ticket is of the ticket's item. Still is one the session made. An account with limits is checked against what it plays |
| `GET /api/dvd/{sid}/subs` | Signed-in; address token | Session exists. Time window of at most 600 seconds. An account with limits is checked against what it plays |
| `GET /api/dvd/{sid}/sub/{name}` | Signed-in; ticket; address token | Session exists, and for a ticket is of the ticket's item. Picture exists. An account with limits is checked against what it plays |
| `GET /api/hls/{id}/master.m3u8` | Signed-in; ticket; address token | Stream exists, and for a ticket is of the ticket's item. An account with limits is checked against what it plays |
| `GET /api/hls/{id}/subs/{file}` | Signed-in; ticket; address token | Stream exists, and for a ticket is of the ticket's item. Name is 'n.m3u8' or 'n.vtt' and the track exists. An account with limits is checked against what it plays |
| `GET /api/hls/{id}/playlist.m3u8` | Signed-in; ticket; address token | Stream exists, and for a ticket is of the ticket's item. An account with limits is checked against what it plays |
| `GET /api/hls/{id}/seg/{n}` | Signed-in; ticket; address token | Stream exists, and for a ticket is of the ticket's item. Segment number. An account with limits is checked against what it plays |
| `DELETE /api/hls/{id}` | Signed-in | Removes the caller as a viewer. The stream stops when no viewer is left. An account with limits is checked against what it plays |
| `GET /api/settings` | Admin | Secrets masked |
| `PUT /api/settings` | Admin | Known keys. Numbers, lists and FFmpeg options checked for each setting |
| `GET /api/about` | Signed-in | Nothing: version and engines |
| `GET /api/licenses` | Signed-in | Nothing |
| `GET /api/licenses/{id}` | Signed-in | Numeric id |
| `GET /api/legal` | Public | Nothing |
| `GET /api/legal/notices` | Public | Nothing |
| `POST /api/legal/acknowledge` | Admin | Nothing further |
| `GET /api/branding/logo` | Public | Nothing |
| `PUT /api/branding/logo` | Admin | PNG or JPEG of at most 2 MB, 16 to 4096 pixels a side |
| `DELETE /api/branding/logo` | Admin | Nothing further |
| `GET /api/system` | Admin | Nothing further |
| `GET /api/users` | Signed-in | Lists only accounts the caller may manage: all for an admin, Kids profiles for an adult without limits while the owner allows that |
| `POST /api/users` | Signed-in | Caller may manage that kind of profile. Administrator flag for admins only. Name and password rules |
| `DELETE /api/users/{id}` | Signed-in | Caller may manage that account, and it isn't their own |
| `PUT /api/users/{id}/access` | Signed-in | Caller may manage that account. Not their own, not an administrator's |
| `PUT /api/users/{id}/password` | Signed-in | Own password for adults, with the current one and an attempt limit. Another account's if the caller may manage it. Length rules |
| `DELETE /api/users/{id}/2fa` | Admin | Not their own, not another administrator's |
| `GET /api/fs` | Admin | Folder listing through the file browser |
| `POST /api/fs/pick` | Admin | Only from a browser on the server's own machine |
| `POST /api/metadata/test` | Admin | Provider and key |
| `GET /api/agents` | Admin | Header values masked |
| `PUT /api/agents` | Admin | At most 20. Each agent validated. Unique ids |
| `POST /api/agents/test` | Admin | Agent validated. Title present |
| `GET /api/logs` | Admin | Nothing further |
| `PUT /api/logs` | Admin | Known levels and categories |
| `GET /api/logs/files/{name}` | Admin; address token | Name is a log file's name |
| `POST /api/logs/open` | Admin | Only from a browser on the server's own machine |
| `POST /api/logs/client` | Signed-in | Body limit. Event count. Rate per account. Only when the owner accepts client logs |
| `GET /api/logs/entries` | Admin | Filters |
| `GET /api/logs/summary` | Admin | Nothing further |
| `GET /api/logs/crashes` | Admin | Nothing further |
| `GET /api/logs/crashes/{id}` | Admin | Report id |
| `POST /api/logs/crashes/{id}/seen` | Admin | Report id |
| `DELETE /api/logs/crashes/{id}` | Admin | Report id |
| `GET /api/logs/bundle` | Admin | Days, redaction |
| `GET /api/logs/bundle/preview` | Admin | Days, redaction |
| `GET /api/reset` | Admin | Nothing further |
| `POST /api/reset` | Admin | Types RESET. Own password, and a code with two-factor sign-in. Folder layout checked |
| `POST /api/reset/restore` | Admin | Own password, and a code with two-factor sign-in. Copy name |
| `DELETE /api/reset/request` | Admin | Nothing further |
| `POST /api/reset/copies/delete` | Admin | Own password, and a code with two-factor sign-in. Copy name |
| `GET /api/update` | Admin | Build can update itself |
| `POST /api/update/check` | Admin | Build can update itself. Answers younger than a minute are reused |
| `POST /api/update/apply` | Admin | Signed release, checked by the updater |
| `GET /api/modulestore` | Admin | Nothing further |
| `POST /api/modulestore/check` | Admin | Answers younger than a minute are reused for this project's catalogue |
| `GET /api/modulestore/item/{store}/{id}` | Admin | Known store and module |
| `POST /api/modulestore/install` | Admin | Known module. Signed version. Says what else comes with it until confirmed |
| `POST /api/modulestore/update-all` | Admin | One run at a time. Signed versions |
| `GET /api/modulestore/run` | Admin | Nothing further |
| `POST /api/modulestore/run/undo` | Admin | Own password, and a code with two-factor sign-in |
| `DELETE /api/modulestore/run` | Admin | Nothing further |
| `GET /api/modulestore/modules/{id}/log` | Admin | Known module. Records are redacted when written |
| `GET /api/modulestore/media/{store}/{sha}` | Admin | A picture a catalogue lists, checked against its hash, PNG or JPEG. At most 64 MB kept for one store, and what it stops listing goes. No token in the address |
| `POST /api/modulestore/stores/preview` | Admin | Public addresses only. The body can allow the local network, with own password and a code with two-factor sign-in, as what comes back says what's listening. Size limits. Keeps nothing |
| `POST /api/modulestore/stores` | Admin | Warning agreed to. Own password, and a code with two-factor sign-in. Signature checked against the key pinned |
| `DELETE /api/modulestore/stores/{id}` | Admin | Known store. What uses it goes back to the official version |
| `PUT /api/modulestore/stores/{id}/enabled` | Admin | Known store. Body says on or off |
| `POST /api/modulestore/stores/{id}/check` | Admin | Known store. Its pinned key has to match |
| `POST /api/modulestore/stores/{id}/fork/{module}` | Admin | Known store and module. Confirmation, with own password and a code with two-factor sign-in. The key the admin was shown. Refused for WittyTuner, where the machine's owner keeps forks out, and where this system can't box one in |
| `GET /api/modules` | Admin | Nothing further |
| `POST /api/modules/check` | Admin | Answers younger than a minute are reused |
| `POST /api/modules/{id}/install` | Admin | Known module. Signed version. Confirmation for a version with a warning, with own password and a code with two-factor sign-in for one older than what is in use |
| `POST /api/modules/{id}/rollback` | Admin | Known module. Own password, and a code with two-factor sign-in, once confirmed |
| `GET /api/modules/{id}/versions` | Admin | Known module |
| `POST /api/modules/{id}/pin` | Admin | Known module |
| `DELETE /api/modules/{id}/pin` | Admin | Known module |
| `PUT /api/modules/{id}/autoupdate` | Admin | Known module. Body says on or off |
| `POST /api/modules/{id}/latest` | Admin | Known module |
| `PUT /api/modules/{id}/source` | Admin | Known module. Repository and key. Confirmation, with own password and a code with two-factor sign-in |
| `DELETE /api/modules/{id}/source` | Admin | Known module |
| `POST /api/modules/{id}/add` | Admin | Known module |
| `POST /api/modules/{id}/remove` | Admin | Known module. No library needs it |
| `POST /api/modules/{id}/enable` | Admin | Known module |
| `POST /api/modules/{id}/disable` | Admin | Known module. No library needs it |
| `PUT /api/modules/installed` | Admin | Known modules. No library needs what is left out |
| `GET /api/modules/libraries` | Signed-in | Nothing: which kinds of library exist |
| `PUT /api/modules/libraries/{type}/label` | Admin | Known type. Label rules |
| `GET /api/modules/web` | Signed-in | Nothing: the bundles in use |
| `GET /api/modules/{id}/icon` | Signed-in | Known module. Only what the signed catalogue names, kept on this server |
| `GET /api/modules/{id}/notices` | Signed-in | Known module. Only the notices file of the version in use |
| `GET /api/module-store/{id}/{key}` | Signed-in | own. Known module. A key of letters and digits |
| `PUT /api/module-store/{id}/{key}` | Signed-in | own. Known module. A key of letters and digits. JSON of at most 64 KB, and 32 keys for a module |
| `GET /api/modules/{id}/web/{version}/{path...}` | Signed-in; address token | Version in use. Name in the bundle's signed list, of a listed file type, hash checked when served |
| `POST /api/modules/{id}/failed` | Signed-in | Known module. 4 KB body. Rate per account. Counts against a version only from an admin |
| `GET /api/reach/{nonce}` | Public | Nonce of a running outside test |
| `GET /api/remote` | Admin | Nothing further |
| `GET /api/remote/status` | Signed-in, or this computer | no account only from the server's own computer, directly (the tray). What is wrong in a fault, which can name files, only for an admin |
| `PUT /api/remote` | Admin | Method, outside address, network lists, limits, ports, HTTPS mode and its fields. Turning on unencrypted connections from away also needs `acceptPlainAway` and the admin's own password (and code) |
| `POST /api/remote/test` | Admin | Address is one the server offered |
| `POST /api/remote/selfcheck` | Admin | Outside address is set |
| `POST /api/remote/router` | Admin | Nothing further |
| `POST /api/remote/obtain` | Admin | HTTPS is on, in certificate-request mode |
| `DELETE /api/remote/lockouts` | Admin | Nothing further |
| `DELETE /api/remote/plain-sessions` | Admin | Nothing further |
| `GET /api/sessions` | Signed-in | own. Everyone's with 'all=1', for an admin |
| `DELETE /api/sessions` | Signed-in | own, except the current one |
| `DELETE /api/sessions/{id}` | Signed-in | own. Any account's for an admin |
| `GET /api/me/2fa` | Signed-in | own |
| `POST /api/me/2fa/setup` | Signed-in | Own password, with an attempt limit |
| `POST /api/me/2fa/confirm` | Signed-in | Code, with an attempt limit |
| `POST /api/me/2fa/disable` | Signed-in | Own password and code, with an attempt limit |
| `GET /api/photos/months` | Signed-in | A photos library the account may open. limits. Album visible to the account |
| `GET /api/photos/list` | Signed-in | A photos library the account may open. limits. Album visible to the account |
| `GET /api/photos/folders` | Signed-in | A photos library the account may open. limits |
| `GET /api/photos/map` | Signed-in | A photos library the account may open. limits. Box of four numbers |
| `GET /api/photos/ai` | Signed-in | Nothing: reserved |
| `GET /api/photos/albums` | Signed-in | A photos library the account may open. Own and shared albums. limits on counts |
| `POST /api/photos/albums` | Adult | A photos library the account may open. Name and description sizes |
| `PUT /api/photos/albums/{id}` | Signed-in | Album's owner or an admin. Sizes. Cover is a visible picture of that library |
| `DELETE /api/photos/albums/{id}` | Signed-in | Album's owner or an admin |
| `POST /api/photos/albums/{id}/items` | Signed-in | Album's owner or an admin. limits on each picture. Same library. 1 to 2000 |
| `DELETE /api/photos/albums/{id}/items` | Signed-in | Album's owner or an admin. 1 to 2000 |
| `GET /api/photos/{id}` | Signed-in | limits. Lists only the caller's own and shared albums |
| `GET /api/items/{id}/image` | Signed-in | limits. Photo or video. Type from a fixed list |
| `GET /api/items/{id}/original` | Signed-in | limits. Photo or video. Type from a fixed list. Sent as a download |
| `GET /api/items/{id}/motion` | Signed-in | limits. Clip's path is inside a library |
| `POST /api/libraries/{id}/upload` | Adult | Uploads turned on. A photos or shorts library the account may open. Name cleaned, listed extension, first bytes checked, never replaces a file. Size per request from settings |
| `GET /api/creators` | Signed-in | A creator library the account may open. limits |
| `GET /api/creators/{id}` | Signed-in | Creator has a video the account may see |
| `PUT /api/creators/{id}` | Admin | Field sizes. Name present |
| `GET /api/creators/{id}/art/{kind}` | Signed-in | Creator has a video the account may see. Pictures only |
| `PUT /api/creators/{id}/subscribe` | Signed-in, not Guest | Creator has a video the account may see |
| `GET /api/videos` | Signed-in | A creator library the account may open. limits. No paths in the answer |
| `GET /api/videos/feed` | Signed-in | A creator library the account may open. limits |
| `GET /api/videos/tags` | Signed-in | A creator library the account may open. limits |
| `GET /api/videos/{id}` | Signed-in | limits |
| `PUT /api/videos/{id}` | Admin | Item exists. Field sizes, date, web address |
| `GET /api/videos/{id}/related` | Signed-in | limits |
| `PUT /api/items/{id}/reaction` | Signed-in, not Guest | limits. Value is 1, 0 or -1 |
| `POST /api/items/{id}/view` | Signed-in | limits. own |
| `GET /api/items/{id}/comments` | Signed-in | limits |
| `POST /api/items/{id}/comments` | Adult | limits. Text cleaned, up to 2000 characters. A reply answers a comment on the same item |
| `PUT /api/comments/{cid}` | Signed-in | Comment's writer, who may still see the item, or an admin. Text rules |
| `DELETE /api/comments/{cid}` | Signed-in | Comment's writer, who may still see the item, or an admin |
| `GET /api/collections` | Signed-in | Own and shared. limits on counts and covers |
| `POST /api/collections` | Adult | Name and description sizes. Rule fields |
| `POST /api/collections/preview` | Signed-in | Rule fields. limits |
| `GET /api/collections/holding` | Signed-in | own |
| `POST /api/collections/watch-later/items` | Signed-in, not Guest | limits on each title. 1 to 2000 |
| `GET /api/collections/{id}` | Signed-in | Own or shared, or an admin. limits on what it holds. Paths for admins only |
| `PUT /api/collections/{id}` | Signed-in | Collection's owner or an admin. Sizes. Rule fields |
| `DELETE /api/collections/{id}` | Signed-in | Collection's owner or an admin. Not a system list |
| `POST /api/collections/{id}/items` | Signed-in | Collection's owner or an admin. Not a smart list. limits on each title. 1 to 2000 |
| `DELETE /api/collections/{id}/items` | Signed-in | Collection's owner or an admin. 1 to 2000 |
| `GET /api/radio` | Signed-in | A radio library the account may open. limits. Stream addresses for admins only |
| `POST /api/radio` | Admin | A radio library. Station fields |
| `POST /api/radio/import` | Admin | A radio library. Playlist parsed |
| `POST /api/radio/check` | Admin | A radio library |
| `GET /api/radio/directory/search` | Admin | Directory turned on |
| `GET /api/radio/directory/browse` | Admin | Directory turned on |
| `POST /api/radio/directory/add` | Admin | Directory turned on. A radio library |
| `GET /api/radio/{id}` | Signed-in | limits. A station. Stream address for admins only |
| `DELETE /api/radio/{id}` | Admin | Numeric id |
| `GET /api/radio/{id}/now` | Signed-in | limits. A station |
| `POST /api/radio/{id}/check` | Admin | A station |
| `POST /api/radio/{id}/lookup` | Admin | Directory turned on. A station |
| `GET /api/radio/{id}/stream` | Signed-in; ticket; address token | limits. A station with a valid address. Limits on streams at once. Type checked. Internet-only client |
| `GET /api/radio/{id}/hls` | Signed-in; ticket; address token | limits. A station. Address sealed by this server for this station. Type checked. Internet-only client |
| `GET /api/me/review` | Signed-in | own. Year from 2000 to 3000 |
| `DELETE /api/me/history` | Signed-in | own |
| `GET /api/me/recommended` | Signed-in | own record. limits. Count up to 50 |
| `GET /api/shares` | Signed-in | own. Everyone's with 'all=1', for an admin |
| `POST /api/shares` | Adult | Links turned on. Item: limits, photo or video. Album: visible to the caller. Days from 0 to 3650 |
| `DELETE /api/shares/{id}` | Signed-in | Link's maker or an admin |
| `GET /api/share/{token}` | Public, with the link's secret | Links turned on. Secret's hash, end date. The maker's limits |
| `GET /api/share/{token}/image/{id}` | Public, with the link's secret | As above. Item is what the link names. Always re-encoded |
| `GET /api/share/{token}/original/{id}` | Public, with the link's secret | As above. Item is what the link names. Link allows download |
| `GET /api/share/{token}/play/{id}` | Public, with the link's secret | As above. Item is what the link names, and a video |
| `GET /api/share/{token}/file/{id}` | Public, with the link's secret | As above. Item is what the link names, a browser-safe video with no location |
| `GET /api/share/{token}/hls/{id}/playlist.m3u8` | Public, with the link's secret | As above. Item is what the link names, and a video |
| `GET /api/share/{token}/hls/{id}/seg/{n}` | Public, with the link's secret | As above. Item is what the link names, and a video. Segment number |
| `GET /api/guide` | Signed-in | Channels the account may watch. Time range, hours, paging |
| `GET /api/tv/{key}/tune` | Signed-in | Channel key. Channel visible to the account, enabled, and not encrypted. For a library channel, limits on what is on now |
| `GET /api/tv/{key}/live/playlist.m3u8` | Signed-in | Channel key. Channel checked again on each request |
| `GET /api/tv/{key}/live/seg/{name}` | Signed-in | Channel key. Channel checked again on each request. Name is a segment the stream made |
| `POST /api/tv/{key}/leave` | Signed-in | Channel key. Viewer name of up to 40 characters |
| `GET /api/tv/settings` | Admin | Nothing further |
| `PUT /api/tv/settings` | Admin | Known keys. Value checked for each |
| `PUT /api/libraries/{id}/main` | Admin | A commercials library |
| `GET /api/channels` | Signed-in | Channels visible to the account. Definitions for admins only |
| `POST /api/channels` | Admin | Definition validated |
| `POST /api/channels/preview` | Admin | Definition validated. Day from 0 to 14 |
| `GET /api/channels/suggestions` | Admin | Nothing further |
| `GET /api/channels/options` | Admin | Nothing further |
| `POST /api/channels/from-network` | Admin | Network name of up to 200 characters. Definition validated |
| `POST /api/channels/networks/refresh` | Admin | Nothing further |
| `GET /api/channels/networks/refresh` | Admin | Nothing further |
| `GET /api/channels/{id}` | Admin | Channel exists |
| `PUT /api/channels/{id}` | Admin | Channel exists. Definition validated |
| `DELETE /api/channels/{id}` | Admin | Channel exists |
| `POST /api/channels/{id}/rebuild` | Admin | Channel exists |
| `GET /api/channels/{id}/history` | Admin | Numeric id |
| `POST /api/channels/{id}/restore` | Admin | Channel and history entry exist |
| `GET /api/channels/{id}/logo` | Signed-in | Numeric id |
| `PUT /api/channels/{id}/logo` | Admin | Channel exists. PNG or JPEG of at most 2 MB, 16 to 4096 pixels a side |
| `DELETE /api/channels/{id}/logo` | Admin | Numeric id |
| `GET /api/tv/encrypted-help` | Signed-in | Nothing: fixed text |
| `GET /api/tv/{key}/logo` | Signed-in | Channel key. Account may watch tuner channels, channel enabled. Web address only, internet-only client, pictures only |
| `GET /api/tuners/drivers` | Admin | Nothing further |
| `GET /api/tuners/discover` | Admin | 12-second limit |
| `GET /api/tuners` | Admin | Options masked |
| `POST /api/tuners` | Admin | Driver and options checked by the tuner manager |
| `PUT /api/tuners/{id}` | Admin | Tuner exists. Options checked by the tuner manager |
| `DELETE /api/tuners/{id}` | Admin | Tuner exists |
| `POST /api/tuners/{id}/refresh` | Admin | Tuner exists |
| `POST /api/tuners/{id}/scan` | Admin | Tuner exists. Source |
| `GET /api/tuners/{id}/scan` | Admin | Numeric id |
| `GET /api/tuners/{id}/channels` | Admin | Numeric id |
| `POST /api/tuners/{id}/channels/enable` | Admin | Numeric id |
| `PUT /api/tuner-channels/{id}` | Admin | Channel exists. Fields |
| `GET /api/epg/sources` | Admin | Nothing further |
| `POST /api/epg/sources` | Admin | Name and address checked by the tuner manager |
| `DELETE /api/epg/sources/{id}` | Admin | Source exists |
| `POST /api/epg/sources/{id}/refresh` | Admin | Numeric id |
| `GET /api/epg/ids` | Admin | Nothing further |
| `GET /api/recordings` | Tuner channels | own, and every account's for an administrator |
| `POST /api/recordings` | Tuner channels | Channel turned on and visible. 50 waiting per account, none for an administrator |
| `DELETE /api/recordings/{id}` | Tuner channels | own, and every account's for an administrator |
| `GET /api/recording-rules` | Tuner channels | own, and every account's for an administrator |
| `POST /api/recording-rules` | Tuner channels | Channel turned on and visible. 20 per account, none for an administrator |
| `DELETE /api/recording-rules/{id}` | Tuner channels | own, and every account's for an administrator |
| `GET /api/games/consoles` | Signed-in | Counts: limits. Custom core assignments for admins only |
| `GET /api/games/modes` | Signed-in | Client name |
| `GET /api/consoles/{console}/controls` | Signed-in | Known console. own layouts. Game: limits |
| `PUT /api/consoles/{console}/controls` | Signed-in | Known console. own. Size, kind, device name, layout cleaned |
| `POST /api/games/{id}/prepare` | Signed-in | limits. A game on a known, available console |
| `POST /api/games/{id}/play` | Signed-in | limits. A game on a known, available console. Core allowed, file refused if encrypted. Limit on games at once |
| `GET /api/games/{id}/saves` | Signed-in | limits. A game. own folder, built from numbers |
| `GET /api/games/{id}/saves/{slot}/thumb` | Signed-in; address token | limits. A game. Slot number. own folder |
| `DELETE /api/games/{id}/saves/{slot}` | Signed-in | limits. A game. Slot number. own folder |
| `GET /api/games/{id}/saves/{slot}/file` | Signed-in; address token | limits. A game. Slot number. own folder |
| `PUT /api/games/{id}/saves/{slot}/file` | Signed-in | limits. A game. Slots 1 and up. At most 256 MB, header checked, limits per game and per account, imports at once |
| `GET /api/games/{id}/battery` | Signed-in; address token | limits. A game. own folder |
| `PUT /api/games/{id}/battery` | Signed-in | limits. A game not being played. At most 64 MB, limits per game and per account, imports at once |
| `GET /api/play/{sid}` | Signed-in | Session started by this account |
| `DELETE /api/play/{sid}` | Signed-in | Session started by this account |
| `POST /api/play/{sid}/webrtc` | Signed-in | Session started by this account. Offer size |
| `GET /api/play/{sid}/ws` | Signed-in; address token | Session started by this account. WebSocket: origin must match the host, messages limited in size |
| `POST /api/play/{sid}/action` | Signed-in | Session started by this account. Known action |
| `GET /api/play/{sid}/stats` | Signed-in | Session started by this account |
| `GET /api/play/{sid}/screenshot` | Signed-in; address token | Session started by this account |
| `GET /api/game-controllers` | Signed-in | Nothing: known controllers |
| `GET /api/game-profiles` | Signed-in | own |
| `POST /api/game-profiles` | Signed-in | own. Size, fields cleaned. Game: limits, and on that console |
| `GET /api/game-profiles/resolve` | Signed-in | Known console, kind. own. Game: limits |
| `GET /api/game-profiles/export` | Signed-in | own |
| `POST /api/game-profiles/import` | Signed-in | own. File size, each profile cleaned. Game: limits |
| `GET /api/game-profiles/{id}` | Signed-in | own |
| `PUT /api/game-profiles/{id}` | Signed-in | own. Size, fields cleaned. Game: limits |
| `DELETE /api/game-profiles/{id}` | Signed-in | own |
| `POST /api/game-profiles/{id}/copy` | Signed-in | own |
| `POST /api/game-profiles/{id}/active` | Signed-in | own |
| `GET /api/game-background` | Signed-in | own |
| `PUT /api/game-background` | Signed-in | own. Fields |
| `DELETE /api/game-background` | Signed-in | own |
| `GET /api/game-background/image` | Signed-in; address token | own |
| `PUT /api/game-background/image` | Signed-in | own. PNG, JPEG or WebP of at most 10 MB and 24 megapixels, drawn again as a new JPEG |
| `GET /api/gaming` | Admin | Nothing further |
| `POST /api/gaming/cores/{id}/install` | Admin | Core id format. Offered by this server |
| `DELETE /api/gaming/cores/{id}` | Admin | Core id format. Installed by this server |
| `DELETE /api/gaming/unpacked` | Admin | Nothing further |
| `GET /api/games/settings/schema` | Signed-in | Nothing: fixed description |
| `GET /api/games/settings` | Signed-in | Console known. Game: limits. own |
| `PUT /api/games/settings` | Signed-in | Console known. Game: limits. own levels; server levels for admins. Size, settings checked |
| `POST /api/games/settings/validate` | Signed-in | Console known. Game: limits. Size. Nothing saved |
| `GET /api/games/recent` | Signed-in | own list. limits on each game. Paths for admins only |
| `GET /api/game-bezels` | Signed-in | own |
| `POST /api/game-bezels` | Signed-in | own. At most 16 per account, 16 MB, 16 megapixels. Decoded and written as a new PNG |
| `GET /api/game-bezels/{bezel}` | Signed-in; address token | own |
| `DELETE /api/game-bezels/{bezel}` | Signed-in | own |
| `GET /api/games/{id}/core-options` | Signed-in | limits. A game |
| `GET /api/games/{id}/meta` | Signed-in | limits. A game. own |
| `PUT /api/games/{id}/meta` | Signed-in | limits. A game. own. Note of up to 8 KB |
| `GET /api/games/{id}/cheats` | Signed-in | limits. A game. own |
| `PUT /api/games/{id}/cheats` | Signed-in | limits. A game. own. Size, list cleaned |
| `POST /api/games/{id}/cheats/import` | Signed-in | limits. A game. Size, file parsed. Nothing saved |
| `GET /api/games/{id}/patches` | Signed-in | limits. A game. own |
| `POST /api/games/{id}/patches` | Signed-in | limits. A game. own. Patch format and size, 16 per game, limit per account, imports at once |
| `PUT /api/games/{id}/patches/{patch}` | Signed-in | limits. A game. own |
| `DELETE /api/games/{id}/patches/{patch}` | Signed-in | limits. A game. own |
| `GET /api/gaming/custom` | Admin | Nothing further |
| `PUT /api/gaming/custom/enabled` | Admin | Own password to turn on |
| `POST /api/gaming/custom/staged/upload` | Admin | Custom cores on. Own password. Package size and contents |
| `POST /api/gaming/custom/staged/url` | Admin | Custom cores on. Own password. Address, hash |
| `POST /api/gaming/custom/staged/placed` | Admin | Custom cores on. Own password. Key from the server's own list |
| `POST /api/gaming/custom/staged/{token}/approve` | Admin | Custom cores on. Staged token. Acknowledgement and fields |
| `DELETE /api/gaming/custom/staged/{token}` | Admin | Staged token |
| `PUT /api/gaming/custom/cores/{id}` | Admin | Custom cores on. Core exists. Fields |
| `DELETE /api/gaming/custom/cores/{id}` | Admin | Core exists |
| `POST /api/gaming/custom/cores/{id}/reapprove` | Admin | Custom cores on. Own password |
| `POST /api/gaming/custom/cores/{id}/rollback` | Admin | Custom cores on. Own password. Hash format |
| `POST /api/gaming/custom/cores/{id}/files` | Admin | Custom cores on. Own password. Package size and contents |
| `POST /api/gaming/custom/consoles` | Admin | Custom cores on. Console JSON size and fields |
| `PUT /api/gaming/custom/consoles/{id}` | Admin | Custom cores on. Console JSON size and fields |
| `DELETE /api/gaming/custom/consoles/{id}` | Admin | No library is set to it |
| `PUT /api/gaming/consoles/{console}/core` | Admin | Known console. Core allowed for it |
| `GET /api/gaming/games/{id}/core` | Admin | A game |
| `PUT /api/gaming/games/{id}/core` | Admin | A game. Core allowed for it |
| `GET /api/games/facets` | Signed-in | Library within the account's allowed libraries. limits |
| `GET /api/games/{id}/metadata` | Signed-in | limits. A game. File hashes and serials for admins only |
| `GET /api/games/{id}/art/{n}` | Signed-in | limits. A game. Picture number. Pictures only |
| `GET /api/games/{id}/metadata/search` | Admin | A game. Query length |
| `PUT /api/games/{id}/metadata/match` | Admin | A game. Source and id |
| `DELETE /api/games/{id}/metadata/match` | Admin | A game |
| `POST /api/games/{id}/metadata/refresh` | Admin | A game |
| `GET /api/gamemeta/status` | Admin | Nothing further |
| `POST /api/gamemeta/refetch` | Admin | Nothing further |
| `GET /api/gamemeta/sources` | Admin | Nothing further |
| `PUT /api/gamemeta/sources` | Admin | Source list |
| `POST /api/gamemeta/sources/{id}/test` | Admin | Known source. Nothing saved |
| `GET /api/rigs` | Signed-in | Game PCs with a game the account may see. Addresses for admins only |
| `GET /api/rigs/find` | Admin | Nothing further |
| `POST /api/rigs` | Admin | Address cleaned. Confirmation for an internet address. Must answer as a game PC |
| `PATCH /api/rigs/{id}` | Admin | Game PC exists. Name size, address cleaned, confirmation for an internet address |
| `DELETE /api/rigs/{id}` | Admin | Game PC exists |
| `POST /api/rigs/{id}/pair` | Admin | Game PC exists |
| `GET /api/rigs/{id}/pair` | Admin | Game PC exists |
| `DELETE /api/rigs/{id}/pair` | Admin | Game PC exists |
| `POST /api/rigs/{id}/devices` | Admin | Game PC exists and is paired. PIN |
| `POST /api/rigs/{id}/sync` | Admin | Game PC exists |
| `GET /api/rigs/{id}/apps` | Admin | Game PC exists |
| `PUT /api/rigs/{id}/apps/{app}` | Admin | Game PC exists. App id |
| `POST /api/rigs/{id}/wake` | Signed-in | Game PC has a game the account may see. Has a MAC address. One signal per 10 seconds |
| `POST /api/rigs/{id}/quit` | Signed-in | Game PC has a game the account may see. Whoever started what is running, or an admin |
| `POST /api/rigs/games/{id}/play` | Signed-in | limits. A game of a game PC. Client and system from fixed lists. Starts nothing |
| `POST /api/rigs/games/{id}/start` | Signed-in | limits. A game of a game PC. Refused if another game is running |
| `POST /api/rigs/games/{id}/stream` | Signed-in | limits. A game of a game PC. Nothing else running, or the caller started it |
| `GET /api/couch` | Signed-in | Answers for the account's access to game PCs |
| `GET /api/couch/find` | Signed-in with game PC access | Owner's access and network settings. Attempt limit per person. One search at a time |
| `GET /api/couch/hosts` | Signed-in with game PC access | own |
| `POST /api/couch/hosts` | Signed-in with game PC access | Address cleaned. Owner's network and port settings. Count and attempt limits. Must answer as a game PC |
| `PATCH /api/couch/hosts/{id}` | Signed-in with game PC access | own. Name |
| `DELETE /api/couch/hosts/{id}` | Signed-in with game PC access | own |
| `POST /api/couch/hosts/{id}/pair` | Signed-in with game PC access | own. Attempt limit |
| `GET /api/couch/hosts/{id}/pair` | Signed-in with game PC access | own |
| `DELETE /api/couch/hosts/{id}/pair` | Signed-in with game PC access | own |
| `GET /api/couch/hosts/{id}/apps` | Signed-in with game PC access | own |
| `GET /api/couch/hosts/{id}/apps/{app}/art` | Signed-in with game PC access | own. App id, size limit, pictures only |
| `POST /api/couch/hosts/{id}/apps/{app}/play` | Signed-in with game PC access | own. Request fields cleaned |
| `POST /api/couch/hosts/{id}/wake` | Signed-in with game PC access | own. Has a MAC address, not an internet address. One signal per 10 seconds |
| `POST /api/couch/hosts/{id}/quit` | Signed-in with game PC access | own |
| `GET /api/couch/sessions/{sid}` | Signed-in | Stream started by this account |
| `POST /api/couch/sessions/{sid}/webrtc` | Signed-in | Stream started by this account. Offer of up to 256 KB |
| `DELETE /api/couch/sessions/{sid}` | Signed-in | Stream started by this account |
| `GET /api/wittyrigs` | Admin | Nothing further |
| `PUT /api/wittyrigs` | Admin | Body says on or off |
| `GET /api/diagnose/targets` | Admin | Nothing further |
| `GET /api/diagnose/browse` | Admin | Path resolved inside the allowed places |
| `POST /api/diagnose/disc` | Admin | Path resolved inside the allowed places |
| `POST /api/diagnose/storage` | Admin | Targets from the server's own list |
| `GET /api/diagnose/jobs` | Admin | Nothing further |
| `GET /api/diagnose/jobs/{id}` | Admin | Job exists |
| `DELETE /api/diagnose/jobs/{id}` | Admin | Job exists |
| `GET /api/diagnose/jobs/{id}/report` | Admin | Job exists. Format, redaction |
| `GET /api/guide/search` | Signed-in | Channels the account may watch, and limits on what they show. 2 to 100 letters, up to two weeks ahead. A few searches at a time |
| `GET /api/tv/favourites` | Signed-in | own |
| `PUT /api/tv/favourites/{key}` | Signed-in | own. A channel the account may watch. A limit per account |
| `DELETE /api/tv/favourites/{key}` | Signed-in | own. A channel the account may watch |
| `GET /api/tv/reminders` | Signed-in | own |
| `POST /api/tv/reminders` | Signed-in | own. A channel the account may watch. Title of up to 300 characters, a time ahead. A limit per account |
| `DELETE /api/tv/reminders/{id}` | Signed-in | own |
| `GET /api/channels/bumpers` | Admin | Nothing further |
| `POST /api/channels/bumpers` | Admin | A video that exists |
| `POST /api/channels/bumpers/upload` | Admin | Name cleaned, video extension, first bytes checked, never replaces a file. At most 2 GB and 30 minutes. Keeps room free on the disk. Read as a video before it is kept |
| `DELETE /api/channels/bumpers/item/{id}` | Admin | Bumper exists. A file is deleted only in the bumpers folder |
| `GET /api/channels/discs` | Admin | Nothing further |
| `POST /api/channels/clip-preview` | Admin | A disc that exists. Title exists. Encrypted discs are refused |
| `GET /api/backup` | An admin, or before any account exists this machine or the setup code | Backups kept, what's going on, and a restore waiting. Before any account exists: no folder, schedule or settings |
| `GET /api/backup/estimate` | Admin | Nothing further |
| `POST /api/backup/create` | Admin | Known parts, and what each needs. A password of at least 8 characters, if given. One job at a time. The file stays on the server until a download asks for the password |
| `POST /api/backup/cancel` | An admin, or before any account exists this machine or the setup code | Nothing: stops the job going |
| `PUT /api/backup/schedule` | Admin | Frequency, hour, weekday, keep of 1 to 365, known parts. A password of at least 8 characters, kept only as a key made from it. Keeping fewer than before takes the account's password, and code with two-factor sign-in |
| `DELETE /api/backup/files/{name}` | Admin | Name has the form of one of this folder's backups, and is a plain file there. The account's password, and code with two-factor sign-in, with an attempt limit |
| `POST /api/backup/files/{name}/link` | Admin | The account's password, and code with two-factor sign-in, with an attempt limit. Name as for deleting. A ticket for that one file, for 10 minutes |
| `GET /api/backup/download` | Public, with a ticket | The ticket (`?st=`) names one file and lasts 10 minutes. It isn't a sign-in, and opens nothing else |
| `POST /api/backup/restore/upload` | An admin, or before any account exists this machine or the setup code | No size limit of its own: stops at the drive's free space, less 2 GB. Name made here. Before any account exists: at most 3 waiting. Nothing is opened |
| `POST /api/backup/restore/inspect` | An admin, or before any account exists this machine or the setup code | Name as for deleting. One job at a time. Every name, link, size, ratio and hash, and each folder's kind of file. A database is read as values into a clean one, and its rows made harmless. Nothing is changed |
| `POST /api/backup/restore/start` | An admin, or before any account exists this machine or the setup code | Typed RESTORE. Password and code, unless no account exists. An archive that's been checked, and known parts. It's read again as it's unpacked. The swap is made at the next start |
| `DELETE /api/backup/restore/request` | An admin, or before any account exists this machine or the setup code | Nothing: takes back what's waiting for the next start |
| `POST /api/backup/undo` | Admin | Password, and code with two-factor sign-in. Safety copy name by form, holding its own list. What it moves stays inside the data folders |
| `GET /api/games/home` | Signed-in | limits, in the query. Library number. own lists, without what the account can no longer see |
| `GET /api/games/{id}/check` | Signed-in | limits. A game. Says what it needs, as Play does: BIOS files beside the game are collected. Names and paths of the server aren't answered |
| `GET /api/libraries/{id}/games/review` | Admin | A games library. Known list. Limit and offset |
| `POST /api/libraries/{id}/games/review/apply` | Admin | A games library. 1 to 5,000 ids, known action, known console. No file is touched |
| `GET /api/libraries/{id}/games/import` | Admin | A games library |
| `GET /api/games/{id}/versions` | Signed-in | limits, on the game and on each version. A game. No paths. What's in an archive for admins only |
| `PUT /api/games/{id}/version` | Admin | A game. Entry is empty, or one this archive holds and this console runs |
| `POST /api/games/{id}/split` | Admin | A game |
| `PUT /api/games/{id}/console` | Admin | A game. Known console. No file is touched |
| `POST /api/games/{id}/not-a-game` | Admin | A game. No file is touched |
| `GET /api/libtranscode/options` | Admin | Nothing further |
| `GET /api/libtranscode/settings` | Admin | Nothing further |
| `PUT /api/libtranscode/settings` | Admin | Numbers within their ranges |
| `GET /api/libtranscode/libraries` | Admin | Nothing further |
| `PUT /api/libtranscode/libraries/{id}` | Admin | A library that holds video. Every choice in a recipe is from a list or a range, and languages are checked by form. Known output. Starting to replace originals takes the account's password, and code with two-factor sign-in |
| `GET /api/libtranscode/libraries/{id}/items` | Admin | A library that holds video. At most 100 answered |
| `POST /api/libtranscode/libraries/{id}/preview` | Admin | A library that holds video. Item in that library. Recipe as for saving |
| `POST /api/libtranscode/libraries/{id}/sample` | Admin | As a preview. Writes only to the cache |
| `POST /api/libtranscode/libraries/{id}/runs` | Admin | A library that holds video. At most 10,000 items. A real run does only what's saved. FFmpeg opens files as a list of media formats, local only |
| `GET /api/libtranscode/runs` | Admin | Library number |
| `GET /api/libtranscode/runs/{id}` | Admin | Numeric id |
| `GET /api/libtranscode/runs/{id}/files` | Admin | Numeric id. Limit of at most 200. Offset |
| `GET /api/libtranscode/leftovers` | Admin | Library number |
| `POST /api/libtranscode/files/{id}/undo` | Admin | A file of a run. Paths from the run's own record, which no backup holds. Refused while it's playing |
| `POST /api/libtranscode/files/{id}/use-copy` | Admin | A file of a run. Paths from the run's own record, which no backup holds |
| `DELETE /api/libtranscode/files/{id}/leftover` | Admin | A file of a run. Only a kept original or a converted copy, by the form of its name, from the run's own record |
| `GET /api/libtranscode/tdarr` | Admin | Nothing further. The key is never answered |
| `PUT /api/libtranscode/tdarr` | Admin | Web address with no login in it. Lengths. A new address needs its key again, and own password and a code with two-factor sign-in |
| `POST /api/libtranscode/tdarr/test` | Admin | Web address with no login, not the cloud metadata address. No redirects. Time and size limits. The saved key goes only to the saved address. Any other address asks for own password and a code with two-factor sign-in |
| `POST /api/libtranscode/tdarr/send` | Admin | At most 10,000 items. Library mapped. Saved address only |
| `GET /api/metadata-folder` | Admin | Nothing: the folder's place is fixed |
| `POST /api/metadata-folder/export` | Admin | Library number. One job at a time. The folder's place is fixed. No link is followed |
| `GET /api/metadata-folder/preview` | Admin | Nothing: the folder's place is fixed. Files are limited in size, and read through their own checks. Nothing is written |
| `POST /api/metadata-folder/apply` | Admin | As a preview. Pictures must be pictures, addresses must be web addresses, and text can't hold markup. One batch that can be undone |
| `GET /api/system/metrics` | Signed-in | What this account gets, by the owner's setting. Nothing about the machine unless that's the whole view |
| `GET /api/system/metrics/now` | Signed-in | By the owner's setting. Only an administrator sees the machine and other people. Known sections |
| `GET /api/system/metrics/stream` | Signed-in | As for one reading. Limits on open streams, per account and overall. The account is looked at again as it runs |
| `GET /api/system/metrics/history` | Signed-in | Administrators only, said in the handler. Known range, known series |
| `GET /api/system/metrics/report` | Signed-in | By the owner's setting. Libraries the account may open, and its own record. The rest for administrators |
| `GET /api/system/metrics/drives` | Admin | Nothing further |
| `GET /api/repair` | Admin | Nothing further |
| `POST /api/repair/check` | Admin | Known checks. One job at a time. Nothing is changed |
| `POST /api/repair/fix` | Admin | Keys from the last check, and fixes that can be made. Password and code for anything but what can't lose anything. Paths come only from the check |
| `POST /api/repair/remap` | Admin | Path lengths. A full path to a folder that exists, outside the data folder, on this machine unless it's allowed or asked for at the server. Most of the library's files must be there. Password and code to apply it. No file moves |
| `POST /api/repair/restart` | Admin | A fix is waiting for it |
| `GET /api/system/tasks` | Signed-in | By the owner's setting. Tasks of libraries the account may open, or its own. No file names, errors or other people for anyone but an administrator |
| `GET /api/system/tasks/stream` | Signed-in | As for the list. Limits on open streams, per account and overall. The account is looked at again as it runs |
| `GET /api/system/tasks/history` | Signed-in | As for the list. Limit, type, state, library |
| `DELETE /api/system/tasks/history` | Admin | Nothing further |
| `DELETE /api/system/tasks/history/{id}` | Admin | Task id |
| `POST /api/system/tasks/{id}/pause` | Admin | Task exists, and can be paused |
| `POST /api/system/tasks/{id}/resume` | Admin | Task exists, and can be resumed |
| `POST /api/system/tasks/{id}/cancel` | Admin | Task exists, and can be cancelled |
| `POST /api/system/tasks/{id}/retry` | Admin | Task exists, and its kind can be started again |
| `POST /api/themes/custom` | Admin | Name, known colours as colour values and nothing else, radius in range. Becomes a theme through the theme manager's own check |
| `GET /api/media-types` | Signed-in | Nothing: known formats and whether this server can play them. Which FFmpeg it runs for admins only |
| `PUT /api/users/{id}/profile` | Signed-in | Caller may manage that account, or it's their own. Which name shows is anyone's own choice. Names: an adult's own, or a manager's. Username has no invisible characters and differs from every other, ignoring case. Display name cleaned and cut to 60 characters |
| `/` (any method, everything else) | Public | Files of the web app, or its page. An unknown `/api/` address answers 404 |

---

<sub>WittyMediaServer documentation</sub>
