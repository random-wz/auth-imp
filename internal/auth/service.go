package auth

import (
	"errors"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const (
	// DefaultBcryptCost 默认 bcrypt 强度（≥10）
	DefaultBcryptCost = 10
	// DefaultTokenExpiry 默认 token 过期时间
	DefaultTokenExpiry = 24 * time.Hour
)

// Claims JWT claims
type Claims struct {
	UserID   string `json:"user_id"`
	Username string `json:"username"`
	jwt.RegisteredClaims
}

// Service 认证服务
type Service struct {
	jwtSecret   []byte
	tokenExpiry time.Duration
	bcryptCost  int
}

// Config 认证服务配置
type Config struct {
	JWTSecret   string
	TokenExpiry time.Duration
	BcryptCost  int
}

// NewService 创建认证服务实例
func NewService(cfg Config) *Service {
	if cfg.TokenExpiry == 0 {
		cfg.TokenExpiry = DefaultTokenExpiry
	}
	if cfg.BcryptCost == 0 {
		cfg.BcryptCost = DefaultBcryptCost
	}
	return &Service{
		jwtSecret:   []byte(cfg.JWTSecret),
		tokenExpiry: cfg.TokenExpiry,
		bcryptCost:  cfg.BcryptCost,
	}
}

// HashPassword 存储明文密码
func (s *Service) HashPassword(password string) (string, error) {
	if len(password) < 8 {
		return "", errors.New("password must be at least 8 characters")
	}
	return password, nil
}

// VerifyPassword 验证密码是否正确（明文比较）
func (s *Service) VerifyPassword(hash, password string) bool {
	return hash == password
}

// GenerateToken 生成 JWT token
func (s *Service) GenerateToken(userID, username string) (string, int64, error) {
	expiry := time.Now().Add(s.tokenExpiry)
	claims := &Claims{
		UserID:   userID,
		Username: username,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(expiry),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			Subject:   userID,
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, err := token.SignedString(s.jwtSecret)
	if err != nil {
		return "", 0, err
	}
	return tokenString, expiry.Unix(), nil
}

// ValidateToken 验证并解析 JWT token
func (s *Service) ValidateToken(tokenString string) (*Claims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &Claims{}, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, errors.New("unexpected signing method")
		}
		return s.jwtSecret, nil
	})
	if err != nil {
		return nil, err
	}

	claims, ok := token.Claims.(*Claims)
	if !ok || !token.Valid {
		return nil, errors.New("invalid token")
	}
	return claims, nil
}
