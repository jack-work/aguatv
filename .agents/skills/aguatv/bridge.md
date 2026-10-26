# The bridge

The page talks to Java through one object, `window.host`, registered with
`addJavascriptInterface`. Java talks to the page by calling functions on
`window.aguatv` with `evaluateJavascript`.

## Page to Java: `window.host`

| method | returns or does |
|---|---|
| `apps()` | JSON array of `{id, label, leanback}`, sorted by label |
| `inputs()` | JSON array of `{id, label, hdmi, connected}`, HDMI first |
| `openApp(id)` | starts the app's launcher activity |
| `openInput(id)` | shows the input (see below) |
| `openSettings()` | Fire OS settings |
| `details(kind, id)` | JSON for the details panel: version, install and update dates, system flag, target sdk for an app; type, state, service and id for an input |
| `openAppInfo(id)` | Android's app details screen for a package |
| `ready()` | the page has painted; lets the guard remove its overlay |

Bridge methods run on a binder thread. Anything that touches the UI is posted to
the main thread in `Home`.

App art is not in the JSON. The page loads `/art/<package>` as an image; `Shelf`
renders the app's TV banner, or its icon when it has none, to PNG. A 404 means no
art, and the tile shows its label.

## Java to page: `window.aguatv`

| function | called when |
|---|---|
| `refresh()` | a package is added, removed or changed, or an input's state changes |
| `resume()` | `Home` resumes: redraw the clock and rows |
| `pause()` | `Home` pauses: stop timers |
| `home()` | HOME pressed while already home |
| `back()` | BACK released: closes a panel, else focuses the first tile |
| `details()` | Menu released: toggles the details panel for the focused tile |
| `settings()` | `Home` started with `--es aguatv.open settings` |

BACK and Menu never reach the page as key events; `Home.dispatchKeyEvent` turns
them into these calls. The Info key would have been the natural details key, but
Fire OS takes it before any app and shows a "pair your remote" dialog.

`am start -n tv.aguatv/.Home --es aguatv.open settings` (or `details`) opens a
panel from outside; the remote's `gs` uses it. If the page has not loaded yet,
`Home` holds the request until `ready()`. The D-pad arrives as `ArrowUp`/`ArrowDown`/`ArrowLeft`/
`ArrowRight` and OK as `Enter`, handled in `home.js`.

## Inputs

`Input.intent` decides how an input opens. If exactly one app other than Amazon's
Live TV claims the passthrough URI, that app gets it. Otherwise aguatv's own
`Input` activity shows it through `TvView`, because Live TV answers passthrough
URIs with its channel-scan settings instead of the picture. `Input` keeps the
screen on while there is video, and BACK returns home.

## Page state

Hidden apps and the settings panel's switches (boil, scanlines, 24-hour clock,
showing hidden apps) live in `localStorage` under `aguatv.*`. Clearing
the app's data resets them.
