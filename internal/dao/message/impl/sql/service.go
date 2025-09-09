package sql

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	plan "github.com/viant/agently/genai/agent/plan"
	"github.com/viant/agently/internal/dao/message/impl/shared"
	read2 "github.com/viant/agently/internal/dao/message/read"
	"github.com/viant/agently/internal/dao/message/write"
	mcread "github.com/viant/agently/internal/dao/modelcall/read"
	pldaoRead "github.com/viant/agently/internal/dao/payload/read"
	tcread "github.com/viant/agently/internal/dao/toolcall/read"
	"github.com/viant/datly"
	"github.com/viant/datly/repository/contract"
)

type Service struct{ dao *datly.Service }

func New(ctx context.Context, dao *datly.Service) *Service { return &Service{dao: dao} }

// Register components (to be invoked by parent module).
func Register(ctx context.Context, dao *datly.Service) error {
	if err := read2.DefineComponent(ctx, dao); err != nil {
		return err
	}
	if _, err := write.DefineComponent(ctx, dao); err != nil {
		return err
	}
	return nil
}

// aggregateExecutions computes tool/model outcomes grouped by root message id.
// It uses DAO read components to load tool and model calls and converts them to
// plan outcomes, assigning global, stable TraceIDs based on chronological order
// and phase. Payloads are referenced via $ref URLs when requested by input.
func (s *Service) aggregateExecutions(ctx context.Context, conversationID, uri string, in *read2.Input) (map[string][]*plan.Outcome, error) {
	// Load all messages to compute parent relationships
	allOut := &read2.Output{}
	_, _ = s.dao.Operate(ctx, datly.WithOutput(allOut), datly.WithURI(uri), datly.WithInput(&read2.Input{ConversationID: conversationID, Has: &read2.Has{ConversationID: true}}))
	allViews := allOut.Data

	// Build parent map and root resolver
	parent := map[string]string{}
	for _, v := range allViews {
		if v == nil {
			continue
		}
		if v.ParentID != nil {
			parent[v.Id] = *v.ParentID
		}
	}
	rootCache := map[string]string{}
	var rootOf func(string) string
	rootOf = func(id string) string {
		if id == "" {
			return ""
		}
		if r, ok := rootCache[id]; ok {
			return r
		}
		p, ok := parent[id]
		if !ok || p == "" || p == id {
			rootCache[id] = id
			return id
		}
		r := rootOf(p)
		rootCache[id] = r
		return r
	}

	// Load tool and model calls
	tcOut := &tcread.Output{}
	_, _ = s.dao.Operate(ctx,
		datly.WithOutput(tcOut),
		datly.WithURI(strings.ReplaceAll(tcread.PathByConversation, "{conversationId}", conversationID)),
		datly.WithInput(&tcread.Input{ConversationID: conversationID, Has: &tcread.Has{ConversationID: true}}),
	)
	mcOut := &mcread.Output{}
	_, _ = s.dao.Operate(ctx,
		datly.WithOutput(mcOut),
		datly.WithURI(strings.ReplaceAll(mcread.PathByConversation, "{conversationId}", conversationID)),
		datly.WithInput(&mcread.Input{ConversationID: conversationID, Has: &mcread.Has{ConversationID: true}}),
	)

	// Compute earliest tool start per turn
	minToolStart := map[string]time.Time{}
	for _, v := range tcOut.Data {
		if v == nil || v.TurnID == nil || *v.TurnID == "" || v.StartedAt == nil {
			continue
		}
		tid := *v.TurnID
		if cur, ok := minToolStart[tid]; !ok || v.StartedAt.Before(cur) {
			minToolStart[tid] = *v.StartedAt
		}
	}

	type callStep struct {
		rootID    string
		outcomeID string
		step      *plan.StepOutcome
		started   *time.Time
		completed *time.Time
		phase     int
	}
	var steps []*callStep

	// No $ref JSON; StepOutcome carries payload IDs for lazy resolution.

	// Tool calls → callStep
	for _, c := range tcOut.Data {
		if c == nil {
			continue
		}
		rid := rootOf(c.MessageID)
		step := &plan.StepOutcome{ID: c.OpID + "-0", Name: c.ToolName}
		step.Success = strings.ToLower(c.Status) == "completed"
		if c.ErrorMessage != nil {
			step.Error = *c.ErrorMessage
		}
		if c.RequestPayloadID != nil {
			step.RequestPayloadID = c.RequestPayloadID
		}
		if c.ResponsePayloadID != nil {
			step.ResponsePayloadID = c.ResponsePayloadID
		}
		step.StartedAt = c.StartedAt
		step.EndedAt = c.CompletedAt
		steps = append(steps, &callStep{rootID: rid, outcomeID: c.OpID, step: step, started: c.StartedAt, completed: c.CompletedAt, phase: 1})
	}
	// Model calls → callStep with phase based on minToolStart
	for _, c := range mcOut.Data {
		if c == nil {
			continue
		}
		rid := rootOf(c.MessageID)
		phase := 0
		if c.TurnID != nil {
			if t0, ok := minToolStart[*c.TurnID]; ok && c.StartedAt != nil {
				if !c.StartedAt.Before(t0) {
					phase = 2
				} else {
					phase = 0
				}
			}
		}
		finish := ""
		if c.FinishReason != nil {
			finish = *c.FinishReason
		}
		opID := "mc:" + c.MessageID
		if c.StartedAt != nil {
			opID += ":" + strconv.FormatInt(c.StartedAt.UnixNano(), 10)
		} else if c.TraceID != nil && *c.TraceID != "" {
			opID += ":" + *c.TraceID
		}
		step := &plan.StepOutcome{ID: opID + "-0", Name: "llm:" + c.Provider + "/" + c.Model}
		step.Success = strings.ToLower(c.Status) == "completed"
		if finish != "" {
			step.Reason = finish
		}
		if c.RequestPayloadID != nil {
			step.RequestPayloadID = c.RequestPayloadID
		}
		if c.ResponsePayloadID != nil {
			step.ResponsePayloadID = c.ResponsePayloadID
		}
		step.StartedAt = c.StartedAt
		step.EndedAt = c.CompletedAt
		steps = append(steps, &callStep{rootID: rid, outcomeID: opID, step: step, started: c.StartedAt, completed: c.CompletedAt, phase: phase})
	}

	// Sort steps similar to adapter logic
	sort.SliceStable(steps, func(i, j int) bool {
		si, sj := steps[i].started, steps[j].started
		if si == nil && sj == nil {
			if steps[i].phase != steps[j].phase {
				return steps[i].phase < steps[j].phase
			}
			if steps[i].completed == nil || steps[j].completed == nil {
				return i < j
			}
			if steps[i].completed.Equal(*steps[j].completed) {
				return i < j
			}
			return steps[i].completed.Before(*steps[j].completed)
		}
		if si == nil {
			return false
		}
		if sj == nil {
			return true
		}
		if si.Equal(*sj) {
			if steps[i].phase != steps[j].phase {
				return steps[i].phase < steps[j].phase
			}
			if steps[i].completed == nil || steps[j].completed == nil {
				return i < j
			}
			if steps[i].completed.Equal(*steps[j].completed) {
				return i < j
			}
			return steps[i].completed.Before(*steps[j].completed)
		}
		return si.Before(*sj)
	})

	outcomesByRoot := map[string][]*plan.Outcome{}
	for idx, cs := range steps {
		cs.step.TraceID = idx + 1
		if cs.started != nil {
			t := cs.started.Local()
			cs.step.StartedAt = &t
		}
		if cs.completed != nil {
			t2 := cs.completed.Local()
			cs.step.EndedAt = &t2
			if cs.started != nil {
				cs.step.Elapsed = cs.completed.Sub(*cs.started).Truncate(time.Millisecond).String()
			}
		}
		outc := &plan.Outcome{ID: cs.outcomeID, Steps: []*plan.StepOutcome{cs.step}}
		outcomesByRoot[cs.rootID] = append(outcomesByRoot[cs.rootID], outc)
	}
	return outcomesByRoot, nil
}

// List returns messages using input options.
func (s *Service) List(ctx context.Context, opts ...read2.InputOption) ([]*read2.MessageView, error) {
	in := &read2.Input{}
	for _, opt := range opts {
		opt(in)
	}
	out := &read2.Output{}
	// prefer path with conversation when provided (predicates still apply)
	if in.Has != nil && in.Has.ConversationID && in.ConversationID != "" {
		uri := strings.ReplaceAll(read2.PathByConversation, "{conversationId}", in.ConversationID)
		_, err := s.dao.Operate(ctx, datly.WithOutput(out), datly.WithURI(uri), datly.WithInput(in))
		if err != nil {
			return nil, err
		}
		return out.Data, nil
	}
	_, err := s.dao.Operate(ctx, datly.WithOutput(out), datly.WithURI(read2.PathBase), datly.WithInput(in))
	if err != nil {
		return nil, err
	}
	return out.Data, nil
}

// GetTranscript returns transcript for a given conversation and turn.
// Includes roles: user, assistant, tool. Excludes control and interim by default.
// Tool messages are de-duplicated by op_id keeping the latest attempt.
func (s *Service) GetTranscript(ctx context.Context, conversationID, turnID string, opts ...read2.InputOption) ([]*read2.MessageView, error) {
	in := &read2.Input{ConversationID: conversationID, TurnID: turnID, Has: &read2.Has{ConversationID: true}}
	if turnID != "" {
		in.Has.TurnID = true
	}
	for _, opt := range opts {
		opt(in)
	}
	out := &read2.Output{}
	uri := strings.ReplaceAll(read2.PathByConversation, "{conversationId}", conversationID)
	_, err := s.dao.Operate(ctx, datly.WithOutput(out), datly.WithURI(uri), datly.WithInput(in))
	if err == nil && out.Status.Status == "error" {
		err = fmt.Errorf("transcript error: %s", out.Status.Message)
	}
	if err != nil {
		return nil, err
	}
	// Normalize transcript with shared logic
	rows := shared.BuildTranscript(out.Data, true)

	// Optional: backfill elicitation inline JSON when requested
	if in.ElicitInline {
		for _, v := range rows {
			if v == nil || v.ElicitationID == nil || *v.ElicitationID == "" {
				continue
			}
			pout := &pldaoRead.Output{}
			_, perr := s.dao.Operate(ctx,
				datly.WithURI(pldaoRead.PathBase+"?id="+*v.ElicitationID),
				datly.WithOutput(pout),
				datly.WithInput(&pldaoRead.Input{Id: *v.ElicitationID, Has: &pldaoRead.Has{Id: true}}),
			)
			if perr != nil || len(pout.Data) == 0 || pout.Data[0] == nil {
				continue
			}
			pv := pout.Data[0]
			if pv.InlineBody != nil {
				s := string(*pv.InlineBody)
				v.ElicitationJSON = &s
			} else if pv.Preview != nil {
				s := string(*pv.Preview)
				v.ElicitationJSON = &s
			}
			// decode typed elicitation when available
			if v.ElicitationJSON != nil && *v.ElicitationJSON != "" {
				var el plan.Elicitation
				if json.Unmarshal([]byte(*v.ElicitationJSON), &el) == nil {
					v.Elicitation = &el
				}
			}
		}
	}

	// Backfill ParentID alias for consumers
	// No-op: ParentID is direct-mapped via sqlx

	// Optional: slice by SinceID (inclusive) when provided.
	if in.SinceID != "" {
		start := -1
		for i, v := range rows {
			if v != nil && v.Id == in.SinceID {
				start = i
				break
			}
		}
		if start >= 0 {
			rows = rows[start:]
		} else {
			rows = rows[:0]
		}
	}

	// Optional: aggregate tool/model outcomes and attach to root messages
	if in.IncludeOutcomes && len(rows) > 0 {
		outcomesByRoot, _ := s.aggregateExecutions(ctx, conversationID, uri, in)
		var updatedRow []*read2.MessageView
		for _, v := range rows {
			if v == nil {
				continue
			}
			if oc := outcomesByRoot[v.Id]; len(oc) > 0 {
				v.Executions = oc
			}
			if in.FlattenExecutions && v.Role == "tool" {
				continue
			}
			updatedRow = append(updatedRow, v)
		}
		rows = updatedRow

		// Optionally flatten executions into synthetic tool messages and clear root executions
		if in.FlattenExecutions {
			flat := make([]*read2.MessageView, 0, len(rows))
			flat = append(flat, rows...)
			for _, v := range rows {
				if v == nil || len(v.Executions) == 0 {
					continue
				}
				tv := &read2.MessageView{Id: v.Id + "/1", ConversationID: v.ConversationID, Role: "tool", Executions: v.Executions}
				if v.ParentID != nil {
					tv.ParentID = v.ParentID
				} else {
					tv.ParentID = &v.Id
				}
				if v.CreatedAt != nil {
					t := v.CreatedAt.Add(time.Second)
					tv.CreatedAt = &t
				} else {
					t := time.Now()
					tv.CreatedAt = &t
				}
				flat = append(flat, tv)
				v.Executions = nil
			}
			rows = flat
		}
		// Optional filter by parent root id
		if strings.TrimSpace(in.ParentRoot) != "" {
			root := strings.TrimSpace(in.ParentRoot)
			filtered := make([]*read2.MessageView, 0, len(rows))
			for _, v := range rows {
				if v == nil {
					continue
				}
				if v.Id == root {
					filtered = append(filtered, v)
					continue
				}
				if v.ParentID != nil && *v.ParentID == root {
					filtered = append(filtered, v)
				}
			}
			rows = filtered
		}
	}

	sort.SliceStable(rows, func(i, j int) bool {
		li, lj := rows[i], rows[j]
		if li.CreatedAt != nil && lj.CreatedAt != nil {
			return li.CreatedAt.Before(*lj.CreatedAt)
		}
		return i < j
	})

	return rows, nil
}

// GetConversation returns assistant/user messages for a conversation (no tool messages).
// Interim messages are excluded by default. Additional filters can be provided via InputOption.
func (s *Service) GetConversation(ctx context.Context, conversationID string, opts ...read2.InputOption) ([]*read2.MessageView, error) {
	in := &read2.Input{ConversationID: conversationID, Has: &read2.Has{ConversationID: true}}
	for _, opt := range opts {
		opt(in)
	}
	out := &read2.Output{}
	uri := strings.ReplaceAll(read2.PathByConversation, "{conversationId}", conversationID)
	_, err := s.dao.Operate(ctx, datly.WithOutput(out), datly.WithURI(uri), datly.WithInput(in))
	if err != nil {
		return nil, err
	}

	// filter to user/assistant only, exclude control and interim by default
	var filtered []*read2.MessageView
	for _, m := range out.Data {
		if m == nil {
			continue
		}
		if m.Type == "control" {
			continue
		}
		if m.IsInterim() {
			continue
		}
		if m.Role == "user" || m.Role == "assistant" {
			filtered = append(filtered, m)
		}
	}
	// sort by created_at asc
	sort.SliceStable(filtered, func(i, j int) bool {
		li, lj := filtered[i], filtered[j]
		if li.CreatedAt != nil && lj.CreatedAt != nil {
			return li.CreatedAt.Before(*lj.CreatedAt)
		}
		return i < j
	})
	return filtered, nil
}

// Patch upserts messages via write component
func (s *Service) Patch(ctx context.Context, messages ...*write.Message) (*write.Output, error) {
	in := &write.Input{Messages: messages}
	out := &write.Output{}
	_, err := s.dao.Operate(ctx,
		datly.WithPath(contract.NewPath("PATCH", write.PathURI)),
		datly.WithInput(in),
		datly.WithOutput(out),
	)
	if err != nil {
		return nil, err
	}
	return out, nil
}

// Re-exports for ergonomics
type InputOption = read2.InputOption
type MessageView = read2.MessageView

func WithConversationID(id string) read2.InputOption { return read2.WithConversationID(id) }
func WithID(id string) read2.InputOption             { return read2.WithID(id) }
func WithIDs(ids ...string) read2.InputOption        { return read2.WithIDs(ids...) }
func WithRole(role string) read2.InputOption         { return read2.WithRoles(role) }
func WithType(typ string) read2.InputOption          { return read2.WithType(typ) }
func WithInterim(values ...int) read2.InputOption    { return read2.WithInterim(values...) }
func WithElicitationID(id string) read2.InputOption  { return read2.WithElicitationID(id) }
func WithTurnID(id string) read2.InputOption         { return read2.WithTurnID(id) }
func WithSince(ts time.Time) read2.InputOption       { return read2.WithSince(ts) }
