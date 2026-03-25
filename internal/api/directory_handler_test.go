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

// ---- directory test setup ----

type dirTestEnv struct {
	server    *httptest.Server
	token     string
	adminID   string
	aliceID   string
	userStore user.Store
	orgStore  directory.OrgStore
	groupStore directory.GroupStore
}

func setupDirTest(t *testing.T) *dirTestEnv {
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

	hash, _ := authSvc.HashPassword("admin123")
	adminID := uuid.New().String()
	userStore.Create(&model.User{
		ID: adminID, Username: "admin", Email: "admin@test.com",
		PasswordHash: hash, Status: model.UserStatusActive,
	})

	hash2, _ := authSvc.HashPassword("alice123")
	aliceID := uuid.New().String()
	userStore.Create(&model.User{
		ID: aliceID, Username: "alice", Email: "alice@test.com",
		PasswordHash: hash2, Status: model.UserStatusActive,
	})

	userSvc := service.NewUserService(userStore, syncStore, authSvc)
	dirSvc := service.NewDirectoryService(orgStore, groupStore, userStore)
	h := api.NewHandler(userSvc, authSvc)
	dh := api.NewDirectoryHandler(dirSvc)
	router := api.SetupRouter(h, dh, authSvc)
	srv := httptest.NewServer(router)
	t.Cleanup(srv.Close)

	// 登录获取 token
	body, _ := json.Marshal(map[string]string{"username": "admin", "password": "admin123"})
	resp, _ := http.Post(srv.URL+"/api/v1/auth/login", "application/json", bytes.NewReader(body))
	var loginResp model.LoginResponse
	json.NewDecoder(resp.Body).Decode(&loginResp)
	resp.Body.Close()

	return &dirTestEnv{
		server: srv, token: loginResp.Token,
		adminID: adminID, aliceID: aliceID,
		userStore: userStore, orgStore: orgStore, groupStore: groupStore,
	}
}

func (e *dirTestEnv) do(method, path string, body interface{}) *http.Response {
	var reqBody *bytes.Reader
	if body != nil {
		data, _ := json.Marshal(body)
		reqBody = bytes.NewReader(data)
	} else {
		reqBody = bytes.NewReader(nil)
	}
	req, _ := http.NewRequest(method, e.server.URL+path, reqBody)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+e.token)
	resp, _ := http.DefaultClient.Do(req)
	return resp
}

func decodeBody(resp *http.Response, v interface{}) {
	defer resp.Body.Close()
	json.NewDecoder(resp.Body).Decode(v)
}

// ========== 组织 CRUD 测试 ==========

func TestOrg_CreateAndGet(t *testing.T) {
	env := setupDirTest(t)

	resp := env.do("POST", "/api/v1/orgs", map[string]string{"name": "Engineering"})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201, got %d", resp.StatusCode)
	}
	var org model.Organization
	decodeBody(resp, &org)
	if org.ID == "" || org.Name != "Engineering" {
		t.Errorf("unexpected org: %+v", org)
	}
	if org.Path == "" {
		t.Error("path should be set")
	}

	// GET
	resp2 := env.do("GET", "/api/v1/orgs/"+org.ID, nil)
	if resp2.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp2.StatusCode)
	}
	var got model.Organization
	decodeBody(resp2, &got)
	if got.Name != "Engineering" {
		t.Errorf("expected Engineering, got %s", got.Name)
	}
}

func TestOrg_CreateHierarchy(t *testing.T) {
	env := setupDirTest(t)

	// 创建父组织
	resp := env.do("POST", "/api/v1/orgs", map[string]string{"name": "Company"})
	var parent model.Organization
	decodeBody(resp, &parent)

	// 创建子组织
	resp2 := env.do("POST", "/api/v1/orgs", map[string]interface{}{
		"name":      "Engineering",
		"parent_id": parent.ID,
	})
	if resp2.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201, got %d", resp2.StatusCode)
	}
	var child model.Organization
	decodeBody(resp2, &child)
	if child.ParentID != parent.ID {
		t.Errorf("expected parent_id=%s, got %s", parent.ID, child.ParentID)
	}

	// 列出子组织
	resp3 := env.do("GET", "/api/v1/orgs/"+parent.ID+"/children", nil)
	var result map[string]interface{}
	decodeBody(resp3, &result)
	if result["total"].(float64) != 1 {
		t.Errorf("expected 1 child, got %v", result["total"])
	}
}

func TestOrg_Update(t *testing.T) {
	env := setupDirTest(t)

	resp := env.do("POST", "/api/v1/orgs", map[string]string{"name": "OldName"})
	var org model.Organization
	decodeBody(resp, &org)

	resp2 := env.do("PUT", "/api/v1/orgs/"+org.ID, map[string]string{"name": "NewName"})
	if resp2.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp2.StatusCode)
	}
	var updated model.Organization
	decodeBody(resp2, &updated)
	if updated.Name != "NewName" {
		t.Errorf("expected NewName, got %s", updated.Name)
	}
}

func TestOrg_Delete(t *testing.T) {
	env := setupDirTest(t)

	resp := env.do("POST", "/api/v1/orgs", map[string]string{"name": "ToDelete"})
	var org model.Organization
	decodeBody(resp, &org)

	resp2 := env.do("DELETE", "/api/v1/orgs/"+org.ID, nil)
	if resp2.StatusCode != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", resp2.StatusCode)
	}
	resp2.Body.Close()

	resp3 := env.do("GET", "/api/v1/orgs/"+org.ID, nil)
	if resp3.StatusCode != http.StatusNotFound {
		t.Errorf("expected 404 after delete, got %d", resp3.StatusCode)
	}
	resp3.Body.Close()
}

func TestOrg_ListOrgs(t *testing.T) {
	env := setupDirTest(t)

	for _, name := range []string{"Sales", "Marketing", "Legal"} {
		env.do("POST", "/api/v1/orgs", map[string]string{"name": name}).Body.Close()
	}

	resp := env.do("GET", "/api/v1/orgs", nil)
	var result map[string]interface{}
	decodeBody(resp, &result)
	if result["total"].(float64) < 3 {
		t.Errorf("expected at least 3 orgs, got %v", result["total"])
	}
}

func TestOrg_NotFound(t *testing.T) {
	env := setupDirTest(t)

	resp := env.do("GET", "/api/v1/orgs/non-existent", nil)
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("expected 404, got %d", resp.StatusCode)
	}
	resp.Body.Close()
}

// ========== 组织成员管理测试 ==========

func TestOrg_MemberManagement(t *testing.T) {
	env := setupDirTest(t)

	// 创建组织
	resp := env.do("POST", "/api/v1/orgs", map[string]string{"name": "DevTeam"})
	var org model.Organization
	decodeBody(resp, &org)

	t.Run("add member", func(t *testing.T) {
		resp := env.do("POST", "/api/v1/orgs/"+org.ID+"/members", map[string]interface{}{
			"user_id": env.aliceID,
			"role":    "member",
		})
		if resp.StatusCode != http.StatusCreated {
			t.Fatalf("expected 201, got %d", resp.StatusCode)
		}
		resp.Body.Close()
	})

	t.Run("add duplicate member", func(t *testing.T) {
		resp := env.do("POST", "/api/v1/orgs/"+org.ID+"/members", map[string]interface{}{
			"user_id": env.aliceID,
			"role":    "member",
		})
		if resp.StatusCode != http.StatusConflict {
			t.Errorf("expected 409 conflict, got %d", resp.StatusCode)
		}
		resp.Body.Close()
	})

	t.Run("list members with user info", func(t *testing.T) {
		resp := env.do("GET", "/api/v1/orgs/"+org.ID+"/members", nil)
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("expected 200, got %d", resp.StatusCode)
		}
		var result map[string]interface{}
		decodeBody(resp, &result)
		if result["total"].(float64) != 1 {
			t.Errorf("expected 1 member, got %v", result["total"])
		}
		items := result["items"].([]interface{})
		member := items[0].(map[string]interface{})
		if member["username"] != "alice" {
			t.Errorf("expected username=alice, got %v", member["username"])
		}
	})

	t.Run("get user orgs", func(t *testing.T) {
		resp := env.do("GET", "/api/v1/users/"+env.aliceID+"/orgs", nil)
		var result map[string]interface{}
		decodeBody(resp, &result)
		if result["total"].(float64) != 1 {
			t.Errorf("expected 1 org for alice, got %v", result["total"])
		}
	})

	t.Run("remove member", func(t *testing.T) {
		resp := env.do("DELETE", "/api/v1/orgs/"+org.ID+"/members/"+env.aliceID, nil)
		if resp.StatusCode != http.StatusNoContent {
			t.Fatalf("expected 204, got %d", resp.StatusCode)
		}
		resp.Body.Close()

		resp2 := env.do("GET", "/api/v1/orgs/"+org.ID+"/members", nil)
		var result map[string]interface{}
		decodeBody(resp2, &result)
		if result["total"].(float64) != 0 {
			t.Errorf("expected 0 members after removal, got %v", result["total"])
		}
	})

	t.Run("add non-existent user", func(t *testing.T) {
		resp := env.do("POST", "/api/v1/orgs/"+org.ID+"/members", map[string]interface{}{
			"user_id": "non-existent-user",
		})
		if resp.StatusCode != http.StatusNotFound {
			t.Errorf("expected 404 for non-existent user, got %d", resp.StatusCode)
		}
		resp.Body.Close()
	})
}

// ========== 群组 CRUD 测试 ==========

func TestGroup_CreateAndGet(t *testing.T) {
	env := setupDirTest(t)

	resp := env.do("POST", "/api/v1/groups", map[string]string{
		"name":        "admins",
		"description": "System administrators",
		"type":        "security",
	})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201, got %d", resp.StatusCode)
	}
	var group model.Group
	decodeBody(resp, &group)
	if group.ID == "" || group.Name != "admins" {
		t.Errorf("unexpected group: %+v", group)
	}

	resp2 := env.do("GET", "/api/v1/groups/"+group.ID, nil)
	if resp2.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp2.StatusCode)
	}
	resp2.Body.Close()
}

func TestGroup_Update(t *testing.T) {
	env := setupDirTest(t)

	resp := env.do("POST", "/api/v1/groups", map[string]string{"name": "ops"})
	var group model.Group
	decodeBody(resp, &group)

	resp2 := env.do("PUT", "/api/v1/groups/"+group.ID, map[string]string{
		"description": "Operations team",
	})
	if resp2.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp2.StatusCode)
	}
	var updated model.Group
	decodeBody(resp2, &updated)
	if updated.Description != "Operations team" {
		t.Errorf("expected updated description, got %s", updated.Description)
	}
}

func TestGroup_Delete(t *testing.T) {
	env := setupDirTest(t)

	resp := env.do("POST", "/api/v1/groups", map[string]string{"name": "temp"})
	var group model.Group
	decodeBody(resp, &group)

	resp2 := env.do("DELETE", "/api/v1/groups/"+group.ID, nil)
	if resp2.StatusCode != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", resp2.StatusCode)
	}
	resp2.Body.Close()

	resp3 := env.do("GET", "/api/v1/groups/"+group.ID, nil)
	if resp3.StatusCode != http.StatusNotFound {
		t.Errorf("expected 404, got %d", resp3.StatusCode)
	}
	resp3.Body.Close()
}

func TestGroup_ListGroups(t *testing.T) {
	env := setupDirTest(t)

	for _, name := range []string{"group1", "group2", "group3"} {
		env.do("POST", "/api/v1/groups", map[string]string{"name": name}).Body.Close()
	}

	resp := env.do("GET", "/api/v1/groups", nil)
	var result map[string]interface{}
	decodeBody(resp, &result)
	if result["total"].(float64) < 3 {
		t.Errorf("expected at least 3 groups, got %v", result["total"])
	}
}

// ========== 群组成员管理测试 ==========

func TestGroup_MemberManagement(t *testing.T) {
	env := setupDirTest(t)

	resp := env.do("POST", "/api/v1/groups", map[string]string{"name": "backend"})
	var group model.Group
	decodeBody(resp, &group)

	t.Run("add member", func(t *testing.T) {
		resp := env.do("POST", "/api/v1/groups/"+group.ID+"/members", map[string]interface{}{
			"user_id": env.aliceID,
			"role":    "member",
		})
		if resp.StatusCode != http.StatusCreated {
			t.Fatalf("expected 201, got %d", resp.StatusCode)
		}
		resp.Body.Close()
	})

	t.Run("add duplicate member", func(t *testing.T) {
		resp := env.do("POST", "/api/v1/groups/"+group.ID+"/members", map[string]interface{}{
			"user_id": env.aliceID,
		})
		if resp.StatusCode != http.StatusConflict {
			t.Errorf("expected 409, got %d", resp.StatusCode)
		}
		resp.Body.Close()
	})

	t.Run("list members with user info", func(t *testing.T) {
		resp := env.do("GET", "/api/v1/groups/"+group.ID+"/members", nil)
		var result map[string]interface{}
		decodeBody(resp, &result)
		if result["total"].(float64) != 1 {
			t.Errorf("expected 1 member, got %v", result["total"])
		}
		items := result["items"].([]interface{})
		m := items[0].(map[string]interface{})
		if m["username"] != "alice" {
			t.Errorf("expected username=alice, got %v", m["username"])
		}
	})

	t.Run("get user groups", func(t *testing.T) {
		resp := env.do("GET", "/api/v1/users/"+env.aliceID+"/groups", nil)
		var result map[string]interface{}
		decodeBody(resp, &result)
		if result["total"].(float64) != 1 {
			t.Errorf("expected 1 group for alice, got %v", result["total"])
		}
	})

	t.Run("remove member", func(t *testing.T) {
		resp := env.do("DELETE", "/api/v1/groups/"+group.ID+"/members/"+env.aliceID, nil)
		if resp.StatusCode != http.StatusNoContent {
			t.Fatalf("expected 204, got %d", resp.StatusCode)
		}
		resp.Body.Close()

		resp2 := env.do("GET", "/api/v1/groups/"+group.ID+"/members", nil)
		var result map[string]interface{}
		decodeBody(resp2, &result)
		if result["total"].(float64) != 0 {
			t.Errorf("expected 0 members, got %v", result["total"])
		}
	})

	t.Run("add admin member", func(t *testing.T) {
		resp := env.do("POST", "/api/v1/groups/"+group.ID+"/members", map[string]interface{}{
			"user_id": env.adminID,
			"role":    "admin",
		})
		if resp.StatusCode != http.StatusCreated {
			t.Fatalf("expected 201, got %d", resp.StatusCode)
		}
		resp.Body.Close()
	})
}

func TestGroup_UserInMultipleGroups(t *testing.T) {
	env := setupDirTest(t)

	// 创建两个群组，把 alice 加入两个群组
	var g1, g2 model.Group
	resp := env.do("POST", "/api/v1/groups", map[string]string{"name": "frontend"})
	decodeBody(resp, &g1)
	resp = env.do("POST", "/api/v1/groups", map[string]string{"name": "fullstack"})
	decodeBody(resp, &g2)

	env.do("POST", "/api/v1/groups/"+g1.ID+"/members", map[string]interface{}{"user_id": env.aliceID}).Body.Close()
	env.do("POST", "/api/v1/groups/"+g2.ID+"/members", map[string]interface{}{"user_id": env.aliceID}).Body.Close()

	resp = env.do("GET", "/api/v1/users/"+env.aliceID+"/groups", nil)
	var result map[string]interface{}
	decodeBody(resp, &result)
	if result["total"].(float64) != 2 {
		t.Errorf("expected alice in 2 groups, got %v", result["total"])
	}
}
