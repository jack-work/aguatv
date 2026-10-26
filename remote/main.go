package main

import (
	"flag"
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
)

var (
	defaultJar    string
	scrcpyVersion string
	defaultAdb    = "adb"
)

type options struct {
	adb, serial, addr, jar, version string
	slow                            bool
}

func dial(o options) (tv, string, error) {
	a := adb{bin: o.adb, serial: o.serial}
	if o.addr != "" {
		if err := a.connect(o.addr); err != nil {
			return nil, "", err
		}
		if a.serial == "" {
			a.serial = o.addr
		}
	}
	if err := a.ready(); err != nil {
		return nil, "", err
	}
	if o.slow || o.jar == "" || o.version == "" {
		return shellInput{a}, "", nil
	}
	s, err := openScrcpy(a, o.jar, o.version)
	if err != nil {
		return shellInput{a}, "fell back to adb input: " + err.Error(), nil
	}
	return s, "", nil
}

func main() {
	var o options
	flag.StringVar(&o.serial, "s", os.Getenv("ANDROID_SERIAL"), "adb serial of the TV")
	flag.StringVar(&o.addr, "c", "", "adb connect to host[:port] first")
	flag.StringVar(&o.adb, "adb", defaultAdb, "adb binary")
	flag.StringVar(&o.jar, "jar", defaultJar, "scrcpy-server jar")
	flag.StringVar(&o.version, "scrcpy-version", scrcpyVersion, "version of that jar")
	flag.BoolVar(&o.slow, "slow", false, "use adb shell input instead of the scrcpy server")
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "usage: aguatv-remote [-c host:port] [-s serial] [-slow]\n\nA vim-keyed remote for an Android or Fire TV over adb. ? inside lists the keys.\n\n")
		flag.PrintDefaults()
	}
	flag.Parse()
	if o.addr != "" && o.serial == "" {
		o.serial = o.addr
	}

	queue := make(chan act, 256)
	ready := make(chan tv, 1)
	send := func(a act) bool {
		select {
		case queue <- a:
			return true
		default:
			return false
		}
	}
	connect := func() tea.Msg {
		t, note, err := dial(o)
		if err != nil {
			return failedMsg{err}
		}
		ready <- t
		if note != "" {
			return connectedMsg{via: t.name() + ": " + note}
		}
		return connectedMsg{via: t.name()}
	}

	p := tea.NewProgram(newModel(send, connect), tea.WithAltScreen())
	quit := make(chan struct{})
	done := make(chan struct{})
	go func() {
		defer close(done)
		var t tv
		select {
		case t = <-ready:
		case <-quit:
			return
		}
		defer t.close()
		for {
			select {
			case <-quit:
				return
			case a := <-queue:
				var err error
				switch {
				case a.text != "":
					err = t.text(a.text)
				case a.hold > 0:
					err = t.hold(a.code, a.hold)
				default:
					err = t.press(a.code)
				}
				if err != nil {
					p.Send(sendErrMsg{err})
				}
			}
		}
	}()

	_, err := p.Run()
	close(quit)
	<-done
	select {
	case t := <-ready:
		t.close()
	default:
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
