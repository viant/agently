package tester

import "github.com/viant/linager/inspector/repository"

type Testable struct {
	Project  *repository.Project
	Packages map[string]bool
}
