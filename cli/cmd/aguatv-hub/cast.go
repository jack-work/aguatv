package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/jack-work/aguatv/cli/internal/play"
)

type Caster struct {
	User   string
	Groups string
}

type Ticket struct {
	Caster string `json:"c"`
	Key    string `json:"k"`
	Device string `json:"d"`
	Expiry int64  `json:"e"`
}

type Cast struct {
	ID      string
	Device  string
	Ticket  string
	Key     string
	Title   string
	Type    string
	Expires time.Time
}

type Source interface {
	Open(ctx context.Context, c *Cast, h http.Header) (*http.Response, error)
}

type Casts struct {
	mu   sync.Mutex
	byID map[string]*Cast
	now  func() time.Time
}

func NewCasts() *Casts {
	return &Casts{byID: map[string]*Cast{}, now: time.Now}
}

var (
	errNoCast    = errors.New("no such cast")
	errBadTicket = errors.New("that ticket is not for this person, this screen, or now")
)

func readTicket(ticket string) (Ticket, error) {
	payload, _, ok := strings.Cut(ticket, ".")
	if !ok || payload == "" {
		return Ticket{}, errBadTicket
	}
	b, err := base64.RawURLEncoding.DecodeString(payload)
	if err != nil {
		return Ticket{}, errBadTicket
	}
	var t Ticket
	if err := json.Unmarshal(b, &t); err != nil || t.Key == "" {
		return Ticket{}, errBadTicket
	}
	return t, nil
}

func (c *Casts) New(device string, by Caster, ticket string) (*Cast, error) {
	t, err := readTicket(ticket)
	if err != nil {
		return nil, err
	}
	now := c.now()
	if t.Device != device || t.Caster != by.User || t.Expiry <= now.Unix() {
		return nil, errBadTicket
	}
	cast := &Cast{ID: randomToken(18), Device: device, Ticket: ticket, Key: t.Key,
		Title: play.Title(t.Key), Type: play.Type(t.Key), Expires: time.Unix(t.Expiry, 0)}
	c.mu.Lock()
	defer c.mu.Unlock()
	for id, old := range c.byID {
		if now.After(old.Expires) {
			delete(c.byID, id)
		}
	}
	c.byID[cast.ID] = cast
	return cast, nil
}

func (c *Casts) For(device, id string) (*Cast, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	cast, ok := c.byID[id]
	if !ok || cast.Device != device || c.now().After(cast.Expires) {
		return nil, errNoCast
	}
	return cast, nil
}

func (c *Casts) DropDevice(device string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for id, cast := range c.byID {
		if cast.Device == device {
			delete(c.byID, id)
		}
	}
}

type TicketStream struct {
	URL    string
	Client *http.Client
}

func NewTicketStream(target string) (*TicketStream, error) {
	if sock, ok := strings.CutPrefix(target, "unix:"); ok {
		path, rest, _ := strings.Cut(sock, ":")
		if path == "" || rest == "" {
			return nil, fmt.Errorf("stream %q wants unix:<socket>:<path>", target)
		}
		return &TicketStream{URL: "http://stream" + rest, Client: &http.Client{Transport: &http.Transport{
			DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
				var d net.Dialer
				return d.DialContext(ctx, "unix", path)
			},
		}}}, nil
	}
	if !strings.HasPrefix(target, "http://") && !strings.HasPrefix(target, "https://") {
		return nil, fmt.Errorf("stream %q is neither unix:<socket>:<path> nor a URL", target)
	}
	return &TicketStream{URL: target, Client: &http.Client{}}, nil
}

var forwardedRequestHeaders = []string{"Range", "If-Range", "If-None-Match", "If-Modified-Since"}

func (s *TicketStream) Open(ctx context.Context, c *Cast, in http.Header) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.URL, nil)
	if err != nil {
		return nil, err
	}
	for _, h := range forwardedRequestHeaders {
		if v := in.Get(h); v != "" {
			req.Header.Set(h, v)
		}
	}
	req.Header.Set("X-Cast-Ticket", c.Ticket)
	req.Header.Set("X-Cast-Device", c.Device)
	return s.Client.Do(req)
}

var forwardedResponseHeaders = []string{"Content-Type", "Content-Length", "Content-Range", "Accept-Ranges", "ETag", "Last-Modified"}
