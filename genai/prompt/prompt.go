package prompt

import (
	"bytes"
	"context"
	"crypto/sha1"
	"encoding/hex"
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
		lastSourceHash string             `yaml:"-" json:"-"`
	}
)

func (a *Prompt) Init(ctx context.Context) error {
	if a.URI != "" {
		a.URI = strings.TrimSpace(a.URI)
	}

	if a.Text != "" {
		return nil
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
			return err
		}
		prompt = string(data)
	}
	// Persist resolved prompt text for downstream Generate()
	if strings.TrimSpace(prompt) != "" {
		a.Text = prompt
	}
	return nil

}

func (a *Prompt) Generate(ctx context.Context, binding *Binding) (string, error) {
	if a == nil {
		return "", nil
	}
	// Always prefer latest content when URI is set to support hot-swap edits.
	if strings.TrimSpace(a.URI) != "" {
		fs := afs.New()
		uri := a.URI
		if url.Scheme(uri, "") == "" {
			uri = file.Scheme + "://" + uri
		}
		data, err := fs.DownloadWithURL(ctx, uri)
		if err != nil {
			return "", err
		}
		a.Text = string(data)
	} else {
		// Fall back to initialisation path when only Text is provided
		if err := a.Init(ctx); err != nil {
			return "", err
		}
	}

	prompt := strings.TrimSpace(a.Text)
	if strings.TrimSpace(prompt) == "" {
		return "", nil
	}

	engine := strings.ToLower(strings.TrimSpace(a.Engine))
	switch engine {
	case "go", "gotmpl", "text/template":
		// Guarded reparse: recompute hash and only reset cache when content changed
		if changed := a.updateGoTemplateHash(prompt); changed {
			a.goTemplatePrompt.once = sync.Once{}
			a.goTemplatePrompt.parsedTemplate = nil
			a.goTemplatePrompt.parseErr = nil
		}
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

// updateGoTemplateHash stores and compares a content hash for go-template prompts.
// Returns true when the hash changed since last call.
func (a *Prompt) updateGoTemplateHash(src string) bool {
	// small, stable hash to detect changes
	// reuse NormalizeContent from binding? keep simple raw content hash here
	h := sha1.Sum([]byte(src))
	newHash := hex.EncodeToString(h[:])
	if a.goTemplatePrompt.lastSourceHash == newHash {
		return false
	}
	a.goTemplatePrompt.lastSourceHash = newHash
	return true
}

// generateVeltyPrompt uses velty engine to process the template
func (a *Prompt) generateVeltyPrompt(binding *Binding) (string, error) {
	return templating.Expand(a.Text, binding.Data())
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
