SELECT
    t.id,
    t.queue_seq
FROM turn t
WHERE t.status = 'queued'
${predicate.Builder().CombineOr($predicate.FilterGroup(0, "AND")).Build("AND")}
ORDER BY COALESCE(t.queue_seq, -1) ASC, t.created_at ASC, t.id ASC
