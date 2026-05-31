package server

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/antines/core/internal/ipc"
	"github.com/antines/core/internal/manifest"
	"github.com/antines/core/internal/router"
	"github.com/antines/core/internal/schema"
	"github.com/antines/core/internal/validator"
	"github.com/antines/core/internal/worker"
)

func newTestServer(t *testing.T, m *manifest.Manifest) *Server {
	t.Helper()

	tr := router.New()
	reg := make(map[int]*RouteEntry)

	for _, route := range m.Routes {
		if err := tr.Insert(route.Method, route.Path, route.HandlerID); err != nil {
			t.Fatalf("trie insert: %v", err)
		}

		entry := &RouteEntry{Route: route}

		if route.Schema.Input != nil {
			layout, err := ipc.CalculateLayout(route.Schema.Input)
			if err != nil {
				t.Fatalf("input layout: %v", err)
			}
			entry.InputLayout = layout

			v, err := validator.Compile(route.Schema.Input)
			if err != nil {
				t.Fatalf("input validator: %v", err)
			}
			entry.InputValidator = v
		}

		if route.Schema.Output != nil {
			layout, err := ipc.CalculateLayout(route.Schema.Output)
			if err != nil {
				t.Fatalf("output layout: %v", err)
			}
			entry.OutputLayout = layout

			v, err := validator.Compile(route.Schema.Output)
			if err != nil {
				t.Fatalf("output validator: %v", err)
			}
			entry.OutputValidator = v
		}

		reg[route.HandlerID] = entry
	}

	return &Server{
		config: Config{
			Port:          0,
			WorkerTimeout: 5 * time.Second,
		},
		manifest:  m,
		trie:      tr,
		registry:  reg,
		startTime: time.Now(),
	}
}

func startTestHTTPServer(t *testing.T, s *Server) int {
	t.Helper()

	mux := http.NewServeMux()
	mux.HandleFunc("/", s.handler)

	listener, err := net.Listen("tcp", ":0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}

	go http.Serve(listener, recoveryMiddleware(requestIDMiddleware(loggerMiddleware(mux))))

	return listener.Addr().(*net.TCPAddr).Port
}

func mustSchemaIR(t *testing.T, rawJSON string) *schema.SchemaIR {
	t.Helper()
	var s schema.SchemaIR
	if err := json.Unmarshal([]byte(rawJSON), &s); err != nil {
		t.Fatalf("Unmarshal schema: %v", err)
	}
	return &s
}

// ---- Tests ----

func TestServerHealthRoute(t *testing.T) {
	m := &manifest.Manifest{
		Version: 1,
		Routes: []manifest.RouteManifest{
			{
				Method:     "GET",
				Path:       "/health",
				HandlerID:  1,
				HasHandler: false,
				Schema: manifest.RouteSchema{
					Output: mustSchemaIR(t, `{
						"type":"object","strict":false,
						"fields":{
							"status":{"schema":{"type":"string"},"optional":false,"nullable":false},
							"uptime":{"schema":{"type":"number"},"optional":false,"nullable":false}
						}
					}`),
				},
			},
		},
	}

	s := newTestServer(t, m)
	port := startTestHTTPServer(t, s)

	resp, err := http.Get(fmt.Sprintf("http://localhost:%d/health", port))
	if err != nil {
		t.Fatalf("GET /health: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}

	var body map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&body)

	if body["status"] != "ok" {
		t.Errorf("expected status=ok, got %v", body["status"])
	}
	if _, ok := body["uptime"].(float64); !ok {
		t.Errorf("expected uptime as float64, got %T", body["uptime"])
	}
}

func TestServerNotFound(t *testing.T) {
	m := &manifest.Manifest{
		Version: 1,
		Routes: []manifest.RouteManifest{
			{Method: "GET", Path: "/health", HandlerID: 1, HasHandler: false},
		},
	}

	s := newTestServer(t, m)
	port := startTestHTTPServer(t, s)

	resp, err := http.Get(fmt.Sprintf("http://localhost:%d/notfound", port))
	if err != nil {
		t.Fatalf("GET /notfound: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("expected 404, got %d", resp.StatusCode)
	}
}

func TestServerWrongMethod(t *testing.T) {
	m := &manifest.Manifest{
		Version: 1,
		Routes: []manifest.RouteManifest{
			{Method: "GET", Path: "/health", HandlerID: 1, HasHandler: false},
		},
	}

	s := newTestServer(t, m)
	port := startTestHTTPServer(t, s)

	resp, err := http.Post(fmt.Sprintf("http://localhost:%d/health", port), "application/json", nil)
	if err != nil {
		t.Fatalf("POST /health: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("expected 404, got %d", resp.StatusCode)
	}
}

func TestServerRouteWithoutHandler(t *testing.T) {
	m := &manifest.Manifest{
		Version: 1,
		Routes: []manifest.RouteManifest{
			{
				Method:     "GET",
				Path:       "/no-handler",
				HandlerID:  2,
				HasHandler: false,
			},
		},
	}

	s := newTestServer(t, m)
	port := startTestHTTPServer(t, s)

	resp, err := http.Get(fmt.Sprintf("http://localhost:%d/no-handler", port))
	if err != nil {
		t.Fatalf("GET /no-handler: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotImplemented {
		t.Errorf("expected 501, got %d", resp.StatusCode)
	}
}

func TestServerValidationError(t *testing.T) {
	m := &manifest.Manifest{
		Version: 1,
		Routes: []manifest.RouteManifest{
			{
				Method:     "POST",
				Path:       "/users",
				HandlerID:  3,
				HasHandler: true,
				Schema: manifest.RouteSchema{
					Input: mustSchemaIR(t, `{
						"type":"object","strict":false,
						"fields":{
							"email":{"schema":{"type":"string","validations":{"email":true}},"optional":false,"nullable":false}
						}
					}`),
				},
			},
		},
	}

	s := newTestServer(t, m)

	// Create a mock worker that reads and discards (so write doesn't block)
	server, client := net.Pipe()
	pool := worker.NewPool(worker.Config{Timeout: 5 * time.Second, MaxRetries: 0})
	pool.AddConn(1, client)

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		// Read and discard the dispatch (validation should fail before dispatch though)
		server.SetReadDeadline(time.Now().Add(100 * time.Millisecond))
		header, err := ipc.ReadHeader(server)
		if err == nil {
			payload := make([]byte, header.PayloadLen)
			server.Read(payload)
			// Send a minimal response
			respHeader := ipc.NewHeader(ipc.DirJSToGo, ipc.MsgResult, header.RequestID, header.HandlerID, 200, 0)
			ipc.WriteHeader(server, respHeader)
		}
		server.Close()
	}()
	defer wg.Wait()

	s.pool = pool
	port := startTestHTTPServer(t, s)

	resp, err := http.Post(
		fmt.Sprintf("http://localhost:%d/users", port),
		"application/json",
		strings.NewReader(`{"email": "not-an-email"}`),
	)
	if err != nil {
		t.Fatalf("POST /users: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusUnprocessableEntity {
		t.Errorf("expected 422, got %d", resp.StatusCode)
	}

	var respBody map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&respBody)

	if respBody["error"] != "validation failed" {
		t.Errorf("expected validation error, got %v", respBody)
	}
}

func TestServerJSHandlerDispatch(t *testing.T) {
	m := &manifest.Manifest{
		Version: 1,
		Routes: []manifest.RouteManifest{
			{
				Method:     "POST",
				Path:       "/users",
				HandlerID:  4,
				HasHandler: true,
				Schema: manifest.RouteSchema{
					Input: mustSchemaIR(t, `{
						"type":"object","strict":false,
						"fieldOrder":["name"],
						"fields":{
							"name":{"schema":{"type":"string"},"optional":false,"nullable":false}
						}
					}`),
					Output: mustSchemaIR(t, `{
						"type":"object","strict":false,
						"fieldOrder":["id"],
						"fields":{
							"id":{"schema":{"type":"string"},"optional":false,"nullable":false}
						}
					}`),
				},
			},
		},
	}

	s := newTestServer(t, m)

	server, client := net.Pipe()
	pool := worker.NewPool(worker.Config{Timeout: 5 * time.Second, MaxRetries: 0})
	pool.AddConn(1, client)

	outputLayout := s.registry[4].OutputLayout

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		defer server.Close()

		header, err := ipc.ReadHeader(server)
		if err != nil {
			t.Errorf("mock: read header: %v", err)
			return
		}

		payload := make([]byte, header.PayloadLen)
		if len(payload) > 0 {
			_, _ = server.Read(payload)
		}

		respData := map[string]interface{}{"id": "user-42"}
		respPayload, _ := ipc.SerializeInput(outputLayout, respData)
		respHeader := ipc.NewHeader(ipc.DirJSToGo, ipc.MsgResult, header.RequestID, header.HandlerID, 200, uint32(len(respPayload)))
		ipc.WriteHeader(server, respHeader)
		if len(respPayload) > 0 {
			server.Write(respPayload)
		}
	}()

	s.pool = pool
	port := startTestHTTPServer(t, s)

	resp, err := http.Post(
		fmt.Sprintf("http://localhost:%d/users", port),
		"application/json",
		strings.NewReader(`{"name": "Alice"}`),
	)
	if err != nil {
		t.Fatalf("POST /users: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}

	var respBody map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&respBody)

	if respBody["id"] != "user-42" {
		t.Errorf("expected id=user-42, got %v", respBody["id"])
	}

	wg.Wait()
}

func TestServerRequestIDHeader(t *testing.T) {
	m := &manifest.Manifest{
		Version: 1,
		Routes: []manifest.RouteManifest{
			{Method: "GET", Path: "/health", HandlerID: 1, HasHandler: false},
		},
	}

	s := newTestServer(t, m)
	port := startTestHTTPServer(t, s)

	req, _ := http.NewRequest("GET", fmt.Sprintf("http://localhost:%d/health", port), nil)
	req.Header.Set("X-Request-ID", "my-test-id")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET /health: %v", err)
	}
	defer resp.Body.Close()

	if resp.Header.Get("X-Request-ID") != "my-test-id" {
		t.Errorf("expected X-Request-ID=my-test-id, got %s", resp.Header.Get("X-Request-ID"))
	}
}

func TestServerJSONContentType(t *testing.T) {
	m := &manifest.Manifest{
		Version: 1,
		Routes: []manifest.RouteManifest{
			{Method: "GET", Path: "/health", HandlerID: 1, HasHandler: false},
		},
	}

	s := newTestServer(t, m)
	port := startTestHTTPServer(t, s)

	resp, err := http.Get(fmt.Sprintf("http://localhost:%d/health", port))
	if err != nil {
		t.Fatalf("GET /health: %v", err)
	}
	defer resp.Body.Close()

	if ct := resp.Header.Get("Content-Type"); ct != "application/json" {
		t.Errorf("expected Content-Type=application/json, got %s", ct)
	}
}
