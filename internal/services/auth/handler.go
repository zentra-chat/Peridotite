package auth

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/zentra/peridotite/internal/middleware"
	"github.com/zentra/peridotite/internal/utils"
)

type Handler struct {
	service *Service
}

func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

func (h *Handler) Routes() chi.Router {
	r := chi.NewRouter()

	// Public routes (with strict rate limiting)
	r.Group(func(r chi.Router) {
		r.Use(middleware.StrictRateLimitMiddleware(10)) // 10 requests per minute, I need to tune this later
		r.Post("/register", h.Register)
		r.Post("/login", h.Login)
		r.Post("/portable", h.PortableAuth)
		r.Post("/refresh", h.RefreshToken)
	})

	// Authenticated routes
	r.Group(func(r chi.Router) {
		r.Post("/logout", h.Logout)
		r.Post("/logout-all", h.LogoutAll)
		r.Post("/change-password", h.ChangePassword)

		// 2FA routes
		r.Post("/2fa/enable", h.Enable2FA)
		r.Post("/2fa/verify", h.Verify2FA)
		r.Post("/2fa/disable", h.Disable2FA)
	})

	return r
}

func (h *Handler) Register(w http.ResponseWriter, r *http.Request) {
	var req RegisterRequest
	if err := utils.DecodeJSON(r, &req); err != nil {
		utils.RespondError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	if err := utils.Validate(&req); err != nil {
		utils.RespondValidationError(w, utils.FormatValidationErrors(err))
		return
	}

	if req.PortableProfile != nil {
		if err := utils.Validate(req.PortableProfile); err != nil {
			utils.RespondValidationError(w, utils.FormatValidationErrors(err))
			return
		}
	}

	resp, err := h.service.Register(r.Context(), &req)
	if err != nil {
		switch err {
		case ErrUserExists:
			utils.RespondErrorWithCode(w, http.StatusConflict, "USER_EXISTS", "Username or email already in use")
		default:
			utils.RespondError(w, http.StatusInternalServerError, "Failed to register user")
		}
		return
	}

	utils.RespondCreated(w, resp)
}

func (h *Handler) Login(w http.ResponseWriter, r *http.Request) {
	var req LoginRequest
	if err := utils.DecodeJSON(r, &req); err != nil {
		utils.RespondError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	if err := utils.Validate(&req); err != nil {
		utils.RespondValidationError(w, utils.FormatValidationErrors(err))
		return
	}

	if req.PortableProfile != nil {
		if err := utils.Validate(req.PortableProfile); err != nil {
			utils.RespondValidationError(w, utils.FormatValidationErrors(err))
			return
		}
	}

	resp, err := h.service.Login(r.Context(), &req)
	if err != nil {
		switch err {
		case ErrInvalidCredentials:
			utils.RespondErrorWithCode(w, http.StatusUnauthorized, "INVALID_CREDENTIALS", "Invalid username/email or password")
		case ErrInvalid2FA:
			utils.RespondErrorWithCode(w, http.StatusUnauthorized, "INVALID_2FA", "Invalid 2FA code")
		default:
			utils.RespondError(w, http.StatusInternalServerError, "Failed to login")
		}
		return
	}

	utils.RespondSuccess(w, resp)
}

func (h *Handler) RefreshToken(w http.ResponseWriter, r *http.Request) {
	var req RefreshRequest
	if err := utils.DecodeJSON(r, &req); err != nil {
		utils.RespondError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	resp, err := h.service.RefreshToken(r.Context(), req.RefreshToken)
	if err != nil {
		switch err {
		case ErrSessionNotFound, ErrSessionExpired:
			utils.RespondErrorWithCode(w, http.StatusUnauthorized, "INVALID_SESSION", "Session not found or expired")
		default:
			utils.RespondError(w, http.StatusInternalServerError, "Failed to refresh token")
		}
		return
	}

	utils.RespondSuccess(w, resp)
}

func (h *Handler) PortableAuth(w http.ResponseWriter, r *http.Request) {
	var req PortableAuthRequest
	if err := utils.DecodeJSON(r, &req); err != nil {
		utils.RespondError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	if err := utils.Validate(&req); err != nil {
		utils.RespondValidationError(w, utils.FormatValidationErrors(err))
		return
	}

	if err := utils.Validate(req.PortableProfile); err != nil {
		utils.RespondValidationError(w, utils.FormatValidationErrors(err))
		return
	}

	resp, err := h.service.PortableAuth(r.Context(), &req)
	if err != nil {
		switch err {
		case ErrPortableProfileReq:
			utils.RespondErrorWithCode(w, http.StatusBadRequest, "PROFILE_REQUIRED", "Portable profile is required")
		default:
			utils.RespondError(w, http.StatusInternalServerError, "Failed to authenticate with portable profile")
		}
		return
	}

	utils.RespondSuccess(w, resp)
}

func (h *Handler) Logout(w http.ResponseWriter, r *http.Request) {
	userID, err := middleware.RequireAuth(r.Context())
	if err != nil {
		utils.RespondError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}

	var req RefreshRequest
	if err := utils.DecodeJSON(r, &req); err != nil {
		utils.RespondError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	if err := h.service.Logout(r.Context(), userID, req.RefreshToken); err != nil {
		utils.RespondError(w, http.StatusInternalServerError, "Failed to logout")
		return
	}

	utils.RespondNoContent(w)
}

func (h *Handler) LogoutAll(w http.ResponseWriter, r *http.Request) {
	userID, err := middleware.RequireAuth(r.Context())
	if err != nil {
		utils.RespondError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}

	if err := h.service.LogoutAll(r.Context(), userID); err != nil {
		utils.RespondError(w, http.StatusInternalServerError, "Failed to logout from all devices")
		return
	}

	utils.RespondNoContent(w)
}

func (h *Handler) ChangePassword(w http.ResponseWriter, r *http.Request) {
	userID, err := middleware.RequireAuth(r.Context())
	if err != nil {
		utils.RespondError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}

	var req struct {
		CurrentPassword string `json:"currentPassword" validate:"required"`
		NewPassword     string `json:"newPassword" validate:"required,strongpassword"`
	}
	if err := utils.DecodeJSON(r, &req); err != nil {
		utils.RespondError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	if err := utils.Validate(&req); err != nil {
		utils.RespondValidationError(w, utils.FormatValidationErrors(err))
		return
	}

	if err := h.service.ChangePassword(r.Context(), userID, req.CurrentPassword, req.NewPassword); err != nil {
		switch err {
		case ErrInvalidCredentials:
			utils.RespondErrorWithCode(w, http.StatusUnauthorized, "INVALID_PASSWORD", "Current password is incorrect")
		default:
			utils.RespondError(w, http.StatusInternalServerError, "Failed to change password")
		}
		return
	}

	utils.RespondJSON(w, http.StatusOK, map[string]string{"message": "Password changed successfully. Please login again."})
}

func (h *Handler) Enable2FA(w http.ResponseWriter, r *http.Request) {
	userID, err := middleware.RequireAuth(r.Context())
	if err != nil {
		utils.RespondError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}

	resp, err := h.service.Enable2FA(r.Context(), userID)
	if err != nil {
		utils.RespondError(w, http.StatusInternalServerError, "Failed to enable 2FA")
		return
	}

	utils.RespondSuccess(w, resp)
}

func (h *Handler) Verify2FA(w http.ResponseWriter, r *http.Request) {
	userID, err := middleware.RequireAuth(r.Context())
	if err != nil {
		utils.RespondError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}

	var req struct {
		Code string `json:"code" validate:"required,len=6"`
	}
	if err := utils.DecodeJSON(r, &req); err != nil {
		utils.RespondError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	if err := h.service.Verify2FA(r.Context(), userID, req.Code); err != nil {
		switch err {
		case ErrInvalid2FA:
			utils.RespondErrorWithCode(w, http.StatusBadRequest, "INVALID_CODE", "Invalid verification code")
		default:
			utils.RespondError(w, http.StatusInternalServerError, "Failed to verify 2FA")
		}
		return
	}

	utils.RespondJSON(w, http.StatusOK, map[string]string{"message": "2FA enabled successfully"})
}

func (h *Handler) Disable2FA(w http.ResponseWriter, r *http.Request) {
	userID, err := middleware.RequireAuth(r.Context())
	if err != nil {
		utils.RespondError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}

	var req struct {
		Password string `json:"password" validate:"required"`
		Code     string `json:"code" validate:"required,len=6"`
	}
	if err := utils.DecodeJSON(r, &req); err != nil {
		utils.RespondError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	if err := h.service.Disable2FA(r.Context(), userID, req.Password, req.Code); err != nil {
		switch err {
		case ErrInvalidCredentials:
			utils.RespondErrorWithCode(w, http.StatusUnauthorized, "INVALID_PASSWORD", "Password is incorrect")
		case ErrInvalid2FA:
			utils.RespondErrorWithCode(w, http.StatusBadRequest, "INVALID_CODE", "Invalid 2FA code")
		default:
			utils.RespondError(w, http.StatusInternalServerError, "Failed to disable 2FA")
		}
		return
	}

	utils.RespondJSON(w, http.StatusOK, map[string]string{"message": "2FA disabled successfully"})
}
