SELECT 
  m.conversation_id,
  mc.provider,
  mc.model,
  SUM(COALESCE(mc.prompt_tokens, 0))     AS total_prompt_tokens,
  SUM(COALESCE(mc.completion_tokens, 0)) AS total_completion_tokens,
  SUM(COALESCE(mc.total_tokens, 0))      AS total_tokens,
  SUM(COALESCE(mc.cost, 0.0))            AS total_cost,
  COUNT(1)                                AS calls_count,
  SUM(CASE WHEN COALESCE(mc.cache_hit,0)=1 THEN 1 ELSE 0 END) AS cached_calls,
  MIN(mc.started_at)                      AS first_call_at,
  MAX(mc.completed_at)                    AS last_call_at
FROM model_calls mc
JOIN message m ON m.id = mc.message_id
${predicate.Builder().CombineOr($predicate.FilterGroup(0, "AND")).Build("WHERE")} 
GROUP BY m.conversation_id, mc.provider, mc.model

