# Building

Everything builds with Nix; there is no Gradle.

| output | what |
|---|---|
| `.#web` | the page, run through `zanni.lib.mkBoiledSite` with boil, gesso, phosphor and fontpack. `zanni-check` runs on the result and fails the build if an effect did not arrive |
| `.#apk` | unsigned, aligned APK: `aapt2` compile and link, `javac --release 8 -Werror`, `d8`, `zipalign`. The built page goes in as `assets/web/` |
| `.#aguatv` | the CLI, with the APK path, build-tools and the remote baked in |
| `.#remote` | `aguatv-remote`, by `buildGoModule`; the scrcpy jar, its version and adb come in through `-ldflags`. Bump `vendorHash` when `remote/go.sum` changes |
| `.#default` | both binaries in one profile entry |

`nix run` runs `aguatv`; `nix run .#remote` runs the remote.

The Android SDK comes from `androidenv.composeAndroidPackages` with platform 28
and build-tools 34. Its licence must be accepted, which the flake does through
`android_sdk.accept_license`.

## zanni-check and the witness

`zanni-check` requires that every class a component promises is worn by some
element. The page does not use every boil variant, so `index.html` carries an
empty, zero-size `<svg class="witness">` wearing the rest. Remove a class from it
only if the component stops promising it.

## Signing

The Nix store cannot hold a private key, so the APK is built unsigned and signed
by `aguatv install` or `aguatv apk`. The key is generated on first use at
`$XDG_DATA_HOME/aguatv/release.keystore`; override with `AGUATV_KEYSTORE` and
`AGUATV_KEYPASS`. Android refuses an update signed by a different key
(`INSTALL_FAILED_UPDATE_INCOMPATIBLE`), and the only way past that is uninstalling,
which also clears the page's saved state.

## javac

`-Werror` with `-Xlint:all` is on. `@SuppressWarnings("deprecation")` in `Input`
covers `requestAudioFocus` and `abandonAudioFocus`, kept from dumbtv, which also
supported Android 7.1. Moving to `AudioFocusRequest` is fine at sdk 28 but untested.
