package server

import (
	svcauth "github.com/viant/agently-core/service/auth"
	"github.com/viant/datly"
)

func NewSessionStoreAdapter(dao *datly.Service) svcauth.SessionStore {
	if dao == nil {
		return nil
	}
	return svcauth.NewSessionStoreDAO(dao)
}
