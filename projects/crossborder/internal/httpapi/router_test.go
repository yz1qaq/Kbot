package httpapi

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"my_kbot-crossborder/internal/domain"
	"my_kbot-crossborder/internal/service"
)

func TestCreateTransferEndpoint(t *testing.T) {
	server := httptest.NewServer(New(service.NewSeeded()))
	t.Cleanup(server.Close)

	body := map[string]any{
		"sku":             "SKU-BLACK-M-01",
		"from_warehouse":  "WH-US-LAX",
		"to_warehouse":    "WH-CN-SZ",
		"quantity":        2,
		"idempotency_key": "http-transfer-test-001",
	}

	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("encode request: %v", err)
	}

	response, err := http.Post(
		server.URL+"/api/transfers",
		"application/json",
		bytes.NewReader(raw),
	)
	if err != nil {
		t.Fatalf("send request: %v", err)
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusCreated {
		t.Fatalf("status = %d, want %d",
			response.StatusCode, http.StatusCreated)
	}

	var transfer domain.InventoryTransfer
	if err := json.NewDecoder(response.Body).Decode(&transfer); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if transfer.ID == "" || transfer.Status != "created" {
		t.Fatalf("unexpected transfer: %+v", transfer)
	}
}

func TestCreateTransferRejectsUnknownField(t *testing.T) {
	body := `{
		"sku": "SKU-BLACK-M-01",
		"from_warehouse": "WH-US-LAX",
		"to_warehouse": "WH-CN-SZ",
		"quantity": 2,
		"idempotency_key": "http-unknown-field-001",
		"unexpected": true
	}`

	request := httptest.NewRequest(
		http.MethodPost,
		"/api/transfers",
		bytes.NewBufferString(body),
	)
	request.Header.Set("Content-Type", "application/json")

	response := httptest.NewRecorder()
	handler := New(service.NewSeeded())
	handler.ServeHTTP(response, request)

	if response.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d",
			response.Code, http.StatusBadRequest)
	}

	var result map[string]string
	if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if result["error"] != "invalid_json" {
		t.Fatalf("error = %q, want invalid_json", result["error"])
	}
}