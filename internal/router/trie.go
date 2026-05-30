package router

import (
	"fmt"
	"strings"
)

// RouteRef holds the handler ID for a matched route.
type RouteRef struct {
	HandlerID int
}

// MatchResult holds the result of a path match.
type MatchResult struct {
	Route    RouteRef
	Params   map[string]string
	Wildcard string
}

// TrieNode represents a single node in the routing trie.
type TrieNode struct {
	segment    string
	children   []*TrieNode
	paramChild *TrieNode           // :param child
	wildChild  *TrieNode           // * wildcard child
	routes     map[string]RouteRef // method → route
	paramName  string
	isParam    bool
	isWild     bool
}

// Trie is the root of the routing trie.
type Trie struct {
	root TrieNode
}

// New creates a new empty trie.
func New() *Trie {
	return &Trie{
		root: TrieNode{
			segment:  "/",
			children: make([]*TrieNode, 0),
			routes:   make(map[string]RouteRef),
		},
	}
}

// Insert adds a route to the trie.
// path should be like "/users/:id/posts" or "/health".
func (t *Trie) Insert(method, path string, handlerID int) error {
	if path == "" || path[0] != '/' {
		return fmt.Errorf("router: path must start with '/': %s", path)
	}

	// Normalize: remove trailing slash (except for root)
	normalized := path
	if len(normalized) > 1 && normalized[len(normalized)-1] == '/' {
		normalized = normalized[:len(normalized)-1]
	}

	segments := splitPath(normalized)

	node := &t.root
	for _, seg := range segments {
		if seg == "" {
			continue
		}

		isParam := len(seg) > 0 && seg[0] == ':'
		isWild := seg == "*"

		if isWild {
			if node.wildChild != nil {
				return fmt.Errorf("router: duplicate wildcard at %s", path)
			}
			node.wildChild = &TrieNode{
				segment:  "*",
				isWild:   true,
				children: make([]*TrieNode, 0),
				routes:   make(map[string]RouteRef),
			}
			node = node.wildChild
			continue
		}

		if isParam {
			paramName := seg[1:]
			if node.paramChild != nil {
				if node.paramChild.paramName != paramName {
					return fmt.Errorf(
						"router: param name mismatch at %s: got %s, existing %s",
						path, paramName, node.paramChild.paramName,
					)
				}
				node = node.paramChild
				continue
			}
			node.paramChild = &TrieNode{
				segment:   seg,
				paramName: paramName,
				isParam:   true,
				children:  make([]*TrieNode, 0),
				routes:    make(map[string]RouteRef),
			}
			node = node.paramChild
			continue
		}

		// Literal segment — look for existing child
		found := false
		for _, child := range node.children {
			if child.segment == seg {
				node = child
				found = true
				break
			}
		}
		if !found {
			newNode := &TrieNode{
				segment:  seg,
				children: make([]*TrieNode, 0),
				routes:   make(map[string]RouteRef),
			}
			node.children = append(node.children, newNode)
			node = newNode
		}
	}

	if _, exists := node.routes[method]; exists {
		return fmt.Errorf("router: duplicate route %s %s", method, path)
	}

	node.routes[method] = RouteRef{HandlerID: handlerID}
	return nil
}

// Match finds a route for the given method and path.
// Returns nil if no route matches.
func (t *Trie) Match(method, path string) (*MatchResult, error) {
	if path == "" || path[0] != '/' {
		return nil, nil
	}

	normalized := path
	if len(normalized) > 1 && normalized[len(normalized)-1] == '/' {
		normalized = normalized[:len(normalized)-1]
	}

	segments := splitPath(normalized)
	params := make(map[string]string)
	node := &t.root

	for i, seg := range segments {
		if seg == "" {
			continue
		}

		matched := false
		for _, child := range node.children {
			if child.segment == seg {
				node = child
				matched = true
				break
			}
		}
		if matched {
			continue
		}

		if node.paramChild != nil {
			params[node.paramChild.paramName] = seg
			node = node.paramChild
			continue
		}

		if node.wildChild != nil {
			remaining := strings.Join(segments[i:], "/")
			params["*"] = remaining
			node = node.wildChild
			break
		}

		// No match
		return nil, nil
	}

	if route, ok := node.routes[method]; ok {
		return &MatchResult{
			Route:    route,
			Params:   params,
			Wildcard: params["*"],
		}, nil
	}

	if node.wildChild != nil {
		params["*"] = ""
		if route, ok := node.wildChild.routes[method]; ok {
			return &MatchResult{
				Route:    route,
				Params:   params,
				Wildcard: "",
			}, nil
		}
	}

	return nil, nil
}

// splitPath splits a path into segments, skipping empty ones.
func splitPath(path string) []string {
	if path == "" || path == "/" {
		return []string{}
	}

	p := path
	if p[0] == '/' {
		p = p[1:]
	}

	if len(p) > 0 && p[len(p)-1] == '/' {
		p = p[:len(p)-1]
	}

	if p == "" {
		return []string{}
	}

	return strings.Split(p, "/")
}
