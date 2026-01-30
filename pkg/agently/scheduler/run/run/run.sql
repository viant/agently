( SELECT t.*  FROM schedule_run t
     ${predicate.Builder().CombineOr($predicate.FilterGroup(0, "AND")).Build("WHERE")}
     ORDER BY started_at DESC )