package service

import (
	"github.com/google/uuid"
	"github.com/idp-service/internal/directory"
	"github.com/idp-service/internal/model"
	"github.com/idp-service/internal/user"
)

// DirectoryService 组织/群组管理业务逻辑
type DirectoryService struct {
	orgStore   directory.OrgStore
	groupStore directory.GroupStore
	userStore  user.Store
}

func NewDirectoryService(orgStore directory.OrgStore, groupStore directory.GroupStore, userStore user.Store) *DirectoryService {
	return &DirectoryService{
		orgStore:   orgStore,
		groupStore: groupStore,
		userStore:  userStore,
	}
}

// ========== 组织 ==========

func (s *DirectoryService) CreateOrg(req model.CreateOrgRequest) (*model.Organization, error) {
	org := &model.Organization{
		ID:           uuid.New().String(),
		Name:         req.Name,
		ParentID:     req.ParentID,
		ExternalID:   req.ExternalID,
		SourceSystem: req.SourceSystem,
	}
	if err := s.orgStore.Create(org); err != nil {
		return nil, err
	}
	return org, nil
}

func (s *DirectoryService) GetOrg(id string) (*model.Organization, error) {
	return s.orgStore.GetByID(id)
}

func (s *DirectoryService) UpdateOrg(id string, req model.UpdateOrgRequest) (*model.Organization, error) {
	if err := s.orgStore.Update(&model.Organization{ID: id, Name: req.Name, ParentID: req.ParentID}); err != nil {
		return nil, err
	}
	return s.orgStore.GetByID(id)
}

func (s *DirectoryService) DeleteOrg(id string) error {
	return s.orgStore.Delete(id)
}

func (s *DirectoryService) ListOrgs(offset, limit int) ([]*model.Organization, int64, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	return s.orgStore.List(offset, limit)
}

func (s *DirectoryService) ListOrgChildren(parentID string) ([]*model.Organization, error) {
	return s.orgStore.ListChildren(parentID)
}

func (s *DirectoryService) AddOrgMember(orgID string, req model.AddOrgMemberRequest) (*model.UserOrgRelation, error) {
	if _, err := s.userStore.GetByID(req.UserID); err != nil {
		return nil, user.ErrUserNotFound
	}
	role := req.Role
	if role == "" {
		role = "member"
	}
	rel := &model.UserOrgRelation{
		UserID:    req.UserID,
		OrgID:     orgID,
		Role:      role,
		IsPrimary: req.IsPrimary,
	}
	if err := s.orgStore.AddMember(rel); err != nil {
		return nil, err
	}
	return rel, nil
}

func (s *DirectoryService) RemoveOrgMember(orgID, userID string) error {
	return s.orgStore.RemoveMember(orgID, userID)
}

func (s *DirectoryService) ListOrgMembers(orgID string) ([]*model.OrgMemberDetail, error) {
	rels, err := s.orgStore.ListMembers(orgID)
	if err != nil {
		return nil, err
	}
	details := make([]*model.OrgMemberDetail, 0, len(rels))
	for _, rel := range rels {
		detail := &model.OrgMemberDetail{UserOrgRelation: *rel}
		if u, err := s.userStore.GetByID(rel.UserID); err == nil {
			detail.Username = u.Username
			detail.Email = u.Email
			detail.DisplayName = u.DisplayName
			detail.Status = u.Status
		}
		details = append(details, detail)
	}
	return details, nil
}

func (s *DirectoryService) GetUserOrgs(userID string) ([]*model.UserOrgRelation, error) {
	return s.orgStore.GetUserOrgs(userID)
}

// ========== 群组 ==========

func (s *DirectoryService) CreateGroup(req model.CreateGroupRequest) (*model.Group, error) {
	group := &model.Group{
		ID:          uuid.New().String(),
		Name:        req.Name,
		Description: req.Description,
		Type:        req.Type,
	}
	if err := s.groupStore.Create(group); err != nil {
		return nil, err
	}
	return group, nil
}

func (s *DirectoryService) GetGroup(id string) (*model.Group, error) {
	return s.groupStore.GetByID(id)
}

func (s *DirectoryService) UpdateGroup(id string, req model.UpdateGroupRequest) (*model.Group, error) {
	if err := s.groupStore.Update(&model.Group{ID: id, Name: req.Name, Description: req.Description, Type: req.Type}); err != nil {
		return nil, err
	}
	return s.groupStore.GetByID(id)
}

func (s *DirectoryService) DeleteGroup(id string) error {
	return s.groupStore.Delete(id)
}

func (s *DirectoryService) ListGroups(offset, limit int) ([]*model.Group, int64, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	return s.groupStore.List(offset, limit)
}

func (s *DirectoryService) AddGroupMember(groupID string, req model.AddGroupMemberRequest) (*model.GroupMember, error) {
	if _, err := s.userStore.GetByID(req.UserID); err != nil {
		return nil, user.ErrUserNotFound
	}
	role := req.Role
	if role == "" {
		role = "member"
	}
	member := &model.GroupMember{
		GroupID: groupID,
		UserID:  req.UserID,
		Role:    role,
	}
	if err := s.groupStore.AddMember(member); err != nil {
		return nil, err
	}
	return member, nil
}

func (s *DirectoryService) RemoveGroupMember(groupID, userID string) error {
	return s.groupStore.RemoveMember(groupID, userID)
}

func (s *DirectoryService) ListGroupMembers(groupID string) ([]*model.GroupMemberDetail, error) {
	members, err := s.groupStore.ListMembers(groupID)
	if err != nil {
		return nil, err
	}
	details := make([]*model.GroupMemberDetail, 0, len(members))
	for _, m := range members {
		detail := &model.GroupMemberDetail{GroupMember: *m}
		if u, err := s.userStore.GetByID(m.UserID); err == nil {
			detail.Username = u.Username
			detail.Email = u.Email
			detail.DisplayName = u.DisplayName
			detail.Status = u.Status
		}
		details = append(details, detail)
	}
	return details, nil
}

func (s *DirectoryService) GetUserGroups(userID string) ([]*model.GroupMember, error) {
	return s.groupStore.GetUserGroups(userID)
}
