SELECT
  t.id,
  t.conversation_id,
  t.created_at,
  t.status,
  t.started_by_message_id,
  t.retry_of,
  t.agent_id_used,
  t.agent_config_used_id,
  t.model_override_provider,
  t.model_override,
  t.model_params_override
FROM turn t
${predicate.Builder().CombineOr($predicate.FilterGroup(0, "AND")).Build("WHERE")} 
