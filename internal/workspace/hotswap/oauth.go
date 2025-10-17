package hotswap

import (
	"context"

	"github.com/viant/agently/genai/oauth2"
	oauthfinder "github.com/viant/agently/internal/finder/oauth"
	"github.com/viant/agently/internal/workspace/loader/oauth"
)

// NewOAuthAdaptor links oauth loader and finder into Reloadable for Manager.
func NewOAuthAdaptor(loader *oauthloader.Service, finder *oauthfinder.Finder) Reloadable {
	if loader == nil || finder == nil {
		panic("hotswap: nil oauth loader/finder")
	}

	loadFn := func(ctx context.Context, name string) (*oauth2.Config, error) {
		// loader.Load expects a full path; Manager passes just name (basename).
		// Let loader resolve relative to workspace oauth path by name.
		return loader.Load(ctx, name)
	}

	setFn := func(name string, cfg *oauth2.Config) {
		finder.AddConfig(name, cfg)
	}

	removeFn := finder.Remove

	return NewAdaptor[*oauth2.Config](loadFn, setFn, removeFn)
}
