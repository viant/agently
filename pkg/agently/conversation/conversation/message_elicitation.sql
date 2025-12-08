SELECT
    id,
    conversation_id,
    turn_id,
    archived,
    sequence,
    created_at,
    updated_at,
    created_by_user_id,
    status,
    mode,
    role,
    type,
    content,
    raw_content,
    summary,
    context_summary,
    tags,
    interim,
    elicitation_id,
    parent_message_id,
    superseded_by,
    linked_conversation_id,
    attachment_payload_id,
    elicitation_payload_id,
    tool_name,
    embedding_index
FROM message
WHERE conversation_id = $criteria.AppendBinding($Unsafe.ConversationId)
  AND elicitation_id  = $criteria.AppendBinding($Unsafe.ElicitationId)
ORDER BY created_at DESC
LIMIT 1
