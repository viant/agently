# Scheduler

Agently supports schedules stored in the database (`schedule`, `schedule_run`). In horizontally scaled or serverless deployments you should run schedule execution in a dedicated process to avoid duplicate triggers.

## Recommended Deployment Modes

### 1) Serverless / horizontally scaled `serve`

- Run `agently serve` without the watchdog.
- Run the scheduler runner as a single instance (separate deployment).

Environment (serve):
- `AGENTLY_SCHEDULER_RUNNER=0` (default)
- `AGENTLY_SCHEDULER_API=1` (default) to keep CRUD APIs available
- `AGENTLY_SCHEDULER_RUN_NOW=1` (default) to allow run-now enqueue

### 2) Dedicated scheduler runner

Run:
- `agently scheduler run --interval 30s`

Environment:
- `AGENTLY_SCHEDULER_LEASE_TTL=60s` (optional; default `60s`)
- `AGENTLY_SCHEDULER_LEASE_OWNER=my-runner-1` (optional; otherwise auto-generated)

The runner uses a DB-backed lease (`schedule.lease_owner`, `schedule.lease_until`) to reduce duplicate processing during deploy overlap.

## HTTP Scheduler API toggles

- `AGENTLY_SCHEDULER_API`:
  - Default: enabled
  - Set to `0` to disable all scheduler HTTP endpoints in `serve`.
- `AGENTLY_SCHEDULER_RUN_NOW`:
  - Default: enabled
  - Set to `0` to disable run-now routes in `serve`.

## Run-now semantics

`POST /v1/api/agently/scheduler/run-now` enqueues a `schedule_run` row (`status=pending`) even when no orchestration scheduler is wired. If a scheduler runner is present, it can pick up and execute the run.

