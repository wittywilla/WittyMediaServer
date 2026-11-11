# Watch parties

Watch something with other people at the same moment, each on their own screen, at home or far away. Everyone sees the same frame, a pause pauses it for everyone, and nobody has to count down from three.

- [Starting one](#starting-one)
- [Joining one](#joining-one)
- [While you watch](#while-you-watch)
- [Who can watch what](#who-can-watch-what)
- [Settings](#settings)
- [When something isn't right](#when-something-isnt-right)
- [For developers](#for-developers)

To play something on another device rather than alongside it, see [the Remote Protocol](REMOTE-PROTOCOL.md).

## Starting one

Open a film or an episode and press **Watch party**, then **Start a watch party**. You get a six-letter code, such as `KXM4T7`. Tell it to the people you want to watch with, however you like. The player opens, paused, so you can wait for them.

You are the **host**. To begin with only you can play, pause and seek. Turn on **Everyone can control** if you'd rather anyone could.

Everyone needs an account on your server, and a way to reach it: your home network, or [remote access](REMOTE.md).

## Joining one

Open any film or episode, press **Watch party**, type the code and press **Join**. Capitals, spaces and dashes don't matter. The player opens at the place the party has got to, even if it started without you.

If you've been invited by name, the party is listed there too, and you can join it without the code.

## While you watch

A bar across the top of the app shows the party's name, its code and who is there. Beside each person:

| It says | It means |
|---|---|
| **Ready** | Loaded and in step |
| **Loading** | Their player hasn't caught up with the last change yet |
| **Buffering** | Their connection has stalled |
| **Catching up** | The party stopped waiting for them. They'll rejoin the moment they've caught up |
| **Away** | Their connection has dropped. Their place is kept for two minutes |
| **Can't watch this one** | Their account isn't allowed to see what's playing now |

- **Play, pause and seek** work as usual, for whoever is allowed to control. Everyone else's player follows within a moment.
- **Waiting for everyone.** If someone's video stalls, the party pauses for them and carries on when they're ready. It waits 15 seconds at most, then goes on without them, and they catch up when they can. The host can turn **Wait for everyone** off.
- **If your connection drops**, you have two minutes to come back to the same place in the party, as host if you were.
- **Leaving.** Press **Leave**, or close the player. If the host leaves, whoever has been there longest becomes host.
- **The next episode.** When one ends, the party moves on to the next thing in its queue, if it has one.

A party ends when its host closes it, when everyone has been gone for five minutes, or when the server restarts: parties are kept in the server's memory and not written down.

## Who can watch what

A watch party never lets anyone see more than their own account may.

- You can only start a party for something your account can open.
- Someone whose account isn't allowed to see what's playing can't join, and isn't told what it is. They're told only that it's something they can't watch.
- If a party moves on to something a member isn't allowed to see, they keep their place, see no title, and can't control anything until it moves on again.
- Each person's video comes to them the usual way. Quality limits for people watching from away, and each account's own limits, apply to each person exactly as when they watch alone.

## Settings

Administrators choose under **Settings → Sharing & photos → Watch parties**:

| Setting | What it does | Default |
|---|---|---|
| **Watch parties** | Off: nobody can start or join one, and any that are running end | On |
| **Who can start one** | **Everyone**, **Adults**, or **Only administrators**. Anyone signed in can still join with a code | Everyone |

## When something isn't right

| What you see | What to do |
|---|---|
| **No such watch party** | Check the code. A party that everyone left more than five minutes ago, or that was running when the server restarted, is gone: start another |
| **Too many tries** | Wrong codes are counted, so that codes can't be guessed. Wait a minute |
| **This watch party is playing something this account can't watch** | Your account's limits don't allow it. An administrator can change them in **Settings → Profiles** |
| **Only the host controls this watch party** | Ask the host to press it, or to turn on **Everyone can control** |
| It keeps pausing | Someone's connection can't keep up. The host can turn **Wait for everyone** off, and that person's player will catch up on its own |
| You're a second behind | Players close a small gap by playing very slightly faster for a while. If it doesn't close, your device may be struggling: try a lower quality in the player's settings |

## For developers

Everything below is the API, for anyone writing a client. All of it goes through the server's ordinary API under `/api/v1`: there is no peer-to-peer connection, and nothing but small JSON messages passes through a room. A server that has this lists `syncplay` in the `features` of [`GET /capabilities`](CLIENT-API.md#start-here).

### The model

The server holds one **state** for each room, and it is the only truth:

```json
{ "rev": 42, "seq": 17, "itemId": 812, "index": 0,
  "positionSec": 754.5, "atMs": 1794000004500, "paused": false, "waiting": false, "rate": 1 }
```

It reads: "at server time `atMs`, the position is `positionSec`". From that every player works out where it should be at any moment:

```
if paused or waiting, or serverNow <= atMs:   position = positionSec
otherwise:                                    position = positionSec + (serverNow - atMs) / 1000 × rate
```

| Field | Meaning |
|---|---|
| `rev` | Goes up with every change of any kind. Send the latest you've applied with your reports |
| `seq` | Goes up with every command. Send it back as `ifSeq` to have a command refused if someone else got in first |
| `itemId`, `index` | What's playing, and its place in the room's `queue`. `itemId` is 0 for a member who may not see it |
| `positionSec`, `atMs` | The position, and the server time at which it is (or will be) that |
| `paused` | Somebody paused it |
| `waiting` | The room wants to play, and is holding for members who are still loading |
| `rate` | Playback speed |

No player is the leader. A player never tells the room where it is; it only asks for changes (`play`, `seek`) and says whether it has caught up (`ready`, `buffering`). So one slow or confused player can't drag the others about.

**Starts are set in the future.** When a room starts playing, `atMs` is a little ahead of now (`leadMs`, between 300 ms and 2.5 seconds, following the slowest member's round trip), so the news reaches every player before the moment comes. Until `atMs`, hold at `positionSec`, paused. At `atMs`, play. A pause takes effect at once, since there's nothing to line up.

### Server time

All of this needs each client to know the server's clock, to a few tens of milliseconds. `POST /syncplay/rooms/{id}/ping` is for that, and is also the heartbeat:

```json
{ "member": "<your seat>", "t0": 1793999990123.4, "rttMs": 41 }
```

`t0` is your clock when you sent it, in milliseconds. The answer returns it with the server's time when the ping arrived (`t1`) and when the answer left (`t2`):

```json
{ "t0": 1793999990123.4, "t1": 1794000000140.2, "t2": 1794000000140.9 }
```

With your clock when the answer arrived, `t3`, this is NTP's sum:

```
offset = ((t1 - t0) + (t2 - t3)) / 2      how far the server's clock is ahead of yours
rtt    = (t3 - t0) - (t2 - t1)            time on the wire, there and back
serverNow = yourClock + offset
```

- On joining, ping four or five times, a few hundred milliseconds apart, and **use the offset from the ping with the smallest `rtt`**: it was delayed least, so its two halves are most nearly equal, which is what the sum assumes.
- Then ping every 5 seconds. Keep the last eight or so and keep using the best of them.
- Send your best `rtt` as `rttMs` with each ping. The room uses the largest among its members to decide how far ahead to set a start.
- Ignore any ping whose `rtt` comes out negative: a clock jumped.
- Use a clock that doesn't jump for measuring (`performance.now()` plus a fixed base, `CLOCK_MONOTONIC`), not the wall clock.

`internal/syncplay/clock.go` has this as `ClockOffset` and `BestOffset`, with tests to check an implementation against.

### Keeping in step

About twice a second while playing, compare where your player is with where the room is, and correct the difference. `behind` is the room's position minus yours, in seconds:

| How far off | What to do |
|---|---|
| under 0.12 s | Nothing. Play at the room's `rate` |
| 0.12 s to 3 s | Play 3% faster if behind, 3% slower if ahead: the room's `rate` × 1.03 or × 0.97. Carry on until within 0.04 s, then go back to the room's `rate` |
| 3 s or more | Seek to the room's position |

3% is too little to notice, and audio keeps its pitch on every platform that corrects pitch when the rate changes (turn that on). With these thresholds two players rest within about a quarter of a second of each other.

`GET /syncplay` sends the numbers as `drift` (`stillSec`, `settledSec`, `seekSec`, `rate`), so a client needn't copy them. They are the same rules the web player keeps a live channel in step with: `web/src/player/sync.ts` in TypeScript, and `Correct` in `internal/syncplay/clock.go` in Go.

While `paused` or `waiting`, or before `atMs`: pause, and if you're more than half a second from `positionSec`, seek to it.

### Ready, buffering and waiting for everyone

After a seek or a change of item every player has to load again. So that the room starts together:

- When your player has loaded what the room is playing, is at (or within a second or two of) the room's position, and could play, send `POST …/report` `{ "member": "…", "state": "ready", "rev": <the rev you've applied> }`.
- When it stalls while the room is playing, send `"state": "buffering"`.

A `ready` whose `rev` is older than the room's last seek or change of item is about a place the room has left, and is ignored: always send the `rev` you've actually applied.

With **wait for everyone** on (the default), a room that should be playing sets `waiting: true` while any member it counts isn't ready, and starts, a moment in the future, when they all are. To keep one member from freezing the room:

- It waits at most 15 seconds. Then the members still not ready are marked `lagging`, the room goes on, and for the next minute their stalls can't hold it again.
- A lagging member counts again as soon as they report `ready`.
- Members who are `away` or `blocked` are never waited for. Someone who joins while the room is playing isn't waited for until their first `ready`.

### Rooms

`GET /syncplay` says whether watch parties are on, whether this account may start one, the drift numbers and limits, and this account's rooms: those it has a seat in from this sign-in (`memberId`), and those it's invited to.

```json
{ "enabled": true, "canCreate": true,
  "drift": { "stillSec": 0.12, "settledSec": 0.04, "seekSec": 3, "rate": 0.03 },
  "limits": { "members": 16, "queue": 200, "roomsPerAccount": 2, "invites": 32, "codeLength": 6, "pingEverySec": 5,
              "awaySec": 20, "seatGraceSec": 120, "waitTimeoutSec": 15, "emptyCloseSec": 300, "minLeadMs": 300, "maxLeadMs": 2500 },
  "rooms": [{ "id": "9f3c…", "name": "Friday film", "hostName": "Sam", "members": 3, "invited": true }],
  "serverTimeMs": 1794000000000 }
```

**Create**: `POST /syncplay/rooms`

```json
{ "itemId": 812, "name": "Friday film", "policy": "host", "waitForAll": true, "device": "tab-1" }
```

Or `items` (up to 200 item ids) with `index` in place of `itemId`, for a queue. `positionSec` starts somewhere other than the beginning. `policy` is `host` (the default) or `everyone`. `device` tells apart two windows of one sign-in: any short text that stays the same while the window lives. The answer is the room.

**Join**: `POST /syncplay/join` with `{ "code": "KXM4T7", "device": "tab-1" }`, or `{ "roomId": "…" }` for a room you're invited to or already have a seat in. The answer is the room, as it is now.

**The room**, as one member sees it:

```json
{ "id": "9f3c1a7e5b2d4c8891aa", "code": "KXM4T7", "name": "Friday film", "policy": "host", "waitForAll": true,
  "hostId": "a1…", "you": "b2…", "canControl": false, "blocked": false,
  "members": [
    { "id": "a1…", "userId": 2, "name": "Sam", "host": true, "state": "ready", "rttMs": 18 },
    { "id": "b2…", "userId": 5, "name": "Ada", "host": false, "you": true, "state": "loading", "rttMs": 64 } ],
  "queue": [812, 813], "items": [{ "id": 812, "title": "Puddle Day", "kind": "video", "runtimeSec": 5400 }, { "id": 813, "title": "Puddle Day 2", "kind": "video", "runtimeSec": 5100 }],
  "state": { "rev": 3, "seq": 1, "itemId": 812, "index": 0, "positionSec": 0, "atMs": 1794000000000, "paused": true, "waiting": false, "rate": 1 },
  "serverTimeMs": 1794000001000, "leadMs": 300 }
```

`you` is your seat: send it as `member` with everything else. A member's `state` is `ready`, `loading`, `buffering`, `lagging`, `away` or `blocked`. An entry of `queue` you may not see is `0`, with an empty entry in `items`.

**The stream**: `GET /syncplay/rooms/{id}/events?member=…` is server-sent events, in the same form as [the Remote Protocol's](REMOTE-PROTOCOL.md#transport).

| Event | Data | Meaning |
|---|---|---|
| `room` | the room, whole | Sent first, and whenever anything about it changes |
| `closed` | `{ "reason": "…" }` | The last event: `closed` (the party ended), `removed` (your seat was taken away), `replaced` (another stream opened for your seat), `signed-out`, or `server-stopping` |

Every `room` event is the whole room, so a client that reconnects or reads slowly is simply told how things stand. Apply the one with the highest `state.rev` and forget the rest. One seat has one stream.

**Commands**: `POST /syncplay/rooms/{id}/command`

```json
{ "member": "b2…", "type": "seek", "positionSec": 754.5, "ifSeq": 17 }
```

| `type` | Fields | What it does |
|---|---|---|
| `play` | | Starts, a moment in the future, or sets `waiting` if members are still loading |
| `pause` | | Pauses at once, at the room's position |
| `seek` | `positionSec` | Moves there. Every member loads again |
| `next`, `previous` | | Moves along the queue, from the start of the item |
| `select` | `index` | Goes to a place in the queue |
| `rate` | `rate`, 0.5 to 2 | Changes the speed |
| `queue` | `items`, `index` | Replaces the queue. The caller must be able to see every item |

The answer is the room after the command. Commands are applied one at a time in the order they reach the server, and that order is the truth. A command that changes nothing (`play` while playing) is accepted and changes nothing.

`ifSeq` is optional. With it, the command is refused with `409`, code `stale`, unless the room's `seq` is still that number; the refusal carries the room as it is now in `room`. Use it for anything a client does by itself: when an episode ends and every player would send `next`, each sends it with the `seq` it saw, and the room moves on once.

**The rest**, each taking `member` in its JSON body (`GET` and `DELETE` take `?member=`):

| Route | Who | What it does |
|---|---|---|
| `GET /syncplay/rooms/{id}` | a member | The room |
| `POST /syncplay/rooms/{id}/report` | a member | `state`: `ready` or `buffering`, with `rev` |
| `POST /syncplay/rooms/{id}/ping` | a member | The clock and the heartbeat |
| `POST /syncplay/rooms/{id}/leave` | a member | Gives up the seat |
| `PUT /syncplay/rooms/{id}` | the host | `name`, `policy`, `waitForAll` |
| `POST /syncplay/rooms/{id}/host` | the host | `to`: hands the room to another member |
| `POST /syncplay/rooms/{id}/kick` | the host | `whom`: takes a member's seat away |
| `POST /syncplay/rooms/{id}/invite` | the host | `userIds`: those accounts find the room in `GET /syncplay` and may join by `roomId` |
| `DELETE /syncplay/rooms/{id}` | the host, or an administrator | Ends the party for everyone |

### Seats, and coming back

A seat belongs to an account, a sign-in and the `device` text given when joining. Joining again with the same three is the same seat, with the same `you`, as host if it was.

- A member whose stream closes is `away` at once, and isn't waited for.
- A member not heard from for 20 seconds (no ping, report or command) is `away` too. Ping every 5 seconds.
- A seat not heard from for 2 minutes is given up. If it was the host's, the member who has been there longest and isn't away becomes host.
- A room with no members closes after 5 minutes. Until then its code still works, and whoever arrives first runs it. It stays paused where the last member left it.
- No room lasts longer than 24 hours.

After a reconnect: join again (same `device`), open the stream, ping a few times, load the room's item at the room's position and report `ready`.

### A client's loop

1. Join or create. Keep `id` and `you`.
2. Open the stream. Start pinging.
3. On each `room` event with a new `state.rev`: if `itemId` changed, open that item (through [`POST /items/{id}/playback`](CLIENT-API.md#playing-something), like any playback); if `blocked`, show that this one isn't available and wait.
4. Twice a second: work out the room's position from the state and the server's time, and pause, play, nudge or seek as [above](#keeping-in-step).
5. When your player becomes able to play at the room's position, report `ready` with the `rev`. When it stalls during play, report `buffering`.
6. When the person presses play, pause or seeks, and `canControl` is true, send the command and let the answer (and the stream) move your player, like everyone else's. When `canControl` is false, don't act on it.
7. When the item ends and `canControl` is true, send `next` with `ifSeq`.
8. On `closed`, or on leaving the player: stop. Send `leave` if the person chose to go.

### Who may do what

- **Starting**: by the `syncplayCreate` setting (`everyone`, `adults`, `admins`), and only for items the account may see. An item it may not see answers `404`, as everywhere.
- **Joining**: any signed-in account with the code, or an invited account by the room's id. The account must be allowed to see what the room is playing **now**, by its library, rating and title limits; otherwise `403` with code `cant-watch`, and nothing in the answer says what the room is playing. Wrong codes are counted per account: ten, then one more every six seconds.
- **A room's id alone opens nothing.** It works only for an account that was invited or already has a seat.
- **Every room route but those two** needs the caller's own seat: the `member` id together with the account it belongs to. Anything else answers `404`, as if the room weren't there.
- **Controlling**: the host, or every member when `policy` is `everyone`. Never a member who is `blocked`.
- **Titles** are sent to a member only for items their account may see, asked afresh when the stream opens and every 25 seconds after.
- **A list of invitations** names the room and its host, never what it's playing.
- **The video itself** never passes through a room. Each member fetches it through the playback routes with their own sign-in, where their own limits and the limits for [viewers away from home](REMOTE.md) apply.
- **Streams** look at their sign-in again every 25 seconds: signing out ends the stream and gives up the seat.

### Limits and errors

| What | Limit | When it's reached |
|---|---|---|
| Rooms in all | 32 | `429`, code `too-many` |
| Rooms one account hosts | 2 | `429`, code `too-many` |
| Members in a room | 16 | `409`, code `full` |
| Items in a queue | 200 | `400`, code `bad-request` |
| Invitations to a room | 32 | `429`, code `too-many` |
| A request body | 8 KB | `413` |
| Commands, and reports, from one seat | one every 250 ms, after a burst of 12 | `429`, code `rate` |
| Pings from one seat | one every 500 ms, after a burst of 10 | `429`, code `rate` |
| Open room streams, one account | 16 | `429`, code `too-many` |

Refusals are `{ "error": "<a sentence fit to show>", "code": "…" }`:

| Status | `code` | Meaning |
|---|---|---|
| 400 | `bad-request` | An unknown command, or a value that makes no sense |
| 403 | `disabled` | Watch parties are turned off |
| 403 | `not-allowed` | This account can't start one |
| 403 | `cant-watch` | This account may not see what the room is playing |
| 403 | `host-only`, `not-host` | Only the host controls this room; only the host can do that |
| 404 | `no-room`, `not-member` | No such room, or you have no seat in it |
| 404 | `no-item` | No such item, or not one this account may see |
| 409 | `stale` | `ifSeq` was out of date. `room` holds the room as it is |
| 409 | `end-of-queue` | There's nothing more that way |
| 409 | `full` | No seats left |
| 429 | `rate`, `too-many` | Slow down, or too many of something |

A room costs the server a few hundred bytes for each member, one open connection for each stream, and one timer that runs once a second while any room exists and stops when none does. The engine is `internal/syncplay`: it never reads the clock itself, so its tests walk through each of the rules above second by second.

---

<sub>WittyMediaServer documentation</sub>
