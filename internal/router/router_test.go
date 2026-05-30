package router

import (
	"testing"

	"github.com/antines/core/internal/manifest"
)

func TestNewFromManifest(t *testing.T) {
	m, err := manifest.Load("../manifest/testdata/valid.json")
	if err != nil {
		t.Fatalf("manifest.Load: %v", err)
	}

	r, err := NewFromManifest(m)
	if err != nil {
		t.Fatalf("NewFromManifest: %v", err)
	}

	if r.RouteCount() != 3 {
		t.Errorf("expected 3 routes, got %d", r.RouteCount())
	}

	// Test matches
	tests := []struct {
		method string
		path   string
		wantID int
	}{
		{"POST", "/users", 1},
		{"POST", "/auth/login", 2},
		{"GET", "/health", 3},
		{"GET", "/users", 0}, // no GET /users
		{"PUT", "/users", 0}, // no PUT /users
		{"GET", "/nonexistent", 0},
	}

	for _, tt := range tests {
		match, err := r.Match(tt.method, tt.path)
		if err != nil {
			t.Fatalf("Match(%s, %s): %v", tt.method, tt.path, err)
		}
		if tt.wantID > 0 {
			if match == nil {
				t.Errorf("Match(%s, %s): expected handlerID=%d, got nil", tt.method, tt.path, tt.wantID)
				continue
			}
			if match.Route.HandlerID != tt.wantID {
				t.Errorf("Match(%s, %s): handlerID=%d, want %d", tt.method, tt.path, match.Route.HandlerID, tt.wantID)
			}
		} else {
			if match != nil {
				t.Errorf("Match(%s, %s): expected no match, got handlerID=%d", tt.method, tt.path, match.Route.HandlerID)
			}
		}
	}

	// RouteByHandlerID
	ref, ok := r.RouteByHandlerID(1)
	if !ok {
		t.Fatal("expected RouteByHandlerID(1) to succeed")
	}
	if ref.HandlerID != 1 {
		t.Errorf("expected handlerID=1, got %d", ref.HandlerID)
	}

	_, ok = r.RouteByHandlerID(99)
	if ok {
		t.Error("expected RouteByHandlerID(99) to fail")
	}
}

func TestRouterDuplicateManifest(t *testing.T) {
	// Create a manifest with duplicate routes
	m := &manifest.Manifest{
		Version: 1,
		Routes: []manifest.RouteManifest{
			{Method: "GET", Path: "/users", HandlerID: 1, HandlerFile: "a.ts", HasHandler: true, Params: []string{}},
			{Method: "GET", Path: "/users", HandlerID: 2, HandlerFile: "b.ts", HasHandler: true, Params: []string{}},
		},
	}

	_, err := NewFromManifest(m)
	if err == nil {
		t.Fatal("expected error for duplicate routes in manifest")
	}
}
