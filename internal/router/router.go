package router

import (
	"fmt"

	"github.com/antines-labs/core/internal/manifest"
)

// Router wraps the trie and provides manifest-based route management.
type Router struct {
	trie   *Trie
	routes []RouteRef // indexed by handlerId (1-based)
}

// NewFromManifest builds a Router from a parsed manifest.
func NewFromManifest(m *manifest.Manifest) (*Router, error) {
	r := &Router{
		trie:   New(),
		routes: make([]RouteRef, len(m.Routes)+1), // 1-based indexing
	}

	for _, rm := range m.Routes {
		if err := r.trie.Insert(rm.Method, rm.Path, rm.HandlerID); err != nil {
			return nil, fmt.Errorf("router: insert %s %s: %w", rm.Method, rm.Path, err)
		}
		r.routes[rm.HandlerID] = RouteRef{HandlerID: rm.HandlerID}
	}

	return r, nil
}

// Match finds the route for the given method and path.
// Returns nil if no route matches.
func (r *Router) Match(method, path string) (*MatchResult, error) {
	return r.trie.Match(method, path)
}

// RouteCount returns the number of registered routes.
func (r *Router) RouteCount() int {
	// Exclude the 0-index placeholder
	return len(r.routes) - 1
}

// RouteByHandlerID returns the route reference for a handler ID.
func (r *Router) RouteByHandlerID(id int) (RouteRef, bool) {
	if id <= 0 || id >= len(r.routes) {
		return RouteRef{}, false
	}
	return r.routes[id], true
}
