SELECT 
  tc.message_id,
  tc.turn_id,
  tc.op_id,
  tc.attempt,
  tc.tool_name,
  tc.tool_kind,
  tc.capability_tags,
  tc.resource_uris,
  tc.status,
  tc.request_payload_id,
  tc.request_hash,
  tc.response_payload_id,
  tc.error_code,
  tc.error_message,
  tc.retriable,
  tc.started_at,
  tc.completed_at,
  tc.latency_ms,
  tc.cost,
  tc.trace_id,
  tc.span_id
FROM tool_call tc
LEFT JOIN message m ON m.id = tc.message_id
${predicate.Builder().CombineOr($predicate.FilterGroup(0, "AND")).Build("WHERE")} 
