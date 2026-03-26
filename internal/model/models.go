package model

import "time"

// UserStatus 用户状态
type UserStatus string

const (
	UserStatusActive   UserStatus = "active"
	UserStatusDisabled UserStatus = "disabled"
	UserStatusDeleted  UserStatus = "deleted"
)

// User 用户实体
type User struct {
	ID           string     `json:"id" gorm:"primaryKey"`
	Username     string     `json:"username" gorm:"uniqueIndex;not null"`
	Email        string     `json:"email" gorm:"uniqueIndex;not null"`
	PasswordHash string     `json:"-" gorm:"not null"`
	DisplayName  string     `json:"display_name"`
	Status       UserStatus `json:"status" gorm:"default:active"`
	IsOnline     bool       `json:"is_online" gorm:"default:false"`
	ExternalID   string     `json:"external_id,omitempty"`
	SourceSystem string     `json:"source_system,omitempty"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
}

// Organization 组织实体
type Organization struct {
	ID           string    `json:"id" gorm:"primaryKey"`
	Name         string    `json:"name" gorm:"not null"`
	ParentID     string    `json:"parent_id,omitempty"`
	Path         string    `json:"path"`
	ExternalID   string    `json:"external_id,omitempty"`
	SourceSystem string    `json:"source_system,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// Group 群组实体
type Group struct {
	ID          string    `json:"id" gorm:"primaryKey"`
	Name        string    `json:"name" gorm:"not null"`
	Description string    `json:"description"`
	Type        string    `json:"type"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// UserOrgRelation 用户-组织关系
type UserOrgRelation struct {
	UserID    string `json:"user_id" gorm:"primaryKey"`
	OrgID     string `json:"org_id" gorm:"primaryKey"`
	Role      string `json:"role"`
	IsPrimary bool   `json:"is_primary"`
}

// SyncJobStatus 同步任务状态
type SyncJobStatus string

const (
	SyncJobStatusPending  SyncJobStatus = "pending"
	SyncJobStatusRunning  SyncJobStatus = "running"
	SyncJobStatusDone     SyncJobStatus = "done"
	SyncJobStatusFailed   SyncJobStatus = "failed"
	SyncJobStatusCanceled SyncJobStatus = "canceled"
)

// SyncType 同步类型
type SyncType string

const (
	SyncTypeFull        SyncType = "full"
	SyncTypeIncremental SyncType = "incremental"
)

// SyncJob 同步任务
type SyncJob struct {
	ID           string        `json:"id" gorm:"primaryKey"`
	SourceSystem string        `json:"source_system"`
	SyncType     SyncType      `json:"sync_type"`
	Status       SyncJobStatus `json:"status"`
	StartedAt    *time.Time    `json:"started_at,omitempty"`
	FinishedAt   *time.Time    `json:"finished_at,omitempty"`
	ErrorMessage string        `json:"error_message,omitempty"`
	CreatedAt    time.Time     `json:"created_at"`
	UpdatedAt    time.Time     `json:"updated_at"`
}

// SyncLog 同步日志
type SyncLog struct {
	ID         string    `json:"id" gorm:"primaryKey"`
	JobID      string    `json:"job_id"`
	Operation  string    `json:"operation"`
	EntityType string    `json:"entity_type"`
	EntityID   string    `json:"entity_id"`
	BeforeData string    `json:"before_data,omitempty"`
	AfterData  string    `json:"after_data,omitempty"`
	Status     string    `json:"status"`
	CreatedAt  time.Time `json:"created_at"`
}

// CreateUserRequest 创建用户请求
type CreateUserRequest struct {
	Username    string `json:"username" binding:"required,min=3,max=64"`
	Email       string `json:"email" binding:"required,email"`
	Password    string `json:"password" binding:"required,min=8"`
	DisplayName string `json:"display_name"`
}

// UpdateUserRequest 更新用户请求
type UpdateUserRequest struct {
	Email       string     `json:"email,omitempty" binding:"omitempty,email"`
	DisplayName string     `json:"display_name,omitempty"`
	Status      UserStatus `json:"status,omitempty"`
}

// LoginRequest 登录请求
type LoginRequest struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
}

// LoginResponse 登录响应
type LoginResponse struct {
	Token   string `json:"token"`
	Expires int64  `json:"expires"`
	User    *User  `json:"user"`
}

// CreateSyncJobRequest 创建同步任务请求
type CreateSyncJobRequest struct {
	SourceSystem string   `json:"source_system" binding:"required"`
	SyncType     SyncType `json:"sync_type" binding:"required"`
}

// GroupMember 群组成员关系
type GroupMember struct {
	GroupID   string    `json:"group_id" gorm:"primaryKey"`
	UserID    string    `json:"user_id" gorm:"primaryKey"`
	Role      string    `json:"role"`
	JoinedAt  time.Time `json:"joined_at"`
}

// CreateOrgRequest 创建组织请求
type CreateOrgRequest struct {
	Name         string `json:"name" binding:"required,min=1,max=128"`
	ParentID     string `json:"parent_id,omitempty"`
	ExternalID   string `json:"external_id,omitempty"`
	SourceSystem string `json:"source_system,omitempty"`
}

// UpdateOrgRequest 更新组织请求
type UpdateOrgRequest struct {
	Name     string `json:"name,omitempty" binding:"omitempty,min=1,max=128"`
	ParentID string `json:"parent_id,omitempty"`
}

// AddOrgMemberRequest 添加组织成员请求
type AddOrgMemberRequest struct {
	UserID    string `json:"user_id" binding:"required"`
	Role      string `json:"role,omitempty"`
	IsPrimary bool   `json:"is_primary"`
}

// OrgMemberDetail 组织成员详情（含用户信息）
type OrgMemberDetail struct {
	UserOrgRelation
	Username    string `json:"username"`
	Email       string `json:"email"`
	DisplayName string `json:"display_name"`
	Status      UserStatus `json:"status"`
}

// CreateGroupRequest 创建群组请求
type CreateGroupRequest struct {
	Name        string `json:"name" binding:"required,min=1,max=128"`
	Description string `json:"description,omitempty"`
	Type        string `json:"type,omitempty"`
}

// UpdateGroupRequest 更新群组请求
type UpdateGroupRequest struct {
	Name        string `json:"name,omitempty" binding:"omitempty,min=1,max=128"`
	Description string `json:"description,omitempty"`
	Type        string `json:"type,omitempty"`
}

// AddGroupMemberRequest 添加群组成员请求
type AddGroupMemberRequest struct {
	UserID string `json:"user_id" binding:"required"`
	Role   string `json:"role,omitempty"`
}

// GroupMemberDetail 群组成员详情（含用户信息）
type GroupMemberDetail struct {
	GroupMember
	Username    string     `json:"username"`
	Email       string     `json:"email"`
	DisplayName string     `json:"display_name"`
	Status      UserStatus `json:"status"`
}
