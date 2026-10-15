package tool

import (
	"context"
	"testing"
)

func TestRegistryCredentialEncryption(t *testing.T) {
	registry := NewRegistry([]byte("test-key"))
	want := "demo-api-key"

	ciphertext, err := registry.encrypt(want)
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}

	got, err := registry.decrypt(ciphertext)
	if err != nil {
		t.Fatalf("decrypt: %v", err)
	}
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}

	ciphertext[len(ciphertext)-1] ^= 1

	if _, err := registry.decrypt(ciphertext); err == nil {
		t.Fatal("expected an error after ciphertext was tampered with")
	}
}

func TestRegistryToolNameOwnership(t *testing.T) {
	registry := NewRegistry([]byte("test-key"))
	ctx := context.Background()

	first := Version{
		ID:          "search-v1",
		ToolID:      "search-tool",
		WorkspaceID: "workspace-1",
		Name:        "Search",
		InputSchema: []byte(`{"type":"object"}`),
	}
	if err := registry.Register(ctx, first); err != nil {
		t.Fatalf("register first version: %v", err)
	}

	second := first
	second.ID = "search-v2"
	second.Name = " search "
	if err := registry.Register(ctx, second); err != nil {
		t.Fatalf("register second version of the same tool: %v", err)
	}

	other := first
	other.ID = "other-v1"
	other.ToolID = "other-tool"
	if err := registry.Register(ctx, other); err == nil {
		t.Fatal("expected an error when another tool uses the same name")
	}
}

func TestRegistryResolve(t *testing.T) {
	registry := NewRegistry([]byte("test-key"))
	ctx := context.Background()

	published := Version{
		ID:          "inventory-v1",
		ToolID:      "inventory-tool",
		WorkspaceID: "workspace-1",
		Name:        "search_inventory",
		InputSchema: []byte(`{"type":"object"}`),
		AuthConfig:  "demo-api-key",
		Published:   true,
	}
	if err := registry.Register(ctx, published); err != nil {
		t.Fatalf("register published version: %v", err)
	}

	got, err := registry.Resolve(ctx, "workspace-1", "inventory-v1")
	if err != nil {
		t.Fatalf("resolve published version: %v", err)
	}
	if got.ID != published.ID {
		t.Fatalf("got ID %q, want %q", got.ID, published.ID)
	}
	if got.AuthConfig != published.AuthConfig {
		t.Fatal("resolved credentials do not match the original")
	}

	if _, err := registry.Resolve(ctx, "workspace-2", "inventory-v1"); err == nil {
		t.Fatal("expected an error when resolving from another workspace")
	}

	draft := published
	draft.ID = "inventory-v2"
	draft.Published = false
	if err := registry.Register(ctx, draft); err != nil {
		t.Fatalf("register draft version: %v", err)
	}

	if _, err := registry.Resolve(ctx, "workspace-1", "inventory-v2"); err == nil {
		t.Fatal("expected an error when resolving a draft version")
	}
}

func TestRegistryList(t *testing.T) {
	registry := NewRegistry([]byte("test-key"))
	ctx := context.Background()

	draft := Version{
		ID:          "inventory-v2",
		ToolID:      "inventory-tool",
		WorkspaceID: "workspace-1",
		Name:        "search_inventory",
		InputSchema: []byte(`{"type":"object"}`),
		AuthConfig:  "demo-api-key",
		Published:   false,
	}

	published := draft
	published.ID = "inventory-v1"
	published.Published = true

	other := draft
	other.ID = "other-v1"
	other.ToolID = "other-tool"
	other.WorkspaceID = "workspace-2"

	for _, version := range []Version{draft, published, other} {
		if err := registry.Register(ctx, version); err != nil {
			t.Fatalf("register %s: %v", version.ID, err)
		}
	}

	got := registry.List(ctx, "workspace-1")
	if len(got) != 2 {
		t.Fatalf("got %d versions, want 2", len(got))
	}

	if got[0].ID != "inventory-v1" || got[1].ID != "inventory-v2" {
		t.Fatalf("unexpected order: %s, %s", got[0].ID, got[1].ID)
	}

	for _, version := range got {
		if version.WorkspaceID != "workspace-1" {
			t.Fatal("list contains a version from another workspace")
		}
		if version.AuthConfig != "" || len(version.authCiphertext) != 0 {
			t.Fatal("list exposes credentials")
		}
		if !version.HasAuth {
			t.Fatal("list should retain the credential indicator")
		}
	}

	resolved, err := registry.Resolve(ctx, "workspace-1", "inventory-v1")
	if err != nil {
		t.Fatalf("resolve after listing: %v", err)
	}
	if resolved.AuthConfig != "demo-api-key" {
		t.Fatal("listing changed the stored credentials")
	}
}

func TestRegistryPinsImmutablePublishedVersion(t *testing.T) {
	registry := NewRegistry([]byte("test-key"))
	ctx := context.Background()
	const wantSchema = `{"type":"object","required":["sku"]}`

	version := Version{
		ID:          "inventory-v1",
		ToolID:      "inventory-tool",
		WorkspaceID: "workspace-1",
		Name:        "search_inventory",
		InputSchema: []byte(wantSchema),
		Published:   true,
	}
	if err := registry.Register(ctx, version); err != nil {
		t.Fatalf("register: %v", err)
	}

	version.InputSchema[0] = 'x'

	resolved, err := registry.Resolve(ctx, "workspace-1", "inventory-v1")
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if string(resolved.InputSchema) != wantSchema {
		t.Fatalf("stored schema was changed: %s", resolved.InputSchema)
	}

	duplicate := resolved
	duplicate.Description = "attempt to change the existing version"
	if err := registry.Register(ctx, duplicate); err == nil {
		t.Fatal("expected registration with an existing version ID to fail")
	}
}
