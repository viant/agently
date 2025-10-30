( SELECT t.*  FROM users t
     ${predicate.Builder().CombineOr($predicate.FilterGroup(0, "AND")).Build("WHERE")} )