package main

import (
	"context"
	"flag"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// 子进程调用实际 main，使 log.Fatal/os.Exit 不会结束父测试。
func TestMigrationCLIProcess(t *testing.T) {
	if os.Getenv("KBOT_MIGRATE_TEST_PROCESS") != "1" {
		return
	}
	for i, arg := range os.Args {
		if arg == "--" {
			os.Args = append([]string{"migrate"}, os.Args[i+1:]...)
			flag.CommandLine = flag.NewFlagSet("migrate", flag.ExitOnError)
			main()
			os.Exit(0)
		}
	}
	os.Exit(2)
}

func TestMigrationCLI(t *testing.T) {
	rawURL := os.Getenv("KBOT_TEST_DATABASE_URL")
	if rawURL == "" {
		t.Skip("set KBOT_TEST_DATABASE_URL to test the migration CLI with PostgreSQL")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	admin, err := pgx.Connect(ctx, rawURL)
	if err != nil {
		t.Fatalf("connect test database: %v", err)
	}
	defer admin.Close(context.Background())
	databaseName := "kbot_migrate_test_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	quotedName := pgx.Identifier{databaseName}.Sanitize()
	if _, err := admin.Exec(ctx, "CREATE DATABASE "+quotedName); err != nil {
		t.Fatalf("create disposable database (test role needs CREATEDB): %v", err)
	}
	defer func() {
		cleanupCtx, stop := context.WithTimeout(context.Background(), 10*time.Second)
		defer stop()
		if _, err := admin.Exec(cleanupCtx, "DROP DATABASE "+quotedName+" WITH (FORCE)"); err != nil {
			t.Errorf("remove disposable database %s: %v", databaseName, err)
		}
	}()
	u, err := url.Parse(rawURL)
	if err != nil {
		t.Fatalf("parse database URL: %v", err)
	}
	u.Path = "/" + databaseName
	testURL := u.String()
	conn, err := pgx.Connect(ctx, testURL)
	if err != nil {
		t.Fatalf("connect disposable database: %v", err)
	}
	defer conn.Close(context.Background())
	migrationPath, err := filepath.Abs(filepath.Join("..", "..", "migrations"))
	if err != nil {
		t.Fatalf("resolve migration path: %v", err)
	}
	runCLI := func(args ...string) string {
		t.Helper()
		processArgs := []string{"-test.run=^TestMigrationCLIProcess$", "--", "-path", migrationPath, "-database-env", "KBOT_MIGRATE_TEST_CONNECTION"}
		cmd := exec.CommandContext(ctx, os.Args[0], append(processArgs, args...)...)
		for _, item := range os.Environ() {
			if strings.HasPrefix(item, "KBOT_DATABASE_URL=") || strings.HasPrefix(item, "KBOT_MIGRATE_TEST_CONNECTION=") || strings.HasPrefix(item, "KBOT_MIGRATE_TEST_PROCESS=") {
				continue
			}
			cmd.Env = append(cmd.Env, item)
		}
		cmd.Env = append(cmd.Env, "KBOT_MIGRATE_TEST_PROCESS=1", "KBOT_MIGRATE_TEST_CONNECTION="+testURL)
		output, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("migration CLI %v failed: %v\n%s", args, err, output)
		}
		return string(output)
	}
	assertState := func(wantTables, wantVersions int) {
		t.Helper()
		var tables, versions int
		err := conn.QueryRow(ctx, `SELECT count(*) FROM pg_catalog.pg_tables WHERE schemaname='public' AND tablename IN ('users','workspaces','workspace_members','roles','user_roles','role_permissions','api_keys')`).Scan(&tables)
		if err != nil || tables != wantTables {
			t.Fatalf("IAM tables=%d, want %d, err=%v", tables, wantTables, err)
		}
		if err := conn.QueryRow(ctx, "SELECT count(*) FROM schema_migrations").Scan(&versions); err != nil || versions != wantVersions {
			t.Fatalf("migration records=%d, want %d, err=%v", versions, wantVersions, err)
		}
		if wantVersions > 0 {
			var version int64
			var dirty bool
			if err := conn.QueryRow(ctx, "SELECT version, dirty FROM schema_migrations").Scan(&version, &dirty); err != nil || version != 1 || dirty {
				t.Fatalf("version=%d dirty=%v err=%v, want version 1 and clean", version, dirty, err)
			}
		}
	}

	runCLI("-version")
	assertState(0, 0)
	runCLI("-up")
	assertState(7, 1)
	runCLI("-up")
	assertState(7, 1)
	if output := runCLI("-version"); !strings.Contains(output, "dirty=false") {
		t.Fatalf("version output does not report clean state: %s", output)
	}
	runCLI("-down", "1")
	assertState(0, 0)
	runCLI("-version")
	runCLI("-up")
	assertState(7, 1)
}
