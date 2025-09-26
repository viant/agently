package read

import "time"

type InputOption func(*Input)

func WithTenantID(id string) InputOption {
	return func(in *Input) { in.TenantID = id; ensureHas(&in.Has); in.Has.TenantID = true }
}
func WithID(id string) InputOption {
	return func(in *Input) { in.Id = id; ensureHas(&in.Has); in.Has.Id = true }
}
func WithIDs(ids ...string) InputOption {
	return func(in *Input) { in.Ids = ids; ensureHas(&in.Has); in.Has.Ids = true }
}
func WithKind(kind string) InputOption {
	return func(in *Input) { in.Kind = kind; ensureHas(&in.Has); in.Has.Kind = true }
}
func WithDigest(d string) InputOption {
	return func(in *Input) { in.Digest = d; ensureHas(&in.Has); in.Has.Digest = true }
}
func WithStorage(s string) InputOption {
	return func(in *Input) { in.Storage = s; ensureHas(&in.Has); in.Has.Storage = true }
}
func WithMimeType(m string) InputOption {
	return func(in *Input) { in.MimeType = m; ensureHas(&in.Has); in.Has.MimeType = true }
}
func WithSince(ts time.Time) InputOption {
	return func(in *Input) { t := ts; in.Since = &t; ensureHas(&in.Has); in.Has.Since = true }
}
func WithInput(src Input) InputOption { return func(in *Input) { *in = src } }

func ensureHas(h **Has) {
	if *h == nil {
		*h = &Has{}
	}
}
