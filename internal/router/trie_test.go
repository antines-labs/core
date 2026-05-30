package router

import (
	"testing"
)

func mustInsert(t *testing.T, trie *Trie, method, path string, id int) {
	t.Helper()
	if err := trie.Insert(method, path, id); err != nil {
		t.Fatalf("Insert(%s, %s): %v", method, path, err)
	}
}

func TestInsertAndMatchLiteral(t *testing.T) {
	trie := New()
	mustInsert(t, trie, "GET", "/users", 1)
	mustInsert(t, trie, "POST", "/users", 2)
	mustInsert(t, trie, "GET", "/health", 3)

	tests := []struct {
		method    string
		path      string
		wantID    int
		wantMatch bool
	}{
		{"GET", "/users", 1, true},
		{"POST", "/users", 2, true},
		{"GET", "/health", 3, true},
		{"PUT", "/users", 0, false},
		{"GET", "/nonexistent", 0, false},
		{"GET", "/", 0, false},
	}

	for _, tt := range tests {
		m, err := trie.Match(tt.method, tt.path)
		if err != nil {
			t.Fatalf("Match(%s, %s): %v", tt.method, tt.path, err)
		}
		if tt.wantMatch {
			if m == nil {
				t.Errorf("Match(%s, %s): expected match, got nil", tt.method, tt.path)
				continue
			}
			if m.Route.HandlerID != tt.wantID {
				t.Errorf("Match(%s, %s): handlerID = %d, want %d", tt.method, tt.path, m.Route.HandlerID, tt.wantID)
			}
		} else {
			if m != nil {
				t.Errorf("Match(%s, %s): expected no match, got handlerID=%d", tt.method, tt.path, m.Route.HandlerID)
			}
		}
	}
}

func TestInsertAndMatchParams(t *testing.T) {
	trie := New()
	mustInsert(t, trie, "GET", "/users/:id", 1)
	mustInsert(t, trie, "PUT", "/users/:id", 2)
	mustInsert(t, trie, "GET", "/users/:id/posts", 3)

	tests := []struct {
		method     string
		path       string
		wantID     int
		wantMatch  bool
		wantParams map[string]string
	}{
		{"GET", "/users/123", 1, true, map[string]string{"id": "123"}},
		{"PUT", "/users/abc", 2, true, map[string]string{"id": "abc"}},
		{"GET", "/users/456/posts", 3, true, map[string]string{"id": "456"}},
		{"GET", "/users", 0, false, nil},
		{"GET", "/users/123/comments", 0, false, nil},
	}

	for _, tt := range tests {
		m, err := trie.Match(tt.method, tt.path)
		if err != nil {
			t.Fatalf("Match(%s, %s): %v", tt.method, tt.path, err)
		}
		if tt.wantMatch {
			if m == nil {
				t.Errorf("Match(%s, %s): expected match, got nil", tt.method, tt.path)
				continue
			}
			if m.Route.HandlerID != tt.wantID {
				t.Errorf("Match(%s, %s): handlerID = %d, want %d", tt.method, tt.path, m.Route.HandlerID, tt.wantID)
			}
			for k, v := range tt.wantParams {
				if m.Params[k] != v {
					t.Errorf("Match(%s, %s): param[%s] = %q, want %q", tt.method, tt.path, k, m.Params[k], v)
				}
			}
		} else {
			if m != nil {
				t.Errorf("Match(%s, %s): expected no match, got handlerID=%d", tt.method, tt.path, m.Route.HandlerID)
			}
		}
	}
}

func TestInsertAndMatchWildcard(t *testing.T) {
	trie := New()
	mustInsert(t, trie, "GET", "/files/*", 1)

	tests := []struct {
		path      string
		wantMatch bool
		wantWild  string
	}{
		{"/files/foo.txt", true, "foo.txt"},
		{"/files/dir/bar.txt", true, "dir/bar.txt"},
		{"/files/", true, ""},
		{"/other", false, ""},
	}

	for _, tt := range tests {
		m, err := trie.Match("GET", tt.path)
		if err != nil {
			t.Fatalf("Match(%s): %v", tt.path, err)
		}
		if tt.wantMatch {
			if m == nil {
				t.Errorf("Match(%s): expected match, got nil", tt.path)
				continue
			}
			if m.Wildcard != tt.wantWild {
				t.Errorf("Match(%s): wildcard = %q, want %q", tt.path, m.Wildcard, tt.wantWild)
			}
		} else {
			if m != nil {
				t.Errorf("Match(%s): expected no match", tt.path)
			}
		}
	}
}

func TestLiteralPreference(t *testing.T) {
	trie := New()
	mustInsert(t, trie, "GET", "/users/new", 1) // literal
	mustInsert(t, trie, "GET", "/users/:id", 2) // param

	m, err := trie.Match("GET", "/users/new")
	if err != nil {
		t.Fatalf("Match: %v", err)
	}
	if m == nil || m.Route.HandlerID != 1 {
		t.Errorf("expected literal match (handlerID=1), got %v", m)
	}

	m, err = trie.Match("GET", "/users/123")
	if err != nil {
		t.Fatalf("Match: %v", err)
	}
	if m == nil || m.Route.HandlerID != 2 {
		t.Errorf("expected param match (handlerID=2), got %v", m)
	}
	if m.Params["id"] != "123" {
		t.Errorf("expected id=123, got id=%s", m.Params["id"])
	}
}

func TestMultipleParams(t *testing.T) {
	trie := New()
	mustInsert(t, trie, "GET", "/users/:userId/posts/:postId", 1)

	m, err := trie.Match("GET", "/users/42/posts/99")
	if err != nil {
		t.Fatalf("Match: %v", err)
	}
	if m == nil {
		t.Fatal("expected match")
	}
	if m.Params["userId"] != "42" {
		t.Errorf("expected userId=42, got %s", m.Params["userId"])
	}
	if m.Params["postId"] != "99" {
		t.Errorf("expected postId=99, got %s", m.Params["postId"])
	}
}

func TestRootPath(t *testing.T) {
	trie := New()
	mustInsert(t, trie, "GET", "/", 1)

	m, err := trie.Match("GET", "/")
	if err != nil {
		t.Fatalf("Match: %v", err)
	}
	if m == nil || m.Route.HandlerID != 1 {
		t.Errorf("expected root match (handlerID=1), got %v", m)
	}
}

func TestDuplicateRoute(t *testing.T) {
	trie := New()
	mustInsert(t, trie, "GET", "/users", 1)

	err := trie.Insert("GET", "/users", 2)
	if err == nil {
		t.Error("expected error for duplicate route")
	}
}

func TestMatchNoRoute(t *testing.T) {
	trie := New()
	mustInsert(t, trie, "GET", "/users", 1)

	m, err := trie.Match("POST", "/users")
	if err != nil {
		t.Fatalf("Match: %v", err)
	}
	if m != nil {
		t.Error("expected nil match for wrong method")
	}
}

func TestTrailingSlash(t *testing.T) {
	trie := New()
	mustInsert(t, trie, "GET", "/users", 1)
	mustInsert(t, trie, "GET", "/health/", 2) // inserted with trailing slash

	m, err := trie.Match("GET", "/users/")
	if err != nil {
		t.Fatalf("Match: %v", err)
	}
	if m == nil || m.Route.HandlerID != 1 {
		t.Errorf("expected match for /users/ (same as /users), got %v", m)
	}

	m, err = trie.Match("GET", "/health")
	if err != nil {
		t.Fatalf("Match: %v", err)
	}
	if m == nil || m.Route.HandlerID != 2 {
		t.Errorf("expected match for /health (same as /health/), got %v", m)
	}
}

func TestDeepRoutes(t *testing.T) {
	trie := New()
	mustInsert(t, trie, "GET", "/a/b/c", 1)
	mustInsert(t, trie, "GET", "/a/:param/c", 2)
	mustInsert(t, trie, "GET", "/a/b/d", 3)

	tests := []struct {
		path      string
		wantID    int
		wantMatch bool
	}{
		{"/a/b/c", 1, true},
		{"/a/x/c", 2, true},
		{"/a/b/d", 3, true},
		{"/a/b/e", 0, false},
		{"/a/b", 0, false},
	}

	for _, tt := range tests {
		m, err := trie.Match("GET", tt.path)
		if err != nil {
			t.Fatalf("Match(%s): %v", tt.path, err)
		}
		if tt.wantMatch {
			if m == nil {
				t.Errorf("Match(%s): expected match, got nil", tt.path)
				continue
			}
			if m.Route.HandlerID != tt.wantID {
				t.Errorf("Match(%s): handlerID = %d, want %d", tt.path, m.Route.HandlerID, tt.wantID)
			}
		} else {
			if m != nil {
				t.Errorf("Match(%s): expected no match, got handlerID=%d", tt.path, m.Route.HandlerID)
			}
		}
	}
}
