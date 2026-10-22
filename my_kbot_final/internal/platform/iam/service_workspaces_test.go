package iam

import (
	"context"
	"errors"
	"github.com/google/uuid"
	"my_kbot_final/internal/domain"
	"reflect"
	"testing"
)

// ---- service_workspace_create_test.go ----

type workspaceCreateStoreStub struct {
	Store
	create func(context.Context, *domain.Workspace) error
	upsert func(context.Context, *domain.WorkspaceMember) error
}

func (s *workspaceCreateStoreStub) CreateWorkspace(ctx context.Context, ws *domain.Workspace) error {
	return s.create(ctx, ws)
}

func (s *workspaceCreateStoreStub) UpsertWorkspaceMember(ctx context.Context, member *domain.WorkspaceMember) error {
	return s.upsert(ctx, member)
}

type atomicWorkspaceCreateStoreStub struct {
	*workspaceCreateStoreStub
	atomic func(context.Context, *domain.Workspace, string) error
}

func (s *atomicWorkspaceCreateStoreStub) CreateWorkspaceWithOwner(ctx context.Context, ws *domain.Workspace, actorID string) error {
	return s.atomic(ctx, ws, actorID)
}

func TestServiceCreateWorkspaceForUserAtomic(t *testing.T) {
	type key struct{}
	ctx := context.WithValue(context.Background(), key{}, "request")
	actorID, parentID := uuid.NewString(), "parent-space"
	var saved *domain.Workspace
	calls := 0
	base := &workspaceCreateStoreStub{
		create: func(context.Context, *domain.Workspace) error {
			t.Fatal("atomic path must not separately create workspace")
			return nil
		},
		upsert: func(context.Context, *domain.WorkspaceMember) error {
			t.Fatal("atomic path must not separately write owner")
			return nil
		},
	}
	store := &atomicWorkspaceCreateStoreStub{workspaceCreateStoreStub: base, atomic: func(got context.Context, ws *domain.Workspace, actor string) error {
		calls++
		if got.Value(key{}) != "request" || actor != actorID || ws.Name != "电商运营" || ws.Description != "订单与库存" || ws.ParentID == nil || *ws.ParentID != parentID {
			t.Fatal("atomic creation lost request context, actor or workspace fields")
		}
		if _, err := uuid.Parse(ws.ID); err != nil || ws.CreatedAt.IsZero() || ws.UpdatedAt.IsZero() {
			t.Fatal("Service must generate workspace identity and timestamps")
		}
		saved = ws
		return nil
	}}
	got, err := NewService(store, nil).CreateWorkspaceForUser(ctx, actorID, "电商运营", "订单与库存", &parentID)
	if err != nil || got == nil || got != saved || calls != 1 {
		t.Fatalf("atomic creation response=%+v calls=%d err=%v", got, calls, err)
	}
}

func TestServiceCreateWorkspaceForUserAtomicFailure(t *testing.T) {
	for _, failure := range []error{errors.New("atomic storage failed"), context.Canceled} {
		store := &atomicWorkspaceCreateStoreStub{
			workspaceCreateStoreStub: &workspaceCreateStoreStub{
				create: func(context.Context, *domain.Workspace) error {
					t.Fatal("atomic failure must not retry through non-atomic creation")
					return nil
				},
				upsert: func(context.Context, *domain.WorkspaceMember) error {
					t.Fatal("atomic failure must not fall back to owner write")
					return nil
				},
			},
			atomic: func(context.Context, *domain.Workspace, string) error { return failure },
		}
		ws, err := NewService(store, nil).CreateWorkspaceForUser(context.Background(), uuid.NewString(), "A", "", nil)
		if ws != nil || !errors.Is(err, failure) {
			t.Fatalf("atomic failure must be returned with nil workspace: %+v err=%v", ws, err)
		}
	}
}

func TestServiceCreateWorkspaceForUserFallback(t *testing.T) {
	for _, stage := range []string{"success", "create failure", "member failure"} {
		t.Run(stage, func(t *testing.T) {
			type key struct{}
			ctx := context.WithValue(context.Background(), key{}, "fallback-request")
			actorID := uuid.NewString()
			failure := errors.New("storage failed")
			var saved *domain.Workspace
			var events []string
			store := &workspaceCreateStoreStub{
				create: func(got context.Context, ws *domain.Workspace) error {
					events = append(events, "workspace")
					if got.Value(key{}) != "fallback-request" || ws.Name != "A" {
						t.Fatal("fallback workspace context/input lost")
					}
					if stage == "create failure" {
						return failure
					}
					saved = ws
					return nil
				},
				upsert: func(got context.Context, member *domain.WorkspaceMember) error {
					events = append(events, "owner")
					if got.Value(key{}) != "fallback-request" || saved == nil || member.WorkspaceID != saved.ID || member.UserID != actorID || member.Role != "owner" || member.CreatedAt.IsZero() {
						t.Fatal("fallback owner must reference the just-created space and actor")
					}
					if stage == "member failure" {
						return failure
					}
					return nil
				},
			}
			ws, err := NewService(store, nil).CreateWorkspaceForUser(ctx, actorID, "A", "", nil)
			if stage == "success" {
				if err != nil || ws == nil || ws != saved || !reflect.DeepEqual(events, []string{"workspace", "owner"}) {
					t.Fatalf("fallback order=%v workspace=%+v err=%v", events, ws, err)
				}
			} else {
				if ws != nil || !errors.Is(err, failure) {
					t.Fatalf("fallback error not preserved: %+v err=%v", ws, err)
				}
				if stage == "create failure" && !reflect.DeepEqual(events, []string{"workspace"}) {
					t.Fatal("failed creation must stop before owner write")
				}
			}
		})
	}
}

func TestServiceCreateWorkspaceForUserPostgres(t *testing.T) {
	ctx, store := newPostgresUserStoreTest(t)
	creator := &domain.User{ID: uuid.NewString(), Email: "creator@example.com", Name: "创建者"}
	if err := store.CreateUser(ctx, creator); err != nil {
		t.Fatal(err)
	}
	service := NewService(store, nil)
	first, err := service.CreateWorkspaceForUser(ctx, creator.ID, "父空间", "根空间", nil)
	if err != nil || first == nil {
		t.Fatalf("create parent: %v", err)
	}
	second, err := service.CreateWorkspaceForUser(ctx, creator.ID, "子空间", "子空间说明", &first.ID)
	if err != nil || second == nil || second.ID == first.ID {
		t.Fatalf("create child: %v", err)
	}
	for _, ws := range []*domain.Workspace{first, second} {
		if _, err := uuid.Parse(ws.ID); err != nil {
			t.Fatal("Service must generate valid UUIDs")
		}
		stored, err := store.GetWorkspaceByID(ctx, ws.ID)
		if err != nil || stored == nil || stored.Name != ws.Name || stored.Description != ws.Description || stored.CreatedAt.IsZero() {
			t.Fatal("created workspace not persisted")
		}
		if ws.ID == second.ID && (stored.ParentID == nil || *stored.ParentID != first.ID) {
			t.Fatal("child parent not persisted")
		}
		member, err := store.GetWorkspaceMember(ctx, ws.ID, creator.ID)
		if err != nil || member == nil || member.Role != "owner" {
			t.Fatal("creator must become owner")
		}
	}
	user, err := store.GetUserByID(ctx, creator.ID)
	if err != nil || user == nil || user.Role != GlobalRoleMember {
		t.Fatal("space ownership must not promote the global user role")
	}
	spaces, err := store.ListUserWorkspaces(ctx, creator.ID, 10, 0)
	if err != nil || len(spaces) != 2 {
		t.Fatal("both created spaces must be visible through memberships")
	}
}

// ---- service_workspace_access_test.go ----

type workspaceAccessStoreStub struct {
	Store
	user func(context.Context, string) (*domain.User, error)
	all  func(context.Context, int32, int32) ([]*domain.Workspace, error)
	mine func(context.Context, string, int32, int32) ([]*domain.Workspace, error)
}

func (s *workspaceAccessStoreStub) GetUserByID(ctx context.Context, id string) (*domain.User, error) {
	return s.user(ctx, id)
}

func (s *workspaceAccessStoreStub) ListWorkspaces(ctx context.Context, limit, offset int32) ([]*domain.Workspace, error) {
	return s.all(ctx, limit, offset)
}

func (s *workspaceAccessStoreStub) ListUserWorkspaces(ctx context.Context, userID string, limit, offset int32) ([]*domain.Workspace, error) {
	return s.mine(ctx, userID, limit, offset)
}

func TestServiceIsGlobalAdmin(t *testing.T) {
	for _, tc := range []struct {
		role, status string
		want         bool
	}{
		{"admin", "active", true}, {"member", "active", false},
		{"owner", "active", false}, {"admin", "inactive", false}, {"admin", "suspended", false},
	} {
		t.Run(tc.role+"/"+tc.status, func(t *testing.T) {
			store := &workspaceAccessStoreStub{user: func(context.Context, string) (*domain.User, error) {
				return &domain.User{Role: tc.role, Status: tc.status}, nil
			}}
			got, err := NewService(store, nil).IsGlobalAdmin(context.Background(), "user-1")
			if err != nil || got != tc.want {
				t.Fatalf("admin=%v err=%v, want %v", got, err, tc.want)
			}
		})
	}
}

func TestServiceListAccessibleWorkspaces(t *testing.T) {
	for _, tc := range []struct {
		role             string
		limit, wantLimit int32
	}{
		{"admin", 20, 20}, {"member", 20, 20}, {"admin", 0, 50}, {"member", -1, 50},
	} {
		type key struct{}
		ctx := context.WithValue(context.Background(), key{}, "request")
		var calls []string
		want := []*domain.Workspace{{ID: "visible-space", Name: "可访问空间"}}
		check := func(got context.Context, limit, offset int32) {
			t.Helper()
			if got.Value(key{}) != "request" || limit != tc.wantLimit || offset != 5 {
				t.Fatalf("request context/page lost: limit=%d offset=%d", limit, offset)
			}
		}
		store := &workspaceAccessStoreStub{
			user: func(got context.Context, id string) (*domain.User, error) {
				calls = append(calls, "user")
				if got.Value(key{}) != "request" || id != "current-user" {
					t.Fatal("current identity not forwarded")
				}
				return &domain.User{ID: id, Role: tc.role, Status: "active"}, nil
			},
			all: func(got context.Context, limit, offset int32) ([]*domain.Workspace, error) {
				calls = append(calls, "all")
				check(got, limit, offset)
				if tc.role != "admin" {
					t.Fatal("ordinary user must not query all spaces")
				}
				return want, nil
			},
			mine: func(got context.Context, id string, limit, offset int32) ([]*domain.Workspace, error) {
				calls = append(calls, "mine")
				check(got, limit, offset)
				if tc.role != "member" || id != "current-user" {
					t.Fatal("member-scoped query must use the current user")
				}
				return want, nil
			},
		}
		got, err := NewService(store, nil).ListAccessibleWorkspaces(ctx, "current-user", tc.limit, 5)
		if err != nil || len(got) != 1 || got[0] != want[0] || len(calls) != 2 || calls[0] != "user" {
			t.Fatalf("result=%v calls=%v err=%v", got, calls, err)
		}
	}
}

func TestServiceWorkspaceAccessErrors(t *testing.T) {
	for _, stage := range []string{"user", "all", "mine"} {
		t.Run(stage, func(t *testing.T) {
			failure := errors.New("store failed")
			listed := false
			store := &workspaceAccessStoreStub{
				user: func(context.Context, string) (*domain.User, error) {
					if stage == "user" {
						return nil, failure
					}
					role := GlobalRoleMember
					if stage == "all" {
						role = GlobalRoleAdmin
					}
					return &domain.User{Role: role, Status: "active"}, nil
				},
				all: func(context.Context, int32, int32) ([]*domain.Workspace, error) { listed = true; return nil, failure },
				mine: func(context.Context, string, int32, int32) ([]*domain.Workspace, error) {
					listed = true
					return nil, failure
				},
			}
			service := NewService(store, nil)
			if stage == "user" {
				if admin, err := service.IsGlobalAdmin(context.Background(), "user-1"); admin || !errors.Is(err, failure) {
					t.Fatal("identity lookup failure lost")
				}
			}
			rows, err := service.ListAccessibleWorkspaces(context.Background(), "user-1", 10, 0)
			if rows != nil || !errors.Is(err, failure) || stage == "user" && listed {
				t.Fatalf("error handling: rows=%v listed=%v err=%v", rows, listed, err)
			}
		})
	}
}

func TestServiceListAccessibleWorkspacesPostgres(t *testing.T) {
	ctx, store := newPostgresUserStoreTest(t)
	x, y, admin := uuid.NewString(), uuid.NewString(), uuid.NewString()
	for _, user := range []*domain.User{
		{ID: x, Email: "x@example.com", Role: GlobalRoleMember},
		{ID: y, Email: "y@example.com", Role: GlobalRoleMember},
		{ID: admin, Email: "admin@example.com", Role: GlobalRoleAdmin},
	} {
		if err := store.CreateUser(ctx, user); err != nil {
			t.Fatal(err)
		}
	}
	service := NewService(store, nil)
	a, err := service.CreateWorkspaceForUser(ctx, x, "A", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	b, err := service.CreateWorkspaceForUser(ctx, y, "B", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	assertSpaces := func(id string, expected ...string) {
		t.Helper()
		spaces, err := service.ListAccessibleWorkspaces(ctx, id, 10, 0)
		if err != nil || spaces == nil || len(spaces) != len(expected) {
			t.Fatalf("spaces=%v err=%v, want %v", spaces, err, expected)
		}
		seen := map[string]bool{}
		for _, ws := range spaces {
			seen[ws.ID] = true
		}
		for _, id := range expected {
			if !seen[id] {
				t.Fatalf("missing accessible workspace %s", id)
			}
		}
	}
	assertSpaces(admin, a.ID, b.ID)
	assertSpaces(x, a.ID)
	assertSpaces(y, b.ID)
	if err := store.UpsertWorkspaceMember(ctx, &domain.WorkspaceMember{WorkspaceID: b.ID, UserID: x, Role: "viewer"}); err != nil {
		t.Fatal(err)
	}
	assertSpaces(x, a.ID, b.ID)
	if deleted, err := store.DeleteWorkspaceMember(ctx, b.ID, x); err != nil || !deleted {
		t.Fatal("remove fixture membership")
	}
	assertSpaces(x, a.ID)
	if err := store.SetUserRole(ctx, admin, GlobalRoleMember); err != nil {
		t.Fatal(err)
	}
	assertSpaces(admin)
	if err := store.SetUserRole(ctx, admin, GlobalRoleAdmin); err != nil {
		t.Fatal(err)
	}
	assertSpaces(admin, a.ID, b.ID)
}
