package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/jack-work/aguatv/cli/internal/tv"
	"rsc.io/qr"
)

const cookieName = "aguatv_device"

type PlayState struct {
	Title    string    `json:"title"`
	Position float64   `json:"position"`
	Duration float64   `json:"duration"`
	Playing  bool      `json:"playing"`
	At       time.Time `json:"at"`
}

type Hub struct {
	store    *Store
	linker   *Linker
	queues   *Queues
	casts    *Casts
	source   Source
	adb      map[string]*link
	adbGroup string
	site     http.Handler
	public   string
	secure   bool
	wait     time.Duration
	stateMu  sync.Mutex
	state    map[string]PlayState
	mux      *http.ServeMux
}

func NewHub(store *Store, casts *Casts, source Source, adb map[string]*link, site http.Handler, public string, secure bool) *Hub {
	h := &Hub{
		store: store, linker: NewLinker(), queues: NewQueues(), casts: casts, source: source,
		adb: adb, site: site, public: strings.TrimSuffix(public, "/"), secure: secure,
		wait: 25 * time.Second, state: map[string]PlayState{}, mux: http.NewServeMux(),
	}
	m := h.mux
	m.HandleFunc("GET /receiver", h.page("/receiver.html"))
	m.HandleFunc("POST /device/start", h.deviceStart)
	m.HandleFunc("POST /device/claim", h.deviceClaim)
	m.HandleFunc("GET /device/qr", h.deviceQR)
	m.HandleFunc("GET /device/whoami", h.asDevice(h.deviceWhoami))
	m.HandleFunc("GET /device/next", h.asDevice(h.deviceNext))
	m.HandleFunc("POST /device/state", h.asDevice(h.deviceState))
	m.HandleFunc("GET /stream", h.asDevice(h.stream))

	m.HandleFunc("GET /tvlogin", h.asPerson(h.pageFor("/tvlogin.html")))
	m.HandleFunc("GET /api/me", h.asPerson(h.me))
	m.HandleFunc("GET /api/devices", h.asPerson(h.devices))
	m.HandleFunc("GET /api/link", h.asPerson(h.peekLink))
	m.HandleFunc("POST /api/link", h.asPerson(h.approveLink))
	m.HandleFunc("POST /api/devices/{id}/rename", h.asPerson(h.rename))
	m.HandleFunc("DELETE /api/devices/{id}", h.asPerson(h.unlink))
	m.HandleFunc("POST /api/devices/{id}/cast", h.asPerson(h.cast))
	m.HandleFunc("POST /api/devices/{id}/control", h.asPerson(h.control))
	m.HandleFunc("POST /api/adb/{name}/press", h.asPerson(h.adbPress))
	m.HandleFunc("POST /api/adb/{name}/text", h.asPerson(h.adbText))
	m.HandleFunc("GET /{$}", h.asPerson(h.pageFor("/index.html")))
	m.HandleFunc("GET /", h.asPerson(func(w http.ResponseWriter, r *http.Request, _ Caster) { h.site.ServeHTTP(w, r) }))
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

func (h *Hub) page(name string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		r2 := r.Clone(r.Context())
		r2.URL.Path = name
		h.site.ServeHTTP(w, r2)
	}
}

func (h *Hub) pageFor(name string) func(http.ResponseWriter, *http.Request, Caster) {
	p := h.page(name)
	return func(w http.ResponseWriter, r *http.Request, _ Caster) { p(w, r) }
}

func (h *Hub) asPerson(f func(http.ResponseWriter, *http.Request, Caster)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user := strings.TrimSpace(r.Header.Get("Remote-User"))
		if user == "" {
			fail(w, http.StatusUnauthorized, errors.New("no login"))
			return
		}
		f(w, r, Caster{User: user, Groups: r.Header.Get("Remote-Groups")})
	}
}

func (h *Hub) asDevice(f func(http.ResponseWriter, *http.Request, Device)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		c, err := r.Cookie(cookieName)
		if err != nil {
			fail(w, http.StatusUnauthorized, errors.New("not linked"))
			return
		}
		d, ok := h.store.ByToken(c.Value)
		if !ok {
			h.clearCookie(w)
			fail(w, http.StatusUnauthorized, errors.New("not linked"))
			return
		}
		f(w, r, d)
	}
}

func (h *Hub) setCookie(w http.ResponseWriter, token string, d Device) {
	c := &http.Cookie{Name: cookieName, Value: token, Path: "/", HttpOnly: true, Secure: h.secure, SameSite: http.SameSiteStrictMode}
	if !d.Session {
		c.MaxAge = int(d.TTL.Seconds())
	}
	http.SetCookie(w, c)
}

func (h *Hub) clearCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{Name: cookieName, Value: "", Path: "/", HttpOnly: true, Secure: h.secure, SameSite: http.SameSiteStrictMode, MaxAge: -1})
}

func clientAddr(r *http.Request) string {
	if v := r.Header.Get("X-Forwarded-For"); v != "" {
		return strings.TrimSpace(strings.Split(v, ",")[0])
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

func (h *Hub) linkURL(code string) string {
	return h.public + "/tvlogin?code=" + FormatCode(code)
}

func (h *Hub) deviceStart(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Kind    string `json:"kind"`
		Name    string `json:"name"`
		Session bool   `json:"session"`
	}
	if !decode(w, r, &body) {
		return
	}
	kind := "browser"
	if body.Kind == "tv" {
		kind = "tv"
	}
	name := strings.TrimSpace(body.Name)
	if len(name) > 64 {
		name = name[:64]
	}
	if name == "" {
		name = map[string]string{"tv": "TV", "browser": "Browser"}[kind]
	}
	code, claim, err := h.linker.Start(kind, name, clientAddr(r), body.Session)
	if err != nil {
		fail(w, http.StatusTooManyRequests, err)
		return
	}
	reply(w, http.StatusOK, map[string]any{
		"code": FormatCode(code), "claim": claim, "url": h.public + "/tvlogin", "link": h.linkURL(code), "expires": int(h.linker.ttl.Seconds()),
	})
}

func (h *Hub) deviceClaim(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Code  string `json:"code"`
		Claim string `json:"claim"`
	}
	if !decode(w, r, &body) {
		return
	}
	token, d, waiting, err := h.linker.Claim(body.Code, body.Claim)
	switch {
	case err != nil:
		fail(w, http.StatusGone, err)
	case waiting:
		reply(w, http.StatusAccepted, map[string]bool{"waiting": true})
	default:
		h.setCookie(w, token, d)
		reply(w, http.StatusOK, map[string]string{"id": d.ID, "name": d.Name})
	}
}

func (h *Hub) deviceQR(w http.ResponseWriter, r *http.Request) {
	code := normalizeCode(r.URL.Query().Get("code"))
	h.linker.mu.Lock()
	_, ok := h.linker.pending[code]
	h.linker.mu.Unlock()
	if !ok {
		fail(w, http.StatusNotFound, errBadCode)
		return
	}
	c, err := qr.Encode(h.linkURL(code), qr.M)
	if err != nil {
		fail(w, http.StatusInternalServerError, err)
		return
	}
	c.Scale = 8
	w.Header().Set("Content-Type", "image/png")
	w.Write(c.PNG())
}

func (h *Hub) deviceWhoami(w http.ResponseWriter, r *http.Request, d Device) {
	reply(w, http.StatusOK, map[string]string{"id": d.ID, "name": d.Name, "kind": d.Kind})
}

func (h *Hub) deviceNext(w http.ResponseWriter, r *http.Request, d Device) {
	e, ok := h.queues.Next(r.Context(), d.ID, h.wait)
	if !ok {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	reply(w, http.StatusOK, e)
}

func (h *Hub) deviceState(w http.ResponseWriter, r *http.Request, d Device) {
	var st PlayState
	if !decode(w, r, &st) {
		return
	}
	st.At = time.Now()
	h.stateMu.Lock()
	h.state[d.ID] = st
	h.stateMu.Unlock()
	w.WriteHeader(http.StatusNoContent)
}

func (h *Hub) stream(w http.ResponseWriter, r *http.Request, d Device) {
	cast, err := h.casts.For(r.Context(), d.ID, r.URL.Query().Get("cast"))
	if err != nil {
		code := http.StatusNotFound
		if errors.Is(err, errNotAllowed) {
			code = http.StatusForbidden
		} else if !errors.Is(err, errNoCast) {
			code = http.StatusBadGateway
		}
		fail(w, code, err)
		return
	}
	if ks, ok := h.source.(keyServer); ok {
		ks.ServeKey(w, r, cast.Key)
		return
	}
	resp, err := h.source.Open(r.Context(), cast.Key, r.Header)
	if err != nil {
		fail(w, http.StatusBadGateway, err)
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 && resp.StatusCode != http.StatusRequestedRangeNotSatisfiable {
		fail(w, http.StatusBadGateway, fmt.Errorf("storage answered %d", resp.StatusCode))
		return
	}
	for _, k := range forwardedResponseHeaders {
		if v := resp.Header.Get(k); v != "" {
			w.Header().Set(k, v)
		}
	}
	w.WriteHeader(resp.StatusCode)
	if r.Method != http.MethodHead {
		io.Copy(w, resp.Body)
	}
}

func (h *Hub) me(w http.ResponseWriter, r *http.Request, c Caster) {
	reply(w, http.StatusOK, map[string]string{"user": c.User})
}

type deviceView struct {
	ID       string     `json:"id"`
	Name     string     `json:"name"`
	Kind     string     `json:"kind"`
	Session  bool       `json:"session"`
	LastSeen time.Time  `json:"lastSeen"`
	State    *PlayState `json:"state,omitempty"`
}

func (h *Hub) devices(w http.ResponseWriter, r *http.Request, c Caster) {
	var out []deviceView
	h.stateMu.Lock()
	for _, d := range h.store.Owned(c.User) {
		v := deviceView{ID: d.ID, Name: d.Name, Kind: d.Kind, Session: d.Session, LastSeen: d.LastSeen}
		if st, ok := h.state[d.ID]; ok && time.Since(st.At) < 2*time.Minute {
			v.State = &st
		}
		out = append(out, v)
	}
	h.stateMu.Unlock()
	var adb []map[string]any
	names := make([]string, 0, len(h.adb))
	for n := range h.adb {
		names = append(names, n)
	}
	sort.Strings(names)
	if !h.mayADB(c) {
		names = nil
	}
	for _, n := range names {
		st := h.adb[n].status()
		st["name"] = n
		adb = append(adb, st)
	}
	reply(w, http.StatusOK, map[string]any{"devices": out, "remotes": adb})
}

func (h *Hub) peekLink(w http.ResponseWriter, r *http.Request, c Caster) {
	p, err := h.linker.Peek(c.User, r.URL.Query().Get("code"))
	if err != nil {
		code := http.StatusNotFound
		if errors.Is(err, errRateLimited) {
			code = http.StatusTooManyRequests
		}
		fail(w, code, err)
		return
	}
	reply(w, http.StatusOK, map[string]any{"code": FormatCode(p.Code), "kind": p.Kind, "name": p.Proposed, "asked": p.Asked, "from": p.Addr, "session": p.Session})
}

func (h *Hub) approveLink(w http.ResponseWriter, r *http.Request, c Caster) {
	var body struct {
		Code string `json:"code"`
		Name string `json:"name"`
	}
	if !decode(w, r, &body) {
		return
	}
	name := strings.TrimSpace(body.Name)
	if len(name) > 64 {
		name = name[:64]
	}
	d, err := h.linker.Approve(c.User, body.Code, h.store, name)
	if err != nil {
		code := http.StatusNotFound
		if errors.Is(err, errRateLimited) {
			code = http.StatusTooManyRequests
		}
		fail(w, code, err)
		return
	}
	log.Printf("%s linked device %s (%s)", c.User, d.ID, d.Kind)
	reply(w, http.StatusOK, map[string]string{"id": d.ID, "name": d.Name})
}

func (h *Hub) rename(w http.ResponseWriter, r *http.Request, c Caster) {
	var body struct {
		Name string `json:"name"`
	}
	if !decode(w, r, &body) {
		return
	}
	name := strings.TrimSpace(body.Name)
	if name == "" || len(name) > 64 {
		fail(w, http.StatusBadRequest, errors.New("a name is 1 to 64 characters"))
		return
	}
	if err := h.store.Rename(c.User, r.PathValue("id"), name); err != nil {
		fail(w, http.StatusNotFound, err)
		return
	}
	reply(w, http.StatusOK, map[string]bool{"ok": true})
}

func (h *Hub) unlink(w http.ResponseWriter, r *http.Request, c Caster) {
	id := r.PathValue("id")
	if err := h.store.Remove(c.User, id); err != nil {
		fail(w, http.StatusNotFound, err)
		return
	}
	h.casts.DropDevice(id)
	h.queues.Drop(id)
	log.Printf("%s unlinked device %s", c.User, id)
	reply(w, http.StatusOK, map[string]bool{"ok": true})
}

func (h *Hub) cast(w http.ResponseWriter, r *http.Request, c Caster) {
	var body struct {
		Ref string `json:"ref"`
	}
	if !decode(w, r, &body) {
		return
	}
	d, ok := h.store.Get(c.User, r.PathValue("id"))
	if !ok {
		fail(w, http.StatusNotFound, errNoDevice)
		return
	}
	cast, err := h.casts.New(r.Context(), d.ID, c, body.Ref)
	if err != nil {
		code := http.StatusBadGateway
		switch {
		case errors.Is(err, errNotAllowed):
			code = http.StatusForbidden
		case strings.Contains(err.Error(), "not an object key"):
			code = http.StatusBadRequest
		}
		fail(w, code, err)
		return
	}
	h.queues.Push(d.ID, Event{Kind: "cast", Cast: cast.ID, Title: cast.Title, Type: cast.Type})
	reply(w, http.StatusOK, map[string]string{"cast": cast.ID, "title": cast.Title})
}

var controls = map[string]bool{"toggle": true, "skip": true, "seekTo": true, "seekFraction": true, "chapter": true, "stop": true}

func (h *Hub) control(w http.ResponseWriter, r *http.Request, c Caster) {
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
	d, ok := h.store.Get(c.User, r.PathValue("id"))
	if !ok {
		fail(w, http.StatusNotFound, errNoDevice)
		return
	}
	h.queues.Push(d.ID, Event{Kind: "control", Action: body.Action, Value: body.Value})
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

func (h *Hub) mayADB(c Caster) bool {
	return h.adbGroup == "" || inGroup(c.Groups, h.adbGroup)
}

func (h *Hub) adbFor(w http.ResponseWriter, r *http.Request) (*link, bool) {
	if !h.mayADB(Caster{Groups: r.Header.Get("Remote-Groups")}) {
		fail(w, http.StatusForbidden, errors.New("adb remotes need the "+h.adbGroup+" group"))
		return nil, false
	}
	l, ok := h.adb[r.PathValue("name")]
	if !ok {
		fail(w, http.StatusNotFound, errors.New("no such remote"))
	}
	return l, ok
}

func (h *Hub) adbPress(w http.ResponseWriter, r *http.Request, c Caster) {
	l, ok := h.adbFor(w, r)
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

func (h *Hub) adbText(w http.ResponseWriter, r *http.Request, c Caster) {
	l, ok := h.adbFor(w, r)
	if !ok {
		return
	}
	var body struct {
		Text string `json:"text"`
	}
	if !decode(w, r, &body) || body.Text == "" {
		if body.Text == "" {
			fail(w, http.StatusBadRequest, errors.New("no text"))
		}
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
