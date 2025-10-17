# Shell Command Execution Guidelines

**Description**  
Executes one or more shell commands in a local sandbox.

- Each **tool call is stateless**: no working directory, environment variables, or outputs are persisted between separate calls.
- If multiple commands are passed in a single call, they execute **within the same session context** — sharing the same `workdir`, `env`, and error-handling settings.
- Do not include `cd` in commands — the tool manages working directory context.
- Do not use **piping (`|`)** — commands must be expressed independently or via simple `&&` chaining.

---

## Behavior

- **Stateless per call**: no `cwd` or state is carried between tool calls.
- **Shared session per call**: multiple commands inside one call run under the same context.
- **Working directory**:
    - For **file system operations** (e.g., `rg`, `ls`, `cat`, `sed`), paths are resolved relative to `workdir`.
    - Non-file-system commands (e.g., `date`, `echo`, `env`) are allowed and unaffected by `workdir`.
- **Command independence**:
    - Each command runs independently unless explicitly chained with `&&` or included in the same call.
    - **Pipes (`|`) are not allowed** — split commands instead.
- **Prohibited commands**: Reject commands starting with `cd`.
- **Search**: Prefer `ripgrep (rg)` for searching; avoid non-portable flags like `--search-path`.
- **Guardrails**:
    - Avoid long-running operations.
    - Always set a small timeout to bound runtime.
    - Known long-running commands without timeout may be blocked.
- **Optional flags**:
    - `abortOnError` → stop after the first non-zero exit.
    - `env` → set per-call environment variables.
- **Output**:
    - Per-command `{input, output, stderr, status}`.
    - Aggregated stdout/stderr across commands.
    - Large outputs may be truncated with a note.

---

## Performance

- ❌ Avoid: `ls -R`, `find`, `grep` — slow in large repos.
- ✅ Use: `rg`, `rg --files`.

---

## Working Directory Rules

- Applies **only to file system operations**.
- Never guess `workdir` — always confirm with the user.
- Never set `workdir` as `.` (current directory).

---

## Validation

- `workdir` must exist and be accessible (typically within the active workspace root).
- Relative paths in file system commands resolve against `workdir`.
- If a command is denied (e.g., starts with `cd`, contains a pipe, or is long-running without timeout), return a **corrective error message**.

---

## Examples

**Correct** (list files in a subdir and read a file chunk):
```yaml
workdir: "/Users/awitas/go/src/github.com/viant/datly"
commands:
  - rg --files v2/json
  - sed -n '1,200p' v2/json/meta/struct.go
