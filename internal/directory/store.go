package directory

import (
	"errors"

	"github.com/idp-service/internal/model"
)

var (
	ErrOrgNotFound      = errors.New("organization not found")
	ErrOrgAlreadyExists = errors.New("organization already exists")
	ErrGroupNotFound    = errors.New("group not found")
	ErrMemberNotFound   = errors.New("member not found")
	ErrMemberExists     = errors.New("member already exists")
)

// OrgStore 组织存储接口
type OrgStore interface {
	Create(org *model.Organization) error
	GetByID(id string) (*model.Organization, error)
	Update(org *model.Organization) error
	Delete(id string) error
	List(offset, limit int) ([]*model.Organization, int64, error)
	// ListChildren 列出直属子组织
	ListChildren(parentID string) ([]*model.Organization, error)
	// AddMember 将用户加入组织
	AddMember(rel *model.UserOrgRelation) error
	// RemoveMember 从组织移除用户
	RemoveMember(orgID, userID string) error
	// ListMembers 列出组织成员关系
	ListMembers(orgID string) ([]*model.UserOrgRelation, error)
	// GetUserOrgs 获取用户所属的所有组织
	GetUserOrgs(userID string) ([]*model.UserOrgRelation, error)
}

// GroupStore 群组存储接口
type GroupStore interface {
	Create(group *model.Group) error
	GetByID(id string) (*model.Group, error)
	Update(group *model.Group) error
	Delete(id string) error
	List(offset, limit int) ([]*model.Group, int64, error)
	// AddMember 添加群组成员
	AddMember(member *model.GroupMember) error
	// RemoveMember 移除群组成员
	RemoveMember(groupID, userID string) error
	// ListMembers 列出群组成员
	ListMembers(groupID string) ([]*model.GroupMember, error)
	// GetUserGroups 获取用户所属群组
	GetUserGroups(userID string) ([]*model.GroupMember, error)
}
