SELECT * FROM tool_calls
WHERE $criteria.In("message_id", $CurIDs.Values)
