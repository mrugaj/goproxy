package balancer

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

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

	selected := pool.GetNextLeastConnections(nil)

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

	selected := pool.GetNextLeastConnections(nil)

	if selected != s2 {
		t.Fatalf(
			"expected healthy server to be selected",
		)
	}
}

func TestTieBreakIsUniform(t *testing.T) {

	var servers []*Server
	for _, u := range []string{"http://a:1", "http://b:2", "http://c:3"} {
		s, err := NewServer(u)
		if err != nil {
			t.Fatalf("NewServer(%s): %v", u, err)
		}
		servers = append(servers, s)
	}

	pool := &ServerPool{servers: servers}

	const n = 30000
	counts := map[*Server]int{}
	for i := 0; i < n; i++ {
		counts[pool.GetNextLeastConnections(nil)]++
	}

	// Uniform would be n/3 each. A broken tie-break (e.g. clock parity) pins
	// everything to one backend, so anything outside +/-20% is a regression.
	for _, s := range servers {
		got := counts[s]
		if got < n/3-n/15 || got > n/3+n/15 {
			t.Fatalf("tie-break not uniform: %s got %d of %d", s.URL, got, n)
		}
	}
}

func TestRetrySkipsFailedBackend(t *testing.T) {

	good := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "GOOD")
	}))
	defer good.Close()

	dead := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	deadURL := dead.URL
	dead.Close() // refuses connections, but still marked alive

	sDead, _ := NewServer(deadURL)
	sGood, _ := NewServer(good.URL)

	// Make the dead backend the strict minimum, which is exactly what makes it
	// attractive: its failures are instant, so it always looks idle. Without an
	// exclusion set every attempt re-selects it and the client gets a 502.
	sGood.IncConnections()

	pool := &ServerPool{servers: []*Server{sDead, sGood}}
	front := httptest.NewServer(NewLoadBalancer(pool, 3))
	defer front.Close()

	resp, err := http.Get(front.URL)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK || string(body) != "GOOD" {
		t.Fatalf("retry did not reach the healthy backend: status=%d body=%q",
			resp.StatusCode, body)
	}
}

func TestNoHealthyBackends(t *testing.T) {

	s, _ := NewServer("http://127.0.0.1:1")
	s.SetAlive(false)

	front := httptest.NewServer(NewLoadBalancer(&ServerPool{servers: []*Server{s}}, 3))
	defer front.Close()

	resp, err := http.Get(front.URL)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("want 503, got %d", resp.StatusCode)
	}
}

func TestConnectionsReturnToZero(t *testing.T) {

	be := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "ok")
	}))
	defer be.Close()

	s, _ := NewServer(be.URL)
	front := httptest.NewServer(NewLoadBalancer(&ServerPool{servers: []*Server{s}}, 3))
	defer front.Close()

	for i := 0; i < 5; i++ {
		resp, err := http.Get(front.URL)
		if err != nil {
			t.Fatalf("request %d: %v", i, err)
		}
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
	}

	if got := s.ActiveConnections(); got != 0 {
		t.Fatalf("connection counter leaked: got %d, want 0", got)
	}
}

func TestRetryBudget(t *testing.T) {

	lb := NewLoadBalancer(&ServerPool{}, 3)

	// No traffic: the floor applies, so exactly minRetryConcurrency slots.
	for i := 0; i < minRetryConcurrency; i++ {
		if !lb.acquireRetry() {
			t.Fatalf("slot %d should be within the floor of %d", i, minRetryConcurrency)
		}
	}
	if lb.acquireRetry() {
		t.Fatal("budget should be exhausted past the floor")
	}

	lb.releaseRetry()
	if !lb.acquireRetry() {
		t.Fatal("releasing a slot should free budget")
	}
	for i := 0; i < minRetryConcurrency; i++ {
		lb.releaseRetry()
	}

	// Under load the ratio applies: 100 in flight => 20 retry slots.
	lb.inFlight.Store(100)
	for i := 0; i < 20; i++ {
		if !lb.acquireRetry() {
			t.Fatalf("slot %d should be within 20%% of 100 in-flight", i)
		}
	}
	if lb.acquireRetry() {
		t.Fatal("budget should cap at 20% of in-flight load")
	}
}

func TestPassiveEjection(t *testing.T) {

	dead := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	deadURL := dead.URL
	dead.Close() // refuses connections but is still marked alive

	s, err := NewServer(deadURL)
	if err != nil {
		t.Fatal(err)
	}

	front := httptest.NewServer(NewLoadBalancer(&ServerPool{servers: []*Server{s}}, 3))
	defer front.Close()

	// Each request records one failure against the only backend.
	for i := 0; i < passiveFailureThreshold; i++ {
		if s.IsAlive() != true {
			t.Fatalf("ejected after %d failures, want %d", i, passiveFailureThreshold)
		}
		resp, err := http.Get(front.URL)
		if err != nil {
			t.Fatalf("request %d: %v", i, err)
		}
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
	}

	if s.IsAlive() {
		t.Fatalf("backend should be ejected after %d consecutive failures",
			passiveFailureThreshold)
	}

	// The cooldown must hold the backend out even though /health would pass.
	if s.CanReadmit() {
		t.Fatal("an ejected backend should not be readmittable during its cooldown")
	}

	// Once the cooldown expires, readmission clears the streak.
	s.ejectedUntil.Store(0)
	if !s.CanReadmit() {
		t.Fatal("cooldown expired, backend should be readmittable")
	}
	s.SetAlive(true)
	if got := s.consecFails.Load(); got != 0 {
		t.Fatalf("readmission should reset the failure streak, got %d", got)
	}
}

func TestSuccessClearsFailureStreak(t *testing.T) {

	be := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "ok")
	}))
	defer be.Close()

	s, _ := NewServer(be.URL)
	s.RecordFailure()
	s.RecordFailure()

	front := httptest.NewServer(NewLoadBalancer(&ServerPool{servers: []*Server{s}}, 3))
	defer front.Close()

	resp, err := http.Get(front.URL)
	if err != nil {
		t.Fatal(err)
	}
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()

	if got := s.consecFails.Load(); got != 0 {
		t.Fatalf("a success should clear the streak, got %d", got)
	}
}
