package agently

import (
	"context"
	"github.com/viant/agently/cmd/service"
)

type ModelResetCmd struct {
	Agent string `short:"a" long:"agent" description:"agent file name (without extension)" required:"yes"`
}

func (c *ModelResetCmd) Execute(_ []string) error {
	svc := service.New(executorSingleton(), service.Options{})
	if err := svc.ResetModel(context.Background(), c.Agent); err != nil {
		return err
	}
	return nil
}
