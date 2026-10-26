package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
	"time"
)

type th struct {
	t     *testing.T
	h     *Hub
	store *Store
	dir   string
	jar   map[string]*http.Cookie
}

func testHub(t *testing.T, adb map[string]*link) *th {
	dir := t.TempDir()
	store, err := OpenStore(filepath.Join(dir, "screens.json"))
	if err != nil {
		t.Fatal(err)
	}
	site := (fstest.MapFS{
		"index.html":     {Data: []byte("<!doctype html>home")},
		"app.js":         {Data: []byte("1")},
		"sub/index.html": {Data: []byte("<!doctype html>no policy")},
	})
	h := NewHub(store, &url.URL{Scheme: "https", Host: "files.example.com"}, adb, site, true)
	h.wait = 50 * time.Millisecond
	return &th{t: t, h: h, store: store, dir: dir, jar: map[string]*http.Cookie{}}
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

func (x *th) as(user, screen, method, path, body string) *httptest.ResponseRecorder {
	return x.req(method, path, body, func(r *http.Request) {
		if user != "" {
			r.Header.Set("Remote-User", user)
		}
		if c := x.jar[screen]; c != nil {
			r.AddCookie(c)
		}
	})
}

func (x *th) do(user, method, path, body string) *httptest.ResponseRecorder {
	return x.as(user, user+"/remote", method, path, body)
}

func jsonOf(t *testing.T, w *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &m); err != nil {
		t.Fatalf("not json (%d): %s", w.Code, w.Body)
	}
	return m
}

func (x *th) register(user, label string, remote bool) string {
	x.t.Helper()
	body, _ := json.Marshal(map[string]any{"name": label, "remote": remote})
	w := x.as(user, "", http.MethodPost, "/api/me", string(body))
	if w.Code != 200 {
		x.t.Fatalf("register: %d %s", w.Code, w.Body)
	}
	for _, c := range w.Result().Cookies() {
		if c.Name == cookieName {
			x.jar[user+"/"+label] = c
		}
	}
	if x.jar[user+"/"+label] == nil {
		x.t.Fatal("no screen cookie")
	}
	return jsonOf(x.t, w)["id"].(string)
}

func (x *th) next(user, screen string) map[string]any {
	x.t.Helper()
	w := x.as(user, screen, http.MethodGet, "/api/me/next", "")
	if w.Code == http.StatusNoContent {
		return nil
	}
	if w.Code != 200 {
		x.t.Fatalf("next: %d %s", w.Code, w.Body)
	}
	return jsonOf(x.t, w)
}

func TestEverythingNeedsALogin(t *testing.T) {
	x := testHub(t, nil)
	for _, p := range []string{"/", "/app.js", "/api/me", "/api/screens", "/api/me/next"} {
		if w := x.as("", "", http.MethodGet, p, ""); w.Code != 401 {
			t.Errorf("%s without a login: %d", p, w.Code)
		}
	}
}

func TestRegisteringSetsAStrictCookieAndKeepsOnlyItsHash(t *testing.T) {
	x := testHub(t, nil)
	x.register("alice", "den", false)
	c := x.jar["alice/den"]
	if !c.HttpOnly || !c.Secure || c.SameSite != http.SameSiteStrictMode || c.MaxAge <= 0 {
		t.Fatalf("cookie %+v", c)
	}
	b, err := os.ReadFile(filepath.Join(x.dir, "screens.json"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(b), c.Value) || !strings.Contains(string(b), hashToken(c.Value)) {
		t.Fatalf("store holds the token in the clear, or not its hash: %s", b)
	}
	me := jsonOf(t, x.as("alice", "alice/den", http.MethodGet, "/api/me", ""))
	if s, _ := me["screen"].(map[string]any); s == nil || s["name"] != "den" || s["remote"] != false {
		t.Fatalf("me: %v", me)
	}
}

func TestRegisteringAgainUpdatesTheSameScreen(t *testing.T) {
	x := testHub(t, nil)
	id := x.register("alice", "den", false)
	w := x.as("alice", "alice/den", http.MethodPost, "/api/me", `{"name":"study","remote":true}`)
	if w.Code != 200 || jsonOf(t, w)["id"] != id || len(x.store.Owned("alice")) != 1 {
		t.Fatalf("%d %s, %d screens", w.Code, w.Body, len(x.store.Owned("alice")))
	}
}

func TestAScreenCookieIsWorthlessUnderAnotherLogin(t *testing.T) {
	x := testHub(t, nil)
	x.register("alice", "remote", true)
	bob := x.register("bob", "tv", false)
	x.jar["bob/stolen"] = x.jar["alice/remote"]
	if w := x.as("bob", "bob/stolen", http.MethodPost, "/api/screens/"+bob+"/cast", `{"url":"films/a.mp4"}`); w.Code != 403 {
		t.Fatalf("alice's remote cookie under bob's login: %d", w.Code)
	}
	if w := x.as("bob", "bob/stolen", http.MethodGet, "/api/me/next", ""); w.Code != 401 {
		t.Fatalf("alice's cookie made bob a screen: %d", w.Code)
	}
}

func TestOnlyARemoteScreenMayCastControlRenameOrRemove(t *testing.T) {
	x := testHub(t, nil)
	tv := x.register("alice", "tv", false)
	calls := []struct{ method, path, body string }{
		{http.MethodPost, "/api/screens/" + tv + "/cast", `{"url":"films/a.mp4"}`},
		{http.MethodPost, "/api/screens/" + tv + "/control", `{"action":"toggle"}`},
		{http.MethodPost, "/api/screens/" + tv + "/name", `{"name":"x"}`},
		{http.MethodDelete, "/api/screens/" + tv, ""},
	}
	for _, screen := range []string{"", "alice/tv"} {
		for _, c := range calls {
			if w := x.as("alice", screen, c.method, c.path, c.body); w.Code != 403 {
				t.Errorf("%s %s from %q: %d", c.method, c.path, screen, w.Code)
			}
		}
	}
	x.register("alice", "remote", true)
	for _, c := range calls {
		if w := x.do("alice", c.method, c.path, c.body); w.Code != 200 {
			t.Errorf("%s %s from a remote: %d %s", c.method, c.path, w.Code, w.Body)
		}
	}
}

func TestARemoteReachesOnlyItsOwnersScreens(t *testing.T) {
	x := testHub(t, nil)
	bob := x.register("bob", "tv", false)
	x.register("alice", "remote", true)
	if w := x.do("alice", http.MethodPost, "/api/screens/"+bob+"/cast", `{"url":"films/a.mp4"}`); w.Code != 404 {
		t.Fatalf("alice cast to bob's screen: %d", w.Code)
	}
	if e := x.next("bob", "bob/tv"); e != nil {
		t.Fatalf("bob's screen got %v", e)
	}
	list := jsonOf(t, x.do("alice", http.MethodGet, "/api/screens", ""))
	if n := len(list["screens"].([]any)); n != 1 {
		t.Fatalf("alice sees %d screens", n)
	}
}

func TestACastIsAFilesURLDeliveredToTheScreen(t *testing.T) {
	x := testHub(t, nil)
	tv := x.register("alice", "tv", false)
	x.register("alice", "remote", true)
	for ref, want := range map[string]string{
		"films/A Film (1999).mkv":                   "https://files.example.com/films/A%20Film%20%281999%29.mkv",
		"/films/a.mp4":                              "https://files.example.com/films/a.mp4",
		"https://files.example.com/films/a%20b.mp4": "https://files.example.com/films/a%20b.mp4",
	} {
		body, _ := json.Marshal(map[string]string{"url": ref})
		if w := x.do("alice", http.MethodPost, "/api/screens/"+tv+"/cast", string(body)); w.Code != 200 {
			t.Fatalf("%q: %d %s", ref, w.Code, w.Body)
		}
		e := x.next("alice", "alice/tv")
		if e["kind"] != "cast" || e["url"] != want {
			t.Fatalf("%q: got %v, want %s", ref, e, want)
		}
	}
}

func TestACastRefusesAnythingButAFileOnTheFilesSite(t *testing.T) {
	x := testHub(t, nil)
	tv := x.register("alice", "tv", false)
	x.register("alice", "remote", true)
	for _, ref := range []string{
		"https://evil.example.com/films/a.mp4",
		"http://files.example.com/films/a.mp4",
		"https://user@files.example.com/films/a.mp4",
		"https://files.example.com/films/a.mp4?x-amz-signature=1",
		"https://files.example.com.evil.com/a.mp4",
		"javascript:alert(1)",
		"films/../secrets/a.mp4",
		"films/",
		"",
		"films/a\nb.mp4",
	} {
		body, _ := json.Marshal(map[string]string{"url": ref})
		if w := x.do("alice", http.MethodPost, "/api/screens/"+tv+"/cast", string(body)); w.Code != 400 {
			t.Errorf("%q: %d %s", ref, w.Code, w.Body)
		}
	}
	if e := x.next("alice", "alice/tv"); e != nil {
		t.Fatalf("a refused cast reached the screen: %v", e)
	}
}

func TestControlsAreAClosedSet(t *testing.T) {
	x := testHub(t, nil)
	tv := x.register("alice", "tv", false)
	x.register("alice", "remote", true)
	if w := x.do("alice", http.MethodPost, "/api/screens/"+tv+"/control", `{"action":"eval"}`); w.Code != 400 {
		t.Fatalf("got %d", w.Code)
	}
	if w := x.do("alice", http.MethodPost, "/api/screens/"+tv+"/control", `{"action":"seekFraction","value":0.5}`); w.Code != 200 {
		t.Fatalf("got %d", w.Code)
	}
	if e := x.next("alice", "alice/tv"); e["action"] != "seekFraction" || e["value"] != 0.5 {
		t.Fatalf("got %v", e)
	}
}

func TestAScreenReportsItsPlayback(t *testing.T) {
	x := testHub(t, nil)
	x.register("alice", "tv", false)
	x.register("alice", "remote", true)
	if w := x.as("alice", "alice/tv", http.MethodPost, "/api/me/state", `{"title":"a.mp4","position":3,"duration":10,"playing":true}`); w.Code != 204 {
		t.Fatalf("got %d", w.Code)
	}
	if !strings.Contains(x.do("alice", http.MethodGet, "/api/screens", "").Body.String(), `"title":"a.mp4"`) {
		t.Fatal("state not listed")
	}
}

func TestRemovingAScreenTellsItAndLocksItOut(t *testing.T) {
	x := testHub(t, nil)
	tv := x.register("alice", "tv", false)
	x.register("alice", "remote", true)
	if w := x.do("alice", http.MethodDelete, "/api/screens/"+tv, ""); w.Code != 200 {
		t.Fatalf("got %d", w.Code)
	}
	if w := x.as("alice", "alice/tv", http.MethodGet, "/api/me/next", ""); w.Code != 401 {
		t.Fatalf("removed screen still listens: %d", w.Code)
	}
}

func TestForgettingThisBrowserClearsTheCookie(t *testing.T) {
	x := testHub(t, nil)
	x.register("alice", "den", false)
	w := x.as("alice", "alice/den", http.MethodDelete, "/api/me", "")
	if w.Code != 200 || len(x.store.Owned("alice")) != 0 {
		t.Fatalf("%d, %d screens", w.Code, len(x.store.Owned("alice")))
	}
	c := w.Result().Cookies()
	if len(c) != 1 || c[0].Name != cookieName || c[0].MaxAge >= 0 {
		t.Fatalf("cookie not cleared: %v", c)
	}
}

func TestAnIdleScreenExpires(t *testing.T) {
	x := testHub(t, nil)
	x.register("alice", "den", false)
	x.store.now = func() time.Time { return time.Now().Add(91 * 24 * time.Hour) }
	if w := x.as("alice", "alice/den", http.MethodGet, "/api/me/next", ""); w.Code != 401 {
		t.Fatalf("got %d", w.Code)
	}
}

func TestCrossSitePostsAreRefused(t *testing.T) {
	x := testHub(t, nil)
	tv := x.register("alice", "tv", false)
	x.register("alice", "remote", true)
	w := x.req(http.MethodPost, "/api/screens/"+tv+"/control", `{"action":"stop"}`, func(r *http.Request) {
		r.Header.Set("Remote-User", "alice")
		r.Header.Set("Sec-Fetch-Site", "cross-site")
		r.AddCookie(x.jar["alice/remote"])
	})
	if w.Code != 403 {
		t.Fatalf("cross-site: %d", w.Code)
	}
	w = x.req(http.MethodPost, "/api/screens/"+tv+"/control", "action=stop", func(r *http.Request) {
		r.Header.Set("Remote-User", "alice")
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		r.AddCookie(x.jar["alice/remote"])
	})
	if w.Code != 403 {
		t.Fatalf("form post: %d", w.Code)
	}
}

func TestThePageCarriesACSPThatAllowsOnlyFilesMedia(t *testing.T) {
	x := testHub(t, nil)
	w := x.as("alice", "", http.MethodGet, "/", "")
	csp := w.Header().Get("Content-Security-Policy")
	if w.Code != 200 || !strings.Contains(csp, "media-src 'self' https://files.example.com") || !strings.Contains(csp, "default-src 'none'") || !strings.Contains(csp, "connect-src 'self';") {
		t.Fatalf("%d %q", w.Code, csp)
	}
	if w := x.as("alice", "", http.MethodGet, "/index.html", ""); w.Code != 404 {
		t.Fatalf("the page is reachable without its CSP: %d", w.Code)
	}
}

func TestLongPollTimesOutEmpty(t *testing.T) {
	x := testHub(t, nil)
	x.register("alice", "den", false)
	if e := x.next("alice", "alice/den"); e != nil {
		t.Fatalf("got %v", e)
	}
}

func TestNoPageIsServedWithoutItsPolicy(t *testing.T) {
	x := testHub(t, nil)
	for _, p := range []string{"/index.html", "/index.html.csp", "/sub/", "/sub", "/sub/index.html", "/missing.js"} {
		if w := x.as("alice", "", http.MethodGet, p, ""); w.Code != 404 {
			t.Errorf("%s: %d %s", p, w.Code, w.Body)
		}
	}
	if w := x.as("alice", "", http.MethodGet, "/app.js", ""); w.Code != 200 {
		t.Fatalf("an asset: %d", w.Code)
	}
}
