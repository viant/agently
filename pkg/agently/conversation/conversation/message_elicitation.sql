SELECT *
FROM message
WHERE conversation_id = $criteria.AppendBinding($Unsafe.ConversationId)
  AND elicitation_id  = $criteria.AppendBinding($Unsafe.ElicitationId)
ORDER BY created_at DESC
LIMIT 1
