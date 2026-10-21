package v1

import (
	"errors"
	"my_kbot_final/internal/platform/iam"
	"net/http"
)

type AuthHandler struct {
	iamService *iam.Service
}

func NewAuthHandler(iamService *iam.Service) *AuthHandler {
	return &AuthHandler{iamService: iamService}
}

func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	var req iam.LoginRequest
	if err := decodeJSON(w, r, &req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	reqsp, err := h.iamService.Login(r.Context(), req)
	if err != nil {
		http.Error(w, "invalid credentials", http.StatusUnauthorized)
		return
	}
	writeJSON(w, http.StatusOK, reqsp)
}

type CreateUserRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
	Name     string `json:"name"`
}

func (h *AuthHandler) CreateUser(w http.ResponseWriter, r *http.Request) {
	var req CreateUserRequest
	if err := decodeJSON(w, r, &req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	user, err := h.iamService.CreateUser(r.Context(), req.Email, req.Password, req.Name)
	if err != nil {
		if errors.Is(err, iam.ErrInvalidEmail) ||
			errors.Is(err, iam.ErrWeakPassword) ||
			errors.Is(err, iam.ErrNameRequired) {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusCreated, user)
}
