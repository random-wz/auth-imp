package api

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/idp-service/internal/directory"
	"github.com/idp-service/internal/model"
	"github.com/idp-service/internal/service"
	"github.com/idp-service/internal/user"
)

// DirectoryHandler 用户目录（组织/群组）协议适配层
type DirectoryHandler struct {
	dirSvc *service.DirectoryService
}

func NewDirectoryHandler(dirSvc *service.DirectoryService) *DirectoryHandler {
	return &DirectoryHandler{dirSvc: dirSvc}
}

// ========== 组织接口 ==========

// CreateOrg POST /api/v1/orgs
func (h *DirectoryHandler) CreateOrg(c *gin.Context) {
	var req model.CreateOrgRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	org, err := h.dirSvc.CreateOrg(req)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, org)
}

// GetOrg GET /api/v1/orgs/:id
func (h *DirectoryHandler) GetOrg(c *gin.Context) {
	org, err := h.dirSvc.GetOrg(c.Param("id"))
	if err != nil {
		if err == directory.ErrOrgNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "organization not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, org)
}

// UpdateOrg PUT /api/v1/orgs/:id
func (h *DirectoryHandler) UpdateOrg(c *gin.Context) {
	var req model.UpdateOrgRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	updated, err := h.dirSvc.UpdateOrg(c.Param("id"), req)
	if err != nil {
		if err == directory.ErrOrgNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "organization not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, updated)
}

// DeleteOrg DELETE /api/v1/orgs/:id
func (h *DirectoryHandler) DeleteOrg(c *gin.Context) {
	if err := h.dirSvc.DeleteOrg(c.Param("id")); err != nil {
		if err == directory.ErrOrgNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "organization not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusNoContent, nil)
}

// ListOrgs GET /api/v1/orgs
func (h *DirectoryHandler) ListOrgs(c *gin.Context) {
	offset, _ := strconv.Atoi(c.DefaultQuery("offset", "0"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
	orgs, total, err := h.dirSvc.ListOrgs(offset, limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"total": total, "offset": offset, "limit": limit, "items": orgs})
}

// ListOrgChildren GET /api/v1/orgs/:id/children
func (h *DirectoryHandler) ListOrgChildren(c *gin.Context) {
	children, err := h.dirSvc.ListOrgChildren(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": children, "total": len(children)})
}

// AddOrgMember POST /api/v1/orgs/:id/members
func (h *DirectoryHandler) AddOrgMember(c *gin.Context) {
	var req model.AddOrgMemberRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	rel, err := h.dirSvc.AddOrgMember(c.Param("id"), req)
	if err != nil {
		switch err {
		case user.ErrUserNotFound:
			c.JSON(http.StatusNotFound, gin.H{"error": "user not found"})
		case directory.ErrOrgNotFound:
			c.JSON(http.StatusNotFound, gin.H{"error": "organization not found"})
		case directory.ErrMemberExists:
			c.JSON(http.StatusConflict, gin.H{"error": "user is already a member"})
		default:
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		}
		return
	}
	c.JSON(http.StatusCreated, rel)
}

// RemoveOrgMember DELETE /api/v1/orgs/:id/members/:userId
func (h *DirectoryHandler) RemoveOrgMember(c *gin.Context) {
	if err := h.dirSvc.RemoveOrgMember(c.Param("id"), c.Param("userId")); err != nil {
		if err == directory.ErrMemberNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "member not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusNoContent, nil)
}

// ListOrgMembers GET /api/v1/orgs/:id/members
func (h *DirectoryHandler) ListOrgMembers(c *gin.Context) {
	details, err := h.dirSvc.ListOrgMembers(c.Param("id"))
	if err != nil {
		if err == directory.ErrOrgNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "organization not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": details, "total": len(details)})
}

// GetUserOrgs GET /api/v1/users/:id/orgs
func (h *DirectoryHandler) GetUserOrgs(c *gin.Context) {
	rels, err := h.dirSvc.GetUserOrgs(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": rels, "total": len(rels)})
}

// ========== 群组接口 ==========

// CreateGroup POST /api/v1/groups
func (h *DirectoryHandler) CreateGroup(c *gin.Context) {
	var req model.CreateGroupRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	group, err := h.dirSvc.CreateGroup(req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, group)
}

// GetGroup GET /api/v1/groups/:id
func (h *DirectoryHandler) GetGroup(c *gin.Context) {
	group, err := h.dirSvc.GetGroup(c.Param("id"))
	if err != nil {
		if err == directory.ErrGroupNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "group not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, group)
}

// UpdateGroup PUT /api/v1/groups/:id
func (h *DirectoryHandler) UpdateGroup(c *gin.Context) {
	var req model.UpdateGroupRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	updated, err := h.dirSvc.UpdateGroup(c.Param("id"), req)
	if err != nil {
		if err == directory.ErrGroupNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "group not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, updated)
}

// DeleteGroup DELETE /api/v1/groups/:id
func (h *DirectoryHandler) DeleteGroup(c *gin.Context) {
	if err := h.dirSvc.DeleteGroup(c.Param("id")); err != nil {
		if err == directory.ErrGroupNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "group not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusNoContent, nil)
}

// ListGroups GET /api/v1/groups
func (h *DirectoryHandler) ListGroups(c *gin.Context) {
	offset, _ := strconv.Atoi(c.DefaultQuery("offset", "0"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
	groups, total, err := h.dirSvc.ListGroups(offset, limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"total": total, "offset": offset, "limit": limit, "items": groups})
}

// AddGroupMember POST /api/v1/groups/:id/members
func (h *DirectoryHandler) AddGroupMember(c *gin.Context) {
	var req model.AddGroupMemberRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	member, err := h.dirSvc.AddGroupMember(c.Param("id"), req)
	if err != nil {
		switch err {
		case user.ErrUserNotFound:
			c.JSON(http.StatusNotFound, gin.H{"error": "user not found"})
		case directory.ErrGroupNotFound:
			c.JSON(http.StatusNotFound, gin.H{"error": "group not found"})
		case directory.ErrMemberExists:
			c.JSON(http.StatusConflict, gin.H{"error": "user is already a member"})
		default:
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		}
		return
	}
	c.JSON(http.StatusCreated, member)
}

// RemoveGroupMember DELETE /api/v1/groups/:id/members/:userId
func (h *DirectoryHandler) RemoveGroupMember(c *gin.Context) {
	if err := h.dirSvc.RemoveGroupMember(c.Param("id"), c.Param("userId")); err != nil {
		if err == directory.ErrMemberNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "member not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusNoContent, nil)
}

// ListGroupMembers GET /api/v1/groups/:id/members
func (h *DirectoryHandler) ListGroupMembers(c *gin.Context) {
	details, err := h.dirSvc.ListGroupMembers(c.Param("id"))
	if err != nil {
		if err == directory.ErrGroupNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "group not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": details, "total": len(details)})
}

// GetUserGroups GET /api/v1/users/:id/groups
func (h *DirectoryHandler) GetUserGroups(c *gin.Context) {
	members, err := h.dirSvc.GetUserGroups(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": members, "total": len(members)})
}
