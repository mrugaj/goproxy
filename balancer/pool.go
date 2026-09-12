package balancer

import (
	"context"
	"io"
	"log"
	"math/rand/v2"
	"net/http"
	"sync"
	"time"
)

type ServerPool struct {
	servers []*Server
}

func NewServerPool(backendURLs []string) (*ServerPool, error) {

	var servers []*Server

	for _, rawURL := range backendURLs {

		s, err := NewServer(rawURL)
		if err != nil {
			return nil, err
		}

		servers = append(servers, s)
	}

	return &ServerPool{
		servers: servers,
	}, nil
}

// GetNextLeastConnections returns the alive backend with the fewest in-flight
// requests, skipping any backend in tried (nil means "none tried"). Ties are
// broken by reservoir sampling so each tied backend is equally likely -- a
// clock-parity coin flip is not random, since UnixNano is always even on
// platforms whose clock is coarser than a nanosecond.
func (p *ServerPool) GetNextLeastConnections(tried map[*Server]bool) *Server {

	var best *Server
	var minConns int64
	ties := 0

	for _, s := range p.servers {

		if !s.IsAlive() || tried[s] {
			continue
		}

		conns := s.ActiveConnections()

		if best == nil || conns < minConns {
			best = s
			minConns = conns
			ties = 1
			continue
		}

		if conns == minConns {
			ties++
			if rand.IntN(ties) == 0 {
				best = s
			}
		}
	}

	return best
}

func (p *ServerPool) StartHealthChecks(
	ctx context.Context,
	interval time.Duration,
) {

	ticker := time.NewTicker(interval)

	go func() {
		defer ticker.Stop()

		// Probe immediately: NewTicker does not fire until the first interval
		// elapses, so without this every backend is assumed alive until then.
		p.healthCheck()

		for {
			select {

			case <-ticker.C:
				p.healthCheck()

			case <-ctx.Done():
				log.Println("[HEALTHCHECK] stopping")
				return
			}
		}
	}()
}

func checkServerHealth(server *Server, client *http.Client) {

	healthURL := server.URL.JoinPath("/health").String()

	resp, err := client.Get(healthURL)

	alive := false

	if err == nil {
		if resp.StatusCode == http.StatusOK {
			alive = true
		}
		// Drain before closing so the connection can be pooled and reused.
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
	}

	// A passively-ejected backend stays out until its cooldown expires, even
	// if it keeps answering /health with 200 -- that is the whole point.
	if alive && !server.CanReadmit() {
		return
	}

	if alive != server.IsAlive() {

		if alive {
			log.Printf("[BACKEND HEALTHY] %s", server.URL)
		} else {
			log.Printf("[BACKEND UNHEALTHY] %s", server.URL)
		}

		server.SetAlive(alive)
	}
}

func (p *ServerPool) healthCheck() {

	client := &http.Client{
		Timeout: 2 * time.Second,
	}

	var wg sync.WaitGroup

	for _, server := range p.servers {

		wg.Add(1)

		go func(s *Server) {
			defer wg.Done()
			checkServerHealth(s, client)
		}(server)
	}

	wg.Wait()
}
