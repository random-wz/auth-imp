# IDP 服务（身份目录提供商）

基于 Go + Gin 实现的轻量级身份目录服务，支持本地用户目录管理、认证（bcrypt + JWT）和 Unix Domain Socket（UDS）高性能通信，UDS 同时支持 JSON 与 Protobuf 两种消息格式。

---

## 目录

- [项目结构](#项目结构)
- [快速开始](#快速开始)
- [REST API](#rest-api)
  - [认证接口](#认证接口)
  - [用户管理接口](#用户管理接口)
  - [同步任务接口](#同步任务接口)
  - [SCIM 2.0 接口](#scim-20-接口)
- [UDS 通信协议](#uds-通信协议)
  - [握手与格式协商](#握手与格式协商)
  - [JSON 模式](#json-模式)
  - [Protobuf 模式](#protobuf-模式)
  - [支持的 Action](#支持的-action)
  - [客户端示例（Python）](#客户端示例python)
- [运行测试](#运行测试)
- [技术选型](#技术选型)

---

## 项目结构

```
auth-imp/
├── cmd/
│   └── main.go                 # 程序入口，启动 REST API 与 UDS 服务端
├── proto/
│   └── idp.proto               # Protobuf 协议定义文件（proto3）
├── pb/
│   └── idp.pb.go               # protoc 生成的 Go 代码
├── internal/
│   ├── model/
│   │   └── models.go           # 数据模型：User、Organization、SyncJob 等
│   ├── auth/
│   │   ├── service.go          # 认证服务：bcrypt 密码哈希 + JWT 令牌
│   │   └── service_test.go
│   ├── user/
│   │   ├── store.go            # 用户存储接口定义
│   │   ├── memory_store.go     # 内存存储实现（含 SyncJobStore）
│   │   └── memory_store_test.go
│   ├── uds/
│   │   ├── codec.go            # 编解码器：JSONCodec / ProtobufCodec
│   │   ├── handler.go          # UDS Action 处理器注册表
│   │   ├── server.go           # UDS 服务端：握手、并发、心跳、空闲检测
│   │   └── server_test.go
│   └── api/
│       ├── handler.go          # REST API 处理器（Gin Handler）
│       ├── router.go           # Gin 路由配置 + JWT 中间件
│       └── handler_test.go
└── go.mod
```

---

## 快速开始

### 环境要求

- Go 1.21+
- protoc（仅重新生成 .proto 时需要）

### 编译与运行

```bash
# 克隆或进入项目目录
cd auth-imp

# 安装依赖
go mod download

# 编译
go build -o bin/idp-server ./cmd/main.go

# 运行（使用 release 模式减少日志噪音）
GIN_MODE=release ./bin/idp-server
```

启动后输出：

```
2026/03/25 10:40:17 [Bootstrap] Admin user created: username=admin password=Admin@123456
2026/03/25 10:40:17 [UDS] Server started on /tmp/idp-uds.sock
2026/03/25 10:40:17 REST API server starting on :8080
```

### 默认配置

| 配置项 | 默认值 |
|--------|--------|
| REST API 监听地址 | `:8080` |
| UDS Socket 路径 | `/tmp/idp-uds.sock` |
| UDS 最大连接数 | `100` |
| JWT Secret | `dev-secret-key-change-in-production` |
| JWT 过期时间 | `24h` |
| 握手超时 | `5s` |
| 空闲连接超时 | `60s` |
| 初始管理员账号 | `admin` / `Admin@123456` |

> **生产环境**请修改 `cmd/main.go` 中的 `JWTSecret`，并使用持久化存储替换内存存储。

---

## 数据库支持

项目支持两种存储方式：

### 内存存储（默认）

适用于开发和测试环境，数据不持久化。

```bash
./bin/idp-server
```

### PostgreSQL 存储

适用于生产环境，数据持久化到 PostgreSQL。

**1. 初始化数据库**

```bash
# 创建数据库
createdb idp_service

# 执行初始化脚本
psql idp_service < migrations/001_init.sql
```

**2. 配置环境变量**

```bash
export USE_PG=true
export DATABASE_URL="postgres://user:password@localhost:5432/idp_service?sslmode=disable"
```

**3. 启动服务**

```bash
./bin/idp-server
```

---

## CLI 工具使用

项目提供 `idpctl` 命令行工具用于管理 IDP 服务。

### 构建

```bash
make build
# 或
go build -o idpctl ./cmd/idpctl
```

### 认证

```bash
# 登录获取 token（REST API）
./idpctl auth login admin Admin@123456

# 使用 UDS 登录
./idpctl --use-uds auth login admin Admin@123456
```

### 用户管理

```bash
# 列出用户（支持分页）
./idpctl --token <TOKEN> user list --page 1 --page-size 10

# 创建用户
./idpctl --token <TOKEN> user create alice alice@123 alice@example.com

# 获取用户信息
./idpctl --token <TOKEN> user get alice

# 更新用户
./idpctl --token <TOKEN> user update <user_id> '{"display_name":"Alice Updated"}'

# 删除用户
./idpctl --token <TOKEN> user delete <user_id>
```

### 组织管理

```bash
# 创建组织
./idpctl --token <TOKEN> org create '{"name":"Engineering"}'

# 列出组织
./idpctl --token <TOKEN> org list

# 添加成员
./idpctl --token <TOKEN> org add-member <org_id> '{"user_id":"<user_id>","role":"member"}'
```

### 群组管理

```bash
# 创建群组
./idpctl --token <TOKEN> group create '{"name":"Developers","description":"Dev team"}'

# 列出群组
./idpctl --token <TOKEN> group list

# 添加成员
./idpctl --token <TOKEN> group add-member <group_id> '{"user_id":"<user_id>"}'
```

---

## REST API

所有需要认证的接口须在请求头中携带 JWT Token：

```
Authorization: Bearer <token>
```

### 认证接口

#### POST /api/v1/auth/login

用户登录，返回 JWT Token。无需认证。

**请求体：**
```json
{
  "username": "admin",
  "password": "Admin@123456"
}
```

**响应示例：**
```json
{
  "token": "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...",
  "expires": 1774492826,
  "user": {
    "id": "99e81ac3-c5da-41b5-a054-3d8287b59e2d",
    "username": "admin",
    "email": "admin@example.com",
    "display_name": "System Administrator",
    "status": "active",
    "created_at": "2026-03-25T10:40:17.045069576+08:00",
    "updated_at": "2026-03-25T10:40:17.045069576+08:00"
  }
}
```

**错误响应：**
```json
{ "error": "invalid credentials" }
{ "error": "user is disabled" }
```

---

### 用户管理接口

所有用户接口均需 JWT 认证。

#### POST /api/v1/users — 创建用户

**请求体：**
```json
{
  "username": "alice",
  "email": "alice@example.com",
  "password": "Alice@8888",
  "display_name": "Alice Wang"
}
```

字段校验：`username` 长度 3~64，`email` 合法邮箱，`password` 最少 8 字符。

**响应（201 Created）：**
```json
{
  "id": "7e179c4b-7885-40c4-a1e5-e71e64766f22",
  "username": "alice",
  "email": "alice@example.com",
  "display_name": "Alice Wang",
  "status": "active",
  "created_at": "2026-03-25T10:40:36.549131125+08:00",
  "updated_at": "2026-03-25T10:40:36.549131125+08:00"
}
```

> `password_hash` 字段不会在任何响应中暴露。

**错误响应：**
```json
{ "error": "user already exists" }   // 409 Conflict
```

---

#### GET /api/v1/users — 列出用户

支持分页与关键字搜索：

| 参数 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| `offset` | int | 0 | 偏移量 |
| `limit` | int | 20 | 每页数量（最大 100） |
| `q` | string | - | 搜索关键字（匹配 username/email/display_name） |

**响应：**
```json
{
  "total": 4,
  "offset": 0,
  "limit": 20,
  "items": [...]
}
```

**示例：**
```bash
# 列出全部用户
curl http://localhost:8080/api/v1/users -H "Authorization: Bearer $TOKEN"

# 搜索用户
curl "http://localhost:8080/api/v1/users?q=alice" -H "Authorization: Bearer $TOKEN"

# 分页
curl "http://localhost:8080/api/v1/users?limit=2&offset=0" -H "Authorization: Bearer $TOKEN"
```

---

#### GET /api/v1/users/:id — 获取用户详情

```bash
curl http://localhost:8080/api/v1/users/7e179c4b-7885-40c4-a1e5-e71e64766f22 \
  -H "Authorization: Bearer $TOKEN"
```

**错误响应：**
```json
{ "error": "user not found" }   // 404
```

---

#### PUT /api/v1/users/:id — 更新用户

可更新 `email`、`display_name`、`status`（active/disabled）。

**请求体（所有字段均为可选）：**
```json
{
  "display_name": "Alice Wang (Updated)",
  "email": "alice_new@example.com",
  "status": "disabled"
}
```

**禁用用户后再登录：**
```json
{ "error": "user is disabled" }
```

---

#### DELETE /api/v1/users/:id — 删除用户（软删除）

```bash
curl -X DELETE http://localhost:8080/api/v1/users/<id> \
  -H "Authorization: Bearer $TOKEN"
```

**响应：** `204 No Content`

---

### 同步任务接口

#### POST /api/v1/sync/jobs — 创建同步任务

**请求体：**
```json
{
  "source_system": "ldap",
  "sync_type": "incremental"
}
```

`sync_type` 可选值：`full`（全量）/ `incremental`（增量）

**响应（201 Created）：**
```json
{
  "id": "9d22419a-a4d6-41ff-afb3-e50c9daaf57d",
  "source_system": "ldap",
  "sync_type": "incremental",
  "status": "pending",
  "started_at": "2026-03-25T10:42:05.535024889+08:00",
  "created_at": "2026-03-25T10:42:05.53502999+08:00",
  "updated_at": "2026-03-25T10:42:05.53502999+08:00"
}
```

#### GET /api/v1/sync/jobs/:id — 查询同步任务状态

```bash
curl http://localhost:8080/api/v1/sync/jobs/9d22419a-a4d6-41ff-afb3-e50c9daaf57d \
  -H "Authorization: Bearer $TOKEN"
```

任务状态（`status`）：`pending` / `running` / `done` / `failed` / `canceled`

---

### SCIM 2.0 接口

兼容 SCIM 2.0 标准，供第三方系统集成。所有接口需 JWT 认证。

| 端点 | 方法 | 描述 |
|------|------|------|
| `/scim/v2/Users` | GET | 获取用户列表（支持分页、搜索） |
| `/scim/v2/Users/:id` | GET | 获取单个用户 |
| `/scim/v2/Users` | POST | 创建用户 |
| `/scim/v2/Users/:id` | PUT | 更新用户 |
| `/scim/v2/Users/:id` | DELETE | 删除用户 |

---

## UDS 通信协议

UDS（Unix Domain Socket）用于与 C 语言模块或本地高性能服务集成，Socket 路径默认为 `/tmp/idp-uds.sock`。

### 握手与格式协商

每次建立连接后，客户端**必须首先发送握手消息**声明消息格式。

#### 服务端格式自动检测

- 首字节为 `{`（0x7B）→ 识别为 **JSON 模式**
- 首字节为非 `{`（通常为小数字节）→ 识别为 **Protobuf 模式**（4 字节小端序长度前缀）

#### JSON 握手

```json
{"version": "1.0", "format": "json"}
```

握手响应（换行符 `\n` 结尾）：
```json
{"status": "ok", "version": "1.0", "format": "json"}
```

#### Protobuf 握手

```
[4字节小端序长度] + [JSON消息体]
```

握手消息体：
```json
{"version": "1.0", "format": "protobuf"}
```

握手响应同样使用长度前缀帧。

**握手失败响应：**
```json
{"status": "error", "version": "", "format": "", "error_message": "unsupported format: xxx"}
```

---

### JSON 模式

**消息分隔符：** `\n`（每条消息一行）

**请求格式：**
```json
{
  "action": "<action_name>",
  "request_id": "<唯一请求ID>",
  "payload": { ... }
}
```

**响应格式：**
```json
{
  "status": "success" | "error" | "pong",
  "request_id": "<与请求相同>",
  "payload": { ... },
  "error": "<错误信息，仅 status=error 时存在>"
}
```

---

### Protobuf 模式

**消息分隔：** 4 字节小端序长度前缀 + 消息体

```
+---------+---------+
| 4 bytes | N bytes |
| length  | payload |
| LE uint32         |
+---------+---------+
```

> 当前实现的消息体使用 JSON 编码（与 JSON 模式内容相同），framing 层完整实现 Protobuf 帧协议。Protobuf 二进制序列化在 `proto/idp.proto` 中定义，可通过替换 Codec 升级。

---

### 支持的 Action

| Action | 描述 | 请求 Payload | 响应 Payload |
|--------|------|-------------|-------------|
| `auth` | 用户认证（验证用户名+密码） | `{username, password}` | `{user_id, username, email, display_name}` |
| `get_user` | 获取用户信息 | `{user_id?} 或 {username?}` | `{user_id, username, email, display_name, organizations}` |
| `trigger_sync` | 触发目录同步任务 | `{source_system, sync_type}` | `{job_id, estimated_duration, message}` |
| `ping` | 心跳检测 | `{}` | `{timestamp}` |

---

### 客户端示例（Python）

#### JSON 模式完整示例

```python
import socket
import json

def send_line(sock, data):
    sock.sendall(json.dumps(data).encode() + b'\n')

def recv_line(sock):
    buf = b""
    while True:
        c = sock.recv(1)
        if not c or c == b'\n':
            break
        buf += c
    return json.loads(buf)

s = socket.socket(socket.AF_UNIX, socket.SOCK_STREAM)
s.connect("/tmp/idp-uds.sock")
s.settimeout(5)

# 1. 握手
send_line(s, {"version": "1.0", "format": "json"})
resp = recv_line(s)
assert resp["status"] == "ok", f"Handshake failed: {resp}"

# 2. 用户认证
send_line(s, {
    "action": "auth",
    "request_id": "req-001",
    "payload": {"username": "admin", "password": "Admin@123456"}
})
resp = recv_line(s)
print(f"Auth: {resp['status']}, user_id={resp['payload']['user_id']}")

# 3. 获取用户信息
send_line(s, {
    "action": "get_user",
    "request_id": "req-002",
    "payload": {"username": "admin"}
})
resp = recv_line(s)
print(f"User: {resp['payload']['display_name']}")

# 4. 触发同步
send_line(s, {
    "action": "trigger_sync",
    "request_id": "req-003",
    "payload": {"source_system": "ldap", "sync_type": "incremental"}
})
resp = recv_line(s)
print(f"Sync job: {resp['payload']['job_id']}")

# 5. 心跳
send_line(s, {"action": "ping", "request_id": "ping-001", "payload": {}})
resp = recv_line(s)
print(f"Pong: timestamp={resp['payload']['timestamp']}")

s.close()
```

#### Protobuf 帧模式示例

```python
import socket
import json
import struct

def send_frame(sock, data):
    encoded = json.dumps(data).encode()
    sock.sendall(struct.pack('<I', len(encoded)) + encoded)

def recv_frame(sock):
    length_bytes = b""
    while len(length_bytes) < 4:
        length_bytes += sock.recv(4 - len(length_bytes))
    length = struct.unpack('<I', length_bytes)[0]
    data = b""
    while len(data) < length:
        data += sock.recv(length - len(data))
    return json.loads(data)

s = socket.socket(socket.AF_UNIX, socket.SOCK_STREAM)
s.connect("/tmp/idp-uds.sock")
s.settimeout(5)

# 握手（Protobuf 模式）
send_frame(s, {"version": "1.0", "format": "protobuf"})
resp = recv_frame(s)
assert resp["status"] == "ok"

# 认证
send_frame(s, {
    "action": "auth",
    "request_id": "req-001",
    "payload": {"username": "admin", "password": "Admin@123456"}
})
resp = recv_frame(s)
print(f"Auth OK: {resp['payload']['username']}")

s.close()
```

---

## 运行测试

```bash
# 运行所有测试
go test ./...

# 带详细输出
go test ./... -v

# 运行指定包的测试
go test ./internal/auth/... -v       # 认证服务测试
go test ./internal/user/... -v       # 用户存储测试
go test ./internal/uds/... -v        # UDS 服务端测试
go test ./internal/api/... -v        # REST API 测试

# 查看测试覆盖率
go test ./... -cover
```

**测试结果：**

```
ok  github.com/idp-service/internal/api    (13 个测试)
ok  github.com/idp-service/internal/auth   (8 个测试)
ok  github.com/idp-service/internal/uds   (14 个测试)
ok  github.com/idp-service/internal/user  (11 个测试)
```

---

## 技术选型

| 组件 | 技术 |
|------|------|
| Web 框架 | [Gin](https://github.com/gin-gonic/gin) v1.9.1 |
| 密码加密 | `golang.org/x/crypto/bcrypt`（cost=10） |
| JWT | [golang-jwt/jwt](https://github.com/golang-jwt/jwt) v5 |
| UUID | [google/uuid](https://github.com/google/uuid) |
| Protobuf | [google.golang.org/protobuf](https://google.golang.org/protobuf) v1.36 |
| UDS 通信 | Go 标准库 `net` 包 |

---

## 功能测试覆盖

| 功能点 | 状态 |
|--------|------|
| 健康检查 `/health` | ✓ |
| 管理员 bootstrap 初始化 | ✓ |
| 用户登录（正确/错误密码/不存在用户） | ✓ |
| JWT Token 生成与验证 | ✓ |
| 无 Token / 无效 Token 拒绝访问 | ✓ |
| 创建用户（含参数校验、重复冲突检测） | ✓ |
| 查询用户（ID/用户名） | ✓ |
| 更新用户（邮箱、昵称、状态） | ✓ |
| 禁用用户后登录被拒 | ✓ |
| 软删除用户 | ✓ |
| 用户列表（分页 + 关键字搜索） | ✓ |
| 创建同步任务（LDAP/SCIM、全量/增量） | ✓ |
| 查询同步任务状态 | ✓ |
| SCIM 2.0 端点 | ✓ |
| UDS JSON 模式握手 | ✓ |
| UDS Protobuf 帧模式握手 | ✓ |
| UDS auth/get_user/trigger_sync/ping | ✓ |
| UDS 并发连接（10 并发全部成功） | ✓ |
| UDS 错误 action 处理 | ✓ |
