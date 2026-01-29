package scheduler

import (
	"context"
	"fmt"
	"os"
	"strings"

	"time"

	"github.com/google/uuid"
	chatcli "github.com/viant/agently/client/chat"
	apiconv "github.com/viant/agently/client/conversation"
	schapi "github.com/viant/agently/client/scheduler"
	schcli "github.com/viant/agently/client/scheduler/store"
	"github.com/viant/agently/internal/codec"
	chatimpl "github.com/viant/agently/internal/service/chat"
	convintern "github.com/viant/agently/internal/service/conversation"
	schedstore "github.com/viant/agently/internal/service/scheduler/store"
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
	//ctx = authctx.WithInternalAccess(ctx) // TODO delete or accept
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
		if v := strings.TrimSpace(row.AgentRef); v != "" {
			req.Agent = v
		}
		if row.ModelOverride != nil && strings.TrimSpace(*row.ModelOverride) != "" {
			req.Model = strings.TrimSpace(*row.ModelOverride)
		}
		resp, err := s.chat.CreateConversation(ctx, req)
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
			_ = s.conv.PatchConversations(ctx, c)
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
		_, err = s.chat.Post(ctx, *in.ConversationId, chatcli.PostRequest{Content: taskContent, Agent: row.AgentRef, Model: strPtrValue(row.ModelOverride)})
		if err != nil {
			return err
		}
		// Mark run as running when task is posted
		in.SetStatus("running")
		in.SetStartedAt(time.Now().UTC())
	}
	// Persist updated state (conversation linkage, running timestamps, etc.)
	if err := s.sch.PatchRun(ctx, in); err != nil {
		return err
	}
	// Fire-and-forget watcher to mark completion based on conversation progress
	if in.ConversationId != nil && strings.TrimSpace(*in.ConversationId) != "" {
		aCtx := context.WithoutCancel(ctx)
		go s.watchRunCompletion(aCtx, strings.TrimSpace(in.Id), schID, strings.TrimSpace(*in.ConversationId))
	}
	return nil
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

	now := time.Now().UTC()
	started := 0
	for _, sc := range rows {
		if sc == nil || !sc.Enabled {
			continue
		}

		due := false
		// Respect optional schedule time window.
		if sc.StartAt != nil && now.Before(sc.StartAt.UTC()) {
			continue
		}
		if sc.EndAt != nil && !now.Before(sc.EndAt.UTC()) {
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
			next := cronNext(spec, base).In(time.UTC)
			if sc.NextRunAt != nil {
				next = sc.NextRunAt.UTC()
			}
			if !now.Before(next) {
				due = true
			}
		} else if sc.NextRunAt != nil && !now.Before(sc.NextRunAt.UTC()) {
			due = true
		} else if sc.IntervalSeconds != nil {
			base := sc.CreatedAt.UTC()
			if sc.LastRunAt != nil {
				base = sc.LastRunAt.UTC()
			}
			if !now.Before(base.Add(time.Duration(*sc.IntervalSeconds) * time.Second)) {
				due = true
			}
		}
		if !due {
			continue
		}

		leaseUntil := now.Add(s.leaseTTL)
		claimed, err := s.sch.TryClaimSchedule(ctx, sc.Id, s.leaseOwner, leaseUntil)
		if err != nil {
			return started, err
		}
		if !claimed {
			continue
		}

		releaseLease := func() {
			_, _ = s.sch.ReleaseScheduleLease(context.Background(), sc.Id, s.leaseOwner)
		}

		err = func() error {
			defer releaseLease()

			runs, err := s.sch.GetRuns(ctx, sc.Id, "")
			if err != nil {
				return err
			}
			scheduledFor := now
			if sc.NextRunAt != nil && !sc.NextRunAt.IsZero() {
				scheduledFor = sc.NextRunAt.UTC()
			}

			var pendingRun *schapi.Run
			for _, r := range runs {
				if r == nil || r.CompletedAt != nil {
					continue
				}
				st := strings.ToLower(strings.TrimSpace(r.Status))
				switch st {
				case "running", "prechecking":
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
						_ = s.sch.PatchSchedule(ctx, mut)
					}
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
			} else {
				run.SetId(uuid.NewString())
			}
			run.SetScheduleId(sc.Id)
			run.SetStatus("pending")
			run.SetScheduledFor(scheduledFor)

			if err := s.Run(ctx, run); err != nil {
				return err
			}

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
			if err := s.sch.PatchSchedule(ctx, mut); err != nil {
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
