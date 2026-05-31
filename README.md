# Antines

**Antines** is a hybrid Go + JavaScript/TypeScript backend framework. The Go runtime handles HTTP, routing, validation, and infrastructure, while JavaScript workers execute business logic — each running in their own process with real debugging, full ecosystem access, and zero lock contention.

Communication between Go and JS happens over Unix sockets using a compact positional binary protocol. Schemas are defined once in TypeScript and serialized to a manifest that both sides consume at startup. Go validates all input against the schema; JavaScript receives pre-validated data and never re-validates.

This repository contains the Go runtime — the core server, router, validator, IPC protocol, worker pool, and manifest loader.

## Status

v0.1.0 — active development. The foundational pipeline is complete:

- Schema-first type definitions (TypeScript DSL → SchemaIR JSON)
- File-based routing (e.g. `users.[id].get.ts` → `GET /users/:id`)
- Trie-based router with literal, parameter and wildcard segments
- Go-only validation compiled from SchemaIR (9 validators: string, number, boolean, enum, date, array, object, nullable, optional)
- Positional binary IPC protocol over Unix sockets (bitmask + fixed fields + offset table + variable data)
- Round-robin worker pool with configurable timeout and retry
- Full HTTP server with middleware (request ID, logging, recovery, graceful shutdown)
- Worker lifecycle management (spawn, connect, dispatch, mark dead, reconnect)

## Quick start

```go
// internal/server/example_test.go — see server_test.go for integration tests
```

Generate the manifest from your routes directory:

```bash
cd antinesjs/apps/example
bun run scripts/generate-manifest.ts
```

Build and start the server:

```bash
cd core
go build -o dist/antines ./cmd/antines
./dist/antines \
  --port 3000 \
  --manifest ../antinesjs/apps/example/antines-manifest.json \
  --workers 2 \
  --worker-entry ../antinesjs/apps/example/worker-entry.ts \
  --bun $(which bun)
```

## Architecture

```
┌─────────────────┐     Unix socket      ┌─────────────────┐
│  Go (core)      │◄────────────────────►│  JS Workers      │
│                 │  positional binary   │  (Bun/Node/Deno) │
│  HTTP server    │  protocol            │                  │
│  Router (trie)  │                      │  Route handlers  │
│  Validator      │  go → js: dispatch   │  Business logic  │
│  Worker pool    │  js → go: result     │                  │
│  Middleware     │                      │                  │
└─────────────────┘                      └─────────────────┘
```

1. Go starts, loads the manifest, builds the route trie, compiles validators, and spawns JS workers.
2. An HTTP request arrives → trie match → parse body + query params → validate input → serialize to binary → send to worker.
3. Worker deserializes, runs the handler, serializes the result, sends it back.
4. Go deserializes, validates output, sends HTTP response.

## Module

```
github.com/antines-labs/core
```

## License

MIT — see [LICENSE](LICENSE).
