package iam

import (
	"context"
	"errors"
	"github.com/google/uuid"
	"my_kbot_final/internal/domain"
	"testing"
)

// ---- service_workspace_role_test.go ----

type workspaceRoleStoreStub struct {
	Store
	user   func(context.Context, string) (*domain.User, error)
	member func(context.Context, string, string) (*domain.WorkspaceMember, error)
}

func (s *workspaceRoleStoreStub) GetUserByID(ctx context.Context, id string) (*domain.User, error) {
	return s.user(ctx, id)
}
func (s *workspaceRoleStoreStub) GetWorkspaceMember(ctx context.Context, workspaceID, userID string) (*domain.WorkspaceMember, error) {
	return s.member(ctx, workspaceID, userID)
}

func TestServiceWorkspaceRole(t *testing.T) {
	for _, tc := range []struct{ name, globalRole, memberRole, want string }{
		{"global administrator", GlobalRoleAdmin, "viewer", WorkspaceRoleAdmin},
		{"workspace owner", GlobalRoleMember, "owner", WorkspaceRoleOwner},
		{"workspace administrator", GlobalRoleMember, "admin", WorkspaceRoleAdmin},
		{"editor", GlobalRoleMember, "editor", WorkspaceRoleEditor},
		{"member", GlobalRoleMember, "member", WorkspaceRoleMember},
		{"viewer", GlobalRoleMember, "viewer", WorkspaceRoleViewer},
	} {
		t.Run(tc.name, func(t *testing.T) {
			type requestKey struct{}
			ctx := context.WithValue(context.Background(), requestKey{}, "request")
			store := &workspaceRoleStoreStub{
				user: func(got context.Context, id string) (*domain.User, error) {
					if id != "actor" || got.Value(requestKey{}) != "request" {
						t.Fatal("identity/context lost")
					}
					return &domain.User{ID: id, Status: "active", Role: tc.globalRole}, nil
				},
				member: func(got context.Context, workspaceID, userID string) (*domain.WorkspaceMember, error) {
					if tc.globalRole == GlobalRoleAdmin {
						t.Fatal("global administrator does not require membership")
					}
					if workspaceID != "space-a" || userID != "actor" || got.Value(requestKey{}) != "request" {
						t.Fatal("membership must use both current IDs/context")
					}
					return &domain.WorkspaceMember{Role: tc.memberRole}, nil
				},
			}
			role, err := NewService(store, nil).WorkspaceRole(ctx, "actor", "space-a")
			if err != nil || role != tc.want {
				t.Fatalf("role=%q err=%v, want %q", role, err, tc.want)
			}
		})
	}
}

func TestServiceWorkspaceRoleErrors(t *testing.T) {
	for _, ids := range [][2]string{{"", "space-a"}, {"actor", ""}, {"", ""}} {
		role, err := NewService(&workspaceRoleStoreStub{}, nil).WorkspaceRole(context.Background(), ids[0], ids[1])
		if role != "" || !errors.Is(err, ErrForbidden) {
			t.Fatalf("empty identity must be rejected before Store access: role=%q err=%v", role, err)
		}
	}
	for _, tc := range []struct {
		name                     string
		user                     *domain.User
		userErr, memberErr, want error
	}{
		{"user lookup failure", nil, context.Canceled, nil, context.Canceled},
		{"inactive global administrator", &domain.User{Role: GlobalRoleAdmin, Status: "inactive"}, nil, nil, ErrForbidden},
		{"inactive owner", &domain.User{Role: GlobalRoleMember, Status: "inactive"}, nil, nil, ErrForbidden},
		{"not a member", &domain.User{Role: GlobalRoleMember, Status: "active"}, nil, errors.New("membership not found"), ErrForbidden},
		{"membership query failure", &domain.User{Role: GlobalRoleMember, Status: "active"}, nil, errors.New("database unavailable"), ErrForbidden},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store := &workspaceRoleStoreStub{
				user: func(context.Context, string) (*domain.User, error) { return tc.user, tc.userErr },
				member: func(context.Context, string, string) (*domain.WorkspaceMember, error) {
					if tc.userErr != nil || tc.user.Status != "active" {
						t.Fatal("denied identity must not query memberships")
					}
					return nil, tc.memberErr
				},
			}
			role, err := NewService(store, nil).WorkspaceRole(context.Background(), "actor", "space-a")
			if role != "" || !errors.Is(err, tc.want) {
				t.Fatalf("role=%q err=%v, want %v", role, err, tc.want)
			}
		})
	}
}

func TestServiceCanManageWorkspace(t *testing.T) {
	for _, tc := range []struct {
		role string
		want bool
	}{
		{WorkspaceRoleOwner, true}, {WorkspaceRoleAdmin, true}, {WorkspaceRoleEditor, false}, {WorkspaceRoleMember, false}, {WorkspaceRoleViewer, false},
	} {
		t.Run(tc.role, func(t *testing.T) {
			store := &workspaceRoleStoreStub{
				user: func(context.Context, string) (*domain.User, error) {
					return &domain.User{Role: GlobalRoleMember, Status: "active"}, nil
				},
				member: func(context.Context, string, string) (*domain.WorkspaceMember, error) {
					return &domain.WorkspaceMember{Role: tc.role}, nil
				},
			}
			ok, err := NewService(store, nil).CanManageWorkspace(context.Background(), "actor", "space-a")
			if err != nil || ok != tc.want {
				t.Fatalf("manage=%v err=%v, want %v", ok, err, tc.want)
			}
		})
	}
	ok, err := NewService(&workspaceRoleStoreStub{}, nil).CanManageWorkspace(context.Background(), "actor", "")
	if ok || !errors.Is(err, ErrForbidden) {
		t.Fatalf("permission error lost: manage=%v err=%v", ok, err)
	}
}

func TestServiceWorkspaceRolePostgres(t *testing.T) {
	ctx, store, tx := newPostgresStoreTransactionTest(t)
	memberID, adminID := uuid.NewString(), uuid.NewString()
	for _, user := range []*domain.User{
		{ID: memberID, Email: "member@example.com", Role: GlobalRoleMember},
		{ID: adminID, Email: "admin@example.com", Role: GlobalRoleAdmin},
	} {
		if err := store.CreateUser(ctx, user); err != nil {
			t.Fatal(err)
		}
	}
	for _, id := range []string{"space-a", "space-b"} {
		if err := store.CreateWorkspace(ctx, &domain.Workspace{ID: id, Name: id}); err != nil {
			t.Fatal(err)
		}
	}
	service := NewService(store, nil)
	assertRole := func(userID, spaceID, want string, wantManage bool) {
		t.Helper()
		role, err := service.WorkspaceRole(ctx, userID, spaceID)
		if err != nil || role != want {
			t.Fatalf("role=%q err=%v, want %q", role, err, want)
		}
		ok, err := service.CanManageWorkspace(ctx, userID, spaceID)
		if err != nil || ok != wantManage {
			t.Fatalf("manage=%v err=%v, want %v", ok, err, wantManage)
		}
	}
	assertDenied := func(userID, spaceID string) {
		t.Helper()
		role, err := service.WorkspaceRole(ctx, userID, spaceID)
		if role != "" || !errors.Is(err, ErrForbidden) {
			t.Fatalf("expected forbidden: role=%q err=%v", role, err)
		}
	}
	for _, fixture := range []struct{ space, role string }{{"space-a", "owner"}, {"space-b", "viewer"}} {
		if err := store.UpsertWorkspaceMember(ctx, &domain.WorkspaceMember{WorkspaceID: fixture.space, UserID: memberID, Role: fixture.role}); err != nil {
			t.Fatal(err)
		}
	}
	assertRole(memberID, "space-a", WorkspaceRoleOwner, true)
	assertRole(memberID, "space-b", WorkspaceRoleViewer, false)
	assertRole(adminID, "space-a", WorkspaceRoleAdmin, true) // No membership row required.
	if err := store.UpsertWorkspaceMember(ctx, &domain.WorkspaceMember{WorkspaceID: "space-a", UserID: memberID, Role: "editor"}); err != nil {
		t.Fatal(err)
	}
	assertRole(memberID, "space-a", WorkspaceRoleEditor, false)
	if deleted, err := store.DeleteWorkspaceMember(ctx, "space-b", memberID); err != nil || !deleted {
		t.Fatal("remove fixture membership")
	}
	assertDenied(memberID, "space-b")
	if err := store.SetUserRole(ctx, adminID, GlobalRoleMember); err != nil {
		t.Fatal(err)
	}
	assertDenied(adminID, "space-a")
	if _, err := tx.Exec(ctx, "UPDATE users SET status='inactive' WHERE id=$1", uuid.MustParse(memberID)); err != nil {
		t.Fatal(err)
	}
	assertDenied(memberID, "space-a")
}

// ---- service_workspace_authority_test.go ----

func TestServiceWorkspaceManagementAuthority(t *testing.T) {
	for _, tc := range []struct {
		name, globalRole, memberRole string
		manageOwners, forbidden      bool
	}{
		{"global administrator", GlobalRoleAdmin, WorkspaceRoleViewer, true, false},
		{"workspace owner", GlobalRoleMember, WorkspaceRoleOwner, true, false},
		{"workspace administrator", GlobalRoleMember, WorkspaceRoleAdmin, false, false},
		{"editor", GlobalRoleMember, WorkspaceRoleEditor, false, true},
		{"member", GlobalRoleMember, WorkspaceRoleMember, false, true},
		{"viewer", GlobalRoleMember, WorkspaceRoleViewer, false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			type key struct{}
			ctx := context.WithValue(context.Background(), key{}, "request")
			store := &workspaceRoleStoreStub{
				user: func(got context.Context, id string) (*domain.User, error) {
					if id != "actor" || got.Value(key{}) != "request" {
						t.Fatal("caller identity/context lost")
					}
					return &domain.User{ID: id, Role: tc.globalRole, Status: "active"}, nil
				},
				member: func(got context.Context, workspaceID, userID string) (*domain.WorkspaceMember, error) {
					if tc.globalRole == GlobalRoleAdmin {
						t.Fatal("global admin must not need a membership")
					}
					if workspaceID != "space-a" || userID != "actor" || got.Value(key{}) != "request" {
						t.Fatal("target workspace/actor/context lost")
					}
					return &domain.WorkspaceMember{Role: tc.memberRole}, nil
				},
			}
			manageOwners, err := NewService(store, nil).workspaceManagementAuthority(ctx, "actor", "space-a")
			if manageOwners != tc.manageOwners {
				t.Fatalf("manageOwners=%v, want %v", manageOwners, tc.manageOwners)
			}
			if tc.forbidden {
				if !errors.Is(err, ErrForbidden) {
					t.Fatalf("expected forbidden, got %v", err)
				}
			} else if err != nil {
				t.Fatalf("qualified manager rejected: %v", err)
			}
		})
	}
}

func TestServiceWorkspaceManagementAuthorityErrors(t *testing.T) {
	for _, ids := range [][2]string{{"", "space-a"}, {"actor", ""}, {"", ""}} {
		manageOwners, err := NewService(&workspaceRoleStoreStub{}, nil).workspaceManagementAuthority(context.Background(), ids[0], ids[1])
		if manageOwners || !errors.Is(err, ErrForbidden) {
			t.Fatalf("empty input must be denied before Store access: %v", err)
		}
	}
	for _, stage := range []string{"user lookup", "inactive admin", "inactive owner", "no membership", "membership failure"} {
		t.Run(stage, func(t *testing.T) {
			store := &workspaceRoleStoreStub{
				user: func(context.Context, string) (*domain.User, error) {
					if stage == "user lookup" {
						return nil, context.Canceled
					}
					user := &domain.User{ID: "actor", Role: GlobalRoleMember, Status: "active"}
					if stage == "inactive admin" {
						user.Role = GlobalRoleAdmin
						user.Status = "inactive"
					}
					if stage == "inactive owner" {
						user.Status = "inactive"
					}
					return user, nil
				},
				member: func(context.Context, string, string) (*domain.WorkspaceMember, error) {
					if stage == "user lookup" || stage == "inactive admin" || stage == "inactive owner" {
						t.Fatal("invalid account must be denied before membership query")
					}
					return nil, errors.New(stage)
				},
			}
			manageOwners, err := NewService(store, nil).workspaceManagementAuthority(context.Background(), "actor", "space-a")
			if manageOwners || !errors.Is(err, ErrForbidden) {
				t.Fatalf("expected forbidden: manageOwners=%v err=%v", manageOwners, err)
			}
		})
	}
}

func TestServiceWorkspaceManagementAuthorityPostgres(t *testing.T) {
	ctx, store, tx := newPostgresStoreTransactionTest(t)
	owner, spaceAdmin, globalAdmin := uuid.NewString(), uuid.NewString(), uuid.NewString()
	for _, user := range []*domain.User{
		{ID: owner, Email: "owner@example.com", Role: GlobalRoleMember},
		{ID: spaceAdmin, Email: "space-admin@example.com", Role: GlobalRoleMember},
		{ID: globalAdmin, Email: "global-admin@example.com", Role: GlobalRoleAdmin},
	} {
		if err := store.CreateUser(ctx, user); err != nil {
			t.Fatal(err)
		}
	}
	service := NewService(store, nil)
	a, err := service.CreateWorkspaceForUser(ctx, owner, "A", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	b, err := service.CreateWorkspaceForUser(ctx, spaceAdmin, "B", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertWorkspaceMember(ctx, &domain.WorkspaceMember{WorkspaceID: a.ID, UserID: spaceAdmin, Role: WorkspaceRoleAdmin}); err != nil {
		t.Fatal(err)
	}
	assertAuthority := func(actor, space string, wantOwners, forbidden bool) {
		t.Helper()
		manageOwners, err := service.workspaceManagementAuthority(ctx, actor, space)
		if manageOwners != wantOwners || forbidden && !errors.Is(err, ErrForbidden) || !forbidden && err != nil {
			t.Fatalf("manageOwners=%v err=%v, want owners=%v forbidden=%v", manageOwners, err, wantOwners, forbidden)
		}
	}
	assertAuthority(owner, a.ID, true, false)
	assertAuthority(spaceAdmin, a.ID, false, false)
	assertAuthority(spaceAdmin, b.ID, true, false)
	assertAuthority(owner, b.ID, false, true)
	assertAuthority(globalAdmin, a.ID, true, false)
	// Both return effective role admin, yet their owner-management authority differs.
	for _, actor := range []string{spaceAdmin, globalAdmin} {
		role, err := service.WorkspaceRole(ctx, actor, a.ID)
		if err != nil || role != WorkspaceRoleAdmin {
			t.Fatal("fixture must expose the effective-role collision")
		}
	}
	if err := store.UpsertWorkspaceMember(ctx, &domain.WorkspaceMember{WorkspaceID: a.ID, UserID: spaceAdmin, Role: WorkspaceRoleEditor}); err != nil {
		t.Fatal(err)
	}
	assertAuthority(spaceAdmin, a.ID, false, true)
	if err := store.SetUserRole(ctx, globalAdmin, GlobalRoleMember); err != nil {
		t.Fatal(err)
	}
	assertAuthority(globalAdmin, a.ID, false, true)
	if _, err := tx.Exec(ctx, "UPDATE users SET status='inactive' WHERE id=$1", uuid.MustParse(owner)); err != nil {
		t.Fatal(err)
	}
	assertAuthority(owner, a.ID, false, true)
}

// ---- service_workspace_members_list_test.go ----

type workspaceMemberListStoreStub struct {
	*workspaceRoleStoreStub
	list func(context.Context, string) ([]*domain.WorkspaceMember, error)
}

func (s *workspaceMemberListStoreStub) ListWorkspaceMembers(ctx context.Context, id string) ([]*domain.WorkspaceMember, error) {
	return s.list(ctx, id)
}

func TestServiceListWorkspaceMembersPermissions(t *testing.T) {
	for _, tc := range []struct {
		name, globalRole, workspaceRole string
		allowed                         bool
	}{
		{"global administrator", GlobalRoleAdmin, "", true},
		{"owner", GlobalRoleMember, WorkspaceRoleOwner, true},
		{"workspace administrator", GlobalRoleMember, WorkspaceRoleAdmin, true},
		{"editor", GlobalRoleMember, WorkspaceRoleEditor, false},
		{"member", GlobalRoleMember, WorkspaceRoleMember, false},
		{"viewer", GlobalRoleMember, WorkspaceRoleViewer, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			type key struct{}
			ctx := context.WithValue(context.Background(), key{}, "request")
			listed := false
			store := &workspaceMemberListStoreStub{
				workspaceRoleStoreStub: &workspaceRoleStoreStub{
					user: func(context.Context, string) (*domain.User, error) {
						return &domain.User{ID: "actor", Role: tc.globalRole, Status: "active"}, nil
					},
					member: func(context.Context, string, string) (*domain.WorkspaceMember, error) {
						return &domain.WorkspaceMember{Role: tc.workspaceRole}, nil
					},
				},
				list: func(got context.Context, id string) ([]*domain.WorkspaceMember, error) {
					listed = true
					if !tc.allowed {
						t.Fatal("denied actor must not read the member list")
					}
					if id != "space-a" || got.Value(key{}) != "request" {
						t.Fatal("workspace/context lost")
					}
					return []*domain.WorkspaceMember{{WorkspaceID: id, UserID: "target", Role: WorkspaceRoleViewer}}, nil
				},
			}
			members, err := NewService(store, nil).ListWorkspaceMembers(ctx, "actor", "space-a")
			if tc.allowed {
				if err != nil || !listed || len(members) != 1 || members[0].UserID != "target" {
					t.Fatalf("members=%v listed=%v err=%v", members, listed, err)
				}
			} else if members != nil || listed || !errors.Is(err, ErrForbidden) {
				t.Fatalf("members=%v listed=%v err=%v", members, listed, err)
			}
		})
	}
}

func TestServiceListWorkspaceMembersErrors(t *testing.T) {
	for _, stage := range []string{"identity", "membership", "list", "empty"} {
		t.Run(stage, func(t *testing.T) {
			failure := errors.New("store failure")
			store := &workspaceMemberListStoreStub{
				workspaceRoleStoreStub: &workspaceRoleStoreStub{
					user: func(context.Context, string) (*domain.User, error) {
						if stage == "identity" {
							return nil, failure
						}
						return &domain.User{ID: "actor", Role: GlobalRoleMember, Status: "active"}, nil
					},
					member: func(context.Context, string, string) (*domain.WorkspaceMember, error) {
						if stage == "membership" {
							return nil, failure
						}
						return &domain.WorkspaceMember{Role: WorkspaceRoleOwner}, nil
					},
				},
				list: func(context.Context, string) ([]*domain.WorkspaceMember, error) {
					if stage == "identity" || stage == "membership" {
						t.Fatal("authorization failure must short circuit the list query")
					}
					if stage == "list" {
						return nil, failure
					}
					return []*domain.WorkspaceMember{}, nil
				},
			}
			members, err := NewService(store, nil).ListWorkspaceMembers(context.Background(), "actor", "space-a")
			switch stage {
			case "identity", "membership":
				if members != nil || !errors.Is(err, ErrForbidden) {
					t.Fatalf("expected forbidden, got %v", err)
				}
			case "list":
				if members != nil || !errors.Is(err, failure) {
					t.Fatalf("list query error lost: %v", err)
				}
			case "empty":
				if err != nil || members == nil || len(members) != 0 {
					t.Fatalf("empty list=%v err=%v", members, err)
				}
			}
		})
	}
}

func TestServiceListWorkspaceMembersPostgres(t *testing.T) {
	ctx, store := newPostgresUserStoreTest(t)
	owner, other, admin := uuid.NewString(), uuid.NewString(), uuid.NewString()
	for _, user := range []*domain.User{
		{ID: owner, Email: "owner@example.com", Name: "A owner", Role: GlobalRoleMember},
		{ID: other, Email: "other@example.com", Name: "B owner", Role: GlobalRoleMember},
		{ID: admin, Email: "admin@example.com", Name: "Global admin", Role: GlobalRoleAdmin},
	} {
		if err := store.CreateUser(ctx, user); err != nil {
			t.Fatal(err)
		}
	}
	service := NewService(store, nil)
	a, err := service.CreateWorkspaceForUser(ctx, owner, "A", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	b, err := service.CreateWorkspaceForUser(ctx, other, "B", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	put := func(space, user, role string) {
		t.Helper()
		if err := store.UpsertWorkspaceMember(ctx, &domain.WorkspaceMember{WorkspaceID: space, UserID: user, Role: role}); err != nil {
			t.Fatal(err)
		}
	}
	put(a.ID, other, WorkspaceRoleViewer)
	put(b.ID, owner, WorkspaceRoleViewer)
	assertList := func(actor, space string, wantIDs ...string) {
		t.Helper()
		members, err := service.ListWorkspaceMembers(ctx, actor, space)
		if err != nil || len(members) != len(wantIDs) {
			t.Fatalf("members=%v err=%v, want %d", members, err, len(wantIDs))
		}
		seen := map[string]bool{}
		for _, member := range members {
			if member.WorkspaceID != space || member.Email == "" || member.Name == "" {
				t.Fatal("membership scope or joined profile lost")
			}
			seen[member.UserID] = true
		}
		for _, id := range wantIDs {
			if !seen[id] {
				t.Fatalf("expected member %s", id)
			}
		}
	}
	assertList(owner, a.ID, owner, other)
	assertList(admin, a.ID, owner, other) // Global access does not invent a membership row.
	assertList(other, b.ID, owner, other)
	if members, err := service.ListWorkspaceMembers(ctx, owner, b.ID); members != nil || !errors.Is(err, ErrForbidden) {
		t.Fatal("owner of A must not manage B as viewer")
	}
	if members, err := service.ListWorkspaceMembers(ctx, other, a.ID); members != nil || !errors.Is(err, ErrForbidden) {
		t.Fatal("viewer must not read administrative member list")
	}
	put(a.ID, other, WorkspaceRoleAdmin)
	assertList(other, a.ID, owner, other)
	put(a.ID, admin, WorkspaceRoleViewer)
	assertList(owner, a.ID, owner, other, admin) // Global admin is visible if explicitly registered as a member.
	if deleted, err := store.DeleteWorkspaceMember(ctx, a.ID, admin); err != nil || !deleted {
		t.Fatal("remove admin membership fixture")
	}
	assertList(admin, a.ID, owner, other) // Deleting membership does not revoke the global role.
}
