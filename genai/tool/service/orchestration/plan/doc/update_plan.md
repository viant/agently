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
- Echoes back the explanation and the normalized plan; returns an error on invalid input (e.g., multiple in_progress, unknown statuses, missing fields).

Example: “Call orchestration-updatePlan with: { explanation: 'Fix Settings preselected tools', plan: [{step:'Locate Settings load flow',status:'in_progress'},{step:'Patch tool binding',status:'pending'},{step:'Add focused
test',status:'pending'}] }”