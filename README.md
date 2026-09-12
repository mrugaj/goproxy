# 🚀 Cloud-Native Go Load Balancer (Layer 7)

A lightweight Layer 7 HTTP Load Balancer built entirely in Go, designed with Cloud-Native Inspired principles and distributed systems concepts in mind.

This project avoids heavyweight frameworks and instead leverages Go’s powerful standard library to implement:

- intelligent traffic routing
- fault tolerance
- active health monitoring
- graceful shutdown handling
- concurrency-safe backend management

Built to explore distributed systems, reverse proxying, and cloud-native networking concepts using Go.

---

# ✨ Features

## ⚖️ Least Connections Load Balancing
Routes each request to the healthy backend with the fewest **in-flight requests**
(not TCP connections — keep-alive means one connection carries many requests).
Ties are broken by uniform reservoir sampling.

## 🏥 Active Health Checks
Probes every backend's `/health` endpoint in parallel on a configurable interval
(first probe runs immediately at startup), removing non-200 backends from
rotation until they recover.

## 🔄 Automatic Retry Mechanism
Idempotent requests (`GET`, `HEAD`, `OPTIONS`) are retried on transport-level
failure. Each attempt excludes the backends already tried, so a retry always
moves to a different backend. Retries stop early if the client disconnects or
if any part of the response has already reached the client.

## 🚫 Passive Outlier Ejection
Active probing is blind to a backend that answers `/health` with `200` while
failing every real request. Five consecutive real-request failures eject a
backend for 30s, and the cooldown outranks the health checker — otherwise a
lying backend is readmitted on the very next tick and simply flaps.

## 🪣 Retry Budget
Retries may consume at most 20% of in-flight load (with a floor of 3, so
low-traffic instances can still retry). Without a budget, a partial backend
outage multiplies load on the survivors by `max_retries + 1` at exactly the
moment they can least absorb it.

## ⚙️ YAML-Based Configuration
All runtime behavior is configurable through a clean `config.yaml` file, passed
with `-config`. The config is validated at startup — a bad port, non-positive
health interval, negative retry count, empty backend list, or malformed backend
URL fails fast with a clear message instead of at request time.

## 🛑 Graceful Shutdown
Handles `SIGINT` and `SIGTERM` signals to ensure in-flight requests complete safely before shutdown.

## 🔒 Concurrency Safe
Lock-free by design. Per-backend state uses:

- `atomic.Int64` — in-flight request count
- `atomic.Bool` — liveness flag

No mutex is needed: the backend set is immutable after startup, so concurrent
readers are safe without synchronization. Verified under `go test -race`.

---

# 🏗️ Architecture Overview

```text
             🌐 Client Requests
                    │
                    ▼
        ┌─────────────────────┐
        │  Load Balancer :8000│
        │ Least Connections   │
        └─────────┬───────────┘
                  │
      ┌───────────┼───────────┐
      ▼           ▼           ▼

 Backend-1     Backend-2    Backend-3
 Active: 2     Active: 5    Unhealthy
     ✅             ✅            ❌
```

---

# 📂 Project Structure

```text
goproxy/
├── cmd/
│   └── loadbalancer/
│       └── main.go
│
├── balancer/
│   ├── balancer.go
│   ├── pool.go
│   └── server.go
│
├── config/
│   └── config.go
│
├── config.yaml
├── go.mod
└── README.md
```

### Structure Explanation

| File | Purpose |
|------|----------|
| `main.go` | Application entry point and graceful shutdown handling |
| `balancer.go` | Core request routing and retry logic |
| `pool.go` | Backend pool management and health checks |
| `server.go` | Backend state management and reverse proxy handling |
| `config.go` | YAML parsing and configuration loading |

---

# 🛠️ Getting Started

## Prerequisites

- Go `1.22+`

---

## 1️⃣ Clone the Repository

```bash
git clone https://github.com/mrugaj/goproxy.git
cd goproxy
```

---

## 2️⃣ Install Dependencies

```bash
go mod tidy
```

---

## 3️⃣ Start Backend Servers

Any HTTP servers exposing a `/health` endpoint that returns `200` will work.
A minimal one:

```go
package main

import ("fmt"; "net/http"; "os")

func main() {
	port := os.Args[1]
	http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) })
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) { fmt.Fprintf(w, "backend-%s\n", port) })
	http.ListenAndServe(":"+port, nil)
}
```

Run two copies on `8080` and `8081`.

---

## 4️⃣ Configure the Load Balancer

Update `config.yaml`:

```yaml
port: 8000
health_check_interval: 5s
max_retries: 3

backends:
  - "http://localhost:8080"
  - "http://localhost:8081"
```

---

## 5️⃣ Run the Load Balancer

```bash
go run ./cmd/loadbalancer -config config.yaml
```

`-config` defaults to `config.yaml` in the working directory.

---

# 🧪 Testing

## Unit & Integration Tests

```bash
go test -race ./...
```

Covers selection fairness, retry backend-exclusion, retry-budget limits,
passive ejection and its cooldown, connection-counter balance, the
no-healthy-backends path, and config validation.

## Send Traffic

```bash
curl http://localhost:8000
```

Requests should distribute dynamically based on in-flight request count.

---

## Test Fault Tolerance

1. Stop one backend server:

```bash
Ctrl + C
```

2. Immediately send another request:

```bash
curl http://localhost:8000
```

3. The load balancer will:

- detect the failed backend
- retry another healthy backend
- return a successful response transparently

---

## Test Graceful Shutdown

Stop the load balancer:

```bash
Ctrl + C
```

The application will wait for in-flight requests to finish before shutting down safely.

---

# ⚡ Core Concepts Demonstrated

- Reverse Proxying
- Layer 7 Load Balancing
- Passive Outlier Ejection
- Retry Budgets
- Least Connections Scheduling
- Distributed Systems Fundamentals
- Fault Tolerance
- Active Health Monitoring
- Retry Mechanisms
- Concurrency Safety
- Graceful Shutdown Handling
- Cloud-Native Inspired System Design

---

# ⚠️ Known Limitations

Deliberate scope boundaries, not oversights:

| Limitation | Impact |
|---|---|
| **No retry backoff or jitter** | Retries are immediate. The retry budget caps concurrency, but there is no delay between attempts. |
| **Ejection thresholds are constants** | `passiveFailureThreshold`, `ejectionDuration`, and the retry budget ratio are compile-time constants, not config. |
| **No health-check hysteresis** | A single failed probe ejects a backend; a single success readmits it. No flap damping. |
| **Per-process connection counts** | Running multiple replicas degrades least-connections toward random, since each replica only sees its own traffic. P2C would be the fix. |
| **No TLS, auth, or rate limiting** | The listener is plain HTTP and there is no concurrency cap. |
| **No metrics or structured logging** | Observability is `log.Printf` to stderr. |

---

# 🚀 Future Enhancements

## 🔍 Dynamic Service Discovery
Integrate with tools like:

- Consul
- etcd
- Kubernetes API

to dynamically discover backend services.

---

## 📊 Prometheus Metrics
Expose a `/metrics` endpoint for:

- request latency
- error rates
- active connections
- backend health statistics

---

## ⚖️ Weighted Load Balancing
Allow more powerful servers to receive a larger percentage of traffic.

---

## 🌐 HTTP/2 & gRPC Support
Extend support for modern Cloud-Native Inspired communication protocols.

---

# 🧠 Tech Stack

- Go
- `net/http`
- `httputil.ReverseProxy`
- YAML
- Goroutines
- Atomic Counters (lock-free)

---

# 📸 Example Workflow

```text
Client Request
      │
      ▼
Load Balancer Receives Request
      │
      ▼
Select Healthy Backend
(Least Connections)
      │
      ▼
Forward Request
      │
      ▼
Backend Responds
      │
      ▼
Response Returned to Client
```

---

# 🤝 Why This Project?

This project was built to deepen understanding of:

- distributed systems
- Cloud-Native Inspired networking
- concurrent programming in Go
- production-grade infrastructure design

while keeping the implementation lightweight, readable, and framework-independent.

---

# ❤️ Built With Go

Designed and implemented using Go’s standard library with a focus on simplicity, reliability, and systems-level engineering.