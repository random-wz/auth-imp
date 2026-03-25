package uds

import (
	"encoding/json"
	"fmt"

	"github.com/idp-service/internal/model"
	"github.com/idp-service/internal/service"
)

// ActionHandler 动作处理器函数类型
type ActionHandler func(payload json.RawMessage, requestID string) *JSONResponse

// HandlerRegistry 动作处理器注册表
type HandlerRegistry struct {
	handlers map[string]ActionHandler
	userSvc  *service.UserService
	dirSvc   *service.DirectoryService
}

// NewHandlerRegistry 创建处理器注册表
func NewHandlerRegistry(userSvc *service.UserService, dirSvc *service.DirectoryService) *HandlerRegistry {
	r := &HandlerRegistry{
		handlers: make(map[string]ActionHandler),
		userSvc:  userSvc,
		dirSvc:   dirSvc,
	}
	// 用户与认证
	r.register("auth", r.handleAuth)
	r.register("get_user", r.handleGetUser)
	r.register("trigger_sync", r.handleTriggerSync)
	r.register("ping", r.handlePing)
	// 组织管理
	r.register("create_org", r.handleCreateOrg)
	r.register("get_org", r.handleGetOrg)
	r.register("list_orgs", r.handleListOrgs)
	r.register("add_org_member", r.handleAddOrgMember)
	r.register("remove_org_member", r.handleRemoveOrgMember)
	r.register("list_org_members", r.handleListOrgMembers)
	r.register("get_user_orgs", r.handleGetUserOrgs)
	// 群组管理
	r.register("create_group", r.handleCreateGroup)
	r.register("get_group", r.handleGetGroup)
	r.register("list_groups", r.handleListGroups)
	r.register("add_group_member", r.handleAddGroupMember)
	r.register("remove_group_member", r.handleRemoveGroupMember)
	r.register("list_group_members", r.handleListGroupMembers)
	r.register("get_user_groups", r.handleGetUserGroups)
	return r
}

func (r *HandlerRegistry) register(action string, handler ActionHandler) {
	r.handlers[action] = handler
}

// Handle 处理请求，返回响应
func (r *HandlerRegistry) Handle(req *JSONRequest) *JSONResponse {
	handler, ok := r.handlers[req.Action]
	if !ok {
		return errorResp(req.RequestID, fmt.Sprintf("unknown action: %s", req.Action))
	}
	return handler(req.Payload, req.RequestID)
}

// ========== 用户与认证 ==========

func (r *HandlerRegistry) handleAuth(payload json.RawMessage, requestID string) *JSONResponse {
	var p AuthPayload
	if err := json.Unmarshal(payload, &p); err != nil {
		return errorResp(requestID, "invalid auth payload")
	}
	if p.Username == "" || p.Password == "" {
		return errorResp(requestID, "username and password are required")
	}
	u, err := r.userSvc.VerifyCredentials(p.Username, p.Password)
	if err != nil {
		return errorResp(requestID, err.Error())
	}
	return &JSONResponse{
		Status:    "success",
		RequestID: requestID,
		Payload: AuthResponsePayload{
			UserID:      u.ID,
			Username:    u.Username,
			Email:       u.Email,
			DisplayName: u.DisplayName,
		},
	}
}

func (r *HandlerRegistry) handleGetUser(payload json.RawMessage, requestID string) *JSONResponse {
	var p GetUserPayload
	if err := json.Unmarshal(payload, &p); err != nil {
		return errorResp(requestID, "invalid get_user payload")
	}
	var u *model.User
	var err error
	switch {
	case p.UserID != "":
		u, err = r.userSvc.GetUser(p.UserID)
	case p.Username != "":
		u, err = r.userSvc.GetUserByUsername(p.Username)
	default:
		return errorResp(requestID, "user_id or username is required")
	}
	if err != nil {
		return errorResp(requestID, "user not found")
	}
	return &JSONResponse{
		Status:    "success",
		RequestID: requestID,
		Payload: GetUserResponsePayload{
			UserID:        u.ID,
			Username:      u.Username,
			Email:         u.Email,
			DisplayName:   u.DisplayName,
			Organizations: []OrgInfo{},
		},
	}
}

func (r *HandlerRegistry) handleTriggerSync(payload json.RawMessage, requestID string) *JSONResponse {
	var p TriggerSyncPayload
	if err := json.Unmarshal(payload, &p); err != nil {
		return errorResp(requestID, "invalid trigger_sync payload")
	}
	if p.SourceSystem == "" {
		return errorResp(requestID, "source_system is required")
	}
	if p.SyncType == "" {
		p.SyncType = string(model.SyncTypeIncremental)
	}
	job, err := r.userSvc.CreateSyncJob(model.CreateSyncJobRequest{
		SourceSystem: p.SourceSystem,
		SyncType:     model.SyncType(p.SyncType),
	})
	if err != nil {
		return errorResp(requestID, fmt.Sprintf("failed to create sync job: %v", err))
	}
	return &JSONResponse{
		Status:    "success",
		RequestID: requestID,
		Payload: TriggerSyncResponsePayload{
			JobID:             job.ID,
			EstimatedDuration: "30s",
			Message:           "Sync job started",
		},
	}
}

func (r *HandlerRegistry) handlePing(_ json.RawMessage, requestID string) *JSONResponse {
	return &JSONResponse{
		Status:    "pong",
		RequestID: requestID,
	}
}

// ========== 组织管理 ==========

func (r *HandlerRegistry) handleCreateOrg(payload json.RawMessage, requestID string) *JSONResponse {
	var p CreateOrgPayload
	if err := json.Unmarshal(payload, &p); err != nil {
		return errorResp(requestID, "invalid create_org payload")
	}
	if p.Name == "" {
		return errorResp(requestID, "name is required")
	}
	org, err := r.dirSvc.CreateOrg(model.CreateOrgRequest{Name: p.Name, ParentID: p.ParentID})
	if err != nil {
		return errorResp(requestID, fmt.Sprintf("failed to create org: %v", err))
	}
	return &JSONResponse{
		Status:    "success",
		RequestID: requestID,
		Payload:   OrgDetailInfo{OrgID: org.ID, Name: org.Name, ParentID: org.ParentID, Path: org.Path},
	}
}

func (r *HandlerRegistry) handleGetOrg(payload json.RawMessage, requestID string) *JSONResponse {
	var p GetOrgPayload
	if err := json.Unmarshal(payload, &p); err != nil {
		return errorResp(requestID, "invalid get_org payload")
	}
	if p.OrgID == "" {
		return errorResp(requestID, "org_id is required")
	}
	org, err := r.dirSvc.GetOrg(p.OrgID)
	if err != nil {
		return errorResp(requestID, "org not found")
	}
	return &JSONResponse{
		Status:    "success",
		RequestID: requestID,
		Payload:   OrgDetailInfo{OrgID: org.ID, Name: org.Name, ParentID: org.ParentID, Path: org.Path},
	}
}

func (r *HandlerRegistry) handleListOrgs(payload json.RawMessage, requestID string) *JSONResponse {
	var p ListOrgsPayload
	json.Unmarshal(payload, &p)
	orgs, total, err := r.dirSvc.ListOrgs(p.Offset, p.Limit)
	if err != nil {
		return errorResp(requestID, fmt.Sprintf("failed to list orgs: %v", err))
	}
	items := make([]OrgDetailInfo, len(orgs))
	for i, o := range orgs {
		items[i] = OrgDetailInfo{OrgID: o.ID, Name: o.Name, ParentID: o.ParentID, Path: o.Path}
	}
	return &JSONResponse{
		Status:    "success",
		RequestID: requestID,
		Payload:   ListOrgsResponsePayload{Items: items, Total: total},
	}
}

func (r *HandlerRegistry) handleAddOrgMember(payload json.RawMessage, requestID string) *JSONResponse {
	var p AddOrgMemberPayload
	if err := json.Unmarshal(payload, &p); err != nil {
		return errorResp(requestID, "invalid add_org_member payload")
	}
	if p.OrgID == "" || p.UserID == "" {
		return errorResp(requestID, "org_id and user_id are required")
	}
	_, err := r.dirSvc.AddOrgMember(p.OrgID, model.AddOrgMemberRequest{UserID: p.UserID, Role: p.Role})
	if err != nil {
		return errorResp(requestID, fmt.Sprintf("failed to add member: %v", err))
	}
	return &JSONResponse{Status: "success", RequestID: requestID, Payload: map[string]string{"message": "member added"}}
}

func (r *HandlerRegistry) handleRemoveOrgMember(payload json.RawMessage, requestID string) *JSONResponse {
	var p RemoveOrgMemberPayload
	if err := json.Unmarshal(payload, &p); err != nil {
		return errorResp(requestID, "invalid remove_org_member payload")
	}
	if p.OrgID == "" || p.UserID == "" {
		return errorResp(requestID, "org_id and user_id are required")
	}
	if err := r.dirSvc.RemoveOrgMember(p.OrgID, p.UserID); err != nil {
		return errorResp(requestID, fmt.Sprintf("failed to remove member: %v", err))
	}
	return &JSONResponse{Status: "success", RequestID: requestID, Payload: map[string]string{"message": "member removed"}}
}

func (r *HandlerRegistry) handleListOrgMembers(payload json.RawMessage, requestID string) *JSONResponse {
	var p ListOrgMembersPayload
	if err := json.Unmarshal(payload, &p); err != nil {
		return errorResp(requestID, "invalid list_org_members payload")
	}
	if p.OrgID == "" {
		return errorResp(requestID, "org_id is required")
	}
	details, err := r.dirSvc.ListOrgMembers(p.OrgID)
	if err != nil {
		return errorResp(requestID, fmt.Sprintf("failed to list members: %v", err))
	}
	items := make([]OrgMemberInfo, len(details))
	for i, d := range details {
		items[i] = OrgMemberInfo{
			UserID:      d.UserID,
			Username:    d.Username,
			DisplayName: d.DisplayName,
			Role:        d.Role,
		}
	}
	return &JSONResponse{
		Status:    "success",
		RequestID: requestID,
		Payload:   ListOrgMembersResponsePayload{Items: items, Total: int64(len(items))},
	}
}

func (r *HandlerRegistry) handleGetUserOrgs(payload json.RawMessage, requestID string) *JSONResponse {
	var p GetUserOrgsPayload
	if err := json.Unmarshal(payload, &p); err != nil {
		return errorResp(requestID, "invalid get_user_orgs payload")
	}
	if p.UserID == "" {
		return errorResp(requestID, "user_id is required")
	}
	rels, err := r.dirSvc.GetUserOrgs(p.UserID)
	if err != nil {
		return errorResp(requestID, fmt.Sprintf("failed to get user orgs: %v", err))
	}
	items := make([]OrgDetailInfo, 0, len(rels))
	for _, rel := range rels {
		info := OrgDetailInfo{OrgID: rel.OrgID, Role: rel.Role}
		if org, err := r.dirSvc.GetOrg(rel.OrgID); err == nil {
			info.Name = org.Name
			info.Path = org.Path
			info.ParentID = org.ParentID
		}
		items = append(items, info)
	}
	return &JSONResponse{
		Status:    "success",
		RequestID: requestID,
		Payload:   ListOrgsResponsePayload{Items: items, Total: int64(len(items))},
	}
}

// ========== 群组管理 ==========

func (r *HandlerRegistry) handleCreateGroup(payload json.RawMessage, requestID string) *JSONResponse {
	var p CreateGroupPayload
	if err := json.Unmarshal(payload, &p); err != nil {
		return errorResp(requestID, "invalid create_group payload")
	}
	if p.Name == "" {
		return errorResp(requestID, "name is required")
	}
	group, err := r.dirSvc.CreateGroup(model.CreateGroupRequest{Name: p.Name, Description: p.Description, Type: p.Type})
	if err != nil {
		return errorResp(requestID, fmt.Sprintf("failed to create group: %v", err))
	}
	return &JSONResponse{
		Status:    "success",
		RequestID: requestID,
		Payload:   GroupDetailInfo{GroupID: group.ID, Name: group.Name, Description: group.Description, Type: group.Type},
	}
}

func (r *HandlerRegistry) handleGetGroup(payload json.RawMessage, requestID string) *JSONResponse {
	var p GetGroupPayload
	if err := json.Unmarshal(payload, &p); err != nil {
		return errorResp(requestID, "invalid get_group payload")
	}
	if p.GroupID == "" {
		return errorResp(requestID, "group_id is required")
	}
	group, err := r.dirSvc.GetGroup(p.GroupID)
	if err != nil {
		return errorResp(requestID, "group not found")
	}
	return &JSONResponse{
		Status:    "success",
		RequestID: requestID,
		Payload:   GroupDetailInfo{GroupID: group.ID, Name: group.Name, Description: group.Description, Type: group.Type},
	}
}

func (r *HandlerRegistry) handleListGroups(payload json.RawMessage, requestID string) *JSONResponse {
	var p ListGroupsPayload
	json.Unmarshal(payload, &p)
	groups, total, err := r.dirSvc.ListGroups(p.Offset, p.Limit)
	if err != nil {
		return errorResp(requestID, fmt.Sprintf("failed to list groups: %v", err))
	}
	items := make([]GroupDetailInfo, len(groups))
	for i, g := range groups {
		items[i] = GroupDetailInfo{GroupID: g.ID, Name: g.Name, Description: g.Description, Type: g.Type}
	}
	return &JSONResponse{
		Status:    "success",
		RequestID: requestID,
		Payload:   ListGroupsResponsePayload{Items: items, Total: total},
	}
}

func (r *HandlerRegistry) handleAddGroupMember(payload json.RawMessage, requestID string) *JSONResponse {
	var p AddGroupMemberPayload
	if err := json.Unmarshal(payload, &p); err != nil {
		return errorResp(requestID, "invalid add_group_member payload")
	}
	if p.GroupID == "" || p.UserID == "" {
		return errorResp(requestID, "group_id and user_id are required")
	}
	_, err := r.dirSvc.AddGroupMember(p.GroupID, model.AddGroupMemberRequest{UserID: p.UserID, Role: p.Role})
	if err != nil {
		return errorResp(requestID, fmt.Sprintf("failed to add member: %v", err))
	}
	return &JSONResponse{Status: "success", RequestID: requestID, Payload: map[string]string{"message": "member added"}}
}

func (r *HandlerRegistry) handleRemoveGroupMember(payload json.RawMessage, requestID string) *JSONResponse {
	var p RemoveGroupMemberPayload
	if err := json.Unmarshal(payload, &p); err != nil {
		return errorResp(requestID, "invalid remove_group_member payload")
	}
	if p.GroupID == "" || p.UserID == "" {
		return errorResp(requestID, "group_id and user_id are required")
	}
	if err := r.dirSvc.RemoveGroupMember(p.GroupID, p.UserID); err != nil {
		return errorResp(requestID, fmt.Sprintf("failed to remove member: %v", err))
	}
	return &JSONResponse{Status: "success", RequestID: requestID, Payload: map[string]string{"message": "member removed"}}
}

func (r *HandlerRegistry) handleListGroupMembers(payload json.RawMessage, requestID string) *JSONResponse {
	var p ListGroupMembersPayload
	if err := json.Unmarshal(payload, &p); err != nil {
		return errorResp(requestID, "invalid list_group_members payload")
	}
	if p.GroupID == "" {
		return errorResp(requestID, "group_id is required")
	}
	details, err := r.dirSvc.ListGroupMembers(p.GroupID)
	if err != nil {
		return errorResp(requestID, fmt.Sprintf("failed to list members: %v", err))
	}
	items := make([]GroupMemberInfo, len(details))
	for i, d := range details {
		items[i] = GroupMemberInfo{
			UserID:      d.UserID,
			Username:    d.Username,
			DisplayName: d.DisplayName,
			Role:        d.Role,
		}
	}
	return &JSONResponse{
		Status:    "success",
		RequestID: requestID,
		Payload:   ListGroupMembersResponsePayload{Items: items, Total: int64(len(items))},
	}
}

func (r *HandlerRegistry) handleGetUserGroups(payload json.RawMessage, requestID string) *JSONResponse {
	var p GetUserGroupsPayload
	if err := json.Unmarshal(payload, &p); err != nil {
		return errorResp(requestID, "invalid get_user_groups payload")
	}
	if p.UserID == "" {
		return errorResp(requestID, "user_id is required")
	}
	members, err := r.dirSvc.GetUserGroups(p.UserID)
	if err != nil {
		return errorResp(requestID, fmt.Sprintf("failed to get user groups: %v", err))
	}
	items := make([]GroupDetailInfo, 0, len(members))
	for _, m := range members {
		info := GroupDetailInfo{GroupID: m.GroupID, Role: m.Role}
		if g, err := r.dirSvc.GetGroup(m.GroupID); err == nil {
			info.Name = g.Name
			info.Description = g.Description
			info.Type = g.Type
		}
		items = append(items, info)
	}
	return &JSONResponse{
		Status:    "success",
		RequestID: requestID,
		Payload:   ListGroupsResponsePayload{Items: items, Total: int64(len(items))},
	}
}

// ========== UDS 内部错误辅助 ==========

func errorResp(requestID, msg string) *JSONResponse {
	return &JSONResponse{
		Status:    "error",
		RequestID: requestID,
		Error:     msg,
	}
}
