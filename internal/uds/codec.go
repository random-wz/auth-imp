package uds

import (
	"bufio"
	"encoding/binary"
	"encoding/json"
	"errors"
	"io"
)

// Format 消息格式
type Format string

const (
	FormatJSON     Format = "json"
	FormatProtobuf Format = "protobuf"
)

// HandshakeRequest 握手请求
type HandshakeRequest struct {
	Version           string   `json:"version"`
	Format            string   `json:"format"`
	SupportedVersions []string `json:"supported_versions,omitempty"`
}

// HandshakeResponse 握手响应
type HandshakeResponse struct {
	Status       string `json:"status"`
	Version      string `json:"version"`
	Format       string `json:"format"`
	ErrorMessage string `json:"error_message,omitempty"`
}

// JSONRequest JSON 模式请求
type JSONRequest struct {
	Action    string          `json:"action"`
	Payload   json.RawMessage `json:"payload"`
	RequestID string          `json:"request_id"`
}

// JSONResponse JSON 模式响应
type JSONResponse struct {
	Status    string      `json:"status"`
	Payload   interface{} `json:"payload,omitempty"`
	RequestID string      `json:"request_id"`
	Error     string      `json:"error,omitempty"`
}

// AuthPayload 认证请求 payload
type AuthPayload struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// AuthResponsePayload 认证响应 payload
type AuthResponsePayload struct {
	UserID      string `json:"user_id"`
	Username    string `json:"username"`
	Email       string `json:"email"`
	DisplayName string `json:"display_name"`
}

// GetUserPayload 获取用户请求 payload
type GetUserPayload struct {
	UserID   string `json:"user_id,omitempty"`
	Username string `json:"username,omitempty"`
}

// OrgInfo 组织信息
type OrgInfo struct {
	OrgID   string `json:"org_id"`
	OrgName string `json:"org_name"`
	Role    string `json:"role"`
}

// GetUserResponsePayload 获取用户响应 payload
type GetUserResponsePayload struct {
	UserID        string    `json:"user_id"`
	Username      string    `json:"username"`
	Email         string    `json:"email"`
	DisplayName   string    `json:"display_name"`
	Organizations []OrgInfo `json:"organizations"`
}

// TriggerSyncPayload 触发同步请求 payload
type TriggerSyncPayload struct {
	SourceSystem string `json:"source_system"`
	SyncType     string `json:"sync_type"`
}

// TriggerSyncResponsePayload 触发同步响应 payload
type TriggerSyncResponsePayload struct {
	JobID             string `json:"job_id"`
	EstimatedDuration string `json:"estimated_duration"`
	Message           string `json:"message"`
}

// --- Org payloads ---

// CreateOrgPayload 创建组织请求 payload
type CreateOrgPayload struct {
	Name     string `json:"name"`
	ParentID string `json:"parent_id,omitempty"`
}

// GetOrgPayload 获取组织请求 payload
type GetOrgPayload struct {
	OrgID string `json:"org_id"`
}

// ListOrgsPayload 列出组织请求 payload
type ListOrgsPayload struct {
	Offset int `json:"offset"`
	Limit  int `json:"limit"`
}

// OrgDetailInfo 组织详情
type OrgDetailInfo struct {
	OrgID    string `json:"org_id"`
	Name     string `json:"name"`
	ParentID string `json:"parent_id,omitempty"`
	Path     string `json:"path"`
	Role     string `json:"role,omitempty"`
}

// ListOrgsResponsePayload 列出组织响应 payload
type ListOrgsResponsePayload struct {
	Items []OrgDetailInfo `json:"items"`
	Total int64           `json:"total"`
}

// AddOrgMemberPayload 添加组织成员请求 payload
type AddOrgMemberPayload struct {
	OrgID  string `json:"org_id"`
	UserID string `json:"user_id"`
	Role   string `json:"role,omitempty"`
}

// RemoveOrgMemberPayload 移除组织成员请求 payload
type RemoveOrgMemberPayload struct {
	OrgID  string `json:"org_id"`
	UserID string `json:"user_id"`
}

// ListOrgMembersPayload 列出组织成员请求 payload
type ListOrgMembersPayload struct {
	OrgID string `json:"org_id"`
}

// OrgMemberInfo 组织成员信息
type OrgMemberInfo struct {
	UserID      string `json:"user_id"`
	Username    string `json:"username"`
	DisplayName string `json:"display_name"`
	Role        string `json:"role"`
}

// ListOrgMembersResponsePayload 列出组织成员响应 payload
type ListOrgMembersResponsePayload struct {
	Items []OrgMemberInfo `json:"items"`
	Total int64           `json:"total"`
}

// GetUserOrgsPayload 获取用户所属组织请求 payload
type GetUserOrgsPayload struct {
	UserID string `json:"user_id"`
}

// --- Group payloads ---

// CreateGroupPayload 创建群组请求 payload
type CreateGroupPayload struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Type        string `json:"type,omitempty"`
}

// GetGroupPayload 获取群组请求 payload
type GetGroupPayload struct {
	GroupID string `json:"group_id"`
}

// ListGroupsPayload 列出群组请求 payload
type ListGroupsPayload struct {
	Offset int `json:"offset"`
	Limit  int `json:"limit"`
}

// GroupDetailInfo 群组详情
type GroupDetailInfo struct {
	GroupID     string `json:"group_id"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Type        string `json:"type,omitempty"`
	Role        string `json:"role,omitempty"`
}

// ListGroupsResponsePayload 列出群组响应 payload
type ListGroupsResponsePayload struct {
	Items []GroupDetailInfo `json:"items"`
	Total int64             `json:"total"`
}

// AddGroupMemberPayload 添加群组成员请求 payload
type AddGroupMemberPayload struct {
	GroupID string `json:"group_id"`
	UserID  string `json:"user_id"`
	Role    string `json:"role,omitempty"`
}

// RemoveGroupMemberPayload 移除群组成员请求 payload
type RemoveGroupMemberPayload struct {
	GroupID string `json:"group_id"`
	UserID  string `json:"user_id"`
}

// ListGroupMembersPayload 列出群组成员请求 payload
type ListGroupMembersPayload struct {
	GroupID string `json:"group_id"`
}

// GroupMemberInfo 群组成员信息
type GroupMemberInfo struct {
	UserID      string `json:"user_id"`
	Username    string `json:"username"`
	DisplayName string `json:"display_name"`
	Role        string `json:"role"`
}

// ListGroupMembersResponsePayload 列出群组成员响应 payload
type ListGroupMembersResponsePayload struct {
	Items []GroupMemberInfo `json:"items"`
	Total int64             `json:"total"`
}

// GetUserGroupsPayload 获取用户所属群组请求 payload
type GetUserGroupsPayload struct {
	UserID string `json:"user_id"`
}

// Codec 编解码器接口
type Codec interface {
	// ReadHandshake 读取握手请求
	ReadHandshake(r io.Reader) (*HandshakeRequest, error)
	// WriteHandshake 写握手响应
	WriteHandshake(w io.Writer, resp *HandshakeResponse) error
	// ReadRequest 读取业务请求
	ReadRequest(r io.Reader) (*JSONRequest, error)
	// WriteResponse 写业务响应
	WriteResponse(w io.Writer, resp *JSONResponse) error
}

// JSONCodec JSON 编解码器（使用换行符分隔消息）
type JSONCodec struct{}

func (c *JSONCodec) ReadHandshake(r io.Reader) (*HandshakeRequest, error) {
	scanner := bufio.NewScanner(r)
	if !scanner.Scan() {
		if err := scanner.Err(); err != nil {
			return nil, err
		}
		return nil, io.EOF
	}
	var req HandshakeRequest
	if err := json.Unmarshal(scanner.Bytes(), &req); err != nil {
		return nil, err
	}
	return &req, nil
}

func (c *JSONCodec) WriteHandshake(w io.Writer, resp *HandshakeResponse) error {
	data, err := json.Marshal(resp)
	if err != nil {
		return err
	}
	data = append(data, '\n')
	_, err = w.Write(data)
	return err
}

func (c *JSONCodec) ReadRequest(r io.Reader) (*JSONRequest, error) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, MaxJSONMessageSize), MaxJSONMessageSize)
	if !scanner.Scan() {
		if err := scanner.Err(); err != nil {
			return nil, err
		}
		return nil, io.EOF
	}
	if len(scanner.Bytes()) > MaxJSONMessageSize {
		return nil, errors.New("message size exceeded limit")
	}
	var req JSONRequest
	if err := json.Unmarshal(scanner.Bytes(), &req); err != nil {
		return nil, err
	}
	return &req, nil
}

func (c *JSONCodec) WriteResponse(w io.Writer, resp *JSONResponse) error {
	data, err := json.Marshal(resp)
	if err != nil {
		return err
	}
	data = append(data, '\n')
	_, err = w.Write(data)
	return err
}

// ProtobufCodec Protobuf 编解码器（4字节小端序长度前缀 + JSON消息体）
// 注意：本实现使用 JSON 作为消息体，生产环境应替换为 proto.Marshal/Unmarshal
type ProtobufCodec struct{}

func (c *ProtobufCodec) ReadHandshake(r io.Reader) (*HandshakeRequest, error) {
	data, err := readLengthPrefixed(r)
	if err != nil {
		return nil, err
	}
	var req HandshakeRequest
	if err := json.Unmarshal(data, &req); err != nil {
		return nil, err
	}
	return &req, nil
}

func (c *ProtobufCodec) WriteHandshake(w io.Writer, resp *HandshakeResponse) error {
	data, err := json.Marshal(resp)
	if err != nil {
		return err
	}
	return writeLengthPrefixed(w, data)
}

func (c *ProtobufCodec) ReadRequest(r io.Reader) (*JSONRequest, error) {
	data, err := readLengthPrefixed(r)
	if err != nil {
		return nil, err
	}
	if len(data) > MaxMessageSize {
		return nil, errors.New("message size exceeded limit")
	}
	var req JSONRequest
	if err := json.Unmarshal(data, &req); err != nil {
		return nil, err
	}
	return &req, nil
}

func (c *ProtobufCodec) WriteResponse(w io.Writer, resp *JSONResponse) error {
	data, err := json.Marshal(resp)
	if err != nil {
		return err
	}
	return writeLengthPrefixed(w, data)
}

// readLengthPrefixed 读取 4 字节小端序长度 + 消息体
func readLengthPrefixed(r io.Reader) ([]byte, error) {
	var lenBuf [4]byte
	if _, err := io.ReadFull(r, lenBuf[:]); err != nil {
		return nil, err
	}
	msgLen := binary.LittleEndian.Uint32(lenBuf[:])
	if msgLen > uint32(MaxMessageSize) {
		return nil, errors.New("message too large")
	}
	buf := make([]byte, msgLen)
	if _, err := io.ReadFull(r, buf); err != nil {
		return nil, err
	}
	return buf, nil
}

// writeLengthPrefixed 写入 4 字节小端序长度 + 消息体
func writeLengthPrefixed(w io.Writer, data []byte) error {
	var lenBuf [4]byte
	binary.LittleEndian.PutUint32(lenBuf[:], uint32(len(data)))
	if _, err := w.Write(lenBuf[:]); err != nil {
		return err
	}
	_, err := w.Write(data)
	return err
}

// NewCodec 根据格式创建对应的编解码器
func NewCodec(format Format) Codec {
	switch format {
	case FormatProtobuf:
		return &ProtobufCodec{}
	default:
		return &JSONCodec{}
	}
}
