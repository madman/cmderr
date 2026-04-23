package cmderr

import (
	"context"
	"errors"
	"fmt"
	"testing"
)

func TestNew(t *testing.T) {
	e := New("TEST_ERROR", "something went wrong")
	if e.Code != "TEST_ERROR" {
		t.Errorf("expected code TEST_ERROR, got %s", e.Code)
	}
	if e.Msg != "something went wrong" {
		t.Errorf("expected msg 'something went wrong', got %s", e.Msg)
	}
}

func TestErrorString(t *testing.T) {
	tests := []struct {
		err      *CommandError
		expected string
	}{
		{New("CODE", "msg"), "CODE: msg"},
		{New("CODE", ""), "CODE"},
	}

	for _, tt := range tests {
		if tt.err.Error() != tt.expected {
			t.Errorf("expected %s, got %s", tt.expected, tt.err.Error())
		}
	}
}

func TestWithDetail(t *testing.T) {
	e := New("CODE", "msg", WithDetail("key", "value"), WithDetail("foo", 123))
	if e.Details["key"] != "value" {
		t.Errorf("expected detail key=value, got %v", e.Details["key"])
	}
	if e.Details["foo"] != 123 {
		t.Errorf("expected detail foo=123, got %v", e.Details["foo"])
	}
}

func TestWrapAndUnwrap(t *testing.T) {
	cause := errors.New("original error")
	e := Wrap("WRAPPED", cause, "wrapped msg")

	if e.Code != "WRAPPED" {
		t.Errorf("expected code WRAPPED, got %s", e.Code)
	}

	unwrapped := e.Unwrap()
	if unwrapped != cause {
		t.Errorf("expected unwrapped to be original error, got %v", unwrapped)
	}

	// Test nested cause
	causeCE := New("CAUSE", "cause msg")
	e2 := New("OUTER", "outer msg", WithCauseCE(causeCE))

	if e2.Unwrap() != causeCE {
		t.Errorf("expected unwrapped to be causeCE, got %v", e2.Unwrap())
	}
}

func TestIs(t *testing.T) {
	err := New("NOT_FOUND", "not found")

	if !errors.Is(err, &CommandError{Code: "NOT_FOUND"}) {
		t.Error("expected errors.Is to match by code")
	}

	if errors.Is(err, &CommandError{Code: "OTHER"}) {
		t.Error("expected errors.Is NOT to match different code")
	}

	if !errors.Is(err, err) {
		t.Error("expected errors.Is to match itself")
	}
}

type mockDomainError struct {
	ID   string
	User string
}

func (e *mockDomainError) Error() string { return "mock domain error: " + e.ID }

func TestRegistrationAndReconstruction(t *testing.T) {
	code := Code("MOCK_ERROR")

	// Register codec for mockDomainError
	RegisterCodec[*mockDomainError](
		code,
		func(e *mockDomainError) (string, map[string]any) {
			return e.Error(), map[string]any{"id": e.ID, "user": e.User}
		},
		func(ce *CommandError) *mockDomainError {
			id, _ := ce.Details["id"].(string)
			user, _ := ce.Details["user"].(string)
			return &mockDomainError{ID: id, User: user}
		},
	)

	// Test WrapDomain
	original := &mockDomainError{ID: "123", User: "bob"}
	ce := WrapDomain(code, original)

	if ce.Code != code {
		t.Errorf("expected code %s, got %s", code, ce.Code)
	}
	if ce.Details["id"] != "123" {
		t.Errorf("expected detail id=123, got %v", ce.Details["id"])
	}

	// Test errors.As reconstruction
	var target *mockDomainError
	if !errors.As(ce, &target) {
		t.Fatal("expected errors.As to reconstruct mockDomainError")
	}
	if target.ID != "123" || target.User != "bob" {
		t.Errorf("reconstructed error has wrong data: %+v", target)
	}
}

func TestJSONSerialization(t *testing.T) {
	cause := New("CAUSE", "cause msg", WithDetail("foo", "bar"))
	original := New("OUTER", "outer msg", WithDetail("id", 42), WithCauseCE(cause))

	data, err := original.EncodeJSON()
	if err != nil {
		t.Fatalf("failed to encode JSON: %v", err)
	}

	decoded, err := DecodeJSON(data)
	if err != nil {
		t.Fatalf("failed to decode JSON: %v", err)
	}

	if decoded.Code != original.Code || decoded.Msg != original.Msg {
		t.Errorf("decoded error mismatch: %+v", decoded)
	}

	if decoded.Details["id"].(float64) != 42 { // JSON unmarshals numbers as float64
		t.Errorf("expected detail id=42, got %v", decoded.Details["id"])
	}

	if decoded.Cause == nil || decoded.Cause.Code != "CAUSE" {
		t.Errorf("decoded cause mismatch or missing: %+v", decoded.Cause)
	}
}

func TestFrom(t *testing.T) {
	t.Run("nil", func(t *testing.T) {
		if From(nil) != nil {
			t.Error("From(nil) should be nil")
		}
	})

	t.Run("already CommandError", func(t *testing.T) {
		ce := New("EXISTING", "msg")
		if From(ce) != ce {
			t.Error("From should return existing CommandError")
		}
		
		wrapped := fmt.Errorf("wrap: %w", ce)
		if From(wrapped) != ce {
			t.Error("From should extract existing CommandError from chain")
		}
	})

	t.Run("context deadline exceeded", func(t *testing.T) {
		ce := From(context.DeadlineExceeded)
		if ce.Code != ErrTimeout.Code {
			t.Errorf("expected TIMEOUT code, got %s", ce.Code)
		}
	})

	t.Run("context canceled", func(t *testing.T) {
		ce := From(context.Canceled)
		if ce.Code != ErrCanceled.Code {
			t.Errorf("expected CANCELED code, got %s", ce.Code)
		}
	})

	t.Run("internal fallback", func(t *testing.T) {
		err := errors.New("something bad")
		ce := From(err)
		if ce.Code != ErrInternal.Code {
			t.Errorf("expected INTERNAL code, got %s", ce.Code)
		}
		if ce.Details["type"] != "*errors.errorString" {
			t.Errorf("expected type detail, got %v", ce.Details["type"])
		}
	})
}
