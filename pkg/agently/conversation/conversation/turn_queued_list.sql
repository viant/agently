SELECT
    t.id,
    t.queue_seq
FROM turn t
${predicate.Builder().CombineOr($predicate.FilterGroup(0, "AND")).Build("WHERE")}
  AND t.status = 'queued'
ORDER BY COALESCE(t.queue_seq, -1) ASC, t.created_at ASC, t.id ASC

