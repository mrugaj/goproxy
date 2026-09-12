package balancer

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"io"
	"log"
	"net"
	"net/http"
	"sync/atomic"
)

const maxRequestBodySize = 10 << 20 // 10MB

// Retry budget, Envoy's model: retries may only consume retryBudgetRatio of
// the current in-flight load, with a small floor so low-traffic instances can
// still retry at all. Without this, a partial backend outage multiplies load
// on the survivors by maxRetries+1 at exactly the wrong moment.
const (
	retryBudgetRatio    = 0.2
	minRetryConcurrency = 3
)

type LoadBalancer struct {
	pool          *ServerPool
	maxRetries    int
	inFlight      atomic.Int64
	activeRetries atomic.Int64
}

func NewLoadBalancer(pool *ServerPool, maxRetries int) *LoadBalancer {
	return &LoadBalancer{
		pool:       pool,
		maxRetries: maxRetries,
	}
}

func isRetryableMethod(method string) bool {
	switch method {
	case http.MethodGet,
		http.MethodHead,
		http.MethodOptions:
		return true
	default:
		return false
	}
}

// responseRecorder reports whether anything has reached the client yet.
// Retrying is only safe while nothing has been written: ReverseProxy calls
// ErrorHandler from the protocol-upgrade path too, where the connection may
// already be written to or hijacked, so a nil-error check alone is not enough.
type responseRecorder struct {
	http.ResponseWriter
	wrote bool
}

func (r *responseRecorder) WriteHeader(code int) {
	r.wrote = true
	r.ResponseWriter.WriteHeader(code)
}

func (r *responseRecorder) Write(b []byte) (int, error) {
	r.wrote = true
	return r.ResponseWriter.Write(b)
}

// Unwrap lets http.ResponseController reach the underlying writer (flushing,
// deadlines); Hijack is asserted for directly by ReverseProxy on upgrades.
func (r *responseRecorder) Unwrap() http.ResponseWriter { return r.ResponseWriter }

func (r *responseRecorder) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	h, ok := r.ResponseWriter.(http.Hijacker)
	if !ok {
		return nil, nil, errors.New("ResponseWriter does not support hijacking")
	}
	r.wrote = true
	return h.Hijack()
}

// acquireRetry reserves a slot in the retry budget, reporting whether one was
// available. Every successful acquire must be released.
func (lb *LoadBalancer) acquireRetry() bool {

	limit := int64(float64(lb.inFlight.Load()) * retryBudgetRatio)
	if limit < minRetryConcurrency {
		limit = minRetryConcurrency
	}

	if lb.activeRetries.Add(1) > limit {
		lb.activeRetries.Add(-1)
		return false
	}

	return true
}

func (lb *LoadBalancer) releaseRetry() {
	lb.activeRetries.Add(-1)
}

func (lb *LoadBalancer) ServeHTTP(w http.ResponseWriter, r *http.Request) {

	lb.inFlight.Add(1)
	defer lb.inFlight.Add(-1)

	retryable := isRetryableMethod(r.Method)

	// Only buffer when the request may actually be replayed. Buffering a body
	// that will never be retried costs memory and blocks streaming for nothing.
	var bodyBytes []byte

	if retryable {
		r.Body = http.MaxBytesReader(w, r.Body, maxRequestBodySize)

		var err error
		bodyBytes, err = io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "Request body too large", http.StatusRequestEntityTooLarge)
			return
		}
	}

	var lastErr error

	attempts := 1
	if retryable {
		attempts = lb.maxRetries + 1
	}

	tried := make(map[*Server]bool, attempts)

	for attempt := 0; attempt < attempts; attempt++ {

		retrying := attempt > 0

		if retrying && !lb.acquireRetry() {
			log.Printf(
				"[RETRY BUDGET] exhausted method=%s path=%s inflight=%d",
				r.Method, r.URL.Path, lb.inFlight.Load(),
			)
			break
		}

		server := lb.pool.GetNextLeastConnections(tried)

		if server == nil {
			if retrying {
				lb.releaseRetry()
			}
			if lastErr == nil {
				http.Error(w, "No healthy backends available", http.StatusServiceUnavailable)
				return
			}
			break
		}

		tried[server] = true

		req := r.Clone(r.Context())
		if retryable {
			req.Body = io.NopCloser(bytes.NewReader(bodyBytes))
		}

		var proxyErr error

		ctx := context.WithValue(req.Context(), proxyErrKey{}, &proxyErr)
		req = req.WithContext(ctx)

		rec := &responseRecorder{ResponseWriter: w}

		func() {
			if retrying {
				defer lb.releaseRetry()
			}

			server.IncConnections()
			defer server.DecConnections()

			server.ServeHTTP(rec, req)
		}()

		// success
		if proxyErr == nil {
			server.RecordSuccess()

			if attempt > 0 {
				log.Printf(
					"[RECOVERY] method=%s path=%s backend=%s attempts=%d",
					r.Method,
					r.URL.Path,
					server.URL,
					attempt+1,
				)
			}

			return
		}

		lastErr = proxyErr

		log.Printf(
			"[RETRY] method=%s path=%s backend=%s attempt=%d error=%v",
			r.Method,
			r.URL.Path,
			server.URL,
			attempt+1,
			proxyErr,
		)

		// A client hanging up is not the backend's fault, so it must not count
		// toward ejection -- otherwise one impatient client can eject a
		// perfectly healthy backend.
		if errors.Is(proxyErr, context.Canceled) {
			return
		}

		server.RecordFailure()

		// The response already started: nothing left to retry onto.
		if rec.wrote {
			return
		}
	}

	// Detail goes to the log, not to the client -- the raw transport error
	// carries internal hostnames and ports.
	log.Printf(
		"[FAILED] method=%s path=%s attempts=%d error=%v",
		r.Method, r.URL.Path, len(tried), lastErr,
	)

	http.Error(w, "Bad Gateway", http.StatusBadGateway)
}
