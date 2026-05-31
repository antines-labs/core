package main

import (
	"flag"
	"log"
	"time"

	"github.com/antines-labs/core/internal/server"
)

func main() {
	port := flag.Int("port", 3000, "HTTP server port")
	manifestPath := flag.String("manifest", "antines-manifest.json", "path to manifest JSON")
	workers := flag.Int("workers", 4, "number of JS worker processes")
	timeout := flag.Duration("timeout", 10*time.Second, "worker request timeout")
	workerEntry := flag.String("worker-entry", "", "path to worker JS entry point")
	bunBinary := flag.String("bun", "bun", "bun binary path")
	flag.Parse()

	cfg := server.Config{
		Port:          *port,
		ManifestPath:  *manifestPath,
		WorkerCount:   *workers,
		WorkerTimeout: *timeout,
		WorkerEntry:   *workerEntry,
		BunBinary:     *bunBinary,
	}

	srv := server.New(cfg)

	go func() {
		if err := srv.Start(); err != nil {
			log.Fatalf("Server error: %v", err)
		}
	}()

	srv.WaitForShutdown()
}
