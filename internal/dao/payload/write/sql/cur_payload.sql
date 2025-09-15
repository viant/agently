SELECT * FROM call_payload
WHERE $criteria.In("id", $CurIDs.Values)
