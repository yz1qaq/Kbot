package api

import (
	"context"
	"encoding/json"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"io"
	"my_kbot_final/internal/domain"
	pgstore "my_kbot_final/internal/infrastructure/postgres/sqlc"
	"my_kbot_final/internal/platform/iam"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// ---- workspaces_postgres_test.go ----

// Requests pass through the real router, JWT middleware, service and PostgreSQL.
// The test owns a transaction and schema; no development account or space is changed.
func TestWorkspaceRoutesPostgres(t *testing.T) {
	databaseURL := os.Getenv("KBOT_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("set KBOT_TEST_DATABASE_URL to verify workspace HTTP/PG behavior")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = tx.Rollback(context.Background()) })
	schema := pgx.Identifier{"kbot_workspace_route_" + uuid.NewString()}.Sanitize()
	if _, err := tx.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, "SELECT set_config('search_path',$1,true)", schema+",pg_catalog"); err != nil {
		t.Fatal(err)
	}
	migration, err := os.ReadFile(filepath.Join("..", "..", "migrations", "000001_init_iam.up.sql"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, string(migration)); err != nil {
		t.Fatal(err)
	}
	store := iam.NewPostgresStore(pool, pgstore.New(tx))
	service := iam.NewService(store, []byte("workspace-route-test-key-with-at-least-32-bytes"))
	for _, email := range []string{"a@example.com", "b@example.com", "admin@example.com"} {
		if _, err := service.CreateUser(ctx, email, "password123", email); err != nil {
			t.Fatal(err)
		}
	}
	adminUser, err := store.GetUserByEmail(ctx, "admin@example.com")
	if err != nil {
		t.Fatal(err)
	}
	if err := store.SetUserRole(ctx, adminUser.ID, iam.GlobalRoleAdmin); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(NewHandler(service).Routes())
	t.Cleanup(server.Close)
	client := server.Client()
	client.Timeout = 5 * time.Second
	type reply struct {
		status      int
		body        []byte
		contentType string
	}
	request := func(method, path, token, body string) reply {
		t.Helper()
		req, err := http.NewRequestWithContext(ctx, method, server.URL+path, strings.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Content-Type", "application/json")
		// Headers controlled by the caller must not select the actor or global role.
		req.Header.Set("X-User-ID", adminUser.ID)
		req.Header.Set("X-Global-Role", "admin")
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		res, err := client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		data, err := io.ReadAll(io.LimitReader(res.Body, 1<<20))
		if err != nil {
			t.Fatal(err)
		}
		return reply{res.StatusCode, data, res.Header.Get("Content-Type")}
	}
	login := func(email string) iam.LoginResponse {
		t.Helper()
		body, _ := json.Marshal(map[string]string{"email": email, "password": "password123"})
		res := request(http.MethodPost, "/api/v1/auth/login", "", string(body))
		var result iam.LoginResponse
		if res.status != http.StatusOK || json.Unmarshal(res.body, &result) != nil || result.Token == "" || result.User == nil {
			t.Fatalf("fixture login failed: status=%d", res.status)
		}
		return result
	}
	a, b, admin := login("a@example.com"), login("b@example.com"), login("admin@example.com")
	for _, method := range []string{http.MethodGet, http.MethodPost} {
		for _, token := range []string{"", "invalid-jwt"} {
			res := request(method, "/api/v1/workspaces", token, `{"name":"denied"}`)
			if res.status != http.StatusUnauthorized {
				t.Fatalf("%s without valid identity: got %d, want 401", method, res.status)
			}
		}
	}
	list := func(token, query string, wantIDs ...string) {
		t.Helper()
		res := request(http.MethodGet, "/api/v1/workspaces"+query, token, "")
		var spaces []domain.Workspace
		if res.status != http.StatusOK || res.contentType != "application/json" || json.Unmarshal(res.body, &spaces) != nil || spaces == nil {
			t.Fatalf("list: status=%d, body=%s", res.status, res.body)
		}
		if len(spaces) != len(wantIDs) {
			t.Fatalf("space count=%d, want %d", len(spaces), len(wantIDs))
		}
		seen := map[string]bool{}
		for _, space := range spaces {
			seen[space.ID] = true
		}
		for _, id := range wantIDs {
			if !seen[id] {
				t.Fatalf("expected accessible workspace %q", id)
			}
		}
	}
	list(a.Token, "") // [] rather than null, and no workspace-selection header required.
	for _, body := range []string{`{"name":`, `{}`, `{"name":""}`, `{"name":"forged","user_id":"other"}`, `{"name":"forged","role":"owner"}`} {
		res := request(http.MethodPost, "/api/v1/workspaces", a.Token, body)
		if res.status != http.StatusBadRequest {
			t.Fatalf("invalid body must return 400, got %d", res.status)
		}
	}
	list(admin.Token, "")
	create := func(actor iam.LoginResponse, name string, parent *string) domain.Workspace {
		t.Helper()
		body, _ := json.Marshal(map[string]any{"name": name, "description": "HTTP test", "parent_id": parent})
		res := request(http.MethodPost, "/api/v1/workspaces", actor.Token, string(body))
		var space domain.Workspace
		if res.status != http.StatusCreated || res.contentType != "application/json" || json.Unmarshal(res.body, &space) != nil || space.ID == "" || space.Name != name {
			t.Fatalf("create: status=%d, body=%s", res.status, res.body)
		}
		member, err := store.GetWorkspaceMember(ctx, space.ID, actor.User.ID)
		if err != nil || member.Role != "owner" {
			t.Fatalf("authenticated creator must own space: member=%+v, err=%v", member, err)
		}
		stored, err := store.GetWorkspaceByID(ctx, space.ID)
		if err != nil || stored.Name != name || stored.Description != "HTTP test" {
			t.Fatalf("space not persisted correctly: %v", err)
		}
		if parent != nil && (space.ParentID == nil || *space.ParentID != *parent || stored.ParentID == nil || *stored.ParentID != *parent) {
			t.Fatal("parent_id lost")
		}
		return space
	}
	spaceA := create(a, "A space", nil)
	spaceB := create(b, "B space", nil)
	list(a.Token, "", spaceA.ID)
	list(b.Token, "", spaceB.ID)
	list(admin.Token, "", spaceA.ID, spaceB.ID)
	list(a.Token, "?limit=1&offset=0", spaceA.ID)
	list(a.Token, "?limit=1&offset=1")
	for _, query := range []string{"?limit=0", "?limit=201", "?offset=-1", "?limit=abc"} {
		res := request(http.MethodGet, "/api/v1/workspaces"+query, a.Token, "")
		if res.status != http.StatusBadRequest {
			t.Fatalf("invalid pagination %s: got %d", query, res.status)
		}
	}
	child := create(a, "A child", &spaceA.ID)
	list(a.Token, "", spaceA.ID, child.ID)
	list(b.Token, "", spaceB.ID)
	list(admin.Token, "", spaceA.ID, spaceB.ID, child.ID)

	t.Run("workspace member HTTP permissions", func(t *testing.T) {
		members := func(token, space, query string, status int, wantIDs ...string) {
			t.Helper()
			res := request(http.MethodGet, "/api/v1/workspaces/"+space+"/members"+query, token, "")
			if res.status != status {
				t.Fatalf("member route status=%d body=%s, want %d", res.status, res.body, status)
			}
			if status != http.StatusOK {
				return
			}
			var rows []domain.WorkspaceMember
			if res.contentType != "application/json" || json.Unmarshal(res.body, &rows) != nil || rows == nil || len(rows) != len(wantIDs) {
				t.Fatalf("member response status=%d body=%s", res.status, res.body)
			}
			seen := map[string]bool{}
			for _, row := range rows {
				if row.WorkspaceID != space || row.Email == "" || row.Name == "" {
					t.Fatal("wrong space or missing joined profile")
				}
				seen[row.UserID] = true
			}
			for _, id := range wantIDs {
				if !seen[id] {
					t.Fatalf("missing member %s", id)
				}
			}
			if strings.Contains(string(res.body), "password_hash") || strings.Contains(string(res.body), "password123") {
				t.Fatal("member response exposed password material")
			}
		}
		members("", spaceA.ID, "", http.StatusUnauthorized)
		members("invalid-jwt", spaceA.ID, "", http.StatusUnauthorized)
		members(a.Token, spaceA.ID, "", http.StatusOK, a.User.ID)
		members(b.Token, spaceB.ID, "", http.StatusOK, b.User.ID)
		members(admin.Token, spaceA.ID, "", http.StatusOK, a.User.ID)
		members(admin.Token, spaceB.ID, "", http.StatusOK, b.User.ID)
		members(a.Token, spaceB.ID, "", http.StatusForbidden)
		members(b.Token, spaceA.ID, "", http.StatusForbidden)
		// The URL, not a forged query field, identifies the target space.
		members(a.Token, spaceA.ID, "?workspace_id="+spaceB.ID, http.StatusOK, a.User.ID)
		put := func(user, role string) {
			t.Helper()
			if err := store.UpsertWorkspaceMember(ctx, &domain.WorkspaceMember{WorkspaceID: spaceA.ID, UserID: user, Role: role}); err != nil {
				t.Fatal(err)
			}
		}
		put(b.User.ID, "viewer")
		members(b.Token, spaceA.ID, "", http.StatusForbidden)
		put(b.User.ID, "admin")
		members(b.Token, spaceA.ID, "", http.StatusOK, a.User.ID, b.User.ID)
		put(b.User.ID, "editor")
		members(b.Token, spaceA.ID, "", http.StatusForbidden)
		put(admin.User.ID, "viewer")
		members(a.Token, spaceA.ID, "", http.StatusOK, a.User.ID, b.User.ID, admin.User.ID)
		if deleted, err := store.DeleteWorkspaceMember(ctx, spaceA.ID, admin.User.ID); err != nil || !deleted {
			t.Fatal("remove fixture global admin membership")
		}
		members(admin.Token, spaceA.ID, "", http.StatusOK, a.User.ID, b.User.ID)
	})
	if err := store.SetUserRole(ctx, admin.User.ID, iam.GlobalRoleMember); err != nil {
		t.Fatal(err)
	}
	list(admin.Token, "") // Existing JWT observes current DB role, not a caller-supplied role header.
	if _, err := tx.Exec(ctx, "UPDATE users SET status='inactive' WHERE id=$1", uuid.MustParse(a.User.ID)); err != nil {
		t.Fatal(err)
	}
	if res := request(http.MethodGet, "/api/v1/workspaces", a.Token, ""); res.status != http.StatusUnauthorized {
		t.Fatalf("inactive account: got %d, want 401", res.status)
	}
}

// ---- workspace_members_write_postgres_test.go ----

type membershipHTTPFixture struct {
	ctx            context.Context
	store          *iam.PostgresStore
	server         *httptest.Server
	users          map[string]*domain.User
	tokens         map[string]string
	spaceA, spaceB string
}

type membershipHTTPReply struct {
	status      int
	body        []byte
	contentType string
}

func (f *membershipHTTPFixture) request(t *testing.T, method, path, token, body string) membershipHTTPReply {
	t.Helper()
	req, err := http.NewRequestWithContext(f.ctx, method, f.server.URL+path, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	// These caller-controlled fields must not replace the authenticated actor or path target.
	req.Header.Set("X-User-ID", f.users["global"].ID)
	req.Header.Set("X-Global-Role", "admin")
	req.Header.Set("X-Workspace-ID", f.spaceB)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	response, err := f.server.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	data, err := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if err != nil {
		t.Fatal(err)
	}
	return membershipHTTPReply{response.StatusCode, data, response.Header.Get("Content-Type")}
}

func newMembershipHTTPFixture(t *testing.T) *membershipHTTPFixture {
	t.Helper()
	databaseURL := os.Getenv("KBOT_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("set KBOT_TEST_DATABASE_URL for real membership HTTP/PostgreSQL tests")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Second)
	t.Cleanup(cancel)
	control, err := pgx.Connect(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = control.Close(context.Background()) })
	schema := pgx.Identifier{"kbot_members_write_" + uuid.NewString()}.Sanitize()
	if _, err := control.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cleanupCtx, stop := context.WithTimeout(context.Background(), 5*time.Second)
		defer stop()
		if _, err := control.Exec(cleanupCtx, "DROP SCHEMA "+schema+" CASCADE"); err != nil {
			t.Error("cleanup membership fixture", err)
		}
	})
	if _, err := control.Exec(ctx, "SELECT set_config('search_path',$1,false)", schema+",pg_catalog"); err != nil {
		t.Fatal(err)
	}
	migration, err := os.ReadFile(filepath.Join("..", "..", "migrations", "000001_init_iam.up.sql"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := control.Exec(ctx, string(migration)); err != nil {
		t.Fatal(err)
	}
	config, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	if config.ConnConfig.RuntimeParams == nil {
		config.ConnConfig.RuntimeParams = map[string]string{}
	}
	config.ConnConfig.RuntimeParams["search_path"] = schema + ",pg_catalog"
	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	store := iam.NewPostgresStore(pool, pgstore.New(pool))
	service := iam.NewService(store, []byte("membership-write-http-isolated-key-at-least-32-bytes"))
	f := &membershipHTTPFixture{ctx: ctx, store: store, users: map[string]*domain.User{}, tokens: map[string]string{}}
	for _, name := range []string{"owner", "outsider", "target", "global"} {
		user, err := service.CreateUser(ctx, name+"@example.com", "password123", name)
		if err != nil {
			t.Fatal(err)
		}
		f.users[name] = user
	}
	if err := store.SetUserRole(ctx, f.users["global"].ID, iam.GlobalRoleAdmin); err != nil {
		t.Fatal(err)
	}
	a, err := service.CreateWorkspaceForUser(ctx, f.users["owner"].ID, "Space A", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	b, err := service.CreateWorkspaceForUser(ctx, f.users["outsider"].ID, "Space B", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	f.spaceA, f.spaceB = a.ID, b.ID
	f.server = httptest.NewServer(NewHandler(service).Routes())
	f.server.Client().Timeout = 5 * time.Second
	t.Cleanup(f.server.Close)
	for name, user := range f.users {
		body, _ := json.Marshal(map[string]string{"email": user.Email, "password": "password123"})
		res := f.request(t, http.MethodPost, "/api/v1/auth/login", "", string(body))
		var login iam.LoginResponse
		if res.status != http.StatusOK || json.Unmarshal(res.body, &login) != nil || login.Token == "" {
			t.Fatalf("fixture login failed for %s: status=%d", name, res.status)
		}
		f.tokens[name] = login.Token
	}
	return f
}

// Use committed data and a schema-bound pool: guarded Store methods open and
// commit their own transactions. An outer uncommitted test tx would hide rows.
func TestUpsertWorkspaceMemberRoutePostgres(t *testing.T) {
	f := newMembershipHTTPFixture(t)
	path := func(space, user string) string { return "/api/v1/workspaces/" + space + "/members/" + user }
	put := func(actor, space, user, body string, want int) membershipHTTPReply {
		t.Helper()
		res := f.request(t, http.MethodPut, path(space, user), f.tokens[actor], body)
		if res.status != want {
			t.Fatalf("PUT as %s: status=%d body=%s, want %d", actor, res.status, res.body, want)
		}
		return res
	}
	owner, outsider, target := f.users["owner"].ID, f.users["outsider"].ID, f.users["target"].ID
	for _, token := range []string{"", "invalid-token"} {
		res := f.request(t, http.MethodPut, path(f.spaceA, target), token, `{"role":"viewer"}`)
		if res.status != http.StatusUnauthorized {
			t.Fatalf("invalid identity: status=%d, want 401", res.status)
		}
	}
	for _, body := range []string{`{"role":`, `{}`, `{"role":"root"}`, `{"role":"OWNER"}`, `{"role":" viewer "}`, `{"role":"viewer","user_id":"forged"}`, `{"role":"viewer","actor_id":"forged"}`} {
		put("owner", f.spaceA, target, body, http.StatusBadRequest)
	}
	if _, err := f.store.GetWorkspaceMember(f.ctx, f.spaceA, target); err == nil {
		t.Fatal("invalid requests inserted a member")
	}
	put("outsider", f.spaceA, target, `{"role":"viewer"}`, http.StatusForbidden)
	decodeMember := func(res membershipHTTPReply, id, role string) domain.WorkspaceMember {
		t.Helper()
		var member domain.WorkspaceMember
		if res.contentType != "application/json" || json.Unmarshal(res.body, &member) != nil || member.WorkspaceID != f.spaceA || member.UserID != id || member.Role != role || member.Email == "" || member.Name == "" || member.CreatedAt.IsZero() {
			t.Fatalf("incomplete member JSON: %s", res.body)
		}
		if strings.Contains(string(res.body), "password") {
			t.Fatal("membership response exposed password material")
		}
		return member
	}
	added := decodeMember(put("owner", f.spaceA, target, `{"role":"viewer"}`, http.StatusOK), target, iam.WorkspaceRoleViewer)
	updated := decodeMember(put("owner", f.spaceA, target, `{"role":"editor"}`, http.StatusOK), target, iam.WorkspaceRoleEditor)
	if !updated.CreatedAt.Equal(added.CreatedAt) {
		t.Fatal("role update replaced initial membership time")
	}
	put("target", f.spaceA, owner, `{"role":"viewer"}`, http.StatusForbidden)
	put("owner", f.spaceA, target, `{"role":"admin"}`, http.StatusOK)
	decodeMember(put("target", f.spaceA, outsider, `{"role":"editor"}`, http.StatusOK), outsider, iam.WorkspaceRoleEditor)
	put("target", f.spaceA, outsider, `{"role":"owner"}`, http.StatusForbidden)
	put("target", f.spaceA, owner, `{"role":"viewer"}`, http.StatusForbidden)
	put("owner", f.spaceA, owner, `{"role":"viewer"}`, http.StatusConflict)
	put("global", f.spaceA, owner, `{"role":"viewer"}`, http.StatusConflict)
	res := f.request(t, http.MethodPut, path(f.spaceA, outsider)+"?workspace_id="+f.spaceB+"&user_id="+owner, f.tokens["owner"], `{"role":"member"}`)
	if res.status != http.StatusOK {
		t.Fatalf("path target: status=%d body=%s", res.status, res.body)
	}
	decodeMember(res, outsider, iam.WorkspaceRoleMember)
	if _, err := f.store.GetWorkspaceMember(f.ctx, f.spaceB, target); err == nil {
		t.Fatal("selected workspace header changed PUT destination")
	}
	bOwner, err := f.store.GetWorkspaceMember(f.ctx, f.spaceB, outsider)
	if err != nil || bOwner.Role != iam.WorkspaceRoleOwner {
		t.Fatal("forged query changed another workspace", err)
	}
	put("global", f.spaceA, target, `{"role":"owner"}`, http.StatusOK)
	put("owner", f.spaceA, owner, `{"role":"viewer"}`, http.StatusOK)
	put("owner", f.spaceA, outsider, `{"role":"editor"}`, http.StatusForbidden)
	stored, err := f.store.GetWorkspaceMember(f.ctx, f.spaceA, target)
	if err != nil || stored.Role != iam.WorkspaceRoleOwner {
		t.Fatal("owner transfer was not persisted", err)
	}
}

// Membership deletion uses the same real router/JWT/schema-bound pool fixture
// as PUT, including the deliberately forged actor and workspace headers.
func TestDeleteWorkspaceMemberRoutePostgres(t *testing.T) {
	f := newMembershipHTTPFixture(t)
	path := func(space, user string) string {
		return "/api/v1/workspaces/" + space + "/members/" + user
	}
	owner := f.users["owner"].ID
	spaceAdmin := f.users["outsider"].ID
	target := f.users["target"].ID
	put := func(actor, space, user, role string) {
		t.Helper()
		body, err := json.Marshal(map[string]string{"role": role})
		if err != nil {
			t.Fatal(err)
		}
		res := f.request(t, http.MethodPut, path(space, user), f.tokens[actor], string(body))
		if res.status != http.StatusOK {
			t.Fatalf("fixture PUT as %s: status=%d body=%s", actor, res.status, res.body)
		}
	}
	remove := func(actor, space, user, query string, want int) {
		t.Helper()
		res := f.request(t, http.MethodDelete, path(space, user)+query, f.tokens[actor], "")
		if res.status != want {
			t.Fatalf("DELETE as %s: status=%d body=%s, want %d", actor, res.status, res.body, want)
		}
		if want == http.StatusNoContent && len(res.body) != 0 {
			t.Fatal("successful 204 deletion must not return a JSON/text body")
		}
	}
	for _, token := range []string{"", "invalid-token"} {
		res := f.request(t, http.MethodDelete, path(f.spaceA, owner), token, "")
		if res.status != http.StatusUnauthorized {
			t.Fatalf("DELETE without valid identity: got %d, want 401", res.status)
		}
	}
	remove("outsider", f.spaceA, owner, "", http.StatusForbidden)
	put("owner", f.spaceA, target, iam.WorkspaceRoleViewer)
	put("outsider", f.spaceB, target, iam.WorkspaceRoleViewer)
	remove("target", f.spaceA, target, "", http.StatusForbidden)
	put("owner", f.spaceA, spaceAdmin, iam.WorkspaceRoleAdmin)
	remove("outsider", f.spaceA, owner, "", http.StatusForbidden)
	remove("owner", f.spaceA, owner, "", http.StatusConflict)
	remove("global", f.spaceA, owner, "", http.StatusConflict)
	remaining, err := f.store.GetWorkspaceMember(f.ctx, f.spaceA, owner)
	if err != nil || remaining.Role != iam.WorkspaceRoleOwner {
		t.Fatal("rejected deletion changed the last owner", err)
	}
	// URL path must win over the selected workspace header and forged query IDs.
	remove("outsider", f.spaceA, target, "?workspace_id="+f.spaceB+"&user_id="+owner, http.StatusNoContent)
	if _, err := f.store.GetWorkspaceMember(f.ctx, f.spaceA, target); err == nil {
		t.Fatal("deleted membership remains visible after the HTTP response")
	}
	other, err := f.store.GetWorkspaceMember(f.ctx, f.spaceB, target)
	if err != nil || other.Role != iam.WorkspaceRoleViewer {
		t.Fatal("deletion crossed the workspace boundary", err)
	}
	account, err := f.store.GetUserByID(f.ctx, target)
	if err != nil || account.Status != "active" {
		t.Fatal("removing membership must preserve the user account", err)
	}
	// Fixed Final currently maps the generic missing-member business error to 500.
	remove("owner", f.spaceA, target, "", http.StatusInternalServerError)
	put("global", f.spaceA, spaceAdmin, iam.WorkspaceRoleOwner)
	remove("global", f.spaceA, owner, "", http.StatusNoContent)
	if _, err := f.store.GetWorkspaceMember(f.ctx, f.spaceA, owner); err == nil {
		t.Fatal("owner membership survived a successful removal")
	}
	account, err = f.store.GetUserByID(f.ctx, owner)
	if err != nil || account.Status != "active" {
		t.Fatal("removing an owner must not delete the account", err)
	}
	other, err = f.store.GetWorkspaceMember(f.ctx, f.spaceB, spaceAdmin)
	if err != nil || other.Role != iam.WorkspaceRoleOwner {
		t.Fatal("another workspace owner was changed", err)
	}
	// The original token still authenticates the account, but current space access is gone.
	res := f.request(t, http.MethodGet, "/api/v1/workspaces/"+f.spaceA+"/members", f.tokens["owner"], "")
	if res.status != http.StatusForbidden {
		t.Fatalf("removed owner's existing JWT retained space access: status=%d", res.status)
	}
	remove("global", f.spaceA, spaceAdmin, "", http.StatusConflict)
}
