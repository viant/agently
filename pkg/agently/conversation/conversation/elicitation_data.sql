SELECT p.*, m.parent_message_id FROM message m
 JOIN call_payload p ON m.elicitation_payload_id = p.id AND m.elicitation_id IS NOT NULL
