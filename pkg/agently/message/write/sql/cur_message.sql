SELECT * FROM message
WHERE $criteria.In("id", $CurMessagesId.Values)
