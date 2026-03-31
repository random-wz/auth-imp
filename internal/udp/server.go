package udp

import (
	"fmt"
	"log"
	"net"
	"net/url"
	"strings"
	"sync"

	"github.com/idp-service/internal/service"
)

type Server struct {
	addr    string
	conn    *net.UDPConn
	userSvc *service.UserService
	bufPool *sync.Pool
}

func NewServer(addr string, userSvc *service.UserService) *Server {
	return &Server{
		addr:    addr,
		userSvc: userSvc,
		bufPool: &sync.Pool{
			New: func() interface{} {
				return make([]byte, 2048)
			},
		},
	}
}

func (s *Server) Start() error {
	udpAddr, err := net.ResolveUDPAddr("udp", s.addr)
	if err != nil {
		return err
	}

	s.conn, err = net.ListenUDP("udp", udpAddr)
	if err != nil {
		return err
	}

	// 增加接收缓冲区大小
	s.conn.SetReadBuffer(1024 * 1024)

	log.Printf("UDP server listening on %s", s.addr)
	go s.handlePackets()
	return nil
}

func (s *Server) Stop() {
	if s.conn != nil {
		s.conn.Close()
	}
}

func (s *Server) handlePackets() {
	for {
		buf := s.bufPool.Get().([]byte)
		n, addr, err := s.conn.ReadFromUDP(buf)
		if err != nil {
			s.bufPool.Put(buf)
			log.Printf("UDP read error: %v", err)
			return
		}
		data := make([]byte, n)
		copy(data, buf[:n])
		s.bufPool.Put(buf)
		go s.processPacket(data, addr)
	}
}

func (s *Server) processPacket(data []byte, addr *net.UDPAddr) {
	msg := string(data)
	username, password, err := s.parseAuthRequest(msg)
	if err != nil {
		log.Printf("Parse error from %s: %v", addr, err)
		s.sendResponse(addr, "FAIL")
		return
	}

	user, err := s.userSvc.VerifyCredentials(username, password)
	if err != nil {
		log.Printf("Auth failed for %s from %s: %v", username, addr, err)
		s.sendResponse(addr, "FAIL")
		return
	}

	if err := s.userSvc.SetOnline(user.ID, true); err != nil {
		log.Printf("Set online failed for %s: %v", username, err)
	}

	log.Printf("Auth success: %s from %s", username, addr)
	s.sendResponse(addr, "OK")
}

func (s *Server) parseAuthRequest(msg string) (string, string, error) {
	parts := strings.SplitN(msg, " -d ", 2)
	if len(parts) != 2 {
		return "", "", fmt.Errorf("invalid format")
	}

	values, err := url.ParseQuery(parts[1])
	if err != nil {
		return "", "", err
	}

	username := values.Get("username")
	password := values.Get("pass_word")
	if password == "" {
		password = values.Get("pass_wd")
	}

	if username == "" || password == "" {
		return "", "", fmt.Errorf("missing credentials")
	}

	return username, password, nil
}

func (s *Server) sendResponse(addr *net.UDPAddr, msg string) {
	s.conn.WriteToUDP([]byte(msg), addr)
}
