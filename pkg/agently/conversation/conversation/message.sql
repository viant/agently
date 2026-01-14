SELECT m.* FROM message m
    ${predicate.Builder().CombineOr($predicate.FilterGroup(4, "AND")).Build("WHERE")}