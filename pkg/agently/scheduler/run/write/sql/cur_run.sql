SELECT * FROM schedule_run
WHERE $criteria.In("id", $CurRunsId.Values)

