SELECT m.* FROM message m WHERE m.attachment_payload_id IS NULL
    ${predicate.Builder().CombineOr($predicate.FilterGroup(4, "AND")).Build("AND")}