package domain

// Store is a composite entry point to the domain services.
type Store interface {
	Conversations() Conversations
	Payloads() Payloads
	//deprecated
	Usage() Usage
	//deprecated
	Messages() Messages
	//deprecated
	Turns() Turns
	//deprecated
	Operations() Operations
}
