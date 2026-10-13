package main

import (
	"context"
	"errors"
	"log"
	"my_kbot/internal/api"
	"my_kbot/internal/config"
	"my_kbot/internal/platform/iam"
	"net/http"
	"os/signal"
	"syscall"
	"time"
)

func main() {

	//读取配置
	cfg := config.Load()
	if err := cfg.Validate(); err != nil {
		log.Fatal(err)
	}

	iamService := iam.New(
		iam.NewMemoryStore(),
		cfg.JWTSecret,
		cfg.JWTIssuer,
	)

	server := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           api.NewRouter(iamService),
		ReadHeaderTimeout: 5 * time.Second,
	}

	ctx, stop := signal.NotifyContext(
		context.Background(),
		syscall.SIGINT,
		syscall.SIGTERM,
	)

	defer stop()
	shutdownDone := make(chan struct{})

	go func() {
		<-ctx.Done()
		log.Println("shutting down server")

		shutdownCtx, cancel := context.WithTimeout(
			context.Background(),
			5*time.Second,
		)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			log.Printf("server shuotdown: %v", err)
		}
		close(shutdownDone)

	}()

	log.Printf("server listening on %s", cfg.HTTPAddr)
	if err := server.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
		log.Fatal(err)
	}

	<-shutdownDone
	log.Println("server stopped")
}
