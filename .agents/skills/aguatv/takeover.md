# The HOME takeover

Fire OS will not let another app be the home screen.

- `cmd package set-home-activity` prints `Success` and changes nothing. HOME still
  resolves to `com.amazon.tv.launcher`, which is a protected package, as is
  `com.amazon.firehomestarter`. Neither can be disabled without root.
- Amazon's launcher declares HOME at priority 950. A third-party app cannot
  out-rank it.
- The HOME key is consumed before any accessibility key filter sees it.

So aguatv does not intercept HOME. It reacts to Amazon's home window appearing.

## How Guard works

1. `Guard` is an accessibility service listening only for
   `TYPE_WINDOW_STATE_CHANGED`. It cannot read screen content
   (`canRetrieveWindowContent="false"`).
2. When the window is `com.amazon.tv.launcher.ui.HomeActivity_vNext`, it shows an
   opaque overlay in aguatv's background colour, so Amazon's screen never appears.
3. Once the overlay has drawn, it launches `Home` through an `AlarmManager`
   `PendingIntent`. A plain `startActivity` right after HOME is held back by
   Android's five-second app-switch delay; an alarm fired by system_server is not.
4. That alarm only fires immediately with `min_futurity=0`. `aguatv install` sets
   it in the global `alarm_manager_constants` (DeviceConfig `alarm_manager` from
   sdk 31) and saves any earlier value in `aguatv_min_futurity` so `undo` can put
   it back.
5. `Home` calls `Guard.shown()` once the page has painted (the page calls
   `host.ready()` after two animation frames), and the overlay is removed. The
   overlay also times out after 1.5 s.
6. Waking from standby resumes Amazon's home without a window event, so Guard
   also relaunches on `ACTION_SCREEN_ON` when Amazon's home was last in front.

Amazon's other launcher screens, such as its settings grid, are left alone.
There is no way to visit Amazon's home from aguatv: an earlier "Amazon's home"
action paused the guard until another app opened, so HOME kept landing on
Amazon's screen and its ads. `aguatv undo` is the way back.

## Install steps that matter

- `deviceidle whitelist +tv.aguatv`, so Doze does not delay the alarm.
- `enabled_accessibility_services` is cleared, then rewritten with the guard
  appended, so a reinstalled guard is unbound and bound again rather than left
  pointing at the old process.
- Some Fire OS builds only bind accessibility services Amazon trusts
  (`A11yAuthorizer` in logcat). There the takeover cannot work; `install` says so
  and undoes its settings.

## Settings

`Settings` is opened with `android.intent.category.LEANBACK_SETTINGS`. The generic
`Settings.ACTION_SETTINGS` resolves to a stub that reopens Amazon's home.
