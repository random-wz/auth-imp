package directory

import (
	"fmt"
	"sync"
	"time"

	"github.com/idp-service/internal/model"
)

// MemoryOrgStore 基于内存的组织存储
type MemoryOrgStore struct {
	mu      sync.RWMutex
	orgs    map[string]*model.Organization // id -> org
	members map[string][]*model.UserOrgRelation // orgID -> []relation
	userOrgs map[string][]*model.UserOrgRelation // userID -> []relation
}

func NewMemoryOrgStore() *MemoryOrgStore {
	return &MemoryOrgStore{
		orgs:     make(map[string]*model.Organization),
		members:  make(map[string][]*model.UserOrgRelation),
		userOrgs: make(map[string][]*model.UserOrgRelation),
	}
}

func (s *MemoryOrgStore) Create(org *model.Organization) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.orgs[org.ID]; ok {
		return ErrOrgAlreadyExists
	}

	now := time.Now()
	org.CreatedAt = now
	org.UpdatedAt = now

	// 构建层级路径
	if org.ParentID == "" {
		org.Path = "/" + org.ID
	} else {
		parent, ok := s.orgs[org.ParentID]
		if !ok {
			return fmt.Errorf("parent organization %s not found", org.ParentID)
		}
		org.Path = parent.Path + "/" + org.ID
	}

	clone := *org
	s.orgs[org.ID] = &clone
	return nil
}

func (s *MemoryOrgStore) GetByID(id string) (*model.Organization, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	org, ok := s.orgs[id]
	if !ok {
		return nil, ErrOrgNotFound
	}
	clone := *org
	return &clone, nil
}

func (s *MemoryOrgStore) Update(org *model.Organization) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	existing, ok := s.orgs[org.ID]
	if !ok {
		return ErrOrgNotFound
	}

	if org.Name != "" {
		existing.Name = org.Name
	}
	if org.ParentID != "" {
		existing.ParentID = org.ParentID
	}
	existing.UpdatedAt = time.Now()

	clone := *existing
	s.orgs[org.ID] = &clone
	return nil
}

func (s *MemoryOrgStore) Delete(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.orgs[id]; !ok {
		return ErrOrgNotFound
	}

	// 清理成员关系
	for _, rel := range s.members[id] {
		s.removeUserOrgRelation(rel.UserID, id)
	}
	delete(s.members, id)
	delete(s.orgs, id)
	return nil
}

func (s *MemoryOrgStore) removeUserOrgRelation(userID, orgID string) {
	rels := s.userOrgs[userID]
	var kept []*model.UserOrgRelation
	for _, r := range rels {
		if r.OrgID != orgID {
			kept = append(kept, r)
		}
	}
	s.userOrgs[userID] = kept
}

func (s *MemoryOrgStore) List(offset, limit int) ([]*model.Organization, int64, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	all := make([]*model.Organization, 0, len(s.orgs))
	for _, o := range s.orgs {
		clone := *o
		all = append(all, &clone)
	}

	total := int64(len(all))
	if offset >= len(all) {
		return []*model.Organization{}, total, nil
	}
	end := offset + limit
	if end > len(all) {
		end = len(all)
	}
	return all[offset:end], total, nil
}

func (s *MemoryOrgStore) ListChildren(parentID string) ([]*model.Organization, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var children []*model.Organization
	for _, o := range s.orgs {
		if o.ParentID == parentID {
			clone := *o
			children = append(children, &clone)
		}
	}
	return children, nil
}

func (s *MemoryOrgStore) AddMember(rel *model.UserOrgRelation) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.orgs[rel.OrgID]; !ok {
		return ErrOrgNotFound
	}

	// 检查是否已存在
	for _, r := range s.members[rel.OrgID] {
		if r.UserID == rel.UserID {
			return ErrMemberExists
		}
	}

	clone := *rel
	s.members[rel.OrgID] = append(s.members[rel.OrgID], &clone)
	s.userOrgs[rel.UserID] = append(s.userOrgs[rel.UserID], &clone)
	return nil
}

func (s *MemoryOrgStore) RemoveMember(orgID, userID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	rels := s.members[orgID]
	var kept []*model.UserOrgRelation
	found := false
	for _, r := range rels {
		if r.UserID == userID {
			found = true
		} else {
			kept = append(kept, r)
		}
	}
	if !found {
		return ErrMemberNotFound
	}
	s.members[orgID] = kept
	s.removeUserOrgRelation(userID, orgID)
	return nil
}

func (s *MemoryOrgStore) ListMembers(orgID string) ([]*model.UserOrgRelation, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if _, ok := s.orgs[orgID]; !ok {
		return nil, ErrOrgNotFound
	}

	rels := s.members[orgID]
	result := make([]*model.UserOrgRelation, len(rels))
	for i, r := range rels {
		clone := *r
		result[i] = &clone
	}
	return result, nil
}

func (s *MemoryOrgStore) GetUserOrgs(userID string) ([]*model.UserOrgRelation, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	rels := s.userOrgs[userID]
	result := make([]*model.UserOrgRelation, len(rels))
	for i, r := range rels {
		clone := *r
		result[i] = &clone
	}
	return result, nil
}

// MemoryGroupStore 基于内存的群组存储
type MemoryGroupStore struct {
	mu         sync.RWMutex
	groups     map[string]*model.Group          // id -> group
	members    map[string][]*model.GroupMember  // groupID -> []member
	userGroups map[string][]*model.GroupMember  // userID -> []member
}

func NewMemoryGroupStore() *MemoryGroupStore {
	return &MemoryGroupStore{
		groups:     make(map[string]*model.Group),
		members:    make(map[string][]*model.GroupMember),
		userGroups: make(map[string][]*model.GroupMember),
	}
}

func (s *MemoryGroupStore) Create(group *model.Group) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.groups[group.ID]; ok {
		return fmt.Errorf("group already exists")
	}

	now := time.Now()
	group.CreatedAt = now
	group.UpdatedAt = now
	clone := *group
	s.groups[group.ID] = &clone
	return nil
}

func (s *MemoryGroupStore) GetByID(id string) (*model.Group, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	g, ok := s.groups[id]
	if !ok {
		return nil, ErrGroupNotFound
	}
	clone := *g
	return &clone, nil
}

func (s *MemoryGroupStore) Update(group *model.Group) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	existing, ok := s.groups[group.ID]
	if !ok {
		return ErrGroupNotFound
	}
	if group.Name != "" {
		existing.Name = group.Name
	}
	if group.Description != "" {
		existing.Description = group.Description
	}
	if group.Type != "" {
		existing.Type = group.Type
	}
	existing.UpdatedAt = time.Now()
	clone := *existing
	s.groups[group.ID] = &clone
	return nil
}

func (s *MemoryGroupStore) Delete(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.groups[id]; !ok {
		return ErrGroupNotFound
	}
	for _, m := range s.members[id] {
		s.removeUserGroupRelation(m.UserID, id)
	}
	delete(s.members, id)
	delete(s.groups, id)
	return nil
}

func (s *MemoryGroupStore) removeUserGroupRelation(userID, groupID string) {
	members := s.userGroups[userID]
	var kept []*model.GroupMember
	for _, m := range members {
		if m.GroupID != groupID {
			kept = append(kept, m)
		}
	}
	s.userGroups[userID] = kept
}

func (s *MemoryGroupStore) List(offset, limit int) ([]*model.Group, int64, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	all := make([]*model.Group, 0, len(s.groups))
	for _, g := range s.groups {
		clone := *g
		all = append(all, &clone)
	}

	total := int64(len(all))
	if offset >= len(all) {
		return []*model.Group{}, total, nil
	}
	end := offset + limit
	if end > len(all) {
		end = len(all)
	}
	return all[offset:end], total, nil
}

func (s *MemoryGroupStore) AddMember(member *model.GroupMember) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.groups[member.GroupID]; !ok {
		return ErrGroupNotFound
	}
	for _, m := range s.members[member.GroupID] {
		if m.UserID == member.UserID {
			return ErrMemberExists
		}
	}

	member.JoinedAt = time.Now()
	clone := *member
	s.members[member.GroupID] = append(s.members[member.GroupID], &clone)
	s.userGroups[member.UserID] = append(s.userGroups[member.UserID], &clone)
	return nil
}

func (s *MemoryGroupStore) RemoveMember(groupID, userID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	rels := s.members[groupID]
	var kept []*model.GroupMember
	found := false
	for _, m := range rels {
		if m.UserID == userID {
			found = true
		} else {
			kept = append(kept, m)
		}
	}
	if !found {
		return ErrMemberNotFound
	}
	s.members[groupID] = kept
	s.removeUserGroupRelation(userID, groupID)
	return nil
}

func (s *MemoryGroupStore) ListMembers(groupID string) ([]*model.GroupMember, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if _, ok := s.groups[groupID]; !ok {
		return nil, ErrGroupNotFound
	}
	rels := s.members[groupID]
	result := make([]*model.GroupMember, len(rels))
	for i, m := range rels {
		clone := *m
		result[i] = &clone
	}
	return result, nil
}

func (s *MemoryGroupStore) GetUserGroups(userID string) ([]*model.GroupMember, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	rels := s.userGroups[userID]
	result := make([]*model.GroupMember, len(rels))
	for i, m := range rels {
		clone := *m
		result[i] = &clone
	}
	return result, nil
}

