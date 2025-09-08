SELECT 
  m.id,
  m.conversation_id,
  m.turn_id,
  m.sequence,
  m.created_at,
  m.role,
  m.type,
  m.content,
  m.context_summary,
  m.tags,
  m.interim,
  m.elicitation_id,
  m.parent_message_id,
  m.superseded_by,
  m.tool_name
FROM message m
${predicate.Builder().CombineOr($predicate.FilterGroup(0, "AND")).Build("WHERE")} 
