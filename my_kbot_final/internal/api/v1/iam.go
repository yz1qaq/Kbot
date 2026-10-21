package v1

import (
	"math"
	"my_kbot_final/internal/platform/iam"
	"net/http"
)

type IAMHandler struct {
	iamService *iam.Service
}

func NewIAMHandler(iamService *iam.Service) *IAMHandler {
	return &IAMHandler{iamService: iamService}
}

func (h *IAMHandler) ListUsers(w http.ResponseWriter, r *http.Request) {
	limit, offset, err := pageParams(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	users, err := h.iamService.ListUsers(r.Context(), limit, offset)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, users)
}

// 解析并校验列表分页参数
func pageParams(r *http.Request) (int32, int32, error) {
	limit, err := queryInt(r, "limit", 50, 1, 200)
	if err != nil {
		return 0, 0, err
	}
	offset, err := queryInt(r, "offset", 0, 0, math.MaxInt32)
	if err != nil {
		return 0, 0, err
	}
	return int32(limit), int32(offset), nil
}
