package user

import (
	"database/sql"
	"strings"

	"github.com/idp-service/internal/model"
	_ "github.com/lib/pq"
)

// PGStore PostgreSQL 用户存储实现
type PGStore struct {
	db *sql.DB
}

// NewPGStore 创建 PostgreSQL 存储实例
func NewPGStore(db *sql.DB) *PGStore {
	return &PGStore{db: db}
}

func (s *PGStore) Create(user *model.User) error {
	query := `
		INSERT INTO users (id, username, email, password_hash, display_name, status, external_id, source_system)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
	`
	_, err := s.db.Exec(query, user.ID, user.Username, user.Email, user.PasswordHash,
		user.DisplayName, user.Status, user.ExternalID, user.SourceSystem)
	if err != nil {
		if strings.Contains(err.Error(), "duplicate") {
			return ErrUserAlreadyExists
		}
		return err
	}
	return nil
}

func (s *PGStore) GetByID(id string) (*model.User, error) {
	query := `SELECT id, username, email, password_hash, display_name, status, external_id, source_system, created_at, updated_at
		FROM users WHERE id = $1 AND status != 'deleted'`
	user := &model.User{}
	err := s.db.QueryRow(query, id).Scan(&user.ID, &user.Username, &user.Email, &user.PasswordHash,
		&user.DisplayName, &user.Status, &user.ExternalID, &user.SourceSystem, &user.CreatedAt, &user.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, ErrUserNotFound
	}
	return user, err
}

func (s *PGStore) GetByUsername(username string) (*model.User, error) {
	query := `SELECT id, username, email, password_hash, display_name, status, external_id, source_system, created_at, updated_at
		FROM users WHERE username = $1 AND status != 'deleted'`
	user := &model.User{}
	err := s.db.QueryRow(query, username).Scan(&user.ID, &user.Username, &user.Email, &user.PasswordHash,
		&user.DisplayName, &user.Status, &user.ExternalID, &user.SourceSystem, &user.CreatedAt, &user.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, ErrUserNotFound
	}
	return user, err
}

func (s *PGStore) GetByEmail(email string) (*model.User, error) {
	query := `SELECT id, username, email, password_hash, display_name, status, external_id, source_system, created_at, updated_at
		FROM users WHERE email = $1 AND status != 'deleted'`
	user := &model.User{}
	err := s.db.QueryRow(query, email).Scan(&user.ID, &user.Username, &user.Email, &user.PasswordHash,
		&user.DisplayName, &user.Status, &user.ExternalID, &user.SourceSystem, &user.CreatedAt, &user.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, ErrUserNotFound
	}
	return user, err
}

func (s *PGStore) Update(user *model.User) error {
	query := `UPDATE users SET email = COALESCE(NULLIF($2, ''), email),
		display_name = COALESCE(NULLIF($3, ''), display_name),
		status = COALESCE(NULLIF($4, ''), status),
		is_online = $5, updated_at = NOW()
		WHERE id = $1 AND status != 'deleted'`
	result, err := s.db.Exec(query, user.ID, user.Email, user.DisplayName, user.Status, user.IsOnline)
	if err != nil {
		return err
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return ErrUserNotFound
	}
	return nil
}

func (s *PGStore) Delete(id string) error {
	query := `UPDATE users SET status = 'deleted', updated_at = NOW() WHERE id = $1 AND status != 'deleted'`
	result, err := s.db.Exec(query, id)
	if err != nil {
		return err
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return ErrUserNotFound
	}
	return nil
}

func (s *PGStore) List(offset, limit int) ([]*model.User, int64, error) {
	var total int64
	s.db.QueryRow(`SELECT COUNT(*) FROM users WHERE status != 'deleted'`).Scan(&total)

	query := `SELECT id, username, email, password_hash, display_name, status, external_id, source_system, created_at, updated_at
		FROM users WHERE status != 'deleted' ORDER BY created_at DESC LIMIT $1 OFFSET $2`
	rows, err := s.db.Query(query, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var users []*model.User
	for rows.Next() {
		u := &model.User{}
		rows.Scan(&u.ID, &u.Username, &u.Email, &u.PasswordHash, &u.DisplayName, &u.Status,
			&u.ExternalID, &u.SourceSystem, &u.CreatedAt, &u.UpdatedAt)
		users = append(users, u)
	}
	return users, total, nil
}

func (s *PGStore) Search(query string, offset, limit int) ([]*model.User, int64, error) {
	pattern := "%" + strings.ToLower(query) + "%"
	var total int64
	s.db.QueryRow(`SELECT COUNT(*) FROM users WHERE status != 'deleted' AND 
		(LOWER(username) LIKE $1 OR LOWER(email) LIKE $1 OR LOWER(display_name) LIKE $1)`, pattern).Scan(&total)

	sql := `SELECT id, username, email, password_hash, display_name, status, external_id, source_system, created_at, updated_at
		FROM users WHERE status != 'deleted' AND 
		(LOWER(username) LIKE $1 OR LOWER(email) LIKE $1 OR LOWER(display_name) LIKE $1)
		ORDER BY created_at DESC LIMIT $2 OFFSET $3`
	rows, err := s.db.Query(sql, pattern, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var users []*model.User
	for rows.Next() {
		u := &model.User{}
		rows.Scan(&u.ID, &u.Username, &u.Email, &u.PasswordHash, &u.DisplayName, &u.Status,
			&u.ExternalID, &u.SourceSystem, &u.CreatedAt, &u.UpdatedAt)
		users = append(users, u)
	}
	return users, total, nil
}
