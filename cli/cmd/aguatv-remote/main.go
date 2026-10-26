package main

import (
	"flag"
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/jack-work/aguatv/cli/internal/adb"
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

func dial(o options) (tv, string, string, error) {
	a, err := adb.Open(o.adb, o.serial, o.addr)
	if err != nil {
		return nil, "", "", err
	}
	label := a.Label()
	if o.slow || o.jar == "" || o.version == "" {
		return shellInput{a}, label, "", nil
	}
	s, err := openScrcpy(a, o.jar, o.version)
	if err != nil {
		return shellInput{a}, label, "fell back to adb input: " + err.Error(), nil
	}
	return s, label, "", nil
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
		fmt.Fprintf(os.Stderr, "usage: aguatv-remote [-c host:port] [-s serial] [-slow]\n\nA vim-keyed remote for any Android TV, Google TV or Fire TV that adb can reach. ? inside lists the keys.\n\n")
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
		t, label, note, err := dial(o)
		if err != nil {
			return failedMsg{err}
		}
		ready <- t
		via := label + " · " + t.name()
		if note != "" {
			via += ": " + note
		}
		return connectedMsg{via: via}
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
				case len(a.shell) > 0:
					err = t.shell(a.shell...)
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
