package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/jack-work/aguatv/cli/internal/adb"
	"github.com/jack-work/aguatv/cli/internal/tv"
)

type adbTargets map[string]string

func (a adbTargets) String() string { return fmt.Sprint(map[string]string(a)) }

func (a adbTargets) Set(v string) error {
	name, target, ok := strings.Cut(v, "=")
	if !ok || name == "" || target == "" {
		return fmt.Errorf("want name=host:port, got %q", v)
	}
	a[name] = target
	return nil
}

func main() {
	targets := adbTargets{}
	var (
		listen, site, state, public, files, stream, adbBin, jar, version string
		insecure                                                         bool
	)
	flag.StringVar(&listen, "listen", "", "address to listen on when systemd passes no socket")
	flag.StringVar(&site, "site", "", "directory holding the pages")
	flag.StringVar(&state, "state", os.Getenv("STATE_DIRECTORY"), "directory for devices.json")
	flag.StringVar(&public, "public", "", "the hub's public URL, used in link codes")
	flag.StringVar(&files, "files", "", "origin that issues cast tickets, e.g. https://files.example.com")
	flag.StringVar(&stream, "stream", "", "where casts stream from with their ticket: unix:<socket>:<path> or a URL")
	flag.Var(targets, "adb", "an adb remote, name=host:port (repeatable)")
	var adbGroup string
	flag.StringVar(&adbGroup, "adb-group", "", "if set, only members of this Remote-Groups group may see or drive adb remotes")
	flag.StringVar(&adbBin, "adb-bin", defaultAdb, "adb binary")
	flag.StringVar(&jar, "jar", defaultJar, "scrcpy-server jar")
	flag.StringVar(&version, "scrcpy-version", scrcpyVersion, "version of that jar")
	flag.BoolVar(&insecure, "insecure-cookies", false, "send device cookies without Secure, for a plain-HTTP test hub")
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "usage: aguatv-hub -site <dir> -public <url> -state <dir> [-files <origin> -stream <target>] [-adb name=host:port -adb-group <group>]\n\nLinks screens, casts to them and drives adb TVs. It trusts Remote-User: run it behind a login proxy.\nCasts carry tickets issued by -files to the caster's browser; the hub streams through -stream with them.\n\n")
		flag.PrintDefaults()
	}
	flag.Parse()
	if site == "" || public == "" || state == "" {
		flag.Usage()
		os.Exit(2)
	}

	store, err := OpenStore(filepath.Join(state, "devices.json"))
	if err != nil {
		log.Fatal(err)
	}

	var src Source = noSource{}
	if stream != "" {
		ts, err := NewTicketStream(stream)
		if err != nil {
			log.Fatal(err)
		}
		src = ts
	} else if dir := os.Getenv("AGUATV_HUB_STUB_DIR"); dir != "" {
		src = dirSource(dir)
		log.Printf("cast source: local directory %s, tickets unchecked", dir)
	}

	remotes := map[string]*link{}
	for name, target := range targets {
		target := target
		remotes[name] = &link{dial: func() (tv.TV, string, string, error) {
			d, err := adb.Open(adbBin, target, target)
			if err != nil {
				return nil, "", "", err
			}
			t, note := tv.Open(d, jar, version, false)
			return t, d.Label(), note, nil
		}}
	}

	h := NewHub(store, NewCasts(), src, remotes, http.FileServer(http.Dir(site)), public, !insecure)
	h.adbGroup = adbGroup
	h.files = strings.TrimSuffix(files, "/")
	h.cspDir = site
	ln, err := listener(listen)
	if err != nil {
		log.Fatal(err)
	}
	srv := &http.Server{Handler: h, ReadHeaderTimeout: 10 * time.Second}
	log.Printf("hub at %s, %d adb remotes", public, len(remotes))
	log.Fatal(srv.Serve(ln))
}
