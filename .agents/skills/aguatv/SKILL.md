---
name: aguatv
description: Working on aguatv, a Fire TV home screen whose interface is a zanni-built web page inside a small Java shell. Read before changing the page, the Java bridge, the HOME takeover, the build, or before telling anyone how much memory it uses. Indexes deeper pages on the takeover, the bridge, the build and memory.
---

# aguatv

A Java shell that owns the parts Android only lets Java do, and a web page that
owns everything you see.

```
android/src/tv/aguatv/
  Home.java    the activity: a full-screen WebView, the asset server, key routing
  Bridge.java  the methods the page can call, exposed as window.host
  Shelf.java   apps, inputs and app art, as JSON and PNG
  Input.java   shows an HDMI or AV input through TvView
  Guard.java   the accessibility service that takes HOME away from Amazon
web/
  index.html   the page; zanni markers are replaced at build time
  home.css     layout, on top of zanni's gesso, phosphor, fontpack and boil
  home.js      rows, D-pad focus, the clock, window.aguatv; in a browser, screens and the adb pad
  cast.js      a cast, full screen under zanni's filo
cli/           one Go module: cmd/aguatv-remote, cmd/aguatv-deploy, cmd/aguatv-hub, internal/{adb,tv,play}
nix/           module.nix (TV options), dumbtv.nix (dumbtv's lists as a layer),
               hub-module.nix (services.aguatv-hub), csp-hashes.py
bin/aguatv     install, undo, status, apk, remote
flake.nix      web, apk, cli, remote, aguatv packages; lib.mkTvs
```

## Rules

- No `INTERNET` permission, ever. The page is served from the APK through
  `shouldInterceptRequest` on `https://aguatv.invalid`; any other origin gets 403.
- No comments in shipped code. Reasons go in commit messages, explanations go here.
- The page runs in two hosts: the Android shell, and a browser behind the hub.
  `?preview` stands in for the shell with sample data. Keep all three working.
- Measure on a television before claiming a performance number.

## Pages

| page | open it when |
|---|---|
| [takeover.md](takeover.md) | HOME lands on Amazon's screen, the guard will not bind, or you touch `Guard.java` or `bin/aguatv` |
| [bridge.md](bridge.md) | adding something the page can ask for or do, or changing keys |
| [build.md](build.md) | the build fails, the SDK changes, or signing goes wrong |
| [hub.md](hub.md) | screens, the remote grant, casting, the hub's security rules, or testing it on a workstation |
| [deploy.md](deploy.md) | declaring a TV's packages and settings, or changing aguatv-deploy |
| [remote.md](remote.md) | changing the terminal remote, its keys, or how it reaches the TV |
| [player.md](player.md) | playing videos on the TV, the player APK, or AGUATV_PRESIGN |
| [memory.md](memory.md) | the TV feels slow, or before adding an effect to the page |
