// Package cmderr provides a durable, serializable representation for asynchronous command errors.
// It enables error category matching and concrete domain error reconstruction across process boundaries.
package cmderr

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
)

// Common categories you can reuse across bounded contexts.
// Importers may define their own codes (e.g., "PAYMENT_DECLINED").
var (
	// ErrTimeout represents a timeout or deadline exceeded error.
	ErrTimeout = &CommandError{Code: "TIMEOUT"}
	// ErrCanceled represents an operation that was canceled.
	ErrCanceled = &CommandError{Code: "CANCELED"}
	// ErrInternal represents an unexpected internal error.
	ErrInternal = &CommandError{Code: "INTERNAL"}
)

var defaultCodecs codecRegistry

type (
	// Code is an open string-based enum for error categories persisted in DB.
	// Keep it stable over time, since it becomes part of your storage/API contract.
	Code string

	// CommandError is the durable, serializable representation for async command errors.
	// It can wrap a "concrete" domain error (used for errors.As) and/or contain a nested
	// CommandError cause (for recursive chains). Only this structure is persisted.
	CommandError struct {
		// concrete is an in-memory domain-specific error reconstructed via Decode func.
		// It is NOT serialized. This enables errors.As(err, &MyDomainError).
		concrete error

		// Details contains serialized context about the error.
		Details map[string]any `json:"details,omitempty"`

		// Cause is a nested, serialized CommandError (DB-safe). Use when you need a
		// strictly-serializable chain (e.g., composed/aggregated failures).
		Cause *CommandError `json:"cause,omitempty"`

		// Code is the category of the error.
		Code Code `json:"code"`

		// Msg is a human-readable description of the error.
		Msg string `json:"msg,omitempty"`
	}

	// Option mutates CommandError construction.
	Option func(*CommandError)

	// ErrorEncodeFunc encodes a concrete domain error into (Msg, Details).
	// It must return ok=false if the passed err is not of the expected type.
	ErrorEncodeFunc func(err error) (msg string, details map[string]any, ok bool)

	// ErrorDecodeFunc decodes a persisted CommandError into a concrete domain error.
	ErrorDecodeFunc func(ce *CommandError) error

	// codecRegistry keeps two orthogonal maps:
	//   - byCode: Code -> decode func (used on load / errors.As)
	//   - byType: reflect.Type -> encode func (used on WrapDomain)
	codecRegistry struct {
		byCode map[Code]ErrorDecodeFunc
		byType map[reflect.Type]ErrorEncodeFunc
	}
)

// RegisterCodec registers BOTH encode and decode in one call.
// T must be a concrete error type (e.g., *payments.BalanceLowError).
func RegisterCodec[T error](
	code Code,
	encode func(T) (msg string, details map[string]any),
	decode func(*CommandError) T,
) {
	RegisterEncode[T](encode)
	RegisterDecode(code, func(ce *CommandError) error { return decode(ce) })
}

// RegisterDecode binds a decode func to a category Code.
func RegisterDecode(code Code, decode ErrorDecodeFunc) {
	if defaultCodecs.byCode == nil {
		defaultCodecs.byCode = make(map[Code]ErrorDecodeFunc)
	}
	defaultCodecs.byCode[code] = decode
}

// RegisterEncode binds an encode func to a concrete error type T using generics.
func RegisterEncode[T error](encode func(T) (msg string, details map[string]any)) {
	if defaultCodecs.byType == nil {
		defaultCodecs.byType = make(map[reflect.Type]ErrorEncodeFunc)
	}
	rt := reflect.TypeOf((*T)(nil)).Elem()
	defaultCodecs.byType[rt] = func(err error) (string, map[string]any, bool) {
		var v T
		ok := errors.As(err, &v)
		if !ok {
			return "", nil, false
		}
		m, d := encode(v)
		return m, d, true
	}
}

func (e *CommandError) Error() string {
	if e.Msg != "" {
		return fmt.Sprintf("%s: %s", e.Code, e.Msg)
	}
	return string(e.Code)
}

// Unwrap exposes the next error in the chain for stdlib errors package.
// Priority: concrete domain error -> nested CommandError cause.
func (e *CommandError) Unwrap() error {
	if e.concrete != nil {
		return e.concrete
	}
	if e.Cause != nil {
		return e.Cause
	}
	return nil
}

// Is enables errors.Is(err, target) semantics across process boundaries by
// comparing category codes. Target should be a *CommandError sentinel with Code set.
func (e *CommandError) Is(target error) bool {
	if t, ok := target.(*CommandError); ok {
		return e.Code == t.Code
	}
	return false
}

// As enables errors.As(err, &T) by delegating to a reconstructed concrete type
// via registry; falls back to nested cause if present.
func (e *CommandError) As(target any) bool {
	// 1) Directly satisfy *CommandError.
	if p, ok := target.(**CommandError); ok {
		*p = e
		return true
	}
	// 2) If we already have a concrete domain error, delegate to it.
	if e.concrete != nil && errors.As(e.concrete, target) {
		return true
	}
	// 3) Attempt to reconstruct a concrete error via registry based on Code.
	if dec := defaultCodecs.lookupDecode(e.Code); dec != nil {
		if conc := dec(e); conc != nil {
			e.concrete = conc
			return errors.As(conc, target)
		}
	}
	// 4) Delegate to serialized cause if any.
	if e.Cause != nil {
		return errors.As(e.Cause, target)
	}
	return false
}

// New builds a standalone CommandError.
func New(code Code, msg string, opts ...Option) *CommandError {
	e := &CommandError{Code: code, Msg: msg}
	for _, o := range opts {
		o(e)
	}
	return e
}

// Wrap attaches an in-memory concrete cause (not serialized) while fixing the
// durable category Code for persistence and errors.Is matching.
func Wrap(code Code, cause error, msg string, opts ...Option) *CommandError {
	e := &CommandError{Code: code, Msg: msg, concrete: cause}
	for _, o := range opts {
		o(e)
	}
	return e
}

// WrapDomain uses a registered encoder (if any) to auto-fill Msg/Details
// from a concrete domain error, while fixing the durable category Code.
// If no encoder found, it behaves like Wrap(code, cause, msg="").
func WrapDomain(code Code, cause error, opts ...Option) *CommandError {
	if cause == nil {
		return New(code, "", opts...)
	}

	var msg string
	var det map[string]any
	if enc, ok := defaultCodecs.lookupEncode(cause); ok {
		if m, d, ok2 := enc(cause); ok2 {
			msg, det = m, d
		}
	}
	e := &CommandError{Code: code, Msg: msg, Details: det, concrete: cause}
	for _, o := range opts {
		o(e)
	}
	return e
}

// WithDetail adds a key-value pair into Details map (serialized).
func WithDetail(k string, v any) Option {
	return func(e *CommandError) {
		if e.Details == nil {
			e.Details = map[string]any{}
		}
		e.Details[k] = v
	}
}

// WithCauseCE sets a nested, serialized CommandError cause.
// Use when your cause is already in CommandError form (e.g., rethrowing persisted error).
func WithCauseCE(cause *CommandError) Option {
	return func(e *CommandError) { e.Cause = cause }
}

// EncodeJSON marshals the CommandError for DB storage.
func (e *CommandError) EncodeJSON() ([]byte, error) { return json.Marshal(e) }

// DecodeJSON rebuilds a CommandError from DB and attempts to reconstruct
// a concrete domain error from registry for errors.As support.
// Causes are handled recursively.
func DecodeJSON(b []byte) (*CommandError, error) {
	var e CommandError
	if err := json.Unmarshal(b, &e); err != nil {
		return nil, err
	}
	if dec := defaultCodecs.lookupDecode(e.Code); dec != nil {
		e.concrete = dec(&e)
	}
	if e.Cause != nil {
		if dec := defaultCodecs.lookupDecode(e.Cause.Code); dec != nil {
			e.Cause.concrete = dec(e.Cause)
		}
	}
	return &e, nil
}

// From converts any error to a *CommandError following a normalization policy.
// Use it at async command boundaries (worker completion, status persistence).
//
// Policy (opinionated defaults):
//   - nil -> nil
//   - already CommandError (anywhere in chain) -> extract and return
//   - context.DeadlineExceeded
//   - context.Canceled
//   - otherwise -> INTERNAL
//
// You can extend this policy in your project (e.g., detect unique-constraint
// violations as CONFLICT, map gRPC status codes, etc.).
func From(err error) *CommandError {
	if err == nil {
		return nil
	}

	// If the chain already has a CommandError, prefer that.
	var ce *CommandError
	if errors.As(err, &ce) {
		return ce
	}

	// TIMEOUT
	if errors.Is(err, context.DeadlineExceeded) {
		return &CommandError{Code: ErrTimeout.Code, Msg: err.Error(), concrete: err}
	}

	// CANCELED
	if errors.Is(err, context.Canceled) {
		return &CommandError{Code: ErrCanceled.Code, Msg: err.Error(), concrete: err}
	}

	// Fallback to INTERNAL, echoing error message and type for observability.
	return &CommandError{
		Code: ErrInternal.Code,
		Msg:  err.Error(),
		Details: map[string]any{
			"type": fmt.Sprintf("%T", err),
		},
		concrete: err,
	}
}

func (r *codecRegistry) lookupDecode(code Code) ErrorDecodeFunc {
	if r == nil || r.byCode == nil {
		return nil
	}
	return r.byCode[code]
}

func (r *codecRegistry) lookupEncode(err error) (ErrorEncodeFunc, bool) {
	if r == nil || r.byType == nil || err == nil {
		return nil, false
	}
	et := reflect.TypeOf(err)
	// direct match
	if enc, ok := r.byType[et]; ok {
		return enc, true
	}
	// try assignable/interface matches (covers interface error types)
	for t, enc := range r.byType {
		// exact interface support
		if t.Kind() == reflect.Interface && et.Implements(t) {
			return enc, true
		}
		// assignable (rare, but safe)
		if et.AssignableTo(t) {
			return enc, true
		}
	}
	return nil, false
}
