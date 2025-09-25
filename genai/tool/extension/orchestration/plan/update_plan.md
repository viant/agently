Updates the current task plan with a clear explanation and concise steps. Use for non-trivial, multi-step tasks; avoid for single-step queries.

Behavior
- Validates that exactly one step is in_progress
(or none if all are completed).
- Each plan item has: step (short sentence) and status (pending|in_progress|completed).
- You may mark multiple steps completed at once and advance the next to in_progress.

- If everything is done, mark all steps completed.
- The harness renders the plan; do not echo the full plan in assistant replies. Summarize the change and highlight the next step instead.

Parameters
-
explanation: short, high-level context for this update.
- plan: ordered list of {step, status} items.

Output
- Echoes back the explanation and the normalized plan; returns an error on invalid input (e.g.,
multiple in_progress, unknown statuses, missing fields).

Example
- explanation: "Implement recursive JSON support"
- plan: [
  {"step": "Analyze json/meta and marshaler", "status": "completed"},
  {"step":
"Patch struct meta recursion", "status": "in_progress"},
  {"step": "Add marshal/unmarshal tests", "status": "pending"}
]