package server

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"github.com/antines-labs/core/internal/ipc"
	"github.com/antines-labs/core/internal/manifest"
	"github.com/antines-labs/core/internal/router"
	"github.com/antines-labs/core/internal/validator"
	"github.com/antines-labs/core/internal/worker"
)

// Config holds the server configuration.
type Config struct {
	Port          int
	ManifestPath  string
	WorkerCount   int
	WorkerTimeout time.Duration
	WorkerEntry   string // path to the worker JS entry point
	BunBinary     string // bun binary path (default: "bun")
	WorkerDir     string // temp dir for worker sockets (default: temp dir)
}

// DefaultConfig returns a sensible default configuration.
func DefaultConfig() Config {
	return Config{
		Port:          3000,
		ManifestPath:  "antines-manifest.json",
		WorkerCount:   4,
		WorkerTimeout: 10 * time.Second,
		BunBinary:     "bun",
	}
}

// RouteEntry holds the compiled information for a single route.
type RouteEntry struct {
	Route           manifest.RouteManifest
	InputLayout     *ipc.CompiledLayout
	OutputLayout    *ipc.CompiledLayout
	InputValidator  validator.Node
	OutputValidator validator.Node
}

// Server is the main HTTP server.
type Server struct {
	config    Config
	manifest  *manifest.Manifest
	trie      *router.Trie
	registry  map[int]*RouteEntry
	pool      *worker.Pool
	httpSrv   *http.Server
	startTime time.Time
	mu        sync.Mutex
	started   bool
}

// New creates a new server with the given config.
func New(config Config) *Server {
	return &Server{
		config:    config,
		startTime: time.Now(),
	}
}

// Start initializes all components and starts the HTTP server.
func (s *Server) Start() error {
	s.mu.Lock()
	if s.started {
		s.mu.Unlock()
		return fmt.Errorf("server: already started")
	}
	s.started = true
	s.mu.Unlock()

	log.Printf("Loading manifest from %s...", s.config.ManifestPath)

	m, err := manifest.Load(s.config.ManifestPath)
	if err != nil {
		return fmt.Errorf("server: load manifest: %w", err)
	}
	s.manifest = m

	log.Printf("Building trie with %d routes...", len(m.Routes))

	tr := router.New()
	reg := make(map[int]*RouteEntry, len(m.Routes))

	for _, route := range m.Routes {
		if err := tr.Insert(route.Method, route.Path, route.HandlerID); err != nil {
			return fmt.Errorf("server: trie insert %s %s: %w", route.Method, route.Path, err)
		}

		entry := &RouteEntry{
			Route: route,
		}

		if route.Schema.Input != nil {
			layout, err := ipc.CalculateLayout(route.Schema.Input)
			if err != nil {
				return fmt.Errorf("server: layout input %s %s: %w", route.Method, route.Path, err)
			}
			entry.InputLayout = layout

			v, err := validator.Compile(route.Schema.Input)
			if err != nil {
				return fmt.Errorf("server: validate input %s %s: %w", route.Method, route.Path, err)
			}
			entry.InputValidator = v
		}

		if route.Schema.Output != nil {
			layout, err := ipc.CalculateLayout(route.Schema.Output)
			if err != nil {
				return fmt.Errorf("server: layout output %s %s: %w", route.Method, route.Path, err)
			}
			entry.OutputLayout = layout

			v, err := validator.Compile(route.Schema.Output)
			if err != nil {
				return fmt.Errorf("server: validate output %s %s: %w", route.Method, route.Path, err)
			}
			entry.OutputValidator = v
		}

		reg[route.HandlerID] = entry
	}
	s.trie = tr
	s.registry = reg

	hasJS := false
	for _, route := range m.Routes {
		if route.HasHandler {
			hasJS = true
			break
		}
	}

	if hasJS {
		log.Printf("Starting worker pool (%d workers)...", s.config.WorkerCount)
		poolCfg := worker.Config{
			WorkerCount: s.config.WorkerCount,
			Timeout:     s.config.WorkerTimeout,
			MaxRetries:  2,
		}
		s.pool = worker.NewPool(poolCfg)
		if err := s.spawnWorkers(); err != nil {
			return fmt.Errorf("server: spawn workers: %w", err)
		}
	} else {
		log.Println("No JS handlers — skipping worker pool")
	}

	return s.serveHTTP()
}

func (s *Server) spawnWorkers() error {
	if s.config.WorkerEntry == "" {
		return fmt.Errorf("worker entry not configured")
	}
	workerDir := s.config.WorkerDir
	if workerDir == "" {
		var err error
		workerDir, err = os.MkdirTemp("", "antines-worker-*")
		if err != nil {
			return fmt.Errorf("create worker dir: %w", err)
		}
	}

	for i := range s.config.WorkerCount {
		socketPath := filepath.Join(workerDir, fmt.Sprintf("worker-%d.sock", i))

		listener, err := net.Listen("unix", socketPath)
		if err != nil {
			return fmt.Errorf("listen worker %d: %w", i, err)
		}

		//nolint:gosec // G204: BunBinary and WorkerEntry are provided via CLI flags by the user
		cmd := exec.Command(s.config.BunBinary, "run", s.config.WorkerEntry,
			"--socket", socketPath,
			"--manifest", s.config.ManifestPath,
		)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr

		if err := cmd.Start(); err != nil {
			listener.Close()
			return fmt.Errorf("start worker %d: %w", i, err)
		}

		conn, err := listener.Accept()
		if err != nil {
			_ = cmd.Process.Kill()
			listener.Close()
			return fmt.Errorf("accept worker %d: %w", i, err)
		}

		listener.Close()
		s.pool.AddConn(uint32(i+1), conn)
		log.Printf("Worker %d connected (socket: %s)", i+1, socketPath)
	}

	return nil
}

func (s *Server) serveHTTP() error {
	mux := http.NewServeMux()
	mux.HandleFunc("/", s.handler)

	addr := fmt.Sprintf(":%d", s.config.Port)
	s.httpSrv = &http.Server{
		Addr:              addr,
		Handler:           recoveryMiddleware(requestIDMiddleware(loggerMiddleware(mux))),
		ReadHeaderTimeout: 30 * time.Second,
	}

	log.Printf("HTTP server listening on %s", addr)
	return s.httpSrv.ListenAndServe()
}

// WaitForShutdown blocks until a signal is received, then shuts down gracefully.
func (s *Server) WaitForShutdown() {
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	sig := <-sigCh
	log.Printf("Received signal %v, shutting down...", sig)
	if err := s.Shutdown(); err != nil {
		log.Printf("shutdown error: %v", err)
	}
}

// Shutdown gracefully stops the server.
func (s *Server) Shutdown() error {
	log.Println("Shutting down server...")

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	if s.httpSrv != nil {
		if err := s.httpSrv.Shutdown(ctx); err != nil {
			log.Printf("HTTP server shutdown: %v", err)
		}
	}

	if s.pool != nil {
		if err := s.pool.Shutdown(); err != nil {
			log.Printf("Worker pool shutdown: %v", err)
		}
	}

	return nil
}

// ---- HTTP handler ----

type responseWriter struct {
	http.ResponseWriter
	statusCode int
	written    bool
}

func (rw *responseWriter) WriteHeader(code int) {
	if !rw.written {
		rw.statusCode = code
		rw.written = true
		rw.ResponseWriter.WriteHeader(code)
	}
}

func (rw *responseWriter) Write(b []byte) (int, error) {
	if !rw.written {
		rw.WriteHeader(http.StatusOK)
	}
	return rw.ResponseWriter.Write(b)
}

func (s *Server) handler(w http.ResponseWriter, r *http.Request) {
	rw := &responseWriter{ResponseWriter: w, statusCode: http.StatusOK}
	method := r.Method
	path := r.URL.Path

	result, err := s.trie.Match(method, path)
	if err != nil {
		writeJSON(rw, http.StatusInternalServerError, map[string]string{"error": "route match error"})
		return
	}
	if result == nil {
		writeJSON(rw, http.StatusNotFound, map[string]string{"error": "not found"})
		return
	}

	entry, ok := s.registry[result.Route.HandlerID]
	if !ok {
		writeJSON(rw, http.StatusInternalServerError, map[string]string{"error": "route not registered"})
		return
	}

	requestID := r.Context().Value(ctxKeyRequestID).(string)

	if !entry.Route.HasHandler {
		s.handleGoOnly(rw, r, entry, result, requestID)
		return
	}

	s.handleJSHander(rw, r, entry, result, requestID)
}

func (s *Server) handleGoOnly(rw *responseWriter, r *http.Request, entry *RouteEntry, result *router.MatchResult, requestID string) {
	_ = r
	_ = requestID

	switch {
	case entry.Route.Method == "GET" && entry.Route.Path == "/health":
		resp := map[string]interface{}{
			"status": "ok",
			"uptime": time.Since(s.startTime).Seconds(),
		}
		writeJSON(rw, http.StatusOK, resp)
		return

	case entry.Route.Method == "GET" && entry.Route.Path == "/version":
		writeJSON(rw, http.StatusOK, map[string]string{
			"version": "0.1.0",
		})
		return

	default:
		writeJSON(rw, http.StatusNotImplemented, map[string]string{
			"error": "route has no JS handler and no built-in Go handler",
		})
	}
}

func (s *Server) handleJSHander(rw *responseWriter, r *http.Request, entry *RouteEntry, result *router.MatchResult, requestID string) {
	var inputData map[string]interface{}

	if entry.Route.Schema.Input != nil {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			writeJSON(rw, http.StatusBadRequest, map[string]string{"error": "cannot read body"})
			return
		}
		defer r.Body.Close()

		if len(body) > 0 {
			if err := json.Unmarshal(body, &inputData); err != nil {
				writeJSON(rw, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
				return
			}
		}

		if inputData == nil {
			inputData = make(map[string]interface{})
		}

		// Add query params to input
		for k, vals := range r.URL.Query() {
			if len(vals) > 0 {
				inputData[k] = vals[0]
			}
		}

		// Add path params to input data
		for k, v := range result.Params {
			inputData["params."+k] = v
		}

		// Validate input
		if entry.InputValidator != nil {
			errs := entry.InputValidator.Validate(inputData)
			if errs != nil {
				writeJSON(rw, http.StatusUnprocessableEntity, map[string]interface{}{
					"error":  "validation failed",
					"fields": errs,
				})
				return
			}
		}
	}

	if s.pool == nil {
		writeJSON(rw, http.StatusServiceUnavailable, map[string]string{
			"error": "worker pool not available",
		})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), s.config.WorkerTimeout)
	defer cancel()

	resultData, err := s.pool.Dispatch(
		ctx,
		uint32(entry.Route.HandlerID),
		uint32(hashRequestID(requestID)),
		entry.InputLayout,
		entry.OutputLayout,
		inputData,
	)
	if err != nil {
		writeJSON(rw, http.StatusInternalServerError, map[string]string{
			"error": fmt.Sprintf("dispatch failed: %v", err),
		})
		return
	}

	if resultData.Output != nil {
		if entry.OutputValidator != nil {
			errs := entry.OutputValidator.Validate(resultData.Output)
			if errs != nil {
				log.Printf("Output validation failed for route %s %s: %v", entry.Route.Method, entry.Route.Path, errs)
			}
		}
	}

	writeJSON(rw, int(resultData.StatusCode), resultData.Output)
}

// ---- Helpers ----

func writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if data != nil {
		if err := json.NewEncoder(w).Encode(data); err != nil {
			log.Printf("write JSON response: %v", err)
		}
	}
}

func hashRequestID(id string) uint32 {
	if id == "" {
		return 0
	}
	var h uint32
	for i := 0; i < len(id) && i < 4; i++ {
		h = (h << 8) | uint32(id[i])
	}
	return h
}
