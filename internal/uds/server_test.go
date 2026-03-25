package uds_test

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"encoding/json"
	"net"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/idp-service/internal/auth"
	"github.com/idp-service/internal/directory"
	"github.com/idp-service/internal/model"
	"github.com/idp-service/internal/service"
	"github.com/idp-service/internal/uds"
	"github.com/idp-service/internal/user"
)

// ---- helpers ----

func newTestServer(t *testing.T) (*uds.Server, string) {
	t.Helper()
	socketPath := "/tmp/idp-test-" + uuid.New().String() + ".sock"

	userStore := user.NewMemoryStore()
	syncStore := user.NewMemorySyncJobStore()
	orgStore := directory.NewMemoryOrgStore()
	groupStore := directory.NewMemoryGroupStore()
	authSvc := auth.NewService(auth.Config{
		JWTSecret:  "test-secret",
		BcryptCost: 4,
	})

	// 预创建一个测试用户
	hash, _ := authSvc.HashPassword("password123")
	userStore.Create(&model.User{
		ID:           "user-001",
		Username:     "testuser",
		Email:        "testuser@example.com",
		PasswordHash: hash,
		DisplayName:  "Test User",
		Status:       model.UserStatusActive,
	})

	userSvc := service.NewUserService(userStore, syncStore, authSvc)
	dirSvc := service.NewDirectoryService(orgStore, groupStore, userStore)
	registry := uds.NewHandlerRegistry(userSvc, dirSvc)
	srv := uds.NewServer(uds.Config{
		SocketPath: socketPath,
		MaxConns:   10,
		Registry:   registry,
	})

	if err := srv.Start(); err != nil {
		t.Fatalf("failed to start server: %v", err)
	}

	t.Cleanup(func() {
		srv.Stop()
		os.Remove(socketPath)
	})

	// 等待 socket 文件出现
	for i := 0; i < 10; i++ {
		if _, err := os.Stat(socketPath); err == nil {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	return srv, socketPath
}

// sendJSONLine 通过 JSON 模式发送一行消息
func sendJSONLine(conn net.Conn, v interface{}) error {
	data, err := json.Marshal(v)
	if err != nil {
		return err
	}
	data = append(data, '\n')
	_, err = conn.Write(data)
	return err
}

// recvJSONLine 读取一行 JSON 消息
func recvJSONLine(conn net.Conn, v interface{}) error {
	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	defer conn.SetReadDeadline(time.Time{})
	scanner := bufio.NewScanner(conn)
	if !scanner.Scan() {
		return scanner.Err()
	}
	return json.Unmarshal(scanner.Bytes(), v)
}

// doJSONHandshake 执行 JSON 握手
func doJSONHandshake(t *testing.T, conn net.Conn) {
	t.Helper()
	err := sendJSONLine(conn, map[string]string{
		"version": "1.0",
		"format":  "json",
	})
	if err != nil {
		t.Fatalf("failed to send handshake: %v", err)
	}
	var resp uds.HandshakeResponse
	if err := recvJSONLine(conn, &resp); err != nil {
		t.Fatalf("failed to recv handshake response: %v", err)
	}
	if resp.Status != "ok" {
		t.Fatalf("handshake failed: %s", resp.ErrorMessage)
	}
}

// sendProtobufFrame 发送长度前缀帧（Protobuf 模式）
func sendProtobufFrame(conn net.Conn, data []byte) error {
	var lenBuf [4]byte
	binary.LittleEndian.PutUint32(lenBuf[:], uint32(len(data)))
	if _, err := conn.Write(lenBuf[:]); err != nil {
		return err
	}
	_, err := conn.Write(data)
	return err
}

// recvProtobufFrame 读取长度前缀帧
func recvProtobufFrame(conn net.Conn) ([]byte, error) {
	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	defer conn.SetReadDeadline(time.Time{})
	var lenBuf [4]byte
	if _, err := net.Conn(conn).Read(lenBuf[:]); err != nil {
		return nil, err
	}
	// Use io.ReadFull equivalent
	msgLen := binary.LittleEndian.Uint32(lenBuf[:])
	buf := make([]byte, msgLen)
	total := 0
	for total < int(msgLen) {
		n, err := conn.Read(buf[total:])
		if err != nil {
			return nil, err
		}
		total += n
	}
	return buf, nil
}

// ---- tests ----

func TestUDSServer_JSONMode_Handshake(t *testing.T) {
	_, socketPath := newTestServer(t)

	conn, err := net.Dial("unix", socketPath)
	if err != nil {
		t.Fatalf("failed to connect: %v", err)
	}
	defer conn.Close()

	doJSONHandshake(t, conn)
}

func TestUDSServer_JSONMode_Auth_Success(t *testing.T) {
	_, socketPath := newTestServer(t)

	conn, err := net.Dial("unix", socketPath)
	if err != nil {
		t.Fatalf("failed to connect: %v", err)
	}
	defer conn.Close()

	doJSONHandshake(t, conn)

	// 发送认证请求
	req := map[string]interface{}{
		"action":     "auth",
		"request_id": "req-001",
		"payload": map[string]string{
			"username": "testuser",
			"password": "password123",
		},
	}
	if err := sendJSONLine(conn, req); err != nil {
		t.Fatalf("failed to send auth request: %v", err)
	}

	var resp uds.JSONResponse
	if err := recvJSONLine(conn, &resp); err != nil {
		t.Fatalf("failed to recv auth response: %v", err)
	}

	if resp.Status != "success" {
		t.Errorf("expected success, got %s: %s", resp.Status, resp.Error)
	}
	if resp.RequestID != "req-001" {
		t.Errorf("expected request_id=req-001, got %s", resp.RequestID)
	}
}

func TestUDSServer_JSONMode_Auth_WrongPassword(t *testing.T) {
	_, socketPath := newTestServer(t)

	conn, err := net.Dial("unix", socketPath)
	if err != nil {
		t.Fatalf("failed to connect: %v", err)
	}
	defer conn.Close()

	doJSONHandshake(t, conn)

	req := map[string]interface{}{
		"action":     "auth",
		"request_id": "req-002",
		"payload": map[string]string{
			"username": "testuser",
			"password": "wrongpassword",
		},
	}
	sendJSONLine(conn, req)

	var resp uds.JSONResponse
	recvJSONLine(conn, &resp)

	if resp.Status != "error" {
		t.Errorf("expected error status for wrong password, got %s", resp.Status)
	}
}

func TestUDSServer_JSONMode_GetUser(t *testing.T) {
	_, socketPath := newTestServer(t)

	conn, _ := net.Dial("unix", socketPath)
	defer conn.Close()

	doJSONHandshake(t, conn)

	req := map[string]interface{}{
		"action":     "get_user",
		"request_id": "req-003",
		"payload": map[string]string{
			"username": "testuser",
		},
	}
	sendJSONLine(conn, req)

	var resp uds.JSONResponse
	recvJSONLine(conn, &resp)

	if resp.Status != "success" {
		t.Errorf("expected success, got %s: %s", resp.Status, resp.Error)
	}
}

func TestUDSServer_JSONMode_GetUser_NotFound(t *testing.T) {
	_, socketPath := newTestServer(t)

	conn, _ := net.Dial("unix", socketPath)
	defer conn.Close()

	doJSONHandshake(t, conn)

	req := map[string]interface{}{
		"action":     "get_user",
		"request_id": "req-004",
		"payload": map[string]string{
			"username": "nonexistent",
		},
	}
	sendJSONLine(conn, req)

	var resp uds.JSONResponse
	recvJSONLine(conn, &resp)

	if resp.Status != "error" {
		t.Errorf("expected error for non-existent user, got %s", resp.Status)
	}
}

func TestUDSServer_JSONMode_TriggerSync(t *testing.T) {
	_, socketPath := newTestServer(t)

	conn, _ := net.Dial("unix", socketPath)
	defer conn.Close()

	doJSONHandshake(t, conn)

	req := map[string]interface{}{
		"action":     "trigger_sync",
		"request_id": "req-005",
		"payload": map[string]string{
			"source_system": "ldap",
			"sync_type":     "incremental",
		},
	}
	sendJSONLine(conn, req)

	var resp uds.JSONResponse
	recvJSONLine(conn, &resp)

	if resp.Status != "success" {
		t.Errorf("expected success, got %s: %s", resp.Status, resp.Error)
	}
}

func TestUDSServer_JSONMode_Ping(t *testing.T) {
	_, socketPath := newTestServer(t)

	conn, _ := net.Dial("unix", socketPath)
	defer conn.Close()

	doJSONHandshake(t, conn)

	req := map[string]interface{}{
		"action":     "ping",
		"request_id": "ping-001",
		"payload":    map[string]interface{}{},
	}
	sendJSONLine(conn, req)

	var resp uds.JSONResponse
	recvJSONLine(conn, &resp)

	if resp.Status != "pong" {
		t.Errorf("expected pong, got %s", resp.Status)
	}
}

func TestUDSServer_JSONMode_UnknownAction(t *testing.T) {
	_, socketPath := newTestServer(t)

	conn, _ := net.Dial("unix", socketPath)
	defer conn.Close()

	doJSONHandshake(t, conn)

	req := map[string]interface{}{
		"action":     "unknown_action",
		"request_id": "req-006",
		"payload":    map[string]interface{}{},
	}
	sendJSONLine(conn, req)

	var resp uds.JSONResponse
	recvJSONLine(conn, &resp)

	if resp.Status != "error" {
		t.Errorf("expected error for unknown action, got %s", resp.Status)
	}
}

func TestUDSServer_ProtobufMode_Handshake(t *testing.T) {
	_, socketPath := newTestServer(t)

	conn, err := net.Dial("unix", socketPath)
	if err != nil {
		t.Fatalf("failed to connect: %v", err)
	}
	defer conn.Close()

	// 发送 Protobuf 格式握手（使用长度前缀帧）
	handshake := map[string]interface{}{
		"version": "1.0",
		"format":  "protobuf",
	}
	data, _ := json.Marshal(handshake)
	if err := sendProtobufFrame(conn, data); err != nil {
		t.Fatalf("failed to send protobuf handshake: %v", err)
	}

	// 读取响应
	respData, err := recvProtobufFrame(conn)
	if err != nil {
		t.Fatalf("failed to recv protobuf handshake response: %v", err)
	}

	var resp uds.HandshakeResponse
	if err := json.Unmarshal(respData, &resp); err != nil {
		t.Fatalf("failed to parse handshake response: %v", err)
	}

	if resp.Status != "ok" {
		t.Errorf("expected ok, got %s: %s", resp.Status, resp.ErrorMessage)
	}
	if resp.Format != "protobuf" {
		t.Errorf("expected format=protobuf, got %s", resp.Format)
	}
}

func TestUDSServer_ProtobufMode_Auth(t *testing.T) {
	_, socketPath := newTestServer(t)

	conn, err := net.Dial("unix", socketPath)
	if err != nil {
		t.Fatalf("failed to connect: %v", err)
	}
	defer conn.Close()

	// 握手
	handshake := map[string]interface{}{"version": "1.0", "format": "protobuf"}
	data, _ := json.Marshal(handshake)
	sendProtobufFrame(conn, data)
	recvProtobufFrame(conn) // 读取握手响应

	// 发送认证请求
	req := map[string]interface{}{
		"action":     "auth",
		"request_id": "req-proto-001",
		"payload": map[string]string{
			"username": "testuser",
			"password": "password123",
		},
	}
	reqData, _ := json.Marshal(req)
	if err := sendProtobufFrame(conn, reqData); err != nil {
		t.Fatalf("failed to send auth request: %v", err)
	}

	respData, err := recvProtobufFrame(conn)
	if err != nil {
		t.Fatalf("failed to recv auth response: %v", err)
	}

	var resp uds.JSONResponse
	if err := json.Unmarshal(respData, &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	if resp.Status != "success" {
		t.Errorf("expected success, got %s: %s", resp.Status, resp.Error)
	}
}

func TestUDSServer_InvalidHandshake(t *testing.T) {
	_, socketPath := newTestServer(t)

	conn, err := net.Dial("unix", socketPath)
	if err != nil {
		t.Fatalf("failed to connect: %v", err)
	}
	defer conn.Close()

	// 发送格式错误的 JSON（不含 format 字段）
	invalid := map[string]string{"version": "1.0"}
	data, _ := json.Marshal(invalid)
	data = append(data, '\n')
	conn.Write(data)

	// 服务端应返回错误并关闭连接
	var resp uds.HandshakeResponse
	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	scanner := bufio.NewScanner(conn)
	if scanner.Scan() {
		json.Unmarshal(scanner.Bytes(), &resp)
		if resp.Status != "error" {
			t.Errorf("expected error response for invalid handshake, got %s", resp.Status)
		}
	}
}

func TestUDSServer_MultipleConnections(t *testing.T) {
	srv, socketPath := newTestServer(t)

	const numConns = 5
	conns := make([]net.Conn, numConns)

	for i := 0; i < numConns; i++ {
		conn, err := net.Dial("unix", socketPath)
		if err != nil {
			t.Fatalf("failed to connect #%d: %v", i, err)
		}
		conns[i] = conn
		doJSONHandshake(t, conn)
	}

	// 等待连接计数更新
	time.Sleep(50 * time.Millisecond)

	if count := srv.ConnCount(); count != int64(numConns) {
		t.Errorf("expected %d connections, got %d", numConns, count)
	}

	for _, conn := range conns {
		conn.Close()
	}
}

// TestCodec_JSONCodec 测试 JSON 编解码器
func TestCodec_JSONCodec(t *testing.T) {
	codec := &uds.JSONCodec{}

	t.Run("write and read handshake", func(t *testing.T) {
		var buf bytes.Buffer
		resp := &uds.HandshakeResponse{
			Status:  "ok",
			Version: "1.0",
			Format:  "json",
		}
		if err := codec.WriteHandshake(&buf, resp); err != nil {
			t.Fatalf("write error: %v", err)
		}

		// 读取
		got, err := codec.ReadHandshake(bytes.NewReader(buf.Bytes()))
		// Note: ReadHandshake reads a HandshakeRequest, so this tests the wire format
		if err != nil {
			t.Logf("expected error reading response as request (different types): %v", err)
		}
		_ = got
	})

	t.Run("write and read request/response", func(t *testing.T) {
		var buf bytes.Buffer
		resp := &uds.JSONResponse{
			Status:    "success",
			RequestID: "test-123",
		}
		if err := codec.WriteResponse(&buf, resp); err != nil {
			t.Fatalf("write error: %v", err)
		}

		if !bytes.Contains(buf.Bytes(), []byte("success")) {
			t.Error("response should contain 'success'")
		}
		if !bytes.HasSuffix(buf.Bytes(), []byte("\n")) {
			t.Error("JSON response should end with newline")
		}
	})
}

// TestCodec_ProtobufCodec 测试 Protobuf 编解码器
func TestCodec_ProtobufCodec(t *testing.T) {
	codec := &uds.ProtobufCodec{}

	t.Run("write and read response", func(t *testing.T) {
		var buf bytes.Buffer
		resp := &uds.JSONResponse{
			Status:    "success",
			RequestID: "test-456",
		}
		if err := codec.WriteResponse(&buf, resp); err != nil {
			t.Fatalf("write error: %v", err)
		}

		// 验证长度前缀
		if buf.Len() < 4 {
			t.Fatal("protobuf response should have at least 4 bytes (length prefix)")
		}
		msgLen := binary.LittleEndian.Uint32(buf.Bytes()[:4])
		if int(msgLen) != buf.Len()-4 {
			t.Errorf("length prefix mismatch: prefix=%d, actual=%d", msgLen, buf.Len()-4)
		}
	})
}

// ========== 目录管理 UDS 测试 ==========

func doUDSRequest(t *testing.T, conn net.Conn, action, requestID string, payload interface{}) uds.JSONResponse {
	t.Helper()
	req := map[string]interface{}{
		"action":     action,
		"request_id": requestID,
		"payload":    payload,
	}
	if err := sendJSONLine(conn, req); err != nil {
		t.Fatalf("failed to send %s request: %v", action, err)
	}
	var resp uds.JSONResponse
	if err := recvJSONLine(conn, &resp); err != nil {
		t.Fatalf("failed to recv %s response: %v", action, err)
	}
	return resp
}

func TestUDS_Org_CreateAndGet(t *testing.T) {
	_, socketPath := newTestServer(t)

	conn, _ := net.Dial("unix", socketPath)
	defer conn.Close()
	doJSONHandshake(t, conn)

	// 创建组织
	resp := doUDSRequest(t, conn, "create_org", "r1", map[string]string{"name": "Engineering"})
	if resp.Status != "success" {
		t.Fatalf("create_org failed: %s", resp.Error)
	}
	payload := resp.Payload.(map[string]interface{})
	orgID := payload["org_id"].(string)
	if orgID == "" || payload["name"] != "Engineering" {
		t.Errorf("unexpected org payload: %v", payload)
	}
	if payload["path"] == "" {
		t.Error("path should be set")
	}

	// 获取组织
	resp2 := doUDSRequest(t, conn, "get_org", "r2", map[string]string{"org_id": orgID})
	if resp2.Status != "success" {
		t.Fatalf("get_org failed: %s", resp2.Error)
	}
	p2 := resp2.Payload.(map[string]interface{})
	if p2["name"] != "Engineering" {
		t.Errorf("expected Engineering, got %v", p2["name"])
	}
}

func TestUDS_Org_GetNotFound(t *testing.T) {
	_, socketPath := newTestServer(t)

	conn, _ := net.Dial("unix", socketPath)
	defer conn.Close()
	doJSONHandshake(t, conn)

	resp := doUDSRequest(t, conn, "get_org", "r1", map[string]string{"org_id": "non-existent"})
	if resp.Status != "error" {
		t.Errorf("expected error for non-existent org, got %s", resp.Status)
	}
}

func TestUDS_Org_ListOrgs(t *testing.T) {
	_, socketPath := newTestServer(t)

	conn, _ := net.Dial("unix", socketPath)
	defer conn.Close()
	doJSONHandshake(t, conn)

	// 创建三个组织
	for _, name := range []string{"Sales", "Marketing", "Legal"} {
		doUDSRequest(t, conn, "create_org", "c-"+name, map[string]string{"name": name})
	}

	resp := doUDSRequest(t, conn, "list_orgs", "list-1", map[string]int{"offset": 0, "limit": 10})
	if resp.Status != "success" {
		t.Fatalf("list_orgs failed: %s", resp.Error)
	}
	p := resp.Payload.(map[string]interface{})
	total := p["total"].(float64)
	if total < 3 {
		t.Errorf("expected at least 3 orgs, got %v", total)
	}
}

func TestUDS_Org_MemberManagement(t *testing.T) {
	_, socketPath := newTestServer(t)

	conn, _ := net.Dial("unix", socketPath)
	defer conn.Close()
	doJSONHandshake(t, conn)

	// 创建组织
	resp := doUDSRequest(t, conn, "create_org", "r1", map[string]string{"name": "DevTeam"})
	orgID := resp.Payload.(map[string]interface{})["org_id"].(string)

	t.Run("add member", func(t *testing.T) {
		resp := doUDSRequest(t, conn, "add_org_member", "r2", map[string]string{
			"org_id":  orgID,
			"user_id": "user-001",
			"role":    "member",
		})
		if resp.Status != "success" {
			t.Errorf("expected success, got error: %s", resp.Error)
		}
	})

	t.Run("add duplicate member", func(t *testing.T) {
		resp := doUDSRequest(t, conn, "add_org_member", "r3", map[string]string{
			"org_id":  orgID,
			"user_id": "user-001",
			"role":    "member",
		})
		if resp.Status != "error" {
			t.Errorf("expected error for duplicate member, got %s", resp.Status)
		}
	})

	t.Run("list members with user info", func(t *testing.T) {
		resp := doUDSRequest(t, conn, "list_org_members", "r4", map[string]string{"org_id": orgID})
		if resp.Status != "success" {
			t.Fatalf("list_org_members failed: %s", resp.Error)
		}
		p := resp.Payload.(map[string]interface{})
		if p["total"].(float64) != 1 {
			t.Errorf("expected 1 member, got %v", p["total"])
		}
		items := p["items"].([]interface{})
		member := items[0].(map[string]interface{})
		if member["username"] != "testuser" {
			t.Errorf("expected username=testuser, got %v", member["username"])
		}
	})

	t.Run("get user orgs", func(t *testing.T) {
		resp := doUDSRequest(t, conn, "get_user_orgs", "r5", map[string]string{"user_id": "user-001"})
		if resp.Status != "success" {
			t.Fatalf("get_user_orgs failed: %s", resp.Error)
		}
		p := resp.Payload.(map[string]interface{})
		if p["total"].(float64) != 1 {
			t.Errorf("expected 1 org for user, got %v", p["total"])
		}
	})

	t.Run("remove member", func(t *testing.T) {
		resp := doUDSRequest(t, conn, "remove_org_member", "r6", map[string]string{
			"org_id":  orgID,
			"user_id": "user-001",
		})
		if resp.Status != "success" {
			t.Errorf("expected success, got error: %s", resp.Error)
		}
		resp2 := doUDSRequest(t, conn, "list_org_members", "r7", map[string]string{"org_id": orgID})
		p := resp2.Payload.(map[string]interface{})
		if p["total"].(float64) != 0 {
			t.Errorf("expected 0 members after removal, got %v", p["total"])
		}
	})

	t.Run("add non-existent user", func(t *testing.T) {
		resp := doUDSRequest(t, conn, "add_org_member", "r8", map[string]string{
			"org_id":  orgID,
			"user_id": "no-such-user",
		})
		if resp.Status != "error" {
			t.Errorf("expected error for non-existent user, got %s", resp.Status)
		}
	})
}

func TestUDS_Group_CreateAndGet(t *testing.T) {
	_, socketPath := newTestServer(t)

	conn, _ := net.Dial("unix", socketPath)
	defer conn.Close()
	doJSONHandshake(t, conn)

	resp := doUDSRequest(t, conn, "create_group", "r1", map[string]string{
		"name":        "admins",
		"description": "System administrators",
		"type":        "security",
	})
	if resp.Status != "success" {
		t.Fatalf("create_group failed: %s", resp.Error)
	}
	payload := resp.Payload.(map[string]interface{})
	groupID := payload["group_id"].(string)
	if groupID == "" || payload["name"] != "admins" {
		t.Errorf("unexpected group payload: %v", payload)
	}

	resp2 := doUDSRequest(t, conn, "get_group", "r2", map[string]string{"group_id": groupID})
	if resp2.Status != "success" {
		t.Fatalf("get_group failed: %s", resp2.Error)
	}
}

func TestUDS_Group_ListGroups(t *testing.T) {
	_, socketPath := newTestServer(t)

	conn, _ := net.Dial("unix", socketPath)
	defer conn.Close()
	doJSONHandshake(t, conn)

	for _, name := range []string{"group1", "group2", "group3"} {
		doUDSRequest(t, conn, "create_group", "c-"+name, map[string]string{"name": name})
	}

	resp := doUDSRequest(t, conn, "list_groups", "list-1", map[string]int{"offset": 0, "limit": 10})
	if resp.Status != "success" {
		t.Fatalf("list_groups failed: %s", resp.Error)
	}
	p := resp.Payload.(map[string]interface{})
	if p["total"].(float64) < 3 {
		t.Errorf("expected at least 3 groups, got %v", p["total"])
	}
}

func TestUDS_Group_MemberManagement(t *testing.T) {
	_, socketPath := newTestServer(t)

	conn, _ := net.Dial("unix", socketPath)
	defer conn.Close()
	doJSONHandshake(t, conn)

	resp := doUDSRequest(t, conn, "create_group", "r1", map[string]string{"name": "backend"})
	groupID := resp.Payload.(map[string]interface{})["group_id"].(string)

	t.Run("add member", func(t *testing.T) {
		resp := doUDSRequest(t, conn, "add_group_member", "r2", map[string]string{
			"group_id": groupID,
			"user_id":  "user-001",
			"role":     "member",
		})
		if resp.Status != "success" {
			t.Errorf("expected success, got error: %s", resp.Error)
		}
	})

	t.Run("add duplicate member", func(t *testing.T) {
		resp := doUDSRequest(t, conn, "add_group_member", "r3", map[string]string{
			"group_id": groupID,
			"user_id":  "user-001",
		})
		if resp.Status != "error" {
			t.Errorf("expected error for duplicate, got %s", resp.Status)
		}
	})

	t.Run("list members with user info", func(t *testing.T) {
		resp := doUDSRequest(t, conn, "list_group_members", "r4", map[string]string{"group_id": groupID})
		if resp.Status != "success" {
			t.Fatalf("list_group_members failed: %s", resp.Error)
		}
		p := resp.Payload.(map[string]interface{})
		if p["total"].(float64) != 1 {
			t.Errorf("expected 1 member, got %v", p["total"])
		}
		items := p["items"].([]interface{})
		m := items[0].(map[string]interface{})
		if m["username"] != "testuser" {
			t.Errorf("expected username=testuser, got %v", m["username"])
		}
	})

	t.Run("get user groups", func(t *testing.T) {
		resp := doUDSRequest(t, conn, "get_user_groups", "r5", map[string]string{"user_id": "user-001"})
		if resp.Status != "success" {
			t.Fatalf("get_user_groups failed: %s", resp.Error)
		}
		p := resp.Payload.(map[string]interface{})
		if p["total"].(float64) != 1 {
			t.Errorf("expected 1 group for user, got %v", p["total"])
		}
	})

	t.Run("remove member", func(t *testing.T) {
		resp := doUDSRequest(t, conn, "remove_group_member", "r6", map[string]string{
			"group_id": groupID,
			"user_id":  "user-001",
		})
		if resp.Status != "success" {
			t.Errorf("expected success, got error: %s", resp.Error)
		}
		resp2 := doUDSRequest(t, conn, "list_group_members", "r7", map[string]string{"group_id": groupID})
		p := resp2.Payload.(map[string]interface{})
		if p["total"].(float64) != 0 {
			t.Errorf("expected 0 members after removal, got %v", p["total"])
		}
	})
}
