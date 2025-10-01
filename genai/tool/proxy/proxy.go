// Package proxy provides a conversation-scoped proxy for Fluxor services.
//
// It implements github.com/viant/fluxor/model/types.Service and routes each
// method call to a per-conversation instance of the underlying service. The
// conversation ID is read from context via github.com/viant/agently/genai/conversation.
//
// Usage:
//
//	base, _ := newYourService("")          // prototype for Name/Methods
//	svc  := proxy.New(base, func(id string) (types.Service, error) {
//	    return newYourService(id)            // build per-conversation instance
//	}, conversation)
//	// svc can now be registered – method execution is routed by conv ID.
package proxy

import (
	"context"
	"fmt"
	"reflect"
	"sync"
	"unsafe"

	convcli "github.com/viant/agently/client/conversation"
	"github.com/viant/agently/genai/memory"
	"github.com/viant/fluxor/model/types"
)

// Proxy is a conversation-aware Service that delegates to per-conversation
// instances of the underlying service created by Factory.
type Proxy struct {
	base         types.Service // prototype: provides Name/Methods and default exec
	services     map[string]map[string]types.Service
	mu           sync.RWMutex // guards services map
	conversation convcli.Client
}

// New constructs a conversation-scoped proxy around the provided base service.
// The base service supplies Name() and Methods(). Executions are dispatched to
// a per-conversation service built on first use via the supplied Factory.
func New(base types.Service, conversation convcli.Client) types.Service {
	return &Proxy{base: base, services: make(map[string]map[string]types.Service), conversation: conversation}
}

// Name delegates to the base prototype service.
func (p *Proxy) Name() string { return p.base.Name() }

// Methods delegates to the base prototype service.
func (p *Proxy) Methods() types.Signatures { return p.base.Methods() }

// Method returns an executable that routes the call to the service instance
// bound to the conversation ID present in ctx. When no conversation ID is
// present, the base service is used.
func (p *Proxy) Method(name string) (types.Executable, error) {
	// We return a tiny shim that defers service selection to execution time,
	// once we have access to the context and thus the conversation ID.
	return func(ctx context.Context, input, output interface{}) error {
		// Determine which concrete service should execute this call.
		svc, err := p.serviceForContext(ctx)
		if err != nil {
			return err
		}
		exec, err := svc.Method(name)
		if err != nil {
			return err
		}
		return exec(ctx, input, output)
	}, nil
}

// serviceForContext resolves the correct service instance for the supplied
// context, creating a new one if needed.
func (p *Proxy) serviceForContext(ctx context.Context) (types.Service, error) {
	convID := ""
	turnID := ""
	if value := ctx.Value(memory.ConversationIDKey); value != nil {
		convID, _ = value.(string)
	}

	if conv, err := p.conversation.GetConversation(ctx, convID); err == nil && conv != nil && conv.LastTurnId != nil {
		turnID = *conv.LastTurnId
	}

	var srv types.Service
	p.mu.RLock()
	turnServices, ok := p.services[convID]
	if ok {
		srv, ok = turnServices[turnID]
	}
	p.mu.RUnlock()
	if ok && srv != nil {
		return srv, nil
	}

	// Slow path: create and memoize
	p.mu.Lock()
	defer p.mu.Unlock()
	if _, ok = p.services[convID]; !ok {
		p.services[convID] = make(map[string]types.Service)
	}
	if srv, ok = p.services[convID][turnID]; ok {
		return srv, nil
	}

	clone, err := cloneStruct(p.base)
	if err != nil {
		return nil, err
	}
	created := clone.(types.Service)
	p.services[convID][turnID] = created
	return created, nil
}

// Caveats:
//   - Works best for POD-like structs. Slices, maps, strings, and pointers will share backing storage or addresses.
//   - Panics if src is not (interface|pointer to struct).
func cloneStruct(src interface{}) (interface{}, error) {
	value := reflect.ValueOf(src)

	// Unwrap interface, if present.
	if value.Type().Kind() == reflect.Interface {
		value = value.Elem()
	}

	// Expect a pointer; dereference to the underlying value.
	if value.Type().Kind() == reflect.Ptr {
		value = value.Elem()
	}

	// Require a struct value to memcpy.
	if value.Type().Kind() != reflect.Struct {
		return nil, fmt.Errorf("cloneStruct: expected pointer to struct (or interface wrapping it)")
	}

	size := value.Type().Size()

	// Allocate new *T
	dstValuePtr := reflect.New(value.Type()) // *T
	dst := dstValuePtr.Elem()                // T

	// Raw memcpy: dst <- src
	srcPtr := unsafe.Pointer(value.UnsafeAddr())
	dstPtr := unsafe.Pointer(dst.UnsafeAddr())

	srcBytes := unsafe.Slice((*byte)(srcPtr), size)
	dstBytes := unsafe.Slice((*byte)(dstPtr), size)
	copy(dstBytes, srcBytes)

	reinitMapsDeep(dst)

	return dstValuePtr.Interface(), nil // *T
}

// reinitMapsDeep traverses a cloned struct value (T) and replaces any map fields
// with new empty maps. It recurses into nested structs and pointers to structs.
// It does not traverse into slices/arrays/maps of structs to avoid unexpected allocations;
// add that if you need it.
func reinitMapsDeep(rv reflect.Value) {
	// Ensure we are working with a struct (dereference pointers if needed).
	for rv.Kind() == reflect.Ptr {
		if rv.IsNil() {
			return
		}
		rv = rv.Elem()
	}
	if rv.Kind() != reflect.Struct {
		return
	}

	rt := rv.Type()
	for i := 0; i < rv.NumField(); i++ {
		fv := rv.Field(i)
		ft := rt.Field(i)
		switch fv.Kind() {
		case reflect.Map:
			// Use unsafe to obtain a settable handle even for unexported fields.
			settable := fv
			if !fv.CanSet() {
				settable = reflect.NewAt(fv.Type(), unsafe.Pointer(fv.UnsafeAddr())).Elem()
			}
			settable.Set(reflect.MakeMap(ft.Type))

		case reflect.Struct:
			// Recurse into nested struct
			reinitMapsDeep(fv)

		case reflect.Ptr:
			// Recurse into pointer-to-struct if non-nil
			if !fv.IsNil() && fv.Type().Elem().Kind() == reflect.Struct {
				reinitMapsDeep(fv)
			}

		}
	}
}
