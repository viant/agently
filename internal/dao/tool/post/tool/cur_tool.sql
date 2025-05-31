SELECT * FROM tool_call
WHERE $criteria.In("id", $CurToolCallId.Values)