// Command server runs the splitfriends API and serves the embedded SPA.
package main

import (
	"context"
	"errors"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path"
	"strings"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"splitfriends/internal/admin"
	"splitfriends/internal/api"
	"splitfriends/internal/auth"
	"splitfriends/internal/clientip"
	"splitfriends/internal/config"
	"splitfriends/internal/db"
	"splitfriends/internal/push"
	"splitfriends/internal/realtime"
	"splitfriends/web"
)

func main() {
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, nil)))
	cfg := config.Load()

	if err := os.MkdirAll(cfg.DataDir, 0o755); err != nil {
		slog.Error("data dir", "err", err)
		os.Exit(1)
	}
	d, err := db.Open(cfg.DataDir)
	if err != nil {
		slog.Error("open db", "err", err)
		os.Exit(1)
	}
	defer d.Close()

	a, err := auth.New(d, cfg.RPID, cfg.AppName, cfg.Origins)
	if err != nil {
		slog.Error("auth", "err", err)
		os.Exit(1)
	}
	p, err := push.New(context.Background(), d, cfg.PushEmail)
	if err != nil {
		slog.Error("push", "err", err)
		os.Exit(1)
	}
	hub := realtime.NewHub()
	srv := api.New(cfg, d, a, hub, p)

	r := chi.NewRouter()
	r.Use(clientip.Middleware(cfg.TrustProxyHeaders), middleware.Recoverer)
	if cfg.Dev {
		r.Use(middleware.Logger)
	}
	r.Get("/healthz", func(w http.ResponseWriter, _ *http.Request) { w.Write([]byte("ok")) })
	r.Mount("/api", srv.Routes())
	// The admin dashboard only exists when ADMIN_PASSWORD is set; otherwise
	// /admin falls through to the SPA like any unknown path.
	if adm := admin.New(cfg, d); adm != nil {
		r.Mount("/admin", adm.Routes())
		slog.Info("admin dashboard enabled")
	}
	r.NotFound(spaHandler())

	httpSrv := &http.Server{
		Addr:              cfg.Addr,
		Handler:           r,
		ReadHeaderTimeout: 10 * time.Second,
		// No WriteTimeout: SSE streams are long-lived.
		IdleTimeout: 120 * time.Second,
	}
	go func() {
		slog.Info("listening", "addr", cfg.Addr, "rp_id", cfg.RPID, "origins", cfg.Origins)
		if err := httpSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("serve", "err", err)
			os.Exit(1)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = httpSrv.Shutdown(ctx)
}

// spaHandler serves the embedded Vite build. Hashed assets get long cache
// headers; everything else falls back to index.html for client-side routing.
func spaHandler() http.HandlerFunc {
	dist, err := fs.Sub(web.Dist, "dist")
	if err != nil {
		panic(err)
	}
	fileServer := http.FileServer(http.FS(dist))
	index, _ := fs.ReadFile(dist, "index.html")
	return func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") {
			http.NotFound(w, r)
			return
		}
		p := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
		if p == "" {
			p = "index.html"
		}
		if f, err := dist.Open(p); err == nil {
			f.Close()
			if strings.HasPrefix(p, "assets/") {
				w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
			} else {
				w.Header().Set("Cache-Control", "no-cache")
			}
			if p == "sw.js" {
				w.Header().Set("Service-Worker-Allowed", "/")
			}
			fileServer.ServeHTTP(w, r)
			return
		}
		if len(index) == 0 {
			http.Error(w, "frontend not built", http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-cache")
		_, _ = w.Write(index)
	}
}
