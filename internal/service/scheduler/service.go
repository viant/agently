package scheduler

import (
	"context"
	"fmt"
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
	in.SetConversationKind("scheduled")

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
	// Persist initial state
	if err := s.sch.PatchRun(ctx, in); err != nil {
		return err
	}
	// Fire-and-forget watcher to mark completion based on conversation progress
	if in.ConversationId != nil && strings.TrimSpace(*in.ConversationId) != "" {
		go s.watchRunCompletion(context.Background(), strings.TrimSpace(in.Id), schID, strings.TrimSpace(*in.ConversationId))
	}
	return nil
}

// watchRunCompletion polls conversation stage until completion and updates the run status.
func (s *Service) watchRunCompletion(ctx context.Context, runID, scheduleID, conversationID string) {
	if s == nil || s.conv == nil || s.sch == nil {
		return
	}
	deadline := time.Now().Add(10 * time.Minute)
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()
	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			conv, err := s.conv.GetConversation(context.Background(), conversationID)
			if err != nil || conv == nil {
				continue
			}
			stage := strings.ToLower(strings.TrimSpace(conv.Stage))
			// Running stages
			if stage == "executing" || stage == "thinking" || stage == "eliciting" || stage == "waiting" {
				continue
			}
			// Decide final status
			status := "succeeded"
			if stage == "error" || stage == "failed" || stage == "canceled" {
				if stage == "canceled" {
					status = "skipped"
				} else {
					status = "failed"
				}
			}
			upd := &schapi.MutableRun{}
			upd.SetId(runID)
			upd.SetScheduleId(scheduleID)
			upd.SetStatus(status)
			done := time.Now().UTC()
			upd.SetCompletedAt(done)
			// Best-effort patch; exit regardless of error to avoid loops
			_ = s.sch.PatchRun(context.Background(), upd)
			return
		}
	}
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
		// Determine due
		due := false
		// 1) Cron-based
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
			}
			next := cronNext(spec, base).In(time.UTC)
			// Use stored NextRunAt when present, else compute it; ensure future update after starting.
			if sc.NextRunAt != nil {
				next = sc.NextRunAt.UTC()
			}
			if !now.Before(next) {
				due = true
			}
		} else if sc.NextRunAt != nil && !now.Before(sc.NextRunAt.UTC()) {
			// 2) Explicit NextRunAt
			due = true
		} else if sc.IntervalSeconds != nil {
			// 3) Interval-based
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

		// Avoid duplicate if there is any active run (not terminal)
		runs, err := s.sch.GetRuns(ctx, sc.Id, "")
		if err != nil {
			return started, err
		}
		active := false
		for _, r := range runs {
			if r == nil {
				continue
			}
			if r.CompletedAt == nil {
				st := strings.ToLower(strings.TrimSpace(r.Status))
				// Treat failed, skipped, and succeeded as non-active terminal states
				if st != "failed" && st != "skipped" && st != "succeeded" {
					active = true
					break
				}
			}
		}
		if active {
			continue
		}

		run := &schapi.MutableRun{}
		run.SetId(uuid.NewString())
		run.SetScheduleId(sc.Id)
		// Insert as pending; transitions to running when task is posted
		run.SetStatus("pending")
		if err := s.Run(ctx, run); err != nil {
			return started, err
		}

		// Update NextRunAt for cron/interval schedules
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
		}
		// fire-and-forget update; errors are bubbled when returned
		if err := s.sch.PatchSchedule(ctx, mut); err != nil {
			return started, err
		}
		started++
	}
	return started, nil
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
