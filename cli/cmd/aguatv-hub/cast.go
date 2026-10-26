package main

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/jack-work/aguatv/cli/internal/play"
)

type Caster struct {
	User   string
	Groups string
}

type Cast struct {
	ID      string
	Device  string
	Key     string
	Title   string
	Type    string
	By      Caster
	Expires time.Time
	checked time.Time
}

type Authorizer interface {
	CanRead(ctx context.Context, by Caster, key string) (bool, error)
}

type Source interface {
	Open(ctx context.Context, key string, h http.Header) (*http.Response, error)
}

type Casts struct {
	mu    sync.Mutex
	byID  map[string]*Cast
	ttl   time.Duration
	now   func() time.Time
	authz Authorizer
}

func NewCasts(a Authorizer) *Casts {
	return &Casts{byID: map[string]*Cast{}, ttl: 12 * time.Hour, now: time.Now, authz: a}
}

var (
	errNotAllowed = errors.New("you may not read that")
	errNoCast     = errors.New("no such cast")
)

func normalizeKey(ref string) (string, error) {
	k := strings.TrimSpace(ref)
	if u, err := url.Parse(k); err == nil && u.Scheme != "" {
		k = u.Path
	}
	k = strings.TrimPrefix(k, "/")
	if k == "" || strings.HasSuffix(k, "/") || path.Clean("/"+k) != "/"+k {
		return "", fmt.Errorf("%q is not an object key", ref)
	}
	return k, nil
}

func (c *Casts) New(ctx context.Context, device string, by Caster, ref string) (*Cast, error) {
	key, err := normalizeKey(ref)
	if err != nil {
		return nil, err
	}
	ok, err := c.authz.CanRead(ctx, by, key)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, errNotAllowed
	}
	now := c.now()
	cast := &Cast{ID: randomToken(18), Device: device, Key: key, Title: play.Title(key), Type: play.Type(key), By: by, Expires: now.Add(c.ttl), checked: now}
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

func (c *Casts) For(ctx context.Context, device, id string) (*Cast, error) {
	c.mu.Lock()
	cast, ok := c.byID[id]
	now := c.now()
	if !ok || cast.Device != device || now.After(cast.Expires) {
		c.mu.Unlock()
		return nil, errNoCast
	}
	stale := now.Sub(cast.checked) > time.Minute
	c.mu.Unlock()
	if stale {
		ok, err := c.authz.CanRead(ctx, cast.By, cast.Key)
		if err != nil {
			return nil, err
		}
		if !ok {
			c.mu.Lock()
			delete(c.byID, id)
			c.mu.Unlock()
			return nil, errNotAllowed
		}
		c.mu.Lock()
		cast.checked = now
		c.mu.Unlock()
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

func unixClient(socket string) *http.Client {
	return &http.Client{
		Timeout: 10 * time.Second,
		Transport: &http.Transport{DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			var d net.Dialer
			return d.DialContext(ctx, "unix", socket)
		}},
	}
}

type FilesAuthz struct {
	URL    string
	Client *http.Client
}

func NewFilesAuthz(target string) (*FilesAuthz, error) {
	if sock, ok := strings.CutPrefix(target, "unix:"); ok {
		path, rest, _ := strings.Cut(sock, ":")
		if rest == "" {
			rest = "/_files/authz"
		}
		return &FilesAuthz{URL: "http://authz" + rest, Client: unixClient(path)}, nil
	}
	if !strings.HasPrefix(target, "http://") && !strings.HasPrefix(target, "https://") {
		return nil, fmt.Errorf("authorizer %q is neither unix:<socket>[:<path>] nor a URL", target)
	}
	return &FilesAuthz{URL: target, Client: &http.Client{Timeout: 10 * time.Second}}, nil
}

func (a *FilesAuthz) CanRead(ctx context.Context, by Caster, key string) (bool, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, a.URL, nil)
	if err != nil {
		return false, err
	}
	req.Header.Set("X-Forwarded-Method", "GET")
	req.Header.Set("X-Forwarded-Uri", "/"+(&url.URL{Path: key}).EscapedPath())
	req.Header.Set("Remote-User", by.User)
	req.Header.Set("Remote-Groups", by.Groups)
	resp, err := a.Client.Do(req)
	if err != nil {
		return false, fmt.Errorf("files authorizer: %w", err)
	}
	io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<16))
	resp.Body.Close()
	switch {
	case resp.StatusCode >= 200 && resp.StatusCode < 400:
		return true, nil
	case resp.StatusCode == 401, resp.StatusCode == 403, resp.StatusCode == 404:
		return false, nil
	}
	return false, fmt.Errorf("files authorizer answered %d", resp.StatusCode)
}

type S3 struct {
	Endpoint string
	Bucket   string
	Region   string
	KeyID    string
	Secret   string
	Client   *http.Client
	now      func() time.Time
}

func readCredential(name string) (string, error) {
	dir := os.Getenv("CREDENTIALS_DIRECTORY")
	if dir == "" {
		return "", fmt.Errorf("no CREDENTIALS_DIRECTORY for %s", name)
	}
	b, err := os.ReadFile(dir + "/" + name)
	return strings.TrimSpace(string(b)), err
}

var forwardedRequestHeaders = []string{"Range", "If-Range", "If-None-Match", "If-Modified-Since"}

func (s *S3) Open(ctx context.Context, key string, in http.Header) (*http.Response, error) {
	u, err := url.Parse(strings.TrimSuffix(s.Endpoint, "/") + "/" + s.Bucket + "/" + (&url.URL{Path: key}).EscapedPath())
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}
	for _, h := range forwardedRequestHeaders {
		if v := in.Get(h); v != "" {
			req.Header.Set(h, v)
		}
	}
	s.sign(req)
	client := s.Client
	if client == nil {
		client = http.DefaultClient
	}
	return client.Do(req)
}

func hmacSHA256(key []byte, data string) []byte {
	m := hmac.New(sha256.New, key)
	m.Write([]byte(data))
	return m.Sum(nil)
}

func (s *S3) sign(req *http.Request) {
	now := time.Now
	if s.now != nil {
		now = s.now
	}
	t := now().UTC()
	amzDate := t.Format("20060102T150405Z")
	day := t.Format("20060102")
	req.Header.Set("X-Amz-Date", amzDate)
	req.Header.Set("X-Amz-Content-Sha256", "UNSIGNED-PAYLOAD")
	host := req.URL.Host
	names := []string{"host"}
	values := map[string]string{"host": host}
	for k := range req.Header {
		lk := strings.ToLower(k)
		names = append(names, lk)
		values[lk] = strings.TrimSpace(req.Header.Get(k))
	}
	sort.Strings(names)
	var canonHeaders strings.Builder
	for _, n := range names {
		canonHeaders.WriteString(n + ":" + values[n] + "\n")
	}
	signed := strings.Join(names, ";")
	canonical := strings.Join([]string{
		req.Method,
		req.URL.EscapedPath(),
		req.URL.RawQuery,
		canonHeaders.String(),
		signed,
		"UNSIGNED-PAYLOAD",
	}, "\n")
	scope := day + "/" + s.Region + "/s3/aws4_request"
	sum := sha256.Sum256([]byte(canonical))
	toSign := "AWS4-HMAC-SHA256\n" + amzDate + "\n" + scope + "\n" + hex.EncodeToString(sum[:])
	k := hmacSHA256([]byte("AWS4"+s.Secret), day)
	k = hmacSHA256(k, s.Region)
	k = hmacSHA256(k, "s3")
	k = hmacSHA256(k, "aws4_request")
	sig := hex.EncodeToString(hmacSHA256(k, toSign))
	req.Header.Set("Authorization", "AWS4-HMAC-SHA256 Credential="+s.KeyID+"/"+scope+", SignedHeaders="+signed+", Signature="+sig)
}

var forwardedResponseHeaders = []string{"Content-Type", "Content-Length", "Content-Range", "Accept-Ranges", "ETag", "Last-Modified"}
