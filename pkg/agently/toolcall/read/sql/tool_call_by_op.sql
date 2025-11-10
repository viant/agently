SELECT t.message_id, t.turn_id, t.op_id, t.trace_id, t.response_payload_id
FROM tool_call t
JOIN message m ON m.id = t.message_id
    ${predicate.Builder().CombineOr($predicate.FilterGroup(0, "AND")).Build("WHERE")}
    ${predicate.Builder().CombineOr($predicate.FilterGroup(1, "AND")).Build("AND")}

