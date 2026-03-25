( SELECT t.*
    FROM generated_file t
    ${predicate.Builder().CombineOr($predicate.FilterGroup(0, "AND")).Build("WHERE")}
    ORDER BY t.created_at ASC )