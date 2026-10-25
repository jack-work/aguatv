# Memory

The page costs more than a native launcher. Know by how much before adding to it.

## Measured

On a Fire TV Edition television with 1.7 GB of RAM, Fire OS 7.7, 1920x1080 UI,
`dumpsys meminfo`, aguatv in front:

| process | PSS |
|---|---|
| `tv.aguatv` | 135 to 195 MB, of which Graphics is 59 to 110 MB |
| WebView renderer (`sandboxed_process0`) | about 59 MB |
| other WebView helper processes | about 20 MB |

For comparison, dumbtv's native launcher reports 60 to 70 MB.

Graphics varies the most. Turning boil off dropped it by about 12 MB at once.

## What costs

- GPU buffers for the WebView's compositing layers. Every element that gets its
  own layer (a `transform`, a `filter`, a `mix-blend-mode`) can cost a texture.
- `phosphor` blends a full-screen `::after` with `multiply`, which is a full-screen
  layer.
- `boil` re-rasters its filtered elements each time the seed steps. Only the
  wordmark, the clock and the focus ring wear it.

## What the shell does about it

- On pause: `WebView.onPause()`, `pauseTimers()`, and
  `RENDERER_PRIORITY_WAIVED`, so Android may kill the renderer while an app or
  input is in front. `onRenderProcessGone` rebuilds the WebView when that happens.
- `onTrimMemory(UI_HIDDEN)` asks the window manager to drop the hidden window's
  GPU context, which on Android 9 otherwise survives until memory is critical.

## Before adding an effect

Measure Graphics and TOTAL with `adb shell dumpsys meminfo tv.aguatv` before and
after, with the page in front and after a minute in an app.
