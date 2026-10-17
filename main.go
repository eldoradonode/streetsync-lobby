// Lobby server.
//
//	Steps 1 and 2: handles, resume after a dropped connection, one public room.
//	Step 3: Postgres storage, optional passwords, stars and tier badges.
package main

import (
	"context"
	_ "embed"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"
)

//go:embed web/index.html
var indexHTML []byte

func envInt(name string, def int) int {
	if v := strings.TrimSpace(os.Getenv(name)); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return n
		}
		log.Printf("ignoring %s=%q, using %d", name, v, def)
	}
	return def
}

func main() {
	dbURL := strings.TrimSpace(os.Getenv("DATABASE_URL"))
	if dbURL == "" {
		log.Fatal("DATABASE_URL is not set. Use the Supabase session pooler connection string. See README.md.")
	}
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	var origins []string
	for _, o := range strings.Split(os.Getenv("ALLOWED_ORIGINS"), ",") {
		if o = strings.TrimSpace(o); o != "" {
			origins = append(origins, o)
		}
	}

	sigCtx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	st, err := openStore(sigCtx, dbURL, envInt("SESSION_DAYS", 30))
	if err != nil {
		log.Fatalf("database: %v", err)
	}
	defer st.close()

	auth := newAuthSvc(envInt("HASH_ITERATIONS", defaultHashIters))
	h := newHub(st, auth, origins,
		time.Duration(envInt("FLUSH_MS", 5000))*time.Millisecond,
		envInt("HISTORY_DAYS", 7), envInt("GUEST_DAYS", 14))
	h.dmDays = envInt("DM_DAYS", 30)
	h.requirePass = strings.TrimSpace(os.Getenv("REQUIRE_PASSWORD")) != "0"
	h.roomIdle = time.Duration(envInt("ROOM_IDLE_DAYS", 3)) * 24 * time.Hour
	h.firstCleanup = time.Duration(envInt("FIRST_CLEANUP_SECONDS", 60)) * time.Second
	if err := h.load(sigCtx); err != nil {
		log.Fatalf("loading history: %v", err)
	}

	if ah := strings.TrimSpace(os.Getenv("ADMIN_HANDLE")); ah != "" {
		handle, key, ok := normHandle(ah)
		if !ok {
			log.Fatal("ADMIN_HANDLE must be 3 to 16 letters, numbers or underscores")
		}
		created, adopted, err := st.ensureAdmin(sigCtx, handle, key, func() (string, error) {
			pw := os.Getenv("ADMIN_PASSWORD")
			if len(pw) < minPassBytes {
				return "", errors.New("ADMIN_PASSWORD (at least 8 characters) is needed to set up the admin account")
			}
			return auth.hash(sigCtx, pw)
		})
		if err != nil {
			log.Fatalf("admin account: %v", err)
		}
		log.Printf("admin account @%s ready (created=%v adopted=%v)", handle, created, adopted)
	}

	hubCtx, hubStop := context.WithCancel(context.Background())
	persistDone := make(chan struct{})
	go h.persistLoop(hubCtx, persistDone)
	go h.run(hubCtx)

	mux := http.NewServeMux()
	mux.HandleFunc("/ws", h.serveWS)
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte("ok")) })
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-cache")
		_, _ = w.Write(indexHTML)
	})

	srv := &http.Server{Addr: ":" + port, Handler: mux, ReadHeaderTimeout: 10 * time.Second}
	serverDone := make(chan struct{})
	go func() {
		defer close(serverDone)
		<-sigCtx.Done()
		sctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(sctx)
	}()

	log.Printf("lobby listening on :%s", port)
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatal(err)
	}
	<-serverDone

	// Save everything that is still waiting before the process ends.
	hubStop()
	<-persistDone
	fctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	h.flushUsers(fctx)
	cancel()
	log.Printf("stopped cleanly")
}
