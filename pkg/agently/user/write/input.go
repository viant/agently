package write

import "embed"

//go:embed sql/*.sql
var FS embed.FS

type Input struct {
	Users []*User `parameter:",kind=body,in=data"`

	CurUsersId *struct{ Values []string } `parameter:",kind=param,in=Users,dataType=user/write.Users" codec:"structql,uri=sql/cur_users_id.sql"`

	CurUser []*User `parameter:",kind=view,in=CurUser" view:"CurUser" sql:"uri=sql/cur_user.sql"`

	CurUserById map[string]*User
}

func (i *Input) EmbedFS() (fs *embed.FS) { return &FS }
