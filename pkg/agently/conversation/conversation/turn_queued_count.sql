SELECT
    COUNT(*) AS queued_count
FROM turn t
${predicate.Builder().CombineOr($predicate.FilterGroup(0, "AND")).Build("WHERE")}
  AND t.status = 'queued'

