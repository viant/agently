Executes shell commands on local or remote host.

- Stateless between calls; commands in one call share `workdir` and `env`.
- No pipes (`|`) or ||, &&  and no `cd`; set `workdir` instead.
- Prefer `rg` for search; set `timeoutMs` to bound long runs.
- Do not include `cd` in commands — the tool manages working directory context.

 Working Directory Rules
    - Applies **only to file system operations**.
    - Never set `workdir` as `.` (current directory).

- Examples
- workdir: /repo/path; commands: ["rg --files", "sed -n '1,50p' main.go"]
- env: {GOFLAGS: "-mod=mod"}; commands: ["go env", "go list ./..."]
