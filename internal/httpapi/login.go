package httpapi

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/ilyast91/CLIProxyNew/internal/auth/identity"
)

// LoginService описывает application-сервис login без зависимости от Gin.
type LoginService interface {
	Login(ctx context.Context, username, password string) (identity.LoginResult, error)
}

// LoginHandler адаптирует login service к management HTTP API.
type LoginHandler struct {
	service      LoginService
	secureCookie bool
}

type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type loginResponse struct {
	UserID    int64     `json:"user_id"`
	Role      string    `json:"role"`
	ExpiresAt time.Time `json:"expires_at"`
}

type errorResponse struct {
	Error errorBody `json:"error"`
}

type errorBody struct {
	Message string `json:"message"`
}

// NewLoginHandler создаёт login handler. secureCookie включается в production.
func NewLoginHandler(service LoginService, secureCookie bool) *LoginHandler {
	return &LoginHandler{service: service, secureCookie: secureCookie}
}

// Handle обрабатывает POST /api/v1/login и устанавливает opaque session-cookie.
func (h *LoginHandler) Handle(c *gin.Context) {
	if h == nil || h.service == nil {
		writeError(c, http.StatusInternalServerError, "authentication service is unavailable")
		return
	}
	var request loginRequest
	if err := c.ShouldBindJSON(&request); err != nil || request.Username == "" || request.Password == "" {
		writeError(c, http.StatusBadRequest, "username and password are required")
		return
	}

	result, err := h.service.Login(c.Request.Context(), request.Username, request.Password)
	if err != nil {
		switch {
		case errors.Is(err, identity.ErrInvalidCredentials):
			writeError(c, http.StatusUnauthorized, "invalid credentials")
		case errors.Is(err, identity.ErrUserBlocked), errors.Is(err, identity.ErrAccessDenied):
			writeError(c, http.StatusForbidden, "access denied")
		default:
			writeError(c, http.StatusInternalServerError, "authentication failed")
		}
		return
	}

	http.SetCookie(c.Writer, &http.Cookie{
		Name:     identity.SessionCookieName,
		Value:    result.Token,
		Path:     "/",
		Expires:  result.ExpiresAt,
		HttpOnly: true,
		Secure:   h.secureCookie,
		SameSite: http.SameSiteLaxMode,
	})
	c.JSON(http.StatusOK, loginResponse{UserID: result.UserID, Role: result.Role, ExpiresAt: result.ExpiresAt})
}

func writeError(c *gin.Context, status int, message string) {
	c.AbortWithStatusJSON(status, errorResponse{Error: errorBody{Message: message}})
}
