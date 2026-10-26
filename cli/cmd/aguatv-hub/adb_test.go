package main

import (
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/jack-work/aguatv/cli/internal/tv"
)

type fake struct {
	presses []int
	texts   []string
	shells  [][]string
	broken  bool
	closed  int
}

func (f *fake) Press(code int) error {
	if f.broken {
		return errors.New("socket closed")
	}
	f.presses = append(f.presses, code)
	return nil
}
func (f *fake) Hold(code int, _ time.Duration) error { return f.Press(code) }
func (f *fake) Text(s string) error                  { f.texts = append(f.texts, s); return nil }
func (f *fake) Shell(a ...string) error              { f.shells = append(f.shells, a); return nil }
func (f *fake) Name() string                         { return "fake" }
func (f *fake) Close()                               { f.closed++ }

func adbLink(tvs ...*fake) (*link, *int) {
	dials := 0
	return &link{dial: func() (tv.TV, string, string, error) {
		t := tvs[min(dials, len(tvs)-1)]
		dials++
		return t, "Test TV", "", nil
	}}, &dials
}

func TestADBPressRepeats(t *testing.T) {
	f := &fake{}
	l, _ := adbLink(f)
	h := testHub(t, map[string]*link{"tv": l})
	h.register("alice", "remote", true)
	if w := h.do("alice", http.MethodPost, "/api/adb/tv/press", `{"action":"down","times":3}`); w.Code != 200 {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	if len(f.presses) != 3 || f.presses[0] != tv.KeyDown {
		t.Fatalf("got %v", f.presses)
	}
}

func TestADBUnknownActionAndRemoteAreRefused(t *testing.T) {
	l, _ := adbLink(&fake{})
	h := testHub(t, map[string]*link{"tv": l})
	h.register("alice", "remote", true)
	if w := h.do("alice", http.MethodPost, "/api/adb/tv/press", `{"action":"selfdestruct"}`); w.Code != 400 {
		t.Fatalf("got %d", w.Code)
	}
	if w := h.do("alice", http.MethodPost, "/api/adb/elsewhere/press", `{"action":"ok"}`); w.Code != 404 {
		t.Fatalf("got %d", w.Code)
	}
}

func TestADBNeedsALogin(t *testing.T) {
	f := &fake{}
	l, _ := adbLink(f)
	h := testHub(t, map[string]*link{"tv": l})
	h.register("alice", "remote", true)
	if w := h.do("", http.MethodPost, "/api/adb/tv/press", `{"action":"ok"}`); w.Code != 401 || len(f.presses) != 0 {
		t.Fatalf("got %d, presses %v", w.Code, f.presses)
	}
}

func TestADBBrokenConnectionIsRedialledOnce(t *testing.T) {
	dead, alive := &fake{broken: true}, &fake{}
	l, dials := adbLink(dead, alive)
	h := testHub(t, map[string]*link{"tv": l})
	h.register("alice", "remote", true)
	if w := h.do("alice", http.MethodPost, "/api/adb/tv/press", `{"action":"ok"}`); w.Code != 200 {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	if *dials != 2 || dead.closed != 1 || len(alive.presses) != 1 {
		t.Fatalf("dials %d closed %d presses %v", *dials, dead.closed, alive.presses)
	}
}

func TestADBTextRefusesControlCharacters(t *testing.T) {
	f := &fake{}
	l, _ := adbLink(f)
	h := testHub(t, map[string]*link{"tv": l})
	h.register("alice", "remote", true)
	if w := h.do("alice", http.MethodPost, "/api/adb/tv/text", `{"text":"a\nreboot"}`); w.Code != 400 || len(f.texts) != 0 {
		t.Fatalf("got %d, texts %v", w.Code, f.texts)
	}
	if w := h.do("alice", http.MethodPost, "/api/adb/tv/text", "{\"text\":\"`reboot` $(id); x\"}"); w.Code != 200 || len(f.texts) != 1 {
		t.Fatalf("printable text with shell characters: %d %v", w.Code, f.texts)
	}
}

func TestADBGroupGatesRemotes(t *testing.T) {
	f := &fake{}
	l, _ := adbLink(f)
	h := testHub(t, map[string]*link{"tv": l})
	h.register("alice", "remote", true)
	h.h.adbGroup = "tv-adb"
	press := func(groups string) int {
		return h.req(http.MethodPost, "/api/adb/tv/press", `{"action":"ok"}`, func(r *http.Request) {
			r.Header.Set("Remote-User", "alice")
			r.Header.Set("Remote-Groups", groups)
			r.AddCookie(h.jar["alice/remote"])
		}).Code
	}
	if c := press("site-tv-access"); c != 403 {
		t.Fatalf("without the group: %d", c)
	}
	if c := press("site-tv-access, tv-adb"); c != 200 {
		t.Fatalf("with the group: %d", c)
	}
	list := h.do("alice", http.MethodGet, "/api/screens", "")
	if strings.Contains(list.Body.String(), `"name":"tv"`) {
		t.Fatalf("remote listed to a non-member: %s", list.Body)
	}
}

func TestADBNeedsARemoteScreen(t *testing.T) {
	f := &fake{}
	l, _ := adbLink(f)
	h := testHub(t, map[string]*link{"tv": l})
	h.register("alice", "den", false)
	if w := h.as("alice", "alice/den", http.MethodPost, "/api/adb/tv/press", `{"action":"ok"}`); w.Code != 403 || len(f.presses) != 0 {
		t.Fatalf("got %d, presses %v", w.Code, f.presses)
	}
	if strings.Contains(h.as("alice", "alice/den", http.MethodGet, "/api/screens", "").Body.String(), `"name":"tv"`) {
		t.Fatal("adb remote listed to a screen that is not a remote")
	}
}
