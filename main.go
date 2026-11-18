// quiver-rss polls the Quiver Launcher catalog for recomps and serves them as
// an RSS 2.0 feed on every HTTP path.
package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"
)

type config struct {
	Addr         string
	PollInterval time.Duration
	FeedSize     int
	Sort         string // "added" (new recomps) or "updated" (new releases)
	SiteURL      string
	APIURL       string
	UserAgent    string
}

func loadConfig() config {
	c := config{
		Addr:         ":" + env("PORT", "8080"),
		PollInterval: 30 * time.Minute,
		FeedSize:     50,
		Sort:         env("SORT", "added"),
		SiteURL:      env("SITE_URL", "https://quiverlauncher.com"),
		APIURL:       env("API_URL", "https://api.quiverlauncher.com/api/v1"),
		UserAgent:    env("USER_AGENT", "quiver-rss/1.0 (+https://github.com/ricardo-duarte-av/quiver-rss)"),
	}
	if v := os.Getenv("POLL_INTERVAL"); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil || d < time.Minute {
			log.Fatalf("invalid POLL_INTERVAL %q (use e.g. 30m, 1h; minimum 1m)", v)
		}
		c.PollInterval = d
	}
	if v := os.Getenv("FEED_SIZE"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 || n > 500 {
			log.Fatalf("invalid FEED_SIZE %q (1-500)", v)
		}
		c.FeedSize = n
	}
	if c.Sort != "added" && c.Sort != "updated" {
		log.Fatalf("invalid SORT %q (added|updated)", c.Sort)
	}
	return c
}

func env(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func main() {
	cfg := loadConfig()
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	svc := newService(cfg)
	go svc.run(ctx)

	srv := &http.Server{
		Addr:              cfg.Addr,
		Handler:           http.HandlerFunc(svc.serveFeed),
		ReadHeaderTimeout: 10 * time.Second,
	}
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdownCtx)
	}()

	log.Printf("listening on %s (poll every %s, %d items, sort=%s)", cfg.Addr, cfg.PollInterval, cfg.FeedSize, cfg.Sort)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatal(err)
	}
}
