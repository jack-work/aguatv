package main

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

type Screen struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Kind      string    `json:"kind"`
	Owner     string    `json:"owner"`
	Remote    bool      `json:"remote"`
	TokenHash string    `json:"tokenHash"`
	Created   time.Time `json:"created"`
	LastSeen  time.Time `json:"lastSeen"`
}

type Store struct {
	mu      sync.Mutex
	path    string
	screens map[string]*Screen
	idle    time.Duration
	now     func() time.Time
}

func OpenStore(path string) (*Store, error) {
	s := &Store{path: path, screens: map[string]*Screen{}, idle: 90 * 24 * time.Hour, now: time.Now}
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return s, nil
	}
	if err != nil {
		return nil, err
	}
	var list []*Screen
	if err := json.Unmarshal(b, &list); err != nil {
		return nil, err
	}
	for _, d := range list {
		s.screens[d.ID] = d
	}
	return s, nil
}

func (s *Store) expired(d *Screen) bool {
	return s.now().Sub(d.LastSeen) > s.idle
}

func (s *Store) saveLocked() error {
	if s.path == "" {
		return nil
	}
	list := make([]*Screen, 0, len(s.screens))
	for _, d := range s.screens {
		list = append(list, d)
	}
	sort.Slice(list, func(i, j int) bool { return list[i].ID < list[j].ID })
	b, err := json.MarshalIndent(list, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(s.path), ".screens-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		return err
	}
	if _, err := tmp.Write(append(b, '\n')); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), s.path)
}

func randomToken(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return base64.RawURLEncoding.EncodeToString(b)
}

func hashToken(t string) string {
	h := sha256.Sum256([]byte(t))
	return hex.EncodeToString(h[:])
}

func (s *Store) Add(d Screen) (string, Screen, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	token := randomToken(32)
	now := s.now()
	d.ID = randomToken(9)
	d.TokenHash = hashToken(token)
	d.Created, d.LastSeen = now, now
	s.screens[d.ID] = &d
	return token, d, s.saveLocked()
}

func (s *Store) ByToken(owner, token string) (Screen, bool) {
	if token == "" {
		return Screen{}, false
	}
	want := hashToken(token)
	s.mu.Lock()
	defer s.mu.Unlock()
	for id, d := range s.screens {
		if subtle.ConstantTimeCompare([]byte(d.TokenHash), []byte(want)) != 1 {
			continue
		}
		if d.Owner != owner {
			return Screen{}, false
		}
		if s.expired(d) {
			delete(s.screens, id)
			s.saveLocked()
			return Screen{}, false
		}
		now := s.now()
		stale := now.Sub(d.LastSeen) > time.Hour
		d.LastSeen = now
		if stale {
			s.saveLocked()
		}
		return *d, true
	}
	return Screen{}, false
}

func (s *Store) Owned(owner string) []Screen {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []Screen
	for _, d := range s.screens {
		if d.Owner == owner && !s.expired(d) {
			out = append(out, *d)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

func (s *Store) Get(owner, id string) (Screen, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	d, ok := s.screens[id]
	if !ok || d.Owner != owner || s.expired(d) {
		return Screen{}, false
	}
	return *d, true
}

func (s *Store) Update(owner, id string, f func(*Screen)) (Screen, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	d, ok := s.screens[id]
	if !ok || d.Owner != owner {
		return Screen{}, errNoScreen
	}
	f(d)
	return *d, s.saveLocked()
}

func (s *Store) Remove(owner, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	d, ok := s.screens[id]
	if !ok || d.Owner != owner {
		return errNoScreen
	}
	delete(s.screens, id)
	return s.saveLocked()
}

var errNoScreen = errors.New("no such screen")
