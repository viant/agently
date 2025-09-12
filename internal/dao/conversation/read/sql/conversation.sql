( SELECT c.* FROM conversation c
    ${predicate.Builder().CombineOr($predicate.FilterGroup(0, "AND"), $predicate.FilterGroup(1, "AND")).Build("WHERE")} )
