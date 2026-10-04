// Command alertmanager-proxy fronts one Alertmanager: it translates the removed
// API v1 to v2 for legacy clients and passes everything else through.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"alertmanager-proxy/internal/config"
	"alertmanager-proxy/internal/server"
)

// version is set at build time via -ldflags "-X main.version=...".
var version = "dev"

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "alertmanager-proxy:", err)
		os.Exit(1)
	}
}

func run() error {
	configPath := flag.String("config", "", "path to YAML config file (optional; AMP_* env vars override it)")
	showVersion := flag.Bool("version", false, "print version and exit")
	flag.Parse()
	if *showVersion {
		fmt.Println(version)
		return nil
	}

	logger := slog.New(slog.NewJSONHandler(os.Stderr, nil))
	cfg, err := config.Load(*configPath, os.Getenv)
	if err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	servers := []*http.Server{
		newServer(cfg.ListenAddress, server.Handler(cfg, logger), logger),
		newServer(cfg.AdminListenAddress, server.AdminHandler(), logger),
	}
	errc := make(chan error, len(servers))
	for _, srv := range servers {
		ln, err := (&net.ListenConfig{}).Listen(ctx, "tcp", srv.Addr)
		if err != nil {
			return err
		}
		go func() { errc <- srv.Serve(ln) }()
	}
	logger.Info("started", "version", version, "listen", cfg.ListenAddress,
		"admin_listen", cfg.AdminListenAddress, "upstream", cfg.Upstream.URL)

	select {
	case <-ctx.Done():
	case err := <-errc:
		if !errors.Is(err, http.ErrServerClosed) {
			logger.Error("server failed", "error", err)
		}
	}

	logger.Info("shutting down", "timeout", cfg.ShutdownTimeout)
	shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
	defer cancel()
	var errs []error
	for _, srv := range servers {
		errs = append(errs, srv.Shutdown(shutdownCtx))
	}
	return errors.Join(errs...)
}

func newServer(addr string, h http.Handler, logger *slog.Logger) *http.Server {
	return &http.Server{
		Addr:              addr,
		Handler:           h,
		ReadHeaderTimeout: 10 * time.Second,
		ErrorLog:          slog.NewLogLogger(logger.Handler(), slog.LevelWarn),
	}
}
