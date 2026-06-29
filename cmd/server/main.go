package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/linq/mvp/internal/api"
	"github.com/linq/mvp/internal/db"
	"github.com/linq/mvp/internal/ledger"
)

func main() {
	log := slog.New(slog.NewTextHandler(os.Stdout, nil))

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	// Postgres
	database, err := db.Connect(ctx)
	if err != nil {
		log.Error("postgres connect failed", "err", err)
		os.Exit(1)
	}
	defer database.Close()
	log.Info("postgres connected")

	// Run migration
	migrationSQL, err := os.ReadFile("migrations/001_init.sql")
	if err != nil {
		log.Error("read migration failed", "err", err)
		os.Exit(1)
	}
	if err := database.RunMigration(ctx, string(migrationSQL)); err != nil {
		log.Error("migration failed", "err", err)
		os.Exit(1)
	}
	log.Info("migration applied")

	// Seed the single MVP business row (upserts on startup so env changes propagate)
	webhookURL := os.Getenv("MVP_PUBLIC_URL")
	if webhookURL == "" {
		webhookURL = "http://localhost:8080"
	}
	webhookURL += "/webhook/deposit"

	webhookSecret := os.Getenv("WEBHOOK_SECRET")
	if webhookSecret == "" {
		log.Error("WEBHOOK_SECRET env var not set")
		os.Exit(1)
	}

	if err := database.SeedBusiness(ctx, webhookURL, webhookSecret); err != nil {
		log.Error("seed business failed", "err", err)
		os.Exit(1)
	}
	log.Info("business row seeded", "webhook_url", webhookURL)

	// TigerBeetle
	tb, err := ledger.NewClient()
	if err != nil {
		log.Error("tigerbeetle connect failed", "err", err)
		os.Exit(1)
	}
	defer tb.Close()
	log.Info("tigerbeetle connected")

	// HTTP server
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	h := api.NewHandler(database, tb)
	srv := &http.Server{
		Addr:         fmt.Sprintf(":%s", port),
		Handler:      h.Routes(),
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
	}

	go func() {
		log.Info("http server started", "port", port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Error("http server error", "err", err)
		}
	}()

	<-ctx.Done()
	log.Info("shutting down")
	shutCtx, shutCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutCancel()
	srv.Shutdown(shutCtx)
}
