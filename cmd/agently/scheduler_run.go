package agently

import (
	"time"

	root "github.com/viant/agently"
)

// SchedulerRunCmd starts the scheduler watchdog loop as a dedicated process.
// Usage: agently scheduler run --interval 30s
type SchedulerRunCmd struct {
	Interval          string `long:"interval" description:"RunDue polling interval (e.g. 30s, 1m)" default:"30s"`
	Once              bool   `long:"once" description:"Run one RunDue cycle and exit"`
	ScratchpadRootURI string `short:"s" long:"scratchpad-root-uri" description:"User-scoped scratchpad URI template (overrides AGENTLY_SCRATCHPAD_URI when set)"`
}

func (s *SchedulerRunCmd) Execute(_ []string) error {
	options, err := s.schedulerRunOptions()
	if err != nil {
		return err
	}
	return root.RunScheduler(options)
}

func (s *SchedulerRunCmd) schedulerRunOptions() (root.SchedulerRunOptions, error) {
	interval := 30 * time.Second
	if s.Interval != "" {
		parsed, err := time.ParseDuration(s.Interval)
		if err != nil {
			return root.SchedulerRunOptions{}, err
		}
		if parsed > 0 {
			interval = parsed
		}
	}
	return root.SchedulerRunOptions{
		Interval:          interval,
		Once:              s.Once,
		ScratchpadRootURI: s.ScratchpadRootURI,
	}, nil
}
