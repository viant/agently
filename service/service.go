package service

import (
	execpkg "github.com/viant/agently/genai/executor"
)

// Options configures behaviour of Service.
type Options struct {
	Interaction InteractionHandler // optional
}

// Service exposes high-level operations (currently Chat) that are decoupled
// from any particular user-interface.
type Service struct {
	exec *execpkg.Service
	opts Options
}

// New returns a Service using the supplied executor.Service. Ownership of
// exec is left to the caller – Service does not Stop()/Shutdown() it.
func New(exec *execpkg.Service, opts Options) *Service {
	return &Service{exec: exec, opts: opts}
}
