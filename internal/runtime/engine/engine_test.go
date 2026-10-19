package engine

import (
	"context"
	"fmt"
	"my_kbot/internal/domain"
	"strings"
	"testing"
)

type fakePlatform struct {
	conversation     *domain.Conversation
	snapshots        map[string]*AgentSnapshot
	requestedVersion string
}

func (p *fakePlatform) LoadConversation(context.Context, string) (*domain.Conversation, error) {
	return p.conversation, nil
}

func (p *fakePlatform) GetAgentSnapshotByVersion(_ context.Context, agentVersionID string) (*AgentSnapshot, error) {
	p.requestedVersion = agentVersionID
	snapshot, ok := p.snapshots[agentVersionID]
	if !ok {
		return nil, fmt.Errorf("snapshot %s not found", agentVersionID)
	}
	return snapshot, nil
}

var _ Platform = (*fakePlatform)(nil)

func TestResolveSnapshotUsesConversationPinnedVersion(t *testing.T) {
	controlPlane := &fakePlatform{
		conversation: &domain.Conversation{
			ID:             "conversation-001",
			AgentID:        "agent-001",
			AgentVersionID: "version-001",
		},
		snapshots: map[string]*AgentSnapshot{
			"version-001": {
				AgentVersionID: "version-001",
				AgentID:        "agent-001",
				SystemPrompt:   "这是第一版提示词:xxxx",
			},
			"version-002": {
				AgentVersionID: "version-002",
				AgentID:        "agent-001",
				SystemPrompt:   "这是第二版提示词:xxxx",
			},
		},
	}
	runtime := New(controlPlane, nil)
	snapshot, err := runtime.ResolveSnapshot(context.Background(), "conversation-001")
	if err != nil {
		t.Fatalf("resolve snapshot: %v", err)
	}
	if snapshot.AgentVersionID != "version-001" {
		t.Fatalf("snapshot ID = %q, want version-001", snapshot.AgentVersionID)
	}
	if controlPlane.requestedVersion != "version-001" {
		t.Fatalf(
			"requested version = %q, want version-001",
			controlPlane.requestedVersion,
		)
	}

}

func TestResolveSnapshotChecksWorkspace(t *testing.T) {
	tests := []struct {
		name              string
		conversationSpace string
		snapshotSpace     string
		wantErr           bool
	}{
		{"same_workspace", "workspace-1", "workspace-1", false},
		{"different_workspaces", "workspace-1", "workspace-2", true},
		{"empty_conversation_workspace", "", "workspace-1", false},
		{"empty_snapshot_workspace", "workspace-1", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			controlPlane := &fakePlatform{
				conversation: &domain.Conversation{
					ID:             "conversation-1",
					AgentVersionID: "agent-v1",
					WorkspaceID:    tt.conversationSpace,
				},
				snapshots: map[string]*AgentSnapshot{
					"agent-v1": {
						AgentVersionID: "agent-v1",
						WorkspaceID:    tt.snapshotSpace,
					},
				},
			}

			runtime := New(controlPlane, nil)
			snapshot, err := runtime.ResolveSnapshot(
				context.Background(),
				"conversation-1",
			)

			if tt.wantErr {
				if err == nil {
					t.Fatal("expected workspace mismatch error")
				}
				if !strings.Contains(err.Error(), "different workspaces") {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}

			if err != nil {
				t.Fatalf("ResolveSnapshot(): %v", err)
			}

			if snapshot.WorkspaceID != tt.snapshotSpace {
				t.Fatalf(
					"snapshot workspace = %q, want %q",
					snapshot.WorkspaceID,
					tt.snapshotSpace,
				)
			}
		})
	}
}
