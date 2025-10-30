package user

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"time"

	"github.com/google/uuid"
	userread "github.com/viant/agently/pkg/agently/user"
	userwrite "github.com/viant/agently/pkg/agently/user/write"
	"github.com/viant/datly"
	"github.com/viant/datly/repository/contract"
)

// Service provides minimal CRUD helpers on top of datly for users.
type Service struct{ dao *datly.Service }

// New registers user components on the provided datly service.
func New(ctx context.Context, dao *datly.Service) (*Service, error) {
	if dao == nil {
		return nil, fmt.Errorf("nil datly service")
	}
	if err := userread.DefineUserComponent(ctx, dao); err != nil {
		return nil, err
	}
	if _, err := userwrite.DefineComponent(ctx, dao); err != nil {
		return nil, err
	}
	return &Service{dao: dao}, nil
}

// FindByUsername returns a single user view or nil.
func (s *Service) FindByUsername(ctx context.Context, username string) (*userread.UserView, error) {
	in := &userread.UserInput{}
	// Prefer setter if generated; otherwise assign directly.
	// Using Has marker to enable username predicate when required by generator.
	in.Has = &userread.UserInputHas{}
	// Attempt to set Username; if the generated struct has a field, this will be set via JSON marshalling in Operate.
	// Some generators provide setters; if available, you may switch to in.SetUsername(username).
	// @regen: ensure UserInput includes Username predicate.
	// Using reflection-free assignment is avoided here; rely on generated struct.
	//nolint:staticcheck // field presence depends on generated code
	// in.SetUsername(username)
	// Best effort: cast to any and set field via type assertion when present
	type hasUsername interface{ SetUsername(string) }
	if setter, ok := any(in).(hasUsername); ok {
		setter.SetUsername(username)
	} else {
		// Fallback: try to set exported field if present
		// This is safe even if field does not exist (no-op via JSON encode rules when omitted)
		// Using struct literal update would require knowing the field, so we leave it as-is.
	}
	out := &userread.UserOutput{}
	if _, err := s.dao.Operate(ctx, datly.WithPath(contract.NewPath("GET", userread.UserPathURI)), datly.WithInput(in), datly.WithOutput(out)); err != nil {
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
