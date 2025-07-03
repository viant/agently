package memory

import "context"

// Policy defines a filter over conversation messages.
// It may transform, filter, or summarize messages and return the set to include.
type Policy interface {
	// Apply processes messages, potentially invoking summarization, returning filtered messages or an error.
	Apply(ctx context.Context, messages []Message) ([]Message, error)
}

// LastNPolicy keeps only the last N messages.
type LastNPolicy struct {
	N int
}

// NewLastNPolicy creates a policy that retains the last N messages.
func NewLastNPolicy(n int) *LastNPolicy {
	return &LastNPolicy{N: n}
}

// Apply filters messages to the last N entries.
func (p *LastNPolicy) Apply(_ context.Context, messages []Message) ([]Message, error) {
	count := len(messages)
	if p.N <= 0 || count <= p.N {
		return messages, nil
	}
	return messages[count-p.N:], nil
}

// RolePolicy includes only messages whose Role matches one of the allowed roles.
type RolePolicy struct {
	allowed map[string]bool
}

// NewRolePolicy creates a policy that retains messages with specified roles.
func NewRolePolicy(roles ...string) *RolePolicy {
	m := make(map[string]bool)
	for _, role := range roles {
		m[role] = true
	}
	return &RolePolicy{allowed: m}
}

// Apply filters messages by role.
func (p *RolePolicy) Apply(_ context.Context, messages []Message) ([]Message, error) {
	var result []Message
	for _, msg := range messages {
		if p.allowed[msg.Role] {
			result = append(result, msg)
		}
	}
	return result, nil
}

// CombinedPolicy composes multiple policies in order.
type CombinedPolicy struct {
	Policies []Policy
}

// NewCombinedPolicy creates a policy that applies all given policies sequentially.
func NewCombinedPolicy(policies ...Policy) *CombinedPolicy {
	return &CombinedPolicy{Policies: policies}
}

// Apply runs each policy in sequence.
func (c *CombinedPolicy) Apply(ctx context.Context, messages []Message) ([]Message, error) {
	var err error
	for _, p := range c.Policies {
		messages, err = p.Apply(ctx, messages)
		if err != nil {
			return nil, err
		}
	}
	return messages, nil
}

// SummarizerFunc defines a function that summarizes a slice of messages.
// It should return a single Message containing the summary.
type SummarizerFunc func(ctx context.Context, messages []Message) (Message, error)

// SummaryPolicy summarizes older messages when count exceeds Threshold.
// It replaces the earliest messages with a single summary Message.
type SummaryPolicy struct {
	Threshold  int
	Summarizer SummarizerFunc
}

// --------------------------------------------------------------------
// Status-based filter policies
// --------------------------------------------------------------------

// statusPolicy filters messages depending on their Status value. When include
// is non-empty, only messages whose Status is present in the map are kept.
// When exclude is non-empty those statuses are removed. If both maps are
// empty Apply returns the original slice unchanged.
type statusPolicy struct {
	include map[string]struct{}
	exclude map[string]struct{}
}

func (p *statusPolicy) Apply(_ context.Context, messages []Message) ([]Message, error) {
	// Fast paths – nothing to filter
	if len(p.include) == 0 && len(p.exclude) == 0 {
		return messages, nil
	}
	var out []Message
	for _, m := range messages {
		if len(p.include) > 0 {
			if _, ok := p.include[m.Status]; !ok {
				continue
			}
		}
		if len(p.exclude) > 0 {
			if _, ok := p.exclude[m.Status]; ok {
				continue
			}
		}
		out = append(out, m)
	}
	return out, nil
}

// SkipSummariesPolicy returns a Policy that removes messages flagged with
// Status=="summary" or "summarized".
func SkipSummariesPolicy() Policy {
	return &statusPolicy{exclude: map[string]struct{}{"summary": {}, "summarized": {}}}
}

// OnlySummariesPolicy keeps only messages whose Status is "summary".
func OnlySummariesPolicy() Policy {
	return &statusPolicy{include: map[string]struct{}{"summary": {}}}
}

// NewSummaryPolicy creates a policy that summarizes messages when count > threshold.
func NewSummaryPolicy(threshold int, summarizer SummarizerFunc) *SummaryPolicy {
	return &SummaryPolicy{Threshold: threshold, Summarizer: summarizer}
}

// Apply applies summarization: if message count > Threshold, summarizes the oldest
// messages and prepends the summary before the remaining Threshold messages.
func (p *SummaryPolicy) Apply(ctx context.Context, messages []Message) ([]Message, error) {
	count := len(messages)
	if p.Threshold <= 0 || count <= p.Threshold {
		return messages, nil
	}
	// Older messages to summarize
	toSum := messages[:count-p.Threshold]
	rest := messages[count-p.Threshold:]
	summary, err := p.Summarizer(ctx, toSum)
	if err != nil {
		return nil, err
	}
	// Prepend summary message
	result := make([]Message, 0, 1+len(rest))
	result = append(result, summary)
	result = append(result, rest...)
	return result, nil
}

// TokenEstimatorFunc estimates token count for a Message.
type TokenEstimatorFunc func(Message) int

// NextTokenPolicy triggers summarization if total estimated tokens exceed Threshold.
// It keeps the last Keep messages and replaces earlier ones with a summary.
type NextTokenPolicy struct {
	Threshold  int
	Keep       int
	Summarizer SummarizerFunc
	Estimator  TokenEstimatorFunc
}

// NewNextTokenPolicy creates a policy that summarizes if tokens > threshold.
// If estimator is nil, a default estimator (len(Result)/4) is used.
func NewNextTokenPolicy(threshold, keep int, summarizer SummarizerFunc, estimator TokenEstimatorFunc) *NextTokenPolicy {
	if estimator == nil {
		estimator = func(m Message) int { return len(m.Content) / 4 }
	}
	return &NextTokenPolicy{Threshold: threshold, Keep: keep, Summarizer: summarizer, Estimator: estimator}
}

// Apply applies token-based summarization: if total tokens <= Threshold returns messages,
// else summarizes oldest messages and prepends summary before last Keep messages.
func (p *NextTokenPolicy) Apply(ctx context.Context, messages []Message) ([]Message, error) {
	total := 0
	for _, m := range messages {
		total += p.Estimator(m)
	}
	if p.Threshold <= 0 || total <= p.Threshold {
		return messages, nil
	}
	count := len(messages)
	if p.Keep < 0 || p.Keep > count {
		p.Keep = count
	}
	toSum := messages[:count-p.Keep]
	rest := messages[count-p.Keep:]
	summary, err := p.Summarizer(ctx, toSum)
	if err != nil {
		return nil, err
	}
	result := make([]Message, 0, 1+len(rest))
	result = append(result, summary)
	result = append(result, rest...)
	return result, nil
}
