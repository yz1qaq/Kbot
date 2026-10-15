package api

import (
	"encoding/json"
	"fmt"
	"my_kbot/internal/api/middleware"
	"my_kbot/internal/platform/iam"
	"net/http"
	"time"

	platformtool "my_kbot/internal/platform/tool"

	"github.com/go-chi/chi/v5"
)

type ControlPlane struct {
	Tools *platformtool.Registry
}

func NewRouterWithControlPlane(iamService *iam.Service, runtime ChatRuntime, control ControlPlane) http.Handler {
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

		if control.Tools != nil {
			protected.With(middleware.Workspace(iamService)).Get(
				"/api/v1/tools",
				func(w http.ResponseWriter, r *http.Request) {
					versions := control.Tools.List(
						r.Context(), middleware.WorkspaceID(r.Context()),
					)
					writeJSON(w, http.StatusOK, versions)
				},
			)
			protected.With(middleware.Workspace(iamService)).Post(
				"/api/v1/tools",
				func(w http.ResponseWriter, r *http.Request) {
					var req struct {
						Name           string `json:"name"`
						SourceType     string `json:"source_type"`
						Description    string `json:"description"`
						SchemaJSON     string `json:"schema_json"`
						EndpointConfig string `json:"endpoint_config"`
						AuthConfig     string `json:"auth_config"`
						Sensitive      bool   `json:"sensitive"`
					}
					if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
						http.Error(w, "invalid JSON", http.StatusBadRequest)
						return
					}
					var endpoint struct {
						URL string `json:"url"`
					}

					if err := json.Unmarshal([]byte(req.EndpointConfig), &endpoint); err != nil {
						http.Error(w, "invalid endpoint_config", http.StatusBadRequest)
						return
					}
					versionID := fmt.Sprintf("tool-version-%d", time.Now().UnixNano())
					version := platformtool.Version{
						ID:          versionID,
						ToolID:      versionID,
						WorkspaceID: middleware.WorkspaceID(r.Context()),
						Name:        req.Name,
						SourceType:  req.SourceType,
						Description: req.Description,
						InputSchema: []byte(req.SchemaJSON),
						Endpoint:    endpoint.URL,
						AuthConfig:  req.AuthConfig,
						HasAuth:     req.AuthConfig != "",
						Sensitive:   req.Sensitive,
						Published:   true,
						CreatedAt:   time.Now().UTC(),
					}
					if err := control.Tools.Register(r.Context(), version); err != nil {
						http.Error(w, err.Error(), http.StatusBadRequest)
						return
					}
					writeJSON(w, http.StatusCreated, version)
				},
			)
		}

		if runtime != nil {
			protected.With(middleware.Workspace(iamService)).Post(
				"/stream/agents/{agentsID}/chat",
				NewstreamHandler(runtime).ServeHTTP,
			)

		}
	})
	return router
}

func firstRuntime(runtimes []ChatRuntime) ChatRuntime {
	if len(runtimes) == 0 {
		return nil
	}
	return runtimes[0]
}

// writeJSON 设置响应类型、状态码，并将数据编码为 JSON。
func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func NewRouter(iamService *iam.Service, runtimes ...ChatRuntime) http.Handler {
	return NewRouterWithControlPlane(
		iamService,
		firstRuntime(runtimes),
		ControlPlane{},
	)
}
