package service

import (
	"context"
	"reflect"
	"time"
)

// Executable represents a callable method accepting an input value and writing
// into the provided output. Implementations should return an error on failure.
type Executable func(ctx context.Context, input, output interface{}) error

// Signature describes a single method exposed by a Service.
type Signature struct {
	Name        string
	Description string
	Internal    bool
	Input       reflect.Type
	Output      reflect.Type
}

// Signatures is an ordered collection of method signatures.
type Signatures []Signature

// Service is a minimal abstraction grouping a set of executable methods.
type Service interface {
	Name() string
	Methods() Signatures
	Method(name string) (Executable, error)
}

// HasToolTimeout can be implemented by services to suggest a per-tool
// execution timeout. Registries may honor this when executing the service.
// Returned duration should be >0 to take effect.
type HasToolTimeout interface {
	ToolTimeout() time.Duration
}

// Error helpers (compat stamps with fluxor types)

type methodNotFoundError struct{ name string }

func (e methodNotFoundError) Error() string    { return "method not found: " + e.name }
func NewMethodNotFoundError(name string) error { return methodNotFoundError{name: name} }

type invalidInputError struct{ v interface{} }

func (e invalidInputError) Error() string      { return "invalid input" }
func NewInvalidInputError(v interface{}) error { return invalidInputError{v: v} }

type invalidOutputError struct{ v interface{} }

func (e invalidOutputError) Error() string      { return "invalid output" }
func NewInvalidOutputError(v interface{}) error { return invalidOutputError{v: v} }
