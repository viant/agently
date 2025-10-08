package codec

import (
	"github.com/viant/datly"
)

// SessionOption aliases Datly's session option to enable reuse without
// forcing callers to import datly directly.
type SessionOption = datly.SessionOption
