( SELECT t.*  FROM schedule t
     ${predicate.Builder().CombineOr($predicate.FilterGroup(0, "AND")).Build("WHERE")} )