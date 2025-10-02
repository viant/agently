( SELECT t.*  FROM schedule_run t
     ${predicate.Builder().CombineOr($predicate.FilterGroup(0, "AND")).Build("WHERE")} )