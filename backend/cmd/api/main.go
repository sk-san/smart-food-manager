package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/example/food-app/backend/internal/config"
	"github.com/example/food-app/backend/internal/logging"
	"github.com/example/food-app/backend/internal/server"
	"github.com/example/food-app/backend/internal/store"
	"github.com/example/food-app/backend/internal/telemetry"
	"go.opentelemetry.io/contrib/bridges/otelslog"
)

func main() {
	cfg := config.Load()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Identity stamped on every structured log entry.
	logging.SetService(cfg.ServiceName, cfg.ServiceVersion, cfg.Environment)
	logging.SetHashSalt(cfg.LogHashSalt)

	// Bootstrap OTel before anything else so all log records are exported.
	// Falls back to stderr logging if the collector is unreachable.
	otelShutdown, err := telemetry.Setup(ctx, cfg.ServiceName, cfg.ServiceVersion, cfg.Environment)
	if err != nil {
		slog.Warn("telemetry unavailable, falling back to stderr", "error", err)
	} else {
		slog.SetDefault(otelslog.NewLogger(cfg.ServiceName))
		defer func() {
			shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if err := otelShutdown(shutdownCtx); err != nil {
				slog.Error("telemetry shutdown", "error", err)
			}
		}()
	}

	pool, err := store.NewPool(ctx, cfg.DatabaseURL)
	if err != nil {
		slog.Error("database connect failed", "error", err)
		os.Exit(1)
	}
	defer pool.Close()

	srv := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           server.New(cfg, pool),
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		slog.Info("server started", "addr", srv.Addr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("server error", "error", err)
			os.Exit(1)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop

	slog.Info("shutting down")
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		slog.Error("http shutdown", "error", err)
	}
}
