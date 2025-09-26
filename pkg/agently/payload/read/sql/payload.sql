SELECT 
  p.id,
  p.tenant_id,
  p.kind,
  p.subtype,
  p.mime_type,
  p.size_bytes,
  p.digest,
  p.storage,
  p.inline_body,
  p.uri,
  p.compression,
  p.encryption_kms_key_id,
  p.redaction_policy_version,
  p.redacted,
  p.created_at,
  p.schema_ref,
  p.preview,
  p.tags
FROM call_payload p
${predicate.Builder().CombineOr($predicate.FilterGroup(0, "AND")).Build("WHERE")} 
