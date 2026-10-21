package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"my_kbot_final/internal/api"
	"my_kbot_final/internal/config"
	"my_kbot_final/internal/infrastructure/postgres"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	pgstore "my_kbot_final/internal/infrastructure/postgres/sqlc"
	"my_kbot_final/internal/platform/iam"
)

func main() {
	if err := run(); err != nil {
		log.Fatalf("run server: %v", err)
	}
}

func run() error {
	cfg := config.Load()
	if err := cfg.Validate(); err != nil {
		return fmt.Errorf("validate config %w", err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	databaseCtx, cancelDatabase := context.WithTimeout(ctx, 10*time.Second)
	pool, err := postgres.Open(databaseCtx, cfg.DatabaseURL)
	defer cancelDatabase()
	if err != nil {
		return fmt.Errorf("open database %w", err)
	}
	defer pool.Close()

	queries := pgstore.New(pool)
	iamStore := iam.NewPostgresStore(pool, queries)
	iamService := iam.NewService(iamStore, cfg.JWTKeyBytes())

	if cfg.AutoseedAdmin {
		if err := iamService.EnsureSeedAdmin(ctx, cfg.AutoseedAdminEmail, cfg.AutoseedAdminPassword); err != nil {
			log.Printf("autoseed amin:%v", err)
		} else {
			log.Printf("admin ready: %s", cfg.AutoseedAdminEmail)
		}
	}

	handler := (&api.Handler{}).SetReadiness(api.ReadinessCheck{
		Name:  "postgres",
		Check: pool.Ping,
	})

	server := &http.Server{
		Addr:              cfg.Addr,
		Handler:           handler.Routes(),
		ReadHeaderTimeout: 10 * time.Second,
	}

	serverErrors := make(chan error, 1)
	go func() {
		serverErrors <- server.ListenAndServe()
	}()
	log.Printf("server starting on %s", cfg.Addr)

	select {
	case err := <-serverErrors:
		if !errors.Is(err, http.ErrServerClosed) {
			return fmt.Errorf("server HTTP: %w", err)
		}
		return nil

	case <-ctx.Done():
		log.Println("shutting down...")
	}

	shutdownCtx, cancelShutdown := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancelShutdown()

	if err := server.Shutdown(shutdownCtx); err != nil {
		_ = server.Close()
		return fmt.Errorf("shutdown HTTP: %w", err)
	}
	log.Println("server stopped")
	return nil

}
