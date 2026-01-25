SELECT
    0 AS elapsedInSec,
    '' AS stage,
    '' AS toolFeed,
    t.id,
    t.conversation_id,
    t.created_at,
    t.queue_seq,
    t.status,
    t.error_message,
    t.started_by_message_id,
    t.retry_of,
    t.agent_id_used,
    t.agent_config_used_id,
    t.model_override_provider,
    t.model_override,
    t.model_params_override
FROM turn t
WHERE (t.status = 'running' OR t.status = 'waiting_for_user')
${predicate.Builder().CombineOr($predicate.FilterGroup(0, "AND")).Build("AND")}
ORDER BY t.created_at DESC, t.id DESC
LIMIT 1
