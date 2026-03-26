package uds

import (
	"context"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"sync"
	"sync/atomic"
	"time"
)

const (
	// MaxMessageSize Protobuf 模式消息最大字节数 (1MB)
	MaxMessageSize = 1 << 20
	// MaxJSONMessageSize JSON 模式消息最大字节数 (512KB)
	MaxJSONMessageSize = 512 << 10
	// HandshakeTimeout 握手超时时间
	HandshakeTimeout = 5 * time.Second
	// IdleTimeout 连接最大空闲时间
	IdleTimeout = 60 * time.Second
	// ProtocolVersion 协议版本
	ProtocolVersion = "1.0"
)

// Connection 封装一个 UDS 连接
type Connection struct {
	id           string
	conn         net.Conn
	format       Format
	codec        Codec
	lastActive   atomic.Int64 // Unix nano
	closeOnce    sync.Once
	closeCh      chan struct{}
}

func newConnection(id string, conn net.Conn, format Format) *Connection {
	c := &Connection{
		id:      id,
		conn:    conn,
		format:  format,
		codec:   NewCodec(format),
		closeCh: make(chan struct{}),
	}
	c.lastActive.Store(time.Now().UnixNano())
	return c
}

func (c *Connection) ID() string { return c.id }

func (c *Connection) LastActiveTime() time.Time {
	return time.Unix(0, c.lastActive.Load())
}

func (c *Connection) touch() {
	c.lastActive.Store(time.Now().UnixNano())
}

func (c *Connection) Close() {
	c.closeOnce.Do(func() {
		close(c.closeCh)
		c.conn.Close()
	})
}

// Server UDS 服务端
type Server struct {
	socketPath  string
	listener    net.Listener
	registry    *HandlerRegistry
	connections sync.Map // id -> *Connection
	connCount   atomic.Int64
	maxConns    int64
	wg          sync.WaitGroup
	ctx         context.Context
	cancel      context.CancelFunc
}

// Config UDS 服务端配置
type Config struct {
	SocketPath string
	MaxConns   int64
	Registry   *HandlerRegistry
}

// NewServer 创建 UDS 服务端
func NewServer(cfg Config) *Server {
	ctx, cancel := context.WithCancel(context.Background())
	maxConns := cfg.MaxConns
	if maxConns == 0 {
		maxConns = 100
	}
	return &Server{
		socketPath: cfg.SocketPath,
		registry:   cfg.Registry,
		maxConns:   maxConns,
		ctx:        ctx,
		cancel:     cancel,
	}
}

// Start 启动 UDS 服务端
func (s *Server) Start() error {
	// 清理旧的 socket 文件
	if err := os.Remove(s.socketPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove old socket: %w", err)
	}

	listener, err := net.Listen("unix", s.socketPath)
	if err != nil {
		return fmt.Errorf("failed to listen on %s: %w", s.socketPath, err)
	}
	// 设置 socket 文件权限为 660
	if err := os.Chmod(s.socketPath, 0660); err != nil {
		listener.Close()
		return fmt.Errorf("failed to chmod socket: %w", err)
	}

	s.listener = listener
	log.Printf("[UDS] Server started on %s", s.socketPath)

	s.wg.Add(1)
	go s.acceptLoop()
	s.wg.Add(1)
	go s.idleChecker()

	return nil
}

// Stop 停止服务端
func (s *Server) Stop() {
	s.cancel()
	if s.listener != nil {
		s.listener.Close()
	}
	// 关闭所有连接
	s.connections.Range(func(key, value interface{}) bool {
		value.(*Connection).Close()
		return true
	})
	s.wg.Wait()
	os.Remove(s.socketPath)
	log.Printf("[UDS] Server stopped")
}

// acceptLoop 接受新连接
func (s *Server) acceptLoop() {
	defer s.wg.Done()
	for {
		conn, err := s.listener.Accept()
		if err != nil {
			select {
			case <-s.ctx.Done():
				return
			default:
				log.Printf("[UDS] Accept error: %v", err)
				continue
			}
		}

		if s.connCount.Load() >= s.maxConns {
			log.Printf("[UDS] Max connections reached, rejecting new connection")
			conn.Close()
			continue
		}

		s.wg.Add(1)
		go s.handleConn(conn)
	}
}

// handleConn 处理单个连接
func (s *Server) handleConn(rawConn net.Conn) {
	defer s.wg.Done()

	// 握手
	rawConn.SetDeadline(time.Now().Add(HandshakeTimeout))
	format, codec, err := s.performHandshake(rawConn)
	if err != nil {
		log.Printf("[UDS] Handshake failed: %v", err)
		rawConn.Close()
		return
	}
	rawConn.SetDeadline(time.Time{}) // 清除握手 deadline

	connID := fmt.Sprintf("conn-%d", time.Now().UnixNano())
	conn := newConnection(connID, rawConn, format)
	conn.codec = codec

	s.connections.Store(connID, conn)
	s.connCount.Add(1)
	defer func() {
		s.connections.Delete(connID)
		s.connCount.Add(-1)
		conn.Close()
	}()

	log.Printf("[UDS] Connection %s established (format: %s)", connID, format)

	for {
		select {
		case <-s.ctx.Done():
			return
		case <-conn.closeCh:
			return
		default:
		}

		// 设置读超时
		rawConn.SetReadDeadline(time.Now().Add(IdleTimeout))
		req, err := codec.ReadRequest(rawConn)
		if err != nil {
			if err == io.EOF {
				log.Printf("[UDS] Connection %s closed by client", connID)
			} else {
				log.Printf("[UDS] Read error on %s: %v", connID, err)
			}
			return
		}
		rawConn.SetReadDeadline(time.Time{})
		conn.touch()

		resp := s.registry.Handle(req)
		rawConn.SetWriteDeadline(time.Now().Add(30 * time.Second))
		if err := codec.WriteResponse(rawConn, resp); err != nil {
			log.Printf("[UDS] Write error on %s: %v", connID, err)
			return
		}
		rawConn.SetWriteDeadline(time.Time{})
	}
}

// performHandshake 执行握手协议，识别客户端格式
func (s *Server) performHandshake(conn net.Conn) (Format, Codec, error) {
	// 读取第一个字节来判断格式
	firstByte := make([]byte, 1)
	if _, err := io.ReadFull(conn, firstByte); err != nil {
		return "", nil, fmt.Errorf("failed to read first byte: %w", err)
	}

	var format Format
	var codec Codec

	if firstByte[0] == '{' {
		// JSON 握手: 将第一个字节放回，使用带缓冲的 reader
		format = FormatJSON
		codec = &JSONCodec{}
		reader := io.MultiReader(
			byteReader(firstByte[0]),
			conn,
		)
		req, err := codec.ReadHandshake(reader)
		if err != nil {
			return "", nil, fmt.Errorf("invalid JSON handshake: %w", err)
		}
		if err := s.validateHandshake(req); err != nil {
			resp := &HandshakeResponse{Status: "error", ErrorMessage: err.Error()}
			codec.WriteHandshake(conn, resp)
			return "", nil, err
		}
		resp := &HandshakeResponse{
			Status:  "ok",
			Version: ProtocolVersion,
			Format:  string(format),
		}
		if err := codec.WriteHandshake(conn, resp); err != nil {
			return "", nil, err
		}
	} else {
		// Protobuf 握手：4字节长度前缀（首字节已读）
		format = FormatProtobuf
		codec = &ProtobufCodec{}
		// 读取剩余 3 个长度字节
		remaining := make([]byte, 3)
		if _, err := io.ReadFull(conn, remaining); err != nil {
			return "", nil, fmt.Errorf("failed to read protobuf length: %w", err)
		}
		lenBuf := []byte{firstByte[0], remaining[0], remaining[1], remaining[2]}
		msgLen := uint32(lenBuf[0]) | uint32(lenBuf[1])<<8 | uint32(lenBuf[2])<<16 | uint32(lenBuf[3])<<24
		if msgLen > uint32(MaxMessageSize) {
			return "", nil, fmt.Errorf("handshake message too large: %d", msgLen)
		}
		msgBuf := make([]byte, msgLen)
		if _, err := io.ReadFull(conn, msgBuf); err != nil {
			return "", nil, fmt.Errorf("failed to read handshake body: %w", err)
		}
		reader := io.MultiReader(
			&lengthPrefixedReader{lenBuf: lenBuf, data: msgBuf},
			conn,
		)
		req, err := codec.ReadHandshake(reader)
		if err != nil {
			return "", nil, fmt.Errorf("invalid protobuf handshake: %w", err)
		}
		if err := s.validateHandshake(req); err != nil {
			resp := &HandshakeResponse{Status: "error", ErrorMessage: err.Error()}
			codec.WriteHandshake(conn, resp)
			return "", nil, err
		}
		resp := &HandshakeResponse{
			Status:  "ok",
			Version: ProtocolVersion,
			Format:  string(format),
		}
		if err := codec.WriteHandshake(conn, resp); err != nil {
			return "", nil, err
		}
	}

	return format, codec, nil
}

func (s *Server) validateHandshake(req *HandshakeRequest) error {
	if req.Version == "" {
		return fmt.Errorf("version is required")
	}
	if req.Format != string(FormatJSON) && req.Format != string(FormatProtobuf) {
		return fmt.Errorf("unsupported format: %s", req.Format)
	}
	return nil
}

// idleChecker 定期检查并关闭空闲连接
func (s *Server) idleChecker() {
	defer s.wg.Done()
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-s.ctx.Done():
			return
		case <-ticker.C:
			s.connections.Range(func(key, value interface{}) bool {
				conn := value.(*Connection)
				if time.Since(conn.LastActiveTime()) > IdleTimeout {
					log.Printf("[UDS] Connection %s idle timeout, closing", conn.ID())
					conn.Close()
				}
				return true
			})
		}
	}
}

// ConnCount 返回当前连接数
func (s *Server) ConnCount() int64 {
	return s.connCount.Load()
}

// byteReader 将单个字节包装为 io.Reader
type byteReader byte

func (b byteReader) Read(p []byte) (n int, err error) {
	if len(p) == 0 {
		return 0, nil
	}
	p[0] = byte(b)
	return 1, io.EOF
}

// lengthPrefixedReader 重新组装长度前缀消息用于 Codec 读取
type lengthPrefixedReader struct {
	lenBuf []byte
	data   []byte
	pos    int
	phase  int // 0=lenBuf, 1=data
}

func (r *lengthPrefixedReader) Read(p []byte) (n int, err error) {
	if r.phase == 0 {
		if r.pos >= len(r.lenBuf) {
			r.phase = 1
			r.pos = 0
		} else {
			n = copy(p, r.lenBuf[r.pos:])
			r.pos += n
			return n, nil
		}
	}
	if r.pos >= len(r.data) {
		return 0, io.EOF
	}
	n = copy(p, r.data[r.pos:])
	r.pos += n
	return n, nil
}
