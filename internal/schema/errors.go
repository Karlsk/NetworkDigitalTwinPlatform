package schema

import "errors"

// Sentinel errors for schema parsing and validation.
// Use errors.Is() to check for specific error types.
var (
	// ErrInvalidSchema indicates malformed YAML or missing required fields.
	ErrInvalidSchema = errors.New("invalid schema")

	// ErrUnsupportedKind indicates an unknown kind value in the YAML document.
	// Valid kinds are "EntityType" and "RelationType".
	ErrUnsupportedKind = errors.New("unsupported kind")

	// ErrInvalidAPIVersion indicates the apiVersion field does not match
	// the expected value "twin.io/v1".
	ErrInvalidAPIVersion = errors.New("invalid api version")
)
