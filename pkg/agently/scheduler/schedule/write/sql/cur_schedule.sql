SELECT * FROM schedule
WHERE $criteria.In("id", $CurSchedulesId.Values)
