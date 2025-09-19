SELECT p.*, m.parent_message_id FROM message m
 JOIN call_payload p ON m.payload_id = p.id