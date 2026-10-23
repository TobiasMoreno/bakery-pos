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

	"bakery-pos/backend/internal/auth"
	"bakery-pos/backend/internal/catalog"
	"bakery-pos/backend/internal/config"
	"bakery-pos/backend/internal/database"
	"bakery-pos/backend/internal/httpapi"
	"bakery-pos/backend/internal/inventory"
	"bakery-pos/backend/internal/pos"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(logger)

	cfg, err := config.Load()
	if err != nil {
		logger.Error("invalid configuration", "error", err)
		os.Exit(1)
	}

	startupCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	pool, err := database.Open(startupCtx, cfg.DatabaseURL)
	if err != nil {
		logger.Error("database unavailable", "error", err)
		os.Exit(1)
	}
	defer pool.Close()

	authRepository := auth.NewPostgresRepository(pool)
	authService := auth.NewService(authRepository, cfg.SessionTTL)
	catalogRepository := catalog.NewPostgresRepository(pool)
	catalogService := catalog.NewService(catalogRepository)
	inventoryRepository := inventory.NewPostgresRepository(pool)
	inventoryService := inventory.NewService(inventoryRepository)
	posRepository := pos.NewPostgresRepository(pool)
	posService := pos.NewService(posRepository, cfg.OfflineSigningKey)
	handler := httpapi.NewServer(authService, pool, httpapi.Options{
		AllowedOrigin: cfg.AllowedOrigin,
		CookieSecure:  cfg.CookieSecure,
		Logger:        logger,
		Catalog:       catalogService,
		Inventory:     inventoryService,
		POS:           posService,
	}).Routes()
	if cfg.FrontendDir != "" {
		handler, err = httpapi.WithFrontend(handler, cfg.FrontendDir)
		if err != nil {
			logger.Error("frontend unavailable", "error", err)
			os.Exit(1)
		}
	}

	server := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	shutdownCtx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go func() {
		logger.Info("api listening", "addr", cfg.HTTPAddr, "environment", cfg.Environment)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error("api stopped unexpectedly", "error", err)
			stop()
		}
	}()

	<-shutdownCtx.Done()
	ctx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()
	if err := server.Shutdown(ctx); err != nil {
		logger.Error("api shutdown failed", "error", err)
	}
}
