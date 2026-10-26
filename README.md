# aguatv

Two programs for televisions that run Android, built by one flake:

- **aguatv**, a home screen for Fire TV Edition televisions, drawn by a web page.
- **aguatv-remote**, a remote control in the terminal with vim keys, for any
  Android TV, Google TV or Fire TV that adb can reach.
- **aguatv-deploy**, which makes a Fire TV match a Nix module: packages disabled,
  starved or removed, settings, and the home screen. Built on
  [dumbtv](https://github.com/humblemuzzu/dumbtv)'s package lists, with your own
  module on top. See [`deploy.md`](.agents/skills/aguatv/deploy.md).

```sh
nix profile install github:jack-work/aguatv      # both: aguatv and aguatv-remote
nix run github:jack-work/aguatv#remote            # just the remote
```

## The home screen

Amazon's home screen is a storefront. aguatv replaces it with three rows: the TV's
inputs, the apps you installed, and a few system actions. The page is plain HTML,
CSS and JavaScript built with [zanni](https://github.com/jack-work/zanni), so its
look changes with a CSS edit rather than a Java rebuild.

- No `INTERNET` permission. The page and everything it shows are served from inside
  the APK; Android will not let the app open a connection.
- No ads, no recommendations, no account, no analytics, no update checks.
- No root. Installation needs ADB over the network, nothing more.
- Fire OS 7 and later (Android 9, sdk 28+). Tested on a Fire TV Edition television;
  Fire TV sticks should work but have no inputs row and are untested.

## Install

You need [Nix](https://nixos.org) with flakes, and a TV with ADB debugging on
(Settings > My Fire TV > Developer options).

```sh
adb connect <tv-ip>:5555          # accept the prompt on the TV
nix run github:jack-work/aguatv -- install
```

`install` builds the APK, signs it with a key kept in
`~/.local/share/aguatv/release.keystore` (created on first run), installs it, and
takes over the HOME button. Later installs must use the same key, so keep that file.

| command | does |
|---|---|
| `aguatv install` | build, sign, install, take over HOME |
| `aguatv undo` | give HOME back to Amazon's launcher; aguatv stays installed |
| `aguatv status` | whether the HOME guard is bound and what is in front |
| `aguatv apk [path]` | write a signed APK without installing |
| `aguatv remote [-c host:port]` | the same as `aguatv-remote` |
| `aguatv play <ref>` | play a video on the TV (see below) |

With several devices connected, set `ANDROID_SERIAL`.

## Using it

| Remote | Does |
|---|---|
| ◀ ▶ ▲ ▼ | move between tiles and rows |
| OK | open the app, switch to the input, or run the action |
| ☰ Menu | details for the focused tile, with its actions (open, hide, app info) |
| Back | jump to the first tile; on an input, return home |
| Hold HOME | Fire OS's quick menu, unchanged |

The Settings row opens aguatv's own settings (boil, scanlines, clock, hidden
apps) and the TV's settings. To get Amazon's home screen back, run `aguatv undo`.

## The remote

`aguatv-remote` (also `aguatv remote`) is a remote control in the terminal. It
needs nothing installed on the TV: any Android TV, Google TV or Fire TV with ADB
debugging on will do, reached on the local network, over Tailscale, or by any
other route to its port 5555. `-c host:port` runs `adb connect` first; `-s`
picks a device when several are connected.

| keys | does |
|---|---|
| `h` `j` `k` `l`, arrows | D-pad |
| `Enter` `o` `Space` | OK |
| `b` `Backspace` `Esc` | Back |
| `gg` `H` | Home; `gh` holds Home for the quick menu |
| `K` `m` | Menu; on aguatv, the details panel for the focused tile |
| `p` `[` `]` | play/pause, rewind, fast forward |
| `+` `=` `-` `M` | volume up, down, mute |
| `5j`, `Ctrl-u` `Ctrl-d` | counts; five up or down |
| `i` `a` | type into a text field; `Esc` leaves |
| `gs` `gS` | aguatv's settings panel; the TV's own settings |
| `:power` `:wake` `:sleep` `:quick` `:type text` | the rest |
| `?` | key list; `q` or `:q` quits |

Keys go through a scrcpy server started on the TV for the session, so a press
lands at once rather than starting a Java process per key as `adb shell input`
does. `-slow` uses `adb shell input` instead.

## Videos

`aguatv play https://example.com/talk.mp4` installs a small player app on the TV
if needed and plays the URL; `:play <url>` does the same from the remote. The
player is a separate APK, so aguatv's home screen still has no internet access.
On the TV, OK pauses, ◀ ▶ skip 10 seconds and Back stops.

With `AGUATV_PRESIGN` set, every reference, URLs included, is handed to that
command, which must print one `https://` URL; it can map links of its own and
print others back unchanged. Without it, only `https://` URLs play. For private
files in S3:

```sh
export AGUATV_PRESIGN='aws s3 presign --expires-in 14400'
aguatv play s3://my-bucket/films/talk.mp4
```

## Develop

```sh
nix build .#web      # the page alone; open result/index.html in a browser
nix build .#apk      # unsigned APK
nix build .#cli      # aguatv-remote and aguatv-deploy; go test ./... in cli/
nix develop          # adb, a JDK and ANDROID_HOME
```

Opened in a desktop browser, the page runs against sample data, so layout and
navigation can be worked on without a TV.

How it works, and the Fire OS behaviour it is built around, is in
[`.agents/skills/aguatv`](.agents/skills/aguatv/SKILL.md).

## Credits

The HOME takeover and the input screen are adapted from
[dumbtv](https://github.com/humblemuzzu/dumbtv)'s launcher, under its MIT licence.
dumbtv also covers what aguatv does not: disabling Amazon's ad and telemetry
packages and filtering DNS on the TV.
