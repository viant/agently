SELECT inline_body, compression, m.id as message_id FROM message m
 JOIN call_payload p ON m.elicitation_payload_id = p.id