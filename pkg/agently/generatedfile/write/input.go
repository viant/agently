package write

import "embed"

//go:embed sql/*.sql
var FS embed.FS

type Input struct {
	GeneratedFiles []*GeneratedFile `parameter:",kind=body,in=data"`

	CurIDs *struct{ Values []string } `parameter:",kind=param,in=GeneratedFiles,dataType=generatedfile/write.GeneratedFiles" codec:"structql,uri=sql/cur_ids.sql"`

	Cur []*GeneratedFile `parameter:",kind=view,in=Cur" view:"Cur" sql:"uri=sql/cur_generated_file.sql"`

	CurByID map[string]*GeneratedFile
}

func (i *Input) EmbedFS() (fs *embed.FS) { return &FS }
