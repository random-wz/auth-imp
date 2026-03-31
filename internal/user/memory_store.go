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
	users      sync.Map // key: id -> *model.User
	byUsername sync.Map // username -> id
	byEmail    sync.Map // email -> id
}

// NewMemoryStore 创建内存存储实例
func NewMemoryStore() *MemoryStore {
	return &MemoryStore{}
}

func (s *MemoryStore) Create(user *model.User) error {
	// 检查用户名是否已存在
	if _, exists := s.byUsername.LoadOrStore(user.Username, user.ID); exists {
		return ErrUserAlreadyExists
	}
	// 检查邮箱是否已存在
	if _, exists := s.byEmail.LoadOrStore(user.Email, user.ID); exists {
		s.byUsername.Delete(user.Username)
		return ErrUserAlreadyExists
	}

	now := time.Now()
	user.CreatedAt = now
	user.UpdatedAt = now
	if user.Status == "" {
		user.Status = model.UserStatusActive
	}

	clone := *user
	s.users.Store(user.ID, &clone)
	return nil
}

func (s *MemoryStore) GetByID(id string) (*model.User, error) {
	val, ok := s.users.Load(id)
	if !ok {
		return nil, ErrUserNotFound
	}
	u := val.(*model.User)
	if u.Status == model.UserStatusDeleted {
		return nil, ErrUserNotFound
	}
	clone := *u
	return &clone, nil
}

func (s *MemoryStore) GetByUsername(username string) (*model.User, error) {
	idVal, ok := s.byUsername.Load(username)
	if !ok {
		return nil, ErrUserNotFound
	}
	return s.GetByID(idVal.(string))
}

func (s *MemoryStore) GetByEmail(email string) (*model.User, error) {
	idVal, ok := s.byEmail.Load(email)
	if !ok {
		return nil, ErrUserNotFound
	}
	return s.GetByID(idVal.(string))
}

func (s *MemoryStore) Update(user *model.User) error {
	val, ok := s.users.Load(user.ID)
	if !ok {
		return ErrUserNotFound
	}
	existing := val.(*model.User)
	if existing.Status == model.UserStatusDeleted {
		return ErrUserNotFound
	}

	updated := *existing

	// 更新 email 索引
	if user.Email != "" && user.Email != existing.Email {
		if _, exists := s.byEmail.LoadOrStore(user.Email, user.ID); exists {
			return ErrUserAlreadyExists
		}
		s.byEmail.Delete(existing.Email)
		updated.Email = user.Email
	}

	if user.DisplayName != "" {
		updated.DisplayName = user.DisplayName
	}
	if user.Status != "" {
		updated.Status = user.Status
	}
	// 更新在线状态
	updated.IsOnline = user.IsOnline
	updated.UpdatedAt = time.Now()

	s.users.Store(user.ID, &updated)
	return nil
}

func (s *MemoryStore) Delete(id string) error {
	val, ok := s.users.Load(id)
	if !ok {
		return ErrUserNotFound
	}
	u := val.(*model.User)
	if u.Status == model.UserStatusDeleted {
		return ErrUserNotFound
	}
	updated := *u
	updated.Status = model.UserStatusDeleted
	updated.UpdatedAt = time.Now()
	s.users.Store(id, &updated)
	return nil
}

func (s *MemoryStore) List(offset, limit int) ([]*model.User, int64, error) {
	var all []*model.User
	s.users.Range(func(key, value interface{}) bool {
		u := value.(*model.User)
		if u.Status != model.UserStatusDeleted {
			clone := *u
			all = append(all, &clone)
		}
		return true
	})

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
	query = strings.ToLower(query)
	var matched []*model.User
	s.users.Range(func(key, value interface{}) bool {
		u := value.(*model.User)
		if u.Status == model.UserStatusDeleted {
			return true
		}
		if strings.Contains(strings.ToLower(u.Username), query) ||
			strings.Contains(strings.ToLower(u.Email), query) ||
			strings.Contains(strings.ToLower(u.DisplayName), query) {
			clone := *u
			matched = append(matched, &clone)
		}
		return true
	})

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
