Executes shell commands on local or remote host.

- Stateless between calls; commands in one call share `workdir` and `env`.
- No pipes (`|`) and no `cd`; set `workdir` instead.
- Prefer `rg` for search; set `timeoutMs` to bound long runs.

Examples
- workdir: /repo/path; commands: ["rg --files", "sed -n '1,50p' main.go"]
- env: {GOFLAGS: "-mod=mod"}; commands: ["go env", "go list ./..."]
