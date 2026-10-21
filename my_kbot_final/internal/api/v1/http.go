package v1

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
)

const maxJSONBodyBytes = 1 << 20

func decodeJSON(w http.ResponseWriter, r *http.Request, dst any) error {
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxJSONBodyBytes))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(dst); err != nil {
		return err
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return fmt.Errorf("request body must contain one JSON value")
		}
		return err
	}
	return nil
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

// 读取整数查询参数,并检查允许范围  eg:GET /api/v1/users?limit=20&offset=40
func queryInt(r *http.Request, name string, dafaultValue, minValue, MaxValue int) (int, error) {
	raw := r.URL.Query().Get(name)
	if raw == "" {
		return dafaultValue, nil
	}
	//把字符串转换为整数,比如limit=20, 会得到raw="20"
	value, err := strconv.Atoi(raw)
	if err != nil || value < minValue || value > MaxValue {
		return 0, fmt.Errorf("%s must be between %d and %d", name, minValue, MaxValue)
	}
	return value, nil
}
