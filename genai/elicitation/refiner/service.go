package refiner

import mcpschema "github.com/viant/mcp-protocol/schema"

type Service interface {
	RefineRequestedSchema(rs *mcpschema.ElicitRequestParamsRequestedSchema)
}
type DefaultService struct{}

func (DefaultService) RefineRequestedSchema(rs *mcpschema.ElicitRequestParamsRequestedSchema) {
	Refine(rs)
}
