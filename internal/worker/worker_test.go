package worker

import (
	"context"
	"encoding/json"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/antines-labs/core/internal/ipc"
	"github.com/antines-labs/core/internal/schema"
)

func mustCompiledLayout(t *testing.T, rawJSON string) *ipc.CompiledLayout {
	t.Helper()
	var s schema.IR
	if err := json.Unmarshal([]byte(rawJSON), &s); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	l, err := ipc.CalculateLayout(&s)
	if err != nil {
		t.Fatalf("CalculateLayout: %v", err)
	}
	return l
}

// mockWorker starts a goroutine that acts as a JS worker over conn.
func mockWorker(t *testing.T, conn net.Conn, inputLayout, outputLayout *ipc.CompiledLayout, wg *sync.WaitGroup) {
	t.Helper()
	defer wg.Done()
	defer conn.Close()

	header, err := ipc.ReadHeader(conn)
	if err != nil {
		t.Errorf("mock: read header: %v", err)
		return
	}

	payload := make([]byte, header.PayloadLen)
	if len(payload) > 0 {
		if _, err := conn.Read(payload); err != nil {
			t.Errorf("mock: read payload: %v", err)
			return
		}
		if inputLayout != nil && header.PayloadLen > 0 {
			if _, err := ipc.DeserializeOutput(inputLayout, payload); err != nil {
				t.Errorf("mock: deserialize input: %v", err)
				return
			}
		}
	}

	var respPayload []byte
	if outputLayout != nil {
		respPayload, _ = ipc.SerializeInput(outputLayout, map[string]interface{}{
			"result": "ok",
		})
	}

	respHeader := ipc.NewHeader(ipc.DirJSToGo, ipc.MsgResult, header.RequestID, header.HandlerID, 200, uint32(len(respPayload)))
	if err := ipc.WriteHeader(conn, respHeader); err != nil {
		t.Errorf("mock: write header: %v", err)
		return
	}
	if len(respPayload) > 0 {
		if _, err := conn.Write(respPayload); err != nil {
			t.Errorf("mock: write payload: %v", err)
		}
	}
}

func TestWorkerPoolAcquireRelease(t *testing.T) {
	pool := NewPool(DefaultConfig())
	c1, _ := net.Pipe()
	c2, _ := net.Pipe()
	pool.AddConn(1, c1)
	pool.AddConn(2, c2)

	w, err := pool.Acquire(context.Background())
	if err != nil {
		t.Fatalf("Acquire: %v", err)
	}
	if w.ID != 1 {
		t.Errorf("expected worker 1, got %d", w.ID)
	}
	if !w.Busy {
		t.Error("expected worker to be busy")
	}

	pool.Release(w)
	if w.Busy {
		t.Error("expected worker to be not busy after release")
	}
}

func TestWorkerPoolRoundRobin(t *testing.T) {
	pool := NewPool(DefaultConfig())

	var closers []net.Conn
	for i := 0; i < 3; i++ {
		srv, cli := net.Pipe()
		closers = append(closers, srv)
		pool.AddConn(uint32(i+1), cli)
	}
	defer func() {
		for _, c := range closers {
			c.Close()
		}
	}()

	for i := 0; i < 3; i++ {
		w, err := pool.Acquire(context.Background())
		if err != nil {
			t.Fatalf("Acquire %d: %v", i, err)
		}
		if w.ID != uint32(i+1) {
			t.Errorf("expected worker %d, got %d", i+1, w.ID)
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	_, err := pool.Acquire(ctx)
	if err == nil {
		t.Error("expected error when all workers busy")
	}

	pool.Release(pool.workers[0])
	w, err := pool.Acquire(context.Background())
	if err != nil {
		t.Fatalf("Acquire after release: %v", err)
	}
	if w.ID != 1 {
		t.Errorf("expected worker 1, got %d", w.ID)
	}
}

func TestWorkerPoolMarkDead(t *testing.T) {
	pool := NewPool(DefaultConfig())
	c1, c2 := net.Pipe()
	defer c2.Close()
	pool.AddConn(1, c1)

	if pool.Len() != 1 {
		t.Fatalf("expected 1 worker, got %d", pool.Len())
	}

	removed := pool.MarkDead(pool.workers[0])
	if !removed {
		t.Error("expected worker to be removed")
	}
	if pool.Len() != 0 {
		t.Errorf("expected 0 workers, got %d", pool.Len())
	}
}

func TestWorkerPoolAcquireNoWorkers(t *testing.T) {
	pool := NewPool(DefaultConfig())
	_, err := pool.Acquire(context.Background())
	if err == nil {
		t.Error("expected error when no workers")
	}
}

func TestWorkerPoolAcquireCancelledContext(t *testing.T) {
	pool := NewPool(DefaultConfig())
	c1, _ := net.Pipe()
	defer c1.Close()
	pool.AddConn(1, c1)

	_, _ = pool.Acquire(context.Background()) // make busy

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := pool.Acquire(ctx)
	if err == nil {
		t.Error("expected error with cancelled context")
	}
}

func TestWorkerPoolShutdown(t *testing.T) {
	pool := NewPool(DefaultConfig())
	c1, _ := net.Pipe()
	pool.AddConn(1, c1)

	if err := pool.Shutdown(); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}
	if pool.Len() != 0 {
		t.Errorf("expected 0 workers after shutdown, got %d", pool.Len())
	}
}

func TestDispatchRoundTrip(t *testing.T) {
	pool := NewPool(Config{
		WorkerCount: 1,
		Timeout:     5 * time.Second,
		MaxRetries:  0,
	})

	inputLayout := mustCompiledLayout(t, `{
		"type":"object","strict":false,
		"fieldOrder":["name"],
		"fields":{
			"name":{"schema":{"type":"string"},"optional":false,"nullable":false}
		}
	}`)
	outputLayout := mustCompiledLayout(t, `{
		"type":"object","strict":false,
		"fieldOrder":["result"],
		"fields":{
			"result":{"schema":{"type":"string"},"optional":false,"nullable":false}
		}
	}`)

	server, client := net.Pipe()
	pool.AddConn(1, client)

	var wg sync.WaitGroup
	wg.Add(1)
	go mockWorker(t, server, inputLayout, outputLayout, &wg)

	result, err := pool.Dispatch(context.Background(), 1, 42, inputLayout, outputLayout, map[string]interface{}{
		"name": "Alice",
	})
	if err != nil {
		t.Fatalf("Dispatch: %v", err)
	}
	if result.StatusCode != 200 {
		t.Errorf("expected 200, got %d", result.StatusCode)
	}
	if result.Output["result"] != "ok" {
		t.Errorf("expected result=ok, got %v", result.Output["result"])
	}

	wg.Wait()
}

func TestDispatchTimeout(t *testing.T) {
	pool := NewPool(Config{
		WorkerCount: 1,
		Timeout:     50 * time.Millisecond,
		MaxRetries:  0,
	})

	server, client := net.Pipe()
	pool.AddConn(1, client)

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		buf := make([]byte, 32)
		_, _ = server.Read(buf)
		time.Sleep(200 * time.Millisecond)
		server.Close()
	}()

	inputLayout := mustCompiledLayout(t, `{
		"type":"object","strict":false,
		"fields":{}
	}`)

	_, err := pool.Dispatch(context.Background(), 1, 42, inputLayout, nil, map[string]interface{}{})
	if err == nil {
		t.Error("expected timeout error")
	}

	wg.Wait()
}

func TestDispatchRetry(t *testing.T) {
	pool := NewPool(Config{
		WorkerCount: 2,
		Timeout:     100 * time.Millisecond,
		MaxRetries:  1,
	})

	badServer, badClient := net.Pipe()
	pool.AddConn(1, badClient)

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		badServer.Close()
	}()

	inputLayout := mustCompiledLayout(t, `{
		"type":"object","strict":false,
		"fieldOrder":["x"],
		"fields":{
			"x":{"schema":{"type":"number"},"optional":false,"nullable":false}
		}
	}`)
	outputLayout := mustCompiledLayout(t, `{
		"type":"object","strict":false,
		"fieldOrder":["result"],
		"fields":{
			"result":{"schema":{"type":"string"},"optional":false,"nullable":false}
		}
	}`)

	goodServer, goodClient := net.Pipe()
	pool.AddConn(2, goodClient)

	wg.Add(1)
	go mockWorker(t, goodServer, inputLayout, outputLayout, &wg)

	result, err := pool.Dispatch(context.Background(), 2, 99, inputLayout, outputLayout, map[string]interface{}{
		"x": float64(42),
	})
	if err != nil {
		t.Fatalf("Dispatch: %v", err)
	}
	if result.StatusCode != 200 {
		t.Errorf("expected 200, got %d", result.StatusCode)
	}
	if result.Output["result"] != "ok" {
		t.Errorf("expected result=ok, got %v", result.Output["result"])
	}

	wg.Wait()
}
