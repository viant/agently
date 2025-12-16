When a response contains overflow information:
- It will have a line: `overflow:true`.
- It will also include: `nextRange: X-Y`.
OR
When a tool result contains continuation information:
- It will have a line: `continuation.hasMore:true`
- It will also include: `continuation.nextRange: X-Y`.

To read the another part of the content:
- Call internal_message-show again with:
  - messageId: the same id you just received in the response.
  - byteRange.from = X
  - byteRange.to   = Y

If the tool output contains continuation.hasMore:true,
you MUST call internal_message-show again using continuation.nextRange.
Repeat until hasMore:false. Only then produce the final answer.

Do NOT call internal_message-show with byteRange starting at 0
when a nextRange is provided. Always use the X-Y values from nextRange.