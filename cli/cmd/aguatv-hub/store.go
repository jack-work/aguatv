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

type Device struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Kind      string    `json:"kind"`
	Owner     string    `json:"owner"`
	TokenHash string    `json:"tokenHash"`
	Session   bool      `json:"session"`
	Created   time.Time `json:"created"`
	LastSeen  time.Time `json:"lastSeen"`
	TTL       Duration  `json:"ttl"`
	ADB       string    `json:"adb,omitempty"`
}

type Duration struct{ time.Duration }

func (d Duration) MarshalJSON() ([]byte, error) { return json.Marshal(d.String()) }

func (d *Duration) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return err
	}
	v, err := time.ParseDuration(s)
	d.Duration = v
	return err
}

func (d Device) expired(now time.Time) bool {
	return d.TTL.Duration > 0 && now.Sub(d.LastSeen) > d.TTL.Duration
}

type Store struct {
	mu      sync.Mutex
	path    string
	devices map[string]*Device
	now     func() time.Time
}

func OpenStore(path string) (*Store, error) {
	s := &Store{path: path, devices: map[string]*Device{}, now: time.Now}
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return s, nil
	}
	if err != nil {
		return nil, err
	}
	var list []*Device
	if err := json.Unmarshal(b, &list); err != nil {
		return nil, err
	}
	for _, d := range list {
		s.devices[d.ID] = d
	}
	return s, nil
}

func (s *Store) saveLocked() error {
	if s.path == "" {
		return nil
	}
	list := make([]*Device, 0, len(s.devices))
	for _, d := range s.devices {
		if !d.Session {
			list = append(list, d)
		}
	}
	sort.Slice(list, func(i, j int) bool { return list[i].ID < list[j].ID })
	b, err := json.MarshalIndent(list, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(s.path), ".devices-*")
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

func (s *Store) Add(d Device) (string, Device, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	token := randomToken(32)
	now := s.now()
	d.TokenHash = hashToken(token)
	d.Created, d.LastSeen = now, now
	if d.ID == "" {
		d.ID = randomToken(9)
	}
	s.devices[d.ID] = &d
	return token, d, s.saveLocked()
}

func (s *Store) ByToken(token string) (Device, bool) {
	if token == "" {
		return Device{}, false
	}
	want := hashToken(token)
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.now()
	for id, d := range s.devices {
		if subtle.ConstantTimeCompare([]byte(d.TokenHash), []byte(want)) != 1 {
			continue
		}
		if d.expired(now) {
			delete(s.devices, id)
			s.saveLocked()
			return Device{}, false
		}
		if now.Sub(d.LastSeen) > time.Hour {
			d.LastSeen = now
			s.saveLocked()
		} else {
			d.LastSeen = now
		}
		return *d, true
	}
	return Device{}, false
}

func (s *Store) Owned(owner string) []Device {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.now()
	var out []Device
	for _, d := range s.devices {
		if d.Owner == owner && !d.expired(now) {
			out = append(out, *d)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

func (s *Store) Get(owner, id string) (Device, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	d, ok := s.devices[id]
	if !ok || d.Owner != owner || d.expired(s.now()) {
		return Device{}, false
	}
	return *d, true
}

func (s *Store) Rename(owner, id, name string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	d, ok := s.devices[id]
	if !ok || d.Owner != owner {
		return errNoDevice
	}
	d.Name = name
	return s.saveLocked()
}

func (s *Store) Remove(owner, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	d, ok := s.devices[id]
	if !ok || d.Owner != owner {
		return errNoDevice
	}
	delete(s.devices, id)
	return s.saveLocked()
}

var errNoDevice = errors.New("no such device")
