package user

import (
	"errors"

	"github.com/idp-service/internal/model"
)

// ErrUserNotFound 用户不存在错误
var ErrUserNotFound = errors.New("user not found")

// ErrUserAlreadyExists 用户已存在错误
var ErrUserAlreadyExists = errors.New("user already exists")

// Store 用户存储接口
type Store interface {
	// Create 创建用户
	Create(user *model.User) error
	// GetByID 通过ID获取用户
	GetByID(id string) (*model.User, error)
	// GetByUsername 通过用户名获取用户
	GetByUsername(username string) (*model.User, error)
	// GetByEmail 通过邮箱获取用户
	GetByEmail(email string) (*model.User, error)
	// Update 更新用户
	Update(user *model.User) error
	// Delete 软删除用户
	Delete(id string) error
	// List 分页列出用户
	List(offset, limit int) ([]*model.User, int64, error)
	// Search 搜索用户
	Search(query string, offset, limit int) ([]*model.User, int64, error)
}

// SyncJobStore 同步任务存储接口
type SyncJobStore interface {
	// Create 创建同步任务
	Create(job *model.SyncJob) error
	// GetByID 通过ID获取同步任务
	GetByID(id string) (*model.SyncJob, error)
	// Update 更新同步任务
	Update(job *model.SyncJob) error
	// List 列出同步任务
	List(offset, limit int) ([]*model.SyncJob, int64, error)
}
