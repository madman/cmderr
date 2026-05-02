# Advanced cmderr Usage

This guide covers more complex scenarios and patterns for using the `cmderr` library.

## Custom Normalization Policy

The `cmderr.From(err)` function has an opinionated default policy. In your project, you might want to extend it to handle domain-specific errors automatically.

```go
func MyProjectFrom(err error) *cmderr.CommandError {
    if err == nil {
        return nil
    }

    // Check for database-specific errors
    if isUniqueConstraintViolation(err) {
        return cmderr.New("ALREADY_EXISTS", err.Error(), cmderr.WithDetail("field", "email"))
    }

    // Fallback to library default
    return cmderr.From(err)
}
```

## Nested Causes

`cmderr` supports serializable nested causes. This is useful when you want to track a chain of failures that happened across different services or layers, and you need the whole chain to be DB-safe.

```go
// Wrapping a previously persisted error
prevErr, _ := cmderr.DecodeJSON(persistedBytes)
newErr := cmderr.New("WORKER_FAILED", "batch processing failed", cmderr.WithCauseCE(prevErr))
```

## Using Options

The `Option` pattern allows adding metadata to errors without changing the constructor signature.

```go
err := cmderr.New("VALIDATION_ERROR", "invalid input",
    cmderr.WithDetail("user_id", 123),
    cmderr.WithDetail("reason", "missing_field"),
)
```

## Interface Encoders

You can register encoders for interfaces. This is useful if you have many error types implementing a common interface.

```go
type TranslatableError interface {
    error
    TranslationKey() string
}

func init() {
    cmderr.RegisterEncode[TranslatableError](func(err TranslatableError) (string, map[string]any) {
        return err.Error(), map[string]any{"key": err.TranslationKey()}
    })
}
```

## Serialization in CQRS

In a CQRS system, you typically use `cmderr` in the Command side to store the failure reason.

```go
func (h *MyCommandHandler) Handle(ctx context.Context, cmd MyCommand) error {
    err := h.service.Execute(cmd)
    
    // Convert to CommandError for storage in the command_results table
    ce := cmderr.From(err)
    data, _ := ce.EncodeJSON()
    
    return h.repo.SaveResult(cmd.ID, data)
}
```
