SELECT 
  mc.message_id,
  CASE WHEN mc.completed_at IS NOT NULL THEN 'completed' ELSE 'scheduled' END AS status,
  mc.turn_id,
  mc.provider,
  mc.model,
  mc.model_kind,
  mc.prompt_tokens,
  mc.completion_tokens,
  mc.total_tokens,
  mc.finish_reason,
  mc.cache_hit,
  mc.cache_key,
  mc.started_at,
  mc.completed_at,
  mc.latency_ms,
  mc.cost,
  mc.trace_id,
  mc.span_id,
  mc.request_payload_id,
  mc.response_payload_id
FROM model_calls mc
LEFT JOIN message m ON m.id = mc.message_id
${predicate.Builder().CombineOr($predicate.FilterGroup(0, "AND")).Build("WHERE")} 
