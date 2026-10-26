# The hub

`aguatv-hub` keeps a person's screens, sends them casts and playback commands,
drives a TV over adb when it can reach one, and serves the web app (`web/`, the
same bundle the Fire TV home screen ships). It has no login of its own: it trusts
`Remote-User` and `Remote-Groups`, so run it only behind a proxy that overwrites
both, on a socket nothing else can reach (`nix/hub-module.nix`).

## Screens

Every screen signs in as its person. A browser that opens the hub is logged in
already; registering it (`POST /api/me {name, remote}`) makes it one of that
person's screens and sets `aguatv_screen`: HttpOnly, Secure, SameSite=Strict,
host-only, 400 days. The cookie only says which of the person's screens this
browser is. Under any other login it is ignored.

The store keeps the SHA-256 of the cookie's 32 random bytes, the owner, the name
and the remote grant, in `screens.json` (0600, rewritten atomically). A screen
unused for 90 days is dropped. Registering again from the same browser updates it.
`DELETE /api/me` forgets this browser and clears the cookie.

## The remote grant

A screen registered as a remote may cast to, control, rename and remove its
owner's screens and drive the adb remotes. Anything else may only receive.

| route | needs |
|---|---|
| `GET /`, assets, `GET /api/me`, `GET /api/screens` | a login |
| `POST /api/me`, `DELETE /api/me` | a login |
| `GET /api/me/next`, `POST /api/me/state` | a login and this browser registered |
| `POST /api/screens/{id}/cast`, `/control`, `/name`, `DELETE /api/screens/{id}` | a remote screen; `{id}` must be the same owner's |
| `POST /api/adb/{name}/press`, `/text` | a remote screen and, if `-adb-group` is set, that group |

`GET /api/screens` lists adb remotes only to a remote screen allowed to drive them.

## Casting

`POST /api/screens/{id}/cast {url}` takes a path (`films/a.mkv`) or a URL on the
`-files` origin and queues `{kind: "cast", url, title, type}` for that screen. The
URL is rebuilt from the path alone: another scheme or host, user info, a query, a
directory, `..` or a control character is refused. The screen fetches the file
itself, with its own login, so the hub never touches storage and casting grants
nothing the screen could not read already.

## Events

Each screen has a queue. `GET /api/me/next` waits up to 25 s and returns the next
event or 204. Kinds: `cast`, `control` (an action from zanni's filo: toggle, skip,
seekTo, seekFraction, chapter, stop) and `forgotten` (the screen was removed). A
queue keeps the last 16 events; a new cast drops controls still waiting. Screens
report playback with `POST /api/me/state`; the list shows reports younger than
two minutes.

## The page

`GET /` serves `index.html` with a CSP: scripts from `'self'` plus the hashes the
site build wrote to `index.html.csp`, media from `'self'` and the files origin,
connections to `'self'` only, no framing. `.html` and `.csp` files are not served
directly, so the page cannot be loaded without its policy. Every non-GET request
must be same-origin JSON (`Sec-Fetch-Site` same-origin or absent), DELETE
excepted from the content type. Every response is `private, no-store`.

## The web app

`web/home.js` runs on two hosts. With the Android bridge (`window.host`) it is
the TV home screen: inputs, apps, settings. In a browser behind the hub it shows
the person's screens instead, a drawer to register this browser and to cast or
control from it, and the adb pad (`h j k l`, `gg`, counts, `i` to type). `?preview`
fakes the Android host for design work. `web/cast.js` plays a cast full screen
under filo (zanni 0.11.0) with `globalKeys`, and maps the hub's controls onto
filo's handle. Escape or back stops it.

## adb

`-adb name=host:port` adds a remote. The adb server should listen on a unix
socket in a 0700 directory, never on loopback TCP. The NixOS module does not wire
adb yet.

## Code

```
cli/cmd/aguatv-hub/
  server.go  routes, the caller (login, screen, remote), the page and its CSP
  store.go   screens.json: hashed tokens, owners, idle expiry
  queue.go   per-screen event queues and the long poll
  cast.go    a files path or URL, rebuilt on the files origin
  adb.go     adb remotes, shared with the terminal remote through internal/tv
web/         the app: index.html, home.js, cast.js, home.css
nix/hub-module.nix, nix/hub-test.nix   services.aguatv-hub and its VM test
```

## Testing on a workstation

Run the hub on loopback behind anything that sets `Remote-User`, and a file
server standing in for the files site:

```sh
nix build .#cli .#web
result/bin/aguatv-hub -site <web> -state /tmp/hub -files http://127.0.0.1:8732 \
  -listen 127.0.0.1:8731 -insecure-cookies
```

A few lines of Go with `httputil.ReverseProxy` that set `Remote-User` on
127.0.0.1:8730 and serve a directory on 8732 are enough; drive two browser
contexts (one screen, one remote) with Playwright. `Remote-User` stands in for
the login, so never bind a test hub anywhere but loopback.
