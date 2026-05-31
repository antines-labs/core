package main

import (
	"flag"
	"log"
	"time"

	"github.com/antines/core/internal/server"
)

func main() {
	port := flag.Int("port", 3000, "HTTP server port")
	manifestPath := flag.String("manifest", "antines-manifest.json", "path to manifest JSON")
	workers := flag.Int("workers", 4, "number of JS worker processes")
	timeout := flag.Duration("timeout", 10*time.Second, "worker request timeout")
	flag.Parse()

	cfg := server.Config{
		Port:          *port,
		ManifestPath:  *manifestPath,
		WorkerCount:   *workers,
		WorkerTimeout: *timeout,
	}

	srv := server.New(cfg)

	go func() {
		if err := srv.Start(); err != nil {
			log.Fatalf("Server error: %v", err)
		}
	}()

	srv.WaitForShutdown()
}
