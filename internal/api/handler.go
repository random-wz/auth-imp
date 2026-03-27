package api

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/idp-service/internal/auth"
	"github.com/idp-service/internal/model"
	"github.com/idp-service/internal/service"
	"github.com/idp-service/internal/user"
)

// Handler REST API 处理器（协议适配层）
type Handler struct {
	userSvc *service.UserService
	authSvc *auth.Service
}

func NewHandler(userSvc *service.UserService, authSvc *auth.Service) *Handler {
	return &Handler{userSvc: userSvc, authSvc: authSvc}
}

// CreateUser POST /api/v1/users
func (h *Handler) CreateUser(c *gin.Context) {
	var req model.CreateUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	u, err := h.userSvc.CreateUser(req)
	if err != nil {
		if err == user.ErrUserAlreadyExists {
			c.JSON(http.StatusConflict, gin.H{"error": "user already exists"})
			return
		}
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, u)
}

// GetUser GET /api/v1/users/:id
func (h *Handler) GetUser(c *gin.Context) {
	u, err := h.userSvc.GetUser(c.Param("id"))
	if err != nil {
		if err == user.ErrUserNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "user not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get user"})
		return
	}
	c.JSON(http.StatusOK, u)
}

// UpdateUser PUT /api/v1/users/:id
func (h *Handler) UpdateUser(c *gin.Context) {
	var req model.UpdateUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	updated, err := h.userSvc.UpdateUser(c.Param("id"), req)
	if err != nil {
		if err == user.ErrUserNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "user not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update user"})
		return
	}
	c.JSON(http.StatusOK, updated)
}

// DeleteUser DELETE /api/v1/users/:id
func (h *Handler) DeleteUser(c *gin.Context) {
	if err := h.userSvc.DeleteUser(c.Param("id")); err != nil {
		if err == user.ErrUserNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "user not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to delete user"})
		return
	}
	c.JSON(http.StatusNoContent, nil)
}

// ListUsers GET /api/v1/users
func (h *Handler) ListUsers(c *gin.Context) {
	offset, _ := strconv.Atoi(c.DefaultQuery("offset", "0"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
	users, total, err := h.userSvc.ListUsers(c.Query("q"), offset, limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list users"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"total": total, "offset": offset, "limit": limit, "items": users})
}

// Login POST /api/v1/auth/login
func (h *Handler) Login(c *gin.Context) {
	var req model.LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	resp, err := h.userSvc.Login(req.Username, req.Password)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, resp)
}

// CreateSyncJob POST /api/v1/sync/jobs
func (h *Handler) CreateSyncJob(c *gin.Context) {
	var req model.CreateSyncJobRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	job, err := h.userSvc.CreateSyncJob(req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create sync job"})
		return
	}
	c.JSON(http.StatusCreated, job)
}

// GetSyncJob GET /api/v1/sync/jobs/:id
func (h *Handler) GetSyncJob(c *gin.Context) {
	job, err := h.userSvc.GetSyncJob(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "sync job not found"})
		return
	}
	c.JSON(http.StatusOK, job)
}

// JWTMiddleware JWT 认证中间件
func JWTMiddleware(authSvc *auth.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		tokenStr := c.GetHeader("Authorization")
		if len(tokenStr) > 7 && tokenStr[:7] == "Bearer " {
			tokenStr = tokenStr[7:]
		}
		if tokenStr == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "missing authorization token"})
			return
		}
		claims, err := authSvc.ValidateToken(tokenStr)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid token"})
			return
		}
		c.Set("user_id", claims.UserID)
		c.Set("username", claims.Username)
		c.Next()
	}
}

// Logout POST /api/v1/auth/logout
func (h *Handler) Logout(c *gin.Context) {
	userID := c.GetString("user_id")
	if userID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	if err := h.userSvc.Logout(userID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "logout failed"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "logged out successfully"})
}

// Online POST /api/v1/auth/online
func (h *Handler) Online(c *gin.Context) {
	userID := c.GetString("user_id")
	if userID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	if err := h.userSvc.SetOnline(userID, true); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to set online"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "online", "user_id": userID})
}
