package imageio

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/viant/afs"
	"github.com/viant/afs/file"
	"github.com/viant/afs/url"
)

type StoreOptions struct {
	// DestURL is an AFS URL (typically file://...). When empty, a file:// URL
	// under the system temp directory is generated.
	DestURL string
}

// StoreEncodedImage writes an already resized/encoded image to DestURL (or a temp file when empty).
// It returns the destination URL.
func StoreEncodedImage(ctx context.Context, encoded *EncodedImage, opts StoreOptions) (string, error) {
	if encoded == nil || len(encoded.Bytes) == 0 {
		return "", fmt.Errorf("encoded image was empty")
	}
	dest := normalizeDestURL(strings.TrimSpace(opts.DestURL))
	if dest == "" {
		dest = defaultTempImageURL(encoded.MimeType)
	}
	fs := afs.New()
	parent, _ := url.Split(dest, file.Scheme)
	if strings.TrimSpace(parent) != "" {
		exists, err := fs.Exists(ctx, parent)
		if err != nil {
			return "", err
		}
		if !exists {
			if err := fs.Create(ctx, parent, file.DefaultDirOsMode, true); err != nil {
				return "", err
			}
		}
	}
	if err := fs.Upload(ctx, dest, file.DefaultFileOsMode, bytes.NewReader(encoded.Bytes)); err != nil {
		return "", err
	}
	return dest, nil
}

func normalizeDestURL(dest string) string {
	if dest == "" {
		return ""
	}
	// Assume already a URL when it has a scheme.
	if strings.Contains(dest, "://") {
		return dest
	}
	// Treat absolute paths as file:// URLs.
	if filepath.IsAbs(dest) {
		return "file://" + dest
	}
	return dest
}

func defaultTempImageURL(mimeType string) string {
	ext := extensionFromMimeType(mimeType)
	if ext == "" {
		ext = "img"
	}
	name := fmt.Sprintf("agently-image-%d.%s", time.Now().UnixNano(), ext)
	p := filepath.Join(os.TempDir(), name)
	// For absolute paths this produces file:///...
	return "file://" + p
}

func extensionFromMimeType(mimeType string) string {
	mt := strings.ToLower(strings.TrimSpace(mimeType))
	switch mt {
	case "image/png":
		return "png"
	case "image/jpeg", "image/jpg":
		return "jpg"
	default:
		if idx := strings.LastIndex(mt, "/"); idx != -1 && idx < len(mt)-1 {
			return mt[idx+1:]
		}
		return ""
	}
}
