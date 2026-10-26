package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type grants map[string]bool

func (g grants) CanRead(_ context.Context, by Caster, key string) (bool, error) {
	return g[by.User+":"+key], nil
}

type th struct {
	t     *testing.T
	h     *Hub
	store *Store
	g     grants
	dir   string
}

func testHub(t *testing.T, adb map[string]*link) *th {
	dir := t.TempDir()
	store, err := OpenStore(filepath.Join(dir, "devices.json"))
	if err != nil {
		t.Fatal(err)
	}
	g := grants{}
	h := NewHub(store, NewCasts(g), dirSource("testdata/files"), adb, http.NotFoundHandler(), "https://tv.example.com", true)
	h.wait = 50 * time.Millisecond
	return &th{t: t, h: h, store: store, g: g, dir: dir}
}

func (x *th) req(method, path, body string, set func(*http.Request)) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, path, strings.NewReader(body))
	if body != "" {
		r.Header.Set("Content-Type", "application/json")
	}
	if set != nil {
		set(r)
	}
	w := httptest.NewRecorder()
	x.h.ServeHTTP(w, r)
	return w
}

func (x *th) do(user, method, path, body string) *httptest.ResponseRecorder {
	return x.req(method, path, body, func(r *http.Request) {
		if user != "" {
			r.Header.Set("Remote-User", user)
		}
	})
}

func (x *th) dev(cookie *http.Cookie, method, path, body string) *httptest.ResponseRecorder {
	return x.req(method, path, body, func(r *http.Request) {
		if cookie != nil {
			r.AddCookie(cookie)
		}
	})
}

func jsonOf(t *testing.T, w *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &m); err != nil {
		t.Fatalf("not json (%d): %s", w.Code, w.Body)
	}
	return m
}

func (x *th) link(user, kind, name string) *http.Cookie {
	x.t.Helper()
	w := x.dev(nil, http.MethodPost, "/device/start", `{"kind":"`+kind+`","name":"`+name+`"}`)
	if w.Code != 200 {
		x.t.Fatalf("start %d %s", w.Code, w.Body)
	}
	s := jsonOf(x.t, w)
	code, claim := s["code"].(string), s["claim"].(string)
	if w := x.do(user, http.MethodPost, "/api/link", `{"code":"`+code+`"}`); w.Code != 200 {
		x.t.Fatalf("approve %d %s", w.Code, w.Body)
	}
	w = x.dev(nil, http.MethodPost, "/device/claim", `{"code":"`+code+`","claim":"`+claim+`"}`)
	if w.Code != 200 {
		x.t.Fatalf("claim %d %s", w.Code, w.Body)
	}
	for _, c := range w.Result().Cookies() {
		if c.Name == cookieName {
			if !c.HttpOnly || !c.Secure || c.SameSite != http.SameSiteStrictMode {
				x.t.Fatalf("cookie is not HttpOnly, Secure and SameSite=Strict: %+v", c)
			}
			return c
		}
	}
	x.t.Fatal("no device cookie")
	return nil
}

func TestLinkingEndToEnd(t *testing.T) {
	x := testHub(t, nil)
	c := x.link("alice", "tv", "Living room")
	w := x.dev(c, http.MethodGet, "/device/whoami", "")
	if w.Code != 200 || jsonOf(t, w)["name"] != "Living room" {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	w = x.do("alice", http.MethodGet, "/api/devices", "")
	if !strings.Contains(w.Body.String(), "Living room") {
		t.Fatalf("alice's list: %s", w.Body)
	}
	if w := x.do("bob", http.MethodGet, "/api/devices", ""); strings.Contains(w.Body.String(), "Living room") {
		t.Fatalf("bob sees alice's device: %s", w.Body)
	}
}

func TestClaimWaitsThenIsSingleUse(t *testing.T) {
	x := testHub(t, nil)
	s := jsonOf(t, x.dev(nil, http.MethodPost, "/device/start", `{"kind":"browser"}`))
	code, claim := s["code"].(string), s["claim"].(string)
	if w := x.dev(nil, http.MethodPost, "/device/claim", `{"code":"`+code+`","claim":"`+claim+`"}`); w.Code != 202 {
		t.Fatalf("before approval: %d", w.Code)
	}
	if w := x.dev(nil, http.MethodPost, "/device/claim", `{"code":"`+code+`","claim":"wrong"}`); w.Code != 410 {
		t.Fatalf("wrong claim secret: %d", w.Code)
	}
	x.do("alice", http.MethodPost, "/api/link", `{"code":"`+strings.ToLower(code)+`"}`)
	if w := x.dev(nil, http.MethodPost, "/device/claim", `{"code":"`+code+`","claim":"`+claim+`"}`); w.Code != 200 {
		t.Fatalf("claim: %d", w.Code)
	}
	if w := x.dev(nil, http.MethodPost, "/device/claim", `{"code":"`+code+`","claim":"`+claim+`"}`); w.Code != 410 {
		t.Fatalf("second claim: %d", w.Code)
	}
	if w := x.do("bob", http.MethodPost, "/api/link", `{"code":"`+code+`"}`); w.Code != 404 {
		t.Fatalf("spent code approved again: %d", w.Code)
	}
}

func TestTheTokenIsNotStoredInTheClear(t *testing.T) {
	x := testHub(t, nil)
	c := x.link("alice", "tv", "TV")
	b, err := os.ReadFile(filepath.Join(x.dir, "devices.json"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(b), c.Value) || !strings.Contains(string(b), hashToken(c.Value)) {
		t.Fatalf("devices.json: %s", b)
	}
	fi, _ := os.Stat(filepath.Join(x.dir, "devices.json"))
	if fi.Mode().Perm() != 0o600 {
		t.Fatalf("mode %v", fi.Mode())
	}
}

func TestDevicesAndPeopleCannotUseEachOthersDoors(t *testing.T) {
	x := testHub(t, nil)
	c := x.link("alice", "tv", "TV")
	if w := x.dev(c, http.MethodGet, "/api/devices", ""); w.Code != 401 {
		t.Fatalf("device cookie on a person's endpoint: %d", w.Code)
	}
	if w := x.do("alice", http.MethodGet, "/device/next", ""); w.Code != 401 {
		t.Fatalf("person on a device endpoint: %d", w.Code)
	}
	if w := x.do("", http.MethodGet, "/tvlogin", ""); w.Code != 401 {
		t.Fatalf("tvlogin without a login: %d", w.Code)
	}
}

func TestOnlyTheOwnerCanRenameCastOrUnlink(t *testing.T) {
	x := testHub(t, nil)
	x.link("alice", "tv", "TV")
	id := x.store.Owned("alice")[0].ID
	x.g["bob:films/clip.mp4"] = true
	for _, tc := range []struct{ method, path, body string }{
		{http.MethodPost, "/api/devices/" + id + "/rename", `{"name":"mine"}`},
		{http.MethodPost, "/api/devices/" + id + "/cast", `{"ref":"films/clip.mp4"}`},
		{http.MethodPost, "/api/devices/" + id + "/control", `{"action":"toggle"}`},
		{http.MethodDelete, "/api/devices/" + id, ""},
	} {
		if w := x.do("bob", tc.method, tc.path, tc.body); w.Code != 404 {
			t.Errorf("bob %s %s: %d", tc.method, tc.path, w.Code)
		}
	}
}

func TestCrossSitePostsAreRefused(t *testing.T) {
	x := testHub(t, nil)
	w := x.req(http.MethodPost, "/api/link", `{"code":"x"}`, func(r *http.Request) {
		r.Header.Set("Remote-User", "alice")
		r.Header.Set("Sec-Fetch-Site", "cross-site")
	})
	if w.Code != 403 {
		t.Fatalf("cross-site: %d", w.Code)
	}
	w = x.req(http.MethodPost, "/device/start", `{}`, func(r *http.Request) { r.Header.Set("Content-Type", "text/plain") })
	if w.Code != 403 {
		t.Fatalf("form post: %d", w.Code)
	}
}

func TestCastIsCheckedAgainstTheCallersRightsAndStreamsWithRanges(t *testing.T) {
	x := testHub(t, nil)
	c := x.link("alice", "tv", "TV")
	other := x.link("alice", "browser", "Laptop")
	id := ""
	for _, d := range x.store.Owned("alice") {
		if d.Kind == "tv" {
			id = d.ID
		}
	}
	if w := x.do("alice", http.MethodPost, "/api/devices/"+id+"/cast", `{"ref":"films/clip.mp4"}`); w.Code != 403 {
		t.Fatalf("cast without a grant: %d %s", w.Code, w.Body)
	}
	if w := x.do("alice", http.MethodPost, "/api/devices/"+id+"/cast", `{"ref":"../etc/passwd"}`); w.Code != 400 {
		t.Fatalf("a path that leaves the bucket: %d", w.Code)
	}
	x.g["alice:films/clip.mp4"] = true
	w := x.do("alice", http.MethodPost, "/api/devices/"+id+"/cast", `{"ref":"https://files.example.com/films/clip.mp4"}`)
	if w.Code != 200 {
		t.Fatalf("cast: %d %s", w.Code, w.Body)
	}
	cast := jsonOf(t, w)["cast"].(string)
	ev := jsonOf(t, x.dev(c, http.MethodGet, "/device/next", ""))
	if ev["kind"] != "cast" || ev["cast"] != cast || ev["type"] != "video/mp4" {
		t.Fatalf("event %v", ev)
	}
	w = x.req(http.MethodGet, "/stream?cast="+cast, "", func(r *http.Request) {
		r.AddCookie(c)
		r.Header.Set("Range", "bytes=10-13")
	})
	if w.Code != 206 || w.Body.String() != "abcd" || w.Header().Get("Content-Range") != "bytes 10-13/20" {
		t.Fatalf("range: %d %q %v", w.Code, w.Body, w.Header())
	}
	if w := x.dev(other, http.MethodGet, "/stream?cast="+cast, ""); w.Code != 404 {
		t.Fatalf("another device streamed it: %d", w.Code)
	}
}

func TestARevokedGrantStopsTheStream(t *testing.T) {
	x := testHub(t, nil)
	c := x.link("alice", "tv", "TV")
	id := x.store.Owned("alice")[0].ID
	x.g["alice:films/clip.mp4"] = true
	cast := jsonOf(t, x.do("alice", http.MethodPost, "/api/devices/"+id+"/cast", `{"ref":"films/clip.mp4"}`))["cast"].(string)
	delete(x.g, "alice:films/clip.mp4")
	later := time.Now().Add(2 * time.Minute)
	x.h.casts.now = func() time.Time { return later }
	if w := x.dev(c, http.MethodGet, "/stream?cast="+cast, ""); w.Code != 403 {
		t.Fatalf("after the grant went: %d", w.Code)
	}
}

func TestUnlinkRevokesAtOnce(t *testing.T) {
	x := testHub(t, nil)
	c := x.link("alice", "tv", "TV")
	id := x.store.Owned("alice")[0].ID
	if w := x.do("alice", http.MethodDelete, "/api/devices/"+id, ""); w.Code != 200 {
		t.Fatalf("unlink %d", w.Code)
	}
	if w := x.dev(c, http.MethodGet, "/device/next", ""); w.Code != 401 {
		t.Fatalf("after unlink: %d", w.Code)
	}
}

func TestATokenExpiresWhenUnused(t *testing.T) {
	x := testHub(t, nil)
	c := x.link("alice", "tv", "TV")
	later := time.Now().Add(91 * 24 * time.Hour)
	x.store.now = func() time.Time { return later }
	if w := x.dev(c, http.MethodGet, "/device/whoami", ""); w.Code != 401 {
		t.Fatalf("expired token: %d", w.Code)
	}
}

func TestLongPollTimesOutEmpty(t *testing.T) {
	x := testHub(t, nil)
	c := x.link("alice", "tv", "TV")
	if w := x.dev(c, http.MethodGet, "/device/next", ""); w.Code != 204 {
		t.Fatalf("idle poll: %d", w.Code)
	}
}

func TestStartIsRateLimitedPerAddress(t *testing.T) {
	x := testHub(t, nil)
	codes := 0
	for i := 0; i < 8; i++ {
		if x.dev(nil, http.MethodPost, "/device/start", `{}`).Code == 200 {
			codes++
		}
	}
	if codes != 5 {
		t.Fatalf("got %d codes from one address in a minute", codes)
	}
}

func TestQROnlyForAPendingCode(t *testing.T) {
	x := testHub(t, nil)
	if w := x.dev(nil, http.MethodGet, "/device/qr?code=AAAAAAA", ""); w.Code != 404 {
		t.Fatalf("unknown code: %d", w.Code)
	}
	code := jsonOf(t, x.dev(nil, http.MethodPost, "/device/start", `{}`))["code"].(string)
	w := x.dev(nil, http.MethodGet, "/device/qr?code="+code, "")
	if w.Code != 200 || w.Header().Get("Content-Type") != "image/png" {
		t.Fatalf("qr: %d %v", w.Code, w.Header())
	}
}
