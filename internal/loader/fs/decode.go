package fs

import "github.com/viant/fluxor/service/meta/yml"

// DecodeFunc is a function type that decodes a YAML node into a specific type
type DecodeFunc[T any] func(node *yml.Node, t *T) error
