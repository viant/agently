The last LLM call failed due to context overflow. Here is the exact error:
ERROR_MESSAGE: {{ERROR_MESSAGE}}

You are given a list of candidate messages that may be removed:
{{CANDIDATES}}

GOAL
Use {{ERROR_MESSAGE}} to infer the model’s max context and how far it was exceeded. Remove or replace (via summaries) the least important messages from {{CANDIDATES}} so the remaining conversation fits under the dynamic limit.

VERY IMPORTANT
The tool will insert each tuple’s "summary" as a NEW MESSAGE in place of the archived messages in that tuple. Therefore each summary must stand alone as a faithful replacement preserving essential context.

SELECTION RULES
Keep:
- Most recent messages relevant to the current user task.
- System/developer instructions and guardrails.
- Tool calls/results that changed state or produced artifacts.
- Messages with code/config/IDs/paths/URLs referenced later.

Remove or summarize:
- Acknowledgements/small talk/thanks and repeated explanations.
- Obsolete tool logs or large raw payloads if summarized or superseded later.
- Older/off-topic content unrelated to the current task.

SUMMARY REQUIREMENTS (for each tuple)
- ≤ 256 characters, single paragraph, plain text (no Markdown/code fences).
- Capture only essentials: purpose → action → key outcome(s); include critical IDs/paths/commands/URLs verbatim if short.
- Neutral tone; no speculation; redact secrets. Prefer compact wording over detail.
- Write so it reads correctly when inserted where the originals were (assume insertion at the earliest removed message).

TIE-BREAKERS
- If importance is similar, remove the older or larger message.
- Prefer summarization over deletion if a short summary preserves needed context.

GROUPING
- Group messages into as few tuples as reasonable by shared topic/reason (e.g., "old acks", "superseded logs", "large raw outputs replaced by brief result").
- Each tuple should have a predominant role for "role".

OUTPUT FORMAT (MANDATORY)
Return ONLY a call to function tool "internal_message-remove" with:
{
"tuples": [
{
"messageIds": ["<id1>", "<id2>", ...],   // IDs from {{CANDIDATES}} to archive together
"role": "<user|assistant|tool|system>",   // predominant role of the grouped messages
"summary": "<<=256 chars standalone replacement capturing essence and preserving any key IDs/paths/commands/URLs>"
},
...
]
}

CONSTRAINTS
- Do NOT assume a fixed token budget; use the figures in {{ERROR_MESSAGE}}.
- Remove enough content to safely fit under the limit with headroom.
- Do NOT output any prose outside the function call.
