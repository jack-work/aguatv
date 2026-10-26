# The hub

`aguatv-hub` is one service that links screens to a person, sends them casts and
playback commands, streams what was cast, and drives a TV over adb when it can
reach one. It has no login of its own: put it behind one that stamps `Remote-User`.

## Two kinds of caller

| caller | proves itself by | may |
|---|---|---|
| a person | `Remote-User`, stamped by the login proxy | list, link, rename and unlink their devices; cast; send playback commands; drive an adb TV |
| a device | the `aguatv_device` cookie: an HttpOnly, Secure, SameSite=Strict, host-only cookie holding its token | wait for its next event, report playback, stream what was cast to it |

A device never holds anything that works anywhere but the hub, and only for its
own device id. Device paths are exact paths, so a login proxy can exempt exactly
them: `/receiver`, `/device/start`, `/device/claim`, `/device/qr`, `/device/next`,
`/device/state`, `/stream`. Everything else needs `Remote-User`.

## Linking

1. A screen opens `/receiver`. With no valid cookie it calls `POST /device/start`
   and gets a link code (`K7M-QX4`: 7 characters from an alphabet without 0, O, 1, I)
   and a claim secret. The page shows the code, the link URL and a QR of
   `/tvlogin?code=K7M-QX4`.
2. A person opens `/tvlogin`, sees what is asking to be linked (kind, the name it
   proposed, when it asked, the address it asked from) and approves it, naming it.
3. The screen polls `POST /device/claim` with its claim secret. Once approved, the
   hub sets the device cookie on that response and the code is spent.

Codes live 10 minutes, are single use, and are compared in constant time.
`/device/start` is limited per address and globally. Approval is limited per
person. A wrong code costs a person one attempt from a small bucket.

## Tokens

32 random bytes, base64url. The store keeps only their SHA-256, bound to a device
id and an owner. A token expires `deviceTTL` after its last use (90 days for a TV;
a browser can ask for a session link, a cookie with no Max-Age and a 24 h TTL).
Unlinking deletes the row, so the next request with that token is refused. Tokens
and codes never appear in logs, argv or URLs.

## Events

Each device has a queue. `GET /device/next` waits up to 25 s and returns the next
event or 204. Kinds: `cast` (a cast id, a title, a type), `control` (an action
from zanni's filo: toggle, skip, seekTo, seekFraction, chapter, stop) and
`unlinked`. A queue keeps the last 16 events; a stale `control` is dropped when a
`cast` replaces what it would act on.

## Casting

`POST /api/devices/{id}/cast {ref}` from a person:

1. the device must belong to the caller;
2. the hub asks the files authorizer whether the caller may `read` that key, every
   cast, not once at linking;
3. it records a cast (random id, device id, key, caster, expiry 12 h) and queues it.

`GET /stream?cast=<id>` from that device streams the object by proxying a ranged
GET to S3 with a read-only key, passing `Range`, `If-Range`, `Content-Range`,
`Content-Length`, `Content-Type`, `ETag`, `Last-Modified` and `Accept-Ranges`.
A cast that belongs to another device, has expired, or whose caster has since lost
`read` (rechecked at most once a minute while streaming) is refused.

## adb

A device may also name an adb target. Then `POST /api/devices/{id}/press` drives it
like the terminal remote. The adb server and its forwards listen on unix sockets in
the hub's own 0700 runtime directory, so nothing listens on loopback TCP.

## State

One JSON file, `devices.json`, mode 0600, rewritten atomically. Casts, codes and
queues are in memory: a restart unlinks nothing but drops casts in flight.

## Code

```
cli/cmd/aguatv-hub/
  server.go  routes, the two doors (asPerson, asDevice), the same-origin rule
  link.go    codes, claim secrets, rate limits
  store.go   devices.json: hashed tokens, owners, expiry
  queue.go   per-device event queues and the long poll
  cast.go    casts, the files authorizer client, S3 SigV4 ranged GET
  adb.go     adb remotes, shared with the terminal remote through internal/tv
  sources.go no storage, and a local directory for tests
hub-web/     index (screens and remotes), tvlogin, receiver (self-contained)
player/src/tv/aguatv/player/Receiver.java   the TV side: a WebView on <hub>/receiver
```

Every non-GET request must be same-origin JSON (`Content-Type: application/json`,
`Sec-Fetch-Site` same-origin or absent), DELETE excepted from the content type.
`/receiver` inlines its CSS and JS because everything else under `/` needs a login.

## The TV side

`aguatv receiver <hub URL>` installs the player APK if needed and opens its
`Receiver` activity on `<hub>/receiver?kind=tv&name=<device_name>`; the URL is
kept in the app's preferences. The WebView keeps the device cookie, plays without
a user gesture, keeps the screen on while video plays (`receiverHost.keepAwake`),
and turns BACK into `aguatvReceiver.back()`, which stops a video or leaves. The app
allows cleartext only to `localhost`, for testing through `adb reverse`.

Measured on a Fire OS 7 TV: the D-pad arrives as `Arrow*`, OK as `Enter`, rewind and
fast-forward as `MediaRewind` and `MediaFastForward` keydown events; play/pause
works as `MediaPlayPause` or keyCode 179.

## Testing on a workstation

```sh
nix build .#cli .#hubWeb .#aguatv
AGUATV_HUB_STUB_GRANTS="me:films/test.mp4" AGUATV_HUB_STUB_DIR=/path/to/files \
  result/bin/aguatv-hub -site <hubWeb> -public http://localhost:8099 -state /tmp/hub \
  -listen 127.0.0.1:8099 -insecure-cookies
adb reverse tcp:8099 tcp:8099
aguatv receiver http://localhost:8099
curl -H 'Remote-User: me' 'http://127.0.0.1:8099/api/link?code=<code on the TV>'
curl -H 'Remote-User: me' -H 'Content-Type: application/json' -d '{"code":"<code>"}' http://127.0.0.1:8099/api/link
curl -H 'Remote-User: me' -H 'Content-Type: application/json' -d '{"ref":"films/test.mp4"}' http://127.0.0.1:8099/api/devices/<id>/cast
```

`Remote-User` stands in for the login proxy, so bind the test hub to loopback only.
`AGUATV_HUB_STUB_GRANTS` holds `user:key` pairs; `AGUATV_HUB_STUB_DIR` stands in for
the bucket. Neither is used once `-files-authz` or `-s3-endpoint` is set.
