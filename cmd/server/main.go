package main

import (
	"context"
	"errors"
	"log"
	"my_kbot/internal/api"
	"my_kbot/internal/config"
	"my_kbot/internal/domain"
	"my_kbot/internal/platform"
	"my_kbot/internal/platform/iam"
	platformtool "my_kbot/internal/platform/tool"
	"my_kbot/internal/runtime/engine"
	"my_kbot/internal/runtime/llm"
	"my_kbot/internal/runtime/sandbox"
	"my_kbot/internal/runtime/tooling"
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

	controlPlane := platform.New()
	controlPlane.PutSnapshot(&engine.AgentSnapshot{
		AgentVersionID: "demo-v1",
		AgentID:        "demo",
		SystemPrompt:   "你是kbot课堂助手",
		MaxSteps:       4,
	})
	controlPlane.PutConversation(&domain.Conversation{
		ID:             "demo-conversation",
		AgentID:        "demo",
		AgentVersionID: "demo-v1",
	})

	iamService := iam.New(
		iam.NewMemoryStore(),
		cfg.JWTSecret,
		cfg.JWTIssuer,
	)

	gateway, err := llm.NewGateway(cfg)
	if err != nil {
		log.Fatalf("create LLM gatway : %v", err)
	}

	runtime := engine.New(controlPlane, gateway)
	toolRegistry := platformtool.NewRegistry()

	sandboxClient, err := sandbox.NewClient(cfg.SandboxRunnerURL, cfg.SandboxRunnerToken)
	if err != nil {
		log.Fatalf("create sandbox runner clinet: %v", err)
	}
	toolExecutor := tooling.NewExecutor(
		toolRegistry,
		nil,
		"crossborder-sim",
		"localhost",
		"127.0.0.1",
	).WithSandbox(sandboxClient)

	runtime.WithTools(toolExecutor)
	cp := api.ControlPlane{Tools: toolRegistry}

	server := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           api.NewRouterWithControlPlane(iamService, runtime, cp),
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
