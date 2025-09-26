package factory

import (
	cht "github.com/viant/agently/client/chat"
	internal "github.com/viant/agently/internal/service/chat"
)

// New constructs a chat API backed by the internal chat service.
func New() cht.Client {
	return internal.NewService()
}
