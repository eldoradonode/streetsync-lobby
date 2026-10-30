package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/jpeg"
	"image/png"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// Profile photos. People pick and crop a picture in the page, the page sends a small JPEG (or the original PNG or JPEG),
// and the server does not trust any of it: it checks the real format, the size and the pixel count, decodes it,
// and builds two clean JPEGs itself (a 512 px picture and a 96 px thumbnail). Metadata and anything hidden in the
// original file never reach storage. Filenames and keys are made here, never taken from the upload.
const (
	maxPhotoUpload = 3 << 20  // bytes accepted from the page
	maxPhotoSide   = 4096     // pixels, either side
	minPhotoSide   = 48       // pixels, either side
	maxPhotoPixels = 12000000 // decoded pixels, so a small file cannot expand into a huge image
	photoMain      = 512
	photoThumb     = 96
)

var photoGap = 5 * time.Second // least time between two changes by one person

type photoError struct{ code string }

func (e photoError) Error() string { return e.code }

// processPhoto turns an upload into the two stored pictures.
func processPhoto(data []byte) (main, thumb []byte, err error) {
	if len(data) == 0 || len(data) > maxPhotoUpload {
		return nil, nil, photoError{"photo_too_big"}
	}
	if len(data) >= 12 && string(data[0:4]) == "RIFF" && string(data[8:12]) == "WEBP" {
		// The server has no WebP decoder (the Go standard library has none). The page converts WebP before sending.
		return nil, nil, photoError{"photo_webp_here"}
	}
	cfg, format, derr := image.DecodeConfig(bytes.NewReader(data))
	if derr != nil || (format != "jpeg" && format != "png") {
		return nil, nil, photoError{"photo_bad"}
	}
	if cfg.Width > maxPhotoSide || cfg.Height > maxPhotoSide || cfg.Width*cfg.Height > maxPhotoPixels {
		return nil, nil, photoError{"photo_dimensions"}
	}
	if cfg.Width < minPhotoSide || cfg.Height < minPhotoSide {
		return nil, nil, photoError{"photo_small"}
	}
	img, _, derr := image.Decode(bytes.NewReader(data))
	if derr != nil {
		return nil, nil, photoError{"photo_bad"}
	}
	if main, err = encodeSquare(img, photoMain, 82); err != nil {
		return nil, nil, err
	}
	if thumb, err = encodeSquare(img, photoThumb, 80); err != nil {
		return nil, nil, err
	}
	return main, thumb, nil
}

// encodeSquare crops the middle square and averages it down to size x size, over a white background.
func encodeSquare(src image.Image, size, quality int) ([]byte, error) {
	b := src.Bounds()
	side := b.Dx()
	if b.Dy() < side {
		side = b.Dy()
	}
	x0, y0 := b.Min.X+(b.Dx()-side)/2, b.Min.Y+(b.Dy()-side)/2
	dst := image.NewRGBA(image.Rect(0, 0, size, size))
	draw.Draw(dst, dst.Bounds(), image.NewUniform(color.White), image.Point{}, draw.Src)
	for dy := 0; dy < size; dy++ {
		sy0, sy1 := y0+dy*side/size, y0+(dy+1)*side/size
		if sy1 <= sy0 {
			sy1 = sy0 + 1
		}
		for dx := 0; dx < size; dx++ {
			sx0, sx1 := x0+dx*side/size, x0+(dx+1)*side/size
			if sx1 <= sx0 {
				sx1 = sx0 + 1
			}
			var r, g, bl, n uint64
			for y := sy0; y < sy1; y++ {
				for x := sx0; x < sx1; x++ {
					cr, cg, cb, ca := src.At(x, y).RGBA() // premultiplied, so adding the missing alpha as white flattens it
					r += uint64(cr + (0xffff - ca))
					g += uint64(cg + (0xffff - ca))
					bl += uint64(cb + (0xffff - ca))
					n++
				}
			}
			i := dst.PixOffset(dx, dy)
			dst.Pix[i], dst.Pix[i+1], dst.Pix[i+2], dst.Pix[i+3] = uint8(r/n>>8), uint8(g/n>>8), uint8(bl/n>>8), 255
		}
	}
	var out bytes.Buffer
	if err := jpeg.Encode(&out, dst, &jpeg.Options{Quality: quality}); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

var _ = png.Decode // registers the PNG decoder for image.Decode

// ---- store ----

// photoLookSQL is the part added to a look code when the person has a photo, like "f3~7". The number only busts caches.
const photoLookSQL = `CASE WHEN u.photo_key IS NOT NULL THEN '~' || u.photo_ver ELSE '' END`

func (s *store) photoVerOf(ctx context.Context, id int64) (int, error) {
	var v int
	err := s.db.QueryRowContext(ctx, `SELECT CASE WHEN photo_key IS NOT NULL THEN photo_ver ELSE 0 END FROM users WHERE id = $1`, id).Scan(&v)
	return v, err
}

func (s *store) photoKeyByID(ctx context.Context, id int64) string {
	var k *string
	_ = s.db.QueryRowContext(ctx, `SELECT photo_key FROM users WHERE id = $1`, id).Scan(&k)
	if k == nil {
		return ""
	}
	return *k
}

// setPhoto records a new photo and returns the key it replaced and the new version.
func (s *store) setPhoto(ctx context.Context, id int64, key string) (old string, ver int, err error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return "", 0, err
	}
	defer tx.Rollback()
	var o *string
	if err = tx.QueryRowContext(ctx, `SELECT photo_key FROM users WHERE id = $1 AND deleted_at IS NULL FOR UPDATE`, id).Scan(&o); err != nil {
		return "", 0, err
	}
	if err = tx.QueryRowContext(ctx, `UPDATE users SET photo_key = $2, photo_ver = photo_ver + 1 WHERE id = $1 RETURNING photo_ver`, id, key).Scan(&ver); err != nil {
		return "", 0, err
	}
	if err = tx.Commit(); err != nil {
		return "", 0, err
	}
	if o != nil {
		old = *o
	}
	return old, ver, nil
}

func (s *store) clearPhoto(ctx context.Context, id int64) (old string, err error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return "", err
	}
	defer tx.Rollback()
	var o *string
	if err = tx.QueryRowContext(ctx, `SELECT photo_key FROM users WHERE id = $1 FOR UPDATE`, id).Scan(&o); err != nil {
		return "", err
	}
	if _, err = tx.ExecContext(ctx, `UPDATE users SET photo_key = NULL, photo_ver = photo_ver + 1 WHERE id = $1`, id); err != nil {
		return "", err
	}
	if err = tx.Commit(); err != nil {
		return "", err
	}
	if o != nil {
		old = *o
	}
	return old, nil
}

// photoOfHandle finds the stored key of someone who may be shown: not deleted, not banned.
func (s *store) photoOfHandle(ctx context.Context, handleKey string) (string, bool) {
	var k *string
	err := s.db.QueryRowContext(ctx,
		`SELECT photo_key FROM users WHERE handle_key = $1 AND deleted_at IS NULL AND banned_at IS NULL`, handleKey).Scan(&k)
	if err != nil || k == nil {
		return "", false
	}
	return *k, true
}

// ---- hub side ----

// look is the avatar code everyone is shown for this person: the picked look plus the photo version, if any.
// The caller holds the hub lock or owns the value.
func (ou *onlineUser) look() string {
	if ou.photoVer > 0 {
		return ou.avatar + "~" + strconv.Itoa(ou.photoVer)
	}
	return ou.avatar
}

func (h *hub) photoRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/photo", h.photoAPI)
	mux.HandleFunc("/api/photo/status", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"on":%v}`, h.photos != nil)
	})
	mux.HandleFunc("/photo/", h.photoGet)
}

func photoReply(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func (h *hub) photoUser(r *http.Request) (userRow, bool) {
	tok := strings.TrimSpace(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "))
	if tok == "" || len(tok) > 200 {
		return userRow{}, false
	}
	ctx, cancel := context.WithTimeout(r.Context(), authWait)
	defer cancel()
	u, err := h.st.userBySession(ctx, tok)
	if err != nil || u.Banned || u.Deleted {
		return userRow{}, false
	}
	return u, true
}

func (h *hub) photoAPI(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost && r.Method != http.MethodDelete {
		w.Header().Set("Allow", "POST, DELETE")
		photoReply(w, http.StatusMethodNotAllowed, map[string]string{"e": "bad_method"})
		return
	}
	if h.photos == nil {
		photoReply(w, http.StatusServiceUnavailable, map[string]string{"e": "photos_off"})
		return
	}
	u, ok := h.photoUser(r) // the person comes from the login token only; there is no way to name another account
	if !ok {
		photoReply(w, http.StatusUnauthorized, map[string]string{"e": "not_signed_in"})
		return
	}
	now := time.Now()
	h.mu.Lock()
	if h.photoLast == nil {
		h.photoLast = map[int64]time.Time{}
	}
	if last := h.photoLast[u.ID]; now.Sub(last) < photoGap {
		h.mu.Unlock()
		photoReply(w, http.StatusTooManyRequests, map[string]string{"e": "slow_down"})
		return
	}
	h.photoLast[u.ID] = now
	h.mu.Unlock()
	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()

	if r.Method == http.MethodDelete {
		old, err := h.st.clearPhoto(ctx, u.ID)
		if err != nil {
			log.Printf("removing a photo failed: %v", err)
			photoReply(w, http.StatusInternalServerError, map[string]string{"e": "server_busy"})
			return
		}
		h.photoDropObjects(old)
		h.photoApply(u.ID)
		photoReply(w, http.StatusOK, map[string]any{"ok": true})
		return
	}

	data, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxPhotoUpload+1))
	if err != nil {
		photoReply(w, http.StatusRequestEntityTooLarge, map[string]string{"e": "photo_too_big"})
		return
	}
	mainJ, thumbJ, perr := processPhoto(data)
	if perr != nil {
		var pe photoError
		code := "photo_bad"
		if errors.As(perr, &pe) {
			code = pe.code
		}
		photoReply(w, http.StatusUnprocessableEntity, map[string]string{"e": code})
		return
	}
	var rnd [9]byte
	_, _ = rand.Read(rnd[:])
	base := fmt.Sprintf("u%d/%s", u.ID, hex.EncodeToString(rnd[:]))
	if err := h.photos.Put(ctx, base+"-m.jpg", mainJ, "image/jpeg"); err == nil {
		err = h.photos.Put(ctx, base+"-t.jpg", thumbJ, "image/jpeg")
		if err != nil {
			h.photoDropObjects(base)
		}
		if err == nil {
			old, _, serr := h.st.setPhoto(ctx, u.ID, base)
			if serr != nil {
				h.photoDropObjects(base)
				log.Printf("saving a photo failed: %v", serr)
				photoReply(w, http.StatusInternalServerError, map[string]string{"e": "server_busy"})
				return
			}
			h.photoDropObjects(old)
			look := h.photoApply(u.ID)
			photoReply(w, http.StatusOK, map[string]any{"ok": true, "look": look})
			return
		}
	}
	log.Printf("storing a photo failed")
	photoReply(w, http.StatusBadGateway, map[string]string{"e": "photo_storage"})
}

// photoApply refreshes the in-memory look of a person who is online and returns the look code.
func (h *hub) photoApply(id int64) string {
	ctx, cancel := context.WithTimeout(context.Background(), authWait)
	defer cancel()
	ver, _ := h.st.photoVerOf(ctx, id)
	h.mu.Lock()
	defer h.mu.Unlock()
	if ou := h.users[id]; ou != nil {
		ou.photoVer = ver
		return ou.look()
	}
	return ""
}

func (h *hub) photoDropObjects(base string) {
	if base == "" || h.photos == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	for _, sfx := range []string{"-m.jpg", "-t.jpg"} {
		if err := h.photos.Delete(ctx, base+sfx); err != nil {
			log.Printf("deleting a stored photo failed")
		}
	}
}

// photoGet serves one picture: /photo/<handle>?s=t (thumbnail, the default) or s=m (profile size).
func (h *hub) photoGet(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	handle := strings.TrimPrefix(r.URL.Path, "/photo/")
	if h.photos == nil || !handleRe.MatchString(handle) {
		w.Header().Set("Cache-Control", "public, max-age=60")
		http.NotFound(w, r)
		return
	}
	sfx := "-t.jpg"
	if r.URL.Query().Get("s") == "m" {
		sfx = "-m.jpg"
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	key, ok := h.st.photoOfHandle(ctx, strings.ToLower(handle))
	if !ok {
		w.Header().Set("Cache-Control", "public, max-age=60")
		http.NotFound(w, r)
		return
	}
	data, err := h.photos.Get(ctx, key+sfx)
	if err != nil {
		w.Header().Set("Cache-Control", "public, max-age=60")
		http.NotFound(w, r)
		return
	}
	etag := `"` + tokenHash(key + sfx)[:16] + `"`
	w.Header().Set("ETag", etag)
	if r.Header.Get("If-None-Match") == etag {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	w.Header().Set("Content-Type", "image/jpeg")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Security-Policy", "default-src 'none'")
	if r.URL.Query().Get("v") != "" {
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable") // the version in the address changes with the picture
	} else {
		w.Header().Set("Cache-Control", "public, max-age=300")
	}
	w.Header().Set("Content-Length", strconv.Itoa(len(data)))
	if r.Method == http.MethodGet {
		_, _ = w.Write(data)
	}
}
