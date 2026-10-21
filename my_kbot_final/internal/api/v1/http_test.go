package v1

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDecodeJSON(t *testing.T) {
	tests := []struct {
		name, body string
		valid      bool
	}{
		{"valid", `{"email":"alice@example.com","password":"password123"}`, true},
		{"trailing whitespace", "{\"email\":\"alice@example.com\",\"password\":\"password123\"} \n\t", true},
		{"empty", "", false},
		{"malformed", `{"email":`, false},
		{"unknown field", `{"email":"alice@example.com","role":"admin"}`, false},
		{"wrong field type", `{"email":123}`, false},
		{"second JSON object", `{"email":"alice@example.com"} {"email":"bob@example.com"}`, false},
		{"second null value", `{"email":"alice@example.com"} null`, false},
		{"trailing junk", `{"email":"alice@example.com"} trailing`, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodPost, "/test", strings.NewReader(tc.body))
			response := httptest.NewRecorder()
			var decoded struct {
				Email    string `json:"email"`
				Password string `json:"password"`
			}
			err := decodeJSON(response, request, &decoded)
			if (err == nil) != tc.valid {
				t.Fatalf("unexpected decoding outcome: valid=%t err=%v", tc.valid, err)
			}
			if tc.valid && (decoded.Email != "alice@example.com" || decoded.Password != "password123") {
				t.Fatal("valid JSON fields were not decoded")
			}
		})
	}
}

func TestDecodeJSONLimitsBody(t *testing.T) {
	for _, body := range []string{
		`{"email":"` + strings.Repeat("a", 1<<20) + `"}`,
		`{"email":"alice@example.com"}` + strings.Repeat(" ", 1<<20),
	} {
		request := httptest.NewRequest(http.MethodPost, "/test", strings.NewReader(body))
		var decoded struct {
			Email string `json:"email"`
		}
		err := decodeJSON(httptest.NewRecorder(), request, &decoded)
		var tooLarge *http.MaxBytesError
		if !errors.As(err, &tooLarge) {
			t.Fatalf("oversized request body must fail with a size-limit error: %v", err)
		}
	}
}

func TestWriteJSON(t *testing.T) {
	response := httptest.NewRecorder()
	writeJSON(response, http.StatusCreated, map[string]any{
		"name": "小明", "active": true,
	})
	if response.Code != http.StatusCreated {
		t.Fatalf("status=%d, want 201", response.Code)
	}
	if response.Header().Get("Content-Type") != "application/json" {
		t.Fatal("response must advertise application/json")
	}
	var decoded struct {
		Name   string `json:"name"`
		Active bool   `json:"active"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &decoded); err != nil {
		t.Fatalf("response must be valid JSON: %v", err)
	}
	if decoded.Name != "小明" || !decoded.Active {
		t.Fatal("response JSON changed the payload")
	}
}
