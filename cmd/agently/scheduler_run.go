package agently

import (
	"time"

	root "github.com/viant/agently"
)

// SchedulerRunCmd starts the scheduler watchdog loop as a dedicated process.
// Usage: agently scheduler run --interval 30s
type SchedulerRunCmd struct {
	Interval string `long:"interval" description:"RunDue polling interval (e.g. 30s, 1m)" default:"30s"`
	Once     bool   `long:"once" description:"Run one RunDue cycle and exit"`
}

func (s *SchedulerRunCmd) Execute(_ []string) error {
	interval := 30 * time.Second
	if s.Interval != "" {
		parsed, err := time.ParseDuration(s.Interval)
		if err != nil {
			return err
		}
		if parsed > 0 {
			interval = parsed
		}
	}
	return root.RunScheduler(root.SchedulerRunOptions{
		Interval: interval,
		Once:     s.Once,
	})
}
