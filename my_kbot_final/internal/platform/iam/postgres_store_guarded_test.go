package iam

import (
	"context"
	"errors"
	"fmt"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"my_kbot_final/internal/domain"
	pgstore "my_kbot_final/internal/infrastructure/postgres/sqlc"
	"os"
	"strings"
	"testing"
	"time"
)

// ---- postgres_store_guarded_upsert_test.go ----

// Guarded methods open their own transactions. Bind the actual pool and Queries
// to a committed disposable schema, rather than hiding writes in an outer tx.
func newGuardedMembershipStoreTest(t *testing.T) (context.Context, *PostgresStore, string, string, string) {
	t.Helper()
	ctx, control, _ := newWorkspaceLockConnections(t)
	var searchPath string
	if err := control.QueryRow(ctx, "SHOW search_path").Scan(&searchPath); err != nil {
		t.Fatal(err)
	}
	config, err := pgxpool.ParseConfig(os.Getenv("KBOT_TEST_DATABASE_URL"))
	if err != nil {
		t.Fatal(err)
	}
	config.MaxConns = 4
	if config.ConnConfig.RuntimeParams == nil {
		config.ConnConfig.RuntimeParams = map[string]string{}
	}
	config.ConnConfig.RuntimeParams["search_path"] = searchPath
	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	store := NewPostgresStore(pool, pgstore.New(pool))
	owner, member, newcomer := uuid.NewString(), uuid.NewString(), uuid.NewString()
	for _, id := range []string{owner, member, newcomer} {
		if err := store.CreateUser(ctx, &domain.User{ID: id, Email: id + "@example.com", Role: GlobalRoleMember}); err != nil {
			t.Fatal(err)
		}
	}
	for _, fixture := range []struct{ space, user, role string }{
		{"space-a", owner, "owner"}, {"space-a", member, "viewer"},
		{"space-b", owner, "owner"}, {"space-b", member, "viewer"},
	} {
		if err := store.UpsertWorkspaceMember(ctx, &domain.WorkspaceMember{WorkspaceID: fixture.space, UserID: fixture.user, Role: fixture.role}); err != nil {
			t.Fatal(err)
		}
	}
	return ctx, store, owner, member, newcomer
}

func TestPostgresStoreUpsertWorkspaceMemberGuarded(t *testing.T) {
	ctx, store, _, _, newcomer := newGuardedMembershipStoreTest(t)
	rawID := strings.ReplaceAll(newcomer, "-", "")
	member := &domain.WorkspaceMember{WorkspaceID: "space-a", UserID: rawID, Role: WorkspaceRoleViewer}
	if err := store.UpsertWorkspaceMemberGuarded(ctx, member, false); err != nil {
		t.Fatal(err)
	}
	if member.CreatedAt.IsZero() || member.UserID != rawID {
		t.Fatal("write back CreatedAt while preserving the caller's UserID")
	}
	joinedAt := member.CreatedAt
	member.Role = WorkspaceRoleEditor
	if err := store.UpsertWorkspaceMemberGuarded(ctx, member, false); err != nil {
		t.Fatal(err)
	}
	if !member.CreatedAt.Equal(joinedAt) {
		t.Fatal("role update must preserve initial membership time")
	}
	stored, err := store.GetWorkspaceMember(ctx, "space-a", newcomer)
	if err != nil || stored.Role != WorkspaceRoleEditor {
		t.Fatalf("committed role not readable: member=%v err=%v", stored, err)
	}
	if _, err := store.GetWorkspaceMember(ctx, "space-b", newcomer); err == nil {
		t.Fatal("membership must not be added to another space")
	}
	user, err := store.GetUserByID(ctx, newcomer)
	if err != nil || user.Role != GlobalRoleMember {
		t.Fatal("space role must not alter the global role")
	}
	members, err := store.ListWorkspaceMembers(ctx, "space-a")
	if err != nil || len(members) != 3 {
		t.Fatalf("upsert must not duplicate the membership: count=%d err=%v", len(members), err)
	}
}

func TestPostgresStoreUpsertWorkspaceMemberGuardedOwnerRules(t *testing.T) {
	t.Run("basic manager cannot grant or touch owner", func(t *testing.T) {
		ctx, store, owner, _, newcomer := newGuardedMembershipStoreTest(t)
		for _, target := range []struct{ id, role string }{{newcomer, WorkspaceRoleOwner}, {owner, WorkspaceRoleOwner}, {owner, WorkspaceRoleViewer}} {
			err := store.UpsertWorkspaceMemberGuarded(ctx, &domain.WorkspaceMember{WorkspaceID: "space-a", UserID: target.id, Role: target.role}, false)
			if !errors.Is(err, ErrForbidden) {
				t.Fatalf("owner operation must be rejected: %v", err)
			}
		}
		stored, err := store.GetWorkspaceMember(ctx, "space-a", owner)
		if err != nil || stored.Role != WorkspaceRoleOwner {
			t.Fatal("rejected operation altered the owner")
		}
		if _, err := store.GetWorkspaceMember(ctx, "space-a", newcomer); err == nil {
			t.Fatal("forbidden grant inserted a member")
		}
	})
	t.Run("last owner cannot be demoted", func(t *testing.T) {
		ctx, store, owner, _, _ := newGuardedMembershipStoreTest(t)
		err := store.UpsertWorkspaceMemberGuarded(ctx, &domain.WorkspaceMember{WorkspaceID: "space-a", UserID: owner, Role: WorkspaceRoleAdmin}, true)
		if !errors.Is(err, ErrLastOwner) {
			t.Fatalf("expected last owner protection, got %v", err)
		}
		stored, err := store.GetWorkspaceMember(ctx, "space-a", owner)
		if err != nil || stored.Role != WorkspaceRoleOwner {
			t.Fatal("last owner was changed despite rejection")
		}
	})
	t.Run("owner transfer with another remaining owner", func(t *testing.T) {
		ctx, store, owner, member, _ := newGuardedMembershipStoreTest(t)
		if err := store.UpsertWorkspaceMemberGuarded(ctx, &domain.WorkspaceMember{WorkspaceID: "space-a", UserID: member, Role: WorkspaceRoleOwner}, true); err != nil {
			t.Fatal(err)
		}
		if err := store.UpsertWorkspaceMemberGuarded(ctx, &domain.WorkspaceMember{WorkspaceID: "space-a", UserID: owner, Role: WorkspaceRoleAdmin}, true); err != nil {
			t.Fatal(err)
		}
		if err := store.UpsertWorkspaceMemberGuarded(ctx, &domain.WorkspaceMember{WorkspaceID: "space-a", UserID: member, Role: WorkspaceRoleOwner}, true); err != nil {
			t.Fatal("unchanged owner role must remain allowed", err)
		}
		other, err := store.GetWorkspaceMember(ctx, "space-b", owner)
		if err != nil || other.Role != WorkspaceRoleOwner {
			t.Fatal("role update crossed workspace boundary")
		}
	})
}

func TestPostgresStoreUpsertWorkspaceMemberGuardedErrors(t *testing.T) {
	if err := NewPostgresStore(nil, nil).UpsertWorkspaceMemberGuarded(context.Background(), &domain.WorkspaceMember{}, true); err == nil {
		t.Fatal("missing transaction pool must be reported")
	}
	ctx, store, owner, _, _ := newGuardedMembershipStoreTest(t)
	if err := store.UpsertWorkspaceMemberGuarded(ctx, &domain.WorkspaceMember{WorkspaceID: "space-a", UserID: "invalid", Role: "viewer"}, false); err == nil {
		t.Fatal("invalid UUID accepted")
	}
	if err := store.UpsertWorkspaceMemberGuarded(ctx, &domain.WorkspaceMember{WorkspaceID: "missing-space", UserID: owner, Role: "viewer"}, false); err == nil {
		t.Fatal("missing workspace accepted")
	}
	original := time.Date(2026, 10, 22, 0, 0, 0, 0, time.UTC)
	missing := &domain.WorkspaceMember{WorkspaceID: "space-a", UserID: uuid.NewString(), Role: "viewer", CreatedAt: original}
	err := store.UpsertWorkspaceMemberGuarded(ctx, missing, false)
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != "23503" {
		t.Fatalf("foreign-key error lost: %v", err)
	}
	if !missing.CreatedAt.Equal(original) {
		t.Fatal("failed write modified the caller's CreatedAt")
	}
	// A query after the failed transaction observes preserved committed data.
	stored, err := store.GetWorkspaceMember(ctx, "space-a", owner)
	if err != nil || stored.Role != WorkspaceRoleOwner {
		t.Fatal("failed transaction damaged committed owner")
	}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	err = store.UpsertWorkspaceMemberGuarded(canceled, &domain.WorkspaceMember{WorkspaceID: "space-a", UserID: owner, Role: "owner"}, true)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("request cancellation lost: %v", err)
	}
}

func TestPostgresStoreUpsertWorkspaceMemberGuardedConcurrentDemotion(t *testing.T) {
	ctx, store, owner, secondOwner, _ := newGuardedMembershipStoreTest(t)
	if err := store.UpsertWorkspaceMember(ctx, &domain.WorkspaceMember{WorkspaceID: "space-a", UserID: secondOwner, Role: WorkspaceRoleOwner}); err != nil {
		t.Fatal(err)
	}
	start := make(chan struct{})
	results := make(chan error, 2)
	for _, id := range []string{owner, secondOwner} {
		go func(userID string) {
			<-start
			results <- store.UpsertWorkspaceMemberGuarded(ctx, &domain.WorkspaceMember{WorkspaceID: "space-a", UserID: userID, Role: WorkspaceRoleViewer}, true)
		}(id)
	}
	close(start)
	succeeded, rejected := 0, 0
	for range 2 {
		select {
		case err := <-results:
			if err == nil {
				succeeded++
			} else if errors.Is(err, ErrLastOwner) {
				rejected++
			} else {
				t.Fatalf("unexpected concurrent error: %v", err)
			}
		case <-ctx.Done():
			t.Fatal("concurrent updates failed to finish", ctx.Err())
		}
	}
	if succeeded != 1 || rejected != 1 {
		t.Fatalf("success=%d last-owner rejection=%d, want 1/1", succeeded, rejected)
	}
	members, err := store.ListWorkspaceMembers(ctx, "space-a")
	if err != nil {
		t.Fatal(err)
	}
	owners := 0
	for _, member := range members {
		if member.Role == WorkspaceRoleOwner {
			owners++
		}
	}
	if owners != 1 {
		t.Fatalf("remaining owners=%d, want 1", owners)
	}
}

// ---- postgres_store_guarded_delete_test.go ----

func TestPostgresStoreDeleteWorkspaceMemberGuarded(t *testing.T) {
	ctx, store, owner, member, newcomer := newGuardedMembershipStoreTest(t)
	deleted, err := store.DeleteWorkspaceMemberGuarded(ctx, "space-a", member, false)
	if err != nil || !deleted {
		t.Fatalf("remove ordinary member: deleted=%v err=%v", deleted, err)
	}
	if _, err := store.GetWorkspaceMember(ctx, "space-a", member); err == nil {
		t.Fatal("deleted membership remains visible after commit")
	}
	other, err := store.GetWorkspaceMember(ctx, "space-b", member)
	if err != nil || other.Role != WorkspaceRoleViewer {
		t.Fatal("deletion changed another workspace membership", err)
	}
	if _, err := store.GetUserByID(ctx, member); err != nil {
		t.Fatal("removing a membership must preserve the user account", err)
	}
	if _, err := store.GetWorkspaceByID(ctx, "space-a"); err != nil {
		t.Fatal("removing a membership must preserve the workspace", err)
	}
	remaining, err := store.GetWorkspaceMember(ctx, "space-a", owner)
	if err != nil || remaining.Role != WorkspaceRoleOwner {
		t.Fatal("ordinary member removal changed the owner", err)
	}
	for _, id := range []string{member, newcomer} {
		deleted, err := store.DeleteWorkspaceMemberGuarded(ctx, "space-a", id, false)
		if err != nil || deleted {
			t.Fatalf("absent membership: deleted=%v err=%v, want false/nil", deleted, err)
		}
	}
}

func TestPostgresStoreDeleteWorkspaceMemberGuardedOwnerRules(t *testing.T) {
	ctx, store, owner, secondOwner, _ := newGuardedMembershipStoreTest(t)
	deleted, err := store.DeleteWorkspaceMemberGuarded(ctx, "space-a", owner, false)
	if deleted || !errors.Is(err, ErrForbidden) {
		t.Fatalf("basic manager touching owner: deleted=%v err=%v", deleted, err)
	}
	deleted, err = store.DeleteWorkspaceMemberGuarded(ctx, "space-a", owner, true)
	if deleted || !errors.Is(err, ErrLastOwner) {
		t.Fatalf("last owner: deleted=%v err=%v", deleted, err)
	}
	stored, err := store.GetWorkspaceMember(ctx, "space-a", owner)
	if err != nil || stored.Role != WorkspaceRoleOwner {
		t.Fatal("rejected deletion changed the last owner", err)
	}
	if err := store.UpsertWorkspaceMember(ctx, &domain.WorkspaceMember{WorkspaceID: "space-a", UserID: secondOwner, Role: WorkspaceRoleOwner}); err != nil {
		t.Fatal(err)
	}
	deleted, err = store.DeleteWorkspaceMemberGuarded(ctx, "space-a", owner, true)
	if err != nil || !deleted {
		t.Fatalf("another owner remains: deleted=%v err=%v", deleted, err)
	}
	if _, err := store.GetWorkspaceMember(ctx, "space-a", owner); err == nil {
		t.Fatal("removed owner membership still exists")
	}
	for _, pair := range []struct{ space, user string }{{"space-a", secondOwner}, {"space-b", owner}} {
		stored, err := store.GetWorkspaceMember(ctx, pair.space, pair.user)
		if err != nil || stored.Role != WorkspaceRoleOwner {
			t.Fatalf("other owner must survive: space=%s err=%v", pair.space, err)
		}
	}
}

func TestPostgresStoreDeleteWorkspaceMemberGuardedErrors(t *testing.T) {
	if deleted, err := NewPostgresStore(nil, nil).DeleteWorkspaceMemberGuarded(context.Background(), "space-a", "invalid", true); err == nil || deleted {
		t.Fatalf("missing pool: deleted=%v err=%v", deleted, err)
	}
	ctx, store, owner, _, _ := newGuardedMembershipStoreTest(t)
	for _, pair := range []struct{ space, user string }{{"space-a", "invalid"}, {"missing-space", owner}} {
		if deleted, err := store.DeleteWorkspaceMemberGuarded(ctx, pair.space, pair.user, true); err == nil || deleted {
			t.Fatalf("invalid target: space=%s deleted=%v err=%v", pair.space, deleted, err)
		}
	}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if deleted, err := store.DeleteWorkspaceMemberGuarded(canceled, "space-a", owner, true); deleted || !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation must survive Begin failure: deleted=%v err=%v", deleted, err)
	}
	stored, err := store.GetWorkspaceMember(ctx, "space-a", owner)
	if err != nil || stored.Role != WorkspaceRoleOwner {
		t.Fatal("failed deletion damaged the committed owner", err)
	}
}

func TestPostgresStoreDeleteWorkspaceMemberGuardedConcurrentOwnerRemoval(t *testing.T) {
	for _, mode := range []string{"delete-delete", "delete-demotion"} {
		t.Run(mode, func(t *testing.T) {
			ctx, store, owner, secondOwner, _ := newGuardedMembershipStoreTest(t)
			if err := store.UpsertWorkspaceMember(ctx, &domain.WorkspaceMember{WorkspaceID: "space-a", UserID: secondOwner, Role: WorkspaceRoleOwner}); err != nil {
				t.Fatal(err)
			}
			start := make(chan struct{})
			results := make(chan error, 2)
			remove := func(id string) error {
				deleted, err := store.DeleteWorkspaceMemberGuarded(ctx, "space-a", id, true)
				if err == nil && !deleted {
					return fmt.Errorf("existing owner %s was not removed", id)
				}
				return err
			}
			go func() {
				<-start
				results <- remove(owner)
			}()
			go func() {
				<-start
				if mode == "delete-demotion" {
					results <- store.UpsertWorkspaceMemberGuarded(ctx, &domain.WorkspaceMember{WorkspaceID: "space-a", UserID: secondOwner, Role: WorkspaceRoleViewer}, true)
					return
				}
				results <- remove(secondOwner)
			}()
			close(start)
			succeeded, rejected := 0, 0
			for range 2 {
				select {
				case err := <-results:
					if err == nil {
						succeeded++
					} else if errors.Is(err, ErrLastOwner) {
						rejected++
					} else {
						t.Fatalf("unexpected concurrent result: %v", err)
					}
				case <-ctx.Done():
					t.Fatal("concurrent operations did not finish", ctx.Err())
				}
			}
			if succeeded != 1 || rejected != 1 {
				t.Fatalf("success=%d last-owner rejection=%d, want 1/1", succeeded, rejected)
			}
			members, err := store.ListWorkspaceMembers(ctx, "space-a")
			if err != nil {
				t.Fatal(err)
			}
			owners := 0
			for _, member := range members {
				if member.Role == WorkspaceRoleOwner {
					owners++
				}
			}
			if owners != 1 {
				t.Fatalf("remaining owners=%d, want 1", owners)
			}
		})
	}
}
