package worker

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/antines-labs/core/internal/ipc"
)

// DispatchResult holds the result of a worker dispatch.
type DispatchResult struct {
	Output     map[string]interface{}
	StatusCode uint32
}

// Dispatch sends input data to a worker, waits for the response, and returns the output.
// It handles serialization, timeout, and retries on failure.
func (p *Pool) Dispatch(
	ctx context.Context,
	handlerID, requestID uint32,
	inputLayout, outputLayout *ipc.CompiledLayout,
	inputData map[string]interface{},
) (*DispatchResult, error) {
	payload, err := ipc.SerializeInput(inputLayout, inputData)
	if err != nil {
		return nil, fmt.Errorf("worker: serialize input: %w", err)
	}

	var lastErr error
	for attempt := 0; attempt <= p.config.MaxRetries; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(100 * time.Millisecond):
			}
		}

		result, err := p.dispatchOnce(ctx, handlerID, requestID, payload, outputLayout)
		if err == nil {
			return result, nil
		}
		lastErr = err
	}

	return nil, fmt.Errorf("worker: dispatch failed after %d retries: %w", p.config.MaxRetries, lastErr)
}

func (p *Pool) dispatchOnce(
	ctx context.Context,
	handlerID, requestID uint32,
	payload []byte,
	outputLayout *ipc.CompiledLayout,
) (*DispatchResult, error) {
	worker, err := p.Acquire(ctx)
	if err != nil {
		return nil, fmt.Errorf("acquire: %w", err)
	}
	defer p.Release(worker)

	deadline := time.Now().Add(p.config.Timeout)
	if d, ok := ctx.Deadline(); ok && d.Before(deadline) {
		deadline = d
	}
	_ = worker.Conn.SetWriteDeadline(deadline)
	_ = worker.Conn.SetReadDeadline(deadline)

	// Write header + payload
	h := ipc.NewHeader(ipc.DirGoToJS, ipc.MsgDispatch, requestID, handlerID, 0, uint32(len(payload)))
	if err := ipc.WriteHeader(worker.Conn, h); err != nil {
		p.MarkDead(worker)
		return nil, fmt.Errorf("write header: %w", err)
	}
	if _, err := worker.Conn.Write(payload); err != nil {
		p.MarkDead(worker)
		return nil, fmt.Errorf("write payload: %w", err)
	}

	respHeader, err := ipc.ReadHeader(worker.Conn)
	if err != nil {
		p.MarkDead(worker)
		return nil, fmt.Errorf("read header: %w", err)
	}

	respPayload := make([]byte, respHeader.PayloadLen)
	if len(respPayload) > 0 {
		if _, err := io.ReadFull(worker.Conn, respPayload); err != nil {
			p.MarkDead(worker)
			return nil, fmt.Errorf("read payload: %w", err)
		}
	}

	if respHeader.MsgType == ipc.MsgError {
		return &DispatchResult{StatusCode: respHeader.StatusCode}, nil
	}

	result := &DispatchResult{StatusCode: respHeader.StatusCode}
	if len(respPayload) > 0 && outputLayout != nil {
		output, err := ipc.DeserializeOutput(outputLayout, respPayload)
		if err != nil {
			return nil, fmt.Errorf("deserialize output: %w", err)
		}
		result.Output = output
	}

	return result, nil
}
