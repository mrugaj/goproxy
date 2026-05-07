package balancer

import (
	"context"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"sync/atomic"
	"time"
)

// proxyErrKey safely passes proxy transport errors
// back into the retry loop.
type proxyErrKey struct{}

type Server struct {
	URL         *url.URL
	proxy       *httputil.ReverseProxy
	activeConns atomic.Int64
	alive       atomic.Bool
}

func NewServer(rawURL string) (*Server, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, err
	}

	transport := &http.Transport{
		MaxIdleConns:          100,
		MaxIdleConnsPerHost:   20,
		IdleConnTimeout:       90 * time.Second,
		ResponseHeaderTimeout: 5 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		DialContext: (&net.Dialer{
			Timeout: 3 * time.Second,
		}).DialContext,
	}

	proxy := httputil.NewSingleHostReverseProxy(u)
	proxy.Transport = transport

	// Capture transport-level proxy failures
	proxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
		if errPtr, ok := r.Context().Value(proxyErrKey{}).(*error); ok {
			*errPtr = err
			return
		}

		http.Error(w, "Bad Gateway", http.StatusBadGateway)
	}

	s := &Server{
		URL:   u,
		proxy: proxy,
	}

	s.alive.Store(true)

	return s, nil
}

func (s *Server) IsAlive() bool {
	return s.alive.Load()
}

func (s *Server) SetAlive(alive bool) {
	s.alive.Store(alive)
}

func (s *Server) ActiveConnections() int64 {
	return s.activeConns.Load()
}

func (s *Server) IncConnections() {
	s.activeConns.Add(1)
}

func (s *Server) DecConnections() {
	s.activeConns.Add(-1)
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.proxy.ServeHTTP(w, r)
}

// optional helper for future graceful shutdown support
func (s *Server) Shutdown(ctx context.Context) error {
	return nil
}
