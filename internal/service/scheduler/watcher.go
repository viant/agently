package scheduler

import (
	"context"
	"fmt"
	"log"

	apiconv "github.com/viant/agently/client/conversation"
	schapi "github.com/viant/agently/client/scheduler"
	agconv "github.com/viant/agently/pkg/agently/conversation"
	turnread "github.com/viant/agently/pkg/agently/turn/read"
	"github.com/viant/datly"
	"github.com/viant/datly/repository/contract"

	"strings"
	"time"
)

const (
	pollEvery                = 3 * time.Second
	callTimeout              = 5 * time.Second
	precheckTimeout          = 5 * time.Second
	getConvTranscriptTimeout = 5 * time.Second
	watchTimeout             = 20 * time.Minute
)

// watchRunCompletion polls conversation stage until completion and updates the run status.
func (s *Service) watchRunCompletion(ctx context.Context, runID, scheduleID, conversationID string, timeoutSeconds int, runName string) {
	// NOTE: Callers pass ctx as context.WithoutCancel(originalCtx).
	// That means:
	//   - ctx carries request-scoped values (trace IDs, auth, etc.)
	//   - ctx has NO cancellation and NO deadline (Done() == nil)
	// We intentionally use ctx only for Value() propagation + per-call timeouts below.
	if s == nil || s.sch == nil {
		return
	}
	s.ensureLeaseConfig()
	if s.conv == nil {
		return
	}

	debugf(
		"watchRunCompletion start schedule_id=%q schedule_name=%q run_id=%q conversation_id=%q timeout_seconds=%d lease_owner=%q lease_ttl=%s poll_every=%s",
		strings.TrimSpace(scheduleID),
		strings.TrimSpace(runName),
		strings.TrimSpace(runID),
		strings.TrimSpace(conversationID),
		timeoutSeconds,
		strings.TrimSpace(s.leaseOwner),
		s.leaseTTL,
		pollEvery,
	)

	heartbeatEvery := s.leaseTTL / 2
	if heartbeatEvery < pollEvery {
		heartbeatEvery = pollEvery
	}

	// Claim initial run lease (best-effort). If another instance owns it, stop.
	if strings.TrimSpace(s.leaseOwner) != "" {
		callCtx, callCancel := context.WithTimeout(ctx, callTimeout)
		claimed, err := s.sch.TryClaimRun(callCtx, strings.TrimSpace(runID), strings.TrimSpace(s.leaseOwner), time.Now().UTC().Add(s.leaseTTL))
		callCancel()
		if err != nil {
			debugf("watchRunCompletion initial run lease claim error schedule_id=%q run_id=%q owner=%q err=%v", strings.TrimSpace(scheduleID), strings.TrimSpace(runID), strings.TrimSpace(s.leaseOwner), err)
		} else {
			debugf("watchRunCompletion initial run lease claim schedule_id=%q run_id=%q owner=%q claimed=%v", strings.TrimSpace(scheduleID), strings.TrimSpace(runID), strings.TrimSpace(s.leaseOwner), claimed)
		}
		if err == nil && !claimed {
			debugf("watchRunCompletion stop: run lease owned by another instance schedule_id=%q run_id=%q owner=%q", strings.TrimSpace(scheduleID), strings.TrimSpace(runID), strings.TrimSpace(s.leaseOwner))
			return
		}
	}

	nextHeartbeatAt := time.Now().UTC().Add(heartbeatEvery)

	// Hard limit for *starting new attempts* in this watcher.
	// We intentionally base this on Background() so it is independent of the caller's ctx
	// (caller cancellation is already stripped by context.WithoutCancel anyway).
	var timeout time.Duration
	if timeoutSeconds <= 0 {
		timeout = watchTimeout // default 10 minutes
	} else {
		timeout = time.Duration(timeoutSeconds) * time.Second
	}

	allCtx, allCancel := context.WithTimeout(context.Background(), timeout)
	defer allCancel()

	// DAO provider (used for cheap "turn in progress" precheck).
	var dao *datly.Service
	type daoProvider interface{ DAO() *datly.Service }
	if s.chat != nil {
		if dp, ok := s.chat.(daoProvider); ok {
			dao = dp.DAO()
		}
	}

	ticker := time.NewTicker(pollEvery)
	defer ticker.Stop()

	var err error
	for {
		select {
		case <-allCtx.Done():
			// Stop polling after watchTimeout
			s.finalizeDeadline(ctx, runID, scheduleID, conversationID, callTimeout, err, timeout)
			return

		case <-ticker.C:
			// Heartbeat: renew the run lease so other scheduler instances can detect liveness.
			// If we fail to renew because another instance took over, stop this watcher.
			if strings.TrimSpace(s.leaseOwner) != "" {
				now := time.Now().UTC()
				if !now.Before(nextHeartbeatAt) {
					debugf("watchRunCompletion heartbeat schedule_id=%q schedule_name=%q run_id=%q owner=%q", strings.TrimSpace(scheduleID), strings.TrimSpace(runName), strings.TrimSpace(runID), strings.TrimSpace(s.leaseOwner))
					callCtx, callCancel := context.WithTimeout(ctx, callTimeout)
					claimed, err := s.sch.TryClaimRun(callCtx, strings.TrimSpace(runID), strings.TrimSpace(s.leaseOwner), now.Add(s.leaseTTL))
					callCancel()
					if err == nil && !claimed {
						debugf("watchRunCompletion stop: lost run lease schedule_id=%q run_id=%q owner=%q", strings.TrimSpace(scheduleID), strings.TrimSpace(runID), strings.TrimSpace(s.leaseOwner))
						return
					}
					if err == nil && claimed {
						nextHeartbeatAt = now.Add(heartbeatEvery)
						debugf("watchRunCompletion heartbeat ok schedule_id=%q run_id=%q next_heartbeat_at=%s", strings.TrimSpace(scheduleID), strings.TrimSpace(runID), nextHeartbeatAt.UTC().Format(time.RFC3339Nano))
					} else if err != nil {
						debugf("watchRunCompletion heartbeat error schedule_id=%q run_id=%q err=%v", strings.TrimSpace(scheduleID), strings.TrimSpace(runID), err)
						// Retry sooner on transient errors while still avoiding per-tick churn.
						nextHeartbeatAt = now.Add(pollEvery)
					}
				}
			}

			// Cheap precheck: if there is any active or queued turn, the conversation is still in progress.
			// Skip full transcript load in this case.
			if dao != nil {
				preCtx, preCancel := context.WithTimeout(ctx, precheckTimeout)
				inProgress, pErr := turnInProgress(preCtx, dao, conversationID)
				preCancel()

				if pErr == nil && inProgress {
					continue
				}
			}

			// Per-tick call budget (prevents a single slow/hung call from blocking the loop forever).
			// We derive from ctx (which is WithoutCancel) so we keep ctx.Values, but we impose a 5s timeout.
			// This timeout is independent of allCtx, by design, so the "last attempt" may complete
			// even if we're near/over the watchTimeout polling window.
			callGetConvTransCtx, callGetConvTransCtxCancel := context.WithTimeout(ctx, getConvTranscriptTimeout)

			conv, err := s.getConversationWithTranscript(callGetConvTransCtx, conversationID, includeTranscript)
			if err != nil {
				debugf("watchRunCompletion get conversation error schedule_id=%q run_id=%q conversation_id=%q err=%v", strings.TrimSpace(scheduleID), strings.TrimSpace(runID), strings.TrimSpace(conversationID), err)
				callGetConvTransCtxCancel()
				continue
			}

			stage := normalizeStage(conv)

			// Running stages: keep polling until stage leaves these values or we hit allCtx deadline.
			if isRunningStage(stage) {
				debugf("watchRunCompletion still running schedule_id=%q run_id=%q stage=%q", strings.TrimSpace(scheduleID), strings.TrimSpace(runID), stage)
				callGetConvTransCtxCancel()
				continue
			}

			// Decide final status from terminal stage.
			status := "succeeded"
			if stage == "error" || stage == "failed" || stage == "canceled" {
				if stage == "canceled" {
					status = "skipped"
				} else {
					status = "failed"
				}
			}

			completedAt := time.Now().UTC()
			upd := &schapi.MutableRun{}
			upd.SetId(runID)
			upd.SetScheduleId(scheduleID)
			upd.SetStatus(status)
			upd.SetCompletedAt(completedAt)
			if status != "succeeded" {
				upd.SetErrorMessage(fmt.Sprintf("conversation completed with stage=%q", stage))
			}

			callCtx, callCancel := context.WithTimeout(ctx, callTimeout)
			err = s.sch.PatchRun(callCtx, upd)
			if err != nil {
				debugf("watchRunCompletion patch run error schedule_id=%q run_id=%q status=%q err=%v", strings.TrimSpace(scheduleID), strings.TrimSpace(runID), status, err)
				callCancel()
				continue
			}

			callCancel()
			if strings.TrimSpace(s.leaseOwner) != "" {
				relCtx, relCancel := context.WithTimeout(ctx, callTimeout)
				_, _ = s.sch.ReleaseRunLease(relCtx, strings.TrimSpace(runID), strings.TrimSpace(s.leaseOwner))
				relCancel()
			}
			s.patchScheduleLastResult(ctx, scheduleID, status, completedAt, upd.ErrorMessage)
			log.Printf("scheduler: run completed with status %q schedule_id=%q run_id=%q conversation_id=%q stage=%q", status, scheduleID, runID, conversationID, stage)
			return
		}
	}
}

func (s *Service) finalizeDeadline(ctx context.Context, runID string, scheduleID string, conversationID string, callTimeout time.Duration, err error, timeout time.Duration) {
	// Best-effort: one final attempt to determine conversation stage and finalize the run.

	callGetConvTransCtx, callGetConvTransCtxCancel := context.WithTimeout(ctx, getConvTranscriptTimeout)
	conv, cerr := s.getConversationWithTranscript(callGetConvTransCtx, conversationID, includeTranscript)
	callGetConvTransCtxCancel()
	// don't stop if cerr != nil; we want to capture err below too

	stage := normalizeStage(conv)

	// If the latest stage still indicates progress, treat as a timeout.
	// Try to stop the conversation best-effort and mark the run as failed with a distinct message.
	isRunning := isRunningStage(stage)

	status := statusFromStage(stage)

	completedAt := time.Now().UTC()
	upd := &schapi.MutableRun{}
	upd.SetId(runID)
	upd.SetScheduleId(scheduleID)
	upd.SetCompletedAt(completedAt)

	finalStatus := status
	if isRunning || stage == "" {
		_ = s.chat.Cancel(conversationID)
		finalStatus = "failed"
		upd.SetStatus(finalStatus)
		msg := fmt.Sprintf("conv. aborted at %q (%v timeout)", stage, timeout)
		if cerr != nil {
			msg += fmt.Sprintf(": %v", cerr)
		} else if err != nil {
			msg += fmt.Sprintf(": %v", err)
		}
		upd.SetErrorMessage(msg)
	} else {
		upd.SetStatus(finalStatus)

		if finalStatus != "succeeded" {
			_ = s.chat.Cancel(conversationID)

			if cerr != nil {
				upd.SetErrorMessage(cerr.Error())
			} else if err != nil {
				upd.SetErrorMessage(err.Error())
			} else {
				upd.SetErrorMessage(fmt.Sprintf("conversation completed with stage=%q", stage))
			}
		}
	}

	patchCtx, patchCancel := context.WithTimeout(ctx, callTimeout)
	pErr := s.sch.PatchRun(patchCtx, upd)
	patchCancel()

	if pErr != nil {
		log.Printf("scheduler: run completion finalization failed schedule_id=%q run_id=%q conversation_id=%q status=%q stage=%q timeout=%v error=%v", scheduleID, runID, conversationID, finalStatus, stage, timeout, pErr)
	} else {
		log.Printf("scheduler: run completed with status %q schedule_id=%q run_id=%q conversation_id=%q stage=%q timeout=%v", finalStatus, scheduleID, runID, conversationID, stage, timeout)
	}
	s.patchScheduleLastResult(ctx, scheduleID, finalStatus, completedAt, upd.ErrorMessage)

	if strings.TrimSpace(s.leaseOwner) != "" {
		relCtx, relCancel := context.WithTimeout(ctx, callTimeout)
		_, _ = s.sch.ReleaseRunLease(relCtx, strings.TrimSpace(runID), strings.TrimSpace(s.leaseOwner))
		relCancel()
	}
}

// Small helpers for readability (NO behavior changes).
func isRunningStage(stage string) bool {
	return stage == "executing" ||
		stage == "thinking" ||
		stage == "waiting" ||
		stage == "eliciting" ||
		stage == "elicitation"
}

func statusFromStage(stage string) string {
	status := "succeeded"
	if stage == "error" || stage == "failed" {
		status = "failed"
	} else if stage == "canceled" {
		status = "skipped"
	}
	return status
}

func normalizeStage(conv *apiconv.Conversation) string {
	if conv == nil {
		return ""
	}
	return strings.ToLower(strings.TrimSpace(conv.Stage))
}

func (s *Service) patchScheduleLastResult(ctx context.Context, scheduleID string, status string, completedAt time.Time, errMsg *string) {
	if s == nil || s.sch == nil {
		return
	}
	scheduleID = strings.TrimSpace(scheduleID)
	status = strings.TrimSpace(status)
	if scheduleID == "" || status == "" {
		return
	}

	upd := &schapi.MutableSchedule{}
	upd.SetId(scheduleID)
	if !completedAt.IsZero() {
		upd.SetLastRunAt(completedAt.UTC())
	}
	upd.SetLastStatus(status)

	if strings.EqualFold(status, "succeeded") {
		upd.LastError = nil
		if upd.Has != nil {
			upd.Has.LastError = true
		}
	} else if errMsg != nil && strings.TrimSpace(*errMsg) != "" {
		upd.SetLastError(strings.TrimSpace(*errMsg))
	} else {
		upd.SetLastError(fmt.Sprintf("run completed with status=%q", status))
	}

	callCtx, callCancel := context.WithTimeout(ctx, callTimeout)
	err := s.sch.PatchSchedule(callCtx, upd)
	callCancel()
	debugf("watchRunCompletion patch schedule last result schedule_id=%q status=%q err=%v", scheduleID, status, err)
}

// turnInProgress is a cheap precheck to avoid loading full transcript every tick.
// It returns true when any active (running/waiting_for_user) or queued turns exist.
func turnInProgress(checkCtx context.Context, dao *datly.Service, conversationID string) (bool, error) {
	if dao == nil {
		return false, nil
	}

	// 1) Active turn: running OR waiting_for_user
	activeIn := &turnread.ActiveTurnInput{
		ConversationID: conversationID,
		Has:            &turnread.ActiveTurnInputHas{ConversationID: true},
	}
	activeOut := &turnread.ActiveTurnOutput{}
	if _, err := dao.Operate(checkCtx,
		datly.WithPath(contract.NewPath("GET", turnread.ActiveTurnPathURI)),
		datly.WithInput(activeIn),
		datly.WithOutput(activeOut),
	); err != nil {
		return false, err
	}
	if len(activeOut.Data) > 0 && activeOut.Data[0] != nil {
		return true, nil
	}

	// 2) Any queued turns waiting to be executed
	queuedIn := &turnread.QueuedCountInput{
		ConversationID: conversationID,
		Has:            &turnread.QueuedCountInputHas{ConversationID: true},
	}
	queuedOut := &turnread.QueuedCountOutput{}
	if _, err := dao.Operate(checkCtx,
		datly.WithPath(contract.NewPath("GET", turnread.QueuedCountPathURI)),
		datly.WithInput(queuedIn),
		datly.WithOutput(queuedOut),
	); err != nil {
		return false, err
	}
	if len(queuedOut.Data) > 0 && queuedOut.Data[0] != nil && queuedOut.Data[0].QueuedCount > 0 {
		return true, nil
	}

	return false, nil
}

func (s *Service) getConversationWithTranscript(callCtx context.Context, conversationID string, includeTranscriptFn func(input *apiconv.Input)) (*apiconv.Conversation, error) {
	conv, err := s.conv.GetConversation(callCtx, conversationID, includeTranscriptFn)
	if err != nil {
		return nil, err
	}

	if conv == nil {
		return nil, fmt.Errorf("conversation not found by getConversationWithTranscript - conversationID: %v", conversationID)
	}

	return conv, nil
}

func includeTranscript(input *apiconv.Input) {
	if input == nil {
		return
	}
	input.IncludeTranscript = true
	if input.Has == nil {
		input.Has = &agconv.ConversationInputHas{}
	}
	input.Has.IncludeTranscript = true
}
