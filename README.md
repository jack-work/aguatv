# aguatv

A home screen for Fire TV Edition televisions, drawn by a web page.

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

With several devices connected, set `ANDROID_SERIAL`.

## Using it

| Remote | Does |
|---|---|
| ◀ ▶ ▲ ▼ | move between tiles and rows |
| OK | open the app, switch to the input, or run the action |
| ☰ Menu on an app | hide it, or unhide it while hidden apps are shown |
| Back | jump to the first tile; on an input, return home |
| Hold HOME | Fire OS's quick menu, unchanged |

The System row opens Settings, turns the boil animation on or off, shows hidden
apps, and opens Amazon's home screen on purpose.

## Develop

```sh
nix build .#web      # the page alone; open result/index.html in a browser
nix build .#apk      # unsigned APK
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
