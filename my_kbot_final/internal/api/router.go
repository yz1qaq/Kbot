package api

import (
	"context"
	"fmt"
	"my_kbot_final/internal/api/middleware"
	v1 "my_kbot_final/internal/api/v1"
	"my_kbot_final/internal/platform/iam"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
)

// 描述一项依赖检查
type ReadinessCheck struct {
	Name  string
	Check func(context.Context) error
}

type Handler struct {
	iamService *iam.Service
	readiness  []ReadinessCheck
}

func NewHandler(iamService *iam.Service) *Handler {
	return &Handler{
		iamService: iamService,
	}
}

// 保存依赖检查函数
func (h *Handler) SetReadiness(checks ...ReadinessCheck) *Handler {
	h.readiness = checks
	return h
}

func (h *Handler) ready(w http.ResponseWriter, r *http.Request) {
	ctx, cancle := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancle()

	for _, check := range h.readiness {
		if check.Check == nil {
			continue
		}
		if err := check.Check(ctx); err != nil {
			http.Error(w, fmt.Sprintf("not ready: %s", check.Name), http.StatusServiceUnavailable)
			return
		}
	}
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))

}

func (h *Handler) Routes() http.Handler {
	r := chi.NewRouter()
	r.Get("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	r.Get("/readyz", h.ready)

	if h.iamService != nil {
		r.Route("/api/v1", func(r chi.Router) {
			authHandler := v1.NewAuthHandler(h.iamService)
			iamHandler := v1.NewIAMHandler(h.iamService)
			r.Post("/auth/login", authHandler.Login)

			r.Group(func(protected chi.Router) {
				//需要认证
				protected.Use(middleware.Auth(h.iamService))
				//创建用户需要全局管理员权限
				protected.With(middleware.RequireGlobalAdmin()).Post(
					"/auth/register",
					authHandler.CreateUser,
				)
				protected.With(middleware.RequireGlobalAdmin()).Get(
					"/users",
					iamHandler.ListUsers,
				)

			})

		})

	}
	return r
}
