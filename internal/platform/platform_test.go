package platform

import (
	"context"
	"testing"

	"my_kbot/internal/domain"
	"my_kbot/internal/runtime/engine"
)

func TestConversationCopyIsolation(t *testing.T) {
	controlPlane := New()
	ctx := context.Background()

	original := &domain.Conversation{
		ID:             "conversation-001",
		AgentVersionID: "version-001",
	}
	controlPlane.PutConversation(original)

	// 修改传入对象，不应影响保存的数据。
	original.AgentVersionID = "version-002"

	loaded, err := controlPlane.LoadConversation(ctx, original.ID)
	if err != nil {
		t.Fatalf("load conversation: %v", err)
	}
	if loaded.AgentVersionID != "version-001" {
		t.Fatalf(
			"stored version = %q, want version-001",
			loaded.AgentVersionID,
		)
	}

	// 修改查询结果，也不应影响保存的数据。
	loaded.AgentVersionID = "version-003"

	reloaded, err := controlPlane.LoadConversation(ctx, original.ID)
	if err != nil {
		t.Fatalf("reload conversation: %v", err)
	}
	if reloaded.AgentVersionID != "version-001" {
		t.Fatalf(
			"stored version after modifying query result = %q, want version-001",
			reloaded.AgentVersionID,
		)
	}
}

func TestEngineResolvesPinnedSnapshot(t *testing.T) {
	controlPlane := New()

	controlPlane.PutSnapshot(&engine.AgentSnapshot{
		AgentVersionID: "version-001",
		AgentID:        "agent-001",
		SystemPrompt:   "第一版提示词",
	})
	controlPlane.PutConversation(&domain.Conversation{
		ID:             "conversation-001",
		AgentID:        "agent-001",
		AgentVersionID: "version-001",
	})

	// 同一个 Agent 后来增加了第二版配置。
	controlPlane.PutSnapshot(&engine.AgentSnapshot{
		AgentVersionID: "version-002",
		AgentID:        "agent-001",
		SystemPrompt:   "第二版提示词",
	})

	runtime := engine.New(controlPlane, nil)

	snapshot, err := runtime.ResolveSnapshot(
		context.Background(),
		"conversation-001",
	)
	if err != nil {
		t.Fatalf("resolve snapshot: %v", err)
	}
	if snapshot.AgentVersionID != "version-001" ||
		snapshot.SystemPrompt != "第一版提示词" {
		t.Fatalf("unexpected snapshot: %+v", snapshot)
	}
}

func TestSnapshotToolVersionsCopyIsolation(t *testing.T) {
	controlPlane := New()
	ctx := context.Background()

	original := &engine.AgentSnapshot{
		AgentVersionID: "agent-v1",
		ToolVersionIDs: []string{"inventory-v1"},
	}
	controlPlane.PutSnapshot(original)

	// 修改原对象，不能影响已保存的工具版本列表。
	original.ToolVersionIDs[0] = "inventory-v2"

	loaded, err := controlPlane.GetAgentSnapshotByVersion(ctx, "agent-v1")
	if err != nil {
		t.Fatalf("load snapshot: %v", err)
	}

	if len(loaded.ToolVersionIDs) != 1 ||
		loaded.ToolVersionIDs[0] != "inventory-v1" {
		t.Fatalf(
			"stored tool versions = %v, want [inventory-v1]",
			loaded.ToolVersionIDs,
		)
	}

	// 修改查询结果，也不能影响平台内部的数据。
	loaded.ToolVersionIDs[0] = "python-v1"

	reloaded, err := controlPlane.GetAgentSnapshotByVersion(ctx, "agent-v1")
	if err != nil {
		t.Fatalf("reload snapshot: %v", err)
	}

	if len(reloaded.ToolVersionIDs) != 1 ||
		reloaded.ToolVersionIDs[0] != "inventory-v1" {
		t.Fatalf(
			"stored tool versions after modifying query result = %v, want [inventory-v1]",
			reloaded.ToolVersionIDs,
		)
	}
}
