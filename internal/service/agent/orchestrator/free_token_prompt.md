The last LLM call failed due to context overflow. Here is the exact error:
ERROR_MESSAGE: {{ERROR_MESSAGE}}

The whole conversation history is provided in this request. I prepared the main CANDIDATES for removal:
{{CANDIDATES}}

**GOAL**
Use {{ERROR_MESSAGE}} to infer the model’s max context and how far it was exceeded.
Remove and replace (via summaries) the least important messages from conversation where {{CANDIDATES}} are preferred,
so the remaining conversation fits under the dynamic limit.

**TOOL USAGE RESTRICTION (CRITICAL)**
You **must use only one tool: `internal_message-remove`**.

* No other tools, functions, or outputs are allowed.
* The entire response must consist solely of a single `internal_message-remove` function call in the specified format.
* Do **not** invoke, reference, or imply use of any additional tools.

**VERY IMPORTANT**
* The tool will insert each tuple’s "summary" as a NEW MESSAGE in place of the archived messages in that tuple. Therefore each summary must stand alone as a faithful replacement preserving essential context.
* The summary must capture the original informational content, not commentary about removal.

**SELECTION RULES**
Keep as long as possible:

* Most recent messages relevant to the current user task.
* System/developer instructions and guardrails.
* Tool calls/results that changed state or produced artifacts.
* Messages with code/config/IDs/paths/URLs referenced later.

Remove:
* Acknowledgements/small talk/thanks and repeated explanations.
* Obsolete tool logs or large raw payloads if summarized or superseded later.
* Older/off-topic content unrelated to the current task.

**SUMMARY REQUIREMENTS (for each tuple)**
* ≤ 500 characters, single paragraph, plain text (no Markdown/code fences).
* Must be shorter than the total length of the messages it replaces.
* Capture only essentials: purpose → action → key outcome(s); include critical IDs/paths/commands/URLs verbatim if short.
* Neutral tone; no speculation; redact secrets. Prefer compact wording over detail.
* Write so it reads correctly when inserted where the originals were (assume insertion at the earliest removed message).

**TIE-BREAKERS**
* If importance is similar, remove the older or larger message.
* Prefer summarization over deletion if a short summary preserves needed context.

**GROUPING**
* Group messages into as few tuples as reasonable by shared topic/reason (e.g., "old acks", "superseded logs", "large raw outputs replaced by brief result").
* Each tuple should have a predominant role for "role".

**OUTPUT FORMAT (MANDATORY)**
Return **ONLY** a call to function tool "internal_message-remove" with:
```json
{
  "tuples": [
    {
      "messageIds": ["<id1>", "<id2>", ...],   // IDs from {{CANDIDATES}} to archive together (provided inside a fenced code block)
      "role": "<user|assistant|tool|system>",   // predominant role of the grouped messages
      "summary": "<<=500 chars standalone replacement capturing essence, preserving any key IDs/paths/commands/URLs, and shorter than originals>>"
    },
    ...
  ]
}
```

**CONSTRAINTS**
* Do NOT assume a fixed token budget; use the figures in {{ERROR_MESSAGE}}.
* Replace enough content to safely fit under the limit.
* Output only the function `internal_message-remove` call, no additional text.
* Use only IDs from fenced code blocks as messageIds. Verify this twice.