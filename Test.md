# IDP Service 测试文档

## 目录

1. [测试架构](#测试架构)
2. [单元测试与集成测试](#单元测试与集成测试)
3. [REST API 功能测试](#rest-api-功能测试)
4. [UDS 功能测试](#uds-功能测试)
5. [测试结果汇总](#测试结果汇总)
6. [运行测试](#运行测试)

---

## 测试架构

```
测试层级
├── 单元测试（store 层）
│   ├── internal/user       — MemoryStore / MemorySyncJobStore
│   ├── internal/auth       — 密码哈希 / JWT 生成与验证
│   └── internal/directory  — MemoryOrgStore / MemoryGroupStore
├── 集成测试（handler 层）
│   ├── internal/api        — REST API 端到端（httptest.Server）
│   └── internal/uds        — UDS 端到端（真实 Unix Socket）
└── 手动功能测试
    ├── REST API（curl）
    └── UDS（nc）
```

**技术栈：**
- Go 标准库 `testing` + `net/http/httptest`
- 全内存存储（无外部数据库依赖，测试零配置）
- UDS 测试使用随机路径（`/tmp/idp-test-<uuid>.sock`），测试隔离

---

## 单元测试与集成测试

### 运行所有自动化测试

```bash
go test ./...
```

### 各包测试说明

#### `internal/auth`（4 个测试，10 个子用例）

| 测试函数 | 描述 |
|----------|------|
| `TestHashPassword` | 正常哈希、密码过短拒绝、不同密码产生不同哈希 |
| `TestVerifyPassword` | 正确密码通过、错误密码拒绝、空密码拒绝 |
| `TestGenerateAndValidateToken` | Token 往返验证、无效 token、篡改 token、不同 secret 签名 |
| `TestTokenExpiry` | 过期 Token 验证失败 |

#### `internal/user`（7 个测试，15 个子用例）

| 测试函数 | 描述 |
|----------|------|
| `TestMemoryStore_Create` | 正常创建、用户名重复、邮箱重复 |
| `TestMemoryStore_GetByID` | 按 ID 查找、不存在返回 ErrUserNotFound |
| `TestMemoryStore_GetByUsername` | 按用户名查找、不存在时的错误 |
| `TestMemoryStore_Update` | 更新 display_name、禁用用户状态、不存在用户 |
| `TestMemoryStore_Delete` | 软删除现有用户、删除不存在用户 |
| `TestMemoryStore_List` | 全量列表、带 offset、带 limit |
| `TestMemoryStore_Search` | 按用户名搜索、按邮箱搜索、无结果 |

#### `internal/directory`（9 个测试，31 个子用例）

| 测试函数 | 描述 |
|----------|------|
| `TestOrgStore_Create` | 根组织、子组织、父不存在时报错、ID 重复报错 |
| `TestOrgStore_GetByID` | 正常获取、不存在返回 ErrOrgNotFound |
| `TestOrgStore_Update` | 更新名称、更新不存在的组织 |
| `TestOrgStore_Delete` | 删除现有、删除不存在 |
| `TestOrgStore_List` | 分页列表 |
| `TestOrgStore_ListChildren` | 按 parent_id 过滤子组织 |
| `TestOrgStore_Members` | 添加/重复添加/列表/获取用户组织/移除/移除不存在 |
| `TestOrgStore_DeleteCleansMembers` | 删除组织时级联清理成员关系 |
| `TestGroupStore_Create / CRUD / List / Members / DeleteCleansMembers` | 群组完整 CRUD 及成员管理 |

#### `internal/api`（17 个测试，27 个子用例）

| 测试函数 | 描述 |
|----------|------|
| `TestLogin_Success/WrongPassword/UserNotFound` | 登录流程 |
| `TestCreateUser_Success/DuplicateUsername` | 用户创建 |
| `TestGetUser_Success/NotFound` | 用户查询 |
| `TestUpdateUser` | 用户更新 |
| `TestDeleteUser` | 用户删除 |
| `TestListUsers` | 用户列表 |
| `TestJWTMiddleware_MissingToken/InvalidToken` | JWT 鉴权中间件 |
| `TestCreateSyncJob` | 同步任务创建 |
| `TestHealthCheck` | 健康检查 |
| `TestOrg_CreateAndGet/CreateHierarchy/Update/Delete/ListOrgs/NotFound` | 组织 CRUD |
| `TestOrg_MemberManagement` | 组织成员管理（含 6 个子用例） |
| `TestGroup_CreateAndGet/Update/Delete/ListGroups` | 群组 CRUD |
| `TestGroup_MemberManagement` | 群组成员管理（含 6 个子用例） |
| `TestGroup_UserInMultipleGroups` | 用户加入多群组 |

#### `internal/uds`（15 个测试，19 个子用例）

| 测试函数 | 描述 |
|----------|------|
| `TestUDSServer_JSONMode_Handshake` | JSON 模式握手 |
| `TestUDSServer_JSONMode_Auth_Success/WrongPassword` | 认证动作 |
| `TestUDSServer_JSONMode_GetUser/NotFound` | 获取用户动作 |
| `TestUDSServer_JSONMode_TriggerSync` | 触发同步动作 |
| `TestUDSServer_JSONMode_Ping` | 心跳动作 |
| `TestUDSServer_JSONMode_UnknownAction` | 未知动作错误处理 |
| `TestUDSServer_ProtobufMode_Handshake/Auth` | Protobuf 帧模式 |
| `TestUDSServer_InvalidHandshake` | 无效握手处理 |
| `TestUDSServer_MultipleConnections` | 多连接并发 |
| `TestCodec_JSONCodec/ProtobufCodec` | 编解码器单测 |
| `TestUDS_Org_CreateAndGet/GetNotFound/ListOrgs` | UDS 组织管理 |
| `TestUDS_Org_MemberManagement` | UDS 组织成员管理（含 6 个子用例） |
| `TestUDS_Group_CreateAndGet/ListGroups` | UDS 群组管理 |
| `TestUDS_Group_MemberManagement` | UDS 群组成员管理（含 5 个子用例） |

---

## REST API 功能测试

### 准备：启动服务并获取 Token

```bash
# 启动服务（内置 admin/Admin@123456 初始账户）
GIN_MODE=release go run ./cmd/main.go

# 获取 Token（后续所有请求均需携带）
TOKEN=$(curl -s -X POST http://localhost:8080/api/v1/auth/login \
  -H "Content-Type: application/json" \
  -d '{"username":"admin","password":"Admin@123456"}' \
  | python3 -c "import sys,json; print(json.load(sys.stdin)['token'])")
```

---

### 健康检查

```bash
curl http://localhost:8080/health
# 预期: {"status":"ok"}  HTTP 200
```

---

### 认证模块

#### 登录成功

```bash
curl -s -X POST http://localhost:8080/api/v1/auth/login \
  -H "Content-Type: application/json" \
  -d '{"username":"admin","password":"Admin@123456"}'
```

预期响应（HTTP 200）：
```json
{
  "token": "<jwt-token>",
  "expires": 1774506817,
  "user": {
    "id": "...",
    "username": "admin",
    "email": "admin@example.com",
    "display_name": "System Administrator",
    "status": "active"
  }
}
```
> **注意：** `password_hash` 字段不会出现在响应中

#### 登录失败 - 密码错误

```bash
curl -s -X POST http://localhost:8080/api/v1/auth/login \
  -H "Content-Type: application/json" \
  -d '{"username":"admin","password":"wrong"}'
# 预期: {"error":"invalid credentials"}  HTTP 401
```

#### 登录失败 - 用户不存在

```bash
curl -s -X POST http://localhost:8080/api/v1/auth/login \
  -H "Content-Type: application/json" \
  -d '{"username":"nobody","password":"x"}'
# 预期: {"error":"invalid credentials"}  HTTP 401
```

#### JWT 鉴权 - 无 Token

```bash
curl -s http://localhost:8080/api/v1/users
# 预期: {"error":"missing authorization token"}  HTTP 401
```

#### JWT 鉴权 - 无效 Token

```bash
curl -s http://localhost:8080/api/v1/users \
  -H "Authorization: Bearer invalid.token"
# 预期: {"error":"invalid token"}  HTTP 401
```

---

### 用户管理

#### 创建用户

```bash
curl -s -X POST http://localhost:8080/api/v1/users \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $TOKEN" \
  -d '{"username":"alice","email":"alice@example.com","password":"Alice@123456","display_name":"Alice Wang"}'
```

预期响应（HTTP 201）：
```json
{
  "id": "<uuid>",
  "username": "alice",
  "email": "alice@example.com",
  "display_name": "Alice Wang",
  "status": "active",
  "created_at": "...",
  "updated_at": "..."
}
```

#### 创建用户 - 用户名重复

```bash
curl -s -X POST http://localhost:8080/api/v1/users \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $TOKEN" \
  -d '{"username":"alice","email":"alice2@example.com","password":"Alice@123456"}'
# 预期: {"error":"user already exists"}  HTTP 409
```

#### 获取用户

```bash
curl -s http://localhost:8080/api/v1/users/<user-id> \
  -H "Authorization: Bearer $TOKEN"
# 预期: 用户对象  HTTP 200

curl -s http://localhost:8080/api/v1/users/non-existent \
  -H "Authorization: Bearer $TOKEN"
# 预期: {"error":"user not found"}  HTTP 404
```

#### 更新用户

```bash
curl -s -X PUT http://localhost:8080/api/v1/users/<user-id> \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $TOKEN" \
  -d '{"display_name":"Alice Chen"}'
# 预期: 更新后的用户对象  HTTP 200
```

#### 列出用户（分页 + 搜索）

```bash
# 分页
curl -s "http://localhost:8080/api/v1/users?offset=0&limit=10" \
  -H "Authorization: Bearer $TOKEN"

# 关键词搜索
curl -s "http://localhost:8080/api/v1/users?q=alice" \
  -H "Authorization: Bearer $TOKEN"
```

预期响应（HTTP 200）：
```json
{"total": 2, "offset": 0, "limit": 10, "items": [...]}
```

#### 删除用户

```bash
curl -s -X DELETE http://localhost:8080/api/v1/users/<user-id> \
  -H "Authorization: Bearer $TOKEN"
# 预期: HTTP 204（无响应体）
```

---

### 组织管理

#### 创建根组织

```bash
ORG=$(curl -s -X POST http://localhost:8080/api/v1/orgs \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $TOKEN" \
  -d '{"name":"Acme Corp"}')
ORG_ID=$(echo $ORG | python3 -c "import sys,json; print(json.load(sys.stdin)['id'])")
```

预期响应（HTTP 201）：
```json
{
  "id": "<uuid>",
  "name": "Acme Corp",
  "path": "/<uuid>",
  "created_at": "...",
  "updated_at": "..."
}
```

#### 创建子组织（层级路径）

```bash
curl -s -X POST http://localhost:8080/api/v1/orgs \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $TOKEN" \
  -d "{\"name\":\"Engineering\",\"parent_id\":\"$ORG_ID\"}"
# 预期 path: "/<parent-id>/<child-id>"  HTTP 201
```

#### 列出子组织

```bash
curl -s http://localhost:8080/api/v1/orgs/$ORG_ID/children \
  -H "Authorization: Bearer $TOKEN"
# 预期: {"items":[...],"total":1}  HTTP 200
```

#### 获取 / 更新 / 删除组织

```bash
# 获取
curl -s http://localhost:8080/api/v1/orgs/$ORG_ID \
  -H "Authorization: Bearer $TOKEN"
# 预期: 组织对象  HTTP 200

# 更新
curl -s -X PUT http://localhost:8080/api/v1/orgs/$ORG_ID \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $TOKEN" \
  -d '{"name":"New Name"}'
# 预期: 更新后的组织对象  HTTP 200

# 删除
curl -s -X DELETE http://localhost:8080/api/v1/orgs/$ORG_ID \
  -H "Authorization: Bearer $TOKEN"
# 预期: HTTP 204
```

#### 组织成员管理

```bash
# 添加成员
curl -s -X POST http://localhost:8080/api/v1/orgs/$ORG_ID/members \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $TOKEN" \
  -d "{\"user_id\":\"$USER_ID\",\"role\":\"member\"}"
# 预期: 成员关系对象  HTTP 201

# 添加重复成员 → HTTP 409
# 添加不存在的用户 → HTTP 404

# 列出成员（含用户详情）
curl -s http://localhost:8080/api/v1/orgs/$ORG_ID/members \
  -H "Authorization: Bearer $TOKEN"
# 预期: {"items":[{"user_id":"...","username":"...","email":"...","role":"..."}],"total":1}

# 查询用户所属组织
curl -s http://localhost:8080/api/v1/users/$USER_ID/orgs \
  -H "Authorization: Bearer $TOKEN"

# 移除成员
curl -s -X DELETE http://localhost:8080/api/v1/orgs/$ORG_ID/members/$USER_ID \
  -H "Authorization: Bearer $TOKEN"
# 预期: HTTP 204
```

---

### 群组管理

#### 创建 / 获取 / 更新 / 删除群组

```bash
# 创建
GROUP=$(curl -s -X POST http://localhost:8080/api/v1/groups \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $TOKEN" \
  -d '{"name":"backend-team","description":"Backend developers","type":"project"}')
GROUP_ID=$(echo $GROUP | python3 -c "import sys,json; print(json.load(sys.stdin)['id'])")
# 预期: 群组对象  HTTP 201

# 更新
curl -s -X PUT http://localhost:8080/api/v1/groups/$GROUP_ID \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $TOKEN" \
  -d '{"description":"Backend & API developers"}'
# 预期: 更新后的群组对象  HTTP 200

# 删除
curl -s -X DELETE http://localhost:8080/api/v1/groups/$GROUP_ID \
  -H "Authorization: Bearer $TOKEN"
# 预期: HTTP 204
```

#### 群组成员管理

```bash
# 添加成员
curl -s -X POST http://localhost:8080/api/v1/groups/$GROUP_ID/members \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $TOKEN" \
  -d "{\"user_id\":\"$USER_ID\",\"role\":\"member\"}"
# 预期: {"group_id":"...","user_id":"...","role":"member","joined_at":"..."}  HTTP 201

# 列出成员（含用户详情）
curl -s http://localhost:8080/api/v1/groups/$GROUP_ID/members \
  -H "Authorization: Bearer $TOKEN"
# 预期: {"items":[{"username":"...","email":"...","role":"...","joined_at":"..."}],"total":1}

# 查询用户所属群组
curl -s http://localhost:8080/api/v1/users/$USER_ID/groups \
  -H "Authorization: Bearer $TOKEN"

# 移除成员
curl -s -X DELETE http://localhost:8080/api/v1/groups/$GROUP_ID/members/$USER_ID \
  -H "Authorization: Bearer $TOKEN"
# 预期: HTTP 204
```

---

### 同步任务

```bash
# 创建同步任务
JOB=$(curl -s -X POST http://localhost:8080/api/v1/sync/jobs \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $TOKEN" \
  -d '{"source_system":"ldap","sync_type":"full"}')
JOB_ID=$(echo $JOB | python3 -c "import sys,json; print(json.load(sys.stdin)['id'])")
# 预期: {"id":"...","source_system":"ldap","sync_type":"full","status":"pending",...}  HTTP 201

# 查询同步任务
curl -s http://localhost:8080/api/v1/sync/jobs/$JOB_ID \
  -H "Authorization: Bearer $TOKEN"
# 预期: 任务对象  HTTP 200
```

---

## UDS 功能测试

UDS 使用换行分隔的 JSON 消息。每次连接需先完成握手，再发送业务请求。

### 辅助函数

```bash
# 发送单次 UDS 请求并获取业务响应（第2行，跳过握手响应）
uds_req() {
  local req="$1"
  (echo '{"version":"1.0","format":"json"}'; sleep 0.1; echo "$req"; sleep 0.3) \
    | nc -U /tmp/idp-uds.sock 2>/dev/null | tail -1
}
```

### 握手协议

```bash
# JSON 模式握手
echo '{"version":"1.0","format":"json"}' | nc -U /tmp/idp-uds.sock
# 预期: {"status":"ok","version":"1.0","format":"json"}

# Protobuf 模式握手（4字节小端序长度前缀 + JSON体）
# 见 internal/uds/codec.go: ProtobufCodec
```

### 认证与用户

#### Ping

```bash
uds_req '{"action":"ping","request_id":"r1","payload":{}}'
# 预期: {"status":"pong","request_id":"r1"}
```

#### 认证（仅验证凭据，不生成 JWT）

```bash
# 成功
uds_req '{"action":"auth","request_id":"r2","payload":{"username":"admin","password":"Admin@123456"}}'
# 预期: {"status":"success","payload":{"user_id":"...","username":"admin","email":"...","display_name":"..."}}

# 密码错误
uds_req '{"action":"auth","request_id":"r3","payload":{"username":"admin","password":"wrong"}}'
# 预期: {"status":"error","error":"invalid credentials"}
```

#### 获取用户（支持 user_id 或 username）

```bash
# 按用户名
uds_req '{"action":"get_user","request_id":"r4","payload":{"username":"admin"}}'
# 预期: {"status":"success","payload":{"user_id":"...","username":"admin",...,"organizations":[]}}

# 按 ID
uds_req "{\"action\":\"get_user\",\"request_id\":\"r5\",\"payload\":{\"user_id\":\"$USER_ID\"}}"

# 不存在
uds_req '{"action":"get_user","request_id":"r6","payload":{"username":"nobody"}}'
# 预期: {"status":"error","error":"user not found"}
```

#### 触发同步

```bash
uds_req '{"action":"trigger_sync","request_id":"r7","payload":{"source_system":"ad","sync_type":"incremental"}}'
# 预期: {"status":"success","payload":{"job_id":"...","estimated_duration":"30s","message":"Sync job started"}}
```

### UDS 组织管理

#### 创建 / 获取组织

```bash
# 创建根组织
RESP=$(uds_req '{"action":"create_org","request_id":"o1","payload":{"name":"GlobalCorp"}}')
ORG_ID=$(echo $RESP | python3 -c "import sys,json; print(json.load(sys.stdin)['payload']['org_id'])")
# 预期 payload: {"org_id":"...","name":"GlobalCorp","path":"/<id>"}

# 创建子组织
uds_req "{\"action\":\"create_org\",\"request_id\":\"o2\",\"payload\":{\"name\":\"TechDiv\",\"parent_id\":\"$ORG_ID\"}}"
# 预期 path: "/<parent-id>/<child-id>"

# 获取组织
uds_req "{\"action\":\"get_org\",\"request_id\":\"o3\",\"payload\":{\"org_id\":\"$ORG_ID\"}}"

# 不存在
uds_req '{"action":"get_org","request_id":"o4","payload":{"org_id":"no-such"}}'
# 预期: {"status":"error","error":"org not found"}
```

#### 列出组织

```bash
uds_req '{"action":"list_orgs","request_id":"o5","payload":{"offset":0,"limit":10}}'
# 预期: {"status":"success","payload":{"items":[...],"total":N}}
```

#### 组织成员管理

```bash
# 添加成员
uds_req "{\"action\":\"add_org_member\",\"request_id\":\"o6\",\"payload\":{\"org_id\":\"$ORG_ID\",\"user_id\":\"$USER_ID\",\"role\":\"admin\"}}"
# 预期: {"status":"success","payload":{"message":"member added"}}

# 重复添加 → error: "failed to add member: member already exists"
# 用户不存在 → error: "user not found"

# 列出成员（含 username/display_name）
uds_req "{\"action\":\"list_org_members\",\"request_id\":\"o7\",\"payload\":{\"org_id\":\"$ORG_ID\"}}"
# 预期: {"items":[{"user_id":"...","username":"...","display_name":"...","role":"admin"}],"total":1}

# 查询用户所属组织
uds_req "{\"action\":\"get_user_orgs\",\"request_id\":\"o8\",\"payload\":{\"user_id\":\"$USER_ID\"}}"
# 预期: {"items":[{"org_id":"...","name":"...","path":"...","role":"admin"}],"total":1}

# 移除成员
uds_req "{\"action\":\"remove_org_member\",\"request_id\":\"o9\",\"payload\":{\"org_id\":\"$ORG_ID\",\"user_id\":\"$USER_ID\"}}"
# 预期: {"status":"success","payload":{"message":"member removed"}}
```

### UDS 群组管理

#### 创建 / 获取群组

```bash
# 创建
RESP=$(uds_req '{"action":"create_group","request_id":"g1","payload":{"name":"sre-team","description":"Site Reliability","type":"ops"}}')
GRP_ID=$(echo $RESP | python3 -c "import sys,json; print(json.load(sys.stdin)['payload']['group_id'])")
# 预期: {"group_id":"...","name":"sre-team","description":"...","type":"ops"}

# 获取
uds_req "{\"action\":\"get_group\",\"request_id\":\"g2\",\"payload\":{\"group_id\":\"$GRP_ID\"}}"

# 不存在
uds_req '{"action":"get_group","request_id":"g3","payload":{"group_id":"no-such"}}'
# 预期: {"status":"error","error":"group not found"}
```

#### 列出群组

```bash
uds_req '{"action":"list_groups","request_id":"g4","payload":{"offset":0,"limit":10}}'
# 预期: {"items":[...],"total":N}
```

#### 群组成员管理

```bash
# 添加成员
uds_req "{\"action\":\"add_group_member\",\"request_id\":\"g5\",\"payload\":{\"group_id\":\"$GRP_ID\",\"user_id\":\"$USER_ID\",\"role\":\"owner\"}}"

# 列出成员
uds_req "{\"action\":\"list_group_members\",\"request_id\":\"g6\",\"payload\":{\"group_id\":\"$GRP_ID\"}}"
# 预期: {"items":[{"user_id":"...","username":"...","display_name":"...","role":"owner"}],"total":1}

# 查询用户所属群组
uds_req "{\"action\":\"get_user_groups\",\"request_id\":\"g7\",\"payload\":{\"user_id\":\"$USER_ID\"}}"
# 预期: {"items":[{"group_id":"...","name":"...","role":"owner"}],"total":1}

# 移除成员
uds_req "{\"action\":\"remove_group_member\",\"request_id\":\"g8\",\"payload\":{\"group_id\":\"$GRP_ID\",\"user_id\":\"$USER_ID\"}}"
```

#### 未知动作

```bash
uds_req '{"action":"invalid_action","request_id":"x1","payload":{}}'
# 预期: {"status":"error","error":"unknown action: invalid_action"}
```

---

## 测试结果汇总

### 自动化测试（2026-03-25）

| 包 | 测试用例 | 结果 | 耗时 |
|----|---------|------|------|
| `internal/auth` | 4 (10 子用例) | ✅ PASS | ~16ms |
| `internal/user` | 7 (15 子用例) | ✅ PASS | ~3ms |
| `internal/directory` | 9 (31 子用例) | ✅ PASS | ~4ms |
| `internal/api` | 17 (27 子用例) | ✅ PASS | ~130ms |
| `internal/uds` | 15 (19 子用例) | ✅ PASS | ~100ms |
| **合计** | **52 函数 / 72 子用例** | **✅ 全部通过** | **~253ms** |

### 手动功能测试（2026-03-25）

| 编号 | 测试项 | 接口 | 预期状态 | 实际状态 |
|------|--------|------|---------|---------|
| 1 | 健康检查 | REST | 200 | ✅ |
| 2 | 登录成功（返回 JWT + 用户信息） | REST | 200 | ✅ |
| 3 | 登录失败 - 密码错误 | REST | 401 | ✅ |
| 4 | 登录失败 - 用户不存在 | REST | 401 | ✅ |
| 5 | 创建用户（password_hash 不外露） | REST | 201 | ✅ |
| 6 | 创建用户 - 用户名重复 | REST | 409 | ✅ |
| 7 | 获取用户 | REST | 200 | ✅ |
| 8 | 获取用户 - 不存在 | REST | 404 | ✅ |
| 9 | 更新用户 | REST | 200 | ✅ |
| 10 | 列出用户（分页） | REST | 200 | ✅ |
| 11 | 无 Token 访问 | REST | 401 | ✅ |
| 12 | 无效 Token 访问 | REST | 401 | ✅ |
| 13 | 创建根组织（path 自动生成） | REST | 201 | ✅ |
| 14 | 创建子组织（path 继承层级） | REST | 201 | ✅ |
| 15 | 列出子组织 | REST | 200 | ✅ |
| 16 | 更新组织 | REST | 200 | ✅ |
| 17 | 获取组织 | REST | 200 | ✅ |
| 18 | 获取组织 - 不存在 | REST | 404 | ✅ |
| 19 | 添加组织成员 | REST | 201 | ✅ |
| 20 | 添加组织成员 - 重复 | REST | 409 | ✅ |
| 21 | 添加组织成员 - 用户不存在 | REST | 404 | ✅ |
| 22 | 列出组织成员（含用户信息） | REST | 200 | ✅ |
| 23 | 获取用户所属组织 | REST | 200 | ✅ |
| 24 | 移除组织成员 | REST | 204 | ✅ |
| 25 | 删除组织 | REST | 204 | ✅ |
| 26 | 创建群组 | REST | 201 | ✅ |
| 27 | 获取群组 | REST | 200 | ✅ |
| 28 | 更新群组 | REST | 200 | ✅ |
| 29 | 添加群组成员（记录 joined_at） | REST | 201 | ✅ |
| 30 | 添加群组成员 - 重复 | REST | 409 | ✅ |
| 31 | 列出群组成员（含用户信息） | REST | 200 | ✅ |
| 32 | 获取用户所属群组 | REST | 200 | ✅ |
| 33 | 移除群组成员 | REST | 204 | ✅ |
| 34 | 删除群组 | REST | 204 | ✅ |
| 35 | 创建同步任务 | REST | 201 | ✅ |
| 36 | 获取同步任务 | REST | 200 | ✅ |
| 37 | 获取同步任务 - 不存在 | REST | 404 | ✅ |
| 38 | 删除用户 | REST | 204 | ✅ |
| 39 | UDS Ping | UDS | pong | ✅ |
| 40 | UDS 认证成功（无 JWT，仅返回用户信息） | UDS | success | ✅ |
| 41 | UDS 认证失败 - 密码错误 | UDS | error | ✅ |
| 42 | UDS 获取用户（按用户名） | UDS | success | ✅ |
| 43 | UDS 获取用户 - 不存在 | UDS | error | ✅ |
| 44 | UDS 触发同步 | UDS | success | ✅ |
| 45 | UDS 创建组织 | UDS | success | ✅ |
| 46 | UDS 创建子组织（层级路径） | UDS | success | ✅ |
| 47 | UDS 获取组织 | UDS | success | ✅ |
| 48 | UDS 列出组织 | UDS | success | ✅ |
| 49 | UDS 添加组织成员 | UDS | success | ✅ |
| 50 | UDS 添加组织成员 - 重复 | UDS | error | ✅ |
| 51 | UDS 列出组织成员（含 username） | UDS | success | ✅ |
| 52 | UDS 获取用户所属组织 | UDS | success | ✅ |
| 53 | UDS 移除组织成员 | UDS | success | ✅ |
| 54 | UDS 创建群组 | UDS | success | ✅ |
| 55 | UDS 获取群组 | UDS | success | ✅ |
| 56 | UDS 列出群组 | UDS | success | ✅ |
| 57 | UDS 添加群组成员 | UDS | success | ✅ |
| 58 | UDS 列出群组成员（含 username） | UDS | success | ✅ |
| 59 | UDS 获取用户所属群组 | UDS | success | ✅ |
| 60 | UDS 移除群组成员 | UDS | success | ✅ |
| 61 | UDS 未知动作 | UDS | error | ✅ |

**手动测试：61 项，全部通过 ✅**

---

## 运行测试

### 运行全部自动化测试

```bash
go test ./...
```

### 带覆盖率报告

```bash
go test ./... -coverprofile=coverage.out
go tool cover -html=coverage.out -o coverage.html
```

### 运行指定包测试

```bash
go test ./internal/auth/...          # 认证服务
go test ./internal/user/...          # 用户存储
go test ./internal/directory/...     # 目录存储
go test ./internal/api/...           # REST API 集成测试
go test ./internal/uds/...           # UDS 集成测试
```

### 运行指定测试函数

```bash
go test ./internal/api/... -run TestOrg_MemberManagement -v
go test ./internal/uds/... -run TestUDS_Group -v
```

### 手动功能测试（需先启动服务）

```bash
# 启动服务
GIN_MODE=release go run ./cmd/main.go

# 获取 Token
TOKEN=$(curl -s -X POST http://localhost:8080/api/v1/auth/login \
  -H "Content-Type: application/json" \
  -d '{"username":"admin","password":"Admin@123456"}' \
  | python3 -c "import sys,json; print(json.load(sys.stdin)['token'])")

# UDS 辅助函数
uds_req() {
  (echo '{"version":"1.0","format":"json"}'; sleep 0.1; echo "$1"; sleep 0.3) \
    | nc -U /tmp/idp-uds.sock 2>/dev/null | tail -1
}
```
