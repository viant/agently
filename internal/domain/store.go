package domain

// Store is a composite entry point to the domain services.
type Store interface {
	Conversations() Conversations
	Payloads() Payloads
	Messages() Messages
	Turns() Turns
	Operations() Operations
}
