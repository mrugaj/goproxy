package balancer

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
)

const maxRequestBodySize = 10 << 20 // 10MB

type LoadBalancer struct {
	pool       *ServerPool
	maxRetries int
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

func (lb *LoadBalancer) ServeHTTP(w http.ResponseWriter, r *http.Request) {

	// protect against giant bodies
	r.Body = http.MaxBytesReader(w, r.Body, maxRequestBodySize)

	var bodyBytes []byte
	var err error

	if r.Body != nil {
		bodyBytes, err = io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "Request body too large", http.StatusRequestEntityTooLarge)
			return
		}
	}

	retryable := isRetryableMethod(r.Method)

	var lastErr error

	attempts := 1
	if retryable {
		attempts = lb.maxRetries + 1
	}

	for attempt := 0; attempt < attempts; attempt++ {

		server := lb.pool.GetNextLeastConnections()

		if server == nil {
			http.Error(w, "No healthy backends available", http.StatusServiceUnavailable)
			return
		}

		req := r.Clone(r.Context())
		req.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))

		var proxyErr error

		ctx := context.WithValue(req.Context(), proxyErrKey{}, &proxyErr)
		req = req.WithContext(ctx)

		func() {
			server.IncConnections()
			defer server.DecConnections()

			server.ServeHTTP(w, req)
		}()

		// success
		if proxyErr == nil {
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
	}

	http.Error(
		w,
		fmt.Sprintf("Bad Gateway: %v", lastErr),
		http.StatusBadGateway,
	)
}
