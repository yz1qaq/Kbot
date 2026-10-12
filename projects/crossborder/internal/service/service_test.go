package service

import (
	"errors"
	"testing"
)

func TestCreateTransferIsIdempotent(t *testing.T) {
	svc := NewSeeded()
	req := TransferRequest{
		SKU:            "SKU-BLACK-M-01",
		FromWarehouse:  "WH-US-LAX",
		ToWarehouse:    "WH-CN-SZ",
		Quantity:       2,
		IdempotencyKey: "lesson-02",
	}

	first, err := svc.CreateTransfer(req)
	if err != nil {
		t.Fatalf("first transfer: %v", err)
	}

	second, err := svc.CreateTransfer(req)
	if err != nil {
		t.Fatalf("second transfer: %v", err)
	}

	if first.ID == "" || first.ID != second.ID {
		t.Fatalf("invalid transfer IDs: first=%q, second=%q",
			first.ID, second.ID)
	}

	if len(svc.transfers) != 1 {
		t.Fatalf("transfer count = %d, want 1", len(svc.transfers))
	}

	if len(svc.idempotency) != 1 {
		t.Fatalf("idempotency count = %d, want 1", len(svc.idempotency))
	}

	actual := make(map[string]int)
	for _, balance := range svc.Inventory(req.SKU) {
		actual[balance.WarehouseID] = balance.Available
	}

	expected := map[string]int{
		"WH-US-LAX": 16,
		"WH-CN-SZ":  2,
	}
	for warehouse, want := range expected {
		got, ok := actual[warehouse]
		if !ok || got != want {
			t.Fatalf("%s: present=%t, available=%d, want %d",
				warehouse, ok, got, want)
		}
	}
}

func TestCreateTransferDryRunDoesNotChangeInventory(t *testing.T) {
	svc := NewSeeded()
	req := TransferRequest{
		SKU:            "SKU-BLACK-M-01",
		FromWarehouse:  "WH-US-LAX",
		ToWarehouse:    "WH-CN-SZ",
		Quantity:       3,
		IdempotencyKey: "lesson-02-dry-run",
		DryRun:         true,
	}

	result, err := svc.CreateTransfer(req)
	if err != nil {
		t.Fatalf("dry run: %v", err)
	}

	if result.Status != "validated" {
		t.Fatalf("status = %q, want validated", result.Status)
	}

	if len(svc.transfers) != 0 {
		t.Fatalf("transfer count = %d, want 0", len(svc.transfers))
	}

	if len(svc.idempotency) != 1 {
		t.Fatalf("idempotency count = %d, want 1", len(svc.idempotency))
	}

	actual := make(map[string]int)
	for _, balance := range svc.Inventory(req.SKU) {
		actual[balance.WarehouseID] = balance.Available
	}

	expected := map[string]int{
		"WH-US-LAX": 18,
		"WH-CN-SZ":  0,
	}
	for warehouse, want := range expected {
		got, ok := actual[warehouse]
		if !ok || got != want {
			t.Fatalf("%s: present=%t, available=%d, want %d",
				warehouse, ok, got, want)
		}
	}
}

func TestCreateTransferRejectsInsufficientInventory(t *testing.T) {
	svc := NewSeeded()
	req := TransferRequest{
		SKU:            "SKU-BLACK-M-01",
		FromWarehouse:  "WH-US-LAX",
		ToWarehouse:    "WH-CN-SZ",
		Quantity:       100,
		IdempotencyKey: "lesson-02-insufficient",
	}

	_, err := svc.CreateTransfer(req)
	if !errors.Is(err, ErrInsufficientStock) {
		t.Fatalf("error = %v, want ErrInsufficientStock", err)
	}

	if len(svc.transfers) != 0 {
		t.Fatalf("transfer count = %d, want 0", len(svc.transfers))
	}

	if len(svc.idempotency) != 0 {
		t.Fatalf("idempotency count = %d, want 0", len(svc.idempotency))
	}

	actual := make(map[string]int)
	for _, balance := range svc.Inventory(req.SKU) {
		actual[balance.WarehouseID] = balance.Available
	}

	expected := map[string]int{
		"WH-US-LAX": 18,
		"WH-CN-SZ":  0,
	}
	for warehouse, want := range expected {
		got, ok := actual[warehouse]
		if !ok || got != want {
			t.Fatalf("%s: present=%t, available=%d, want %d",
				warehouse, ok, got, want)
		}
	}
}

func TestCreateTransferBindsIdempotencyKeyToRequest(t *testing.T) {
	svc := NewSeeded()
	first := TransferRequest{
		SKU:            "SKU-BLACK-M-01",
		FromWarehouse:  "WH-US-LAX",
		ToWarehouse:    "WH-CN-SZ",
		Quantity:       2,
		IdempotencyKey: "lesson-02-conflict",
	}

	if _, err := svc.CreateTransfer(first); err != nil {
		t.Fatalf("first transfer: %v", err)
	}

	changed := first
	changed.Quantity = 3

	_, err := svc.CreateTransfer(changed)
	if !errors.Is(err, ErrIdempotencyConflict) {
		t.Fatalf("changed request error = %v, want ErrIdempotencyConflict", err)
	}

	if len(svc.transfers) != 1 {
		t.Fatalf("transfer count = %d, want 1", len(svc.transfers))
	}

	if len(svc.idempotency) != 1 {
		t.Fatalf("idempotency count = %d, want 1", len(svc.idempotency))
	}

	actual := make(map[string]int)
	for _, balance := range svc.Inventory(first.SKU) {
		actual[balance.WarehouseID] = balance.Available
	}

	expected := map[string]int{
		"WH-US-LAX": 16,
		"WH-CN-SZ":  2,
	}
	for warehouse, want := range expected {
		got, ok := actual[warehouse]
		if !ok || got != want {
			t.Fatalf("%s: present=%t, available=%d, want %d",
				warehouse, ok, got, want)
		}
	}
}
