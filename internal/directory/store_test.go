package directory_test

import (
	"fmt"
	"testing"

	"github.com/google/uuid"
	"github.com/idp-service/internal/directory"
	"github.com/idp-service/internal/model"
)

// ========== OrgStore 测试 ==========

func newOrg(name, parentID string) *model.Organization {
	return &model.Organization{
		ID:       uuid.New().String(),
		Name:     name,
		ParentID: parentID,
	}
}

func TestOrgStore_Create(t *testing.T) {
	store := directory.NewMemoryOrgStore()

	t.Run("create root org", func(t *testing.T) {
		org := newOrg("Engineering", "")
		if err := store.Create(org); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if org.Path == "" {
			t.Error("path should be set after creation")
		}
	})

	t.Run("create child org", func(t *testing.T) {
		parent := newOrg("Product", "")
		store.Create(parent)

		child := newOrg("Frontend", parent.ID)
		if err := store.Create(child); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if child.Path == "" {
			t.Error("child path should be set")
		}
	})

	t.Run("create child with non-existent parent", func(t *testing.T) {
		child := newOrg("Orphan", "non-existent-id")
		err := store.Create(child)
		if err == nil {
			t.Error("expected error for non-existent parent")
		}
	})

	t.Run("duplicate id", func(t *testing.T) {
		org := newOrg("Sales", "")
		store.Create(org)
		err := store.Create(org)
		if err != directory.ErrOrgAlreadyExists {
			t.Errorf("expected ErrOrgAlreadyExists, got %v", err)
		}
	})
}

func TestOrgStore_GetByID(t *testing.T) {
	store := directory.NewMemoryOrgStore()
	org := newOrg("HR", "")
	store.Create(org)

	t.Run("existing org", func(t *testing.T) {
		got, err := store.GetByID(org.ID)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got.Name != "HR" {
			t.Errorf("expected name=HR, got %s", got.Name)
		}
	})

	t.Run("not found", func(t *testing.T) {
		_, err := store.GetByID("non-existent")
		if err != directory.ErrOrgNotFound {
			t.Errorf("expected ErrOrgNotFound, got %v", err)
		}
	})
}

func TestOrgStore_Update(t *testing.T) {
	store := directory.NewMemoryOrgStore()
	org := newOrg("Finance", "")
	store.Create(org)

	t.Run("update name", func(t *testing.T) {
		err := store.Update(&model.Organization{ID: org.ID, Name: "Finance Dept"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		got, _ := store.GetByID(org.ID)
		if got.Name != "Finance Dept" {
			t.Errorf("expected Finance Dept, got %s", got.Name)
		}
	})

	t.Run("update non-existent", func(t *testing.T) {
		err := store.Update(&model.Organization{ID: "non-existent", Name: "X"})
		if err != directory.ErrOrgNotFound {
			t.Errorf("expected ErrOrgNotFound, got %v", err)
		}
	})
}

func TestOrgStore_Delete(t *testing.T) {
	store := directory.NewMemoryOrgStore()
	org := newOrg("Legal", "")
	store.Create(org)

	t.Run("delete existing", func(t *testing.T) {
		if err := store.Delete(org.ID); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		_, err := store.GetByID(org.ID)
		if err != directory.ErrOrgNotFound {
			t.Error("expected org to be deleted")
		}
	})

	t.Run("delete non-existent", func(t *testing.T) {
		err := store.Delete("non-existent")
		if err != directory.ErrOrgNotFound {
			t.Errorf("expected ErrOrgNotFound, got %v", err)
		}
	})
}

func TestOrgStore_List(t *testing.T) {
	store := directory.NewMemoryOrgStore()
	for i := 0; i < 5; i++ {
		store.Create(newOrg(fmt.Sprintf("Org%d", i), ""))
	}

	orgs, total, err := store.List(0, 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if total != 5 {
		t.Errorf("expected total=5, got %d", total)
	}
	if len(orgs) != 5 {
		t.Errorf("expected 5 orgs, got %d", len(orgs))
	}
}

func TestOrgStore_ListChildren(t *testing.T) {
	store := directory.NewMemoryOrgStore()
	parent := newOrg("Root", "")
	store.Create(parent)

	for i := 0; i < 3; i++ {
		store.Create(newOrg(fmt.Sprintf("Child%d", i), parent.ID))
	}
	store.Create(newOrg("Unrelated", ""))

	children, err := store.ListChildren(parent.ID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(children) != 3 {
		t.Errorf("expected 3 children, got %d", len(children))
	}
}

func TestOrgStore_Members(t *testing.T) {
	store := directory.NewMemoryOrgStore()
	org := newOrg("Engineering", "")
	store.Create(org)

	userID1 := uuid.New().String()
	userID2 := uuid.New().String()

	t.Run("add member", func(t *testing.T) {
		err := store.AddMember(&model.UserOrgRelation{
			UserID: userID1, OrgID: org.ID, Role: "member",
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("add duplicate member", func(t *testing.T) {
		err := store.AddMember(&model.UserOrgRelation{
			UserID: userID1, OrgID: org.ID, Role: "member",
		})
		if err != directory.ErrMemberExists {
			t.Errorf("expected ErrMemberExists, got %v", err)
		}
	})

	t.Run("add second member", func(t *testing.T) {
		err := store.AddMember(&model.UserOrgRelation{
			UserID: userID2, OrgID: org.ID, Role: "admin",
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("list members", func(t *testing.T) {
		rels, err := store.ListMembers(org.ID)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(rels) != 2 {
			t.Errorf("expected 2 members, got %d", len(rels))
		}
	})

	t.Run("get user orgs", func(t *testing.T) {
		rels, err := store.GetUserOrgs(userID1)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(rels) != 1 {
			t.Errorf("expected 1 org for user, got %d", len(rels))
		}
	})

	t.Run("remove member", func(t *testing.T) {
		err := store.RemoveMember(org.ID, userID1)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		rels, _ := store.ListMembers(org.ID)
		if len(rels) != 1 {
			t.Errorf("expected 1 member after removal, got %d", len(rels))
		}
		// 确认 userOrgs 也被清理
		userRels, _ := store.GetUserOrgs(userID1)
		if len(userRels) != 0 {
			t.Errorf("expected 0 orgs for removed user, got %d", len(userRels))
		}
	})

	t.Run("remove non-existent member", func(t *testing.T) {
		err := store.RemoveMember(org.ID, "non-existent")
		if err != directory.ErrMemberNotFound {
			t.Errorf("expected ErrMemberNotFound, got %v", err)
		}
	})
}

func TestOrgStore_DeleteCleansMembers(t *testing.T) {
	store := directory.NewMemoryOrgStore()
	org := newOrg("Temp", "")
	store.Create(org)
	userID := uuid.New().String()
	store.AddMember(&model.UserOrgRelation{UserID: userID, OrgID: org.ID, Role: "member"})

	store.Delete(org.ID)

	// 用户-组织关系也应清理
	rels, _ := store.GetUserOrgs(userID)
	if len(rels) != 0 {
		t.Errorf("expected 0 orgs after org deletion, got %d", len(rels))
	}
}

// ========== GroupStore 测试 ==========

func TestGroupStore_Create(t *testing.T) {
	store := directory.NewMemoryGroupStore()

	t.Run("create group", func(t *testing.T) {
		group := &model.Group{
			ID: uuid.New().String(), Name: "admins", Type: "security",
		}
		if err := store.Create(group); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})
}

func TestGroupStore_CRUD(t *testing.T) {
	store := directory.NewMemoryGroupStore()
	group := &model.Group{ID: uuid.New().String(), Name: "developers", Description: "Dev team"}
	store.Create(group)

	t.Run("get by id", func(t *testing.T) {
		got, err := store.GetByID(group.ID)
		if err != nil || got.Name != "developers" {
			t.Errorf("get failed: err=%v name=%s", err, got.Name)
		}
	})

	t.Run("update", func(t *testing.T) {
		err := store.Update(&model.Group{ID: group.ID, Description: "All developers"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		got, _ := store.GetByID(group.ID)
		if got.Description != "All developers" {
			t.Errorf("expected updated description, got %s", got.Description)
		}
	})

	t.Run("not found", func(t *testing.T) {
		_, err := store.GetByID("non-existent")
		if err != directory.ErrGroupNotFound {
			t.Errorf("expected ErrGroupNotFound, got %v", err)
		}
	})

	t.Run("delete", func(t *testing.T) {
		if err := store.Delete(group.ID); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		_, err := store.GetByID(group.ID)
		if err != directory.ErrGroupNotFound {
			t.Error("expected group to be deleted")
		}
	})
}

func TestGroupStore_List(t *testing.T) {
	store := directory.NewMemoryGroupStore()
	for i := 0; i < 4; i++ {
		store.Create(&model.Group{ID: uuid.New().String(), Name: fmt.Sprintf("group%d", i)})
	}

	groups, total, err := store.List(0, 10)
	if err != nil || total != 4 || len(groups) != 4 {
		t.Errorf("list failed: err=%v total=%d len=%d", err, total, len(groups))
	}
}

func TestGroupStore_Members(t *testing.T) {
	store := directory.NewMemoryGroupStore()
	group := &model.Group{ID: uuid.New().String(), Name: "ops"}
	store.Create(group)

	userID1 := uuid.New().String()
	userID2 := uuid.New().String()

	t.Run("add member", func(t *testing.T) {
		err := store.AddMember(&model.GroupMember{GroupID: group.ID, UserID: userID1, Role: "member"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("add duplicate", func(t *testing.T) {
		err := store.AddMember(&model.GroupMember{GroupID: group.ID, UserID: userID1, Role: "member"})
		if err != directory.ErrMemberExists {
			t.Errorf("expected ErrMemberExists, got %v", err)
		}
	})

	t.Run("add second member", func(t *testing.T) {
		store.AddMember(&model.GroupMember{GroupID: group.ID, UserID: userID2, Role: "admin"})
	})

	t.Run("list members", func(t *testing.T) {
		members, err := store.ListMembers(group.ID)
		if err != nil || len(members) != 2 {
			t.Errorf("list failed: err=%v len=%d", err, len(members))
		}
	})

	t.Run("get user groups", func(t *testing.T) {
		groups, err := store.GetUserGroups(userID1)
		if err != nil || len(groups) != 1 {
			t.Errorf("get user groups failed: err=%v len=%d", err, len(groups))
		}
	})

	t.Run("remove member", func(t *testing.T) {
		err := store.RemoveMember(group.ID, userID1)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		members, _ := store.ListMembers(group.ID)
		if len(members) != 1 {
			t.Errorf("expected 1 member, got %d", len(members))
		}
		userGroups, _ := store.GetUserGroups(userID1)
		if len(userGroups) != 0 {
			t.Errorf("expected 0 groups for removed user, got %d", len(userGroups))
		}
	})

	t.Run("remove non-existent member", func(t *testing.T) {
		err := store.RemoveMember(group.ID, "nobody")
		if err != directory.ErrMemberNotFound {
			t.Errorf("expected ErrMemberNotFound, got %v", err)
		}
	})
}

func TestGroupStore_DeleteCleansMembers(t *testing.T) {
	store := directory.NewMemoryGroupStore()
	group := &model.Group{ID: uuid.New().String(), Name: "temp"}
	store.Create(group)
	userID := uuid.New().String()
	store.AddMember(&model.GroupMember{GroupID: group.ID, UserID: userID, Role: "member"})

	store.Delete(group.ID)

	userGroups, _ := store.GetUserGroups(userID)
	if len(userGroups) != 0 {
		t.Errorf("expected 0 groups after group deletion, got %d", len(userGroups))
	}
}
