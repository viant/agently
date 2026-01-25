package delete

type Input struct {
	Ids []string `parameter:",kind=query,in=id" json:"ids,omitempty"`
}
