package user

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/google/uuid"
	userread "github.com/viant/agently/pkg/agently/user"
	userwrite "github.com/viant/agently/pkg/agently/user/write"
	"github.com/viant/datly"
	"github.com/viant/datly/repository/contract"
	"time"
)

// Service provides minimal CRUD helpers on top of datly for users.
type Service struct{ dao *datly.Service }

// New registers user components on the provided datly service.
func New(ctx context.Context, dao *datly.Service) (*Service, error) {
	if dao == nil {
		return nil, fmt.Errorf("nil datly service")
	}
	if err := userread.DefineComponent(ctx, dao); err != nil {
		return nil, err
	}
	if err := userread.DefineListComponent(ctx, dao); err != nil {
		return nil, err
	}
	if _, err := userwrite.DefineComponent(ctx, dao); err != nil {
		return nil, err
	}
	return &Service{dao: dao}, nil
}

// FindByUsername returns a single user view or nil.
func (s *Service) FindByUsername(ctx context.Context, username string) (*userread.View, error) {
	in := &userread.Input{}
	// Extend: use query predicate by adding Username into Input (see user.go changes)
	in.Has = &userread.InputHas{}
	// We rely on user view predicate builder using username; fallback by list filtering below if not supported.
	out := &userread.Output{}
	if _, err := s.dao.Operate(ctx, datly.WithOutput(out), datly.WithURI(userread.PathListURI), datly.WithInput(in)); err != nil {
		return nil, err
	}
	uname := strings.ToLower(strings.TrimSpace(username))
	for _, v := range out.Data {
		if v != nil && strings.ToLower(strings.TrimSpace(v.Username)) == uname {
			vv := *v
			return &vv, nil
		}
	}
	return nil, nil
}

// UpsertLocal ensures a user with provider=local exists/updated. Returns user id.
func (s *Service) UpsertLocal(ctx context.Context, username, displayName, email string) (string, error) {
	var id string
	if v, err := s.FindByUsername(ctx, username); err == nil && v != nil {
		id = v.Id
	}
	if id == "" {
		id = uuid.NewString()
	}
	u := &userwrite.User{}
	u.SetId(id)
	u.SetUsername(username)
	if strings.TrimSpace(displayName) != "" {
		u.SetDisplayName(displayName)
	}
	if strings.TrimSpace(email) != "" {
		u.SetEmail(email)
	}
	u.SetProvider("local")
	if strings.TrimSpace(u.Timezone) == "" {
		u.SetTimezone("UTC")
	}
	out := &userwrite.Output{}
	in := &userwrite.Input{Users: []*userwrite.User{u}}
	if _, err := s.dao.Operate(ctx,
		datly.WithPath(contract.NewPath("PATCH", userwrite.PathURI)),
		datly.WithInput(in),
		datly.WithOutput(out)); err != nil {
		return "", err
	}
	return id, nil
}

// UpsertWithProvider ensures a user exists/updated with a specific provider/subject. Returns user id.
func (s *Service) UpsertWithProvider(ctx context.Context, username, displayName, email, provider, subject string) (string, error) {
	var id string
	if v, err := s.FindByUsername(ctx, username); err == nil && v != nil {
		id = v.Id
	}
	if id == "" {
		id = uuid.NewString()
	}
	u := &userwrite.User{}
	u.SetId(id)
	u.SetUsername(username)
	if strings.TrimSpace(displayName) != "" {
		u.SetDisplayName(displayName)
	}
	if strings.TrimSpace(email) != "" {
		u.SetEmail(email)
	}
	if strings.TrimSpace(provider) == "" {
		provider = "oauth"
	}
	u.SetProvider(provider)
	if strings.TrimSpace(subject) != "" {
		u.SetSubject(subject)
	}
	if strings.TrimSpace(u.Timezone) == "" {
		u.SetTimezone("UTC")
	}
	out := &userwrite.Output{}
	in := &userwrite.Input{Users: []*userwrite.User{u}}
	if _, err := s.dao.Operate(ctx,
		datly.WithPath(contract.NewPath("PATCH", userwrite.PathURI)),
		datly.WithInput(in),
		datly.WithOutput(out)); err != nil {
		return "", err
	}
	return id, nil
}

// UpdateHashIPByID sets users.hash_ip in a server-controlled way.
func (s *Service) UpdateHashIPByID(ctx context.Context, id, hash string) error {
	if strings.TrimSpace(id) == "" {
		return fmt.Errorf("id required")
	}
	u := &userwrite.User{}
	u.SetId(id)
	if strings.TrimSpace(hash) != "" {
		u.SetHashIP(hash)
	}
	u.SetUpdatedAt(time.Now().UTC())
	in := &userwrite.Input{Users: []*userwrite.User{u}}
	out := &userwrite.Output{}
	_, err := s.dao.Operate(ctx,
		datly.WithPath(contract.NewPath("PATCH", userwrite.PathURI)),
		datly.WithInput(in),
		datly.WithOutput(out))
	return err
}

// UpdatePreferencesByUsername updates default agent/model and basic fields for a user identified by username.
func (s *Service) UpdatePreferencesByUsername(ctx context.Context, username string, displayName, timezone, defaultAgentRef, defaultModelRef, defaultEmbedderRef *string) error {
	if strings.TrimSpace(username) == "" {
		return fmt.Errorf("username required")
	}
	v, err := s.FindByUsername(ctx, username)
	if err != nil {
		return err
	}
	if v == nil {
		return fmt.Errorf("user not found")
	}
	u := &userwrite.User{}
	u.SetId(v.Id)
	if displayName != nil {
		u.SetDisplayName(*displayName)
	}
	if timezone != nil && strings.TrimSpace(*timezone) != "" {
		u.SetTimezone(*timezone)
	}
	if defaultAgentRef != nil {
		u.SetDefaultAgentRef(*defaultAgentRef)
	}
	if defaultModelRef != nil {
		u.SetDefaultModelRef(*defaultModelRef)
	}
	if defaultEmbedderRef != nil {
		u.SetDefaultEmbedderRef(*defaultEmbedderRef)
	}
	now := time.Now().UTC()
	u.SetUpdatedAt(now)
	in := &userwrite.Input{Users: []*userwrite.User{u}}
	out := &userwrite.Output{}
	_, err = s.dao.Operate(ctx,
		datly.WithPath(contract.NewPath("PATCH", userwrite.PathURI)),
		datly.WithInput(in),
		datly.WithOutput(out))
	return err
}

// UpdateAgentSettingsByUsername merges per-agent settings into users.settings JSON for the given username.
// The payload shape is expected as: { agentId: { tools: [], toolCallExposure: "turn|conversation", autoSummarize: bool, chainsEnabled: bool }, ... }
func (s *Service) UpdateAgentSettingsByUsername(ctx context.Context, username string, agentPrefs map[string]map[string]interface{}) error {
	if strings.TrimSpace(username) == "" {
		return fmt.Errorf("username required")
	}
	if len(agentPrefs) == 0 {
		return nil
	}
	v, err := s.FindByUsername(ctx, username)
	if err != nil {
		return err
	}
	if v == nil {
		return fmt.Errorf("user not found")
	}
	// Parse existing settings JSON when present
	var settings map[string]interface{}
	if v.Settings != nil && strings.TrimSpace(*v.Settings) != "" {
		_ = json.Unmarshal([]byte(*v.Settings), &settings)
	}
	if settings == nil {
		settings = map[string]interface{}{}
	}
	// Merge into settings.agentPrefs
	apRaw, _ := settings["agentPrefs"].(map[string]interface{})
	if apRaw == nil {
		apRaw = map[string]interface{}{}
	}
	for agentID, prefs := range agentPrefs {
		if strings.TrimSpace(agentID) == "" || prefs == nil {
			continue
		}
		apRaw[agentID] = prefs
	}
	settings["agentPrefs"] = apRaw
	// Persist
	b, err := json.Marshal(settings)
	if err != nil {
		return err
	}
	u := &userwrite.User{}
	u.SetId(v.Id)
	u.SetSettings(string(b))
	now := time.Now().UTC()
	u.SetUpdatedAt(now)
	in := &userwrite.Input{Users: []*userwrite.User{u}}
	out := &userwrite.Output{}
	_, err = s.dao.Operate(ctx,
		datly.WithPath(contract.NewPath("PATCH", userwrite.PathURI)),
		datly.WithInput(in),
		datly.WithOutput(out))
	return err
}
