package factory

import (
	"github.com/viant/agently/client/chat"
	d "github.com/viant/agently/internal/domain"
	internal "github.com/viant/agently/internal/service/chat"
)

// New constructs a chat API backed by the internal chat service.
func New(store d.Store) chat.API {
	return internal.NewService(store)
}
