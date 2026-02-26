SELECT
  gf.id,
  gf.conversation_id,
  gf.turn_id,
  gf.message_id,
  gf.provider,
  gf.mode,
  gf.copy_mode,
  gf.status,
  gf.payload_id,
  gf.container_id,
  gf.provider_file_id,
  gf.filename,
  gf.mime_type,
  gf.size_bytes,
  gf.checksum,
  gf.error_message,
  gf.expires_at,
  gf.created_at,
  gf.updated_at
FROM generated_file gf
${predicate.Builder().CombineOr($predicate.FilterGroup(0, "AND")).Build("WHERE")}
ORDER BY gf.created_at ASC
