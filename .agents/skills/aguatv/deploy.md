# Declarative deploy

A TV's state is a Nix module. `aguatv.lib.mkTvs` evaluates it, writes it out as
JSON, and wraps `aguatv-deploy`, which compares that JSON with the TV and makes the
TV match.

## The layers

`lib.evalModules` merges, lowest priority first:

1. `nix/dumbtv.nix`: dumbtv's `data/fireos/*.txt`, parsed at evaluation time from
   the pinned `dumbtv` input (`flake = false`). Every value is `mkDefault`.
   - `packages.txt`: `state = "disabled"`, `group` from its `# --- section ---`
   - `protected.txt`: `protected = true`, `state = "disabled"`
   - `alexa.txt`: `group = "alexa"`, disabled when `dumbtv.alexa = false`
   - `user-apps.txt`: `group = "apps"`, left `unmanaged`
   - `never-disable.txt`: `pinned = true`, `state = "enabled"`
   - `settings.txt`: `settings.<namespace>.<key>`
   Trailing `# comments`, and indented `#` lines under an entry, become `note`.
2. The TV's own module, in a private flake. Plain assignments win.

`nix flake update dumbtv` is how new lists arrive. Diff them before deploying.

## Options (`nix/module.nix`)

| option | meaning |
|---|---|
| `name` | defaults to the attribute name in `tvs` |
| `address`, `serial` | `adb connect` target, or an existing adb serial |
| `packages.<pkg>.state` | `enabled`, `disabled`, `absent` (uninstalled for user 0), `unmanaged` |
| `packages.<pkg>.protected` | Fire OS refuses to disable it; `disabled` then means starved |
| `packages.<pkg>.pinned` | evaluation fails if it is set `disabled` or `absent`; set `pinned = false` beside the state to override |
| `packages.<pkg>.group`, `.note` | shown in the plan |
| `settings.<ns>.<key>` | `adb shell settings put`; bools become `1`/`0` |
| `home.enable` | install aguatv and take over HOME; `false` hands HOME back |
| `player.enable` | install and keep current the video player; `false` removes it only if deploy installed it, so a player that `aguatv play` installed on its own is left alone |
| `dumbtv.enable`, `dumbtv.alexa` | use the dumbtv layer; keep Alexa |

## What deploy does

`aguatv-deploy [-dry-run] [-yes] [-no-remote] plan.json`

1. Reads the TV: `pm list packages -u`, installed, `-d`; `settings get` and
   `cmd appops get … RUN_ANY_IN_BACKGROUND` in one shell call; whether the HOME
   guard is bound.
2. Plans. Packages not on this TV are skipped and counted. A disable that Fire OS
   refuses as protected is applied as a starve and reported.
3. Undoes what left the config, from its record in
   `$XDG_STATE_HOME/aguatv/<name>.json`: re-enables, reinstalls, unstarves, and
   restores settings to the value they had before deploy first changed them.
4. Reports drift: packages disabled on the TV that the config does not mention.
5. After applying, offers once to `nix profile install` the remote if it is not on
   `PATH`, only on a terminal, never with `-no-remote`.

The record adopts everything the config manages that already holds on the TV, so
the first deploy against a TV cleaned by hand still knows what to undo later.
Settings changed by hand before the first deploy have no recorded original; dropping
them from the config leaves them as they are.

Starving is `RUN_IN_BACKGROUND` and `RUN_ANY_IN_BACKGROUND` set to `ignore`.
Blocking a protected package's servers is DNS work, which deploy does not do yet.

## A private flake

```nix
{
  inputs.aguatv.url = "git+ssh://git@<host>/<you>/aguatv.git";
  outputs = { aguatv, ... }:
    let tvs = aguatv.lib.mkTvs { system = "x86_64-linux"; tvs.living-room = ./living-room.nix; };
    in { apps.x86_64-linux = tvs.apps; packages.x86_64-linux = tvs.packages; };
}
```

`nix run .#living-room -- -dry-run`, then without it. `nix build .#living-room-plan`
shows the JSON. Keep addresses and serials there, never in this repo.
