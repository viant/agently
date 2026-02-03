package scheduler

import (
	"context"
	"fmt"
	"log"
	"os"
	"reflect"
	"strings"
	"time"

	"github.com/google/uuid"
	chatcli "github.com/viant/agently/client/chat"
	apiconv "github.com/viant/agently/client/conversation"
	schapi "github.com/viant/agently/client/scheduler"
	schcli "github.com/viant/agently/client/scheduler/store"
	authctx "github.com/viant/agently/internal/auth"
	"github.com/viant/agently/internal/codec"
	chatimpl "github.com/viant/agently/internal/service/chat"
	convintern "github.com/viant/agently/internal/service/conversation"
	schedstore "github.com/viant/agently/internal/service/scheduler/store"
	"github.com/viant/scy"
	scyauth "github.com/viant/scy/auth"
	"github.com/viant/scy/auth/authorizer"
	"github.com/viant/scy/cred"
)

// CRUD operations for schedules and runs are implemented in
// internal/service/schedule; this package focuses on run orchestration.

// Service implements client/scheduler.Client by delegating persistence to the
// schedule store and optionally enriching runs with a created conversation via chat
// when missing.
type Service struct {
	sch  schcli.Client
	chat chatcli.Client
	conv apiconv.Client

	leaseOwner string
	leaseTTL   time.Duration
	authCfg    *authctx.Config
}

// New constructs a scheduler service requiring both a schedule store client and a chat client.
func New(sch schcli.Client, chat chatcli.Client) (schapi.Client, error) {
	if sch == nil {
		return nil, fmt.Errorf("schedule client is required")
	}
	if chat == nil {
		return nil, fmt.Errorf("chat client is required")
	}
	return &Service{sch: sch, chat: chat}, nil
}

// AttachAuthConfig sets auth configuration for OOB schedule runs.
func (s *Service) AttachAuthConfig(cfg *authctx.Config) { s.authCfg = cfg }

// NewFromEnv constructs a scheduler client using env-backed datly and wires
// an internal chat service instance for optional orchestration.
func NewFromEnv(ctx context.Context) (schapi.Client, error) {
	dao, err := convintern.NewDatly(ctx)
	if err != nil {
		return nil, err
	}
	sch, err := schedstore.New(ctx, dao)
	if err != nil {
		return nil, err
	}
	// Reuse the same dao and conversation client across chat + scheduler
	conv, err := convintern.New(ctx, dao)
	if err != nil {
		return nil, err
	}
	chat := chatimpl.NewServiceWithClient(conv, dao)
	svc, err := New(sch, chat)
	if err != nil {
		return nil, err
	}
	if s, ok := svc.(*Service); ok {
		s.conv = conv
	}
	return svc, nil
}

// No setters: dependencies are provided via constructor to ensure invariants.

// ListSchedules returns all schedules.
func (s *Service) ListSchedules(ctx context.Context, session ...codec.SessionOption) ([]*schapi.Schedule, error) {
	return s.sch.GetSchedules(ctx, session...)
}

// GetSchedule returns a schedule by id or nil if not found.
func (s *Service) GetSchedule(ctx context.Context, id string, session ...codec.SessionOption) (*schapi.Schedule, error) {
	return s.sch.GetSchedule(ctx, id, session...)
}

// Schedule creates or updates a schedule (generic upsert via Has flags).
func (s *Service) Schedule(ctx context.Context, in *schapi.MutableSchedule) error {
	return s.sch.PatchSchedule(ctx, in)
}

// Run creates or updates a run, optionally creating a conversation using chat
// when ConversationId is missing.
func (s *Service) Run(ctx context.Context, in *schapi.MutableRun) error {
	if s == nil || s.sch == nil {
		return fmt.Errorf("scheduler service not initialized")
	}
	s.ensureLeaseConfig()
	if in == nil {
		return fmt.Errorf("run is required")
	}
	if strings.TrimSpace(in.Id) == "" {
		in.SetId(uuid.NewString())
	}
	schID := strings.TrimSpace(in.ScheduleId)
	if schID == "" {
		return fmt.Errorf("scheduleId is required")
	}

	row, err := s.sch.GetSchedule(ctx, schID)
	if err != nil {
		return err
	}
	if row == nil {
		return fmt.Errorf("schedule not found")
	}

	// Optional: OOB auth for scheduled runs when user credentials are provided.
	if row.UserCredURL != nil && strings.TrimSpace(*row.UserCredURL) != "" {
		var err error
		ctx, err = s.applyUserCred(ctx, strings.TrimSpace(*row.UserCredURL))
		if err != nil {
			return err
		}
	}

	// Establish an execution user identity for background runs so that:
	//   - chat.CreateConversation writes conversation.created_by_user_id
	//   - private scheduled conversations remain visible to the schedule owner
	runCtx := ctx
	if owner := strPtrValue(row.CreatedByUserId); owner != "" {
		runCtx = authctx.WithUserInfo(runCtx, &authctx.UserInfo{Subject: owner})
	} else if strings.TrimSpace(authctx.EffectiveUserID(runCtx)) == "" {
		runCtx = authctx.EnsureUser(runCtx, s.authCfg)
	}
	// Best-effort backfill for legacy schedules created before created_by_user_id existed.
	if strings.TrimSpace(strPtrValue(row.CreatedByUserId)) == "" {
		if uid := strings.TrimSpace(authctx.EffectiveUserID(runCtx)); uid != "" {
			mut := &schapi.MutableSchedule{}
			mut.SetId(schID)
			mut.SetCreatedByUserID(uid)
			_ = s.sch.PatchSchedule(runCtx, mut)
		}
	}

	// Ensure required fields are set before persisting (status + conversationKind are required).
	if strings.TrimSpace(in.Status) == "" {
		in.SetStatus("pending")
	}
	in.SetConversationKind("scheduled")

	// Persist initial state before any side-effects (conversation creation / LLM calls).
	// This prevents duplicate/failed inserts from generating orphan conversations.
	if err := s.sch.PatchRun(ctx, in); err != nil {
		return err
	}

	// Always create a dedicated conversation for this run to avoid
	// polluting/consuming prior agent context across runs.
	{
		req := chatcli.CreateConversationRequest{Title: row.Name}
		if v := strings.TrimSpace(row.Visibility); v != "" {
			req.Visibility = v
		}
		if v := strings.TrimSpace(row.AgentRef); v != "" {
			req.Agent = v
		}
		if row.ModelOverride != nil && strings.TrimSpace(*row.ModelOverride) != "" {
			req.Model = strings.TrimSpace(*row.ModelOverride)
		}
		resp, err := s.chat.CreateConversation(runCtx, req)
		if err != nil {
			return err
		}
		if resp == nil || strings.TrimSpace(resp.ID) == "" {
			return fmt.Errorf("failed to create conversation")
		}
		in.SetConversationId(resp.ID)
		// Best-effort: annotate conversation with schedule linkage
		// Try to obtain a conversation client if not provided
		if s.conv == nil {
			type chatConv interface{ ConversationClient() apiconv.Client }
			if c, ok := s.chat.(chatConv); ok {
				s.conv = c.ConversationClient()
			}
		}
		if s.conv != nil {
			c := apiconv.NewConversation()
			c.SetId(resp.ID)
			if uid := strings.TrimSpace(authctx.EffectiveUserID(runCtx)); uid != "" {
				c.SetCreatedByUserID(uid)
			}
			c.SetScheduled(1)
			c.SetScheduleId(schID)
			c.SetScheduleRunId(in.Id)
			c.SetScheduleKind(strings.TrimSpace(row.ScheduleType))
			if tz := strings.TrimSpace(row.Timezone); tz != "" {
				c.SetScheduleTimezone(tz)
			}
			if row.CronExpr != nil && strings.TrimSpace(*row.CronExpr) != "" {
				c.SetScheduleCronExpr(strings.TrimSpace(*row.CronExpr))
			}
			_ = s.conv.PatchConversations(runCtx, c)
		}
	}

	// Post task prompt if defined
	var taskContent string
	if row.TaskPrompt != nil && strings.TrimSpace(*row.TaskPrompt) != "" {
		taskContent = strings.TrimSpace(*row.TaskPrompt)
	} else if row.TaskPromptUri != nil && strings.TrimSpace(*row.TaskPromptUri) != "" {
		// Let chat layer resolve URI-based prompt if supported by the agent
		taskContent = strings.TrimSpace(*row.TaskPromptUri)
	}
	if taskContent != "" && in.ConversationId != nil && strings.TrimSpace(*in.ConversationId) != "" {
		_, err = s.chat.Post(runCtx, *in.ConversationId, chatcli.PostRequest{Content: taskContent, Agent: row.AgentRef, Model: strPtrValue(row.ModelOverride)})
		if err != nil {
			return err
		}
		// Mark run as running when task is posted
		in.SetStatus("running")
		in.SetStartedAt(time.Now().UTC())
		log.Printf("scheduler: run started schedule_id=%q run_id=%q conversation_id=%q agent=%q model=%q", schID, strings.TrimSpace(in.Id), strings.TrimSpace(*in.ConversationId), strings.TrimSpace(row.AgentRef), strPtrValue(row.ModelOverride))
	}
	// Persist updated state (conversation linkage, running timestamps, etc.)
	if err := s.sch.PatchRun(ctx, in); err != nil {
		return err
	}
	// Best-effort: claim the run lease so other scheduler instances can detect liveness
	// via periodic heartbeats from watchRunCompletion.
	if strings.TrimSpace(s.leaseOwner) != "" {
		_, _ = s.sch.TryClaimRun(ctx, strings.TrimSpace(in.Id), s.leaseOwner, time.Now().UTC().Add(s.leaseTTL))
	}
	// Fire-and-forget watcher to mark completion based on conversation progress
	if in.ConversationId != nil && strings.TrimSpace(*in.ConversationId) != "" {
		aCtx := context.WithoutCancel(runCtx)
		go s.watchRunCompletion(aCtx, strings.TrimSpace(in.Id), schID, strings.TrimSpace(*in.ConversationId), row.TimeoutSeconds)
	}
	return nil
}

func (s *Service) applyUserCred(ctx context.Context, credRef string) (context.Context, error) {
	if s == nil {
		return ctx, fmt.Errorf("scheduler service not initialized")
	}
	if s.authCfg == nil || s.authCfg.OAuth == nil || s.authCfg.OAuth.Client == nil {
		return ctx, fmt.Errorf("schedule user_cred_url requires auth.oauth configuration")
	}
	mode := strings.ToLower(strings.TrimSpace(s.authCfg.OAuth.Mode))
	if mode != "bff" {
		return ctx, fmt.Errorf("schedule user_cred_url requires auth.oauth.mode=bff")
	}
	cfgURL := strings.TrimSpace(s.authCfg.OAuth.Client.ConfigURL)
	if cfgURL == "" {
		return ctx, fmt.Errorf("schedule user_cred_url requires auth.oauth.client.configURL")
	}

	cmd := &authorizer.Command{
		AuthFlow:   "OOB",
		UsePKCE:    true,
		SecretsURL: credRef,
		OAuthConfig: authorizer.OAuthConfig{
			ConfigURL: cfgURL,
		},
	}
	if scopes := s.authCfg.OAuth.Client.Scopes; len(scopes) > 0 {
		cmd.Scopes = scopes
	} else {
		cmd.Scopes = []string{"openid"}
	}
	// Detach from request cancellation to allow OOB auth to complete; still bound by a short timeout.
	authCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 1*time.Minute)
	defer cancel()
	start := time.Now()
	log.Printf("scheduler: user_cred_url auth start configURL=%q secretsURL=%q scopes=%v", cfgURL, credRef, cmd.Scopes)

	scySvc := scy.New()
	{
		cfgStart := time.Now()
		log.Printf("scheduler: user_cred_url config load start url=%q", cfgURL)
		resource := scy.EncodedResource(cfgURL).Decode(authCtx, reflect.TypeOf(cred.Oauth2Config{}))
		resource.TimeoutMs = int((1 * time.Minute).Milliseconds())
		secret, err := scySvc.Load(authCtx, resource)
		if err != nil {
			log.Printf("scheduler: user_cred_url config load error url=%q duration=%s err=%v", cfgURL, time.Since(cfgStart), err)
			return ctx, fmt.Errorf("schedule user_cred oauth config load failed: %w", err)
		}
		cfg, ok := secret.Target.(*cred.Oauth2Config)
		if !ok {
			log.Printf("scheduler: user_cred_url config load cast failed url=%q target=%T duration=%s", cfgURL, secret.Target, time.Since(cfgStart))
			return ctx, fmt.Errorf("schedule user_cred oauth config cast failed: %T", secret.Target)
		}
		log.Printf("scheduler: user_cred_url config load ok url=%q duration=%s client_id=%q auth_url=%q token_url=%q redirect_url=%q",
			cfgURL, time.Since(cfgStart), cfg.Config.ClientID, cfg.Config.Endpoint.AuthURL, cfg.Config.Endpoint.TokenURL, cfg.Config.RedirectURL)
		cmd.Config = &cfg.Config
		cmd.ConfigURL = ""
	}

	{
		secStart := time.Now()
		log.Printf("scheduler: user_cred_url secret load start url=%q", credRef)
		resource := scy.EncodedResource(credRef).Decode(authCtx, reflect.TypeOf(cred.Basic{}))
		resource.TimeoutMs = int((1 * time.Minute).Milliseconds())
		secret, err := scySvc.Load(authCtx, resource)
		if err != nil {
			log.Printf("scheduler: user_cred_url secret load error url=%q duration=%s err=%v", credRef, time.Since(secStart), err)
			return ctx, fmt.Errorf("schedule user_cred secret load failed: %w", err)
		}
		basic, ok := secret.Target.(*cred.Basic)
		if !ok {
			log.Printf("scheduler: user_cred_url secret load cast failed url=%q target=%T duration=%s", credRef, secret.Target, time.Since(secStart))
			return ctx, fmt.Errorf("schedule user_cred secret cast failed: %T", secret.Target)
		}
		cmd.Secrets = map[string]string{
			"username": basic.Username,
			"password": basic.Password,
		}
		cmd.SecretsURL = ""
		log.Printf("scheduler: user_cred_url secret load ok url=%q duration=%s", credRef, time.Since(secStart))
	}

	tok, err := authorizer.New().Authorize(authCtx, cmd)
	log.Printf("scheduler: user_cred_url auth done configURL=%q secretsURL=%q duration=%s err=%v", cfgURL, credRef, time.Since(start), err)
	if err != nil {
		return ctx, fmt.Errorf("schedule user_cred authorize failed: %w", err)
	}
	if tok == nil {
		return ctx, fmt.Errorf("schedule user_cred authorize returned empty token")
	}

	st := &scyauth.Token{Token: *tok}
	st.PopulateIDToken()
	ctx = authctx.WithTokens(ctx, st)
	if strings.TrimSpace(st.AccessToken) != "" {
		ctx = authctx.WithBearer(ctx, st.AccessToken)
	}
	if strings.TrimSpace(st.IDToken) != "" {
		ctx = authctx.WithIDToken(ctx, st.IDToken)
		if ui, _ := authctx.DecodeUserInfo(st.IDToken); ui != nil {
			ctx = authctx.WithUserInfo(ctx, ui)
		}
	}
	return ctx, nil
}

func strPtrValue(p *string) string {
	if p == nil {
		return ""
	}
	return strings.TrimSpace(*p)
}

// GetRuns lists runs for a schedule, optionally filtered by since id.
func (s *Service) GetRuns(ctx context.Context, scheduleID, since string, session ...codec.SessionOption) ([]*schapi.Run, error) {
	return s.sch.GetRuns(ctx, scheduleID, since, session...)
}

// RunDue lists schedules, checks if due, and triggers runs while avoiding duplicates.
// Returns number of runs started.
func (s *Service) RunDue(ctx context.Context) (int, error) {
	if s == nil || s.sch == nil || s.chat == nil {
		return 0, fmt.Errorf("scheduler service not initialized")
	}
	//ctx = authctx.WithInternalAccess(ctx) TODO delete or accept
	s.ensureLeaseConfig()

	rows, err := s.sch.GetSchedules(ctx)
	if err != nil {
		return 0, err
	}

	started := 0
	for _, sc := range rows {
		fmt.Printf("RunDue checking schedule: %v\n", sc.Id)
		now := time.Now().UTC()

		if sc == nil || !sc.Enabled {
			fmt.Printf("00 RunDue checking schedule: %v\n", sc.Id)
			continue
		}

		// Run list/read APIs apply privacy filters based on conversation visibility.
		// For private scheduled conversations, background scheduler ticks must operate
		// under the schedule owner identity; otherwise active runs become invisible and
		// the scheduler can start overlapping runs for the same schedule.
		scheduleCtx := ctx
		if owner := strPtrValue(sc.CreatedByUserId); owner != "" {
			scheduleCtx = authctx.WithUserInfo(scheduleCtx, &authctx.UserInfo{Subject: owner})
		} else {
			scheduleCtx = authctx.EnsureUser(scheduleCtx, s.authCfg)
		}

		due := false
		// Respect optional schedule time window.
		if sc.StartAt != nil && now.Before(sc.StartAt.UTC()) {
			fmt.Printf("01 RunDue checking schedule: %v\n", sc.Id)
			continue
		}
		if sc.EndAt != nil && !now.Before(sc.EndAt.UTC()) {
			fmt.Printf("02 RunDue checking schedule: %v\n", sc.Id)
			continue
		}
		if strings.EqualFold(strings.TrimSpace(sc.ScheduleType), "cron") && sc.CronExpr != nil && strings.TrimSpace(*sc.CronExpr) != "" {
			fmt.Printf("03A RunDue checking schedule: %v\n", sc.Id)
			loc, _ := time.LoadLocation(strings.TrimSpace(sc.Timezone))
			if loc == nil {
				loc = time.UTC
			}
			spec, err := parseCron(strings.TrimSpace(*sc.CronExpr))
			if err != nil {
				return started, fmt.Errorf("invalid cron expr for schedule %s: %w", sc.Id, err)
			}
			base := now.In(loc)
			if sc.LastRunAt != nil {
				base = sc.LastRunAt.In(loc)
			} else if !sc.CreatedAt.IsZero() {
				// When a cron schedule has never run and next_run_at is NULL, using `now`
				// makes it perpetually "not due" (because next is always in the future).
				// Use a stable reference time so the first run can be triggered.
				base = sc.CreatedAt.In(loc)
			}
			next := cronNext(spec, base).In(time.UTC)
			if sc.NextRunAt != nil {
				next = sc.NextRunAt.UTC()
			}
			if !now.Before(next) {
				due = true
			}
			fmt.Printf("03B RunDue checking schedule: %v\n", sc.Id)
		} else if sc.NextRunAt != nil && !now.Before(sc.NextRunAt.UTC()) {
			fmt.Printf("04 RunDue checking schedule: %v\n", sc.Id)
			due = true
		} else if sc.IntervalSeconds != nil {
			fmt.Printf("05 RunDue checking schedule: %v\n", sc.Id)
			base := sc.CreatedAt.UTC()
			if sc.LastRunAt != nil {
				base = sc.LastRunAt.UTC()
			}
			if !now.Before(base.Add(time.Duration(*sc.IntervalSeconds) * time.Second)) {
				due = true
			}
		}
		if !due {
			fmt.Printf("06 RunDue checking schedule: %v\n", sc.Id)
			continue
		}

		fmt.Printf("07 RunDue checking schedule: %v\n", sc.Id)
		leaseUntil := now.Add(s.leaseTTL)
		claimed, err := s.sch.TryClaimSchedule(scheduleCtx, sc.Id, s.leaseOwner, leaseUntil)
		if err != nil {
			fmt.Printf("08 RunDue checking schedule: %v\n", sc.Id)
			return started, err
		}
		if !claimed {
			fmt.Printf("09 RunDue checking schedule: %v\n", sc.Id)
			continue
		}

		releaseLease := func() {
			_, _ = s.sch.ReleaseScheduleLease(context.Background(), sc.Id, s.leaseOwner)
		}

		err = func() error {
			defer releaseLease()
			fmt.Printf("10 RunDue checking schedule: %v\n", sc.Id)
			runs, err := s.sch.GetRuns(scheduleCtx, sc.Id, "")
			if err != nil {
				return err
			}
			scheduledFor := now
			if sc.NextRunAt != nil && !sc.NextRunAt.IsZero() {
				scheduledFor = sc.NextRunAt.UTC()
			}
			fmt.Printf("11 RunDue checking schedule: %v\n", sc.Id)
			var pendingRun *schapi.Run
			for _, r := range runs {
				fmt.Printf("12 RunDue checking schedule: %v\n", sc.Id)
				if r == nil {
					fmt.Printf("13 RunDue checking schedule: %v\n", sc.Id)
					continue
				}
				// If a run already exists for the current scheduled slot (even completed),
				// do not create another run for the same (schedule_id, scheduled_for) slot.
				// This can happen after a crash where the run completed but the schedule row
				// didn't advance next_run_at.
				if sc.NextRunAt != nil && !sc.NextRunAt.IsZero() &&
					r.ScheduledFor != nil && r.ScheduledFor.UTC().Equal(scheduledFor) &&
					r.CompletedAt != nil {
					fmt.Printf("14 RunDue checking schedule: %v\n", sc.Id)

					mut := &schapi.MutableSchedule{}
					mut.SetId(sc.Id)
					mut.SetLastRunAt(now)
					if strings.EqualFold(strings.TrimSpace(sc.ScheduleType), "cron") && sc.CronExpr != nil && strings.TrimSpace(*sc.CronExpr) != "" {
						fmt.Printf("15 RunDue checking schedule: %v\n", sc.Id)
						loc, _ := time.LoadLocation(strings.TrimSpace(sc.Timezone))
						if loc == nil {
							loc = time.UTC
						}
						spec, err := parseCron(strings.TrimSpace(*sc.CronExpr))
						if err == nil {
							mut.SetNextRunAt(cronNext(spec, now.In(loc)).UTC())
						}
					} else if sc.IntervalSeconds != nil {
						fmt.Printf("16 RunDue checking schedule: %v\n", sc.Id)
						mut.SetNextRunAt(now.Add(time.Duration(*sc.IntervalSeconds) * time.Second))
					} else if strings.EqualFold(strings.TrimSpace(sc.ScheduleType), "adhoc") {
						fmt.Printf("17 RunDue checking schedule: %v\n", sc.Id)
						mut.NextRunAt = nil
						if mut.Has != nil {
							mut.Has.NextRunAt = true
						}
					}
					if err := s.sch.PatchSchedule(scheduleCtx, mut); err != nil {
						fmt.Printf("18 RunDue checking schedule: %v error %v\n", sc.Id, err)
						return err
					}
					fmt.Printf("19 RunDue checking schedule: %v error %v\n", sc.Id, err)
					return nil
				}

				if r.CompletedAt != nil {
					fmt.Printf("20 RunDue checking schedule: %v error %v\n", sc.Id, err)
					continue
				}
				st := strings.ToLower(strings.TrimSpace(r.Status))
				switch st {
				case "running", "prechecking":
					fmt.Printf("21 RunDue checking schedule: %v error %v\n", sc.Id, err)
					// If the run is stale (e.g. scheduler crashed and no watcher is running),
					// mark it failed so it doesn't block future runs.
					runStart := r.CreatedAt.UTC()
					if r.StartedAt != nil && !r.StartedAt.IsZero() {
						runStart = r.StartedAt.UTC()
					}
					timeout := watchTimeout
					if sc.TimeoutSeconds > 0 {
						timeout = time.Duration(sc.TimeoutSeconds) * time.Second
					}
					if now.After(runStart.Add(timeout).Add(15 * time.Second)) {
						fmt.Printf("22 RunDue checking schedule: %v error %v\n", sc.Id, err)
						claimCtx, claimCancel := context.WithTimeout(scheduleCtx, callTimeout)
						claimed, claimErr := s.sch.TryClaimRun(claimCtx, strings.TrimSpace(r.Id), s.leaseOwner, now.Add(s.leaseTTL))
						claimCancel()
						// If another instance owns the run lease, let it finalize.
						if claimErr == nil && !claimed {
							return nil
						}

						convID := ""
						if r.ConversationId != nil {
							convID = strings.TrimSpace(*r.ConversationId)
						}
						if convID != "" {
							_ = s.chat.Cancel(convID)
						}

						upd := &schapi.MutableRun{}
						upd.SetId(strings.TrimSpace(r.Id))
						upd.SetScheduleId(sc.Id)
						upd.SetStatus("failed")
						upd.SetCompletedAt(now)
						upd.SetErrorMessage(fmt.Sprintf("stale run detected: status=%q started_at=%v timeout=%v", strings.TrimSpace(r.Status), runStart, timeout))

						callCtx, callCancel := context.WithTimeout(scheduleCtx, callTimeout)
						if err := s.sch.PatchRun(callCtx, upd); err != nil {
							callCancel()
							return err
						}
						callCancel()

						if claimErr == nil && claimed {
							_, _ = s.sch.ReleaseRunLease(scheduleCtx, strings.TrimSpace(r.Id), s.leaseOwner)
						}
						fmt.Printf("23 RunDue checking schedule: %v error %v\n", sc.Id, err)
						// If this stale run belongs to the current scheduled slot, treat that slot
						// as processed by advancing schedule.next_run_at (do not create a new run
						// for the same scheduled_for).
						if sc.NextRunAt != nil && !sc.NextRunAt.IsZero() &&
							r.ScheduledFor != nil && r.ScheduledFor.UTC().Equal(scheduledFor) {
							fmt.Printf("24 RunDue checking schedule: %v error %v\n", sc.Id, err)
							mut := &schapi.MutableSchedule{}
							mut.SetId(sc.Id)
							mut.SetLastRunAt(now)
							if strings.EqualFold(strings.TrimSpace(sc.ScheduleType), "cron") && sc.CronExpr != nil && strings.TrimSpace(*sc.CronExpr) != "" {
								loc, _ := time.LoadLocation(strings.TrimSpace(sc.Timezone))
								if loc == nil {
									loc = time.UTC
								}
								spec, err := parseCron(strings.TrimSpace(*sc.CronExpr))
								if err == nil {
									mut.SetNextRunAt(cronNext(spec, now.In(loc)).UTC())
								}
							} else if sc.IntervalSeconds != nil {
								mut.SetNextRunAt(now.Add(time.Duration(*sc.IntervalSeconds) * time.Second))
							} else if strings.EqualFold(strings.TrimSpace(sc.ScheduleType), "adhoc") {
								mut.NextRunAt = nil
								if mut.Has != nil {
									mut.Has.NextRunAt = true
								}
							}
							if err := s.sch.PatchSchedule(scheduleCtx, mut); err != nil {
								fmt.Printf("25 RunDue checking schedule: %v error %v\n", sc.Id, err)
								return err
							}
							fmt.Printf("26 RunDue checking schedule: %v error %v\n", sc.Id, err)
							return nil
						}
						// Stale run was for an older slot; continue so a new run can be started.
						fmt.Printf("27 RunDue checking schedule: %v error %v\n", sc.Id, err)
						continue
					}

					fmt.Printf("28 RunDue checking schedule: %v error %v\n", sc.Id, err)

					// Another instance is processing this schedule.
					// For ad-hoc schedules, clear next_run_at so they don't remain due
					// while a run is already in progress (e.g. when started via run-now).
					if strings.EqualFold(strings.TrimSpace(sc.ScheduleType), "adhoc") && sc.NextRunAt != nil && !sc.NextRunAt.IsZero() {
						mut := &schapi.MutableSchedule{}
						mut.SetId(sc.Id)
						mut.NextRunAt = nil
						if mut.Has != nil {
							mut.Has.NextRunAt = true
						}
						_ = s.sch.PatchSchedule(scheduleCtx, mut)
					}

					fmt.Printf("29 RunDue checking schedule: %v error %v\n", sc.Id, err)

					// TODO can cause panic in some edge cases; revisit later
					// emergency lease extension logic could go here
					//if st == "running" {
					//	started := r.StartedAt
					//	timeoutSec := time.Duration(sc.TimeoutSeconds)
					//	if timeoutSec <= 0 {
					//		timeoutSec = watchTimeout
					//	}
					//	maxTime := started.Add(timeoutSec * time.Second)
					//	maxTime = maxTime.Add(15 * time.Second)
					//	if time.Now().After(maxTime) {
					//		aCtx := context.WithoutCancel(ctx)
					//		convId := ""
					//		if r.ConversationId != nil {
					//			convId = strings.TrimSpace(*r.ConversationId)
					//		}
					//		s.finalizeDeadline(aCtx, r.Id, sc.Id, convId, callTimeout, fmt.Errorf("emergency termination of an abandoned schedule run"), time.Duration(sc.TimeoutSeconds)*time.Second)
					//	}
					//}

					fmt.Printf("30 RunDue checking schedule: %v error %v\n", sc.Id, err)
					return nil
				case "pending":
					fmt.Printf("31 RunDue checking schedule: %v error %v\n", sc.Id, err)
					// Prefer a pending run that matches the current scheduled slot.
					if pendingRun == nil {
						pendingRun = r
					}
					if r.ScheduledFor != nil && r.ScheduledFor.UTC().Equal(scheduledFor) {
						pendingRun = r
					}
				}
			}

			fmt.Printf("40 RunDue checking schedule: %v\n", sc.Id)
			run := &schapi.MutableRun{}
			if pendingRun != nil {
				run.SetId(strings.TrimSpace(pendingRun.Id))
			} else {
				run.SetId(uuid.NewString())
			}
			run.SetScheduleId(sc.Id)
			run.SetStatus("pending")
			run.SetScheduledFor(scheduledFor)
			fmt.Printf("41 RunDue checking schedule: %v\n", sc.Id)
			if err := s.Run(scheduleCtx, run); err != nil {
				return err
			}

			// Update schedule last_run_at and next_run_at
			mut := &schapi.MutableSchedule{}
			mut.SetId(sc.Id)
			mut.SetLastRunAt(now)
			if strings.EqualFold(strings.TrimSpace(sc.ScheduleType), "cron") && sc.CronExpr != nil && strings.TrimSpace(*sc.CronExpr) != "" {
				loc, _ := time.LoadLocation(strings.TrimSpace(sc.Timezone))
				if loc == nil {
					loc = time.UTC
				}
				spec, err := parseCron(strings.TrimSpace(*sc.CronExpr))
				if err == nil {
					mut.SetNextRunAt(cronNext(spec, now.In(loc)).UTC())
				}
			} else if sc.IntervalSeconds != nil {
				mut.SetNextRunAt(now.Add(time.Duration(*sc.IntervalSeconds) * time.Second))
			} else if strings.EqualFold(strings.TrimSpace(sc.ScheduleType), "adhoc") {
				// Ad-hoc schedules are one-shot: once triggered, clear next_run_at so
				// the runner doesn't keep treating it as perpetually due.
				mut.NextRunAt = nil
				if mut.Has != nil {
					mut.Has.NextRunAt = true
				}
			}
			if err := s.sch.PatchSchedule(scheduleCtx, mut); err != nil {
				return err
			}

			started++
			return nil
		}()
		if err != nil {
			return started, err
		}
	}

	return started, nil
}

func (s *Service) ensureLeaseConfig() {
	if s == nil {
		return
	}
	if s.leaseTTL <= 0 {
		s.leaseTTL = 60 * time.Second
		if v := strings.TrimSpace(os.Getenv("AGENTLY_SCHEDULER_LEASE_TTL")); v != "" {
			if d, err := time.ParseDuration(v); err == nil && d > 0 {
				s.leaseTTL = d
			}
		}
	}

	if strings.TrimSpace(s.leaseOwner) != "" {
		return
	}
	if v := strings.TrimSpace(os.Getenv("AGENTLY_SCHEDULER_LEASE_OWNER")); v != "" {
		s.leaseOwner = v
		return
	}
	host, _ := os.Hostname()
	host = strings.TrimSpace(host)
	if host == "" {
		host = "unknown-host"
	}
	s.leaseOwner = fmt.Sprintf("%s:%d:%s", host, os.Getpid(), uuid.NewString())
}

// --------------------
// Minimal cron support
// --------------------

type cronSpec struct {
	min, hour, dom, mon, dow map[int]bool
}

func parseCron(expr string) (*cronSpec, error) {
	parts := strings.Fields(expr)
	if len(parts) < 5 {
		return nil, fmt.Errorf("invalid cron expr: %s", expr)
	}
	min, err := parseCronField(parts[0], 0, 59)
	if err != nil {
		return nil, fmt.Errorf("minute: %w", err)
	}
	hour, err := parseCronField(parts[1], 0, 23)
	if err != nil {
		return nil, fmt.Errorf("hour: %w", err)
	}
	dom, err := parseCronField(parts[2], 1, 31)
	if err != nil {
		return nil, fmt.Errorf("dom: %w", err)
	}
	mon, err := parseCronField(parts[3], 1, 12)
	if err != nil {
		return nil, fmt.Errorf("month: %w", err)
	}
	dow, err := parseCronField(parts[4], 0, 6) // Sunday=0
	if err != nil {
		return nil, fmt.Errorf("dow: %w", err)
	}
	return &cronSpec{min: min, hour: hour, dom: dom, mon: mon, dow: dow}, nil
}

func parseCronField(s string, min, max int) (map[int]bool, error) {
	s = strings.TrimSpace(s)
	set := map[int]bool{}
	add := func(v int) {
		if v >= min && v <= max {
			set[v] = true
		}
	}
	if s == "*" || s == "?" {
		for i := min; i <= max; i++ {
			set[i] = true
		}
		return set, nil
	}
	// handle lists
	for _, tok := range strings.Split(s, ",") {
		tok = strings.TrimSpace(tok)
		if tok == "" {
			continue
		}
		// step syntax a-b/n or */n
		step := 1
		if parts := strings.Split(tok, "/"); len(parts) == 2 {
			tok = parts[0]
			if v, err := atoiSafe(parts[1]); err == nil && v > 0 {
				step = v
			}
		}
		// range or single
		if tok == "*" {
			for i := min; i <= max; i += step {
				set[i] = true
			}
			continue
		}
		if strings.Contains(tok, "-") {
			rs := strings.SplitN(tok, "-", 2)
			a, err1 := atoiSafe(rs[0])
			b, err2 := atoiSafe(rs[1])
			if err1 != nil || err2 != nil {
				return nil, fmt.Errorf("invalid range: %s", tok)
			}
			if a > b {
				a, b = b, a
			}
			for i := a; i <= b; i += step {
				add(i)
			}
			continue
		}
		if v, err := atoiSafe(tok); err == nil {
			add(v)
			continue
		}
		return nil, fmt.Errorf("invalid token: %s", tok)
	}
	if len(set) == 0 {
		return nil, fmt.Errorf("empty set")
	}
	return set, nil
}

func atoiSafe(s string) (int, error) {
	var n int
	for _, r := range s {
		if r < '0' || r > '9' {
			return 0, fmt.Errorf("invalid int: %s", s)
		}
		n = n*10 + int(r-'0')
	}
	return n, nil
}

func cronMatch(spec *cronSpec, t time.Time) bool {
	if !spec.min[t.Minute()] {
		return false
	}
	if !spec.hour[t.Hour()] {
		return false
	}
	if !spec.dom[t.Day()] {
		return false
	}
	if !spec.mon[int(t.Month())] {
		return false
	}
	if !spec.dow[int(t.Weekday())] {
		return false
	}
	return true
}

func cronNext(spec *cronSpec, from time.Time) time.Time {
	// naive minute-by-minute scan with cap to avoid infinite loops
	t := from.Add(time.Minute)
	for i := 0; i < 525600; i++ { // up to 1 year
		if cronMatch(spec, t) {
			return t
		}
		t = t.Add(time.Minute)
	}
	return t
}
