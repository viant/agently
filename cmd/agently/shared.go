package agently

import (
	"bufio"
	"context"
	"fmt"
	"log"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/viant/agently/genai/executor"
	"github.com/viant/agently/genai/executor/instance"
	"github.com/viant/agently/genai/tool"
	fluxpol "github.com/viant/fluxor/policy"
	"github.com/viant/fluxor/service/approval"
)

var (
	cfgMu   sync.RWMutex
	cfgPath string

	execOptsMu sync.Mutex
	execOpts   []executor.Option
)

// executorSingleton initialises global executor only once to speed up CLI.
func executorSingleton() *executor.Service {
	// Ensure singleton is initialised once.
	cfgMu.RLock()
	path := cfgPath
	cfgMu.RUnlock()

	if instance.Get() == nil {
		execOptsMu.Lock()
		opts := append([]executor.Option(nil), execOpts...)
		execOptsMu.Unlock()

		if err := instance.Init(context.Background(), path, opts...); err != nil {
			log.Fatalf("executor init error: %v", err)
		}
	}
	return instance.Get()
}

// called from CLI after flag parsing
func setConfigPath(p string) {
	cfgMu.Lock()
	cfgPath = p
	cfgMu.Unlock()
}

// registerExecOption appends an option that will be passed to executor.New on
// first initialisation.
func registerExecOption(o executor.Option) {
	execOptsMu.Lock()
	execOpts = append(execOpts, o)
	execOptsMu.Unlock()
}

// Helper ---------------------------------------------------------------

func buildPolicy(mode string) *tool.Policy {
	switch strings.ToLower(mode) {
	case tool.ModeDeny:
		return &tool.Policy{Mode: tool.ModeDeny}
	case tool.ModeAsk:
		return &tool.Policy{Mode: tool.ModeAsk, Ask: stdinAsk}
	default:
		return &tool.Policy{Mode: tool.ModeAuto}
	}
}

// buildFluxorPolicy mirrors buildPolicy but returns a *fluxpol.Policy used by
// the workflow engine approval layer.
func buildFluxorPolicy(mode string) *fluxpol.Policy {
	switch strings.ToLower(mode) {
	case fluxpol.ModeDeny:
		return &fluxpol.Policy{Mode: fluxpol.ModeDeny}
	case fluxpol.ModeAsk:
		return &fluxpol.Policy{Mode: fluxpol.ModeAsk}
	default:
		return &fluxpol.Policy{Mode: fluxpol.ModeAuto}
	}
}

// stdinAsk is used when Policy.Mode==ask to prompt user.
func stdinAsk(_ context.Context, name string, args map[string]interface{}, p *tool.Policy) bool {
	reader := bufio.NewReader(os.Stdin)
	fmt.Printf("Execute tool %s with args %v? [y/n/all] ", name, args)
	line, _ := reader.ReadString('\n')
	line = strings.ToLower(strings.TrimSpace(line))
	switch line {
	case "y", "yes":
		return true
	case "all":
		p.Mode = tool.ModeAuto
		return true
	default:
		return false
	}
}

// ---------------------------------------------------------------------------
// Fluxor approval interactive helper
// ---------------------------------------------------------------------------

// startApprovalLoop launches a goroutine that consumes approval requests from
// the executor's Fluxor approval queue and prompts the user for a decision via
// stdin. It returns a stop function that can be awaited to ensure the goroutine
// exits after ctx is cancelled.
func startApprovalLoop(ctx context.Context, execSvc *executor.Service, pol *fluxpol.Policy) (stop func()) {
	if pol == nil || strings.ToLower(pol.Mode) != fluxpol.ModeAsk {
		return func() {}
	}

	// Wait until ApprovalService is initialised (executor boots asynchronously)
	var appr approval.Service
	for appr == nil {
		select {
		case <-ctx.Done():
			return func() {}
		default:
		}
		appr = execSvc.ApprovalService()
		if appr == nil {
			time.Sleep(20 * time.Millisecond)
		}
	}

	decided := make(map[string]bool)

	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			select {
			case <-ctx.Done():
				return
			default:
			}

			msg, err := appr.Queue().Consume(ctx)
			if err != nil {
				continue // retry unless ctx cancelled
			}

			evt := msg.T()
			if evt == nil {
				_ = msg.Ack()
				continue
			}

			if evt.Topic != approval.TopicRequestCreated && evt.Topic != approval.LegacyTopicRequestNew {
				_ = msg.Ack()
				continue
			}

			req, ok := evt.Data.(*approval.Request)
			if !ok {
				_ = msg.Ack()
				continue
			}

			if decided[req.ID] {
				_ = msg.Ack()
				continue
			}

			approved, reason := promptApproval(req)

			if _, err := appr.Decide(ctx, req.ID, approved, reason); err == nil {
				decided[req.ID] = true
			}

			_ = msg.Ack()
		}
	}()

	return func() { <-done }
}

// promptApproval asks the user to approve or reject the supplied request. It
// returns (approved, reason).
func promptApproval(req *approval.Request) (bool, string) {
	fmt.Printf("Approve action %s with args %s? [y/n] ", req.Action, string(req.Args))
	reader := bufio.NewReader(os.Stdin)
	line, _ := reader.ReadString('\n')
	line = strings.ToLower(strings.TrimSpace(line))
	switch line {
	case "y", "yes":
		return true, ""
	default:
		fmt.Print("reason: ")
		reason, _ := reader.ReadString('\n')
		reason = strings.TrimSpace(reason)
		return false, reason
	}
}

// withFluxorPolicy embeds a copy of the tool.Policy data into the context
// using the fluxor policy key so that the workflow engine can access the same
// mode/allow/block settings.
func withFluxorPolicy(ctx context.Context, pol *fluxpol.Policy) context.Context {
	if pol == nil {
		return ctx
	}
	return fluxpol.WithPolicy(ctx, pol)
}
