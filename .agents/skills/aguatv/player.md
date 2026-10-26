# The player

`tv.aguatv.player` is a second APK whose only job is playing one HTTPS video.
It is separate from aguatv so the home screen keeps no `INTERNET` permission.

```
player/src/tv/aguatv/player/Player.java   VideoView, an on-screen bar, keys
nix/player.nix                            the unsigned build, same recipe as aguatv
```

## How a video gets there

`aguatv play <ref>` and `:play <ref>` in the remote take one reference.

- An `https://` URL is played as it is.
- Anything else goes to the command in `AGUATV_PRESIGN`, with the reference
  appended as its last argument. It must print one `https://` URL. Without
  `AGUATV_PRESIGN` set, both refuse and say so.

The TV then gets `am start -a VIEW -n tv.aguatv.player/.Player -t <type> -d <url>`.
The type comes from the extension of the reference, ignoring any query.

A presigned link is the intended source: private storage behind a login page
cannot be reached by Android's media player, which follows no login, while a
presigned link reads one object for a limited time and the credentials stay on
the computer. With the AWS CLI:

```sh
export AGUATV_PRESIGN='aws s3 presign --expires-in 14400'
aguatv play s3://my-bucket/films/talk.mp4
```

Any S3-compatible store works if it answers `Range` requests on presigned links
(`206 Partial Content`); that is what makes seeking work. A wrapper that maps
your own short names or browser links onto bucket URIs belongs in your private
configuration, not here.

The URL is an argument to `am start`, so it sits in the TV's activity history
until it expires. Keep expiries to the length of what you are watching.

## Keys on the TV

| key | does |
|---|---|
| OK, play/pause | pause and resume |
| ◀ ▶ | 10 s back or forward |
| rewind, fast forward | 30 s |
| ▲ ▼ Menu Info | show the title and time |
| Back | stop and leave |

The video ends the activity when it finishes. Leaving it pauses and remembers
the position until it comes back.

## Installing

`aguatv play` installs or updates the player when the built APK differs from the
one it last installed (recorded in `$XDG_DATA_HOME/aguatv/player.installed`),
signed with the same key as aguatv. `:play` in the remote does not install
anything; it says so when the player is missing.

## Formats

Whatever the TV's `MediaPlayer` decodes. H.264 in MP4 is tested. MKV and HEVC
depend on the television's hardware.
