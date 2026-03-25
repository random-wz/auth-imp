package auth_test

import (
	"testing"
	"time"

	"github.com/idp-service/internal/auth"
)

func newService() *auth.Service {
	return auth.NewService(auth.Config{
		JWTSecret:   "test-secret-key",
		TokenExpiry: time.Hour,
		BcryptCost:  4, // 测试用最低强度，加快速度
	})
}

func TestHashPassword(t *testing.T) {
	svc := newService()

	t.Run("valid password", func(t *testing.T) {
		hash, err := svc.HashPassword("mypassword")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if hash == "" {
			t.Fatal("expected non-empty hash")
		}
		if hash == "mypassword" {
			t.Fatal("hash should not equal plain password")
		}
	})

	t.Run("password too short", func(t *testing.T) {
		_, err := svc.HashPassword("short")
		if err == nil {
			t.Fatal("expected error for short password")
		}
	})

	t.Run("different passwords produce different hashes", func(t *testing.T) {
		h1, _ := svc.HashPassword("password1!")
		h2, _ := svc.HashPassword("password2!")
		if h1 == h2 {
			t.Fatal("different passwords should produce different hashes")
		}
	})
}

func TestVerifyPassword(t *testing.T) {
	svc := newService()
	hash, _ := svc.HashPassword("correct-password")

	t.Run("correct password", func(t *testing.T) {
		if !svc.VerifyPassword(hash, "correct-password") {
			t.Fatal("expected password to verify correctly")
		}
	})

	t.Run("wrong password", func(t *testing.T) {
		if svc.VerifyPassword(hash, "wrong-password") {
			t.Fatal("expected wrong password to fail")
		}
	})

	t.Run("empty password", func(t *testing.T) {
		if svc.VerifyPassword(hash, "") {
			t.Fatal("expected empty password to fail")
		}
	})
}

func TestGenerateAndValidateToken(t *testing.T) {
	svc := newService()

	t.Run("valid token round-trip", func(t *testing.T) {
		token, expires, err := svc.GenerateToken("user-123", "alice")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if token == "" {
			t.Fatal("expected non-empty token")
		}
		if expires <= time.Now().Unix() {
			t.Fatal("expected future expiry")
		}

		claims, err := svc.ValidateToken(token)
		if err != nil {
			t.Fatalf("unexpected validation error: %v", err)
		}
		if claims.UserID != "user-123" {
			t.Errorf("expected UserID=user-123, got %s", claims.UserID)
		}
		if claims.Username != "alice" {
			t.Errorf("expected Username=alice, got %s", claims.Username)
		}
	})

	t.Run("invalid token", func(t *testing.T) {
		_, err := svc.ValidateToken("invalid.token.here")
		if err == nil {
			t.Fatal("expected error for invalid token")
		}
	})

	t.Run("tampered token", func(t *testing.T) {
		token, _, _ := svc.GenerateToken("user-123", "alice")
		tampered := token[:len(token)-4] + "XXXX"
		_, err := svc.ValidateToken(tampered)
		if err == nil {
			t.Fatal("expected error for tampered token")
		}
	})

	t.Run("token signed with different secret", func(t *testing.T) {
		otherSvc := auth.NewService(auth.Config{
			JWTSecret: "other-secret",
		})
		token, _, _ := otherSvc.GenerateToken("user-123", "alice")
		_, err := svc.ValidateToken(token)
		if err == nil {
			t.Fatal("expected error for token with different secret")
		}
	})
}

func TestTokenExpiry(t *testing.T) {
	svc := auth.NewService(auth.Config{
		JWTSecret:   "test-secret",
		TokenExpiry: -time.Second, // 已过期
		BcryptCost:  4,
	})

	token, _, err := svc.GenerateToken("user-123", "alice")
	if err != nil {
		t.Fatalf("unexpected error generating token: %v", err)
	}

	_, err = svc.ValidateToken(token)
	if err == nil {
		t.Fatal("expected error for expired token")
	}
}
