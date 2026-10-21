package main

import (
	"bytes"
	"context"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"golang.org/x/crypto/bcrypt"
)

// 用子进程运行真实启动入口，退出信号不会影响父测试进程。
func TestServerStartupProcess(t *testing.T) {
	if os.Getenv("KBOT_SERVER_TEST_PROCESS") != "1" {
		return
	}
	if err := run(); err != nil {
		t.Fatalf("server startup: %v", err)
	}
}

func TestServerBootstrapPostgres(t *testing.T) {
	rawURL := os.Getenv("KBOT_TEST_DATABASE_URL")
	if rawURL == "" {
		t.Skip("set KBOT_TEST_DATABASE_URL to verify actual server startup")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Second)
	defer cancel()
	conn, err := pgx.Connect(ctx, rawURL)
	if err != nil {
		t.Fatalf("connect test database: %v", err)
	}
	defer conn.Close(context.Background())
	schema := pgx.Identifier{"kbot_server_bootstrap_" + uuid.NewString()}.Sanitize()
	if _, err := conn.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		t.Fatalf("create isolated schema: %v", err)
	}
	defer func() {
		cleanupCtx, stop := context.WithTimeout(context.Background(), 10*time.Second)
		defer stop()
		if _, err := conn.Exec(cleanupCtx, "DROP SCHEMA "+schema+" CASCADE"); err != nil {
			t.Errorf("cleanup server schema: %v", err)
		}
	}()
	if _, err := conn.Exec(ctx, "SELECT set_config('search_path', $1, false)", schema+",pg_catalog"); err != nil {
		t.Fatalf("set fixture search path: %v", err)
	}
	migration, err := os.ReadFile(filepath.Join("..", "..", "migrations", "000001_init_iam.up.sql"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := conn.Exec(ctx, string(migration)); err != nil {
		t.Fatalf("create fixture tables: %v", err)
	}
	u, err := url.Parse(rawURL)
	if err != nil {
		t.Fatal(err)
	}
	query := u.Query()
	query.Set("search_path", schema+",pg_catalog")
	u.RawQuery = query.Encode()

	runServer := func(enabled bool, password string) {
		t.Helper()
		listener, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		addr := listener.Addr().String()
		if err := listener.Close(); err != nil {
			t.Fatal(err)
		}
		cmd := exec.Command(os.Args[0], "-test.run=^TestServerStartupProcess$")
		for _, item := range os.Environ() {
			if !strings.HasPrefix(item, "KBOT_") {
				cmd.Env = append(cmd.Env, item)
			}
		}
		cmd.Env = append(cmd.Env,
			"KBOT_SERVER_TEST_PROCESS=1", "KBOT_ADDR="+addr, "KBOT_DATABASE_URL="+u.String(),
			"KBOT_JWT_SECRET_KEY="+strings.Repeat("k", 32),
			"KBOT_AUTOSEED_ADMIN="+strconv.FormatBool(enabled),
			"KBOT_AUTOSEED_ADMIN_EMAIL=bootstrap-test@example.com", "KBOT_AUTOSEED_ADMIN_PASSWORD="+password,
		)
		var output bytes.Buffer
		cmd.Stdout, cmd.Stderr = &output, &output
		if err := cmd.Start(); err != nil {
			t.Fatal(err)
		}
		exited := make(chan error, 1)
		go func() { exited <- cmd.Wait() }()
		stopped := false
		defer func() {
			if !stopped {
				_ = cmd.Process.Kill()
				<-exited
			}
		}()
		client := &http.Client{Timeout: time.Second}
		deadline := time.Now().Add(12 * time.Second)
		for {
			select {
			case err := <-exited:
				stopped = true
				t.Fatalf("server exited before ready: %v; %s", err, output.String())
			default:
			}
			response, err := client.Get("http://" + addr + "/readyz")
			if err == nil {
				response.Body.Close()
				if response.StatusCode == http.StatusOK {
					break
				}
			}
			if time.Now().After(deadline) || ctx.Err() != nil {
				t.Fatal("server did not become ready within startup deadline")
			}
			time.Sleep(40 * time.Millisecond)
		}
		if err := cmd.Process.Signal(os.Interrupt); err != nil {
			t.Fatalf("signal server: %v", err)
		}
		select {
		case err := <-exited:
			stopped = true
			if err != nil || !strings.Contains(output.String(), "server stopped") {
				t.Fatalf("server did not shut down gracefully: %v; %s", err, output.String())
			}
		case <-time.After(18 * time.Second):
			t.Fatal("server did not exit after interrupt")
		}
	}

	runServer(false, "password123")
	var count int
	if err := conn.QueryRow(ctx, "SELECT count(*) FROM users").Scan(&count); err != nil || count != 0 {
		t.Fatalf("disabled bootstrap must not create users: count=%d err=%v", count, err)
	}
	runServer(true, "password123")
	var firstID, firstHash, role string
	if err := conn.QueryRow(ctx, "SELECT id::text,password_hash,role FROM users WHERE email=$1", "bootstrap-test@example.com").Scan(&firstID, &firstHash, &role); err != nil {
		t.Fatalf("startup must create seed account: %v", err)
	}
	if role != "admin" || bcrypt.CompareHashAndPassword([]byte(firstHash), []byte("password123")) != nil {
		t.Fatal("startup must persist the admin role and bcrypt password")
	}
	runServer(true, "changed-password456")
	var secondID, secondHash string
	if err := conn.QueryRow(ctx, "SELECT id::text,password_hash FROM users WHERE email=$1", "bootstrap-test@example.com").Scan(&secondID, &secondHash); err != nil {
		t.Fatal(err)
	}
	if err := conn.QueryRow(ctx, "SELECT count(*) FROM users").Scan(&count); err != nil || count != 1 || secondID != firstID || secondHash != firstHash {
		t.Fatal("restarting server must retain the original seed account and password")
	}
}
