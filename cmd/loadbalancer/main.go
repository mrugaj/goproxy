package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/mrugaj/goproxy/balancer"
	"github.com/mrugaj/goproxy/config"
)

func main() {

	configPath := flag.String("config", "config.yaml", "path to the YAML config file")
	flag.Parse()

	cfg, err := config.LoadConfig(*configPath)
	if err != nil {
		log.Fatalf("failed to load config: %v", err)
	}

	pool, err := balancer.NewServerPool(cfg.Backends)
	if err != nil {
		log.Fatalf("failed to create backend pool: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	pool.StartHealthChecks(
		ctx,
		cfg.HealthCheckInterval,
	)

	lb := balancer.NewLoadBalancer(
		pool,
		cfg.MaxRetries,
	)

	server := &http.Server{
		Addr:        fmt.Sprintf(":%d", cfg.Port),
		Handler:     lb,
		ReadTimeout: 5 * time.Second,
		// No WriteTimeout: it caps total response duration, which truncates
		// streaming responses and large downloads. Upstream stalls are bounded
		// by the transport's ResponseHeaderTimeout instead.
		WriteTimeout:      0,
		IdleTimeout:       120 * time.Second,
		ReadHeaderTimeout: 2 * time.Second,
	}

	go func() {
		log.Printf("load balancer listening on :%d", cfg.Port)

		if err := server.ListenAndServe(); err != nil &&
			err != http.ErrServerClosed {

			log.Fatalf("server failed: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)

	signal.Notify(
		quit,
		syscall.SIGINT,
		syscall.SIGTERM,
	)

	<-quit

	log.Println("shutdown signal received")

	cancel()

	shutdownCtx, shutdownCancel := context.WithTimeout(
		context.Background(),
		10*time.Second,
	)
	defer shutdownCancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Fatalf("graceful shutdown failed: %v", err)
	}

	log.Println("server exited cleanly")
}
