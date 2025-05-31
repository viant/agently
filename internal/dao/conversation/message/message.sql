( SELECT m.* FROM message m
    ${predicate.Builder().CombineOr($predicate.FilterGroup(0, "AND")).Build("WHERE")} )