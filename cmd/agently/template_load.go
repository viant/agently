package agently

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/viant/afs"
	templ "github.com/viant/agently-core/protocol/template"
	tplrepo "github.com/viant/agently-core/workspace/repository/template"
	meta "github.com/viant/agently-core/workspace/service/meta"
	fsstore "github.com/viant/agently-core/workspace/store/fs"
)

// TemplateLoadCmd loads a template either directly from a file path or
// through the workspace repository path used by the server.
type TemplateLoadCmd struct {
	File      string `long:"file" description:"Absolute or relative path to a template YAML file to load directly"`
	Workspace string `long:"workspace" description:"Workspace root used for repository-style loading (defaults to AGENTLY_WORKSPACE resolution)"`
	Name      string `long:"name" description:"Template name/id for repository-style loading"`
	JSON      bool   `long:"json" description:"Print the loaded template as JSON"`
}

func (c *TemplateLoadCmd) Execute(_ []string) error {
	ctx := context.Background()
	switch {
	case strings.TrimSpace(c.File) != "":
		return c.loadByFile(ctx)
	case strings.TrimSpace(c.Name) != "":
		return c.loadByWorkspace(ctx)
	default:
		return fmt.Errorf("provide either --file or --name")
	}
}

func (c *TemplateLoadCmd) loadByFile(ctx context.Context) error {
	filename, err := filepath.Abs(strings.TrimSpace(c.File))
	if err != nil {
		return fmt.Errorf("resolve file path: %w", err)
	}
	var tpl templ.Template
	svc := meta.New(afs.New(), filepath.Dir(filename))
	if err := svc.Load(ctx, filename, &tpl); err != nil {
		return fmt.Errorf("load template file %q: %w", filename, err)
	}
	if err := tpl.Validate(); err != nil {
		return fmt.Errorf("validate template file %q: %w", filename, err)
	}
	return c.printTemplate("file", filename, &tpl)
}

func (c *TemplateLoadCmd) loadByWorkspace(ctx context.Context) error {
	store := fsstore.New(strings.TrimSpace(c.Workspace))
	repo := tplrepo.NewWithStore(store)
	name := strings.TrimSpace(c.Name)
	tpl, err := repo.Load(ctx, name)
	if err != nil {
		return fmt.Errorf("load template %q from workspace %q: %w", name, store.Root(), err)
	}
	if tpl == nil {
		return fmt.Errorf("template %q loaded nil", name)
	}
	if err := tpl.Validate(); err != nil {
		return fmt.Errorf("validate template %q from workspace %q: %w", name, store.Root(), err)
	}
	return c.printTemplate("workspace", store.Root(), tpl)
}

func (c *TemplateLoadCmd) printTemplate(mode, source string, tpl *templ.Template) error {
	if c.JSON {
		payload := map[string]any{
			"mode":         mode,
			"source":       source,
			"id":           tpl.ID,
			"name":         tpl.Name,
			"description":  tpl.Description,
			"format":       tpl.Format,
			"appliesTo":    tpl.AppliesTo,
			"platforms":    tpl.Platforms,
			"formFactors":  tpl.FormFactors,
			"surfaces":     tpl.Surfaces,
			"instructions": tpl.Instructions,
			"fences":       tpl.Fences,
			"schema":       tpl.Schema,
			"examples":     tpl.Examples,
		}
		data, err := json.MarshalIndent(payload, "", "  ")
		if err != nil {
			return err
		}
		fmt.Println(string(data))
		return nil
	}

	fmt.Printf("Loaded template successfully\n")
	fmt.Printf("Mode         : %s\n", mode)
	fmt.Printf("Source       : %s\n", source)
	fmt.Printf("ID           : %s\n", strings.TrimSpace(tpl.ID))
	fmt.Printf("Name         : %s\n", strings.TrimSpace(tpl.Name))
	fmt.Printf("Format       : %s\n", strings.TrimSpace(tpl.Format))
	fmt.Printf("Fences       : %d\n", len(tpl.Fences))
	fmt.Printf("Examples     : %d\n", len(tpl.Examples))
	fmt.Printf("Instructions : %d chars\n", len(tpl.Instructions))
	if len(tpl.AppliesTo) > 0 {
		fmt.Printf("AppliesTo    : %s\n", strings.Join(tpl.AppliesTo, ", "))
	}
	return nil
}
