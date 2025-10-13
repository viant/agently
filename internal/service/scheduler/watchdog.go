package scheduler

import (
	"context"
	"time"

	schapi "github.com/viant/agently/client/scheduler"
)

// Watchdog encapsulates a background ticker that periodically invokes
// scheduler.Client.RunDue to trigger due runs. Call Stop to cancel.
type Watchdog struct {
	stop context.CancelFunc
	// Errors receives errors encountered during RunDue calls. It is buffered
	// and non-blocking; consumers may choose to drain it for diagnostics.
	Errors chan error
}

// StartWatchdog launches a background goroutine that calls RunDue on the provided
// scheduler client at the specified interval. If interval <= 0, a default of 30s is used.
// The returned Watchdog can be stopped via Stop().
func StartWatchdog(parent context.Context, client schapi.Client, interval time.Duration) *Watchdog {
	if client == nil {
		return nil
	}
	if interval <= 0 {
		interval = 30 * time.Second
	}
	ctx, cancel := context.WithCancel(parent)
	wd := &Watchdog{stop: cancel, Errors: make(chan error, 4)}
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		// First tick shortly after start to reduce perceived latency
		timer := time.NewTimer(2 * time.Second)
		defer timer.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-timer.C:
				if _, err := client.RunDue(context.Background()); err != nil {
					select {
					case wd.Errors <- err:
					default:
					}
				}
			case <-ticker.C:
				if _, err := client.RunDue(context.Background()); err != nil {
					select {
					case wd.Errors <- err:
					default:
					}
				}
			}
		}
	}()
	return wd
}

// Stop cancels the watchdog loop.
func (w *Watchdog) Stop() {
	if w != nil && w.stop != nil {
		w.stop()
	}
}
