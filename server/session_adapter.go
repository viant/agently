package server

import (
	svcauth "github.com/viant/agently-core/service/auth"
	dexec "github.com/viant/datly/exec"
)

func NewSessionStoreAdapter(invoker dexec.ComponentInvoker) svcauth.SessionStore {
	if invoker == nil {
		return nil
	}
	return svcauth.NewSessionStoreNative(invoker)
}
