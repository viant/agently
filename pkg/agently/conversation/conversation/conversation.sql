( SELECT t.*,
          (SELECT id
           FROM turn
           WHERE conversation_id = t.id
           ORDER BY created_at DESC
           LIMIT 1) AS last_turn_id,
    '' AS stage FROM conversation t WHERE (id = $criteria.AppendBinding($Unsafe.Id) OR "" = $criteria.AppendBinding($Unsafe.Id)) )