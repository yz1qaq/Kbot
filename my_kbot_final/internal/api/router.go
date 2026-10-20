package api

import (
	"context"
	"fmt"
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
	readiness []ReadinessCheck
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
	return r
}
