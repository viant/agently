SELECT * FROM turns
WHERE $criteria.In("id", $CurTurnsId.Values)
