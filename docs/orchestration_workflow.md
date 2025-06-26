# Agently Default Orchestration Workflow

This document explains the **default Fluxor workflow** shipped with Agently, the
data-structures it manipulates and several recipes for customising it.

It should serve as a reference when you want to:

* adapt the LLM planning strategy (e.g. use a dedicated prompt template)
* inject your own post-processing or additional tool steps
* tweak retry / loop behaviour or error handling

---

## 1. File locations

| Artifact | Path |
|----------|------|
| Default workflow YAML | `genai/extension/fluxor/llm/agent/orchestration/workflow.yaml` |
| LLM Plan prompt VM     | `genai/extension/fluxor/llm/core/prompt/plan_prompt.vm` |
| Go structs             | `genai/agent/plan/*` and `genai/extension/fluxor/llm/core/*.go` |

---

## 2. Workflow YAML – high level view

```yaml
init:
  results: []                # accumulator for tool results

stage:
  loop:                      # repeat until the plan has no more steps

    # 1️⃣ Ask the LLM to propose / refine a Plan or a direct answer
    plan:
      action: llm/core:plan
      input:
        model:      ${model}
        tools:      ${tools}
        query:      ${query}
        context:    ${context}
        results:    ${results}
        transcript: ${transcript}

      post:
        plan:        ${plan}
        answer:      ${answer}
        elicitation: ${plan.elicitation}
        results:     ${results}
        transcript:  ${transcript}

    # 2️⃣ Execute outstanding plan steps (if any)
    exec:
      action: llm/exec:run_plan
      when: len(plan.steps) > 0
      input:
        plan:       ${plan}
        model:      ${model}
        tools:      ${tools}
        query:      ${query}
        context:    ${context}
        results:    ${results}
        transcript: ${transcript}

      post:
        results:    ${results}
        elicitation: ${elicitation}
        transcript: ${transcript}

      goto:               # continue the loop until plan is empty
        task: loop
```

The workflow has two alternating **tasks**:

1. **llm/core:plan** – the *planner* action. It calls the LLM with a prompt
   and available tools to produce a `Plan` or a direct `Answer`.
2. **llm/exec:run_plan** – the *executor* action. Runs the remaining `plan.steps`
   (tools or elicitation) and appends their results to `results`.

The `goto: loop` makes the pair repeat until the planner returns an empty
`plan.steps` array (meaning work is complete) or raises an `elicitation`
request that waits for user input.

---

## 3. Plan data-structure

```yaml
id:        "uuid-…"       # optional
intention: "Summarise website ‚…‘ as bullet list"
steps:                       # ordered execution list
  - type:  tool             # "tool" | "elicitation" | "noop" | "abort" …
    name:  web.fetch        # tool identifier in registry (service_method)
    args:                   # arbitrary JSON – validated against tool schema
      url: "https://…"
    reason: "need raw html"
    retries: 2

elicitation:                # present only when interactive input is required
  message: "Provide API key"
  schema:  "{…JSON Schema…}"
```

Important helper types:

* `plan.Result` – tuple `(id, name, args, result, error)` appended after each
  tool execution and passed back into the planner for refinement.
* `memory.Message` – transcript slice for conversational agents.

See Go definitions in `genai/agent/plan/*.go`.

---

## 4. Action contracts

### 4.1 `llm/core:plan`

| Input field | Type | Description |
|-------------|------|-------------|
| `query`     | string | Original user question |
| `context`   | string | Optional extra context (system prompt) |
| `model`     | string | Override for default LLM model |
| `tools`     | []string | Names of tools the planner may choose from |
| `results`   | []plan.Result | Accumulated results so far |
| `transcript`| []memory.Message | Conversation so far |

Output:

| Field | Meaning |
|-------|---------|
| `plan` | Newly generated / refined Plan |
| `answer` | Direct textual answer when no tool steps are needed |
| `elicitation` | Optional plan-level elicitation |
| `results` | Unchanged accumulator |
| `transcript` | Updated LLM dialogue |

### 4.2 `llm/exec:run_plan`

Consumes the remaining `plan.steps` and executes each tool.

Important behaviours:

* Supports **retries** at step level.
* Converts nested function calls (OpenAI tool calls) into steps when returned
  by the LLM.
* Emits `elicitation` when a step requires user input via MCP.

Input/Output mirrors the planner but with `results` being appended.

---

## 5. Customisation recipes

### 5.x Duplicate-call safety net *(added in vNext)*

Agently now enforces two lightweight heuristics to prevent the agent from
entering an infinite tool-call loop:

1. **Consecutive guard** – if the *same* tool is called with the *same
   arguments* three times **in a row**, the 3rd and any further consecutive
   calls are blocked.
2. **Sliding-window guard** – inside the last eight executed steps, if a
   single `(tool, args)` pair appears four or more times *or* the pattern
   alternates between exactly two distinct keys (e.g. `A B A B A B A`), the
   next repeat is blocked.

When a call is blocked the executor appends a synthetic `plan.Result` to
`results`:

```yaml
- name:   cat.file
  args:   {path: "file.txt"}
  error:  duplicate_call_blocked   # <- special marker
  result: "...last successful output..."  # echoed for convenience
```

The LLM therefore sees a clear signal that it must adjust its strategy.

No configuration is required; the defaults work well in practice.  Advanced
users can tune the limits via service options in code (`consecutiveLimit`,
`windowSize`, `windowFreqLimit`).

### 5.1 Change the plan prompt

1. Copy `plan_prompt.vm` to a new file (e.g. `my_prompt.vm`).
2. Reference it in workflow YAML (or JSON) by populating `input.promptTemplate`:

```yaml
plan:
  action: llm/core:plan
  input:
    promptTemplate: ${myPrompt}   # variable set by caller (string with full VM template)
```

If the `promptTemplate` field is **empty or omitted** the engine defaults to the
embedded `plan_prompt.vm`.  Therefore you only need to pass a value when you want
to override the default planning prompt.

How to supply `${myPrompt}`:

* **CLI** – upcoming flags will allow `--prompt-template file.vm` or inline JSON
  via `agently run -i '{"promptTemplate": "..."}'`. For now you can fork the
  workflow YAML and hard-code the string.
* **HTTP / gRPC** – put the template text directly into the JSON payload you
  POST to `llm/core:plan`.

### 5.2 Restrict or extend available tools

Pass a filtered list via `tools` input. Example in CLI:

```sh
agently chat --tools web.search,calc.eval "How many days between …"
```

### 5.3 Inject post-processing step

Append another task after `exec`:

```yaml
postproc:
  action: my/tool:summarise
  when: len(results) > 0
  input:
    data: ${results[-1].result}
  post:
    summary: ${summary}
```

### 5.4 Alter loop termination

Instead of looping until `plan.steps` empty you can exit after N iterations:

```yaml
vars:
  iteration: 0

exec:
  post:
    iteration: ${iteration + 1}
  goto:
    when: iteration < 3 && len(plan.steps) > 0
    task: loop
```

### 5.5 Custom error handling

Tool errors are stored in `results[i].error`.  You can abort early:

```yaml
exec:
  onError:
    action: notify:send_slack
    input:
      channel: "#runtime-alerts"
      text: "tool ${step.name} failed: ${error}"
  abort: true
```

---

## 6. Inputs/outputs cheat-sheet

```
# Planner input  (llm/core:plan)
query, context, model, tools, results[], transcript[]

# Planner output
plan, answer, elicitation, results, transcript

# Executor input  (llm/exec:run_plan)
plan, model, tools, query, context, results, transcript

# Executor output
results, elicitation, transcript
```

---

## 7. Tool naming convention

Tools are referenced by **registry name** `<service>_<method>` (snake-case).

Example: the Go function `web.Fetch` registered as `web_fetch` becomes
`web.fetch` in Plan YAML (period instead of underscore) – the converter is
handled by Agently.

Validate a Plan with:

```sh
agently exec validate_plan -i plan.yaml
```

---

## 8. Further reading

* Fluxor syntax – https://github.com/viant/fluxor
* Tool registry API – `service/tool.go`
* MCP elicitation protocol – https://github.com/viant/mcp-protocol

---

Feel free to open issues or PRs if parts of this document are unclear ✨.
