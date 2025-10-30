package write

import "embed"

//go:embed sql/*.sql
var FS embed.FS

type Input struct {
    Token    *Token `parameter:",kind=body,in=data"`
    CurToken *Token `parameter:",kind=view,in=CurToken" view:"CurToken" sql:"uri=sql/cur_user_oauth_token.sql"`
}

func (i *Input) EmbedFS() (fs *embed.FS) { return &FS }

