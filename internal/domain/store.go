package domain

// Store is a composite entry point to the domain services.
// Deprecated: use client/conversation and client/chat clients instead.
type Store interface {
	Conversations() Conversations
	Payloads() Payloads
	Messages() Messages
	Turns() Turns
	Operations() Operations
}
