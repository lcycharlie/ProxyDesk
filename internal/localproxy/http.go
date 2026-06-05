package localproxy

import (
	"bufio"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"sync"
	"time"

	"proxydesk/internal/app"
)

type HTTPServer struct {
	route  app.PortRoute
	server *http.Server
	ln     net.Listener
	mu     sync.Mutex
	OnLog  func(format string, args ...any)
}

func NewHTTPServer(route app.PortRoute) *HTTPServer {
	return &HTTPServer{route: route}
}

func (s *HTTPServer) Start() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.server != nil {
		return nil
	}

	addr := "127.0.0.1:" + strconv.Itoa(s.route.LocalHTTPPort)
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/", s.handle)
	s.server = &http.Server{
		Handler:           mux,
		ReadHeaderTimeout: 15 * time.Second,
	}
	s.ln = ln

	go func() {
		if err := s.server.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Printf("local HTTP proxy stopped: %v", err)
		}
	}()
	return nil
}

func (s *HTTPServer) Stop(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.server == nil {
		return nil
	}
	err := s.server.Shutdown(ctx)
	s.server = nil
	s.ln = nil
	return err
}

func (s *HTTPServer) handle(w http.ResponseWriter, r *http.Request) {
	s.logf("收到请求：%s %s", r.Method, requestTarget(r))
	if r.Method == http.MethodConnect {
		s.handleConnect(w, r)
		return
	}
	s.handleHTTP(w, r)
}

func (s *HTTPServer) handleHTTP(w http.ResponseWriter, r *http.Request) {
	upstreamURL := &url.URL{Scheme: "http", Host: s.route.Upstream.Address()}
	if s.route.Upstream.Username != "" || s.route.Upstream.Password != "" {
		upstreamURL.User = url.UserPassword(s.route.Upstream.Username, s.route.Upstream.Password)
	}

	transport := &http.Transport{
		Proxy: http.ProxyURL(upstreamURL),
		DialContext: (&net.Dialer{
			Timeout:   20 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		TLSHandshakeTimeout: 20 * time.Second,
	}
	defer transport.CloseIdleConnections()

	outReq := r.Clone(r.Context())
	outReq.RequestURI = ""
	outReq.URL.Scheme = r.URL.Scheme
	outReq.URL.Host = r.URL.Host
	removeHopHeaders(outReq.Header)

	resp, err := transport.RoundTrip(outReq)
	if err != nil {
		s.logf("HTTP 转发失败：%s %v", requestTarget(r), err)
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()
	s.logf("HTTP 转发完成：%s %s", requestTarget(r), resp.Status)

	removeHopHeaders(resp.Header)
	for k, values := range resp.Header {
		for _, value := range values {
			w.Header().Add(k, value)
		}
	}
	w.WriteHeader(resp.StatusCode)
	_, _ = io.Copy(w, resp.Body)
}

func (s *HTTPServer) handleConnect(w http.ResponseWriter, r *http.Request) {
	hijacker, ok := w.(http.Hijacker)
	if !ok {
		http.Error(w, "hijacking is not supported", http.StatusInternalServerError)
		return
	}

	upstreamConn, err := net.DialTimeout("tcp", s.route.Upstream.Address(), 20*time.Second)
	if err != nil {
		s.logf("CONNECT 连接上游失败：%s %v", r.Host, err)
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}

	connectReq := fmt.Sprintf("CONNECT %s HTTP/1.1\r\nHost: %s\r\n", r.Host, r.Host)
	if s.route.Upstream.Username != "" || s.route.Upstream.Password != "" {
		token := base64.StdEncoding.EncodeToString([]byte(s.route.Upstream.Username + ":" + s.route.Upstream.Password))
		connectReq += "Proxy-Authorization: Basic " + token + "\r\n"
	}
	connectReq += "\r\n"

	if _, err := upstreamConn.Write([]byte(connectReq)); err != nil {
		_ = upstreamConn.Close()
		s.logf("CONNECT 写入上游失败：%s %v", r.Host, err)
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}

	br := bufio.NewReader(upstreamConn)
	resp, err := http.ReadResponse(br, r)
	if err != nil {
		_ = upstreamConn.Close()
		s.logf("CONNECT 读取上游响应失败：%s %v", r.Host, err)
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	if resp.StatusCode != http.StatusOK {
		_ = upstreamConn.Close()
		s.logf("CONNECT 上游拒绝：%s %s", r.Host, resp.Status)
		http.Error(w, "upstream CONNECT failed: "+resp.Status, http.StatusBadGateway)
		return
	}

	clientConn, _, err := hijacker.Hijack()
	if err != nil {
		_ = upstreamConn.Close()
		s.logf("CONNECT 接管客户端失败：%s %v", r.Host, err)
		return
	}
	_, _ = clientConn.Write([]byte("HTTP/1.1 200 Connection Established\r\n\r\n"))
	s.logf("CONNECT 隧道已建立：%s", r.Host)

	go proxyCopy(upstreamConn, clientConn)
	go proxyCopy(clientConn, upstreamConn)
}

func (s *HTTPServer) logf(format string, args ...any) {
	if s.OnLog != nil {
		s.OnLog(format, args...)
	}
}

func requestTarget(r *http.Request) string {
	if r.URL != nil && r.URL.String() != "" {
		return r.URL.String()
	}
	return r.Host
}

func proxyCopy(dst net.Conn, src net.Conn) {
	defer dst.Close()
	defer src.Close()
	_, _ = io.Copy(dst, src)
}

func removeHopHeaders(h http.Header) {
	for _, header := range []string{
		"Connection",
		"Proxy-Connection",
		"Keep-Alive",
		"Proxy-Authenticate",
		"Proxy-Authorization",
		"Te",
		"Trailer",
		"Transfer-Encoding",
		"Upgrade",
	} {
		h.Del(header)
	}
}
