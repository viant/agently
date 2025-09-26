package elicitation

import (
	presetrefiner "github.com/viant/agently/genai/elicitation/refiner"
	"github.com/viant/mcp-protocol/schema"
)

// DefaultRefiner adapts the preset refiner to the local Refiner interface.
type DefaultRefiner struct{}

func (DefaultRefiner) RefineRequestedSchema(rs *schema.ElicitRequestParamsRequestedSchema) {
	presetrefiner.Refine(rs)
}
