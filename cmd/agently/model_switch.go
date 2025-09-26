package agently

import (
	"context"
	"fmt"

	"github.com/viant/agently/cmd/service"
)

type ModelSwitchCmd struct {
	Agent string `short:"a" long:"agent" description:"agent file name (without extension)" required:"yes"`
	Model string `short:"m" long:"model" description:"model name to set as default" required:"yes"`
}

func (c *ModelSwitchCmd) Execute(_ []string) error {
	svc := service.New(executorSingleton(), service.Options{})
	if err := svc.SwitchModel(context.Background(), c.Agent, c.Model); err != nil {
		return err
	}
	fmt.Printf("agent %s model switched to %s\n", c.Agent, c.Model)
	return nil
}
