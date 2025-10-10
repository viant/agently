package delete

// Input carries the list of message IDs to delete.
type Input struct {
	Ids []string `parameter:",kind=body,in=data"`
}
