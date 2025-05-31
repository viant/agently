SELECT * FROM message
WHERE $criteria.In("id", $CurConversationsMessageId.Values)