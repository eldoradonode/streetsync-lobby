package main

import (
	"bytes"
	"compress/gzip"
	"crypto/sha1"
	"embed"
	"encoding/hex"
	"io/fs"
	"mime"
	"net/http"
	"path"
	"strconv"
	"strings"
)

// The page and its files are built into the program. Each one is compressed once, when the program starts, and handed out
// compressed to browsers that accept it, so a visit costs the compressed size and not the raw size.
//
//	/                 the page itself. Checked again on every visit, but answered with "not modified" when unchanged.
//	/assets/<name>    the app script, its stylesheet and the staff screens. Their names change when their content
//	                  does, so browsers keep them for a year and never download the same file twice.
//
//go:embed web
var webFiles embed.FS

type asset struct {
	raw, gz []byte
	ctype   string
	cache   string
	etag    string
}

func loadAssets(fsys fs.FS, root string) (map[string]*asset, error) {
	out := map[string]*asset{}
	err := fs.WalkDir(fsys, root, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		raw, err := fs.ReadFile(fsys, p)
		if err != nil {
			return err
		}
		name := strings.TrimPrefix(p, root)
		if name == "/index.html" {
			name = "/"
		}
		sum := sha1.Sum(raw)
		a := &asset{raw: raw, etag: `"` + hex.EncodeToString(sum[:8]) + `"`, cache: "public, max-age=31536000, immutable"}
		if name == "/" {
			a.cache = "no-cache" // may change on every deploy
		}
		a.ctype = mime.TypeByExtension(path.Ext(p))
		switch path.Ext(p) {
		case ".js", ".mjs":
			a.ctype = "text/javascript; charset=utf-8"
		case ".css":
			a.ctype = "text/css; charset=utf-8"
		case ".html":
			a.ctype = "text/html; charset=utf-8"
		}
		if a.ctype == "" {
			a.ctype = "application/octet-stream"
		}
		if strings.HasPrefix(a.ctype, "text/") || strings.HasSuffix(a.ctype, "json") || strings.HasSuffix(a.ctype, "svg+xml") {
			var buf bytes.Buffer
			zw, _ := gzip.NewWriterLevel(&buf, gzip.BestCompression)
			_, _ = zw.Write(raw)
			_ = zw.Close()
			if buf.Len() < len(raw) {
				a.gz = buf.Bytes()
			}
		}
		out[name] = a
		return nil
	})
	return out, err
}

func staticHandler(fsys fs.FS, root string) (http.Handler, error) {
	assets, err := loadAssets(fsys, root)
	if err != nil {
		return nil, err
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.Header().Set("Allow", "GET, HEAD")
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		a := assets[r.URL.Path]
		if a == nil || (r.URL.Path != "/" && !strings.HasPrefix(r.URL.Path, "/assets/")) {
			http.NotFound(w, r)
			return
		}
		h := w.Header()
		h.Set("Content-Type", a.ctype)
		h.Set("Cache-Control", a.cache)
		h.Set("ETag", a.etag)
		h.Set("X-Content-Type-Options", "nosniff")
		h.Add("Vary", "Accept-Encoding")
		if r.Header.Get("If-None-Match") == a.etag {
			w.WriteHeader(http.StatusNotModified)
			return
		}
		body := a.raw
		if a.gz != nil && strings.Contains(r.Header.Get("Accept-Encoding"), "gzip") {
			body = a.gz
			h.Set("Content-Encoding", "gzip")
		}
		h.Set("Content-Length", strconv.Itoa(len(body)))
		if r.Method == http.MethodGet {
			_, _ = w.Write(body)
		}
	}), nil
}
