package localproxy

import (
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"strconv"
	"sync"
	"time"

	"golang.org/x/net/proxy"
	"proxydesk/internal/app"
)

type SOCKS5Server struct {
	route app.PortRoute
	ln    net.Listener
	mu    sync.Mutex
	OnLog func(format string, args ...any)
}

func NewSOCKS5Server(route app.PortRoute) *SOCKS5Server {
	return &SOCKS5Server{route: route}
}

func (s *SOCKS5Server) Start() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.ln != nil {
		return nil
	}
	host := s.route.LocalHost
	if host == "" {
		host = "127.0.0.1"
	}
	ln, err := net.Listen("tcp", net.JoinHostPort(host, strconv.Itoa(s.route.LocalHTTPPort)))
	if err != nil {
		return err
	}
	s.ln = ln
	go s.acceptLoop(ln)
	return nil
}

func (s *SOCKS5Server) Stop(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.ln == nil {
		return nil
	}
	err := s.ln.Close()
	s.ln = nil
	return err
}

func (s *SOCKS5Server) acceptLoop(ln net.Listener) {
	for {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		go s.handle(conn)
	}
}

func (s *SOCKS5Server) handle(client net.Conn) {
	defer client.Close()
	_ = client.SetDeadline(time.Now().Add(60 * time.Second))

	target, err := s.handshake(client)
	if err != nil {
		s.logf("SOCKS5 握手失败：%v", err)
		return
	}
	s.logf("SOCKS5 收到请求：CONNECT %s", target)

	upstream, err := s.dialTarget("tcp", target)
	if err != nil {
		s.logf("SOCKS5 连接上游失败：%s %v", target, err)
		_, _ = client.Write([]byte{0x05, 0x05, 0x00, 0x01, 0, 0, 0, 0, 0, 0})
		return
	}
	defer upstream.Close()

	_, _ = client.Write([]byte{0x05, 0x00, 0x00, 0x01, 0, 0, 0, 0, 0, 0})
	_ = client.SetDeadline(time.Time{})
	s.logf("SOCKS5 隧道已建立：%s", target)

	go proxyCopy(upstream, client)
	proxyCopy(client, upstream)
}

func (s *SOCKS5Server) handshake(conn net.Conn) (string, error) {
	header := make([]byte, 2)
	if _, err := io.ReadFull(conn, header); err != nil {
		return "", err
	}
	if header[0] != 0x05 {
		return "", fmt.Errorf("unsupported socks version %d", header[0])
	}
	methods := make([]byte, int(header[1]))
	if _, err := io.ReadFull(conn, methods); err != nil {
		return "", err
	}
	_, _ = conn.Write([]byte{0x05, 0x00})

	req := make([]byte, 4)
	if _, err := io.ReadFull(conn, req); err != nil {
		return "", err
	}
	if req[0] != 0x05 || req[1] != 0x01 {
		return "", fmt.Errorf("only CONNECT is supported")
	}

	host, err := readSOCKS5Host(conn, req[3])
	if err != nil {
		return "", err
	}
	portBytes := make([]byte, 2)
	if _, err := io.ReadFull(conn, portBytes); err != nil {
		return "", err
	}
	port := binary.BigEndian.Uint16(portBytes)
	return net.JoinHostPort(host, strconv.Itoa(int(port))), nil
}

func readSOCKS5Host(r io.Reader, atyp byte) (string, error) {
	switch atyp {
	case 0x01:
		buf := make([]byte, 4)
		if _, err := io.ReadFull(r, buf); err != nil {
			return "", err
		}
		return net.IP(buf).String(), nil
	case 0x03:
		lenBuf := make([]byte, 1)
		if _, err := io.ReadFull(r, lenBuf); err != nil {
			return "", err
		}
		buf := make([]byte, int(lenBuf[0]))
		if _, err := io.ReadFull(r, buf); err != nil {
			return "", err
		}
		return string(buf), nil
	case 0x04:
		buf := make([]byte, 16)
		if _, err := io.ReadFull(r, buf); err != nil {
			return "", err
		}
		return net.IP(buf).String(), nil
	default:
		return "", fmt.Errorf("unsupported address type %d", atyp)
	}
}

func (s *SOCKS5Server) dialTarget(network, addr string) (net.Conn, error) {
	switch s.route.Upstream.Protocol {
	case app.ProtocolHTTP:
		h := &HTTPServer{route: s.route}
		return h.dialTargetViaHTTPProxy(addr)
	case app.ProtocolSOCKS5:
		dialer, err := s.socks5Dialer()
		if err != nil {
			return nil, err
		}
		return dialer.Dial(network, addr)
	default:
		return nil, fmt.Errorf("unsupported upstream protocol %s", s.route.Upstream.Protocol)
	}
}

func (s *SOCKS5Server) socks5Dialer() (proxy.Dialer, error) {
	var auth *proxy.Auth
	if s.route.Upstream.Username != "" || s.route.Upstream.Password != "" {
		auth = &proxy.Auth{User: s.route.Upstream.Username, Password: s.route.Upstream.Password}
	}
	return proxy.SOCKS5("tcp", s.route.Upstream.Address(), auth, proxy.Direct)
}

func (s *SOCKS5Server) logf(format string, args ...any) {
	if s.OnLog != nil {
		s.OnLog(format, args...)
	}
}
