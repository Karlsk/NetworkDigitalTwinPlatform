package schema

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

const expectedAPIVersion = "twin.io/v1"

// LoadFromFile parses a single YAML file and returns either an EntityType
// or RelationType based on the kind field.
//
// For single-document files: returns (*EntityType, nil, nil) or (nil, *RelationType, nil).
// For multi-document files: returns only the first document. Use LoadFromDir
// for full multi-document expansion.
//
// Errors are wrapped with file path context for debugging.
func LoadFromFile(path string) (*EntityType, *RelationType, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, fmt.Errorf("read schema file %s: %w", path, err)
	}

	return unmarshalDocument(data, path)
}

// LoadFromDir scans all .yaml/.yml files in a directory and parses each into
// EntityType or RelationType. Multi-document YAML files (separated by ---)
// are fully expanded — each document produces a separate result.
//
// Non-YAML files and subdirectories are skipped. Empty directories return
// empty slices with no error. Fails fast on the first parse error, wrapping
// the filename in the error context.
func LoadFromDir(dir string) ([]EntityType, []RelationType, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, nil, fmt.Errorf("read ontology directory %s: %w", dir, err)
	}

	var entityTypes []EntityType
	var relationTypes []RelationType

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasSuffix(name, ".yaml") && !strings.HasSuffix(name, ".yml") {
			continue
		}

		path := filepath.Join(dir, name)
		ets, rts, err := loadFileExpanded(path)
		if err != nil {
			return nil, nil, fmt.Errorf("parse schema %q: %w", name, err)
		}

		entityTypes = append(entityTypes, ets...)
		relationTypes = append(relationTypes, rts...)
	}

	return entityTypes, relationTypes, nil
}

// loadFileExpanded reads a YAML file and expands all documents (including
// multi-document files separated by ---). Empty documents (Kind == "")
// from trailing separators are filtered out.
func loadFileExpanded(path string) ([]EntityType, []RelationType, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, fmt.Errorf("read file: %w", err)
	}

	var entityTypes []EntityType
	var relationTypes []RelationType

	decoder := yaml.NewDecoder(bytes.NewReader(data))
	for {
		var node yaml.Node
		if err := decoder.Decode(&node); err != nil {
			if err == io.EOF {
				break
			}
			return nil, nil, fmt.Errorf("decode document: %w", ErrInvalidSchema)
		}

		et, rt, err := unmarshalNode(&node, path)
		if err != nil {
			return nil, nil, err
		}

		// Skip empty documents (from trailing --- separators)
		if et == nil && rt == nil {
			continue
		}

		if et != nil {
			entityTypes = append(entityTypes, *et)
		}
		if rt != nil {
			relationTypes = append(relationTypes, *rt)
		}
	}

	return entityTypes, relationTypes, nil
}

// unmarshalDocument parses raw YAML bytes into either an EntityType or
// RelationType. Uses yaml.Node as an intermediate representation to
// determine the kind before full unmarshaling.
func unmarshalDocument(data []byte, path string) (*EntityType, *RelationType, error) {
	var node yaml.Node
	if err := yaml.Unmarshal(data, &node); err != nil {
		return nil, nil, fmt.Errorf("parse schema %s: %w", filepath.Base(path), ErrInvalidSchema)
	}

	return unmarshalNode(&node, path)
}

// unmarshalNode converts a yaml.Node into either an EntityType or RelationType
// based on the kind field. Returns (nil, nil, nil) for empty documents.
func unmarshalNode(node *yaml.Node, path string) (*EntityType, *RelationType, error) {
	// Probe kind by unmarshaling into a lightweight struct
	var probe struct {
		APIVersion string `yaml:"apiVersion"`
		Kind       string `yaml:"kind"`
	}
	if err := node.Decode(&probe); err != nil {
		return nil, nil, fmt.Errorf("parse schema %s: %w", filepath.Base(path), ErrInvalidSchema)
	}

	// Skip empty documents (trailing --- with no content)
	if probe.Kind == "" && probe.APIVersion == "" {
		return nil, nil, nil
	}

	// Validate apiVersion
	if probe.APIVersion != expectedAPIVersion {
		return nil, nil, fmt.Errorf("parse schema %s: %w: got %q, want %q",
			filepath.Base(path), ErrInvalidAPIVersion, probe.APIVersion, expectedAPIVersion)
	}

	// Validate and dispatch by kind
	switch probe.Kind {
	case "EntityType":
		var et EntityType
		if err := node.Decode(&et); err != nil {
			return nil, nil, fmt.Errorf("parse schema %s: %w", filepath.Base(path), ErrInvalidSchema)
		}
		return &et, nil, nil

	case "RelationType":
		var rt RelationType
		if err := node.Decode(&rt); err != nil {
			return nil, nil, fmt.Errorf("parse schema %s: %w", filepath.Base(path), ErrInvalidSchema)
		}
		return nil, &rt, nil

	default:
		return nil, nil, fmt.Errorf("parse schema %s: %w: %q",
			filepath.Base(path), ErrUnsupportedKind, probe.Kind)
	}
}
