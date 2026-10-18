package main

import (
	"compress/gzip"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
)

func testSite(t *testing.T) http.Handler {
	t.Helper()
	big := strings.Repeat("console.log('hello staff screens');\n", 200)
	h, err := staticHandler(fstest.MapFS{
		"web/index.html":          {Data: []byte("<!doctype html><title>x</title>" + strings.Repeat("a", 500))},
		"web/assets/app-abc.js":   {Data: []byte(big)},
		"web/assets/app-abc.css":  {Data: []byte(strings.Repeat("body{color:red}", 100))},
		"web/assets/logo-abc.png": {Data: []byte{0x89, 'P', 'N', 'G'}},
		"web/secret.txt":          {Data: []byte("not for the web")},
	}, "web")
	if err != nil {
		t.Fatal(err)
	}
	return h
}

func get(h http.Handler, method, path string, hdr map[string]string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, path, nil)
	for k, v := range hdr {
		r.Header.Set(k, v)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

func TestStaticPageIsRevalidated(t *testing.T) {
	h := testSite(t)
	w := get(h, "GET", "/", nil)
	if w.Code != 200 || w.Header().Get("Cache-Control") != "no-cache" || !strings.HasPrefix(w.Header().Get("Content-Type"), "text/html") {
		t.Fatalf("page: %d %v", w.Code, w.Header())
	}
	etag := w.Header().Get("ETag")
	if etag == "" {
		t.Fatal("page has no ETag")
	}
	w = get(h, "GET", "/", map[string]string{"If-None-Match": etag})
	if w.Code != 304 || w.Body.Len() != 0 {
		t.Fatalf("unchanged page should be 304 with no body, got %d with %d bytes", w.Code, w.Body.Len())
	}
	if w = get(h, "GET", "/", map[string]string{"If-None-Match": `"other"`}); w.Code != 200 {
		t.Fatalf("a different ETag must get the page, got %d", w.Code)
	}
}

func TestStaticAssetsAreImmutableAndCompressed(t *testing.T) {
	h := testSite(t)
	w := get(h, "GET", "/assets/app-abc.js", map[string]string{"Accept-Encoding": "gzip, br"})
	if w.Code != 200 || w.Header().Get("Cache-Control") != "public, max-age=31536000, immutable" || w.Header().Get("Content-Encoding") != "gzip" {
		t.Fatalf("asset: %d %v", w.Code, w.Header())
	}
	if !strings.HasPrefix(w.Header().Get("Content-Type"), "text/javascript") {
		t.Fatalf("script content type: %s", w.Header().Get("Content-Type"))
	}
	zr, err := gzip.NewReader(w.Body)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := io.ReadAll(zr)
	if !strings.Contains(string(raw), "hello staff screens") || w.Body.Len() != 0 && false {
		t.Fatal("body does not decompress to the script")
	}
	plain := get(h, "GET", "/assets/app-abc.js", nil)
	if plain.Header().Get("Content-Encoding") != "" || plain.Body.Len() != len(raw) {
		t.Fatalf("without Accept-Encoding the file is sent as it is: %q %d vs %d", plain.Header().Get("Content-Encoding"), plain.Body.Len(), len(raw))
	}
	if !strings.Contains(w.Header().Get("Vary"), "Accept-Encoding") {
		t.Fatal("missing Vary header")
	}
	if w = get(h, "GET", "/assets/logo-abc.png", map[string]string{"Accept-Encoding": "gzip"}); w.Header().Get("Content-Encoding") != "" {
		t.Fatal("images are not compressed again")
	}
}

func TestStaticOnlyServesThePageAndAssets(t *testing.T) {
	h := testSite(t)
	for _, p := range []string{"/secret.txt", "/index.html", "/assets/", "/assets/missing.js", "/assets/../secret.txt", "/web/index.html", "/ws-not-here"} {
		if w := get(h, "GET", p, nil); w.Code != 404 {
			t.Errorf("%s should be 404, got %d", p, w.Code)
		}
	}
	if w := get(h, "POST", "/", nil); w.Code != 405 {
		t.Errorf("POST should be refused, got %d", w.Code)
	}
	if w := get(h, "HEAD", "/", nil); w.Code != 200 || w.Body.Len() != 0 {
		t.Errorf("HEAD should give headers only, got %d with %d bytes", w.Code, w.Body.Len())
	}
}
