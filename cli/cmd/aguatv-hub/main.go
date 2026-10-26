package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"net/url"
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
	var listen, site, state, files, cspDir, adbGroup, adbBin, jar, version string
	var insecure bool
	flag.StringVar(&listen, "listen", "", "address to listen on when systemd passes no socket")
	flag.StringVar(&site, "site", "", "directory holding the web app")
	flag.StringVar(&cspDir, "csp", "", "directory of <page>.csp script hashes (default: -site)")
	flag.StringVar(&state, "state", os.Getenv("STATE_DIRECTORY"), "directory for screens.json")
	flag.StringVar(&files, "files", "", "origin screens may be cast from, like https://files.example.com")
	flag.Var(targets, "adb", "an adb remote, name=host:port (repeatable)")
	flag.StringVar(&adbGroup, "adb-group", "", "if set, only members of this Remote-Groups group may see or drive adb remotes")
	flag.StringVar(&adbBin, "adb-bin", defaultAdb, "adb binary")
	flag.StringVar(&jar, "jar", defaultJar, "scrcpy-server jar")
	flag.StringVar(&version, "scrcpy-version", scrcpyVersion, "version of that jar")
	flag.BoolVar(&insecure, "insecure-cookies", false, "send screen cookies without Secure, for a plain-HTTP test hub")
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "usage: aguatv-hub -site <dir> -state <dir> -files <origin> [-adb name=host:port]\n\nKeeps a person's screens, casts files to them and drives adb TVs.\nIt trusts Remote-User and Remote-Groups: run it behind a login proxy that sets them.\n\n")
		flag.PrintDefaults()
	}
	flag.Parse()
	if site == "" || state == "" {
		flag.Usage()
		os.Exit(2)
	}
	if cspDir == "" {
		cspDir = site
	}
	var filesOrigin *url.URL
	if files != "" {
		u, err := url.Parse(files)
		if err != nil || u.Scheme != "https" && u.Scheme != "http" || u.Host == "" || strings.Trim(u.Path, "/") != "" {
			log.Fatalf("-files %q is not an origin like https://files.example.com", files)
		}
		filesOrigin = &url.URL{Scheme: u.Scheme, Host: u.Host}
	}

	store, err := OpenStore(filepath.Join(state, "screens.json"))
	if err != nil {
		log.Fatal(err)
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

	h := NewHub(store, filesOrigin, remotes, os.DirFS(site), !insecure)
	h.adbGroup, h.cspDir = adbGroup, cspDir
	ln, err := listener(listen)
	if err != nil {
		log.Fatal(err)
	}
	srv := &http.Server{Handler: h, ReadHeaderTimeout: 10 * time.Second}
	log.Printf("hub up, %d adb remotes", len(remotes))
	log.Fatal(srv.Serve(ln))
}
