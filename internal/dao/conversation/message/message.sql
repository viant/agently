SELECT 
    id,
    conversation_id,
    role,
    content,
    tool_name,
    created_at
FROM message
WHERE conversation_id = $ConvId
