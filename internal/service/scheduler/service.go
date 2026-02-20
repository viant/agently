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
	runpkg "github.com/viant/agently/pkg/agently/scheduler/run"
	schedulepkg "github.com/viant/agently/pkg/agently/scheduler/schedule"
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
	debugf(
		"Run start schedule_id=%q run_id=%q status=%q scheduled_for=%s conversation_id=%q lease_owner=%q",
		schID,
		strings.TrimSpace(in.Id),
		strings.TrimSpace(in.Status),
		timePtrString(in.ScheduledFor),
		strPtrValue(in.ConversationId),
		strings.TrimSpace(s.leaseOwner),
	)

	row, err := s.sch.GetSchedule(ctx, schID)
	if err != nil {
		debugf("Run load schedule error schedule_id=%q run_id=%q err=%v", schID, strings.TrimSpace(in.Id), err)
		return err
	}
	if row == nil {
		debugf("Run schedule not found schedule_id=%q run_id=%q", schID, strings.TrimSpace(in.Id))
		return fmt.Errorf("schedule not found")
	}
	debugf(
		"Run schedule loaded schedule_id=%q name=%q type=%q enabled=%v tz=%q cron=%q interval_seconds=%v start_at=%s end_at=%s visibility=%q created_by_user_id=%q next_run_at=%s last_run_at=%s timeout_seconds=%d user_cred_url=%q task_prompt_len=%d task_prompt_uri=%q model_override=%q",
		strings.TrimSpace(row.Id),
		strings.TrimSpace(row.Name),
		strings.TrimSpace(row.ScheduleType),
		row.Enabled,
		strings.TrimSpace(row.Timezone),
		strPtrValue(row.CronExpr),
		row.IntervalSeconds,
		timePtrString(row.StartAt),
		timePtrString(row.EndAt),
		strings.TrimSpace(row.Visibility),
		strPtrValue(row.CreatedByUserId),
		timePtrString(row.NextRunAt),
		timePtrString(row.LastRunAt),
		row.TimeoutSeconds,
		redactCredRef(strPtrValue(row.UserCredURL)),
		len(strings.TrimSpace(strPtrValue(row.TaskPrompt))),
		strPtrValue(row.TaskPromptUri),
		strPtrValue(row.ModelOverride),
	)

	// Optional: OOB auth for scheduled runs when user credentials are provided.
	if row.UserCredURL != nil && strings.TrimSpace(*row.UserCredURL) != "" {
		debugf("Run applying user_cred_url schedule_id=%q run_id=%q user_cred_url=%q", schID, strings.TrimSpace(in.Id), redactCredRef(strPtrValue(row.UserCredURL)))
		var err error
		ctx, err = s.applyUserCred(ctx, strings.TrimSpace(*row.UserCredURL))
		if err != nil {
			debugf("Run apply user_cred_url failed schedule_id=%q run_id=%q err=%v", schID, strings.TrimSpace(in.Id), err)
			return err
		}
		debugf("Run applied user_cred_url schedule_id=%q run_id=%q", schID, strings.TrimSpace(in.Id))
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
	debugf("Run effective user schedule_id=%q run_id=%q effective_user_id=%q", schID, strings.TrimSpace(in.Id), strings.TrimSpace(authctx.EffectiveUserID(runCtx)))
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
		debugf("Run patch initial run error schedule_id=%q run_id=%q err=%v", schID, strings.TrimSpace(in.Id), err)
		return err
	}
	debugf("Run patched initial run schedule_id=%q run_id=%q status=%q", schID, strings.TrimSpace(in.Id), strings.TrimSpace(in.Status))

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
		debugf("Run create conversation schedule_id=%q run_id=%q visibility=%q agent=%q model=%q", schID, strings.TrimSpace(in.Id), strings.TrimSpace(req.Visibility), strings.TrimSpace(req.Agent), strings.TrimSpace(req.Model))
		resp, err := s.chat.CreateConversation(runCtx, req)
		if err != nil {
			debugf("Run create conversation error schedule_id=%q run_id=%q err=%v", schID, strings.TrimSpace(in.Id), err)
			return err
		}
		if resp == nil || strings.TrimSpace(resp.ID) == "" {
			debugf("Run create conversation returned empty id schedule_id=%q run_id=%q", schID, strings.TrimSpace(in.Id))
			return fmt.Errorf("failed to create conversation")
		}
		debugf("Run created conversation schedule_id=%q run_id=%q conversation_id=%q", schID, strings.TrimSpace(in.Id), strings.TrimSpace(resp.ID))
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
			if err := s.conv.PatchConversations(runCtx, c); err != nil {
				debugf("Run annotate conversation error schedule_id=%q run_id=%q conversation_id=%q err=%v", schID, strings.TrimSpace(in.Id), strings.TrimSpace(resp.ID), err)
			} else {
				debugf("Run annotated conversation schedule_id=%q run_id=%q conversation_id=%q", schID, strings.TrimSpace(in.Id), strings.TrimSpace(resp.ID))
			}
		}
	}

	// Post task prompt if defined
	var taskContent string
	taskSource := ""
	if row.TaskPrompt != nil && strings.TrimSpace(*row.TaskPrompt) != "" {
		taskContent = strings.TrimSpace(*row.TaskPrompt)
		taskSource = "task_prompt"
	} else if row.TaskPromptUri != nil && strings.TrimSpace(*row.TaskPromptUri) != "" {
		// Let chat layer resolve URI-based prompt if supported by the agent
		taskContent = strings.TrimSpace(*row.TaskPromptUri)
		taskSource = "task_prompt_uri"
	}
	debugf("Run task prompt resolved schedule_id=%q run_id=%q source=%q len=%d", schID, strings.TrimSpace(in.Id), taskSource, len(taskContent))
	if taskContent != "" && in.ConversationId != nil && strings.TrimSpace(*in.ConversationId) != "" {
		debugf("Run post task prompt schedule_id=%q run_id=%q conversation_id=%q agent=%q model=%q", schID, strings.TrimSpace(in.Id), strings.TrimSpace(*in.ConversationId), strings.TrimSpace(row.AgentRef), strPtrValue(row.ModelOverride))
		_, err = s.chat.Post(runCtx, *in.ConversationId, chatcli.PostRequest{Content: taskContent, Agent: row.AgentRef, Model: strPtrValue(row.ModelOverride)})
		if err != nil {
			debugf("Run post task prompt error schedule_id=%q run_id=%q conversation_id=%q err=%v", schID, strings.TrimSpace(in.Id), strings.TrimSpace(*in.ConversationId), err)
			return err
		}
		// Mark run as running when task is posted
		in.SetStatus("running")
		in.SetStartedAt(time.Now().UTC())
		log.Printf("scheduler: run started schedule_id=%q run_id=%q conversation_id=%q agent=%q model=%q", schID, strings.TrimSpace(in.Id), strings.TrimSpace(*in.ConversationId), strings.TrimSpace(row.AgentRef), strPtrValue(row.ModelOverride))
	}
	// Persist updated state (conversation linkage, running timestamps, etc.)
	if err := s.sch.PatchRun(ctx, in); err != nil {
		debugf("Run patch run after conversation/prompt error schedule_id=%q run_id=%q err=%v", schID, strings.TrimSpace(in.Id), err)
		return err
	}
	debugf("Run patched run after conversation/prompt schedule_id=%q run_id=%q status=%q started_at=%s completed_at=%s", schID, strings.TrimSpace(in.Id), strings.TrimSpace(in.Status), timePtrString(in.StartedAt), timePtrString(in.CompletedAt))
	// Best-effort: claim the run lease so other scheduler instances can detect liveness
	// via periodic heartbeats from watchRunCompletion.
	if strings.TrimSpace(s.leaseOwner) != "" {
		claimed, claimErr := s.sch.TryClaimRun(ctx, strings.TrimSpace(in.Id), s.leaseOwner, time.Now().UTC().Add(s.leaseTTL))
		debugf("Run claim run lease schedule_id=%q run_id=%q owner=%q claimed=%v err=%v", schID, strings.TrimSpace(in.Id), strings.TrimSpace(s.leaseOwner), claimed, claimErr)
	}
	// Fire-and-forget watcher to mark completion based on conversation progress
	if in.ConversationId != nil && strings.TrimSpace(*in.ConversationId) != "" {
		aCtx := context.WithoutCancel(runCtx)
		debugf("Run start watcher schedule_id=%q run_id=%q conversation_id=%q", schID, strings.TrimSpace(in.Id), strings.TrimSpace(*in.ConversationId))
		var startedAt *time.Time
		if in.StartedAt != nil && !in.StartedAt.IsZero() {
			t := in.StartedAt.UTC()
			startedAt = &t
		}
		go s.watchRunCompletion(aCtx, strings.TrimSpace(in.Id), schID, strings.TrimSpace(*in.ConversationId), row.TimeoutSeconds, row.Name, startedAt)
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

func (s *Service) getRunsForDueCheck(ctx context.Context, scheduleID string, scheduledFor time.Time, includeScheduledSlot bool) ([]*schapi.Run, error) {
	if s == nil || s.sch == nil {
		return nil, nil
	}
	debugf("getRunsForDueCheck start schedule_id=%q scheduled_for=%s include_slot=%v", strings.TrimSpace(scheduleID), scheduledFor.UTC().Format(time.RFC3339Nano), includeScheduledSlot)
	var slotRuns []*schapi.Run
	if includeScheduledSlot {
		in := &runpkg.RunInput{
			Id:           scheduleID,
			ScheduledFor: scheduledFor,
			Has:          &runpkg.RunInputHas{Id: true, ScheduledFor: true},
		}
		out, err := s.sch.ReadRuns(ctx, in, nil)
		if err != nil {
			debugf("getRunsForDueCheck read slot runs error schedule_id=%q scheduled_for=%s err=%v", strings.TrimSpace(scheduleID), scheduledFor.UTC().Format(time.RFC3339Nano), err)
			return nil, err
		}

		if out.Status.Status == "error" {
			debugf("getRunsForDueCheck read slot runs status=error schedule_id=%q scheduled_for=%s message=%q", strings.TrimSpace(scheduleID), scheduledFor.UTC().Format(time.RFC3339Nano), strings.TrimSpace(out.Status.Message))
			return nil, fmt.Errorf("failed to read runs for schedule %q scheduled_for %s: %s", scheduleID, scheduledFor.Format(time.RFC3339), out.Status.Message)
		}

		slotRuns = out.Data
		debugf("getRunsForDueCheck slot runs loaded schedule_id=%q scheduled_for=%s count=%d", strings.TrimSpace(scheduleID), scheduledFor.UTC().Format(time.RFC3339Nano), len(slotRuns))
	}

	in := &runpkg.RunInput{
		Id:              scheduleID,
		ExcludeStatuses: []string{"succeeded", "failed", "skipped"},
		Has:             &runpkg.RunInputHas{Id: true, ExcludeStatuses: true},
	}
	out, err := s.sch.ReadRuns(ctx, in, nil)
	if err != nil {
		debugf("getRunsForDueCheck read active runs error schedule_id=%q err=%v", strings.TrimSpace(scheduleID), err)
		return nil, err
	}

	if out.Status.Status == "error" {
		debugf("getRunsForDueCheck read active runs status=error schedule_id=%q message=%q", strings.TrimSpace(scheduleID), strings.TrimSpace(out.Status.Message))
		return nil, fmt.Errorf("failed to read runs for schedule %q scheduled_for %s: %s", scheduleID, scheduledFor.Format(time.RFC3339), out.Status.Message)
	}

	activeRuns := out.Data
	debugf("getRunsForDueCheck active runs loaded schedule_id=%q count=%d", strings.TrimSpace(scheduleID), len(activeRuns))

	runs := make([]*schapi.Run, 0, len(slotRuns)+len(activeRuns))
	seen := map[string]struct{}{}
	add := func(list []*schapi.Run) {
		for _, r := range list {
			if r == nil {
				continue
			}
			id := strings.TrimSpace(r.Id)
			if id == "" {
				continue
			}
			if _, ok := seen[id]; ok {
				continue
			}
			seen[id] = struct{}{}
			runs = append(runs, r)
		}
	}
	add(slotRuns)
	add(activeRuns)
	debugf("getRunsForDueCheck done schedule_id=%q total=%d", strings.TrimSpace(scheduleID), len(runs))
	return runs, nil
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

	s.ensureLeaseConfig()
	debugf("RunDue tick start lease_owner=%q lease_ttl=%s", strings.TrimSpace(s.leaseOwner), s.leaseTTL)

	rows, err := s.sch.GetSchedules(ctx)
	if err != nil {
		return 0, err
	}
	debugf("RunDue schedules loaded count=%d", len(rows))

	started := 0
	for _, sc := range rows {
		now := time.Now().UTC()

		if sc == nil || !sc.Enabled {
			if sc != nil {
				debugf("RunDue skip schedule disabled id=%q name=%q", strings.TrimSpace(sc.Id), strings.TrimSpace(sc.Name))
			}

			if sc != nil && !sc.Enabled {
				// Stale run detection (disabled schedules are skipped by due-check logic).
				// This handles the case where a schedule was disabled while a run was active
				// and the scheduler was restarted (watchers are in-memory and won't resume).
				scheduleCtx := ctx
				if owner := strPtrValue(sc.CreatedByUserId); owner != "" {
					scheduleCtx = authctx.WithUserInfo(scheduleCtx, &authctx.UserInfo{Subject: owner})
				} else {
					scheduleCtx = authctx.EnsureUser(scheduleCtx, s.authCfg)
				}

				scheduledFor := now
				if sc.NextRunAt != nil && !sc.NextRunAt.IsZero() {
					scheduledFor = sc.NextRunAt.UTC()
				}

				runs, err := s.getRunsForDueCheck(scheduleCtx, sc.Id, scheduledFor, false)
				if err != nil {
					debugf("RunDue disabled schedule stale-check read runs error schedule_id=%q err=%v", strings.TrimSpace(sc.Id), err)
				} else {
					for _, r := range runs {
						if r == nil || r.CompletedAt != nil {
							continue
						}
						status := strings.ToLower(strings.TrimSpace(r.Status))
						switch status {
						case "running", "prechecking":
							runStart, timeout, stale := s.isStaleRun(r, sc, now)
							if stale {
								debugf("RunDue disabled schedule stale run detected schedule_id=%q run_id=%q status=%q", strings.TrimSpace(sc.Id), strings.TrimSpace(r.Id), strings.TrimSpace(r.Status))
								if err, _ := s.handleStaleRun(scheduleCtx, r, now, sc, runStart, timeout, scheduledFor); err != nil {
									debugf("RunDue disabled schedule stale run handling failed schedule_id=%q run_id=%q err=%v", strings.TrimSpace(sc.Id), strings.TrimSpace(r.Id), err)
								}
							}
						}
					}
				}
			}
			continue
		}

		debugf(
			"RunDue schedule check id=%q name=%q type=%q enabled=%v now=%s tz=%q cron=%q interval_seconds=%v start_at=%s end_at=%s created_at=%s last_run_at=%s next_run_at=%s visibility=%q created_by_user_id=%q lease_owner=%q lease_until=%s timeout_seconds=%d user_cred_url=%q",
			strings.TrimSpace(sc.Id),
			strings.TrimSpace(sc.Name),
			strings.TrimSpace(sc.ScheduleType),
			sc.Enabled,
			now.Format(time.RFC3339Nano),
			strings.TrimSpace(sc.Timezone),
			strPtrValue(sc.CronExpr),
			sc.IntervalSeconds,
			timePtrString(sc.StartAt),
			timePtrString(sc.EndAt),
			sc.CreatedAt.UTC().Format(time.RFC3339Nano),
			timePtrString(sc.LastRunAt),
			timePtrString(sc.NextRunAt),
			strings.TrimSpace(sc.Visibility),
			strPtrValue(sc.CreatedByUserId),
			strPtrValue(sc.LeaseOwner),
			timePtrString(sc.LeaseUntil),
			sc.TimeoutSeconds,
			redactCredRef(strPtrValue(sc.UserCredURL)),
		)

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
			debugf("RunDue skip schedule not started id=%q now=%s start_at=%s", strings.TrimSpace(sc.Id), now.Format(time.RFC3339Nano), timePtrString(sc.StartAt))
			continue
		}
		if sc.EndAt != nil && !now.Before(sc.EndAt.UTC()) {
			debugf("RunDue skip schedule expired id=%q now=%s end_at=%s", strings.TrimSpace(sc.Id), now.Format(time.RFC3339Nano), timePtrString(sc.EndAt))
			continue
		}
		if strings.EqualFold(strings.TrimSpace(sc.ScheduleType), "cron") && sc.CronExpr != nil && strings.TrimSpace(*sc.CronExpr) != "" {
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
			computedNext := cronNext(spec, base).In(time.UTC)
			next := computedNext
			if sc.NextRunAt != nil {
				next = sc.NextRunAt.UTC()
			} else {
				// When next_run_at is NULL, use computedNext as the effective slot for this tick.
				// Persist it only when the schedule isn't due yet, so subsequent ticks and the UI
				// have a stable reference (due schedules will be advanced after a run is started).
				seedNext := computedNext
				if now.Before(seedNext) {
					mut := &schapi.MutableSchedule{}
					mut.SetId(sc.Id)
					mut.SetNextRunAt(seedNext)
					if err := s.sch.PatchSchedule(scheduleCtx, mut); err != nil {
						debugf(
							"RunDue persist cron next_run_at failed schedule_id=%q next_run_at=%s err=%v",
							strings.TrimSpace(sc.Id),
							seedNext.UTC().Format(time.RFC3339Nano),
							err,
						)
					}
				}
				sc.NextRunAt = &seedNext
			}
			if !now.Before(next) {
				due = true
			}
			debugf(
				"RunDue cron due-check id=%q now=%s loc=%q base=%s computed_next=%s stored_next_run_at=%s effective_next=%s due=%v",
				strings.TrimSpace(sc.Id),
				now.Format(time.RFC3339Nano),
				loc.String(),
				base.Format(time.RFC3339Nano),
				computedNext.UTC().Format(time.RFC3339Nano),
				timePtrString(sc.NextRunAt),
				next.UTC().Format(time.RFC3339Nano),
				due,
			)
		} else if sc.NextRunAt != nil && !now.Before(sc.NextRunAt.UTC()) {
			due = true
			debugf("RunDue due-check next_run_at id=%q now=%s next_run_at=%s due=%v", strings.TrimSpace(sc.Id), now.Format(time.RFC3339Nano), timePtrString(sc.NextRunAt), due)
		} else if sc.IntervalSeconds != nil {
			base := sc.CreatedAt.UTC()
			if sc.LastRunAt != nil {
				base = sc.LastRunAt.UTC()
			}
			next := base.Add(time.Duration(*sc.IntervalSeconds) * time.Second)
			if !now.Before(next) {
				due = true
			}
			debugf(
				"RunDue interval due-check id=%q now=%s base=%s interval_seconds=%d next=%s due=%v",
				strings.TrimSpace(sc.Id),
				now.Format(time.RFC3339Nano),
				base.UTC().Format(time.RFC3339Nano),
				*sc.IntervalSeconds,
				next.UTC().Format(time.RFC3339Nano),
				due,
			)
		}
		if !due {
			debugf("RunDue skip schedule not due id=%q now=%s next_run_at=%s", strings.TrimSpace(sc.Id), now.Format(time.RFC3339Nano), timePtrString(sc.NextRunAt))
			continue
		}

		leaseUntil := now.Add(s.leaseTTL)
		debugf(
			"RunDue claim schedule lease id=%q owner=%q lease_until=%s current_lease_owner=%q current_lease_until=%s",
			strings.TrimSpace(sc.Id),
			strings.TrimSpace(s.leaseOwner),
			leaseUntil.UTC().Format(time.RFC3339Nano),
			strPtrValue(sc.LeaseOwner),
			timePtrString(sc.LeaseUntil),
		)
		claimed, err := s.sch.TryClaimSchedule(scheduleCtx, sc.Id, s.leaseOwner, leaseUntil)
		if err != nil {
			debugf("RunDue claim schedule lease error id=%q err=%v", strings.TrimSpace(sc.Id), err)
			return started, err
		}
		if !claimed {
			debugf("RunDue schedule lease not claimed; skip id=%q owner=%q current_lease_owner=%q current_lease_until=%s", strings.TrimSpace(sc.Id), strings.TrimSpace(s.leaseOwner), strPtrValue(sc.LeaseOwner), timePtrString(sc.LeaseUntil))
			continue
		}

		releaseLease := func() {
			released, relErr := s.sch.ReleaseScheduleLease(context.Background(), sc.Id, s.leaseOwner)
			debugf("RunDue release schedule lease id=%q owner=%q released=%v err=%v", strings.TrimSpace(sc.Id), strings.TrimSpace(s.leaseOwner), released, relErr)
		}

		err = func() error {
			defer releaseLease()
			scheduledFor := now
			includeScheduledSlot := false
			if sc.NextRunAt != nil && !sc.NextRunAt.IsZero() {
				scheduledFor = sc.NextRunAt.UTC()
				includeScheduledSlot = true
			}
			debugf("RunDue due schedule id=%q scheduled_for=%s include_slot=%v", strings.TrimSpace(sc.Id), scheduledFor.UTC().Format(time.RFC3339Nano), includeScheduledSlot)

			runs, err := s.getRunsForDueCheck(scheduleCtx, sc.Id, scheduledFor, includeScheduledSlot)
			if err != nil {
				debugf("RunDue get runs error schedule_id=%q scheduled_for=%s err=%v", strings.TrimSpace(sc.Id), scheduledFor.UTC().Format(time.RFC3339Nano), err)
				return err
			}
			if DebugEnabled() {
				for _, r := range runs {
					if r == nil {
						continue
					}
					debugf(
						"RunDue due-check run schedule_id=%q run_id=%q status=%q scheduled_for=%s created_at=%s started_at=%s completed_at=%s lease_owner=%q lease_until=%s conv_id=%q",
						strings.TrimSpace(sc.Id),
						strings.TrimSpace(r.Id),
						strings.TrimSpace(r.Status),
						timePtrString(r.ScheduledFor),
						r.CreatedAt.UTC().Format(time.RFC3339Nano),
						timePtrString(r.StartedAt),
						timePtrString(r.CompletedAt),
						strPtrValue(r.LeaseOwner),
						timePtrString(r.LeaseUntil),
						strPtrValue(r.ConversationId),
					)
				}
			}

			var pendingRun *schapi.Run
			for _, r := range runs {
				if r == nil {
					continue
				}

				// If a run already exists for the current scheduled slot (even completed),
				// do not create another run for the same (schedule_id, scheduled_for) slot.
				// This can happen after a crash where the run completed but the schedule row
				// didn't advance next_run_at.
				if includeScheduledSlot && r.ScheduledFor != nil && r.ScheduledFor.UTC().Equal(scheduledFor) && r.CompletedAt != nil {
					debugf("RunDue completed run exists for current slot; advance schedule id=%q run_id=%q scheduled_for=%s", strings.TrimSpace(sc.Id), strings.TrimSpace(r.Id), scheduledFor.UTC().Format(time.RFC3339Nano))
					lastStatus := strings.TrimSpace(r.Status)
					var lastRunAt *time.Time
					switch {
					case r.StartedAt != nil && !r.StartedAt.IsZero():
						t := r.StartedAt.UTC()
						lastRunAt = &t
					case r.ScheduledFor != nil && !r.ScheduledFor.IsZero():
						t := r.ScheduledFor.UTC()
						lastRunAt = &t
					case !r.CreatedAt.IsZero():
						t := r.CreatedAt.UTC()
						lastRunAt = &t
					case r.CompletedAt != nil && !r.CompletedAt.IsZero():
						t := r.CompletedAt.UTC()
						lastRunAt = &t
					}
					err = s.updateNextRunAt(sc, now, scheduleCtx, &lastStatus, r.ErrorMessage, lastRunAt)
					if err != nil {
						debugf("RunDue advance schedule error id=%q err=%v", strings.TrimSpace(sc.Id), err)
						return err
					}
					return nil //TODO
				}

				if r.CompletedAt != nil {
					continue
				}

				status := strings.ToLower(strings.TrimSpace(r.Status))
				switch status {
				case "running", "prechecking":
					// If the run is stale (e.g. scheduler crashed and no watcher is running),
					// mark it failed so it doesn't block future runs.
					runStart, timeout, stale := s.isStaleRun(r, sc, now)

					if stale {
						debugf(
							"RunDue stale run detected schedule_id=%q run_id=%q status=%q run_start=%s timeout=%s now=%s lease_owner=%q lease_until=%s run_scheduled_for=%s current_slot=%s",
							strings.TrimSpace(sc.Id),
							strings.TrimSpace(r.Id),
							strings.TrimSpace(r.Status),
							runStart.UTC().Format(time.RFC3339Nano),
							timeout,
							now.UTC().Format(time.RFC3339Nano),
							strPtrValue(r.LeaseOwner),
							timePtrString(r.LeaseUntil),
							timePtrString(r.ScheduledFor),
							scheduledFor.UTC().Format(time.RFC3339Nano),
						)
						err, done := s.handleStaleRun(scheduleCtx, r, now, sc, runStart, timeout, scheduledFor)
						if done {
							debugf("RunDue stale run handled schedule_id=%q run_id=%q done=%v err=%v", strings.TrimSpace(sc.Id), strings.TrimSpace(r.Id), done, err)
							return err
						}
						// Stale run was for an older slot; continue so a new run can be started.
						continue
					}

					// Another instance is processing this schedule.
					// For ad-hoc schedules, clear next_run_at so they don't remain due
					// while a run is already in progress (e.g. when started via run-now).
					if strings.EqualFold(strings.TrimSpace(sc.ScheduleType), "adhoc") && sc.NextRunAt != nil && !sc.NextRunAt.IsZero() {
						debugf("RunDue adhoc active run; clearing next_run_at schedule_id=%q run_id=%q next_run_at=%s", strings.TrimSpace(sc.Id), strings.TrimSpace(r.Id), timePtrString(sc.NextRunAt))
						mut := &schapi.MutableSchedule{}
						mut.SetId(sc.Id)
						mut.NextRunAt = nil
						if mut.Has != nil {
							mut.Has.NextRunAt = true
						}
						_ = s.sch.PatchSchedule(scheduleCtx, mut)
					}

					debugf("RunDue active run blocks scheduling schedule_id=%q run_id=%q status=%q", strings.TrimSpace(sc.Id), strings.TrimSpace(r.Id), strings.TrimSpace(r.Status))
					return nil
				case "pending":
					// Prefer a pending run that matches the current scheduled slot.
					if pendingRun == nil {
						pendingRun = r
					}
					if r.ScheduledFor != nil && r.ScheduledFor.UTC().Equal(scheduledFor) {
						pendingRun = r
					}
				}
			}

			run := &schapi.MutableRun{}
			if pendingRun != nil {
				run.SetId(strings.TrimSpace(pendingRun.Id))
				debugf("RunDue reusing pending run schedule_id=%q run_id=%q scheduled_for=%s", strings.TrimSpace(sc.Id), strings.TrimSpace(pendingRun.Id), scheduledFor.UTC().Format(time.RFC3339Nano))
			} else {
				run.SetId(uuid.NewString())
				debugf("RunDue creating new run schedule_id=%q run_id=%q scheduled_for=%s", strings.TrimSpace(sc.Id), strings.TrimSpace(run.Id), scheduledFor.UTC().Format(time.RFC3339Nano))
			}
			run.SetScheduleId(sc.Id)
			run.SetStatus("pending")
			run.SetScheduledFor(scheduledFor)
			debugf("RunDue enqueue run schedule_id=%q run_id=%q scheduled_for=%s", strings.TrimSpace(sc.Id), strings.TrimSpace(run.Id), scheduledFor.UTC().Format(time.RFC3339Nano))
			if err := s.Run(scheduleCtx, run); err != nil {
				debugf("RunDue start run error schedule_id=%q run_id=%q err=%v", strings.TrimSpace(sc.Id), strings.TrimSpace(run.Id), err)
				return err
			}

			// Update schedule next_run_at (last_run_at is patched on completion alongside last_status).
			err = s.updateNextRunAt(sc, now, scheduleCtx, nil, nil, nil)
			if err != nil {
				debugf("RunDue update schedule next_run_at error schedule_id=%q run_id=%q err=%v", strings.TrimSpace(sc.Id), strings.TrimSpace(run.Id), err)
				return err
			}
			debugf("RunDue started run schedule_id=%q run_id=%q scheduled_for=%s", strings.TrimSpace(sc.Id), strings.TrimSpace(run.Id), scheduledFor.UTC().Format(time.RFC3339Nano))
			started++
			return nil
		}()
		if err != nil {
			debugf("RunDue schedule processing error schedule_id=%q name=%q err=%v", strings.TrimSpace(sc.Id), strings.TrimSpace(sc.Name), err)
			return started, err
		}
	}

	return started, nil
}

func (s *Service) handleStaleRun(scheduleCtx context.Context, r *schapi.Run, now time.Time, sc *schedulepkg.ScheduleView, runStart time.Time, timeout time.Duration, scheduledFor time.Time) (error, bool) {
	debugf(
		"handleStaleRun start schedule_id=%q run_id=%q now=%s run_status=%q run_start=%s timeout=%s run_scheduled_for=%s current_slot=%s",
		strings.TrimSpace(sc.Id),
		strings.TrimSpace(r.Id),
		now.UTC().Format(time.RFC3339Nano),
		strings.TrimSpace(r.Status),
		runStart.UTC().Format(time.RFC3339Nano),
		timeout,
		timePtrString(r.ScheduledFor),
		scheduledFor.UTC().Format(time.RFC3339Nano),
	)
	claimCtx, claimCancel := context.WithTimeout(scheduleCtx, callTimeout)
	claimed, claimErr := s.sch.TryClaimRun(claimCtx, strings.TrimSpace(r.Id), s.leaseOwner, now.Add(s.leaseTTL))
	claimCancel()
	// If another instance owns the run lease, let it finalize.
	if claimErr == nil && !claimed {
		debugf("handleStaleRun skip: run lease owned by another instance schedule_id=%q run_id=%q owner=%q", strings.TrimSpace(sc.Id), strings.TrimSpace(r.Id), strings.TrimSpace(s.leaseOwner))
		return nil, true
	}
	if claimErr != nil {
		debugf("handleStaleRun claim run lease error schedule_id=%q run_id=%q owner=%q err=%v", strings.TrimSpace(sc.Id), strings.TrimSpace(r.Id), strings.TrimSpace(s.leaseOwner), claimErr)
	} else {
		debugf("handleStaleRun claimed run lease schedule_id=%q run_id=%q owner=%q claimed=%v lease_until=%s", strings.TrimSpace(sc.Id), strings.TrimSpace(r.Id), strings.TrimSpace(s.leaseOwner), claimed, now.Add(s.leaseTTL).UTC().Format(time.RFC3339Nano))
	}

	convID := ""
	if r.ConversationId != nil {
		convID = strings.TrimSpace(*r.ConversationId)
	}
	if convID != "" {
		debugf("handleStaleRun cancel conversation schedule_id=%q run_id=%q conv_id=%q", strings.TrimSpace(sc.Id), strings.TrimSpace(r.Id), convID)
		_ = s.chat.Cancel(convID)
	}

	upd := &schapi.MutableRun{}
	upd.SetId(strings.TrimSpace(r.Id))
	upd.SetScheduleId(sc.Id)
	upd.SetStatus("failed")
	upd.SetCompletedAt(now)
	msg := fmt.Sprintf("stale run detected (current time: %v): status=%q started_at=%v timeout=%v", now, strings.TrimSpace(r.Status), runStart, timeout)
	if r.LeaseUntil != nil && !r.LeaseUntil.IsZero() {
		msg += fmt.Sprintf(" lease_until=%v lease_owner=%q", r.LeaseUntil.UTC(), strPtrValue(r.LeaseOwner))
	}
	upd.SetErrorMessage(msg)

	callCtx, callCancel := context.WithTimeout(scheduleCtx, callTimeout)
	if err := s.sch.PatchRun(callCtx, upd); err != nil {
		callCancel()
		debugf("handleStaleRun patch run failed schedule_id=%q run_id=%q err=%v", strings.TrimSpace(sc.Id), strings.TrimSpace(r.Id), err)
		return err, true
	}
	callCancel()
	debugf("handleStaleRun patched run failed schedule_id=%q run_id=%q", strings.TrimSpace(sc.Id), strings.TrimSpace(r.Id))

	if claimErr == nil && claimed {
		released, relErr := s.sch.ReleaseRunLease(scheduleCtx, strings.TrimSpace(r.Id), s.leaseOwner)
		debugf("handleStaleRun release run lease schedule_id=%q run_id=%q released=%v err=%v", strings.TrimSpace(sc.Id), strings.TrimSpace(r.Id), released, relErr)
	}

	// If this stale run belongs to the current scheduled slot, treat that slot
	// as processed by advancing schedule.next_run_at (do not create a new run
	// for the same scheduled_for).
	if r.ScheduledFor != nil && r.ScheduledFor.UTC().Equal(scheduledFor) {
		debugf("handleStaleRun stale run belongs to current slot; advancing schedule schedule_id=%q run_id=%q", strings.TrimSpace(sc.Id), strings.TrimSpace(r.Id))
		status := "failed"
		runStartedAt := runStart.UTC()
		err := s.updateNextRunAt(sc, now, scheduleCtx, &status, &msg, &runStartedAt)
		if err != nil {
			debugf("handleStaleRun advance schedule error schedule_id=%q err=%v", strings.TrimSpace(sc.Id), err)
		}
		return err, true
	}

	// Older slot: update schedule last result, but do not advance next_run_at so a new run can be started.
	runStartedAt := runStart.UTC()
	s.patchScheduleLastResult(scheduleCtx, sc.Id, "failed", &runStartedAt, &msg)
	debugf("handleStaleRun stale run belonged to older slot; allow new run schedule_id=%q run_id=%q run_scheduled_for=%s current_slot=%s", strings.TrimSpace(sc.Id), strings.TrimSpace(r.Id), timePtrString(r.ScheduledFor), scheduledFor.UTC().Format(time.RFC3339Nano))
	return nil, false
}

func (s *Service) updateNextRunAt(sc *schedulepkg.ScheduleView, now time.Time, scheduleCtx context.Context, lastStatus *string, lastError *string, lastRunAt *time.Time) error {
	debugf(
		"updateNextRunAt start schedule_id=%q now=%s prev_last_run_at=%s prev_next_run_at=%s last_status=%q last_run_at=%s",
		strings.TrimSpace(sc.Id),
		now.UTC().Format(time.RFC3339Nano),
		timePtrString(sc.LastRunAt),
		timePtrString(sc.NextRunAt),
		strPtrValue(lastStatus),
		timePtrString(lastRunAt),
	)
	mut := &schapi.MutableSchedule{}
	mut.SetId(sc.Id)
	if lastRunAt != nil && !lastRunAt.IsZero() {
		mut.SetLastRunAt(lastRunAt.UTC())
	}
	if lastStatus != nil && strings.TrimSpace(*lastStatus) != "" {
		mut.SetLastStatus(strings.TrimSpace(*lastStatus))
	}
	if lastError != nil {
		if strings.TrimSpace(*lastError) == "" {
			mut.LastError = nil
			if mut.Has != nil {
				mut.Has.LastError = true
			}
		} else {
			mut.SetLastError(strings.TrimSpace(*lastError))
		}
	} else if lastStatus != nil && strings.EqualFold(strings.TrimSpace(*lastStatus), "succeeded") {
		// Clear old errors on success so the schedule doesn't show stale failures.
		mut.LastError = nil
		if mut.Has != nil {
			mut.Has.LastError = true
		}
	}

	if err := s.setNextRunAt(sc, mut, now); err != nil {
		return err
	}

	debugf(
		"updateNextRunAt patch schedule_id=%q last_run_at=%s next_run_at=%s",
		strings.TrimSpace(sc.Id),
		timePtrString(mut.LastRunAt),
		timePtrString(mut.NextRunAt),
	)
	if err := s.sch.PatchSchedule(scheduleCtx, mut); err != nil {
		debugf("updateNextRunAt patch schedule error schedule_id=%q err=%v", strings.TrimSpace(sc.Id), err)
		return err
	}
	debugf("updateNextRunAt done schedule_id=%q next_run_at=%s", strings.TrimSpace(sc.Id), timePtrString(mut.NextRunAt))
	return nil
}

func (s *Service) isStaleRun(r *schapi.Run, sc *schedulepkg.ScheduleView, now time.Time) (time.Time, time.Duration, bool) {
	runStart := r.CreatedAt.UTC()
	if r.StartedAt != nil && !r.StartedAt.IsZero() {
		runStart = r.StartedAt.UTC()
	}
	timeout := watchTimeout
	if sc.TimeoutSeconds > 0 {
		timeout = time.Duration(sc.TimeoutSeconds) * time.Second
	}
	staleGrace := 15 * time.Second
	leaseExpired := false
	if r.LeaseUntil != nil && !r.LeaseUntil.IsZero() {
		leaseExpired = now.After(r.LeaseUntil.UTC().Add(staleGrace))
	}
	deadlineExceeded := now.After(runStart.Add(timeout).Add(staleGrace))

	stale := false
	if r.LeaseUntil != nil && !r.LeaseUntil.IsZero() {
		// Prefer lease-based detection to quickly recover after crashes.
		// If the lease is still being heartbeated, another instance is likely alive.
		stale = leaseExpired
	} else {
		// Fallback for legacy runs without a lease.
		stale = deadlineExceeded
	}
	debugf(
		"isStaleRun schedule_id=%q run_id=%q status=%q now=%s run_start=%s timeout=%s lease_until=%s lease_expired=%v deadline_exceeded=%v stale=%v",
		strings.TrimSpace(sc.Id),
		strings.TrimSpace(r.Id),
		strings.TrimSpace(r.Status),
		now.UTC().Format(time.RFC3339Nano),
		runStart.UTC().Format(time.RFC3339Nano),
		timeout,
		timePtrString(r.LeaseUntil),
		leaseExpired,
		deadlineExceeded,
		stale,
	)
	return runStart, timeout, stale
}

func (s *Service) setNextRunAt(sc *schedulepkg.ScheduleView, mut *schapi.MutableSchedule, now time.Time) error {
	if strings.EqualFold(strings.TrimSpace(sc.ScheduleType), "cron") && sc.CronExpr != nil && strings.TrimSpace(*sc.CronExpr) != "" {
		loc, _ := time.LoadLocation(strings.TrimSpace(sc.Timezone))
		if loc == nil {
			loc = time.UTC
		}
		spec, err := parseCron(strings.TrimSpace(*sc.CronExpr))
		if err != nil {
			return fmt.Errorf("invalid cron expr for schedule %s: %w", sc.Id, err)
		}
		next := cronNext(spec, now.In(loc)).UTC()
		mut.SetNextRunAt(next)
		debugf(
			"setNextRunAt cron schedule_id=%q cron=%q tz=%q now=%s next_run_at=%s",
			strings.TrimSpace(sc.Id),
			strPtrValue(sc.CronExpr),
			strings.TrimSpace(sc.Timezone),
			now.UTC().Format(time.RFC3339Nano),
			next.UTC().Format(time.RFC3339Nano),
		)
	} else if sc.IntervalSeconds != nil {
		next := now.Add(time.Duration(*sc.IntervalSeconds) * time.Second)
		mut.SetNextRunAt(next)
		debugf(
			"setNextRunAt interval schedule_id=%q interval_seconds=%d now=%s next_run_at=%s",
			strings.TrimSpace(sc.Id),
			*sc.IntervalSeconds,
			now.UTC().Format(time.RFC3339Nano),
			next.UTC().Format(time.RFC3339Nano),
		)
	} else if strings.EqualFold(strings.TrimSpace(sc.ScheduleType), "adhoc") {
		mut.NextRunAt = nil
		if mut.Has != nil {
			mut.Has.NextRunAt = true
		}
		debugf("setNextRunAt adhoc schedule_id=%q now=%s next_run_at=nil", strings.TrimSpace(sc.Id), now.UTC().Format(time.RFC3339Nano))
	}

	return nil
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
