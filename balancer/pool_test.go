package balancer

import "testing"

func TestLeastConnections(t *testing.T) {

	s1, err := NewServer("http://localhost:8080")
	if err != nil {
		t.Fatalf("failed to create server 1: %v", err)
	}

	s2, err := NewServer("http://localhost:8081")
	if err != nil {
		t.Fatalf("failed to create server 2: %v", err)
	}

	s3, err := NewServer("http://localhost:8082")
	if err != nil {
		t.Fatalf("failed to create server 3: %v", err)
	}

	// simulate active connections
	s1.IncConnections()
	s1.IncConnections()

	s2.IncConnections()

	pool := &ServerPool{
		servers: []*Server{s1, s2, s3},
	}

	selected := pool.GetNextLeastConnections()

	if selected != s3 {
		t.Fatalf(
			"expected server 3 with least connections, got %v",
			selected.URL,
		)
	}
}

func TestDeadServersIgnored(t *testing.T) {

	s1, _ := NewServer("http://localhost:8080")
	s2, _ := NewServer("http://localhost:8081")

	s1.SetAlive(false)

	pool := &ServerPool{
		servers: []*Server{s1, s2},
	}

	selected := pool.GetNextLeastConnections()

	if selected != s2 {
		t.Fatalf(
			"expected healthy server to be selected",
		)
	}
}
