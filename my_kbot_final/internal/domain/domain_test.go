package domain

import (
	"bytes"
	"encoding/json"
	"testing"
)

func TestUserJSONOmitsPasswordHash(t *testing.T) {
	const secret = "private-password-hash-marker"
	user := User{ID: "user-1", Email: "learner@example.com", PasswordHash: secret}

	data, err := json.Marshal(user)
	if err != nil {
		t.Fatalf("marshal user: %v", err)
	}
	if bytes.Contains(data, []byte(secret)) {
		t.Fatal("user JSON exposes the password hash")
	}

	var payload map[string]json.RawMessage
	if err := json.Unmarshal(data, &payload); err != nil {
		t.Fatalf("decode user JSON: %v", err)
	}
	for _, key := range []string{"password_hash", "PasswordHash"} {
		if _, ok := payload[key]; ok {
			t.Fatalf("user JSON contains private field %q", key)
		}
	}
	if _, ok := payload["email"]; !ok {
		t.Fatal("user JSON is missing its public email field")
	}
}
