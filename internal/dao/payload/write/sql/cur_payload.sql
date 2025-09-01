SELECT * FROM call_payloads
WHERE $criteria.In("id", $CurIDs.Values)
