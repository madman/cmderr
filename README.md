# cmderr

`cmderr` provides a durable, serializable representation for asynchronous command errors in Go. It enables error category matching and concrete domain error reconstruction across process boundaries.

[![Go Reference](https://pkg.go.dev/badge/github.com/madman/cmderr.svg)](https://pkg.go.dev/github.com/madman/cmderr)

## Why cmderr?

In distributed systems or asynchronous workflows (like background workers or CQRS), errors often need to be:

1. **Persisted** to a database.
2. **Transmitted** over the wire (JSON).
3. **Reconstructed** in another process to maintain `errors.Is` and `errors.As` functionality.

`cmderr` solves this by decoupling the durable error category (a string-based `Code`) from the in-memory concrete error type.

## Features

- ✅ **Serializability**: Built-in support for JSON marshaling/unmarshaling of error chains.
- ✅ **Category Matching**: Compare errors across process boundaries using string codes (`errors.Is`).
- ✅ **Domain Error Reconstruction**: Register codecs to automatically restore concrete domain error types when decoding (`errors.As`).
- ✅ **Recursive Chains**: Supports nested causes for complex failure tracking.
- ✅ **Opinionated Normalization**: Easily convert standard Go errors (including context timeouts/cancellations) into durable `CommandError` instances.

## Installation

```bash
go get github.com/madman/cmderr
```

## Quick Start

### Basic Usage

Create a new command error or wrap an existing one:

```go
import "github.com/madman/cmderr"

// Simple error with a code
err := cmderr.New("INSUFFICIENT_FUNDS", "Account balance is too low")

// Wrap an existing error
wrapped := cmderr.Wrap("DB_ERROR", queryErr, "failed to update user")
```

### Normalizing Errors

Use `cmderr.From()` at command boundaries to ensure every error is a `*CommandError`. It automatically maps common errors like `context.DeadlineExceeded` to predefined codes.

```go
func (w *Worker) Process(ctx context.Context) error {
    err := w.service.DoWork(ctx)
    return cmderr.From(err) // Ensures result is serializable
}
```

## Working with Domain Errors

`cmderr` allows you to maintain type-safe error handling even after serialization.

### 1. Register a Codec

Register how your domain error should be encoded to and decoded from a `CommandError`.

```go
type LowBalanceError struct {
    Amount float64
}

func (e *LowBalanceError) Error() string { return "balance too low" }

func init() {
    cmderr.RegisterCodec[*LowBalanceError](
        "BALANCE_LOW",
        func(err *LowBalanceError) (string, map[string]any) {
            return err.Error(), map[string]any{"amount": err.Amount}
        },
        func(ce *cmderr.CommandError) *LowBalanceError {
            amount, _ := ce.Details["amount"].(float64)
            return &LowBalanceError{Amount: amount}
        },
    )
}
```

### 2. Wrap and Reconstruct

Now you can wrap your domain error and later reconstruct it using `errors.As`.

```go
// In Service A:
err := &LowBalanceError{Amount: 10.50}
cmdErr := cmderr.WrapDomain("BALANCE_LOW", err)

// ... persist to DB or send over wire ...

// In Service B (after decoding):
var lowBalance *LowBalanceError
if errors.As(cmdErr, &lowBalance) {
    fmt.Printf("Recovered amount: %.2f\n", lowBalance.Amount)
}
```

## Serialization

`cmderr` provides helpers for JSON serialization that preserve the error chain and reconstruction capabilities.

```go
// Encode for storage
bytes, err := cmdErr.EncodeJSON()

// Decode from storage
recoveredErr, err := cmderr.DecodeJSON(bytes)
```

## Predefined Categories

The package includes several common categories that you can use out of the box:

- `cmderr.ErrTimeout` (`"TIMEOUT"`)
- `cmderr.ErrCanceled` (`"CANCELED"`)
- `cmderr.ErrInternal` (`"INTERNAL"`)

---

## AI Agent Skill

`cmderr` includes a built-in **AI Agent Skill** that provides procedural knowledge to AI coding assistants (like Claude Code, Cursor, etc.) on how to use this library correctly.

### Skill Installation

You can install the skill using the [skills.sh](https://skills.sh) CLI:

```bash
npx skills add madman/cmderr
```

Or, if you are working directly in this repository, you can link the local skill:

```bash
./skills.sh link skills/cmderr
```

### Benefits

Once installed, your AI agent will:

- Know the best practices for error normalization and persistence.
- Automatically suggest using `cmderr.From(err)` at command boundaries.
- Help you register custom codecs and wrap domain errors correctly.
- Understand when to use `cmderr` vs. standard Go errors.

---

> [!TIP]
> Always use `cmderr.From()` when persisting command results to ensure a consistent, durable error schema in your database.
