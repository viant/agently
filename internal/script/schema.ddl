-- v2 schema aligned with proposed design. Includes legacy-friendly columns.

PRAGMA foreign_keys = ON;

DROP TABLE IF EXISTS model_calls;
DROP TABLE IF EXISTS tool_calls;
DROP TABLE IF EXISTS call_payloads;
DROP TABLE IF EXISTS turns;
DROP TABLE IF EXISTS message;
DROP TABLE IF EXISTS conversation;

CREATE TABLE conversation (
    id                         TEXT PRIMARY KEY,
    -- legacy-friendly columns
    summary                    TEXT,
    agent_name                 TEXT,
    last_activity              TIMESTAMP,
    usage_input_tokens         INT DEFAULT 0,
    usage_output_tokens        INT DEFAULT 0,
    usage_embedding_tokens     INT DEFAULT 0,

    -- v2 columns
    created_at                 TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at                 TIMESTAMP,
    created_by_user_id         TEXT,
    tenant_id                  TEXT,
    agent_id                   TEXT,
    agent_config_id            TEXT,
    default_model_provider     TEXT,
    default_model              TEXT,
    default_model_params       TEXT,
    title                      TEXT,
    metadata                   TEXT,
    visibility                 TEXT NOT NULL DEFAULT 'private',
    archived                   INTEGER NOT NULL DEFAULT 0,
    deleted_at                 TIMESTAMP,
    last_message_at            TIMESTAMP,
    last_turn_id               TEXT,
    message_count              INTEGER NOT NULL DEFAULT 0,
    turn_count                 INTEGER NOT NULL DEFAULT 0,
    retention_ttl_days         INTEGER,
    expires_at                 TIMESTAMP
);

-- Optional usage breakdown table (kept for compatibility)

CREATE TABLE turns (
    id                         TEXT PRIMARY KEY,
    conversation_id            TEXT NOT NULL REFERENCES conversation(id) ON DELETE CASCADE,
    created_at                 TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    status                     TEXT NOT NULL CHECK (status IN ('pending','running','waiting_for_user','succeeded','failed','canceled')),
    started_by_message_id      TEXT,
    retry_of                   TEXT,
    agent_id_used              TEXT,
    agent_config_used_id       TEXT,
    model_override_provider    TEXT,
    model_override             TEXT,
    model_params_override      TEXT
);

CREATE INDEX idx_turns_conversation ON turns(conversation_id);

CREATE TABLE message (
    id                  TEXT PRIMARY KEY,
    conversation_id     TEXT NOT NULL REFERENCES conversation(id) ON DELETE CASCADE,
    turn_id             TEXT REFERENCES turns(id) ON DELETE SET NULL,
    sequence            INTEGER,
    created_at          TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    status              TEXT,
    role                TEXT NOT NULL CHECK (role IN ('system','user','assistant','tool','control')),
    type                TEXT NOT NULL DEFAULT 'text' CHECK (type IN ('text','tool_op','control')),
    content             TEXT NOT NULL,
    context_summary     TEXT,
    tags                TEXT,
    interim             INTEGER NOT NULL DEFAULT 0 CHECK (interim IN (0,1)),
    elicitation_id      TEXT,
    parent_message_id   TEXT,
    superseded_by       TEXT,
    -- legacy column to remain compatible with older readers
    tool_name           TEXT
);


CREATE UNIQUE INDEX idx_message_turn_seq ON message(turn_id, sequence);
CREATE INDEX idx_msg_conv_created ON message (conversation_id, created_at DESC);

CREATE TABLE call_payloads (
    id                         TEXT PRIMARY KEY,
    tenant_id                  TEXT,
    kind                       TEXT NOT NULL CHECK (kind IN ('model_request','model_response','tool_request','tool_response', 'elicitation_request')),
    subtype                    TEXT,
    mime_type                  TEXT NOT NULL,
    size_bytes                 INTEGER NOT NULL,
    digest                     TEXT,
    storage                    TEXT NOT NULL CHECK (storage IN ('inline','object')),
    inline_body                BLOB,
    uri                        TEXT,
    compression                TEXT NOT NULL DEFAULT 'none' CHECK (compression IN ('none','gzip','zstd')),
    encryption_kms_key_id      TEXT,
    redaction_policy_version   TEXT,
    redacted                   INTEGER NOT NULL DEFAULT 0 CHECK (redacted IN (0,1)),
    created_at                 TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    schema_ref                 TEXT,
    preview                    TEXT,
    tags                       TEXT,
    CHECK ((storage='inline' AND inline_body IS NOT NULL AND uri IS NULL) OR (storage='object' AND uri IS NOT NULL AND inline_body IS NULL))
);
CREATE INDEX idx_payloads_tenant_kind ON call_payloads(tenant_id, kind, created_at);
CREATE INDEX idx_payloads_digest ON call_payloads(digest);

CREATE TABLE model_calls (
    message_id                 TEXT PRIMARY KEY REFERENCES message(id) ON DELETE CASCADE,
    turn_id                    TEXT REFERENCES turns(id) ON DELETE SET NULL,
    provider                   TEXT NOT NULL,
    model                      TEXT NOT NULL,
    model_kind                 TEXT NOT NULL CHECK (model_kind IN ('chat','completion','vision','reranker','embedding','other')),
    prompt_snapshot            TEXT,
    prompt_ref                 TEXT,
    prompt_hash                TEXT,
    response_snapshot          TEXT,
    response_ref               TEXT,
    finish_reason              TEXT,
    prompt_tokens              INTEGER,
    completion_tokens          INTEGER,
    total_tokens               INTEGER,
    started_at                 TIMESTAMP,
    completed_at               TIMESTAMP,
    latency_ms                 INTEGER,
    cache_hit                  INTEGER NOT NULL DEFAULT 0 CHECK (cache_hit IN (0,1)),
    cache_key                  TEXT,
    cost                       REAL,
    safety_blocked             INTEGER CHECK (safety_blocked IN (0,1)),
    safety_reasons             TEXT,
    redaction_policy_version   TEXT,
    redacted                   INTEGER NOT NULL DEFAULT 0 CHECK (redacted IN (0,1)),
    trace_id                   TEXT,
    span_id                    TEXT,
    request_payload_id         TEXT REFERENCES call_payloads(id) ON DELETE SET NULL,
    response_payload_id        TEXT REFERENCES call_payloads(id) ON DELETE SET NULL
);

CREATE INDEX idx_model_calls_model ON model_calls(model);
CREATE INDEX idx_model_calls_started_at ON model_calls(started_at);

CREATE TABLE tool_calls (
    message_id                 TEXT PRIMARY KEY REFERENCES message(id) ON DELETE CASCADE,
    turn_id                    TEXT REFERENCES turns(id) ON DELETE SET NULL,
    op_id                      TEXT NOT NULL,
    attempt                    INTEGER NOT NULL DEFAULT 1,
    tool_name                  TEXT NOT NULL,
    tool_kind                  TEXT NOT NULL CHECK (tool_kind IN ('general','resource')),
    capability_tags            TEXT,
    resource_uris              TEXT,
    status                     TEXT NOT NULL CHECK (status IN ('queued','running','completed','failed','skipped','canceled')),
    request_snapshot           TEXT,
    request_ref                TEXT,
    request_hash               TEXT,
    response_snapshot          TEXT,
    response_ref               TEXT,
    error_code                 TEXT,
    error_message              TEXT,
    retriable                  INTEGER CHECK (retriable IN (0,1)),
    started_at                 TIMESTAMP,
    completed_at               TIMESTAMP,
    latency_ms                 INTEGER,
    cost                       REAL,
    trace_id                   TEXT,
    span_id                    TEXT,
    request_payload_id         TEXT REFERENCES call_payloads(id) ON DELETE SET NULL,
    response_payload_id        TEXT REFERENCES call_payloads(id) ON DELETE SET NULL
);
CREATE UNIQUE INDEX idx_tool_ops_attempt ON tool_calls(turn_id, op_id, attempt);
CREATE INDEX idx_tool_calls_status ON tool_calls(status);
CREATE INDEX idx_tool_calls_name ON tool_calls(tool_name);
CREATE INDEX idx_tool_calls_op ON tool_calls(turn_id, op_id);
