package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/jack-work/aguatv/cli/internal/play"
	"github.com/jack-work/aguatv/cli/internal/tv"
)

const cookieName = "aguatv_screen"

type PlayState struct {
	Title    string    `json:"title"`
	Position float64   `json:"position"`
	Duration float64   `json:"duration"`
	Playing  bool      `json:"playing"`
	At       time.Time `json:"at"`
}

type Caller struct {
	User   string
	Groups string
	Screen *Screen
}

func (c Caller) remote() bool { return c.Screen != nil && c.Screen.Remote }

type Hub struct {
	store    *Store
	queues   *Queues
	adb      map[string]*link
	adbGroup string
	site     fs.FS
	cspDir   string
	files    *url.URL
	secure   bool
	wait     time.Duration
	stateMu  sync.Mutex
	state    map[string]PlayState
	mux      *http.ServeMux
}

func NewHub(store *Store, files *url.URL, adb map[string]*link, site fs.FS, secure bool) *Hub {
	h := &Hub{
		store: store, queues: NewQueues(), adb: adb, site: site, files: files, secure: secure,
		wait: 25 * time.Second, state: map[string]PlayState{}, mux: http.NewServeMux(),
	}
	m := h.mux
	m.HandleFunc("GET /api/me", h.as(h.me))
	m.HandleFunc("POST /api/me", h.as(h.register))
	m.HandleFunc("DELETE /api/me", h.as(h.forget))
	m.HandleFunc("GET /api/me/next", h.asScreen(h.next))
	m.HandleFunc("POST /api/me/state", h.asScreen(h.report))
	m.HandleFunc("GET /api/screens", h.as(h.screens))
	m.HandleFunc("POST /api/screens/{id}/cast", h.asRemote(h.cast))
	m.HandleFunc("POST /api/screens/{id}/control", h.asRemote(h.control))
	m.HandleFunc("POST /api/screens/{id}/name", h.asRemote(h.rename))
	m.HandleFunc("DELETE /api/screens/{id}", h.asRemote(h.remove))
	m.HandleFunc("POST /api/adb/{name}/press", h.asRemote(h.adbPress))
	m.HandleFunc("POST /api/adb/{name}/text", h.asRemote(h.adbText))
	m.HandleFunc("GET /{$}", h.as(h.page("index.html")))
	static := http.FileServerFS(site)
	m.HandleFunc("GET /", h.as(func(w http.ResponseWriter, r *http.Request, _ Caller) {
		name := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
		if fi, err := fs.Stat(site, name); err != nil || fi.IsDir() || strings.HasSuffix(name, ".html") || strings.HasSuffix(name, ".csp") {
			http.NotFound(w, r)
			return
		}
		static.ServeHTTP(w, r)
	}))
	return h
}

func (h *Hub) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "private, no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	if r.Method != http.MethodGet && r.Method != http.MethodHead && !sameOrigin(r) {
		fail(w, http.StatusForbidden, errors.New("same-origin JSON requests only"))
		return
	}
	h.mux.ServeHTTP(w, r)
}

func reply(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(v)
}

func fail(w http.ResponseWriter, code int, err error) {
	reply(w, code, map[string]string{"error": err.Error()})
}

func sameOrigin(r *http.Request) bool {
	switch r.Header.Get("Sec-Fetch-Site") {
	case "", "same-origin", "none":
	default:
		return false
	}
	return strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") || r.Method == http.MethodDelete
}

func decode(w http.ResponseWriter, r *http.Request, v any) bool {
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(v); err != nil && !errors.Is(err, io.EOF) {
		fail(w, http.StatusBadRequest, err)
		return false
	}
	return true
}

func (h *Hub) csp(name string) string {
	var hashes string
	if h.cspDir != "" {
		if b, err := os.ReadFile(filepath.Join(h.cspDir, filepath.Base(name)+".csp")); err == nil {
			hashes = strings.TrimSpace(string(b))
		}
	}
	media := "'self'"
	if h.files != nil {
		media += " " + h.files.Scheme + "://" + h.files.Host
	}
	return strings.Join([]string{
		"default-src 'none'",
		strings.TrimSpace("script-src 'self' " + hashes),
		"style-src 'self' 'unsafe-inline'",
		"img-src 'self' data:",
		"font-src 'self' data:",
		"media-src " + media,
		"connect-src 'self'",
		"base-uri 'none'",
		"form-action 'none'",
		"frame-ancestors 'none'",
	}, "; ")
}

func (h *Hub) page(name string) func(http.ResponseWriter, *http.Request, Caller) {
	return func(w http.ResponseWriter, r *http.Request, _ Caller) {
		w.Header().Set("Content-Security-Policy", h.csp(name))
		w.Header().Set("Referrer-Policy", "no-referrer")
		http.ServeFileFS(w, r, h.site, name)
	}
}

func (h *Hub) as(f func(http.ResponseWriter, *http.Request, Caller)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user := strings.TrimSpace(r.Header.Get("Remote-User"))
		if user == "" {
			fail(w, http.StatusUnauthorized, errors.New("no login"))
			return
		}
		c := Caller{User: user, Groups: r.Header.Get("Remote-Groups")}
		if k, err := r.Cookie(cookieName); err == nil {
			if s, ok := h.store.ByToken(user, k.Value); ok {
				c.Screen = &s
			}
		}
		f(w, r, c)
	}
}

func (h *Hub) asScreen(f func(http.ResponseWriter, *http.Request, Caller)) http.HandlerFunc {
	return h.as(func(w http.ResponseWriter, r *http.Request, c Caller) {
		if c.Screen == nil {
			fail(w, http.StatusUnauthorized, errors.New("this browser is not a screen"))
			return
		}
		f(w, r, c)
	})
}

func (h *Hub) asRemote(f func(http.ResponseWriter, *http.Request, Caller)) http.HandlerFunc {
	return h.as(func(w http.ResponseWriter, r *http.Request, c Caller) {
		if !c.remote() {
			fail(w, http.StatusForbidden, errors.New("this screen is not a remote"))
			return
		}
		f(w, r, c)
	})
}

func (h *Hub) setCookie(w http.ResponseWriter, token string) {
	http.SetCookie(w, &http.Cookie{Name: cookieName, Value: token, Path: "/", HttpOnly: true, Secure: h.secure, SameSite: http.SameSiteStrictMode, MaxAge: 400 * 24 * 3600})
}

func (h *Hub) clearCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{Name: cookieName, Value: "", Path: "/", HttpOnly: true, Secure: h.secure, SameSite: http.SameSiteStrictMode, MaxAge: -1})
}

func screenName(s string) (string, error) {
	s = strings.TrimSpace(s)
	if s == "" || len(s) > 64 || strings.ContainsFunc(s, func(r rune) bool { return r < 0x20 || r == 0x7f }) {
		return "", errors.New("a name is 1 to 64 printable characters")
	}
	return s, nil
}

type screenView struct {
	ID       string     `json:"id"`
	Name     string     `json:"name"`
	Kind     string     `json:"kind"`
	Remote   bool       `json:"remote"`
	Here     bool       `json:"here,omitempty"`
	LastSeen time.Time  `json:"lastSeen"`
	State    *PlayState `json:"state,omitempty"`
}

func (h *Hub) view(s Screen, c Caller) screenView {
	v := screenView{ID: s.ID, Name: s.Name, Kind: s.Kind, Remote: s.Remote, LastSeen: s.LastSeen, Here: c.Screen != nil && c.Screen.ID == s.ID}
	h.stateMu.Lock()
	if st, ok := h.state[s.ID]; ok && time.Since(st.At) < 2*time.Minute {
		v.State = &st
	}
	h.stateMu.Unlock()
	return v
}

func (h *Hub) me(w http.ResponseWriter, r *http.Request, c Caller) {
	out := map[string]any{"user": c.User}
	if c.Screen != nil {
		out["screen"] = h.view(*c.Screen, c)
	}
	reply(w, http.StatusOK, out)
}

func (h *Hub) register(w http.ResponseWriter, r *http.Request, c Caller) {
	var body struct {
		Name   string `json:"name"`
		Remote bool   `json:"remote"`
	}
	if !decode(w, r, &body) {
		return
	}
	name, err := screenName(body.Name)
	if err != nil {
		fail(w, http.StatusBadRequest, err)
		return
	}
	if c.Screen != nil {
		s, err := h.store.Update(c.User, c.Screen.ID, func(s *Screen) { s.Name, s.Remote = name, body.Remote })
		if err != nil {
			fail(w, http.StatusNotFound, err)
			return
		}
		reply(w, http.StatusOK, h.view(s, Caller{Screen: &s}))
		return
	}
	token, s, err := h.store.Add(Screen{Name: name, Kind: "browser", Owner: c.User, Remote: body.Remote})
	if err != nil {
		fail(w, http.StatusInternalServerError, err)
		return
	}
	h.setCookie(w, token)
	log.Printf("%s registered screen %s (remote %v)", c.User, s.ID, s.Remote)
	reply(w, http.StatusOK, h.view(s, Caller{Screen: &s}))
}

func (h *Hub) drop(owner, id string) error {
	if err := h.store.Remove(owner, id); err != nil {
		return err
	}
	h.queues.Push(id, Event{Kind: "forgotten"})
	h.stateMu.Lock()
	delete(h.state, id)
	h.stateMu.Unlock()
	return nil
}

func (h *Hub) forget(w http.ResponseWriter, r *http.Request, c Caller) {
	h.clearCookie(w)
	if c.Screen != nil {
		h.drop(c.User, c.Screen.ID)
		h.queues.Drop(c.Screen.ID)
	}
	reply(w, http.StatusOK, map[string]bool{"ok": true})
}

func (h *Hub) next(w http.ResponseWriter, r *http.Request, c Caller) {
	e, ok := h.queues.Next(r.Context(), c.Screen.ID, h.wait)
	if !ok {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	reply(w, http.StatusOK, e)
}

func (h *Hub) report(w http.ResponseWriter, r *http.Request, c Caller) {
	var st PlayState
	if !decode(w, r, &st) {
		return
	}
	if len(st.Title) > 256 {
		st.Title = st.Title[:256]
	}
	st.At = time.Now()
	h.stateMu.Lock()
	h.state[c.Screen.ID] = st
	h.stateMu.Unlock()
	w.WriteHeader(http.StatusNoContent)
}

func (h *Hub) screens(w http.ResponseWriter, r *http.Request, c Caller) {
	out := []screenView{}
	for _, s := range h.store.Owned(c.User) {
		out = append(out, h.view(s, c))
	}
	adb := []map[string]any{}
	if c.remote() && h.mayADB(c) {
		names := make([]string, 0, len(h.adb))
		for n := range h.adb {
			names = append(names, n)
		}
		sort.Strings(names)
		for _, n := range names {
			st := h.adb[n].status()
			st["name"] = n
			adb = append(adb, st)
		}
	}
	reply(w, http.StatusOK, map[string]any{"screens": out, "remotes": adb})
}

func (h *Hub) target(w http.ResponseWriter, r *http.Request, c Caller) (Screen, bool) {
	s, ok := h.store.Get(c.User, r.PathValue("id"))
	if !ok {
		fail(w, http.StatusNotFound, errNoScreen)
	}
	return s, ok
}

func (h *Hub) cast(w http.ResponseWriter, r *http.Request, c Caller) {
	var body struct {
		URL string `json:"url"`
	}
	if !decode(w, r, &body) {
		return
	}
	if h.files == nil {
		fail(w, http.StatusServiceUnavailable, errors.New("this hub has no files site"))
		return
	}
	u, err := filesURL(h.files, body.URL)
	if err != nil {
		fail(w, http.StatusBadRequest, err)
		return
	}
	s, ok := h.target(w, r, c)
	if !ok {
		return
	}
	title := play.Title(u)
	if t, err := url.PathUnescape(title); err == nil {
		title = t
	}
	h.queues.Push(s.ID, Event{Kind: "cast", URL: u, Title: title, Type: play.Type(u)})
	reply(w, http.StatusOK, map[string]string{"url": u, "title": title})
}

var controls = map[string]bool{"toggle": true, "skip": true, "seekTo": true, "seekFraction": true, "chapter": true, "stop": true}

func (h *Hub) control(w http.ResponseWriter, r *http.Request, c Caller) {
	var body struct {
		Action string  `json:"action"`
		Value  float64 `json:"value"`
	}
	if !decode(w, r, &body) {
		return
	}
	if !controls[body.Action] {
		fail(w, http.StatusBadRequest, fmt.Errorf("unknown control %q", body.Action))
		return
	}
	s, ok := h.target(w, r, c)
	if !ok {
		return
	}
	h.queues.Push(s.ID, Event{Kind: "control", Action: body.Action, Value: body.Value})
	reply(w, http.StatusOK, map[string]bool{"ok": true})
}

func (h *Hub) rename(w http.ResponseWriter, r *http.Request, c Caller) {
	var body struct {
		Name string `json:"name"`
	}
	if !decode(w, r, &body) {
		return
	}
	name, err := screenName(body.Name)
	if err != nil {
		fail(w, http.StatusBadRequest, err)
		return
	}
	s, err := h.store.Update(c.User, r.PathValue("id"), func(s *Screen) { s.Name = name })
	if err != nil {
		fail(w, http.StatusNotFound, err)
		return
	}
	reply(w, http.StatusOK, h.view(s, c))
}

func (h *Hub) remove(w http.ResponseWriter, r *http.Request, c Caller) {
	id := r.PathValue("id")
	if err := h.drop(c.User, id); err != nil {
		fail(w, http.StatusNotFound, err)
		return
	}
	log.Printf("%s removed screen %s", c.User, id)
	reply(w, http.StatusOK, map[string]bool{"ok": true})
}

func inGroup(groups, want string) bool {
	for _, g := range strings.Split(groups, ",") {
		if strings.TrimSpace(g) == want {
			return true
		}
	}
	return false
}

func (h *Hub) mayADB(c Caller) bool {
	return h.adbGroup == "" || inGroup(c.Groups, h.adbGroup)
}

func (h *Hub) adbFor(w http.ResponseWriter, r *http.Request, c Caller) (*link, bool) {
	if !h.mayADB(c) {
		fail(w, http.StatusForbidden, errors.New("adb remotes need the "+h.adbGroup+" group"))
		return nil, false
	}
	l, ok := h.adb[r.PathValue("name")]
	if !ok {
		fail(w, http.StatusNotFound, errors.New("no such remote"))
	}
	return l, ok
}

func (h *Hub) adbPress(w http.ResponseWriter, r *http.Request, c Caller) {
	l, ok := h.adbFor(w, r, c)
	if !ok {
		return
	}
	var body struct {
		Action string `json:"action"`
		Times  int    `json:"times"`
	}
	if !decode(w, r, &body) {
		return
	}
	st, ok := actions[body.Action]
	if !ok {
		fail(w, http.StatusBadRequest, fmt.Errorf("unknown action %q", body.Action))
		return
	}
	n := min(max(body.Times, 1), 50)
	err := l.do(func(t tv.TV) error {
		for i := 0; i < n; i++ {
			if err := st.run(t); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		fail(w, http.StatusBadGateway, err)
		return
	}
	reply(w, http.StatusOK, map[string]bool{"ok": true})
}

func (h *Hub) adbText(w http.ResponseWriter, r *http.Request, c Caller) {
	l, ok := h.adbFor(w, r, c)
	if !ok {
		return
	}
	var body struct {
		Text string `json:"text"`
	}
	if !decode(w, r, &body) {
		return
	}
	if body.Text == "" {
		fail(w, http.StatusBadRequest, errors.New("no text"))
		return
	}
	if _, err := tv.InputTextArg(body.Text); err != nil {
		fail(w, http.StatusBadRequest, err)
		return
	}
	if err := l.do(func(t tv.TV) error { return t.Text(body.Text) }); err != nil {
		fail(w, http.StatusBadGateway, err)
		return
	}
	reply(w, http.StatusOK, map[string]bool{"ok": true})
}
