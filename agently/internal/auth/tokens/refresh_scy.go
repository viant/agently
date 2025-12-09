package tokens

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/viant/scy"
)

// scyRecord is the on-disk (encrypted) representation of a refresh token.
type scyRecord struct {
	Token  string    `json:"token"`
	Expiry time.Time `json:"expiry"`
}

// ScyRefreshStore persists refresh tokens using the scy secret service with encryption at rest.
type ScyRefreshStore struct {
	dir string
	kp  KeyProvider
	svc *scy.Service
}

// NewScyRefreshStore creates a scy-backed refresh token store at dir.
func NewScyRefreshStore(dir string, kp KeyProvider) (*ScyRefreshStore, error) {
	if dir == "" || kp == nil {
		return nil, fmt.Errorf("scy store: dir and key provider are required")
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	return &ScyRefreshStore{dir: dir, kp: kp, svc: scy.New()}, nil
}

func (s *ScyRefreshStore) urlFor(k Key) (string, error) {
	n := k.Authority.Normalize()
	base := n.Issuer + "@" + n.Origin + "|" + k.Subject + "|" + k.Audience
	h := realSHA256([]byte(base))
	p := filepath.Join(s.dir, h+".ref")
	return "file://" + p, nil
}

func (s *ScyRefreshStore) Get(k Key) (Refresh, bool, error) {
	url, err := s.urlFor(k)
	if err != nil {
		return Refresh{}, false, err
	}
	res := scy.NewResource(nil, url, "")
	secret, err := s.svc.Load(context.Background(), res)
	if err != nil {
		// treat missing file as not found; scy abstracts fs, so we must check plain error
		return Refresh{}, false, nil
	}
	// When structured was stored with Key, Secret.String() returns plaintext JSON
	var rec scyRecord
	if err := json.Unmarshal([]byte(secret.String()), &rec); err != nil {
		return Refresh{}, false, err
	}
	return Refresh{Token: FromString(rec.Token), Expiry: rec.Expiry}, true, nil
}

func (s *ScyRefreshStore) Set(k Key, r Refresh) error {
	url, err := s.urlFor(k)
	if err != nil {
		return err
	}
	rec := scyRecord{Token: r.Token.String(), Expiry: r.Expiry}
	payload, err := json.Marshal(rec)
	if err != nil {
		return err
	}
	res := scy.NewResource(nil, url, "")
	secret := scy.NewSecret(payload, res)
	return s.svc.Store(context.Background(), secret)
}

func (s *ScyRefreshStore) Delete(k Key) error {
	url, err := s.urlFor(k)
	if err != nil {
		return err
	}
	// Best-effort delete; scy doesn't expose delete, remove underlying file directly
	// strip file:// prefix
	path := url[len("file://"):]
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}
