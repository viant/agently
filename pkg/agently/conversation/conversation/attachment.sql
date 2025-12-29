SELECT
    NULL AS inline_body,
    p.compression,
    p.uri,
    p.mime_type,
    m.parent_message_id
FROM message m
JOIN call_payload p ON m.attachment_payload_id = p.id
