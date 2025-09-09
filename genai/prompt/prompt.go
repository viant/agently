package prompt

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"sync"
	"text/template"

	afs "github.com/viant/afs"
	"github.com/viant/afs/file"
	"github.com/viant/afs/url"
	"github.com/viant/agently/internal/templating"
)

type (
	Prompt struct {
		Text   string `yaml:"text,omitempty" json:"text,omitempty"`
		URI    string `yaml:"uri,omitempty" json:"uri,omitempty"`
		Engine string `yaml:"engine,omitempty" json:"engine,omitempty"`
		goTemplatePrompt
	}

	goTemplatePrompt struct {
		once           sync.Once
		parsedTemplate *template.Template `yaml:"-" json:"-"`
		parseErr       error              `yaml:"-" json:"-"`
	}
)

func (a *Prompt) Generate(ctx context.Context, binding *Binding) (string, error) {
	if a == nil {
		return "", nil
	}
	// Determine template source
	prompt := strings.TrimSpace(a.Text)
	if prompt == "" && strings.TrimSpace(a.URI) != "" {
		fs := afs.New()
		uri := a.URI
		if url.Scheme(uri, "") == "" {
			uri = file.Scheme + "://" + uri
		}
		data, err := fs.DownloadWithURL(ctx, uri)
		if err != nil {
			return "", err
		}
		prompt = string(data)
	}
	if strings.TrimSpace(prompt) == "" {
		return "", nil
	}

	engine := strings.ToLower(strings.TrimSpace(a.Engine))
	switch engine {
	case "go", "gotmpl", "text/template":
		return a.generateGoTemplatePrompt(prompt, binding)
	case "velty", "vm", "":
		// Use velty (velocity-like) by default
		// Ensure a.Text carries the active prompt text
		a.Text = prompt
		return a.generateVeltyPrompt(binding)
	default:
		// Unknown engine type
		return "", errors.New("unsupported prompt type: " + engine)
	}
}

// generateVeltyPrompt uses velty engine to process the template
func (a *Prompt) generateVeltyPrompt(binding *Binding) (string, error) {
	var context = map[string]interface{}{
		"Task":      binding.Task,
		"History":   binding.History,
		"Tools":     binding.Tools,
		"Flags":     binding.Flags,
		"Documents": binding.Documents,
	}
	return templating.Expand(a.Text, context)
}

func (a *goTemplatePrompt) generateGoTemplatePrompt(prompt string, binding *Binding) (string, error) {
	// lazily compile template once
	a.once.Do(func() {
		a.parsedTemplate, a.parseErr = template.New("prompt").Parse(prompt)
	})
	if a.parseErr != nil {
		return "", a.parseErr
	}
	var buf bytes.Buffer
	if err := a.parsedTemplate.Execute(&buf, binding); err != nil {
		return "", err
	}
	return buf.String(), nil
}
