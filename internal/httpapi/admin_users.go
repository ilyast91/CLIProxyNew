package httpapi

import (
	"context"
	"errors"
	"net/http"
	"net/netip"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/ilyast91/CLIProxyNew/internal/store"
)

type adminUserStore interface {
	List(context.Context) ([]store.User, error)
	SetStatusWithAudit(context.Context, int64, int64, string, *netip.Addr) error
}

type sessionUserInvalidator interface {
	InvalidateUser(int64)
}

// AdminUserHandler обслуживает admin-операции с пользователями.
type AdminUserHandler struct {
	store        adminUserStore
	invalidators []sessionUserInvalidator
}

type adminUserResponse struct {
	ID             int64     `json:"id"`
	Username       string    `json:"username"`
	Email          string    `json:"email"`
	Role           string    `json:"role"`
	Status         string    `json:"status"`
	IdentitySource string    `json:"identity_source"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

type setUserStatusRequest struct {
	Status string `json:"status"`
}

// NewAdminUserHandler создаёт handler admin-операций с пользователями.
func NewAdminUserHandler(userStore adminUserStore, invalidators ...sessionUserInvalidator) *AdminUserHandler {
	configured := make([]sessionUserInvalidator, 0, len(invalidators))
	for _, invalidator := range invalidators {
		if invalidator != nil {
			configured = append(configured, invalidator)
		}
	}
	return &AdminUserHandler{store: userStore, invalidators: configured}
}

// List возвращает пользователей для администратора.
func (h *AdminUserHandler) List(c *gin.Context) {
	if h == nil || h.store == nil {
		writeError(c, http.StatusInternalServerError, "admin user service is unavailable")
		return
	}
	users, err := h.store.List(c.Request.Context())
	if err != nil {
		writeError(c, http.StatusInternalServerError, "list users failed")
		return
	}
	response := make([]adminUserResponse, 0, len(users))
	for _, user := range users {
		response = append(response, adminUserResponse{
			ID: user.ID, Username: user.Username, Email: user.Email, Role: user.Role,
			Status: user.Status, IdentitySource: user.IdentitySource,
			CreatedAt: user.CreatedAt, UpdatedAt: user.UpdatedAt,
		})
	}
	c.JSON(http.StatusOK, gin.H{"data": response})
}

// SetStatus блокирует или разблокирует пользователя и пишет admin audit log.
func (h *AdminUserHandler) SetStatus(c *gin.Context) {
	actorUserID, ok := currentUserID(c)
	if !ok {
		return
	}
	if h == nil || h.store == nil {
		writeError(c, http.StatusInternalServerError, "admin user service is unavailable")
		return
	}
	targetUserID, err := strconv.ParseInt(c.Param("userID"), 10, 64)
	if err != nil || targetUserID <= 0 {
		writeError(c, http.StatusBadRequest, "invalid user ID")
		return
	}
	var request setUserStatusRequest
	if err := c.ShouldBindJSON(&request); err != nil || (request.Status != "active" && request.Status != "blocked") {
		writeError(c, http.StatusBadRequest, "invalid user status")
		return
	}
	if err := h.store.SetStatusWithAudit(c.Request.Context(), actorUserID, targetUserID, request.Status, requestActorIP(c)); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(c, http.StatusNotFound, "user not found")
			return
		}
		if errors.Is(err, store.ErrInvalidInput) {
			writeError(c, http.StatusBadRequest, "invalid user status")
			return
		}
		writeError(c, http.StatusInternalServerError, "set user status failed")
		return
	}
	for _, invalidator := range h.invalidators {
		invalidator.InvalidateUser(targetUserID)
	}
	c.Status(http.StatusNoContent)
}

func requestActorIP(c *gin.Context) *netip.Addr {
	ip, err := netip.ParseAddr(c.ClientIP())
	if err != nil {
		return nil
	}
	return &ip
}
