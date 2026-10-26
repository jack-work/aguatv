package main

import (
	"crypto/rand"
	"crypto/subtle"
	"errors"
	"strings"
	"sync"
	"time"
)

const codeAlphabet = "23456789ABCDEFGHJKLMNPQRSTUVWXYZ"

type Pending struct {
	Code      string
	ClaimHash string
	Kind      string
	Proposed  string
	Addr      string
	Asked     time.Time
	Session   bool
	token     string
	device    Device
	approved  bool
	claimed   bool
}

type Linker struct {
	mu      sync.Mutex
	pending map[string]*Pending
	ttl     time.Duration
	now     func() time.Time
	starts  *limiter
	tries   *limiter
}

func NewLinker() *Linker {
	return &Linker{
		pending: map[string]*Pending{},
		ttl:     10 * time.Minute,
		now:     time.Now,
		starts:  newLimiter(5, time.Minute),
		tries:   newLimiter(10, 10*time.Minute),
	}
}

func newCode() string {
	b := make([]byte, 7)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	out := make([]byte, 7)
	for i, v := range b {
		out[i] = codeAlphabet[int(v)%len(codeAlphabet)]
	}
	return string(out)
}

func normalizeCode(c string) string {
	c = strings.ToUpper(strings.TrimSpace(c))
	c = strings.NewReplacer("-", "", " ", "").Replace(c)
	return c
}

func FormatCode(c string) string {
	if len(c) != 7 {
		return c
	}
	return c[:3] + "-" + c[3:]
}

var (
	errRateLimited = errors.New("too many attempts, try again later")
	errBadCode     = errors.New("that code is not waiting to be linked")
)

func (l *Linker) sweepLocked() {
	now := l.now()
	for k, p := range l.pending {
		if now.Sub(p.Asked) > l.ttl || p.claimed {
			delete(l.pending, k)
		}
	}
}

func (l *Linker) Start(kind, proposed, addr string, session bool) (code, claim string, err error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if !l.starts.allow(addr) || !l.starts.allow("*") {
		return "", "", errRateLimited
	}
	l.sweepLocked()
	if len(l.pending) > 64 {
		return "", "", errRateLimited
	}
	code = newCode()
	for l.pending[code] != nil {
		code = newCode()
	}
	claim = randomToken(32)
	l.pending[code] = &Pending{Code: code, ClaimHash: hashToken(claim), Kind: kind, Proposed: proposed, Addr: addr, Asked: l.now(), Session: session}
	return code, claim, nil
}

func (l *Linker) Peek(owner, code string) (Pending, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if !l.tries.allow(owner) {
		return Pending{}, errRateLimited
	}
	l.sweepLocked()
	p, ok := l.pending[normalizeCode(code)]
	if !ok || p.approved {
		return Pending{}, errBadCode
	}
	return *p, nil
}

func (l *Linker) Approve(owner, code string, store *Store, name string) (Device, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if !l.tries.allow(owner) {
		return Device{}, errRateLimited
	}
	l.sweepLocked()
	p, ok := l.pending[normalizeCode(code)]
	if !ok || p.approved {
		return Device{}, errBadCode
	}
	if name == "" {
		name = p.Proposed
	}
	ttl := 90 * 24 * time.Hour
	if p.Session {
		ttl = 24 * time.Hour
	}
	d := Device{Name: name, Kind: p.Kind, Owner: owner, Session: p.Session, TTL: Duration{ttl}}
	token, dev, err := store.Add(d)
	if err != nil {
		return Device{}, err
	}
	p.token, p.device, p.approved = token, dev, true
	return dev, nil
}

func (l *Linker) Claim(code, claim string) (token string, d Device, waiting bool, err error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.sweepLocked()
	p, ok := l.pending[normalizeCode(code)]
	if !ok || subtle.ConstantTimeCompare([]byte(p.ClaimHash), []byte(hashToken(claim))) != 1 {
		return "", Device{}, false, errBadCode
	}
	if !p.approved {
		return "", Device{}, true, nil
	}
	p.claimed = true
	token, d = p.token, p.device
	p.token = ""
	return token, d, false, nil
}

type limiter struct {
	n    int
	per  time.Duration
	hits map[string][]time.Time
	now  func() time.Time
}

func newLimiter(n int, per time.Duration) *limiter {
	return &limiter{n: n, per: per, hits: map[string][]time.Time{}, now: time.Now}
}

func (r *limiter) allow(key string) bool {
	now := r.now()
	kept := r.hits[key][:0]
	for _, t := range r.hits[key] {
		if now.Sub(t) < r.per {
			kept = append(kept, t)
		}
	}
	if len(kept) >= r.n {
		r.hits[key] = kept
		return false
	}
	r.hits[key] = append(kept, now)
	return true
}
