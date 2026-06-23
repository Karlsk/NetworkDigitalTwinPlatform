package schema

import (
	"errors"
	"testing"
)

func TestSentinelErrorMessages(t *testing.T) {
	tests := []struct {
		name string
		err  error
		msg  string
	}{
		{
			name: "ErrInvalidSchema",
			err:  ErrInvalidSchema,
			msg:  "invalid schema",
		},
		{
			name: "ErrUnsupportedKind",
			err:  ErrUnsupportedKind,
			msg:  "unsupported kind",
		},
		{
			name: "ErrInvalidAPIVersion",
			err:  ErrInvalidAPIVersion,
			msg:  "invalid api version",
		},
		{
			name: "ErrSchemaNotFound",
			err:  ErrSchemaNotFound,
			msg:  "schema not found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.err.Error() != tt.msg {
				t.Errorf("Error() = %q, want %q", tt.err.Error(), tt.msg)
			}
		})
	}
}

func TestSentinelErrorsConvention(t *testing.T) {
	// All error messages must be lowercase with no trailing period
	allErrors := []error{
		ErrInvalidSchema,
		ErrUnsupportedKind,
		ErrInvalidAPIVersion,
		ErrSchemaNotFound,
	}

	for _, err := range allErrors {
		msg := err.Error()
		if len(msg) == 0 {
			t.Error("sentinel error has empty message")
			continue
		}
		if msg[0] >= 'A' && msg[0] <= 'Z' {
			t.Errorf("error message %q starts with uppercase, convention requires lowercase", msg)
		}
		if msg[len(msg)-1] == '.' {
			t.Errorf("error message %q ends with period, convention forbids trailing period", msg)
		}
	}
}

func TestSentinelErrorsIsDetection(t *testing.T) {
	// Verify errors.Is works correctly for each sentinel error
	tests := []struct {
		name   string
		target error
	}{
		{"ErrInvalidSchema", ErrInvalidSchema},
		{"ErrUnsupportedKind", ErrUnsupportedKind},
		{"ErrInvalidAPIVersion", ErrInvalidAPIVersion},
		{"ErrSchemaNotFound", ErrSchemaNotFound},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Direct match
			if !errors.Is(tt.target, tt.target) {
				t.Errorf("errors.Is(target, target) = false, want true")
			}

			// Wrapped match
			wrapped := errors.Join(errors.New("context"), tt.target)
			if !errors.Is(wrapped, tt.target) {
				t.Errorf("errors.Is(wrapped, target) = false, want true")
			}

			// Non-match (different sentinel)
			if errors.Is(tt.target, errors.New("something else")) {
				t.Errorf("errors.Is(target, different) = true, want false")
			}
		})
	}
}

func TestSentinelErrorsAreDistinct(t *testing.T) {
	// Verify all sentinel errors are distinct from each other
	allErrors := []error{
		ErrInvalidSchema,
		ErrUnsupportedKind,
		ErrInvalidAPIVersion,
		ErrSchemaNotFound,
	}

	for i, e1 := range allErrors {
		for j, e2 := range allErrors {
			if i != j && errors.Is(e1, e2) {
				t.Errorf("errors.Is(allErrors[%d], allErrors[%d]) = true, want false (sentinels must be distinct)", i, j)
			}
		}
	}
}
