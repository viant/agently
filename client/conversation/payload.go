package conversation

import (
	write "github.com/viant/agently/pkg/agently/payload"
	"github.com/viant/agently/pkg/agently/payload/read"
)

type MutablePayload write.Payload
type Payload read.PayloadView
