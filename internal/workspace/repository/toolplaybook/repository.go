package toolplaybook

import (
	"context"
	"fmt"
	"path"
	"path/filepath"
	"strings"

	"github.com/viant/afs"
	afsurl "github.com/viant/afs/url"
	"github.com/viant/agently/internal/workspace"
)

// Repository loads markdown playbooks stored under $AGENTLY_WORKSPACE/tools.
// Playbooks are plain text files (typically .md) and are not tool configs.
type Repository struct {
	fs   afs.Service
	dirs []string
}

func New(fs afs.Service, dirs ...string) *Repository {
	roots := make([]string, 0, len(dirs)+1)
	for _, dir := range dirs {
		if strings.TrimSpace(dir) == "" {
			continue
		}
		roots = append(roots, dir)
	}
	if len(roots) == 0 {
		roots = append(roots, workspace.Path(workspace.KindTool))
	}
	return &Repository{fs: fs, dirs: roots}
}

// Load reads a playbook file by name. Name may be provided with or without the .md extension.
// It returns the content and the resolved file:// URI.
func (r *Repository) Load(ctx context.Context, name string) (string, string, error) {
	if r == nil || r.fs == nil {
		return "", "", fmt.Errorf("tool playbook repository not configured")
	}

	filename, err := normalizeName(name)
	if err != nil {
		return "", "", err
	}

	for _, root := range r.dirs {
		location, err := joinRoot(root, filename)
		if err != nil {
			return "", "", err
		}
		ok, err := r.fs.Exists(ctx, location)
		if err != nil {
			return "", "", err
		}
		if !ok {
			continue
		}
		data, err := r.fs.DownloadWithURL(ctx, location)
		if err != nil {
			return "", "", err
		}
		return string(data), asSourceURI(location), nil
	}
	return "", "", nil
}

func normalizeName(name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", fmt.Errorf("playbook name was empty")
	}
	if filepath.Ext(name) == "" {
		name += ".md"
	}
	clean := path.Clean(filepath.ToSlash(name))
	if strings.HasPrefix(clean, "../") || clean == ".." || strings.HasPrefix(clean, "/") {
		return "", fmt.Errorf("invalid playbook path: %s", name)
	}
	return clean, nil
}

func joinRoot(root, rel string) (string, error) {
	root = strings.TrimSpace(root)
	if root == "" {
		return "", fmt.Errorf("playbook root was empty")
	}
	if afsurl.Scheme(root, "") != "" {
		return afsurl.JoinUNC(root, rel), nil
	}
	return filepath.Join(root, filepath.FromSlash(rel)), nil
}

func asSourceURI(location string) string {
	if strings.TrimSpace(location) == "" {
		return ""
	}
	if afsurl.Scheme(location, "") != "" {
		return location
	}
	return afsurl.ToFileURL(location)
}
