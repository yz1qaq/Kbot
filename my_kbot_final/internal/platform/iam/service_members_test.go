package iam

import (
	"context"
	"errors"
	"my_kbot_final/internal/domain"
	"strings"
	"testing"
	"time"
)

// ---- service_workspace_member_upsert_test.go ----

type membershipWriteStoreStub struct {
	Store
	user    func(context.Context, string) (*domain.User, error)
	member  func(context.Context, string, string) (*domain.WorkspaceMember, error)
	members func(context.Context, string) ([]*domain.WorkspaceMember, error)
	upsert  func(context.Context, *domain.WorkspaceMember) error
}

func (s *membershipWriteStoreStub) GetUserByID(ctx context.Context, id string) (*domain.User, error) {
	return s.user(ctx, id)
}
func (s *membershipWriteStoreStub) GetWorkspaceMember(ctx context.Context, space, id string) (*domain.WorkspaceMember, error) {
	return s.member(ctx, space, id)
}
func (s *membershipWriteStoreStub) ListWorkspaceMembers(ctx context.Context, space string) ([]*domain.WorkspaceMember, error) {
	return s.members(ctx, space)
}
func (s *membershipWriteStoreStub) UpsertWorkspaceMember(ctx context.Context, member *domain.WorkspaceMember) error {
	return s.upsert(ctx, member)
}

type guardedMembershipWriteStoreStub struct {
	*membershipWriteStoreStub
	guarded func(context.Context, *domain.WorkspaceMember, bool) error
}

func (s *guardedMembershipWriteStoreStub) UpsertWorkspaceMemberGuarded(ctx context.Context, member *domain.WorkspaceMember, owners bool) error {
	return s.guarded(ctx, member, owners)
}
func (s *guardedMembershipWriteStoreStub) DeleteWorkspaceMemberGuarded(context.Context, string, string, bool) (bool, error) {
	return false, errors.New("unexpected guarded deletion in upsert test")
}

// Both methods must be present for the production Store to select the guarded path.
var _ guardedMembershipStore = (*PostgresStore)(nil)
var _ guardedMembershipStore = (*guardedMembershipWriteStoreStub)(nil)

func TestServiceUpsertWorkspaceMemberGuardedDispatch(t *testing.T) {
	for _, tc := range []struct {
		name, globalRole, spaceRole string
		owners                      bool
	}{
		{"global admin", GlobalRoleAdmin, "", true},
		{"owner", GlobalRoleMember, WorkspaceRoleOwner, true},
		{"space admin", GlobalRoleMember, WorkspaceRoleAdmin, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			type requestKey struct{}
			ctx := context.WithValue(context.Background(), requestKey{}, "request")
			written := false
			stored := &domain.WorkspaceMember{WorkspaceID: "space-a", UserID: "target", Role: WorkspaceRoleEditor, Email: "target@example.com", Name: "Target", CreatedAt: time.Now().Add(-time.Hour)}
			base := &membershipWriteStoreStub{
				user: func(got context.Context, id string) (*domain.User, error) {
					if got.Value(requestKey{}) != "request" {
						t.Fatal("request context lost")
					}
					if id == "actor" {
						return &domain.User{ID: id, Role: tc.globalRole, Status: "active"}, nil
					}
					if id != "target" {
						t.Fatalf("unexpected account %q", id)
					}
					return &domain.User{ID: id}, nil
				},
				member: func(got context.Context, space, id string) (*domain.WorkspaceMember, error) {
					if space != "space-a" || got.Value(requestKey{}) != "request" {
						t.Fatal("workspace/context lost")
					}
					if id == "actor" {
						return &domain.WorkspaceMember{Role: tc.spaceRole}, nil
					}
					if id != "target" || !written {
						t.Fatal("target readback must follow the guarded write")
					}
					return stored, nil
				},
				upsert: func(context.Context, *domain.WorkspaceMember) error {
					t.Fatal("guarded Store must not use basic write")
					return nil
				},
			}
			store := &guardedMembershipWriteStoreStub{membershipWriteStoreStub: base, guarded: func(got context.Context, member *domain.WorkspaceMember, owners bool) error {
				if got.Value(requestKey{}) != "request" || member.WorkspaceID != "space-a" || member.UserID != "target" || member.Role != WorkspaceRoleEditor || owners != tc.owners || member.CreatedAt.IsZero() {
					t.Fatalf("guarded write received incorrect identity, target, role or authority: %+v owners=%v", member, owners)
				}
				written = true
				return nil
			}}
			got, err := NewService(store, nil).UpsertWorkspaceMember(ctx, "actor", "space-a", "target", WorkspaceRoleEditor)
			if err != nil || got != stored {
				t.Fatalf("must return joined Store readback: got=%+v err=%v", got, err)
			}
		})
	}
}

func TestServiceUpsertWorkspaceMemberValidationAndErrors(t *testing.T) {
	for _, role := range []string{"owner", "admin", "editor", "member", "viewer"} {
		if !validWorkspaceRole(role) {
			t.Fatalf("valid role rejected: %q", role)
		}
	}
	for _, role := range []string{"", "root", "OWNER", " owner ", "administrator"} {
		if validWorkspaceRole(role) {
			t.Fatalf("invalid role accepted: %q", role)
		}
	}
	for _, stage := range []string{"denied", "invalid role", "target missing", "guarded write", "readback"} {
		t.Run(stage, func(t *testing.T) {
			sentinel := errors.New(stage)
			wrote := false
			base := &membershipWriteStoreStub{
				user: func(_ context.Context, id string) (*domain.User, error) {
					if id == "actor" {
						status := "active"
						if stage == "denied" {
							status = "inactive"
						}
						return &domain.User{ID: id, Role: GlobalRoleAdmin, Status: status}, nil
					}
					if stage == "denied" || stage == "invalid role" {
						t.Fatal("rejected request must not query target")
					}
					if stage == "target missing" {
						return nil, sentinel
					}
					return &domain.User{ID: id}, nil
				},
				member: func(context.Context, string, string) (*domain.WorkspaceMember, error) {
					if stage != "readback" || !wrote {
						t.Fatal("failed write must not read back")
					}
					return nil, sentinel
				},
			}
			store := &guardedMembershipWriteStoreStub{membershipWriteStoreStub: base, guarded: func(context.Context, *domain.WorkspaceMember, bool) error {
				if stage != "guarded write" && stage != "readback" {
					t.Fatal("invalid request reached write")
				}
				if stage == "guarded write" {
					return sentinel
				}
				wrote = true
				return nil
			}}
			role := WorkspaceRoleViewer
			if stage == "invalid role" {
				role = "root"
			}
			got, err := NewService(store, nil).UpsertWorkspaceMember(context.Background(), "actor", "space-a", "target", role)
			if got != nil || err == nil {
				t.Fatalf("failure reported as success: got=%v err=%v", got, err)
			}
			switch stage {
			case "denied":
				if !errors.Is(err, ErrForbidden) {
					t.Fatal(err)
				}
			case "invalid role":
				if !strings.Contains(err.Error(), "invalid workspace role") {
					t.Fatal(err)
				}
			default:
				if !errors.Is(err, sentinel) {
					t.Fatalf("cause lost: %v", err)
				}
			}
		})
	}
}

func TestServiceUpsertWorkspaceMemberFallback(t *testing.T) {
	for _, tc := range []struct {
		name, oldRole, newRole string
		owners                 bool
		ownerCount             int
		want                   error
	}{
		{"insert ordinary", "", WorkspaceRoleViewer, false, 0, nil},
		{"update ordinary", WorkspaceRoleViewer, WorkspaceRoleEditor, false, 0, nil},
		{"cannot grant owner", "", WorkspaceRoleOwner, false, 0, ErrForbidden},
		{"cannot change owner", WorkspaceRoleOwner, WorkspaceRoleViewer, false, 2, ErrForbidden},
		{"cannot touch unchanged owner", WorkspaceRoleOwner, WorkspaceRoleOwner, false, 2, ErrForbidden},
		{"last owner", WorkspaceRoleOwner, WorkspaceRoleViewer, true, 1, ErrLastOwner},
		{"unchanged owner", WorkspaceRoleOwner, WorkspaceRoleOwner, true, 1, nil},
		{"another owner remains", WorkspaceRoleOwner, WorkspaceRoleViewer, true, 2, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			written := false
			result := &domain.WorkspaceMember{WorkspaceID: "space-a", UserID: "target", Role: tc.newRole, Email: "target@example.com"}
			store := &membershipWriteStoreStub{
				user: func(_ context.Context, id string) (*domain.User, error) {
					role := GlobalRoleMember
					if id == "actor" && tc.owners {
						role = GlobalRoleAdmin
					}
					return &domain.User{ID: id, Role: role, Status: "active"}, nil
				},
				member: func(_ context.Context, space, id string) (*domain.WorkspaceMember, error) {
					if space != "space-a" {
						t.Fatal("wrong target workspace")
					}
					if id == "actor" {
						return &domain.WorkspaceMember{Role: WorkspaceRoleAdmin}, nil
					}
					if written {
						return result, nil
					}
					if tc.oldRole == "" {
						return nil, errors.New("member not found")
					}
					return &domain.WorkspaceMember{Role: tc.oldRole}, nil
				},
				members: func(context.Context, string) ([]*domain.WorkspaceMember, error) {
					members := []*domain.WorkspaceMember{{Role: WorkspaceRoleViewer}}
					for range tc.ownerCount {
						members = append(members, &domain.WorkspaceMember{Role: WorkspaceRoleOwner})
					}
					return members, nil
				},
				upsert: func(_ context.Context, member *domain.WorkspaceMember) error {
					if tc.want != nil {
						t.Fatal("rejected fallback reached basic write")
					}
					if member.WorkspaceID != "space-a" || member.UserID != "target" || member.Role != tc.newRole {
						t.Fatal("fallback write lost target")
					}
					written = true
					return nil
				},
			}
			got, err := NewService(store, nil).UpsertWorkspaceMember(context.Background(), "actor", "space-a", "target", tc.newRole)
			if tc.want != nil {
				if got != nil || !errors.Is(err, tc.want) {
					t.Fatalf("got=%v err=%v want %v", got, err, tc.want)
				}
			} else if err != nil || got != result || !written {
				t.Fatalf("fallback did not write and read back: got=%v err=%v", got, err)
			}
		})
	}
}

func TestServiceUpsertWorkspaceMemberPostgres(t *testing.T) {
	ctx, store, owner, viewer, newcomer := newGuardedMembershipStoreTest(t)
	service := NewService(store, nil)
	if got, err := service.UpsertWorkspaceMember(ctx, viewer, "space-a", newcomer, WorkspaceRoleViewer); got != nil || !errors.Is(err, ErrForbidden) {
		t.Fatalf("viewer must not manage members: got=%v err=%v", got, err)
	}
	added, err := service.UpsertWorkspaceMember(ctx, owner, "space-a", newcomer, WorkspaceRoleAdmin)
	if err != nil || added.Email != newcomer+"@example.com" || added.Role != WorkspaceRoleAdmin || added.CreatedAt.IsZero() {
		t.Fatalf("joined insert readback: got=%v err=%v", added, err)
	}
	updated, err := service.UpsertWorkspaceMember(ctx, newcomer, "space-a", viewer, WorkspaceRoleEditor)
	if err != nil || updated.Role != WorkspaceRoleEditor {
		t.Fatalf("space admin can manage ordinary roles: got=%v err=%v", updated, err)
	}
	for _, target := range []string{newcomer, owner} {
		if got, err := service.UpsertWorkspaceMember(ctx, newcomer, "space-a", target, WorkspaceRoleOwner); got != nil || !errors.Is(err, ErrForbidden) {
			t.Fatalf("space admin cannot operate owner: got=%v err=%v", got, err)
		}
	}
	if _, err := service.UpsertWorkspaceMember(ctx, owner, "space-a", owner, WorkspaceRoleViewer); !errors.Is(err, ErrLastOwner) {
		t.Fatalf("service lost last-owner protection: %v", err)
	}
	if _, err := service.UpsertWorkspaceMember(ctx, newcomer, "space-b", viewer, WorkspaceRoleAdmin); !errors.Is(err, ErrForbidden) {
		t.Fatalf("authority crossed workspace boundary: %v", err)
	}
	if err := store.SetUserRole(ctx, viewer, GlobalRoleAdmin); err != nil {
		t.Fatal(err)
	}
	if _, err := service.UpsertWorkspaceMember(ctx, viewer, "space-a", newcomer, WorkspaceRoleOwner); err != nil {
		t.Fatal("global admin must be able to grant owner", err)
	}
	if _, err := service.UpsertWorkspaceMember(ctx, owner, "space-a", owner, WorkspaceRoleViewer); err != nil {
		t.Fatal("another owner remains", err)
	}
	stored, err := store.GetWorkspaceMember(ctx, "space-a", newcomer)
	if err != nil || stored.Role != WorkspaceRoleOwner || !stored.CreatedAt.Equal(added.CreatedAt) {
		t.Fatal("owner promotion must preserve original joined time", err)
	}
	other, err := store.GetWorkspaceMember(ctx, "space-b", owner)
	if err != nil || other.Role != WorkspaceRoleOwner {
		t.Fatal("another workspace changed", err)
	}
	user, err := store.GetUserByID(ctx, newcomer)
	if err != nil || user.Role != GlobalRoleMember {
		t.Fatal("space promotion changed global role", err)
	}
}

// ---- service_workspace_member_delete_test.go ----

type membershipDeleteStoreStub struct {
	*membershipWriteStoreStub
	deleteMember func(context.Context, string, string) (bool, error)
}

func (s *membershipDeleteStoreStub) DeleteWorkspaceMember(ctx context.Context, space, id string) (bool, error) {
	return s.deleteMember(ctx, space, id)
}

type guardedMembershipDeleteStoreStub struct {
	*membershipDeleteStoreStub
	guardedDelete func(context.Context, string, string, bool) (bool, error)
}

func (s *guardedMembershipDeleteStoreStub) UpsertWorkspaceMemberGuarded(context.Context, *domain.WorkspaceMember, bool) error {
	return errors.New("unexpected upsert in deletion test")
}
func (s *guardedMembershipDeleteStoreStub) DeleteWorkspaceMemberGuarded(ctx context.Context, space, id string, owners bool) (bool, error) {
	return s.guardedDelete(ctx, space, id, owners)
}

var _ guardedMembershipStore = (*guardedMembershipDeleteStoreStub)(nil)

func TestServiceDeleteWorkspaceMemberGuardedDispatch(t *testing.T) {
	for _, tc := range []struct {
		name, globalRole, spaceRole string
		owners                      bool
	}{
		{"global admin", GlobalRoleAdmin, "", true},
		{"owner", GlobalRoleMember, WorkspaceRoleOwner, true},
		{"space admin", GlobalRoleMember, WorkspaceRoleAdmin, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, outcome := range []string{"deleted", "absent", "store error"} {
				t.Run(outcome, func(t *testing.T) {
					type requestKey struct{}
					ctx := context.WithValue(context.Background(), requestKey{}, "request")
					sentinel := errors.New("delete failed")
					called := 0
					base := &membershipWriteStoreStub{
						user: func(got context.Context, id string) (*domain.User, error) {
							if id != "actor" || got.Value(requestKey{}) != "request" {
								t.Fatal("actor/context lost")
							}
							return &domain.User{ID: id, Role: tc.globalRole, Status: "active"}, nil
						},
						member: func(got context.Context, space, id string) (*domain.WorkspaceMember, error) {
							if tc.globalRole == GlobalRoleAdmin || space != "space-a" || id != "actor" || got.Value(requestKey{}) != "request" {
								t.Fatal("unexpected membership query")
							}
							return &domain.WorkspaceMember{Role: tc.spaceRole}, nil
						},
						members: func(context.Context, string) ([]*domain.WorkspaceMember, error) {
							t.Fatal("guarded path must not use fallback reads")
							return nil, nil
						},
					}
					store := &guardedMembershipDeleteStoreStub{
						membershipDeleteStoreStub: &membershipDeleteStoreStub{membershipWriteStoreStub: base, deleteMember: func(context.Context, string, string) (bool, error) {
							t.Fatal("guarded path must not use basic deletion")
							return false, nil
						}},
						guardedDelete: func(got context.Context, space, id string, owners bool) (bool, error) {
							called++
							if got.Value(requestKey{}) != "request" || space != "space-a" || id != "target" || owners != tc.owners {
								t.Fatal("guarded deletion lost target/context/authority")
							}
							if outcome == "store error" {
								return false, sentinel
							}
							return outcome == "deleted", nil
						},
					}
					err := NewService(store, nil).DeleteWorkspaceMember(ctx, "actor", "space-a", "target")
					if called != 1 {
						t.Fatalf("guarded calls=%d, want 1", called)
					}
					switch outcome {
					case "deleted":
						if err != nil {
							t.Fatal(err)
						}
					case "absent":
						if err == nil || !strings.Contains(err.Error(), "workspace member not found") {
							t.Fatalf("missing membership must be a business error: %v", err)
						}
					case "store error":
						if !errors.Is(err, sentinel) {
							t.Fatalf("delete error lost: %v", err)
						}
					}
				})
			}
		})
	}
}

func TestServiceDeleteWorkspaceMemberAuthority(t *testing.T) {
	for _, ids := range [][2]string{{"", "space-a"}, {"actor", ""}} {
		err := NewService(&membershipDeleteStoreStub{}, nil).DeleteWorkspaceMember(context.Background(), ids[0], ids[1], "target")
		if !errors.Is(err, ErrForbidden) {
			t.Fatalf("empty actor/workspace must be denied: %v", err)
		}
	}
	for _, role := range []string{WorkspaceRoleEditor, WorkspaceRoleMember, WorkspaceRoleViewer, "inactive", "no membership"} {
		t.Run(role, func(t *testing.T) {
			base := &membershipWriteStoreStub{
				user: func(context.Context, string) (*domain.User, error) {
					status := "active"
					if role == "inactive" {
						status = "inactive"
					}
					return &domain.User{Role: GlobalRoleMember, Status: status}, nil
				},
				member: func(context.Context, string, string) (*domain.WorkspaceMember, error) {
					if role == "no membership" {
						return nil, errors.New("not found")
					}
					return &domain.WorkspaceMember{Role: role}, nil
				},
			}
			store := &guardedMembershipDeleteStoreStub{membershipDeleteStoreStub: &membershipDeleteStoreStub{membershipWriteStoreStub: base}, guardedDelete: func(context.Context, string, string, bool) (bool, error) {
				t.Fatal("unauthorized request reached delete")
				return false, nil
			}}
			if err := NewService(store, nil).DeleteWorkspaceMember(context.Background(), "actor", "space-a", "target"); !errors.Is(err, ErrForbidden) {
				t.Fatalf("role %s accepted: %v", role, err)
			}
		})
	}
}

func TestServiceDeleteWorkspaceMemberFallback(t *testing.T) {
	for _, tc := range []struct {
		name                      string
		manageOwners, targetOwner bool
		ownerCount                int
		stage                     string
		want                      error
	}{
		{name: "ordinary member", ownerCount: 1},
		{name: "space admin cannot remove owner", targetOwner: true, ownerCount: 2, want: ErrForbidden},
		{name: "last owner", manageOwners: true, targetOwner: true, ownerCount: 1, want: ErrLastOwner},
		{name: "another owner remains", manageOwners: true, targetOwner: true, ownerCount: 2},
		{name: "list failure", stage: "list"},
		{name: "delete failure", stage: "delete"},
		{name: "absent", stage: "absent"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sentinel := errors.New("fallback failed")
			deleted := false
			base := &membershipWriteStoreStub{
				user: func(context.Context, string) (*domain.User, error) {
					role := GlobalRoleMember
					if tc.manageOwners {
						role = GlobalRoleAdmin
					}
					return &domain.User{Role: role, Status: "active"}, nil
				},
				member: func(context.Context, string, string) (*domain.WorkspaceMember, error) {
					return &domain.WorkspaceMember{Role: WorkspaceRoleAdmin}, nil
				},
				members: func(_ context.Context, space string) ([]*domain.WorkspaceMember, error) {
					if space != "space-a" {
						t.Fatal("wrong workspace")
					}
					if tc.stage == "list" {
						return nil, sentinel
					}
					members := []*domain.WorkspaceMember{{UserID: "target", Role: WorkspaceRoleViewer}}
					if tc.targetOwner {
						members[0].Role = WorkspaceRoleOwner
					}
					owners := tc.ownerCount
					if tc.targetOwner {
						owners--
					}
					for range owners {
						members = append(members, &domain.WorkspaceMember{UserID: "another-owner", Role: WorkspaceRoleOwner})
					}
					return members, nil
				},
			}
			store := &membershipDeleteStoreStub{membershipWriteStoreStub: base, deleteMember: func(_ context.Context, space, id string) (bool, error) {
				if tc.want != nil || tc.stage == "list" {
					t.Fatal("rejected operation reached delete")
				}
				if space != "space-a" || id != "target" {
					t.Fatal("deletion lost target")
				}
				if tc.stage == "delete" {
					return false, sentinel
				}
				if tc.stage == "absent" {
					return false, nil
				}
				deleted = true
				return true, nil
			}}
			err := NewService(store, nil).DeleteWorkspaceMember(context.Background(), "actor", "space-a", "target")
			switch {
			case tc.want != nil:
				if !errors.Is(err, tc.want) {
					t.Fatalf("err=%v, want %v", err, tc.want)
				}
			case tc.stage == "list" || tc.stage == "delete":
				if !errors.Is(err, sentinel) {
					t.Fatalf("cause lost: %v", err)
				}
			case tc.stage == "absent":
				if err == nil || !strings.Contains(err.Error(), "workspace member not found") {
					t.Fatalf("expected missing-member error, got %v", err)
				}
			default:
				if err != nil || !deleted {
					t.Fatalf("valid deletion rejected: %v", err)
				}
			}
		})
	}
}

func TestServiceDeleteWorkspaceMemberPostgres(t *testing.T) {
	ctx, store, owner, member, globalAdmin := newGuardedMembershipStoreTest(t)
	service := NewService(store, nil)
	if err := store.SetUserRole(ctx, globalAdmin, GlobalRoleAdmin); err != nil {
		t.Fatal(err)
	}
	if err := service.DeleteWorkspaceMember(ctx, member, "space-a", owner); !errors.Is(err, ErrForbidden) {
		t.Fatalf("viewer must not manage members: %v", err)
	}
	if _, err := service.UpsertWorkspaceMember(ctx, owner, "space-a", member, WorkspaceRoleAdmin); err != nil {
		t.Fatal(err)
	}
	if err := service.DeleteWorkspaceMember(ctx, member, "space-a", owner); !errors.Is(err, ErrForbidden) {
		t.Fatalf("space admin must not remove owner: %v", err)
	}
	if _, err := service.UpsertWorkspaceMember(ctx, owner, "space-a", globalAdmin, WorkspaceRoleViewer); err != nil {
		t.Fatal(err)
	}
	if err := service.DeleteWorkspaceMember(ctx, member, "space-a", globalAdmin); err != nil {
		t.Fatal("space admin may remove an ordinary membership even for a global admin", err)
	}
	if _, err := store.GetWorkspaceMember(ctx, "space-a", globalAdmin); err == nil {
		t.Fatal("membership still present after removal")
	}
	account, err := store.GetUserByID(ctx, globalAdmin)
	if err != nil || account.Role != GlobalRoleAdmin {
		t.Fatal("membership removal must preserve global role", err)
	}
	if err := service.DeleteWorkspaceMember(ctx, globalAdmin, "space-a", owner); !errors.Is(err, ErrLastOwner) {
		t.Fatalf("global admin must also preserve last owner: %v", err)
	}
	if _, err := service.UpsertWorkspaceMember(ctx, owner, "space-a", member, WorkspaceRoleOwner); err != nil {
		t.Fatal(err)
	}
	if err := service.DeleteWorkspaceMember(ctx, globalAdmin, "space-a", owner); err != nil {
		t.Fatal("another owner remains", err)
	}
	if err := service.DeleteWorkspaceMember(ctx, globalAdmin, "space-a", owner); err == nil || !strings.Contains(err.Error(), "workspace member not found") {
		t.Fatalf("repeat deletion must report missing membership: %v", err)
	}
	if _, err := store.GetUserByID(ctx, owner); err != nil {
		t.Fatal("removed member account disappeared", err)
	}
	other, err := store.GetWorkspaceMember(ctx, "space-b", owner)
	if err != nil || other.Role != WorkspaceRoleOwner {
		t.Fatal("other workspace owner changed", err)
	}
	if err := service.DeleteWorkspaceMember(ctx, member, "space-b", owner); !errors.Is(err, ErrForbidden) {
		t.Fatalf("management authority crossed workspace boundary: %v", err)
	}
}
