package manifest

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/antines/core/internal/schema"
)

// Manifest is the top-level structure of antines-manifest.json.
type Manifest struct {
	Version int             `json:"version"`
	Routes  []RouteManifest `json:"routes"`
}

// RouteManifest represents a single route in the manifest.
type RouteManifest struct {
	Method      string      `json:"method"`
	Path        string      `json:"path"`
	HandlerID   int         `json:"handlerId"`
	HandlerFile string      `json:"handlerFile"`
	HasHandler  bool        `json:"hasHandler"`
	Params      []string    `json:"params"`
	Schema      RouteSchema `json:"schema"`
}

// RouteSchema holds the input/output/errors schemas for a route.
type RouteSchema struct {
	Input  *schema.SchemaIR    `json:"input,omitempty"`
	Output *schema.SchemaIR    `json:"output,omitempty"`
	Errors map[string]ErrorDef `json:"errors,omitempty"`
}

// ErrorDef describes a business-logic error.
type ErrorDef struct {
	Status  int    `json:"status"`
	Message string `json:"message"`
}

// ---- Loader ----

// Load reads and parses an antines-manifest.json file.
func Load(path string) (*Manifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("manifest: reading %s: %w", path, err)
	}

	var m Manifest
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("manifest: parsing %s: %w", path, err)
	}

	if err := m.validate(); err != nil {
		return nil, fmt.Errorf("manifest: validating %s: %w", path, err)
	}

	return &m, nil
}

// validate checks the manifest structure for basic correctness.
func (m *Manifest) validate() error {
	if m.Version != 1 {
		return fmt.Errorf("unsupported manifest version: %d", m.Version)
	}

	if len(m.Routes) == 0 {
		return fmt.Errorf("no routes defined in manifest")
	}

	usedIDs := make(map[int]bool)
	seenPaths := make(map[string]bool)

	for i, r := range m.Routes {
		if r.Method == "" {
			return fmt.Errorf("route %d: missing method", i)
		}
		if r.Path == "" {
			return fmt.Errorf("route %d: missing path", i)
		}
		if r.HandlerID == 0 {
			return fmt.Errorf("route %d: missing or zero handlerId", i)
		}

		if usedIDs[r.HandlerID] {
			return fmt.Errorf("route %d: duplicate handlerId %d", i, r.HandlerID)
		}
		usedIDs[r.HandlerID] = true

		key := r.Method + " " + r.Path
		if seenPaths[key] {
			return fmt.Errorf("route %d: duplicate %s", i, key)
		}
		seenPaths[key] = true

		if r.Schema.Input != nil {
			if err := validateSchemaNode(r.Schema.Input); err != nil {
				return fmt.Errorf("route %d (%s): input schema: %w", i, key, err)
			}
		}
		if r.Schema.Output != nil {
			if err := validateSchemaNode(r.Schema.Output); err != nil {
				return fmt.Errorf("route %d (%s): output schema: %w", i, key, err)
			}
		}
	}

	return nil
}

func validateSchemaNode(n *schema.SchemaIR) error {
	if n == nil {
		return fmt.Errorf("schema node is nil")
	}

	switch n.Type {
	case "string", "number", "boolean", "date", "enum", "array", "object", "nullable", "optional":
		// valid types
	default:
		return fmt.Errorf("unknown schema type: %q", n.Type)
	}

	if n.Type == "object" {
		for name, f := range n.Fields {
			if err := validateSchemaNode(&f.Schema); err != nil {
				return fmt.Errorf("field %q: %w", name, err)
			}
		}
	}

	if n.Type == "array" && n.Items != nil {
		if err := validateSchemaNode(n.Items); err != nil {
			return fmt.Errorf("array items: %w", err)
		}
	}

	if (n.Type == "nullable" || n.Type == "optional") && n.Inner != nil {
		if err := validateSchemaNode(n.Inner); err != nil {
			return fmt.Errorf("%s inner: %w", n.Type, err)
		}
	}

	return nil
}
