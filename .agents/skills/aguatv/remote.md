# The terminal remote

`remote/` is a Go program on bubbletea v1.3.10, bubbles v1.0.0 and lipgloss
v1.1.0. It does not depend on aguatv being installed.

```
remote/
  main.go     flags, the key queue and the worker that owns the connection
  tv.go       the tv interface: scrcpy control socket, or adb shell input
  ui.go       the model: modes, counts, the drawn remote, the help
  ui_test.go  key handling against a recording sender
```

## Why scrcpy

`adb shell input keyevent` starts `app_process` for every key, which on a
television's CPU takes long enough to feel. scrcpy's server is one process that
injects events through `InputManager` and reads them from a socket.

The session:

1. push the scrcpy-server jar from nixpkgs to `/data/local/tmp/aguatv-remote.jar`
2. `adb forward tcp:0 localabstract:scrcpy_<scid>` for a random scid
3. start the server with `video=false audio=false control=true tunnel_forward=true
   send_device_meta=false send_dummy_byte=true`
4. dial the forwarded port and wait for the dummy byte; that socket is the
   control socket

Messages are big-endian. A key is 14 bytes: type `0`, action (`0` down, `1` up),
keycode `i32`, repeat `i32`, meta state `i32`. Text is type `1`, a `u32` length,
then UTF-8. A held key is repeated downs with an increasing repeat count, then
an up.

The server's first argument is its version and must equal the jar's, so the
flake passes `pkgs.scrcpy.version` and the jar path in through `-ldflags`. If the
server will not start, the remote falls back to `adb shell input` and says so in
the status line.

On quit the worker closes the socket, kills the `adb shell` running the server,
and removes the forward. Check with `adb forward --list` and `ps -A` on the TV.

## Keys

Mode is dispatched before key. Normal mode collects a count (two digits, capped
at 50, and a lone `0` is not a count), a `g` prefix, and then one action. `Esc`
first cancels a pending count and only otherwise means Back. Insert mode sends
printable runes as text, Backspace as `KEYCODE_DEL`, Enter as `KEYCODE_ENTER`,
and arrows as the D-pad. `Ctrl-c` quits from every mode.

Updates never block: actions go into a buffered channel, and one goroutine sends
them in order. A full queue drops the key and says so.

## Testing

`go test -race ./...` in `remote/` covers the key logic. For anything about the
screen or the TV, run the binary in tmux on a private socket, never the default
one, and look at the TV with `adb shell screencap`:

```sh
tmux -L aguatv-test new-session -d -s rem -x 70 -y 30 'aguatv remote'
tmux -L aguatv-test send-keys -t rem -l g
tmux -L aguatv-test capture-pane -p -t rem
tmux -L aguatv-test kill-session -t rem
```
