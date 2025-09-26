package main

import (
	"os"

	_ "github.com/viant/afsc/aws"
	_ "github.com/viant/afsc/gcp"
	_ "github.com/viant/afsc/gs"
	_ "github.com/viant/afsc/s3"
	"github.com/viant/agently/cmd/agently"
)

func main() {
	//
	os.Setenv("AGENTLY_DB_DRIVER", "mysql")
	os.Setenv("AGENTLY_DB_DSN", "root:dev@tcp(127.0.0.1:3307)/agently?parseTime=true")
	os.Setenv("AGENTLY_V1_DOMAIN", "1")
	os.Setenv("AGENTLY_DOMAIN_MODE", "full")
	os.Setenv("AGENTLY_ROOT", "/Users/awitas/go/src/github.com/viant/agently/ag")

	os.Args = []string{"", "serve"}

	//os.Args = []string{"", "chat"}
	//"-q=how big is cat in the image?", "-c=5ce6a243-7025-4c03-a913-6a5746c56ad2", "--attach=/Users/awitas/Downloads/cat05.jpeg::Please describe this image"}

	//os.Args = []string{"", "chat", "-q=What's day today?", "-c=501451ae-3e0b-6d47-89e6-4c32e42edb18"}

	//	os.Args = []string{"", "exec", "-n=sqlkit-dbSetConnection", "-i={}"}
	//
	//convId := fmt.Sprintf("151451ae-3e0b-%v", time.Now().Second())
	//fmt.Println(convId)
	//os.Args = []string{"", "chat", "-q=plan trip to favourite city?", "-c=" + convId}
	////os.Args = []string{"", "chat", "-q=plan trip to my favourite city?", "-c=52115111-2e0b-6d47-49e6-6c32e42eda11"}

	os.Args = []string{"", "serve"}
	agently.RunWithCommands(os.Args[1:])
}
