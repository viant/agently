package filebrowser

import (
	"net/http"

	forgefile "github.com/viant/forge/backend/handlers"
	filesvc "github.com/viant/forge/backend/service/file"
)

// New returns an http.Handler exposing the Forge file-browser API rooted at
// the Agently workspace. Supported endpoints:
//
//	GET /list?uri=<path>&folderOnly=true|false
//	GET /download?uri=<path>
//
// It reuses the FileHandler from Forge backend so the UI can navigate and
// download workspace files regardless of scheme (file://, gs://, …).
func New() http.Handler {
	svc := filesvc.New("/")
	fh := forgefile.NewFileBrowser(svc)

	mux := http.NewServeMux()
	mux.HandleFunc("/list", fh.ListHandler)
	mux.HandleFunc("/download", fh.DownloadHandler)
	return mux
}
