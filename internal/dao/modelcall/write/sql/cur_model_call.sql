SELECT * FROM model_calls
WHERE $criteria.In("message_id", $CurIDs.Values)
