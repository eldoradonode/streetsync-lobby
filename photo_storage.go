package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// Profile photo storage. The database keeps only a key and a version. The bytes live behind this interface so the
// provider can be changed without touching the rest of the code. Nothing here is paid or switched on by itself:
// with no configuration photos are off and the page hides the upload button.
//
//	PHOTO_STORAGE=supabase   uses Supabase Storage (SUPABASE_URL, SUPABASE_SERVICE_KEY, optional PHOTO_BUCKET)
//	PHOTO_STORAGE=dir        files in PHOTO_DIR (development only: Render's free disk is wiped on every deploy)
//	unset                    photos are off
type photoStorage interface {
	Put(ctx context.Context, key string, data []byte, contentType string) error
	Get(ctx context.Context, key string) ([]byte, error)
	Delete(ctx context.Context, key string) error
}

var errNoPhoto = errors.New("no such photo")

// validPhotoKey is the only shape of key the server ever builds, so a key can never walk out of its folder.
func validPhotoKey(k string) bool {
	if len(k) < 8 || len(k) > 120 {
		return false
	}
	for _, r := range k {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '-', r == '_', r == '/', r == '.':
		default:
			return false
		}
	}
	return !strings.Contains(k, "..") && !strings.HasPrefix(k, "/") && !strings.Contains(k, "//")
}

func photoStorageFromEnv() photoStorage {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("PHOTO_STORAGE"))) {
	case "supabase":
		base, key := strings.TrimRight(strings.TrimSpace(os.Getenv("SUPABASE_URL")), "/"), strings.TrimSpace(os.Getenv("SUPABASE_SERVICE_KEY"))
		if base == "" || key == "" {
			return nil
		}
		b := strings.TrimSpace(os.Getenv("PHOTO_BUCKET"))
		if b == "" {
			b = "avatars"
		}
		return &supabaseStorage{base: base, key: key, bucket: b, hc: &http.Client{Timeout: 10 * time.Second}}
	case "dir":
		d := strings.TrimSpace(os.Getenv("PHOTO_DIR"))
		if d == "" {
			return nil
		}
		return &dirStorage{root: d}
	}
	return nil
}

// ---- Supabase Storage over its REST interface (private bucket, the service key stays on the server) ----

type supabaseStorage struct {
	base, key, bucket string
	hc                *http.Client
}

func (s *supabaseStorage) objURL(key string) string {
	parts := strings.Split(key, "/")
	for i := range parts {
		parts[i] = url.PathEscape(parts[i])
	}
	return s.base + "/storage/v1/object/" + url.PathEscape(s.bucket) + "/" + strings.Join(parts, "/")
}

func (s *supabaseStorage) do(ctx context.Context, method, key string, body []byte, ct string) (*http.Response, error) {
	if !validPhotoKey(key) {
		return nil, errors.New("bad photo key")
	}
	req, err := http.NewRequestWithContext(ctx, method, s.objURL(key), bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+s.key)
	req.Header.Set("apikey", s.key)
	if ct != "" {
		req.Header.Set("Content-Type", ct)
		req.Header.Set("x-upsert", "true")
	}
	return s.hc.Do(req)
}

func (s *supabaseStorage) Put(ctx context.Context, key string, data []byte, ct string) error {
	resp, err := s.do(ctx, http.MethodPost, key, data, ct)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		return fmt.Errorf("storage put: status %d", resp.StatusCode)
	}
	return nil
}

func (s *supabaseStorage) Get(ctx context.Context, key string) ([]byte, error) {
	resp, err := s.do(ctx, http.MethodGet, key, nil, "")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusBadRequest {
		return nil, errNoPhoto
	}
	if resp.StatusCode/100 != 2 {
		return nil, fmt.Errorf("storage get: status %d", resp.StatusCode)
	}
	return io.ReadAll(io.LimitReader(resp.Body, 2<<20))
}

func (s *supabaseStorage) Delete(ctx context.Context, key string) error {
	resp, err := s.do(ctx, http.MethodDelete, key, nil, "")
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 && resp.StatusCode != http.StatusNotFound {
		return fmt.Errorf("storage delete: status %d", resp.StatusCode)
	}
	return nil
}

// ---- local folder, for development ----

type dirStorage struct{ root string }

func (d *dirStorage) path(key string) (string, error) {
	if !validPhotoKey(key) {
		return "", errors.New("bad photo key")
	}
	return filepath.Join(d.root, filepath.FromSlash(key)), nil
}

func (d *dirStorage) Put(_ context.Context, key string, data []byte, _ string) error {
	p, err := d.path(key)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o750); err != nil {
		return err
	}
	return os.WriteFile(p, data, 0o640)
}

func (d *dirStorage) Get(_ context.Context, key string) ([]byte, error) {
	p, err := d.path(key)
	if err != nil {
		return nil, err
	}
	b, err := os.ReadFile(p)
	if os.IsNotExist(err) {
		return nil, errNoPhoto
	}
	return b, err
}

func (d *dirStorage) Delete(_ context.Context, key string) error {
	p, err := d.path(key)
	if err != nil {
		return err
	}
	if err := os.Remove(p); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// ---- in memory, for tests ----

type memStorage struct {
	mu   sync.Mutex
	data map[string][]byte
	fail bool
}

func newMemStorage() *memStorage { return &memStorage{data: map[string][]byte{}} }

func (m *memStorage) Put(_ context.Context, key string, b []byte, _ string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.fail {
		return errors.New("storage down")
	}
	if !validPhotoKey(key) {
		return errors.New("bad photo key")
	}
	m.data[key] = append([]byte(nil), b...)
	return nil
}

func (m *memStorage) Get(_ context.Context, key string) ([]byte, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	b, ok := m.data[key]
	if !ok {
		return nil, errNoPhoto
	}
	return b, nil
}

func (m *memStorage) Delete(_ context.Context, key string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.data, key)
	return nil
}

func (m *memStorage) count() int { m.mu.Lock(); defer m.mu.Unlock(); return len(m.data) }
