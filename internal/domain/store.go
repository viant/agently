package domain

// Store is a composite entry point to the domain services.
type Store interface {
	Conversations() Conversations
	Messages() Messages
	Turns() Turns
	Operations() Operations
	Payloads() Payloads
	Usage() Usage
}
