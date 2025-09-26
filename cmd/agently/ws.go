package agently

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/viant/agently/cmd/service"
	repoiface "github.com/viant/agently/internal/repository"
)

// WorkspaceCmd groups workspace sub-commands.
type WorkspaceCmd struct {
	List   *WsListCmd   `command:"list" description:"List resources (agents|models|workflows)"`
	Get    *WsGetCmd    `command:"get" description:"Print resource YAML"`
	Add    *WsAddCmd    `command:"add" description:"Add or overwrite a resource"`
	Remove *WsRemoveCmd `command:"remove" description:"Delete resource"`
}

// ---- Common flag struct ---------------------------------------------------
type wsCommon struct {
	Kind string `short:"k" long:"kind" required:"yes" description:"Resource kind: agent|model|workflow"`
	Name string `short:"n" long:"name" description:"Resource name (without extension)"`
}

func (c *wsCommon) repo(s *service.Service) (repoiface.CRUD, error) {
	switch strings.ToLower(c.Kind) {
	case "model", "models":
		return s.ModelRepo(), nil
	case "agent", "agents":
		return s.AgentRepo(), nil
	case "workflow", "workflows":
		return s.WorkflowRepo(), nil
	default:
		return nil, fmt.Errorf("unknown kind %s", c.Kind)
	}
}

// ---- list -----------------------------------------------------------------
type WsListCmd struct{ wsCommon }

func (c *WsListCmd) Execute(_ []string) error {
	svc := service.New(executorSingleton(), service.Options{})
	repo, err := c.wsCommon.repo(svc)
	if err != nil {
		return err
	}

	var names []string
	switch r := repo.(type) {
	case interface {
		List(context.Context) ([]string, error)
	}:
		names, err = r.List(context.Background())
	default:
		err = fmt.Errorf("repo kind not supported for list")
	}
	if err != nil {
		return err
	}
	bytes, _ := json.MarshalIndent(names, "", "  ")
	fmt.Println(string(bytes))
	return nil
}

// ---- get ------------------------------------------------------------------
type WsGetCmd struct{ wsCommon }

func (c *WsGetCmd) Execute(_ []string) error {
	if c.Name == "" {
		return fmt.Errorf("--name required")
	}
	svc := service.New(executorSingleton(), service.Options{})
	repo, err := c.wsCommon.repo(svc)
	if err != nil {
		return err
	}

	data, err := repo.GetRaw(context.Background(), c.Name)
	if err != nil {
		return err
	}
	fmt.Print(string(data))
	return nil
}

// ---- add ------------------------------------------------------------------
type WsAddCmd struct {
	wsCommon
	File string `short:"f" long:"file" description:"YAML file ('-' for stdin)"`
}

func (c *WsAddCmd) readData() ([]byte, error) {
	var rdr io.Reader
	if c.File == "-" || c.File == "" {
		rdr = os.Stdin
	} else {
		f, err := os.Open(c.File)
		if err != nil {
			return nil, err
		}
		defer f.Close()
		rdr = f
	}
	return io.ReadAll(rdr)
}

func (c *WsAddCmd) Execute(_ []string) error {
	if c.Name == "" {
		return fmt.Errorf("--name required")
	}
	data, err := c.readData()
	if err != nil {
		return err
	}
	svc := service.New(executorSingleton(), service.Options{})
	repo, err := c.wsCommon.repo(svc)
	if err != nil {
		return err
	}

	return repo.Add(context.Background(), c.Name, data)
}

// ---- remove ---------------------------------------------------------------
type WsRemoveCmd struct{ wsCommon }

func (c *WsRemoveCmd) Execute(_ []string) error {
	if c.Name == "" {
		return fmt.Errorf("--name required")
	}
	svc := service.New(executorSingleton(), service.Options{})
	repo, err := c.wsCommon.repo(svc)
	if err != nil {
		return err
	}
	return repo.Delete(context.Background(), c.Name)
}
