package api_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/idp-service/internal/api"
	"github.com/idp-service/internal/auth"
	"github.com/idp-service/internal/directory"
	"github.com/idp-service/internal/model"
	"github.com/idp-service/internal/service"
	"github.com/idp-service/internal/user"
)

// ---- test setup ----

func newTestHandler(t *testing.T) (*api.Handler, *api.Handler, *auth.Service, user.Store) {
	t.Helper()
	userStore := user.NewMemoryStore()
	syncStore := user.NewMemorySyncJobStore()
	authSvc := auth.NewService(auth.Config{
		JWTSecret:   "test-secret",
		TokenExpiry: time.Hour,
		BcryptCost:  4,
	})
	userSvc := service.NewUserService(userStore, syncStore, authSvc)
	h := api.NewHandler(userSvc, authSvc)
	return h, h, authSvc, userStore
}

func setupRouter(t *testing.T) (*httptest.Server, *auth.Service, user.Store) {
	t.Helper()
	userStore := user.NewMemoryStore()
	syncStore := user.NewMemorySyncJobStore()
	orgStore := directory.NewMemoryOrgStore()
	groupStore := directory.NewMemoryGroupStore()
	authSvc := auth.NewService(auth.Config{
		JWTSecret:   "test-secret",
		TokenExpiry: time.Hour,
		BcryptCost:  4,
	})
	userSvc := service.NewUserService(userStore, syncStore, authSvc)
	dirSvc := service.NewDirectoryService(orgStore, groupStore, userStore)
	h := api.NewHandler(userSvc, authSvc)
	dh := api.NewDirectoryHandler(dirSvc)
	router := api.SetupRouter(h, dh, authSvc)
	return httptest.NewServer(router), authSvc, userStore
}

func getAuthToken(t *testing.T, server *httptest.Server, username, password string) string {
	t.Helper()
	body, _ := json.Marshal(map[string]string{
		"username": username,
		"password": password,
	})
	resp, err := http.Post(server.URL+"/api/v1/auth/login", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("login request failed: %v", err)
	}
	defer resp.Body.Close()

	var result model.LoginResponse
	json.NewDecoder(resp.Body).Decode(&result)
	return result.Token
}

func createTestUser(t *testing.T, store user.Store, authSvc *auth.Service) *model.User {
	t.Helper()
	hash, _ := authSvc.HashPassword("testpassword")
	u := &model.User{
		ID:           uuid.New().String(),
		Username:     "testuser",
		Email:        "testuser@example.com",
		PasswordHash: hash,
		DisplayName:  "Test User",
		Status:       model.UserStatusActive,
	}
	store.Create(u)
	return u
}

// ---- tests ----

func TestLogin_Success(t *testing.T) {
	server, authSvc, userStore := setupRouter(t)
	defer server.Close()

	createTestUser(t, userStore, authSvc)

	body, _ := json.Marshal(map[string]string{
		"username": "testuser",
		"password": "testpassword",
	})
	resp, err := http.Post(server.URL+"/api/v1/auth/login", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}

	var result model.LoginResponse
	json.NewDecoder(resp.Body).Decode(&result)
	if result.Token == "" {
		t.Error("expected non-empty token")
	}
}

func TestLogin_WrongPassword(t *testing.T) {
	server, authSvc, userStore := setupRouter(t)
	defer server.Close()

	createTestUser(t, userStore, authSvc)

	body, _ := json.Marshal(map[string]string{
		"username": "testuser",
		"password": "wrongpassword",
	})
	resp, _ := http.Post(server.URL+"/api/v1/auth/login", "application/json", bytes.NewReader(body))
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", resp.StatusCode)
	}
}

func TestLogin_UserNotFound(t *testing.T) {
	server, _, _ := setupRouter(t)
	defer server.Close()

	body, _ := json.Marshal(map[string]string{
		"username": "nonexistent",
		"password": "password",
	})
	resp, _ := http.Post(server.URL+"/api/v1/auth/login", "application/json", bytes.NewReader(body))
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", resp.StatusCode)
	}
}

func TestCreateUser_Success(t *testing.T) {
	server, authSvc, userStore := setupRouter(t)
	defer server.Close()

	createTestUser(t, userStore, authSvc)
	token := getAuthToken(t, server, "testuser", "testpassword")

	body, _ := json.Marshal(map[string]string{
		"username":     "newuser",
		"email":        "newuser@example.com",
		"password":     "newpassword123",
		"display_name": "New User",
	})

	req, _ := http.NewRequest("POST", server.URL+"/api/v1/users", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		t.Errorf("expected 201, got %d", resp.StatusCode)
	}

	var u model.User
	json.NewDecoder(resp.Body).Decode(&u)
	if u.Username != "newuser" {
		t.Errorf("expected username=newuser, got %s", u.Username)
	}
	if u.PasswordHash != "" {
		t.Error("password_hash should not be exposed in response")
	}
}

func TestCreateUser_DuplicateUsername(t *testing.T) {
	server, authSvc, userStore := setupRouter(t)
	defer server.Close()

	createTestUser(t, userStore, authSvc)
	token := getAuthToken(t, server, "testuser", "testpassword")

	// 尝试创建同名用户
	body, _ := json.Marshal(map[string]string{
		"username": "testuser",
		"email":    "another@example.com",
		"password": "password123",
	})

	req, _ := http.NewRequest("POST", server.URL+"/api/v1/users", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)

	client := &http.Client{}
	resp, _ := client.Do(req)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusConflict {
		t.Errorf("expected 409 conflict, got %d", resp.StatusCode)
	}
}

func TestGetUser_Success(t *testing.T) {
	server, authSvc, userStore := setupRouter(t)
	defer server.Close()

	u := createTestUser(t, userStore, authSvc)
	token := getAuthToken(t, server, "testuser", "testpassword")

	req, _ := http.NewRequest("GET", server.URL+"/api/v1/users/"+u.ID, nil)
	req.Header.Set("Authorization", "Bearer "+token)

	client := &http.Client{}
	resp, _ := client.Do(req)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}

	var got model.User
	json.NewDecoder(resp.Body).Decode(&got)
	if got.ID != u.ID {
		t.Errorf("expected ID=%s, got %s", u.ID, got.ID)
	}
}

func TestGetUser_NotFound(t *testing.T) {
	server, authSvc, userStore := setupRouter(t)
	defer server.Close()

	createTestUser(t, userStore, authSvc)
	token := getAuthToken(t, server, "testuser", "testpassword")

	req, _ := http.NewRequest("GET", server.URL+"/api/v1/users/non-existent-id", nil)
	req.Header.Set("Authorization", "Bearer "+token)

	client := &http.Client{}
	resp, _ := client.Do(req)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("expected 404, got %d", resp.StatusCode)
	}
}

func TestUpdateUser(t *testing.T) {
	server, authSvc, userStore := setupRouter(t)
	defer server.Close()

	u := createTestUser(t, userStore, authSvc)
	token := getAuthToken(t, server, "testuser", "testpassword")

	body, _ := json.Marshal(map[string]string{
		"display_name": "Updated Name",
	})

	req, _ := http.NewRequest("PUT", server.URL+"/api/v1/users/"+u.ID, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)

	client := &http.Client{}
	resp, _ := client.Do(req)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}

	var updated model.User
	json.NewDecoder(resp.Body).Decode(&updated)
	if updated.DisplayName != "Updated Name" {
		t.Errorf("expected display_name=Updated Name, got %s", updated.DisplayName)
	}
}

func TestDeleteUser(t *testing.T) {
	server, authSvc, userStore := setupRouter(t)
	defer server.Close()

	u := createTestUser(t, userStore, authSvc)
	token := getAuthToken(t, server, "testuser", "testpassword")

	req, _ := http.NewRequest("DELETE", server.URL+"/api/v1/users/"+u.ID, nil)
	req.Header.Set("Authorization", "Bearer "+token)

	client := &http.Client{}
	resp, _ := client.Do(req)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent {
		t.Errorf("expected 204, got %d", resp.StatusCode)
	}
}

func TestListUsers(t *testing.T) {
	server, authSvc, userStore := setupRouter(t)
	defer server.Close()

	createTestUser(t, userStore, authSvc)
	token := getAuthToken(t, server, "testuser", "testpassword")

	// 创建更多用户
	for i := 0; i < 3; i++ {
		hash, _ := authSvc.HashPassword("password123")
		userStore.Create(&model.User{
			ID:           uuid.New().String(),
			Username:     "extra_user_" + string(rune('a'+i)),
			Email:        "extra" + string(rune('a'+i)) + "@example.com",
			PasswordHash: hash,
			Status:       model.UserStatusActive,
		})
	}

	req, _ := http.NewRequest("GET", server.URL+"/api/v1/users", nil)
	req.Header.Set("Authorization", "Bearer "+token)

	client := &http.Client{}
	resp, _ := client.Do(req)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}

	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)
	total := result["total"].(float64)
	if total < 4 {
		t.Errorf("expected at least 4 users, got %v", total)
	}
}

func TestJWTMiddleware_MissingToken(t *testing.T) {
	server, _, _ := setupRouter(t)
	defer server.Close()

	req, _ := http.NewRequest("GET", server.URL+"/api/v1/users", nil)
	client := &http.Client{}
	resp, _ := client.Do(req)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("expected 401 for missing token, got %d", resp.StatusCode)
	}
}

func TestJWTMiddleware_InvalidToken(t *testing.T) {
	server, _, _ := setupRouter(t)
	defer server.Close()

	req, _ := http.NewRequest("GET", server.URL+"/api/v1/users", nil)
	req.Header.Set("Authorization", "Bearer invalid.token.here")

	client := &http.Client{}
	resp, _ := client.Do(req)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("expected 401 for invalid token, got %d", resp.StatusCode)
	}
}

func TestCreateSyncJob(t *testing.T) {
	server, authSvc, userStore := setupRouter(t)
	defer server.Close()

	createTestUser(t, userStore, authSvc)
	token := getAuthToken(t, server, "testuser", "testpassword")

	body, _ := json.Marshal(map[string]string{
		"source_system": "ldap",
		"sync_type":     "incremental",
	})

	req, _ := http.NewRequest("POST", server.URL+"/api/v1/sync/jobs", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)

	client := &http.Client{}
	resp, _ := client.Do(req)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		t.Errorf("expected 201, got %d", resp.StatusCode)
	}

	var job model.SyncJob
	json.NewDecoder(resp.Body).Decode(&job)
	if job.ID == "" {
		t.Error("expected non-empty job ID")
	}
}

func TestHealthCheck(t *testing.T) {
	server, _, _ := setupRouter(t)
	defer server.Close()

	resp, _ := http.Get(server.URL + "/health")
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
}
