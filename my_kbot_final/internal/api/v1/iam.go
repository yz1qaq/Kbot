package v1

import (
	"errors"
	"math"
	"my_kbot_final/internal/api/middleware"
	"my_kbot_final/internal/platform/iam"
	"net/http"

	"github.com/go-chi/chi/v5"
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

// 列出所有工作空间
func (h *IAMHandler) ListWorkspaces(w http.ResponseWriter, r *http.Request) {
	limit, offset, err := pageParams(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	workspaces, err := h.iamService.ListAccessibleWorkspaces(r.Context(), middleware.GetUserIDFromContext(r.Context()), limit, offset)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, workspaces)
}

type CreateWorkspaceRequest struct {
	Name        string  `json:"name"`
	Description string  `json:"description"`
	ParentID    *string `json:"parent_id,omitempty"`
}

type UpsertWorkspaceMemberRequest struct {
	Role string `json:"role"`
}

// 创建工作空间
func (h *IAMHandler) CreateWorkspace(w http.ResponseWriter, r *http.Request) {
	var req CreateWorkspaceRequest
	if err := decodeJSON(w, r, &req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if req.Name == "" {
		http.Error(w, "name is required", http.StatusBadRequest)
		return
	}
	workspace, err := h.iamService.CreateWorkspaceForUser(r.Context(), middleware.GetUserIDFromContext(r.Context()), req.Name, req.Description, req.ParentID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusCreated, workspace)
}

func (h *IAMHandler) ListWorkspaceMembers(w http.ResponseWriter, r *http.Request) {
	members, err := h.iamService.ListWorkspaceMembers(r.Context(), middleware.GetUserIDFromContext(r.Context()), chi.URLParam(r, "workspace_id"))
	if err != nil {
		writeIAMError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, members)
}

func (h *IAMHandler) UpsertWorkspaceMember(w http.ResponseWriter, r *http.Request) {
	var body UpsertWorkspaceMemberRequest
	if err := decodeJSON(w, r, &body); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	member, err := h.iamService.UpsertWorkspaceMember(r.Context(), middleware.GetUserIDFromContext(r.Context()),
		chi.URLParam(r, "workspace_id"), chi.URLParam(r, "user_id"), body.Role)

	if err != nil {
		writeIAMError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, member)

}

func (h *IAMHandler) DeleteWorkspaceMember(w http.ResponseWriter, r *http.Request) {
	err := h.iamService.DeleteWorkspaceMember(
		r.Context(),
		middleware.GetUserIDFromContext(r.Context()),
		chi.URLParam(r, "workspace_id"),
		chi.URLParam(r, "user_id"),
	)
	if err != nil {
		writeIAMError(w, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func writeIAMError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, iam.ErrForbidden):
		http.Error(
			w,
			"forbidden",
			http.StatusForbidden,
		)

	case errors.Is(err, iam.ErrLastOwner):
		http.Error(
			w,
			err.Error(),
			http.StatusConflict,
		)

	case len(err.Error()) >= 7 && err.Error()[:7] == "invalid":
		http.Error(
			w,
			err.Error(),
			http.StatusBadRequest,
		)

	default:
		http.Error(
			w,
			err.Error(),
			http.StatusInternalServerError,
		)
	}
}
