package balancer

import (
	"log"
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

// passiveFailureThreshold is how many consecutive real-request failures eject
// a backend. Active probing alone is blind to a backend that answers /health
// with 200 while failing everything else.
const passiveFailureThreshold = 5

// ejectionDuration is how long a passively-ejected backend stays out. Without
// it the active health checker readmits a backend that answers /health with
// 200 on its very next tick, and the backend just flaps.
const ejectionDuration = 30 * time.Second

type Server struct {
	URL         *url.URL
	proxy       *httputil.ReverseProxy
	activeConns atomic.Int64
	alive       atomic.Bool
	consecFails atomic.Int64

	// ejectedUntil is a unix-nano deadline before which health probes must not
	// readmit this backend. Zero means "not ejected".
	ejectedUntil atomic.Int64
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
	if alive {
		// Readmission clears the streak, or a backend would be re-ejected by
		// the very next failure after recovering.
		s.consecFails.Store(0)
	}
	s.alive.Store(alive)
}

// RecordSuccess clears the passive failure streak.
func (s *Server) RecordSuccess() {
	s.consecFails.Store(0)
}

// RecordFailure counts a real request failure and ejects the backend once it
// has failed passiveFailureThreshold times in a row. Recovery is the active
// health checker's job: it readmits the backend on the next successful probe.
func (s *Server) RecordFailure() {
	if s.consecFails.Add(1) >= passiveFailureThreshold &&
		s.alive.CompareAndSwap(true, false) {

		s.ejectedUntil.Store(time.Now().Add(ejectionDuration).UnixNano())

		log.Printf(
			"[BACKEND EJECTED] %s for %v after %d consecutive request failures",
			s.URL,
			ejectionDuration,
			passiveFailureThreshold,
		)
	}
}

// CanReadmit reports whether a passive ejection has expired, so a successful
// health probe is allowed to put this backend back into rotation.
func (s *Server) CanReadmit() bool {
	return time.Now().UnixNano() >= s.ejectedUntil.Load()
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
