package main

import (
	"bytes"
	"compress/gzip"
	"io"
	"strings"
	"testing"

	"github.com/andybalholm/brotli"
)

func TestStaticBrotliAndNegotiation(t *testing.T) {
	h := testSite(t)
	raw := strings.Repeat("console.log('hello staff screens');\n", 200)
	// Brotli is preferred when accepted, and decodes to the original.
	w := get(h, "GET", "/assets/app-abc.js", map[string]string{"Accept-Encoding": "gzip, deflate, br"})
	if w.Header().Get("Content-Encoding") != "br" {
		t.Fatalf("expected br, got %q", w.Header().Get("Content-Encoding"))
	}
	out, err := io.ReadAll(brotli.NewReader(bytes.NewReader(w.Body.Bytes())))
	if err != nil || string(out) != raw {
		t.Fatalf("brotli body does not decode to the original: %v", err)
	}
	// Gzip when brotli is not accepted.
	w = get(h, "GET", "/assets/app-abc.js", map[string]string{"Accept-Encoding": "gzip"})
	zr, err := gzip.NewReader(bytes.NewReader(w.Body.Bytes()))
	if w.Header().Get("Content-Encoding") != "gzip" || err != nil {
		t.Fatalf("expected gzip: %q %v", w.Header().Get("Content-Encoding"), err)
	}
	if b, _ := io.ReadAll(zr); string(b) != raw {
		t.Error("gzip body differs")
	}
	// q=0 means refused; identity when nothing is accepted; a header with a refused br falls back to gzip.
	for hdr, want := range map[string]string{"br;q=0, gzip": "gzip", "gzip;q=0": "", "": "", "identity": "", "br;q=0.5, gzip;q=0.9": "gzip", "gzip;q=0.5, br;q=0.9": "br", "BR": "br"} {
		w = get(h, "GET", "/assets/app-abc.js", map[string]string{"Accept-Encoding": hdr})
		if got := w.Header().Get("Content-Encoding"); got != want {
			t.Errorf("Accept-Encoding %q: got %q want %q", hdr, got, want)
		}
	}
	if w = get(h, "GET", "/assets/app-abc.js", nil); w.Body.String() != raw || w.Header().Get("Content-Encoding") != "" {
		t.Error("identity body must be the raw file")
	}
	// Each encoding has its own ETag, and a conditional request matches only its own.
	br := get(h, "GET", "/assets/app-abc.js", map[string]string{"Accept-Encoding": "br"}).Header().Get("ETag")
	gz := get(h, "GET", "/assets/app-abc.js", map[string]string{"Accept-Encoding": "gzip"}).Header().Get("ETag")
	id := get(h, "GET", "/assets/app-abc.js", nil).Header().Get("ETag")
	if br == gz || br == id || gz == id {
		t.Errorf("ETags must differ per encoding: %s %s %s", br, gz, id)
	}
	if w = get(h, "GET", "/assets/app-abc.js", map[string]string{"Accept-Encoding": "br", "If-None-Match": br}); w.Code != 304 {
		t.Errorf("matching ETag: %d", w.Code)
	}
	if w = get(h, "GET", "/assets/app-abc.js", map[string]string{"Accept-Encoding": "br", "If-None-Match": gz}); w.Code != 200 {
		t.Errorf("another encoding's ETag must not match: %d", w.Code)
	}
	// Vary is set so shared caches keep encodings apart; images are never compressed again.
	if !strings.Contains(w.Header().Get("Vary"), "Accept-Encoding") {
		t.Error("Vary: Accept-Encoding missing")
	}
	if w = get(h, "GET", "/assets/logo-abc.png", map[string]string{"Accept-Encoding": "br, gzip"}); w.Header().Get("Content-Encoding") != "" {
		t.Error("an image must not be compressed again")
	}
	// Caching: fingerprinted assets are immutable for a year, the page is revalidated.
	if cc := get(h, "GET", "/assets/app-abc.js", nil).Header().Get("Cache-Control"); cc != "public, max-age=31536000, immutable" {
		t.Errorf("asset cache: %s", cc)
	}
	if cc := get(h, "GET", "/", nil).Header().Get("Cache-Control"); cc != "no-cache" {
		t.Errorf("page cache: %s", cc)
	}
}
