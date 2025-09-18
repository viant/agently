SELECT * FROM model_call
WHERE $criteria.In("message_id", $CurIDs.Values)
