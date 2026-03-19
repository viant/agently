package query

import (
	"os"
	"testing"

	"github.com/viant/agently/internal/gobootstrap"
)

func TestMain(m *testing.M) {
	gobootstrap.EnableDiagnostics()
	os.Exit(m.Run())
}
