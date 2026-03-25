package user_test

import (
	"fmt"
	"testing"

	"github.com/google/uuid"
	"github.com/idp-service/internal/model"
	"github.com/idp-service/internal/user"
)

func newUser(username, email string) *model.User {
	return &model.User{
		ID:           uuid.New().String(),
		Username:     username,
		Email:        email,
		PasswordHash: "hashed",
		DisplayName:  "Test User",
		Status:       model.UserStatusActive,
	}
}

func TestMemoryStore_Create(t *testing.T) {
	store := user.NewMemoryStore()

	t.Run("create user successfully", func(t *testing.T) {
		u := newUser("alice", "alice@example.com")
		if err := store.Create(u); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("duplicate username", func(t *testing.T) {
		u1 := newUser("bob", "bob@example.com")
		u2 := &model.User{
			ID:           uuid.New().String(),
			Username:     "bob",
			Email:        "bob2@example.com",
			PasswordHash: "hashed",
		}
		store.Create(u1)
		err := store.Create(u2)
		if err != user.ErrUserAlreadyExists {
			t.Errorf("expected ErrUserAlreadyExists, got %v", err)
		}
	})

	t.Run("duplicate email", func(t *testing.T) {
		u1 := newUser("carol", "carol@example.com")
		u2 := &model.User{
			ID:           uuid.New().String(),
			Username:     "carol2",
			Email:        "carol@example.com",
			PasswordHash: "hashed",
		}
		store.Create(u1)
		err := store.Create(u2)
		if err != user.ErrUserAlreadyExists {
			t.Errorf("expected ErrUserAlreadyExists, got %v", err)
		}
	})
}

func TestMemoryStore_GetByID(t *testing.T) {
	store := user.NewMemoryStore()
	u := newUser("dave", "dave@example.com")
	store.Create(u)

	t.Run("existing user", func(t *testing.T) {
		got, err := store.GetByID(u.ID)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got.ID != u.ID {
			t.Errorf("expected ID=%s, got %s", u.ID, got.ID)
		}
	})

	t.Run("non-existing user", func(t *testing.T) {
		_, err := store.GetByID("non-existent-id")
		if err != user.ErrUserNotFound {
			t.Errorf("expected ErrUserNotFound, got %v", err)
		}
	})
}

func TestMemoryStore_GetByUsername(t *testing.T) {
	store := user.NewMemoryStore()
	u := newUser("eve", "eve@example.com")
	store.Create(u)

	t.Run("existing username", func(t *testing.T) {
		got, err := store.GetByUsername("eve")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got.Username != "eve" {
			t.Errorf("expected username=eve, got %s", got.Username)
		}
	})

	t.Run("non-existing username", func(t *testing.T) {
		_, err := store.GetByUsername("nonexistent")
		if err != user.ErrUserNotFound {
			t.Errorf("expected ErrUserNotFound, got %v", err)
		}
	})
}

func TestMemoryStore_Update(t *testing.T) {
	store := user.NewMemoryStore()
	u := newUser("frank", "frank@example.com")
	store.Create(u)

	t.Run("update display name", func(t *testing.T) {
		updated := &model.User{
			ID:          u.ID,
			DisplayName: "Frank Updated",
		}
		if err := store.Update(updated); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		got, _ := store.GetByID(u.ID)
		if got.DisplayName != "Frank Updated" {
			t.Errorf("expected display name updated, got %s", got.DisplayName)
		}
	})

	t.Run("update status to disabled", func(t *testing.T) {
		updated := &model.User{
			ID:     u.ID,
			Status: model.UserStatusDisabled,
		}
		if err := store.Update(updated); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		got, _ := store.GetByID(u.ID)
		if got.Status != model.UserStatusDisabled {
			t.Errorf("expected status disabled, got %s", got.Status)
		}
	})

	t.Run("non-existing user", func(t *testing.T) {
		err := store.Update(&model.User{ID: "non-existent"})
		if err != user.ErrUserNotFound {
			t.Errorf("expected ErrUserNotFound, got %v", err)
		}
	})
}

func TestMemoryStore_Delete(t *testing.T) {
	store := user.NewMemoryStore()
	u := newUser("grace", "grace@example.com")
	store.Create(u)

	t.Run("delete existing user", func(t *testing.T) {
		if err := store.Delete(u.ID); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		_, err := store.GetByID(u.ID)
		if err != user.ErrUserNotFound {
			t.Error("expected user to be deleted (soft delete)")
		}
	})

	t.Run("delete non-existing user", func(t *testing.T) {
		err := store.Delete("non-existent")
		if err != user.ErrUserNotFound {
			t.Errorf("expected ErrUserNotFound, got %v", err)
		}
	})
}

func TestMemoryStore_List(t *testing.T) {
	store := user.NewMemoryStore()
	for i := 0; i < 5; i++ {
		store.Create(newUser(fmt.Sprintf("user%d", i), fmt.Sprintf("user%d@example.com", i)))
	}

	t.Run("list all", func(t *testing.T) {
		users, total, err := store.List(0, 10)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if total != 5 {
			t.Errorf("expected total=5, got %d", total)
		}
		if len(users) != 5 {
			t.Errorf("expected 5 users, got %d", len(users))
		}
	})

	t.Run("list with offset", func(t *testing.T) {
		users, total, err := store.List(3, 10)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if total != 5 {
			t.Errorf("expected total=5, got %d", total)
		}
		if len(users) != 2 {
			t.Errorf("expected 2 users with offset=3, got %d", len(users))
		}
	})

	t.Run("list with limit", func(t *testing.T) {
		users, _, err := store.List(0, 3)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(users) != 3 {
			t.Errorf("expected 3 users with limit=3, got %d", len(users))
		}
	})
}

func TestMemoryStore_Search(t *testing.T) {
	store := user.NewMemoryStore()
	store.Create(newUser("john_doe", "john@example.com"))
	store.Create(newUser("jane_doe", "jane@example.com"))
	store.Create(newUser("bob_smith", "bob@example.com"))

	t.Run("search by username", func(t *testing.T) {
		users, total, err := store.Search("doe", 0, 10)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if total != 2 {
			t.Errorf("expected 2 results for 'doe', got %d", total)
		}
		_ = users
	})

	t.Run("search by email", func(t *testing.T) {
		users, total, err := store.Search("bob@example", 0, 10)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if total != 1 {
			t.Errorf("expected 1 result for 'bob@example', got %d", total)
		}
		_ = users
	})

	t.Run("no results", func(t *testing.T) {
		_, total, err := store.Search("nonexistent", 0, 10)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if total != 0 {
			t.Errorf("expected 0 results, got %d", total)
		}
	})
}
