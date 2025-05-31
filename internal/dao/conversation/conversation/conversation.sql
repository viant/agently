( SELECT c.* FROM conversation c
    ${predicate.Builder().CombineOr($predicate.FilterGroup(0, "AND")).Build("WHERE")} )