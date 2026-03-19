# Go Error Handling Best Practices

## Rule 1: Always wrap errors with context
Use fmt.Errorf with %w to preserve the error chain:
```go
if err != nil {
    return fmt.Errorf("failed to open config: %w", err)
}
```

## Rule 2: Define sentinel errors for expected conditions
```go
var ErrNotFound = errors.New("resource not found")
var ErrPermissionDenied = errors.New("permission denied")
```

## Rule 3: Use errors.Is and errors.As for checking
```go
if errors.Is(err, ErrNotFound) {
    // handle not found
}
```

## Rule 4: Never ignore errors silently
Bad: `result, _ := doSomething()`
Good: `result, err := doSomething(); if err != nil { return err }`

## Rule 5: Return early on error
```go
func process(data []byte) error {
    if len(data) == 0 {
        return errors.New("empty data")
    }
    // continue processing...
    return nil
}
```
