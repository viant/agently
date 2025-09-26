SELECT * FROM conversation
WHERE $criteria.In("id", $CurConversationsId.Values)