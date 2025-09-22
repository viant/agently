package refiner

import (
	mcpschema "github.com/viant/mcp-protocol/schema"
)

// Service refines elicitation schemas for improved UX.
type Service interface {
	RefineRequestedSchema(rs *mcpschema.ElicitRequestParamsRequestedSchema)
}

// DefaultService adapts the existing global preset-based Refine function to the Service interface.
type DefaultService struct{}

func (DefaultService) RefineRequestedSchema(rs *mcpschema.ElicitRequestParamsRequestedSchema) {
	Refine(rs)
}
