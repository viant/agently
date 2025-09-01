SELECT * FROM conversation
WHERE $criteria.In("id", $CurIds.Values)

