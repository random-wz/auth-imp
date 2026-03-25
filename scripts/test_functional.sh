#!/usr/bin/env bash
# =============================================================================
# IDP Service 功能自动化测试脚本
# 用法: ./scripts/test_functional.sh [--no-unit] [--no-rest] [--no-uds]
# =============================================================================
set -euo pipefail

# ── 颜色 ──────────────────────────────────────────────────────────────────────
RED='\033[0;31m'; GREEN='\033[0;32m'; YELLOW='\033[1;33m'
CYAN='\033[0;36m'; BOLD='\033[1m'; RESET='\033[0m'

# ── 参数 ──────────────────────────────────────────────────────────────────────
RUN_UNIT=true; RUN_REST=true; RUN_UDS=true
for arg in "$@"; do
  case $arg in
    --no-unit) RUN_UNIT=false ;;
    --no-rest) RUN_REST=false ;;
    --no-uds)  RUN_UDS=false  ;;
  esac
done

# ── 配置 ──────────────────────────────────────────────────────────────────────
BASE_URL="http://localhost:8080/api/v1"
HEALTH_URL="http://localhost:8080/health"
UDS_SOCK="/tmp/idp-uds.sock"
SERVER_BIN="/tmp/idp-test-server"
SERVER_LOG="/tmp/idp-test-server.log"
SERVER_PID=""

# ── 计数 ──────────────────────────────────────────────────────────────────────
TOTAL=0; PASSED=0; FAILED=0
FAILED_CASES=()

# ── 工具函数 ──────────────────────────────────────────────────────────────────
pass() { echo -e "  ${GREEN}✔ PASS${RESET}  $1"; PASSED=$((PASSED+1)); TOTAL=$((TOTAL+1)); }
fail() { echo -e "  ${RED}✘ FAIL${RESET}  $1${RED} — $2${RESET}"; FAILED=$((FAILED+1)); TOTAL=$((TOTAL+1)); FAILED_CASES+=("$1: $2"); }

section() { echo -e "\n${CYAN}${BOLD}▶ $1${RESET}"; }

# 检查 HTTP 状态码
check_status() {
  local name="$1" expected="$2" actual="$3"
  if [[ "$actual" == "$expected" ]]; then
    pass "$name (HTTP $actual)"
  else
    fail "$name" "期望 HTTP $expected，实际 HTTP $actual"
  fi
}

# 检查 JSON 字段值
check_field() {
  local name="$1" body="$2" field="$3" expected="$4"
  local actual
  actual=$(echo "$body" | python3 -c "import sys,json; d=json.load(sys.stdin); print(d$field)" 2>/dev/null || echo "__ERROR__")
  if [[ "$actual" == "$expected" ]]; then
    pass "$name ($field = $expected)"
  else
    fail "$name" "$field 期望 '$expected'，实际 '$actual'"
  fi
}

# 检查 JSON 字段存在且非空
check_nonempty() {
  local name="$1" body="$2" field="$3"
  local actual
  actual=$(echo "$body" | python3 -c "import sys,json; d=json.load(sys.stdin); print(d$field)" 2>/dev/null || echo "")
  if [[ -n "$actual" && "$actual" != "None" && "$actual" != "__ERROR__" ]]; then
    pass "$name ($field 非空)"
  else
    fail "$name" "$field 为空或不存在"
  fi
}

# 检查 JSON 字段不存在或为空（敏感字段不外露）
check_absent() {
  local name="$1" body="$2" field="$3"
  local actual
  actual=$(echo "$body" | python3 -c "import sys,json; d=json.load(sys.stdin); print(d$field)" 2>/dev/null || echo "")
  if [[ -z "$actual" || "$actual" == "None" ]]; then
    pass "$name ($field 未暴露)"
  else
    fail "$name" "$field 不应出现在响应中，实际值: '$actual'"
  fi
}

# REST 请求
rest() {
  local method="$1" path="$2"; shift 2
  curl -s -o /tmp/rest_body.txt -w "%{http_code}" \
    -X "$method" "$BASE_URL$path" \
    -H "Content-Type: application/json" \
    "$@"
}

rest_auth() {
  local method="$1" path="$2"; shift 2
  curl -s -o /tmp/rest_body.txt -w "%{http_code}" \
    -X "$method" "$BASE_URL$path" \
    -H "Content-Type: application/json" \
    -H "Authorization: Bearer $TOKEN" \
    "$@"
}

body() { cat /tmp/rest_body.txt; }

# UDS 请求（JSON 模式：握手 + 业务请求，取最后一行响应）
uds_req() {
  local req="$1"
  (printf '%s\n' '{"version":"1.0","format":"json"}'; sleep 0.05; printf '%s\n' "$req"; sleep 0.2) \
    | nc -U "$UDS_SOCK" 2>/dev/null | grep -v '^$' | tail -1
}

check_uds() {
  local name="$1" resp="$2" expected_status="$3"
  local actual_status
  actual_status=$(echo "$resp" | python3 -c "import sys,json; print(json.load(sys.stdin).get('status',''))" 2>/dev/null || echo "")
  if [[ "$actual_status" == "$expected_status" ]]; then
    pass "$name (status=$expected_status)"
  else
    fail "$name" "期望 status='$expected_status'，实际 '$actual_status'（响应: $resp）"
  fi
}

check_uds_field() {
  local name="$1" resp="$2" field="$3" expected="$4"
  local actual
  actual=$(echo "$resp" | python3 -c "import sys,json; d=json.load(sys.stdin); print(d$field)" 2>/dev/null || echo "__ERROR__")
  if [[ "$actual" == "$expected" ]]; then
    pass "$name ($field = $expected)"
  else
    fail "$name" "$field 期望 '$expected'，实际 '$actual'"
  fi
}

# ── 服务器管理 ────────────────────────────────────────────────────────────────
build_server() {
  echo -e "${BOLD}▷ 编译服务器...${RESET}"
  if ! go build -o "$SERVER_BIN" ./cmd/main.go 2>/tmp/build.log; then
    echo -e "${RED}编译失败:${RESET}"; cat /tmp/build.log; exit 1
  fi
  echo -e "  编译完成: $SERVER_BIN"
}

start_server() {
  echo -e "${BOLD}▷ 启动服务器...${RESET}"
  fuser -k 8080/tcp 2>/dev/null || true
  rm -f "$UDS_SOCK"
  GIN_MODE=release "$SERVER_BIN" > "$SERVER_LOG" 2>&1 &
  SERVER_PID=$!
  # 等待 HTTP 端口就绪
  local max=30 i=0
  while ! curl -sf "$HEALTH_URL" >/dev/null 2>&1; do
    sleep 0.2; i=$((i+1))
    if [ "$i" -ge "$max" ]; then
      echo -e "${RED}服务器启动超时，日志:${RESET}"; cat "$SERVER_LOG"; exit 1
    fi
  done
  echo -e "  服务器就绪 (PID=$SERVER_PID)"
}

stop_server() {
  if [[ -n "$SERVER_PID" ]]; then
    kill "$SERVER_PID" 2>/dev/null || true
    wait "$SERVER_PID" 2>/dev/null || true
    SERVER_PID=""
  fi
  rm -f "$SERVER_BIN" "$SERVER_LOG" /tmp/rest_body.txt
}

# ── 初始化 Token 和公共变量 ───────────────────────────────────────────────────
TOKEN=""
ALICE_ID=""; BOB_ID=""; ORG_ID=""; CHILD_ORG_ID=""; GROUP_ID=""; JOB_ID=""

init_token() {
  local resp
  resp=$(curl -s -X POST "$BASE_URL/auth/login" \
    -H "Content-Type: application/json" \
    -d '{"username":"admin","password":"Admin@123456"}')
  TOKEN=$(echo "$resp" | python3 -c "import sys,json; print(json.load(sys.stdin)['token'])" 2>/dev/null)
  if [[ -z "$TOKEN" ]]; then
    echo -e "${RED}获取 Token 失败，终止测试${RESET}"; stop_server; exit 1
  fi
}

# =============================================================================
# 单元测试 / 集成测试
# =============================================================================
run_unit_tests() {
  section "单元测试 & 集成测试 (go test)"
  local output exit_code=0
  output=$(go test ./... -count=1 2>&1) || exit_code=$?

  # 逐包解析
  while IFS= read -r line; do
    if [[ "$line" =~ ^ok[[:space:]]+(.*)[[:space:]]+([0-9.]+s)$ ]]; then
      pkg="${BASH_REMATCH[1]}"
      dur="${BASH_REMATCH[2]}"
      pass "${pkg} (${dur})"
    elif [[ "$line" =~ ^FAIL[[:space:]]+(.*) ]]; then
      pkg="${BASH_REMATCH[1]}"
      fail "$pkg" "go test 报告失败"
    fi
  done <<< "$output"

  # 输出失败详情
  if [ "$exit_code" -ne 0 ]; then
    echo -e "\n${RED}失败详情:${RESET}"
    echo "$output" | grep -A5 "FAIL\|Error\|panic" || true
  fi
}

# =============================================================================
# REST API 功能测试
# =============================================================================
run_rest_tests() {
  section "REST API 功能测试"
  init_token

  # ── 1. 健康检查 ──────────────────────────────────────────────────────────
  echo -e "\n  ${BOLD}[健康检查]${RESET}"
  code=$(curl -s -o /tmp/rest_body.txt -w "%{http_code}" "$HEALTH_URL")
  check_status "健康检查" "200" "$code"
  check_field  "健康检查响应体" "$(body)" "['status']" "ok"

  # ── 2. 认证 ──────────────────────────────────────────────────────────────
  echo -e "\n  ${BOLD}[认证]${RESET}"

  code=$(rest POST /auth/login -d '{"username":"admin","password":"Admin@123456"}')
  check_status "登录成功" "200" "$code"
  check_nonempty "登录返回 token" "$(body)" "['token']"
  check_nonempty "登录返回用户信息" "$(body)" "['user']['id']"
  check_absent   "密码哈希不外露" "$(body)" ".get('user',{}).get('password_hash','')"

  code=$(rest POST /auth/login -d '{"username":"admin","password":"wrongpass"}')
  check_status "登录失败-密码错误" "401" "$code"

  code=$(rest POST /auth/login -d '{"username":"nobody","password":"x"}')
  check_status "登录失败-用户不存在" "401" "$code"

  code=$(curl -s -o /tmp/rest_body.txt -w "%{http_code}" "$BASE_URL/users")
  check_status "无 Token 访问" "401" "$code"

  code=$(curl -s -o /tmp/rest_body.txt -w "%{http_code}" "$BASE_URL/users" \
    -H "Authorization: Bearer invalid.token.xxx")
  check_status "无效 Token 访问" "401" "$code"

  # ── 3. 用户管理 ──────────────────────────────────────────────────────────
  echo -e "\n  ${BOLD}[用户管理]${RESET}"

  code=$(rest_auth POST /users -d '{"username":"alice","email":"alice@example.com","password":"Alice@123456","display_name":"Alice Wang"}')
  check_status "创建用户 alice" "201" "$code"
  check_field  "用户名正确" "$(body)" "['username']" "alice"
  check_field  "初始状态 active" "$(body)" "['status']" "active"
  check_absent "密码哈希不外露" "$(body)" ".get('password_hash','')"
  ALICE_ID=$(body | python3 -c "import sys,json; print(json.load(sys.stdin)['id'])" 2>/dev/null)

  code=$(rest_auth POST /users -d '{"username":"alice","email":"alice2@example.com","password":"Alice@123456"}')
  check_status "创建用户-用户名重复" "409" "$code"

  code=$(rest_auth POST /users -d '{"username":"bob","email":"bob@example.com","password":"Bob@123456","display_name":"Bob Li"}')
  check_status "创建用户 bob" "201" "$code"
  BOB_ID=$(body | python3 -c "import sys,json; print(json.load(sys.stdin)['id'])" 2>/dev/null)

  code=$(rest_auth GET /users/"$ALICE_ID")
  check_status "获取用户" "200" "$code"
  check_field  "获取用户用户名" "$(body)" "['username']" "alice"

  code=$(rest_auth GET /users/non-existent-id-xyz)
  check_status "获取用户-不存在" "404" "$code"

  code=$(rest_auth PUT /users/"$ALICE_ID" -d '{"display_name":"Alice Chen"}')
  check_status "更新用户" "200" "$code"
  check_field  "更新后 display_name" "$(body)" "['display_name']" "Alice Chen"

  code=$(rest_auth GET "/users?offset=0&limit=10")
  check_status "列出用户" "200" "$code"
  local total
  total=$(body | python3 -c "import sys,json; print(json.load(sys.stdin)['total'])" 2>/dev/null)
  [[ "$total" -ge 3 ]] && pass "用户列表 total >= 3 (actual=$total)" || fail "用户列表 total 不足" "total=$total"

  code=$(rest_auth GET "/users?q=alice")
  check_status "搜索用户" "200" "$code"
  local search_total
  search_total=$(body | python3 -c "import sys,json; print(json.load(sys.stdin)['total'])" 2>/dev/null)
  [[ "$search_total" -ge 1 ]] && pass "搜索结果 >= 1 (actual=$search_total)" || fail "搜索无结果" "total=$search_total"

  # ── 4. 组织管理 ──────────────────────────────────────────────────────────
  echo -e "\n  ${BOLD}[组织管理]${RESET}"

  code=$(rest_auth POST /orgs -d '{"name":"Acme Corp"}')
  check_status "创建根组织" "201" "$code"
  check_field  "组织名称正确" "$(body)" "['name']" "Acme Corp"
  check_nonempty "组织 path 已生成" "$(body)" "['path']"
  ORG_ID=$(body | python3 -c "import sys,json; print(json.load(sys.stdin)['id'])" 2>/dev/null)

  code=$(rest_auth POST /orgs -d "{\"name\":\"Engineering\",\"parent_id\":\"$ORG_ID\"}")
  check_status "创建子组织" "201" "$code"
  check_field  "子组织 parent_id 正确" "$(body)" "['parent_id']" "$ORG_ID"
  local child_path
  child_path=$(body | python3 -c "import sys,json; print(json.load(sys.stdin)['path'])" 2>/dev/null)
  [[ "$child_path" == *"$ORG_ID"* ]] && pass "子组织 path 包含父 ID" || fail "子组织 path 不含父 ID" "path=$child_path"
  CHILD_ORG_ID=$(body | python3 -c "import sys,json; print(json.load(sys.stdin)['id'])" 2>/dev/null)

  code=$(rest_auth GET /orgs/"$ORG_ID"/children)
  check_status "列出子组织" "200" "$code"
  check_field  "子组织数量=1" "$(body)" "['total']" "1"

  code=$(rest_auth PUT /orgs/"$ORG_ID" -d '{"name":"Acme Corp Updated"}')
  check_status "更新组织" "200" "$code"
  check_field  "更新后名称" "$(body)" "['name']" "Acme Corp Updated"

  code=$(rest_auth GET /orgs/"$ORG_ID")
  check_status "获取组织" "200" "$code"

  code=$(rest_auth GET /orgs/no-such-org-id)
  check_status "获取组织-不存在" "404" "$code"

  code=$(rest_auth GET "/orgs?offset=0&limit=10")
  check_status "列出所有组织" "200" "$code"

  # ── 5. 组织成员管理 ───────────────────────────────────────────────────────
  echo -e "\n  ${BOLD}[组织成员管理]${RESET}"

  code=$(rest_auth POST /orgs/"$ORG_ID"/members -d "{\"user_id\":\"$ALICE_ID\",\"role\":\"member\"}")
  check_status "添加组织成员" "201" "$code"
  check_field  "成员 role 正确" "$(body)" "['role']" "member"

  code=$(rest_auth POST /orgs/"$ORG_ID"/members -d "{\"user_id\":\"$ALICE_ID\",\"role\":\"member\"}")
  check_status "添加成员-重复" "409" "$code"

  code=$(rest_auth POST /orgs/"$ORG_ID"/members -d '{"user_id":"ghost-user-id-xyz"}')
  check_status "添加成员-用户不存在" "404" "$code"

  code=$(rest_auth GET /orgs/"$ORG_ID"/members)
  check_status "列出组织成员" "200" "$code"
  check_field  "成员数量=1" "$(body)" "['total']" "1"
  local member_username
  member_username=$(body | python3 -c "import sys,json; d=json.load(sys.stdin); print(d['items'][0]['username'])" 2>/dev/null)
  [[ "$member_username" == "alice" ]] && pass "成员列表含用户信息 username=alice" || fail "成员列表用户信息缺失" "username=$member_username"

  code=$(rest_auth GET /users/"$ALICE_ID"/orgs)
  check_status "获取用户所属组织" "200" "$code"
  check_field  "用户组织数量=1" "$(body)" "['total']" "1"

  code=$(rest_auth DELETE /orgs/"$ORG_ID"/members/"$ALICE_ID")
  check_status "移除组织成员" "204" "$code"

  code=$(rest_auth GET /orgs/"$ORG_ID"/members)
  check_status "移除后成员列表" "200" "$code"
  check_field  "移除后成员数量=0" "$(body)" "['total']" "0"

  # ── 6. 群组管理 ──────────────────────────────────────────────────────────
  echo -e "\n  ${BOLD}[群组管理]${RESET}"

  code=$(rest_auth POST /groups -d '{"name":"backend-team","description":"Backend devs","type":"project"}')
  check_status "创建群组" "201" "$code"
  check_field  "群组名称正确" "$(body)" "['name']" "backend-team"
  check_field  "群组类型正确" "$(body)" "['type']" "project"
  GROUP_ID=$(body | python3 -c "import sys,json; print(json.load(sys.stdin)['id'])" 2>/dev/null)

  code=$(rest_auth GET /groups/"$GROUP_ID")
  check_status "获取群组" "200" "$code"

  code=$(rest_auth PUT /groups/"$GROUP_ID" -d '{"description":"Backend & API devs"}')
  check_status "更新群组" "200" "$code"
  check_field  "更新后描述" "$(body)" "['description']" "Backend & API devs"

  code=$(rest_auth GET /groups/no-such-group-id)
  check_status "获取群组-不存在" "404" "$code"

  code=$(rest_auth GET "/groups?offset=0&limit=10")
  check_status "列出所有群组" "200" "$code"

  # ── 7. 群组成员管理 ───────────────────────────────────────────────────────
  echo -e "\n  ${BOLD}[群组成员管理]${RESET}"

  code=$(rest_auth POST /groups/"$GROUP_ID"/members -d "{\"user_id\":\"$BOB_ID\",\"role\":\"member\"}")
  check_status "添加群组成员" "201" "$code"
  check_nonempty "joined_at 已记录" "$(body)" "['joined_at']"

  code=$(rest_auth POST /groups/"$GROUP_ID"/members -d "{\"user_id\":\"$BOB_ID\"}")
  check_status "添加成员-重复" "409" "$code"

  code=$(rest_auth POST /groups/"$GROUP_ID"/members -d '{"user_id":"ghost-user-id-xyz"}')
  check_status "添加成员-用户不存在" "404" "$code"

  code=$(rest_auth GET /groups/"$GROUP_ID"/members)
  check_status "列出群组成员" "200" "$code"
  check_field  "成员数量=1" "$(body)" "['total']" "1"
  local gm_username
  gm_username=$(body | python3 -c "import sys,json; d=json.load(sys.stdin); print(d['items'][0]['username'])" 2>/dev/null)
  [[ "$gm_username" == "bob" ]] && pass "群组成员列表含用户信息 username=bob" || fail "群组成员用户信息缺失" "username=$gm_username"

  code=$(rest_auth GET /users/"$BOB_ID"/groups)
  check_status "获取用户所属群组" "200" "$code"
  check_field  "用户群组数量=1" "$(body)" "['total']" "1"

  code=$(rest_auth DELETE /groups/"$GROUP_ID"/members/"$BOB_ID")
  check_status "移除群组成员" "204" "$code"

  code=$(rest_auth GET /groups/"$GROUP_ID"/members)
  check_status "移除后成员数量=0" "200" "$code"
  check_field  "成员数量清零" "$(body)" "['total']" "0"

  # ── 8. 同步任务 ──────────────────────────────────────────────────────────
  echo -e "\n  ${BOLD}[同步任务]${RESET}"

  code=$(rest_auth POST /sync/jobs -d '{"source_system":"ldap","sync_type":"full"}')
  check_status "创建同步任务" "201" "$code"
  check_field  "source_system 正确" "$(body)" "['source_system']" "ldap"
  check_field  "sync_type 正确" "$(body)" "['sync_type']" "full"
  check_field  "初始状态 pending" "$(body)" "['status']" "pending"
  check_nonempty "started_at 已记录" "$(body)" "['started_at']"
  JOB_ID=$(body | python3 -c "import sys,json; print(json.load(sys.stdin)['id'])" 2>/dev/null)

  code=$(rest_auth GET /sync/jobs/"$JOB_ID")
  check_status "获取同步任务" "200" "$code"

  code=$(rest_auth GET /sync/jobs/no-such-job-id)
  check_status "获取同步任务-不存在" "404" "$code"

  # ── 9. 删除用户 ──────────────────────────────────────────────────────────
  echo -e "\n  ${BOLD}[删除用户]${RESET}"

  code=$(rest_auth POST /users -d '{"username":"temp_del","email":"tmp@example.com","password":"Tmp@123456"}')
  local tmp_id
  tmp_id=$(body | python3 -c "import sys,json; print(json.load(sys.stdin)['id'])" 2>/dev/null)

  code=$(rest_auth DELETE /users/"$tmp_id")
  check_status "删除用户" "204" "$code"

  code=$(rest_auth GET /users/"$tmp_id")
  check_status "删除后不可访问" "404" "$code"

  # ── 10. 删除组织（级联清理验证） ─────────────────────────────────────────
  echo -e "\n  ${BOLD}[删除组织/群组]${RESET}"

  # 先把 alice 加回去再测级联删除
  rest_auth POST /orgs/"$ORG_ID"/members \
    -d "{\"user_id\":\"$ALICE_ID\"}" > /dev/null 2>&1 || true

  code=$(rest_auth DELETE /orgs/"$CHILD_ORG_ID")
  check_status "删除子组织" "204" "$code"

  code=$(rest_auth DELETE /orgs/"$ORG_ID")
  check_status "删除根组织" "204" "$code"

  code=$(rest_auth DELETE /groups/"$GROUP_ID")
  check_status "删除群组" "204" "$code"

  code=$(rest_auth GET /orgs/"$ORG_ID")
  check_status "删除后组织不可访问" "404" "$code"
}

# =============================================================================
# UDS 功能测试
# =============================================================================
run_uds_tests() {
  section "UDS 功能测试"

  # 等待 UDS Socket 就绪
  local i=0
  while [[ ! -S "$UDS_SOCK" ]]; do
    sleep 0.1; i=$((i+1))
    if [ "$i" -ge 50 ]; then echo -e "${RED}UDS Socket 未就绪${RESET}"; return 1; fi
  done

  # 取 admin 用户 ID
  local admin_resp
  admin_resp=$(curl -s -X POST "$BASE_URL/auth/login" \
    -H "Content-Type: application/json" \
    -d '{"username":"admin","password":"Admin@123456"}')
  local ADMIN_ID
  ADMIN_ID=$(echo "$admin_resp" | python3 -c "import sys,json; print(json.load(sys.stdin)['user']['id'])" 2>/dev/null)

  local resp

  # ── Ping ────────────────────────────────────────────────────────────────
  echo -e "\n  ${BOLD}[基础通信]${RESET}"
  resp=$(uds_req '{"action":"ping","request_id":"p1","payload":{}}')
  check_uds "Ping" "$resp" "pong"

  resp=$(uds_req '{"action":"no_such_action","request_id":"p2","payload":{}}')
  check_uds "未知动作返回 error" "$resp" "error"

  # ── 认证 ─────────────────────────────────────────────────────────────────
  echo -e "\n  ${BOLD}[认证]${RESET}"
  resp=$(uds_req '{"action":"auth","request_id":"a1","payload":{"username":"admin","password":"Admin@123456"}}')
  check_uds       "认证成功" "$resp" "success"
  check_uds_field "认证返回 username" "$resp" "['payload']['username']" "admin"
  local has_token
  has_token=$(echo "$resp" | python3 -c "import sys,json; d=json.load(sys.stdin); print('token' in d.get('payload',{}))" 2>/dev/null)
  [[ "$has_token" == "False" ]] && pass "UDS 认证不返回 JWT token" || fail "UDS 认证不应返回 JWT token" "payload contains token"

  resp=$(uds_req '{"action":"auth","request_id":"a2","payload":{"username":"admin","password":"wrongpass"}}')
  check_uds "认证失败-密码错误" "$resp" "error"

  resp=$(uds_req '{"action":"auth","request_id":"a3","payload":{"username":"nobody","password":"x"}}')
  check_uds "认证失败-用户不存在" "$resp" "error"

  resp=$(uds_req '{"action":"auth","request_id":"a4","payload":{}}')
  check_uds "认证失败-缺少参数" "$resp" "error"

  # ── 获取用户 ─────────────────────────────────────────────────────────────
  echo -e "\n  ${BOLD}[用户查询]${RESET}"
  resp=$(uds_req '{"action":"get_user","request_id":"u1","payload":{"username":"admin"}}')
  check_uds       "按用户名获取用户" "$resp" "success"
  check_uds_field "返回 username" "$resp" "['payload']['username']" "admin"

  resp=$(uds_req "{\"action\":\"get_user\",\"request_id\":\"u2\",\"payload\":{\"user_id\":\"$ADMIN_ID\"}}")
  check_uds "按 user_id 获取用户" "$resp" "success"

  resp=$(uds_req '{"action":"get_user","request_id":"u3","payload":{"username":"nobody"}}')
  check_uds "获取用户-不存在" "$resp" "error"

  resp=$(uds_req '{"action":"get_user","request_id":"u4","payload":{}}')
  check_uds "获取用户-缺少参数" "$resp" "error"

  # ── 触发同步 ─────────────────────────────────────────────────────────────
  echo -e "\n  ${BOLD}[同步任务]${RESET}"
  resp=$(uds_req '{"action":"trigger_sync","request_id":"s1","payload":{"source_system":"ldap","sync_type":"incremental"}}')
  check_uds       "触发同步" "$resp" "success"
  check_uds_field "返回 job_id" "$resp" "['payload']['job_id']" "$(echo "$resp" | python3 -c "import sys,json; print(json.load(sys.stdin)['payload']['job_id'])" 2>/dev/null)"
  check_uds_field "message 正确" "$resp" "['payload']['message']" "Sync job started"

  resp=$(uds_req '{"action":"trigger_sync","request_id":"s2","payload":{}}')
  check_uds "触发同步-缺少 source_system" "$resp" "error"

  # ── 组织管理 ─────────────────────────────────────────────────────────────
  echo -e "\n  ${BOLD}[组织管理]${RESET}"
  resp=$(uds_req '{"action":"create_org","request_id":"o1","payload":{"name":"UDS-RootOrg"}}')
  check_uds       "创建根组织" "$resp" "success"
  check_uds_field "组织名称正确" "$resp" "['payload']['name']" "UDS-RootOrg"
  local uorg_id
  uorg_id=$(echo "$resp" | python3 -c "import sys,json; print(json.load(sys.stdin)['payload']['org_id'])" 2>/dev/null)
  local uorg_path
  uorg_path=$(echo "$resp" | python3 -c "import sys,json; print(json.load(sys.stdin)['payload']['path'])" 2>/dev/null)
  [[ "$uorg_path" == "/$uorg_id" ]] && pass "根组织 path 格式正确 ($uorg_path)" || fail "根组织 path 不正确" "path=$uorg_path"

  resp=$(uds_req "{\"action\":\"create_org\",\"request_id\":\"o2\",\"payload\":{\"name\":\"UDS-ChildOrg\",\"parent_id\":\"$uorg_id\"}}")
  check_uds "创建子组织" "$resp" "success"
  local uchild_path
  uchild_path=$(echo "$resp" | python3 -c "import sys,json; print(json.load(sys.stdin)['payload']['path'])" 2>/dev/null)
  [[ "$uchild_path" == *"$uorg_id"* ]] && pass "子组织 path 包含父 ID" || fail "子组织 path 不含父 ID" "path=$uchild_path"

  resp=$(uds_req "{\"action\":\"get_org\",\"request_id\":\"o3\",\"payload\":{\"org_id\":\"$uorg_id\"}}")
  check_uds "获取组织" "$resp" "success"

  resp=$(uds_req '{"action":"get_org","request_id":"o4","payload":{"org_id":"no-such-org"}}')
  check_uds "获取组织-不存在" "$resp" "error"

  resp=$(uds_req '{"action":"list_orgs","request_id":"o5","payload":{"offset":0,"limit":10}}')
  check_uds "列出组织" "$resp" "success"
  local org_total
  org_total=$(echo "$resp" | python3 -c "import sys,json; print(json.load(sys.stdin)['payload']['total'])" 2>/dev/null)
  [[ "$org_total" -ge 1 ]] && pass "列出组织 total >= 1 (actual=$org_total)" || fail "组织列表为空" "total=$org_total"

  resp=$(uds_req '{"action":"create_org","request_id":"o6","payload":{}}')
  check_uds "创建组织-缺少 name" "$resp" "error"

  # ── 组织成员 ─────────────────────────────────────────────────────────────
  echo -e "\n  ${BOLD}[组织成员管理]${RESET}"
  resp=$(uds_req "{\"action\":\"add_org_member\",\"request_id\":\"om1\",\"payload\":{\"org_id\":\"$uorg_id\",\"user_id\":\"$ADMIN_ID\",\"role\":\"admin\"}}")
  check_uds "添加组织成员" "$resp" "success"

  resp=$(uds_req "{\"action\":\"add_org_member\",\"request_id\":\"om2\",\"payload\":{\"org_id\":\"$uorg_id\",\"user_id\":\"$ADMIN_ID\"}}")
  check_uds "添加成员-重复" "$resp" "error"

  resp=$(uds_req "{\"action\":\"add_org_member\",\"request_id\":\"om3\",\"payload\":{\"org_id\":\"$uorg_id\",\"user_id\":\"ghost-user-xyz\"}}")
  check_uds "添加成员-用户不存在" "$resp" "error"

  resp=$(uds_req "{\"action\":\"list_org_members\",\"request_id\":\"om4\",\"payload\":{\"org_id\":\"$uorg_id\"}}")
  check_uds       "列出组织成员" "$resp" "success"
  check_uds_field "成员数量=1" "$resp" "['payload']['total']" "1"
  local om_username
  om_username=$(echo "$resp" | python3 -c "import sys,json; print(json.load(sys.stdin)['payload']['items'][0]['username'])" 2>/dev/null)
  [[ "$om_username" == "admin" ]] && pass "成员含 username=admin" || fail "成员缺少 username" "username=$om_username"

  resp=$(uds_req "{\"action\":\"get_user_orgs\",\"request_id\":\"om5\",\"payload\":{\"user_id\":\"$ADMIN_ID\"}}")
  check_uds "获取用户所属组织" "$resp" "success"
  local user_org_total
  user_org_total=$(echo "$resp" | python3 -c "import sys,json; print(json.load(sys.stdin)['payload']['total'])" 2>/dev/null)
  [[ "$user_org_total" -ge 1 ]] && pass "用户所属组织 >= 1" || fail "用户所属组织为空" "total=$user_org_total"

  resp=$(uds_req "{\"action\":\"remove_org_member\",\"request_id\":\"om6\",\"payload\":{\"org_id\":\"$uorg_id\",\"user_id\":\"$ADMIN_ID\"}}")
  check_uds "移除组织成员" "$resp" "success"

  resp=$(uds_req "{\"action\":\"list_org_members\",\"request_id\":\"om7\",\"payload\":{\"org_id\":\"$uorg_id\"}}")
  check_uds_field "移除后成员数量=0" "$resp" "['payload']['total']" "0"

  # ── 群组管理 ─────────────────────────────────────────────────────────────
  echo -e "\n  ${BOLD}[群组管理]${RESET}"
  resp=$(uds_req '{"action":"create_group","request_id":"g1","payload":{"name":"uds-sre","description":"SRE Team","type":"ops"}}')
  check_uds       "创建群组" "$resp" "success"
  check_uds_field "群组名称正确" "$resp" "['payload']['name']" "uds-sre"
  local ugrp_id
  ugrp_id=$(echo "$resp" | python3 -c "import sys,json; print(json.load(sys.stdin)['payload']['group_id'])" 2>/dev/null)

  resp=$(uds_req "{\"action\":\"get_group\",\"request_id\":\"g2\",\"payload\":{\"group_id\":\"$ugrp_id\"}}")
  check_uds "获取群组" "$resp" "success"

  resp=$(uds_req '{"action":"get_group","request_id":"g3","payload":{"group_id":"no-such-group"}}')
  check_uds "获取群组-不存在" "$resp" "error"

  resp=$(uds_req '{"action":"list_groups","request_id":"g4","payload":{"offset":0,"limit":10}}')
  check_uds "列出群组" "$resp" "success"
  local grp_total
  grp_total=$(echo "$resp" | python3 -c "import sys,json; print(json.load(sys.stdin)['payload']['total'])" 2>/dev/null)
  [[ "$grp_total" -ge 1 ]] && pass "列出群组 total >= 1 (actual=$grp_total)" || fail "群组列表为空" "total=$grp_total"

  resp=$(uds_req '{"action":"create_group","request_id":"g5","payload":{}}')
  check_uds "创建群组-缺少 name" "$resp" "error"

  # ── 群组成员 ─────────────────────────────────────────────────────────────
  echo -e "\n  ${BOLD}[群组成员管理]${RESET}"
  resp=$(uds_req "{\"action\":\"add_group_member\",\"request_id\":\"gm1\",\"payload\":{\"group_id\":\"$ugrp_id\",\"user_id\":\"$ADMIN_ID\",\"role\":\"owner\"}}")
  check_uds "添加群组成员" "$resp" "success"

  resp=$(uds_req "{\"action\":\"add_group_member\",\"request_id\":\"gm2\",\"payload\":{\"group_id\":\"$ugrp_id\",\"user_id\":\"$ADMIN_ID\"}}")
  check_uds "添加成员-重复" "$resp" "error"

  resp=$(uds_req "{\"action\":\"add_group_member\",\"request_id\":\"gm3\",\"payload\":{\"group_id\":\"$ugrp_id\",\"user_id\":\"ghost-user-xyz\"}}")
  check_uds "添加成员-用户不存在" "$resp" "error"

  resp=$(uds_req "{\"action\":\"list_group_members\",\"request_id\":\"gm4\",\"payload\":{\"group_id\":\"$ugrp_id\"}}")
  check_uds       "列出群组成员" "$resp" "success"
  check_uds_field "成员数量=1" "$resp" "['payload']['total']" "1"
  local gm_uname
  gm_uname=$(echo "$resp" | python3 -c "import sys,json; print(json.load(sys.stdin)['payload']['items'][0]['username'])" 2>/dev/null)
  [[ "$gm_uname" == "admin" ]] && pass "成员含 username=admin" || fail "成员缺少 username" "username=$gm_uname"

  resp=$(uds_req "{\"action\":\"get_user_groups\",\"request_id\":\"gm5\",\"payload\":{\"user_id\":\"$ADMIN_ID\"}}")
  check_uds "获取用户所属群组" "$resp" "success"
  local user_grp_total
  user_grp_total=$(echo "$resp" | python3 -c "import sys,json; print(json.load(sys.stdin)['payload']['total'])" 2>/dev/null)
  [[ "$user_grp_total" -ge 1 ]] && pass "用户所属群组 >= 1" || fail "用户所属群组为空" "total=$user_grp_total"

  resp=$(uds_req "{\"action\":\"remove_group_member\",\"request_id\":\"gm6\",\"payload\":{\"group_id\":\"$ugrp_id\",\"user_id\":\"$ADMIN_ID\"}}")
  check_uds "移除群组成员" "$resp" "success"

  resp=$(uds_req "{\"action\":\"list_group_members\",\"request_id\":\"gm7\",\"payload\":{\"group_id\":\"$ugrp_id\"}}")
  check_uds_field "移除后成员数量=0" "$resp" "['payload']['total']" "0"
}

# =============================================================================
# 最终报告
# =============================================================================
print_report() {
  echo -e "\n${BOLD}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${RESET}"
  echo -e "${BOLD}                        测试结果汇总${RESET}"
  echo -e "${BOLD}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${RESET}"
  echo -e "  总计: ${BOLD}$TOTAL${RESET}  通过: ${GREEN}${BOLD}$PASSED${RESET}  失败: ${RED}${BOLD}$FAILED${RESET}"

  if [ "$FAILED" -gt 0 ]; then
    echo -e "\n${RED}${BOLD}失败项目:${RESET}"
    for c in "${FAILED_CASES[@]}"; do
      echo -e "  ${RED}✘${RESET} $c"
    done
    echo -e "${BOLD}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${RESET}"
    return 1
  else
    echo -e "\n${GREEN}${BOLD}所有测试通过 ✔${RESET}"
    echo -e "${BOLD}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${RESET}"
    return 0
  fi
}

# =============================================================================
# 主流程
# =============================================================================
trap 'stop_server' EXIT

echo -e "${BOLD}╔══════════════════════════════════════════════════════════════════╗${RESET}"
echo -e "${BOLD}║            IDP Service 自动化功能测试                           ║${RESET}"
echo -e "${BOLD}╚══════════════════════════════════════════════════════════════════╝${RESET}"
echo -e "  工作目录: $(pwd)"
echo -e "  时间:     $(date '+%Y-%m-%d %H:%M:%S')"

if $RUN_UNIT; then
  run_unit_tests
fi

if $RUN_REST || $RUN_UDS; then
  build_server
  start_server
  $RUN_REST && run_rest_tests
  $RUN_UDS  && run_uds_tests
fi

print_report
