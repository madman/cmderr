---
name: cmderr
description: "Durable, serializable representation for asynchronous command errors in Go. Enables error category matching and concrete domain error reconstruction across process boundaries. Use for persisting errors, transmitting them over JSON, and maintaining errors.Is/As functionality in distributed systems or async workflows."
user-invocable: true
license: MIT
compatibility: Designed for Claude Code or similar AI coding agents, and for projects using Golang.
metadata:
  author: madman
  version: "1.0.0"
  openclaw:
    emoji: "🛑"
    homepage: https://github.com/madman/cmderr
    requires:
      bins:
        - go
    install: []
allowed-tools: Read Edit Write Glob Grep Bash(go:*) Bash(git:*) Agent
---

# cmderr

**Persona:** You are a distributed systems reliability engineer. You understand that in asynchronous systems, errors are not just for logs—they are data that must survive persistence and transport without losing their semantic meaning or type information.

## cmderr Best Practices

This skill guides you in using the `cmderr` library to handle errors in asynchronous workflows (CQRS, background jobs, etc.) where errors need to be serializable and reconstructible.

## Best Practices Summary

1. **Use `cmderr.CommandError` for persistence** — When saving an error to a database, always convert it to a `*CommandError`.
2. **Use `cmderr.From(err)` at boundaries** — Normalize any Go error into a serializable `*CommandError` before storage or transmission.
3. **Register Codecs for Domain Errors** — Use `RegisterCodec` to enable `errors.As` support across process boundaries.
4. **Use `WrapDomain` for automatic encoding** — Let the library handle Msg and Details extraction using registered encoders.
5. **Stable Error Codes** — Keep `Code` values stable as they are part of your storage/API contract.
6. **Prefer `errors.Is` for category checks** — Compare errors against `*CommandError` sentinels or codes.
7. **Use `EncodeJSON`/`DecodeJSON`** — Use the library's built-in JSON helpers to preserve reconstruction capabilities.

## Core API Usage

### Normalizing Errors

Always use `From` when you're about to persist or return an error from an async command.

```go
func (w *Worker) Run(ctx context.Context) error {
    err := w.task.Execute(ctx)
    return cmderr.From(err) // Maps timeouts, cancels, and already-wrapped errors
}
```

### Registering Custom Errors

Enable reconstruction of your domain errors after they've been serialized to JSON.

```go
type PaymentError struct {
    TransactionID string
}

func (e *PaymentError) Error() string { return "payment failed" }

func init() {
    cmderr.RegisterCodec[*PaymentError](
        "PAYMENT_FAILED",
        func(err *PaymentError) (string, map[string]any) {
            return err.Error(), map[string]any{"tx_id": err.TransactionID}
        },
        func(ce *cmderr.CommandError) *PaymentError {
            txID, _ := ce.Details["tx_id"].(string)
            return &PaymentError{TransactionID: txID}
        },
    )
}
```

### Wrapping Domain Errors

Use `WrapDomain` to automatically use the registered encoder.

```go
if err != nil {
    return cmderr.WrapDomain("PAYMENT_FAILED", err)
}
```

### Reconstruction and Inspection

Use standard `errors` package functions.

```go
var payErr *PaymentError
if errors.As(err, &payErr) {
    fmt.Println("Failed transaction:", payErr.TransactionID)
}

if errors.Is(err, cmderr.ErrTimeout) {
    // Handle timeout
}
```

## When to use cmderr vs standard errors

- **Standard errors**: Use within a single process, for synchronous logic, or when errors are only for logging.
- **cmderr**: Use when an error needs to be saved to a DB (e.g., `failed_reason` column), sent over an event bus, or returned via an API that needs to preserve the original error type/category for the caller.

## Detailed Reference

- **[Advanced Usage](./references/usage.md)** — Detailed patterns for custom normalization, nested causes, interface encoders, and CQRS integration.

## Related Skills

- → See `golang-error-handling` for general Go error handling principles.
- → See `golang-patterns` for idiomatic Go patterns.
