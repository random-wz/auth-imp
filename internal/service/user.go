package service

import (
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/idp-service/internal/auth"
	"github.com/idp-service/internal/model"
	"github.com/idp-service/internal/user"
)

// 业务层错误
var (
	ErrInvalidCredentials = errors.New("invalid credentials")
	ErrUserDisabled       = errors.New("user is disabled")
)

// UserService 用户管理业务逻辑
type UserService struct {
	userStore user.Store
	syncStore user.SyncJobStore
	authSvc   *auth.Service
}

func NewUserService(userStore user.Store, syncStore user.SyncJobStore, authSvc *auth.Service) *UserService {
	return &UserService{
		userStore: userStore,
		syncStore: syncStore,
		authSvc:   authSvc,
	}
}

func (s *UserService) CreateUser(req model.CreateUserRequest) (*model.User, error) {
	hash, err := s.authSvc.HashPassword(req.Password)
	if err != nil {
		return nil, err
	}
	u := &model.User{
		ID:           uuid.New().String(),
		Username:     req.Username,
		Email:        req.Email,
		PasswordHash: hash,
		DisplayName:  req.DisplayName,
		Status:       model.UserStatusActive,
	}
	if err := s.userStore.Create(u); err != nil {
		return nil, err
	}
	return u, nil
}

func (s *UserService) GetUser(id string) (*model.User, error) {
	return s.userStore.GetByID(id)
}

func (s *UserService) GetUserByUsername(username string) (*model.User, error) {
	return s.userStore.GetByUsername(username)
}

func (s *UserService) UpdateUser(id string, req model.UpdateUserRequest) (*model.User, error) {
	u := &model.User{
		ID:          id,
		Email:       req.Email,
		DisplayName: req.DisplayName,
		Status:      req.Status,
	}
	if err := s.userStore.Update(u); err != nil {
		return nil, err
	}
	return s.userStore.GetByID(id)
}

func (s *UserService) DeleteUser(id string) error {
	return s.userStore.Delete(id)
}

func (s *UserService) ListUsers(q string, offset, limit int) ([]*model.User, int64, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	if q != "" {
		return s.userStore.Search(q, offset, limit)
	}
	return s.userStore.List(offset, limit)
}

// VerifyCredentials 验证用户凭据（不生成 Token，供 UDS auth 动作使用）
func (s *UserService) VerifyCredentials(username, password string) (*model.User, error) {
	u, err := s.userStore.GetByUsername(username)
	if err != nil {
		return nil, ErrInvalidCredentials
	}
	if u.Status != model.UserStatusActive {
		return nil, ErrUserDisabled
	}
	if !s.authSvc.VerifyPassword(u.PasswordHash, password) {
		return nil, ErrInvalidCredentials
	}
	return u, nil
}

// Login 登录并生成 JWT Token（供 REST API 使用）
func (s *UserService) Login(username, password string) (*model.LoginResponse, error) {
	u, err := s.VerifyCredentials(username, password)
	if err != nil {
		return nil, err
	}
	token, expires, err := s.authSvc.GenerateToken(u.ID, u.Username)
	if err != nil {
		return nil, err
	}
	return &model.LoginResponse{
		Token:   token,
		Expires: expires,
		User:    u,
	}, nil
}

func (s *UserService) CreateSyncJob(req model.CreateSyncJobRequest) (*model.SyncJob, error) {
	now := time.Now()
	job := &model.SyncJob{
		ID:           uuid.New().String(),
		SourceSystem: req.SourceSystem,
		SyncType:     req.SyncType,
		Status:       model.SyncJobStatusPending,
		StartedAt:    &now,
	}
	if err := s.syncStore.Create(job); err != nil {
		return nil, err
	}
	return job, nil
}

func (s *UserService) GetSyncJob(id string) (*model.SyncJob, error) {
	return s.syncStore.GetByID(id)
}
