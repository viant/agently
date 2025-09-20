package write

import "embed"

//go:embed sql/*.sql
var FS embed.FS

type Input struct {
	Payloads []*Payload `parameter:",kind=body,in=data"`

	CurIDs *struct{ Values []string } `parameter:",kind=param,in=Payloads,dataType=payload/write.Payloads" codec:"structql,uri=sql/cur_ids.sql"`

	Cur []*Payload `parameter:",kind=view,in=Cur" view:"Cur" sql:"uri=sql/cur_payload.sql"`

	CurByID map[string]*Payload
}

func (i *Input) EmbedFS() (fs *embed.FS) { return &FS }
