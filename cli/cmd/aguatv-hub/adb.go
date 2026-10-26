package main

import (
	"errors"
	"net"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/jack-work/aguatv/cli/internal/tv"
)

var (
	defaultJar    string
	scrcpyVersion string
	defaultAdb    = "adb"
)

type step struct {
	code  int
	hold  time.Duration
	shell []string
}

var actions = map[string]step{
	"up":         {code: tv.KeyUp},
	"down":       {code: tv.KeyDown},
	"left":       {code: tv.KeyLeft},
	"right":      {code: tv.KeyRight},
	"ok":         {code: tv.KeyCenter},
	"back":       {code: tv.KeyBack},
	"home":       {code: tv.KeyHome},
	"menu":       {code: tv.KeyMenu},
	"play":       {code: tv.KeyPlayPause},
	"rewind":     {code: tv.KeyRewind},
	"forward":    {code: tv.KeyFastForward},
	"volup":      {code: tv.KeyVolumeUp},
	"voldown":    {code: tv.KeyVolumeDown},
	"mute":       {code: tv.KeyMute},
	"power":      {code: tv.KeyPower},
	"wake":       {code: tv.KeyWakeup},
	"sleep":      {code: tv.KeySleep},
	"delete":     {code: tv.KeyDel},
	"enter":      {code: tv.KeyEnter},
	"quick":      {code: tv.KeyHome, hold: 1200 * time.Millisecond},
	"settings":   {shell: []string{"am", "start", "-n", "tv.aguatv/.Home", "--es", "aguatv.open", "settings"}},
	"tvsettings": {shell: []string{"am", "start", "-a", "android.intent.action.MAIN", "-c", "android.intent.category.LEANBACK_SETTINGS"}},
}

type link struct {
	mu      sync.Mutex
	dial    func() (tv.TV, string, string, error)
	t       tv.TV
	label   string
	note    string
	lastErr string
}

func (l *link) get() (tv.TV, error) {
	if l.t != nil {
		return l.t, nil
	}
	t, label, note, err := l.dial()
	if err != nil {
		l.lastErr = err.Error()
		return nil, err
	}
	l.t, l.label, l.note, l.lastErr = t, label, note, ""
	return t, nil
}

func (l *link) drop(err error) {
	if l.t != nil {
		l.t.Close()
		l.t = nil
	}
	l.lastErr = err.Error()
}

func (l *link) do(f func(tv.TV) error) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	for attempt := 0; attempt < 2; attempt++ {
		t, err := l.get()
		if err != nil {
			return err
		}
		if err = f(t); err == nil {
			return nil
		}
		l.drop(err)
	}
	return errors.New(l.lastErr)
}

func (l *link) status() map[string]any {
	l.mu.Lock()
	defer l.mu.Unlock()
	s := map[string]any{"connected": l.t != nil, "tv": l.label, "error": l.lastErr}
	if l.t != nil {
		s["via"] = l.t.Name()
		s["note"] = l.note
	}
	return s
}

func (s step) run(t tv.TV) error {
	switch {
	case len(s.shell) > 0:
		return t.Shell(s.shell...)
	case s.hold > 0:
		return t.Hold(s.code, s.hold)
	default:
		return t.Press(s.code)
	}
}

func listener(addr string) (net.Listener, error) {
	if os.Getenv("LISTEN_PID") == strconv.Itoa(os.Getpid()) && os.Getenv("LISTEN_FDS") == "1" {
		return net.FileListener(os.NewFile(3, "systemd"))
	}
	if addr == "" {
		return nil, errors.New("no socket from systemd and no -listen address")
	}
	return net.Listen("tcp", addr)
}
