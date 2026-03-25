package user

import (
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/idp-service/internal/model"
)

// MemoryStore 基于内存的用户存储实现（用于测试和开发）
type MemoryStore struct {
	mu    sync.RWMutex
	users map[string]*model.User // key: id
	byUsername map[string]string // username -> id
	byEmail    map[string]string // email -> id
}

// NewMemoryStore 创建内存存储实例
func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		users:      make(map[string]*model.User),
		byUsername: make(map[string]string),
		byEmail:    make(map[string]string),
	}
}

func (s *MemoryStore) Create(user *model.User) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.byUsername[user.Username]; ok {
		return ErrUserAlreadyExists
	}
	if _, ok := s.byEmail[user.Email]; ok {
		return ErrUserAlreadyExists
	}

	now := time.Now()
	user.CreatedAt = now
	user.UpdatedAt = now
	if user.Status == "" {
		user.Status = model.UserStatusActive
	}

	clone := *user
	s.users[user.ID] = &clone
	s.byUsername[user.Username] = user.ID
	s.byEmail[user.Email] = user.ID
	return nil
}

func (s *MemoryStore) GetByID(id string) (*model.User, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	u, ok := s.users[id]
	if !ok || u.Status == model.UserStatusDeleted {
		return nil, ErrUserNotFound
	}
	clone := *u
	return &clone, nil
}

func (s *MemoryStore) GetByUsername(username string) (*model.User, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	id, ok := s.byUsername[username]
	if !ok {
		return nil, ErrUserNotFound
	}
	u, ok := s.users[id]
	if !ok || u.Status == model.UserStatusDeleted {
		return nil, ErrUserNotFound
	}
	clone := *u
	return &clone, nil
}

func (s *MemoryStore) GetByEmail(email string) (*model.User, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	id, ok := s.byEmail[email]
	if !ok {
		return nil, ErrUserNotFound
	}
	u, ok := s.users[id]
	if !ok || u.Status == model.UserStatusDeleted {
		return nil, ErrUserNotFound
	}
	clone := *u
	return &clone, nil
}

func (s *MemoryStore) Update(user *model.User) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	existing, ok := s.users[user.ID]
	if !ok || existing.Status == model.UserStatusDeleted {
		return ErrUserNotFound
	}

	// 更新 email 索引
	if user.Email != "" && user.Email != existing.Email {
		if _, taken := s.byEmail[user.Email]; taken {
			return ErrUserAlreadyExists
		}
		delete(s.byEmail, existing.Email)
		s.byEmail[user.Email] = user.ID
		existing.Email = user.Email
	}

	if user.DisplayName != "" {
		existing.DisplayName = user.DisplayName
	}
	if user.Status != "" {
		existing.Status = user.Status
	}
	existing.UpdatedAt = time.Now()

	clone := *existing
	s.users[user.ID] = &clone
	return nil
}

func (s *MemoryStore) Delete(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	u, ok := s.users[id]
	if !ok || u.Status == model.UserStatusDeleted {
		return ErrUserNotFound
	}
	u.Status = model.UserStatusDeleted
	u.UpdatedAt = time.Now()
	return nil
}

func (s *MemoryStore) List(offset, limit int) ([]*model.User, int64, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var all []*model.User
	for _, u := range s.users {
		if u.Status != model.UserStatusDeleted {
			clone := *u
			all = append(all, &clone)
		}
	}

	total := int64(len(all))
	if offset >= len(all) {
		return []*model.User{}, total, nil
	}
	end := offset + limit
	if end > len(all) {
		end = len(all)
	}
	return all[offset:end], total, nil
}

func (s *MemoryStore) Search(query string, offset, limit int) ([]*model.User, int64, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	query = strings.ToLower(query)
	var matched []*model.User
	for _, u := range s.users {
		if u.Status == model.UserStatusDeleted {
			continue
		}
		if strings.Contains(strings.ToLower(u.Username), query) ||
			strings.Contains(strings.ToLower(u.Email), query) ||
			strings.Contains(strings.ToLower(u.DisplayName), query) {
			clone := *u
			matched = append(matched, &clone)
		}
	}

	total := int64(len(matched))
	if offset >= len(matched) {
		return []*model.User{}, total, nil
	}
	end := offset + limit
	if end > len(matched) {
		end = len(matched)
	}
	return matched[offset:end], total, nil
}

// MemorySyncJobStore 基于内存的同步任务存储
type MemorySyncJobStore struct {
	mu   sync.RWMutex
	jobs map[string]*model.SyncJob
}

// NewMemorySyncJobStore 创建内存同步任务存储
func NewMemorySyncJobStore() *MemorySyncJobStore {
	return &MemorySyncJobStore{
		jobs: make(map[string]*model.SyncJob),
	}
}

func (s *MemorySyncJobStore) Create(job *model.SyncJob) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	job.CreatedAt = now
	job.UpdatedAt = now
	clone := *job
	s.jobs[job.ID] = &clone
	return nil
}

func (s *MemorySyncJobStore) GetByID(id string) (*model.SyncJob, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	j, ok := s.jobs[id]
	if !ok {
		return nil, errors.New("sync job not found")
	}
	clone := *j
	return &clone, nil
}

func (s *MemorySyncJobStore) Update(job *model.SyncJob) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	_, ok := s.jobs[job.ID]
	if !ok {
		return errors.New("sync job not found")
	}
	job.UpdatedAt = time.Now()
	clone := *job
	s.jobs[job.ID] = &clone
	return nil
}

func (s *MemorySyncJobStore) List(offset, limit int) ([]*model.SyncJob, int64, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var all []*model.SyncJob
	for _, j := range s.jobs {
		clone := *j
		all = append(all, &clone)
	}

	total := int64(len(all))
	if offset >= len(all) {
		return []*model.SyncJob{}, total, nil
	}
	end := offset + limit
	if end > len(all) {
		end = len(all)
	}
	return all[offset:end], total, nil
}
