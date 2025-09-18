SELECT * FROM turn
WHERE $criteria.In("id", $CurTurnsId.Values)
