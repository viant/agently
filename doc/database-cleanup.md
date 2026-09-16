# Database cleanup

Agently can run bounded retention and orphan-maintenance passes against its
SQLite or MySQL store. The worker is disabled by default. Enabling the worker
does not enable any deletion policy: every policy has its own `off`, `dry-run`,
or `execute` mode.

The worker is independent of the scheduler API and scheduler runner switches.
It may run in an API instance, a scheduler instance, or both. A database lease
ensures that only one instance performs a pass at a time.

## Prerequisites

Deploy the database schema, including `maintenance_lease` and the cleanup query
indexes, before enabling the worker. Agently does not create or migrate these
objects at startup. A missing table or index is an environment/schema problem,
not a signal that the worker should silently change the database.

Use the same database for every instance that participates in cleanup
coordination. The worker supports the `mysql` and `sqlite` database drivers.

## Configuration

| Variable | Default | Accepted value and effect |
|---|---:|---|
| `AGENTLY_CLEANUP_ENABLED` | `false` | Boolean master switch. If true while every policy is `off`, the worker logs that no policy is enabled and exits. |
| `AGENTLY_CLEANUP_INTERVAL` | `1h` | Positive Go duration between passes, for example `30m`, `2h`, or `24h`. Go durations do not support `d`; use `24h`. |
| `AGENTLY_CLEANUP_BATCH_SIZE` | `50` | Positive integer. Maximum eligible candidates processed by each policy in one pass, not a global limit shared by all policies. |
| `AGENTLY_CLEANUP_TIMEOUT` | `5m` | Positive Go duration applied separately to each policy. |
| `AGENTLY_CLEANUP_RUN_ON_START` | `false` | Boolean. Runs the first pass in the worker goroutine at startup; HTTP serving does not wait for the pass to finish. |
| `AGENTLY_CLEANUP_DEBUG` | `false` | Boolean. Adds per-candidate and lease acquire/renew/release logs. Policy and pass summaries are always logged. |
| `AGENTLY_CLEANUP_INTERACTIVE_MODE` | `off` | `off`, `dry-run`, or `execute`. Controls ordinary conversation retention and its related technical rows. |
| `AGENTLY_CLEANUP_INTERACTIVE_RETENTION_DAYS` | `30` | Positive integer number of days. |
| `AGENTLY_CLEANUP_SCHEDULED_MODE` | `off` | `off`, `dry-run`, or `execute`. Controls persisted scheduler runs, their conversation graphs, and related technical rows. |
| `AGENTLY_CLEANUP_SCHEDULED_RETENTION_DAYS` | `30` | Positive integer number of days. |
| `AGENTLY_CLEANUP_ORPHAN_MODE` | `off` | `off`, `dry-run`, or `execute`. Controls orphan maintenance and unclassified technical retention. |
| `AGENTLY_CLEANUP_ORPHAN_MIN_AGE_DAYS` | `14` | Positive integer grace period before a broken or unused reference becomes an orphan candidate. |

Boolean values accept `1/0`, `true/false`, `yes/no`, `y/n`, and `on/off`,
case-insensitively. Explicit invalid values stop server configuration rather
than falling back silently.

`AGENTLY_CLEANUP_INTERVAL=0s` is invalid. Sub-second values such as `2ms` are
technically valid Go durations but create an effectively continuous cleanup
loop and must not be used in a deployed environment.

`AGENTLY_DEBUG_CONVERSATION_DELETE` is a separate diagnostic switch for an
individual conversation-tree deletion. It does not control worker logging.

## Policy behavior

### Interactive retention

The worker starts from an old root conversation and evaluates its complete
graph, including parent/child, parent-turn, and linked-conversation edges. The
latest activity of every conversation in the graph must be at or before the
retention cutoff. Ownership must be present and consistent across the graph.

A candidate is skipped if it has recent activity, a live run or schedule,
an active report export, an external graph reference, an ownership mismatch,
or cannot be classified safely. A live run is determined from its current
lease and heartbeat, not from stale conversation or tool-call status alone.

### Scheduled retention

Scheduled retention has two complementary policies that share the scheduled
mode, retention cutoff, per-policy batch size, and timeout.

The primary policy starts from persisted scheduler runs in `run`, and also
supports the legacy `schedule_run` table when present. It deletes an old run
and its contained conversation graph but keeps the user schedule so future
occurrences can run.

The fallback policy starts from old root conversation graphs that classify as
scheduled but are no longer represented by any current `run` row or existing
legacy `schedule_run` row. This covers historical scheduled-conversation shells
left after their run metadata disappeared. A stale `schedule_run_id` value by
itself is not treated as an existing run. Before deletion, the complete graph
is locked and the absence of current and legacy run rows is checked again in
the transaction. If any run has appeared anywhere in the graph, the candidate
is skipped with `run_present`. The user schedule is retained.

System retention does not use historical owner columns as an authorization
boundary. It instead requires structural containment: the graph cannot belong
to another schedule or another persisted scheduled run. Internal schedules are
not candidates for this policy. Recent activity, live runs, and active report
exports still block deletion.

### Technical retention

Technical retention is enabled together with its parent mode and uses the same
retention period:

- interactive mode processes technical rows linked to interactive conversations;
- scheduled mode processes technical rows linked to scheduled conversations;
- orphan mode processes technical rows that no longer have a conversation.

The unclassified technical retention period is the greater of the configured
interactive and scheduled retention periods. It is not controlled by the
orphan minimum-age setting.

Technical retention covers `report_run`, `report_export_job`,
`report_audit_event`, and expired `session` rows. A positive per-row report
export TTL is honored where supported. Status is deliberately not an
eligibility condition: a row left `queued` or `running` for an entire retention
period is treated as stale technical state. Each candidate is rechecked before
mutation.

A session is not removed immediately when it expires. It becomes eligible only
when its `expires_at` value is older than the unclassified technical retention
cutoff.

### Orphan maintenance

Orphan rules have one of three fixed actions:

- `safe-delete`: delete a dependent row that cannot be valid without its parent;
- `safe-detach`: preserve the row and set the broken optional reference to NULL;
- `report-only`: report a suspicious relationship without changing it.

Examples include deleting unused `call_payload` rows, detaching an
`investigation` from a missing conversation, and reporting a report export job
whose optional report reference cannot be resolved. Execute mode rechecks every
candidate under the maintenance lease before applying its fixed action.

## Protected and external data

The cleanup worker observes these boundaries:

- `report_shared_artifact` contains durable saved-report definitions and is
  never selected by technical retention or orphan cleanup.
- `report_shared_artifact.source_artifact_id` is a logical source identity, not
  a foreign key to another shared artifact. The former
  `report_shared_artifact.missing_source` rule is disabled and must never become
  an automatic delete rule.
- `investigation` rows are retained; a missing conversation reference is
  detached.
- database metadata for report exports may be removed, but physical files and
  object-store artifacts are outside this worker. Their lifecycle is managed by
  the owning storage system, for example through TTL.

## Coordination and failure safety

Every pass uses the database lease key `conversation_cleanup`:

- lease TTL: 2 minutes;
- renewal interval: 30 seconds;
- released immediately after a completed pass;
- expired lease rows are retained for 7 days and then removed by a lease holder.

The lease has a fencing token. Every destructive maintenance transaction
validates and locks the current lease, so a former owner cannot continue
mutating data after another instance takes over. The process also prevents two
passes from overlapping inside one instance.

If another instance owns the lease, the pass is skipped and logged. Losing or
failing to renew the lease cancels the pass. SQLite additionally serializes the
relevant local writes; MySQL uses database transactions and row locks.

## Logging

Startup logs list the effective interval, per-policy batch size, timeout,
modes, retention periods, policy names, and lease settings. Every completed
policy and pass emits a summary containing:

- `scanned`: candidates inspected;
- `eligible`: candidates that passed their policy checks;
- `deleted`: candidate operations that deleted data, not the number of SQL rows;
- `mutated`: candidate operations that changed data. A `safe-delete` orphan
  contributes to both `deleted` and `mutated`; a `safe-detach` contributes only
  to `mutated`, so these counters must not be added together;
- `skipped`: candidates rejected after revalidation;
- `failed`: candidate operations that returned errors;
- `reasons`: aggregate eligibility, deletion, skip, or orphan-rule reasons.

With `AGENTLY_CLEANUP_DEBUG=true`, logs additionally identify each candidate and
show lease acquisition, renewal, and release. Disable debug after rollout unless
candidate-level diagnostics are needed.

## Recommended rollout

Start with a bounded dry run:

```bash
AGENTLY_CLEANUP_ENABLED=true \
AGENTLY_CLEANUP_RUN_ON_START=true \
AGENTLY_CLEANUP_INTERVAL=24h \
AGENTLY_CLEANUP_BATCH_SIZE=100 \
AGENTLY_CLEANUP_TIMEOUT=5m \
AGENTLY_CLEANUP_DEBUG=true \
AGENTLY_CLEANUP_INTERACTIVE_MODE=dry-run \
AGENTLY_CLEANUP_INTERACTIVE_RETENTION_DAYS=30 \
AGENTLY_CLEANUP_SCHEDULED_MODE=dry-run \
AGENTLY_CLEANUP_SCHEDULED_RETENTION_DAYS=30 \
AGENTLY_CLEANUP_ORPHAN_MODE=dry-run \
AGENTLY_CLEANUP_ORPHAN_MIN_AGE_DAYS=14 \
./agently serve
```

Review policy summaries and candidate-level reasons. Then run a limited execute
pass with the same retention boundaries:

```bash
AGENTLY_CLEANUP_ENABLED=true \
AGENTLY_CLEANUP_RUN_ON_START=true \
AGENTLY_CLEANUP_INTERVAL=24h \
AGENTLY_CLEANUP_BATCH_SIZE=50 \
AGENTLY_CLEANUP_TIMEOUT=5m \
AGENTLY_CLEANUP_DEBUG=true \
AGENTLY_CLEANUP_INTERACTIVE_MODE=execute \
AGENTLY_CLEANUP_INTERACTIVE_RETENTION_DAYS=30 \
AGENTLY_CLEANUP_SCHEDULED_MODE=execute \
AGENTLY_CLEANUP_SCHEDULED_RETENTION_DAYS=30 \
AGENTLY_CLEANUP_ORPHAN_MODE=execute \
AGENTLY_CLEANUP_ORPHAN_MIN_AGE_DAYS=14 \
./agently serve
```

After validating database counts and application behavior, select the normal
batch size and interval for the environment and set
`AGENTLY_CLEANUP_DEBUG=false`. Running the same configuration on multiple
instances is supported; one instance acquires the lease and the others log a
lease-held skip.
