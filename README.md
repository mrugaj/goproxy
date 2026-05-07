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
Routes incoming requests to the backend with the fewest active connections, helping distribute traffic efficiently under load.

## 🏥 Active Health Checks
Continuously monitors backend `/health` endpoints and automatically removes unhealthy servers from rotation until recovery.

## 🔄 Automatic Retry Mechanism
Failed requests are transparently retried against healthy backends without dropping the client connection.

## ⚙️ YAML-Based Configuration
All runtime behavior is configurable through a clean `config.yaml` file.

## 🛑 Graceful Shutdown
Handles `SIGINT` and `SIGTERM` signals to ensure in-flight requests complete safely before shutdown.

## 🔒 Concurrency Safe
Built using:

- `sync/atomic`
- `sync.RWMutex`

to prevent race conditions during concurrent traffic handling.

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
git clone https://github.com/yourusername/go-load-balancer.git
cd go-load-balancer
```

---

## 2️⃣ Install Dependencies

```bash
go mod tidy
```

---

## 3️⃣ Start Backend Servers

You can use the included backend servers (`be-1` and `be-2`) or any HTTP servers exposing a `/health` endpoint.

### Terminal 1

```bash
cd be-1
go run main.go
```

### Terminal 2

```bash
cd be-2
go run main.go
```

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
go run cmd/loadbalancer/main.go
```

---

# 🧪 Testing

## Send Traffic

```bash
curl http://localhost:8000
```

Requests should distribute dynamically based on active connection count.

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
- Least Connections Scheduling
- Distributed Systems Fundamentals
- Fault Tolerance
- Active Health Monitoring
- Retry Mechanisms
- Concurrency Safety
- Graceful Shutdown Handling
- Cloud-Native Inspired System Design

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
- Atomic Counters
- Mutex Synchronization

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