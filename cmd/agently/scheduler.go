package agently

// SchedulerCmd groups scheduler-related subcommands.
type SchedulerCmd struct {
	Run SchedulerRunCmd `command:"run" description:"Run the scheduler watchdog loop"`
}
