package v1

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func TestPageParams(t *testing.T) {
	tests := []struct {
		name, query   string
		limit, offset int32
		invalidField  string
	}{
		{"defaults", "", 50, 0, ""},
		{"empty values use defaults", "?limit=&offset=", 50, 0, ""},
		{"explicit page", "?limit=20&offset=40", 20, 40, ""},
		{"minimum limit", "?limit=1", 1, 0, ""},
		{"maximum limit", "?limit=200", 200, 0, ""},
		{"maximum int32 offset", "?offset=2147483647", 50, 2147483647, ""},
		{"zero limit", "?limit=0", 0, 0, "limit"},
		{"negative limit", "?limit=-1", 0, 0, "limit"},
		{"excessive limit", "?limit=201", 0, 0, "limit"},
		{"noninteger limit", "?limit=abc", 0, 0, "limit"},
		{"fractional limit", "?limit=1.5", 0, 0, "limit"},
		{"negative offset", "?offset=-1", 0, 0, "offset"},
		{"overflow on int32 conversion", "?offset=2147483648", 0, 0, "offset"},
		{"overflow on integer parsing", "?offset=999999999999999999999999999", 0, 0, "offset"},
		{"noninteger offset", "?offset=abc", 0, 0, "offset"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			request := httptest.NewRequest("GET", "/api/v1/users"+tc.query, nil)
			limit, offset, err := pageParams(request)
			if tc.invalidField != "" {
				if err == nil || !strings.Contains(err.Error(), tc.invalidField) {
					t.Fatalf("invalid pagination must identify %s, err=%v", tc.invalidField, err)
				}
				return
			}
			if err != nil || limit != tc.limit || offset != tc.offset {
				t.Fatalf("page parameters=(%d,%d),err=%v; want (%d,%d)", limit, offset, err, tc.limit, tc.offset)
			}
		})
	}
}
