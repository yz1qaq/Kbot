package api

import (
	"encoding/json"
	"my_kbot/internal/api/middleware"
	"my_kbot/internal/platform/iam"
	"net/http"

	"github.com/go-chi/chi/v5"
)

func NewRouter(iamService *iam.Service) http.Handler {
	router := chi.NewRouter()
	//注册通用中间件
	router.Use(middleware.Recoverer, middleware.RequestID)
	router.Get("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	router.Post("/api/v1/auth/register", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Email    string
			Password string
			Name     string
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalied Json", http.StatusBadRequest)
			return
		}
		user, err := iamService.Register(r.Context(), req.Email, req.Password, req.Name)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		writeJSON(w, http.StatusCreated, user)
	})

	router.Post("/api/v1/auth/login", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Email    string
			Password string
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalied Json", http.StatusBadRequest)
			return
		}
		loginResult, err := iamService.Login(r.Context(), req.Email, req.Password)
		if err != nil {
			http.Error(w, err.Error(), http.StatusUnauthorized)
			return
		}
		writeJSON(w, http.StatusOK, loginResult)
	})

	router.Group(func(protected chi.Router) {
		protected.Use(middleware.Auth(iamService))

		protected.Get("/api/v1/workspaces", func(w http.ResponseWriter, r *http.Request) {
			workspaces, err := iamService.ListUserWorkspaces(r.Context(), middleware.UserID(r.Context()))
			if err != nil {
				http.Error(w, "list workspaces", http.StatusInternalServerError)
				return
			}
			writeJSON(w, http.StatusOK, workspaces)
		})

		protected.With(middleware.Workspace(iamService)).Get(
			"/api/v1/context", func(w http.ResponseWriter, r *http.Request) {
				writeJSON(w, http.StatusOK, map[string]string{
					"user_id":      middleware.UserID(r.Context()),
					"workspace_id": middleware.WorkspaceID(r.Context()),
					"role":         middleware.WorkspaceRole(r.Context()),
				})
			},
		)
	})

	return router
}

// writeJSON 设置响应类型、状态码，并将数据编码为 JSON。
func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
