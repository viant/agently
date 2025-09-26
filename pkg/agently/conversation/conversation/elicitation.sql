SELECT m.content AS inline_body, 'none' AS compression, m.parent_message_id
FROM message m
WHERE m.elicitation_id IS NOT NULL
