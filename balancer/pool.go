package balancer

import (
	"context"
	"log"
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

func (p *ServerPool) GetNextLeastConnections() *Server {

	var best *Server
	minConns := int64(-1)

	for _, s := range p.servers {

		if !s.IsAlive() {
			continue
		}

		conns := s.ActiveConnections()

		if minConns == -1 || conns < minConns {
			best = s
			minConns = conns
			continue
		}

		if conns == minConns {
			if time.Now().UnixNano()%2 == 0 {
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

	healthURL := server.URL.String() + "/health"

	resp, err := client.Get(healthURL)

	alive := false

	if err == nil {
		if resp.StatusCode == http.StatusOK {
			alive = true
		}
		resp.Body.Close()
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
