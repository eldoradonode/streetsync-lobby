package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// Step 18: room membership, join and leave notices, over the real hub and database.

func (e *testEnv) communityRoom(host string) string {
	e.t.Helper()
	u := e.account(host, "user")
	e.st.db.Exec(`UPDATE users SET points = 500 WHERE id = $1`, u.ID)
	e.st.applyCredit(context.Background(), u.ID, "grant", 100, "g18"+host, "")
	w := e.login(host)
	w.send(map[string]any{"t": "croom", "n": "T18 " + host, "k": "public", "d": "step 18 room"})
	slug := w.expect("croomok")["s"].(string)
	w.join(slug)
	w.drain()
	return slug + "|" + host
}

func TestStep18RoomMembershipNoticesAndDelivery(t *testing.T) {
	e := newTestEnv(t)
	spec := e.communityRoom("t_18host")
	slug := spec[:len(spec)-len("|t_18host")]
	// the host's first connection already joined above; use a second login as the watcher of the room
	wh := e.login("t_18host")
	wh.join(slug)
	wh.drain()
	e.account("t_18bob", "user")
	e.account("t_18cy", "user")

	// A failed join (no such room) changes nothing: no notice, no membership.
	wb := e.login("t_18bob")
	if got := wb.ask(map[string]any{"t": "join", "r": "no-such-room-18"}, "joined"); got != "err:no_such_room" {
		t.Fatalf("failed join: %s", got)
	}
	wh.quiet("rs", 200*time.Millisecond)
	if e.count(`SELECT count(*) FROM room_members m JOIN users u ON u.id = m.user_id WHERE u.handle_key = 't_18bob'`) != 0 {
		t.Fatal("a failed join created a membership")
	}

	// A good join is confirmed with joined+j, notified once, and repeated requests add nothing.
	wb.send(map[string]any{"t": "join", "r": slug})
	jf := wb.expect("joined")
	if jf["j"] != true {
		t.Fatalf("joined frame lacks the confirmed membership flag: %v", jf)
	}
	if f := wh.expect("rs"); f["a"] != "j" || f["h"] != "t_18bob" {
		t.Fatalf("join notice: %v", f)
	}
	wb.send(map[string]any{"t": "join", "r": slug})
	wb2 := e.login("t_18bob")
	wb2.join(slug)
	wh.quiet("rs", 400*time.Millisecond)
	if e.count(`SELECT count(*) FROM room_members m JOIN users u ON u.id = m.user_id WHERE u.handle_key = 't_18bob' AND m.left_at IS NULL`) != 1 {
		t.Fatal("membership should be exactly one row")
	}

	// The room list tells the joiner they are joined, and nobody else.
	if r := roomEntry(wb, slug); r == nil || r["j"] != true {
		t.Fatalf("list entry after join: %v", r)
	}

	// Messages reach members in the room; leaving removes delivery at once.
	wh.say("hello before leave")
	wb.sawMessage("hello before leave")
	wb.send(map[string]any{"t": "rleave", "r": slug})
	wb.expect("rleft")
	if f := wh.expect("rs"); f["a"] != "l" || f["h"] != "t_18bob" {
		t.Fatalf("leave notice: %v", f)
	}
	wb2.drain()
	wh.say("hello after leave")
	wb.quiet("m", 400*time.Millisecond)
	wb2.quiet("m", 100*time.Millisecond)
	// A repeated leave is refused and makes no second notice.
	if got := wb.ask(map[string]any{"t": "rleave", "r": slug}, "rleft"); got != "err:not_member" {
		t.Fatalf("second leave: %s", got)
	}
	wh.quiet("rs", 300*time.Millisecond)

	// Rejoin notifies once more; system notices store nothing and earn nothing.
	pts := e.count(`SELECT points FROM users WHERE handle_key = 't_18bob'`)
	msgs := e.count(`SELECT count(*) FROM messages`)
	wb.join(slug)
	wh.expect("rs")
	if e.count(`SELECT count(*) FROM messages`) != msgs || e.count(`SELECT points FROM users WHERE handle_key = 't_18bob'`) != pts {
		t.Fatal("a notice stored a message or earned points")
	}

	// Walking away and coming back within the grace period: no leave and no second join.
	wb.send(map[string]any{"t": "leave"})
	wb.expect("left")
	wh.quiet("rs", 300*time.Millisecond)
	wb.join(slug)
	wh.quiet("rs", 300*time.Millisecond)
}

func presOf(f map[string]any) string { s, _ := f["o"].(string); return s }

func TestStep18Presence(t *testing.T) {
	old := presenceGrace
	presenceGrace = 400 * time.Millisecond
	defer func() { presenceGrace = old }()
	e := newTestEnv(t)
	e.account("t_18pa", "user")
	e.account("t_18pb", "user")
	e.account("t_18pc", "user")
	wa := e.login("t_18pa")

	// Offline person: the thread says so.
	wa.send(map[string]any{"t": "thread", "h": "t_18pb"})
	if f := wa.expect("pres"); f["h"] != "t_18pb" || presOf(f) != "0" {
		t.Fatalf("expected offline: %v", f)
	}
	// They connect: online at once.
	b1 := e.login("t_18pb")
	if f := wa.expect("pres"); presOf(f) != "1" {
		t.Fatalf("expected online: %v", f)
	}
	// A second session, then the first drops: still online, nothing announced.
	b2 := e.login("t_18pb")
	wa.quiet("pres", 150*time.Millisecond)
	b1.c.CloseNow()
	wa.quiet("pres", 800*time.Millisecond)
	// Last session drops: offline only after the grace period.
	b2.c.CloseNow()
	wa.quiet("pres", 100*time.Millisecond)
	if f := wa.expect("pres"); presOf(f) != "0" {
		t.Fatalf("expected offline after grace: %v", f)
	}
	// A reconnect inside the grace period flaps nothing.
	b3 := e.login("t_18pb")
	wa.expect("pres")
	b3.c.CloseNow()
	time.Sleep(100 * time.Millisecond)
	b4 := e.login("t_18pb")
	wa.quiet("pres", 800*time.Millisecond)
	// Someone who has not opened the conversation hears nothing about it.
	wc := e.login("t_18pc")
	b4.c.CloseNow()
	wc.quiet("pres", 800*time.Millisecond)
	// Closing the conversation stops updates.
	wa.send(map[string]any{"t": "unwatch"})
	time.Sleep(50 * time.Millisecond)
	wa.drain()
	b5 := e.login("t_18pb")
	wa.quiet("pres", 300*time.Millisecond)
	b5.c.CloseNow()
	// A blocked pair gets no presence.
	wa.send(map[string]any{"t": "blk", "h": "t_18pb", "a": "add"})
	time.Sleep(100 * time.Millisecond)
	wa.drain()
	wa.send(map[string]any{"t": "thread", "h": "t_18pb"})
	wa.expect("thread")
	wa.quiet("pres", 300*time.Millisecond)
	// The other person blocking me hides it too.
	wa.send(map[string]any{"t": "blk", "h": "t_18pb", "a": "del"})
	time.Sleep(100 * time.Millisecond)
	wb := e.login("t_18pb")
	wb.send(map[string]any{"t": "blk", "h": "t_18pa", "a": "add"})
	time.Sleep(100 * time.Millisecond)
	wa.drain()
	wa.send(map[string]any{"t": "thread", "h": "t_18pb"})
	wa.expect("thread")
	wa.quiet("pres", 300*time.Millisecond)
	// "Show when I am online" off: the person looks offline to everyone.
	wb.send(map[string]any{"t": "blk", "h": "t_18pa", "a": "del"})
	time.Sleep(100 * time.Millisecond)
	wb.send(map[string]any{"t": "set", "k": "so", "b": "0"})
	wb.expect("pf")
	wa.drain()
	wa.send(map[string]any{"t": "thread", "h": "t_18pb"})
	if f := wa.expect("pres"); presOf(f) != "0" {
		t.Fatalf("a person who hides their online state looked online: %v", f)
	}
	wb.send(map[string]any{"t": "set", "k": "so", "b": "1"})
	if f := wa.expect("pres"); presOf(f) != "1" {
		t.Fatalf("showing again: %v", f)
	}
	// Presence writes nothing to the database.
	if e.count(`SELECT count(*) FROM information_schema.columns WHERE column_name IN ('online','last_seen_presence')`) != 0 {
		t.Fatal("presence must not have a database column")
	}
}

// ---- profile photos ----

func photoEnv(t *testing.T) (*testEnv, *memStorage) {
	e := newTestEnv(t)
	ms := newMemStorage()
	e.h.photos = ms
	mux := http.NewServeMux()
	e.h.photoRoutes(mux)
	e.photoSrv = httptest.NewServer(mux)
	t.Cleanup(e.photoSrv.Close)
	return e, ms
}

func makeJPEG(w, h int) []byte {
	im := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			im.Set(x, y, color.RGBA{uint8(x), uint8(y), 120, 255})
		}
	}
	var b bytes.Buffer
	jpeg.Encode(&b, im, nil)
	return b.Bytes()
}

func makePNG(w, h int) []byte {
	im := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			im.Set(x, y, color.NRGBA{200, 30, 30, 128})
		}
	}
	var b bytes.Buffer
	png.Encode(&b, im)
	return b.Bytes()
}

func (e *testEnv) photoReq(method, token string, body []byte) (int, map[string]any) {
	e.t.Helper()
	req, _ := http.NewRequest(method, e.photoSrv.URL+"/api/photo", bytes.NewReader(body))
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		e.t.Fatal(err)
	}
	defer resp.Body.Close()
	var out map[string]any
	json.NewDecoder(resp.Body).Decode(&out)
	return resp.StatusCode, out
}

func TestStep18ProfilePhotos(t *testing.T) {
	old := photoGap
	photoGap = 0
	defer func() { photoGap = old }()
	e, ms := photoEnv(t)
	e.account("t_18ua", "user")
	e.account("t_18ub", "user")
	wa, wb := e.login("t_18ua"), e.login("t_18ub")

	// No login, a made up token, or a wrong method: refused and nothing stored.
	if c, _ := e.photoReq("POST", "", makeJPEG(200, 200)); c != 401 {
		t.Fatalf("no token: %d", c)
	}
	if c, _ := e.photoReq("POST", "not-a-token", makeJPEG(200, 200)); c != 401 {
		t.Fatalf("bad token: %d", c)
	}
	if c, _ := e.photoReq("DELETE", "", nil); c != 401 {
		t.Fatalf("unauthenticated delete: %d", c)
	}
	if ms.count() != 0 {
		t.Fatal("something was stored without a login")
	}

	// Validation: not an image, a disguised file, truncated, too big, too small, too many pixels, WebP.
	bad := map[string][]byte{
		"text":          []byte("<script>alert(1)</script>"),
		"html as image": append([]byte("GIF89a"), []byte("<html>")...),
		"truncated":     makeJPEG(300, 300)[:200],
		"tiny":          makeJPEG(20, 20),
		"webp":          append([]byte("RIFF\x10\x00\x00\x00WEBPVP8 "), make([]byte, 30)...),
	}
	for name, b := range bad {
		if c, o := e.photoReq("POST", wa.Token, b); c < 400 || o["e"] == nil {
			t.Fatalf("%s accepted: %d %v", name, c, o)
		}
	}
	if c, o := e.photoReq("POST", wa.Token, bytes.Repeat([]byte{0xff}, maxPhotoUpload+10)); c != 413 && c != 422 {
		t.Fatalf("oversize upload: %d %v", c, o)
	}
	if _, _, err := processPhoto(makeJPEG(4200, 100)); err == nil {
		t.Fatal("an image beyond the dimension limit passed")
	}
	if ms.count() != 0 {
		t.Fatal("a refused upload left files behind")
	}

	// A good JPEG and a transparent PNG are accepted and become two stored pictures with safe keys.
	c, o := e.photoReq("POST", wa.Token, makeJPEG(800, 500))
	if c != 200 || !strings.Contains(o["look"].(string), "~") {
		t.Fatalf("good jpeg: %d %v", c, o)
	}
	if ms.count() != 2 {
		t.Fatalf("expected 2 stored pictures, have %d", ms.count())
	}
	for k := range ms.data {
		if !validPhotoKey(k) || !strings.HasPrefix(k, "u") {
			t.Fatalf("unsafe key %q", k)
		}
	}
	// Served sizes: a thumbnail by default and the larger one on request, both real JPEGs of the right size.
	get := func(path string) (*http.Response, []byte) {
		resp, err := http.Get(e.photoSrv.URL + path)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		b, _ := io.ReadAll(resp.Body)
		return resp, b
	}
	r1, tb := get("/photo/t_18ua")
	cfg, fmtName, err := image.DecodeConfig(bytes.NewReader(tb))
	if r1.StatusCode != 200 || err != nil || fmtName != "jpeg" || cfg.Width != photoThumb {
		t.Fatalf("thumbnail: %d %v %v", r1.StatusCode, fmtName, err)
	}
	r2, mb := get("/photo/t_18ua?s=m&v=1")
	cfg, _, _ = image.DecodeConfig(bytes.NewReader(mb))
	if cfg.Width != photoMain || len(mb) <= len(tb) || !strings.Contains(r2.Header.Get("Cache-Control"), "immutable") {
		t.Fatalf("main picture: %d w=%d cc=%s", r2.StatusCode, cfg.Width, r2.Header.Get("Cache-Control"))
	}
	if r1.Header.Get("X-Content-Type-Options") != "nosniff" || r1.Header.Get("ETag") == "" {
		t.Fatal("missing safety or cache headers")
	}
	// Unknown handle, path tricks and a person without a photo are plain 404s.
	for _, p := range []string{"/photo/t_18ub", "/photo/nobody_here", "/photo/..%2f..%2fetc", "/photo/t_18ua%00"} {
		if r, _ := get(p); r.StatusCode != 404 {
			t.Fatalf("%s gave %d", p, r.StatusCode)
		}
	}

	// The look everyone is shown carries the photo version, in messages and in lookups.
	wb.send(map[string]any{"t": "thread", "h": "t_18ua"})
	if av := wb.expect("thread")["av"].(map[string]any); !strings.Contains(av["t_18ua"].(string), "~") {
		t.Fatalf("thread looks lack the photo: %v", av)
	}

	// Another person's token cannot touch it: B deleting removes only B's (nothing), A's stays.
	if c, _ := e.photoReq("DELETE", wb.Token, nil); c != 200 {
		t.Fatalf("delete of own (empty) photo: %d", c)
	}
	if r, _ := get("/photo/t_18ua"); r.StatusCode != 200 {
		t.Fatal("someone else's delete removed the photo")
	}

	// Replace: old files are removed, new ones stored, the version moves on.
	c, o2 := e.photoReq("POST", wa.Token, makePNG(300, 300))
	if c != 200 || o2["look"] == o["look"] || ms.count() != 2 {
		t.Fatalf("replace: %d %v files=%d", c, o2, ms.count())
	}
	// Remove: files gone, fallback (404), the look loses its photo part.
	if c, _ := e.photoReq("DELETE", wa.Token, nil); c != 200 || ms.count() != 0 {
		t.Fatalf("remove: %d files=%d", c, ms.count())
	}
	if r, _ := get("/photo/t_18ua"); r.StatusCode != 404 {
		t.Fatal("a removed photo is still served")
	}
	wb.send(map[string]any{"t": "thread", "h": "t_18ua"})
	if av, _ := wb.expect("thread")["av"].(map[string]any); av != nil && strings.Contains(fmt.Sprint(av["t_18ua"]), "~") {
		t.Fatalf("look still has a photo: %v", av)
	}
	// Storage trouble: a clean error, no half saved state.
	ms.fail = true
	if c, o := e.photoReq("POST", wa.Token, makeJPEG(300, 300)); c != 502 || o["e"] != "photo_storage" {
		t.Fatalf("storage down: %d %v", c, o)
	}
	ms.fail = false
	if e.count(`SELECT count(*) FROM users WHERE handle_key = 't_18ua' AND photo_key IS NOT NULL`) != 0 {
		t.Fatal("a failed upload saved a key")
	}

	// Account deletion takes the picture with it.
	e.photoReq("POST", wb.Token, makeJPEG(300, 300))
	id := mustID(t, e, "t_18ub")
	if _, err := e.st.deleteAccount(context.Background(), id); err != nil {
		t.Fatal(err)
	}
	if e.st.photoKeyByID(context.Background(), id) != "" {
		t.Fatal("deleted account kept a photo key")
	}

	// With storage off the upload endpoint says so, and the status tells the page to hide the button.
	e.h.photos = nil
	if c, o := e.photoReq("POST", wa.Token, makeJPEG(300, 300)); c != 503 || o["e"] != "photos_off" {
		t.Fatalf("photos off: %d %v", c, o)
	}
}

func TestStep18PhotoKeysAndStorageBackends(t *testing.T) {
	for _, k := range []string{"../x", "/abs", "u1/../u2/a", "u1//a", "u1/a b", "U1/A", "", "a"} {
		if validPhotoKey(k) {
			t.Fatalf("key %q should be refused", k)
		}
	}
	if !validPhotoKey("u12/ab34cd-m.jpg") {
		t.Fatal("a normal key was refused")
	}
	d := &dirStorage{root: t.TempDir()}
	ctx := context.Background()
	if err := d.Put(ctx, "../escape.jpg", []byte("x"), ""); err == nil {
		t.Fatal("path traversal accepted")
	}
	if err := d.Put(ctx, "u1/aa11bb22-t.jpg", []byte("hi"), ""); err != nil {
		t.Fatal(err)
	}
	if b, err := d.Get(ctx, "u1/aa11bb22-t.jpg"); err != nil || string(b) != "hi" {
		t.Fatal("dir storage round trip")
	}
	d.Delete(ctx, "u1/aa11bb22-t.jpg")
	if _, err := d.Get(ctx, "u1/aa11bb22-t.jpg"); err != errNoPhoto {
		t.Fatal("delete did not remove")
	}
	// The Supabase client against a stand-in server: right paths, the key only in headers, errors surfaced.
	var gotAuth, gotPath, gotMethod string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth, gotPath, gotMethod = r.Header.Get("Authorization"), r.URL.Path, r.Method
		if r.Method == "GET" {
			w.Write([]byte("img"))
		}
	}))
	defer srv.Close()
	s := &supabaseStorage{base: srv.URL, key: "svc-key", bucket: "avatars", hc: srv.Client()}
	if err := s.Put(ctx, "u1/aa11bb22-m.jpg", []byte("d"), "image/jpeg"); err != nil || gotMethod != "POST" || gotPath != "/storage/v1/object/avatars/u1/aa11bb22-m.jpg" || gotAuth != "Bearer svc-key" {
		t.Fatalf("supabase put: %v %s %s %s", err, gotMethod, gotPath, gotAuth)
	}
	if b, err := s.Get(ctx, "u1/aa11bb22-m.jpg"); err != nil || string(b) != "img" {
		t.Fatal("supabase get")
	}
	if err := s.Put(ctx, "../bad", []byte("d"), "image/jpeg"); err == nil {
		t.Fatal("supabase accepted a bad key")
	}
	t.Setenv("PHOTO_STORAGE", "")
	if photoStorageFromEnv() != nil {
		t.Fatal("photos must be off unless configured")
	}
}

func TestStep18InboxOrderUnreadAndIsolation(t *testing.T) {
	e := newTestEnv(t)
	for _, h := range []string{"t_18ia", "t_18ib", "t_18ic", "t_18id"} {
		e.account(h, "user")
	}
	e.st.db.Exec(`UPDATE users SET credits = 500`)
	wa, wb, wc, wd := e.login("t_18ia"), e.login("t_18ib"), e.login("t_18ic"), e.login("t_18id")
	// b writes to a, then c writes to a, then b again: b is the latest conversation, c second.
	for _, s := range []struct {
		w    *wsc
		from string
	}{{wb, "t_18ib"}, {wc, "t_18ic"}, {wb, "t_18ib"}} {
		e.refill()
		s.w.send(e.dmFrame(s.from, "t_18ia", "hello "+s.from))
		wa.expect("dm")
		time.Sleep(5 * time.Millisecond)
	}
	wa.send(map[string]any{"t": "inbox"})
	inb := wa.expect("inbox")
	rows := inb["c"].([]any)
	if len(rows) != 2 || rows[0].(map[string]any)["h"] != "t_18ib" || rows[1].(map[string]any)["h"] != "t_18ic" {
		t.Fatalf("order by latest message: %v", rows)
	}
	if rows[0].(map[string]any)["u"] != float64(2) || rows[1].(map[string]any)["u"] != float64(1) {
		t.Fatalf("unread counts: %v", rows)
	}
	// The server never sees plaintext: the row carries an envelope and no body text.
	if r0 := rows[0].(map[string]any); r0["e"] == nil || (r0["b"] != nil && r0["b"] != "") {
		t.Fatalf("inbox row leaks or lacks the envelope: %v", r0)
	}
	// Opening the conversation reads it: that row's unread drops to zero, the other keeps its count.
	wa.send(map[string]any{"t": "thread", "h": "t_18ib"})
	wa.expect("thread")
	wa.send(map[string]any{"t": "inbox"})
	rows = wa.expect("inbox")["c"].([]any)
	if rows[0].(map[string]any)["u"] != nil && rows[0].(map[string]any)["u"] != float64(0) || rows[1].(map[string]any)["u"] != float64(1) {
		t.Fatalf("unread after opening: %v", rows)
	}
	// Someone not in those conversations sees nothing of them.
	wd.send(map[string]any{"t": "inbox"})
	if got := wd.expect("inbox")["c"]; got != nil && len(got.([]any)) != 0 {
		t.Fatalf("another account sees conversations: %v", got)
	}
	// A blocked person drops out of the list.
	wa.send(map[string]any{"t": "blk", "h": "t_18ic", "a": "add"})
	time.Sleep(100 * time.Millisecond)
	wa.drain()
	wa.send(map[string]any{"t": "inbox"})
	rows = wa.expect("inbox")["c"].([]any)
	if len(rows) != 1 || rows[0].(map[string]any)["h"] != "t_18ib" {
		t.Fatalf("blocked conversation still listed: %v", rows)
	}
}

func TestStep18RoomDiscoveryPrivacyAndOfficialJoinedState(t *testing.T) {
	e := newTestEnv(t)
	host := e.account("t_18rh", "user")
	e.account("t_18ro", "user")
	e.st.db.Exec(`UPDATE users SET points = 500 WHERE id = $1`, host.ID)
	e.st.applyCredit(context.Background(), host.ID, "grant", 100, "g18rh", "")
	wh, wo := e.login("t_18rh"), e.login("t_18ro")
	wh.send(map[string]any{"t": "croom", "n": "T18 Secret", "k": "private", "d": "hidden room"})
	secret := wh.expect("croomok")["s"].(string)
	e.st.db.Exec(`UPDATE users SET points = 100000 WHERE id = $1`, host.ID)
	e.h.mu.Lock()
	e.h.users[host.ID].points, e.h.users[host.ID].tier = 100000, tierFor(100000)
	e.h.mu.Unlock()
	if got := wh.ask(map[string]any{"t": "croom", "n": "T18 Passworded", "k": "public", "p": "open-sesame-1"}, "croomok"); got != "ok:croomok" {
		t.Fatalf("second room: %s", got)
	}
	var pw string
	e.st.db.QueryRow(`SELECT slug FROM rooms WHERE name = 'T18 Passworded'`).Scan(&pw)

	// Outsiders never find them: not in the directory, not in search, not in their room list.
	for _, slug := range []string{secret, pw} {
		if roomEntry(wo, slug) != nil {
			t.Fatalf("%s leaked into the directory", slug)
		}
	}
	wo.drain()
	wo.send(map[string]any{"t": "find", "n": "t18"})
	for _, r := range wo.expect("found")["r"].([]any) {
		if n := r.(map[string]any)["s"]; n == secret || n == pw {
			t.Fatal("hidden room found by search")
		}
	}
	wo.send(map[string]any{"t": "dir"})
	wo.expect("dir")
	// Entering without the code or password changes nothing: no join, no notice, no membership.
	if got := wo.ask(map[string]any{"t": "join", "r": secret}, "joined"); got != "err:needs_invite" {
		t.Fatalf("private without code: %s", got)
	}
	if got := wo.ask(map[string]any{"t": "join", "r": pw, "c": "WRONGCDE"}, "joined"); got != "err:needs_invite" {
		t.Fatalf("password room without code: %s", got)
	}
	if e.count(`SELECT count(*) FROM room_members m JOIN users u ON u.id = m.user_id WHERE u.handle_key = 't_18ro'`) != 0 {
		t.Fatal("a refused join left a membership")
	}

	// An official room is "joined" while you are in it, as the server says it, and not for others.
	wo.send(map[string]any{"t": "join", "r": "lobby"})
	if jf := wo.expect("joined"); jf["j"] != true {
		t.Fatalf("official room join not confirmed: %v", jf)
	}
	wo.send(map[string]any{"t": "leave", "a": "x"})
	wo.expect("left")
	rooms := wo.expect("rooms")["r"].([]any)
	for _, r := range rooms {
		if m := r.(map[string]any); m["s"] == "lobby" && m["j"] == true {
			t.Fatal("official room still joined after leaving")
		}
	}
}
