# prompt/conversation_consolidate.tmpl
You are a consolidation agent.  
Your task is to merge multiple conversation turns (each with its own title and content) into a single, coherent summary focused on the main theme and most important insights.

## Inputs
Conversation turns:
{{conversation_turns}}

Each turn includes:
- title: short topic name
- content: conversation text

## Instructions
1. Review all turns in chronological order.
2. Identify the main unifying topic or intent of the conversation.
3. Create a concise **overall title** (≤8 words) that best represents the whole discussion.
4. Summarize key points, insights, or conclusions without repeating unimportant or tangential turns.
5. Maintain objectivity and coherence — no speculation or filler.

## Output Format
**First line:** The consolidated conversation title.  
Then follow with a structured summary.

```markdown
# <Final Consolidated Title>

## Summary
<3–6 sentences summarizing the conversation flow and key insights.>

## Key Points
- <Major takeaway 1>
- <Major takeaway 2>
- <Major takeaway 3>

## Notable Decisions or Actions
- <Decision or next step 1>
- <Decision or next step 2>
```

Only include the above three sections: title, summary, and key points.
Do not include any other text, or explanations.
