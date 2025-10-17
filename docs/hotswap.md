# Hot-Swap Live Reload

Agently supports **live reloading** of workspace resources such as agents,
models, embedders and workflows while the process is running.  The feature –
called *Hot-Swap* – is primarily intended for local development where rapid
iteration is important.  It can be disabled for deterministic production
deployments.

## Architecture

```
┌─────────────────────────────┐            file-system events
│    fsnotify.Watcher         │───────────────┐
└─────────────────────────────┘               │
                                             ▼
┌─────────────────────────────┐   change   ┌──────────────┐
│     hotswap.Manager         │──────────▶│ Reloadable… │
└─────────────────────────────┘            └──────────────┘
          ▲            ▲                          ▲
          │            │                          │
          │            │                          │
          │            │                          │
   RegisterAll()  Start()/Stop()          Agent / Model / … registries

```

1. `hotswap.RegisterAll(exec, cfg)` is invoked from
   `executor.bootstrap.init()` after the executor has initialised its internal
   registries.  It creates a single `hotswap.Manager` instance and registers
   adaptors for each workspace kind (agents, models, embedders, workflows, MCP
   clusters).
2. `Manager.Start()` begins watching the workspace root obtained from
   `internal/workspace.Root()` using `fsnotify`.  Events are debounced to avoid
   thrashing on editors that emit multiple writes.
3. Each event is translated into a *hot-swap action* (`AddOrUpdate`, `Delete`)
   and dispatched to the corresponding *Reloadable* registry.
4. Registries perform the minimal update (re-parse YAML, replace entry, etc.)
   so existing references continue working with zero downtime.
5. `Manager.Stop()` is called from `executor.Service.Shutdown()` ensuring a
   graceful exit.

## Enabling / Disabling

Hot-Swap is **enabled by default**.

### 1. Programmatic toggle

```go
// Disable at construction time
exec, _ := executor.New(ctx, executor.WithoutHotSwap())
```

### 2. Environment-variable toggle

The toggle can also be controlled at runtime without code changes.

*Environment variable*: `AGENTLY_HOTSWAP`

| Value (case-insensitive) | Effect                   |
|--------------------------|--------------------------|
| `0`, `false`, `off`, `no`, `disable`, `disabled` | Hot-Swap **disabled** |
| *unset* or any other value | Hot-Swap **enabled** |

The variable is evaluated once inside `executor.New(...)`.  When an explicit
`executor.WithoutHotSwap()` option is supplied it takes precedence over the
environment variable.

## Operational notes

• Hot-Swap is best for development; disable it in production to avoid
  mid-execution drift.

• File-system notifications depend on the underlying OS.  Very large
  repositories may hit watch limits – adjust `fs.inotify.max_user_watches` on
  Linux if necessary.

• The default debounce interval is **200 ms** (configured in
  `internal/workspace/hotswap/register.go`).  Tune `NewManager(root, debounce)` if
  required.

---

_Document version: 1.0_
