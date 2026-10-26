package main

import (
	"context"
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

type stubAuthz map[string]bool

func (s stubAuthz) CanRead(_ context.Context, by Caster, key string) (bool, error) {
	return s[by.User+":"+key] || s[by.User+":*"], nil
}

func main() {
	targets := adbTargets{}
	var (
		listen, site, state, public, authz, s3Endpoint, s3Bucket, s3Region, adbBin, jar, version string
		insecure                                                                                 bool
	)
	flag.StringVar(&listen, "listen", "", "address to listen on when systemd passes no socket")
	flag.StringVar(&site, "site", "", "directory holding the pages")
	flag.StringVar(&state, "state", os.Getenv("STATE_DIRECTORY"), "directory for devices.json")
	flag.StringVar(&public, "public", "", "the hub's public URL, used in link codes")
	flag.StringVar(&authz, "files-authz", "", "files authorizer: unix:<socket>[:<path>] or a URL")
	flag.StringVar(&s3Endpoint, "s3-endpoint", "", "S3 endpoint for casts")
	flag.StringVar(&s3Bucket, "s3-bucket", "", "S3 bucket for casts")
	flag.StringVar(&s3Region, "s3-region", "garage", "S3 region")
	flag.Var(targets, "adb", "an adb remote, name=host:port (repeatable)")
	var adbGroup string
	flag.StringVar(&adbGroup, "adb-group", "", "if set, only members of this Remote-Groups group may see or drive adb remotes")
	flag.StringVar(&adbBin, "adb-bin", defaultAdb, "adb binary")
	flag.StringVar(&jar, "jar", defaultJar, "scrcpy-server jar")
	flag.StringVar(&version, "scrcpy-version", scrcpyVersion, "version of that jar")
	flag.BoolVar(&insecure, "insecure-cookies", false, "send device cookies without Secure, for a plain-HTTP test hub")
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "usage: aguatv-hub -site <dir> -public <url> -state <dir> [-files-authz … -s3-endpoint … -s3-bucket …] [-adb name=host:port]\n\nLinks screens, casts to them and drives adb TVs. It trusts Remote-User: run it behind a login proxy.\nS3 keys come from the credentials s3-key-id and s3-secret ($CREDENTIALS_DIRECTORY).\n\n")
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

	var a Authorizer = stubAuthz{}
	if authz != "" {
		fa, err := NewFilesAuthz(authz)
		if err != nil {
			log.Fatal(err)
		}
		a = fa
	} else if v := os.Getenv("AGUATV_HUB_STUB_GRANTS"); v != "" {
		s := stubAuthz{}
		for _, g := range strings.Fields(v) {
			s[g] = true
		}
		a = s
		log.Printf("files authorizer: local stub with %d grants", len(s))
	}

	var src Source = noSource{}
	if s3Endpoint != "" {
		id, err := readCredential("s3-key-id")
		if err != nil {
			log.Fatal(err)
		}
		secret, err := readCredential("s3-secret")
		if err != nil {
			log.Fatal(err)
		}
		src = &S3{Endpoint: s3Endpoint, Bucket: s3Bucket, Region: s3Region, KeyID: id, Secret: secret, Client: &http.Client{}}
	} else if dir := os.Getenv("AGUATV_HUB_STUB_DIR"); dir != "" {
		src = dirSource(dir)
		log.Printf("cast source: local directory %s", dir)
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

	h := NewHub(store, NewCasts(a), src, remotes, http.FileServer(http.Dir(site)), public, !insecure)
	h.adbGroup = adbGroup
	ln, err := listener(listen)
	if err != nil {
		log.Fatal(err)
	}
	srv := &http.Server{Handler: h, ReadHeaderTimeout: 10 * time.Second}
	log.Printf("hub at %s, %d adb remotes", public, len(remotes))
	log.Fatal(srv.Serve(ln))
}
