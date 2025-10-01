( SELECT t.*,
          (SELECT id
           FROM turn
           WHERE conversation_id = t.id
           ORDER BY created_at DESC
           LIMIT 1) AS last_turn_id,
    '' AS stage FROM conversation t
     ${predicate.Builder().CombineOr($predicate.FilterGroup(0, "AND")).Build("WHERE")} )