SELECT * FROM tool_call
WHERE $criteria.In("message_id", $CurIDs.Values)
