Show a message body with optional transform, range and sed. Make sure that either range, transform or set is set - otherwise you would only see preview
When an response contains overflow information:
- It will have a line: `overflow:true`.
- It will also include: `nextRange: X-Y`.

To read the rest of the content:
- Call internal_message-show again with:
    - messageId: the same id you just received in the response.
    - byteRange.from = X
    - byteRange.to   = Y

Do NOT call internal_message-show with byteRange starting at 0
when a nextRange is provided. Always use the X-Y values from nextRange.
