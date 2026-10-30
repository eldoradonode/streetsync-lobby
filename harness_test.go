package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
)

// An in-process server on the real hub and the real WebSocket handler, backed by the test database.
// Tests talk to it the way the page does, so they exercise the same code a hostile client would reach.

type testEnv struct {
	t    *testing.T
	st   *store
	h    *hub
	srv  *httptest.Server
	auth *authSvc

	clients map[string]*testClient // simulated browsers with their own keys, by handle

	photoSrv *httptest.Server // the photo routes, when a test turns photos on
}

func newTestEnv(t *testing.T) *testEnv {
	t.Helper()
	st := testStore(t)
	auth := newAuthSvc(1000)
	h := newHub(st, auth, []string{"*"}, time.Second, 7, 14)
	if err := h.load(context.Background()); err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/ws", h.serveWS)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return &testEnv{t: t, st: st, h: h, srv: srv, auth: auth}
}

// account makes a password account directly in the database, with a role.
func (e *testEnv) account(handle, role string) userRow {
	e.t.Helper()
	hash, err := hashPassword("password-"+handle, 1000)
	if err != nil {
		e.t.Fatal(err)
	}
	u, created, err := e.st.createAccount(context.Background(), handle, strings.ToLower(handle), "", hash)
	if err != nil || !created {
		e.t.Fatalf("account %s: %v created=%v", handle, err, created)
	}
	// Skip the asynchronous welcome credit so ledger counts in tests are stable.
	if _, err := e.st.db.Exec(`UPDATE users SET welcomed = true WHERE id = $1`, u.ID); err != nil {
		e.t.Fatal(err)
	}
	if role != "user" {
		if err := e.st.setRole(context.Background(), u.ID, role); err != nil {
			e.t.Fatal(err)
		}
		u.Role = role
	}
	return u
}

type wsc struct {
	t      *testing.T
	c      *websocket.Conn
	ctx    context.Context
	frames chan map[string]any
	closed chan struct{}
	Token  string
}

func (e *testEnv) dialRaw() *wsc {
	e.t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	e.t.Cleanup(cancel)
	c, _, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(e.srv.URL, "http")+"/ws", nil)
	if err != nil {
		e.t.Fatal(err)
	}
	w := &wsc{t: e.t, c: c, ctx: ctx, frames: make(chan map[string]any, 512), closed: make(chan struct{})}
	go func() {
		defer close(w.closed)
		for {
			_, data, err := c.Read(ctx)
			if err != nil {
				return
			}
			var m map[string]any
			if json.Unmarshal(data, &m) == nil {
				w.frames <- m
			}
		}
	}()
	e.t.Cleanup(func() { _ = c.CloseNow() })
	return w
}

func (w *wsc) send(m map[string]any) {
	w.t.Helper()
	b, _ := json.Marshal(m)
	if err := w.c.Write(w.ctx, websocket.MessageText, b); err != nil {
		w.t.Fatalf("send %v: %v", m["t"], err)
	}
}

// expect returns the next frame of a type, skipping others. It fails on timeout.
func (w *wsc) expect(typ string) map[string]any {
	w.t.Helper()
	tm := time.After(6 * time.Second)
	for {
		select {
		case f := <-w.frames:
			if f["t"] == typ {
				return f
			}
		case <-tm:
			w.t.Fatalf("no %q frame arrived", typ)
		case <-w.closed:
			// drain what is left
			select {
			case f := <-w.frames:
				if f["t"] == typ {
					return f
				}
				continue
			default:
			}
			w.t.Fatalf("connection closed before a %q frame", typ)
		}
	}
}

// outcome sends a frame and returns "ok:<type>" for the wanted success type or "err:<code>" for an error.
func (w *wsc) ask(m map[string]any, okTypes ...string) string {
	w.t.Helper()
	w.drain()
	w.send(m)
	tm := time.After(6 * time.Second)
	for {
		select {
		case f := <-w.frames:
			if f["t"] == "err" {
				return "err:" + f["c"].(string)
			}
			for _, o := range okTypes {
				if f["t"] == o {
					return "ok:" + o
				}
			}
		case <-tm:
			return "timeout"
		case <-w.closed:
			return "closed"
		}
	}
}

func (w *wsc) drain() {
	for {
		select {
		case <-w.frames:
		default:
			return
		}
	}
}

// isClosed waits briefly for the server to close the connection.
func (w *wsc) isClosed() bool {
	select {
	case <-w.closed:
		return true
	case <-time.After(3 * time.Second):
		return false
	}
}

func (e *testEnv) login(handle string) *wsc {
	e.t.Helper()
	w := e.dialRaw()
	w.send(map[string]any{"t": "login", "h": handle, "p": "password-" + handle})
	ok := w.expect("ok")
	w.Token, _ = ok["k"].(string)
	return w
}

func (e *testEnv) guest(handle string) *wsc {
	e.t.Helper()
	w := e.dialRaw()
	w.send(map[string]any{"t": "hi", "h": handle})
	ok := w.expect("ok")
	w.Token, _ = ok["k"].(string)
	return w
}

func (e *testEnv) count(q string, args ...any) int {
	e.t.Helper()
	var n int
	if err := e.st.db.QueryRow(q, args...).Scan(&n); err != nil {
		e.t.Fatal(err)
	}
	return n
}

// say posts a message in lobby and returns its id as seen by an observer.
func (w *wsc) join(room string) {
	w.t.Helper()
	w.send(map[string]any{"t": "join", "r": room})
	w.expect("joined")
}

func (w *wsc) say(body string) int64 {
	w.t.Helper()
	w.send(map[string]any{"t": "m", "b": body})
	for {
		f := w.expect("m")
		if f["b"] == body {
			return int64(f["i"].(float64))
		}
	}
}

func (w *wsc) sawMessage(body string) int64 {
	w.t.Helper()
	for {
		f := w.expect("m")
		if f["b"] == body {
			return int64(f["i"].(float64))
		}
	}
}

func mustID(t *testing.T, e *testEnv, handle string) int64 {
	t.Helper()
	var id int64
	if err := e.st.db.QueryRow(`SELECT id FROM users WHERE handle_key = lower($1)`, handle).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

func mustJSON(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}
