package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"future-assessment/internal/httpapi"
	"future-assessment/internal/store"
)

const shutdownTimeout = 10 * time.Second

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := run(ctx); err != nil {
		slog.Error("fatal", "err", err)
		os.Exit(1)
	}
}

func run(ctx context.Context) error {
	cfg, err := loadConfig()
	if err != nil {
		return err
	}

	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: cfg.logLevel}))
	slog.SetDefault(logger)

	db, err := store.New(ctx, cfg.databaseURL)
	if err != nil {
		return fmt.Errorf("connect to database: %w", err)
	}
	defer db.Close()

	if err := db.Migrate(ctx); err != nil {
		return err
	}
	logger.Info("database ready")

	srv := &http.Server{
		Addr:              net.JoinHostPort("", cfg.port),
		Handler:           httpapi.NewRouter(logger),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	serveErr := make(chan error, 1)
	go func() {
		logger.Info("server listening", "addr", srv.Addr)
		serveErr <- srv.ListenAndServe()
	}()

	select {
	case err := <-serveErr:
		return fmt.Errorf("serve: %w", err)
	case <-ctx.Done():
	}

	logger.Info("shutting down")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("shutdown: %w", err)
	}
	if err := <-serveErr; !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("serve: %w", err)
	}

	logger.Info("shutdown complete")
	return nil
}

type config struct {
	port        string
	logLevel    slog.Level
	databaseURL string
}

func loadConfig() (config, error) {
	cfg := config{
		port:        envOr("PORT", "8080"),
		logLevel:    slog.LevelInfo,
		databaseURL: os.Getenv("DATABASE_URL"),
	}

	if cfg.databaseURL == "" {
		return config{}, errors.New("DATABASE_URL is required")
	}

	if raw, ok := os.LookupEnv("LOG_LEVEL"); ok {
		if err := cfg.logLevel.UnmarshalText([]byte(raw)); err != nil {
			return config{}, fmt.Errorf("invalid LOG_LEVEL %q: %w", raw, err)
		}
	}

	return cfg, nil
}

func envOr(key, fallback string) string {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		return v
	}
	return fallback
}
