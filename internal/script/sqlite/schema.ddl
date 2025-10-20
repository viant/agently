-- v2 schema aligned with proposed design. Includes legacy-friendly columns.

PRAGMA
foreign_keys = ON;

DROP TABLE IF EXISTS model_call;
DROP TABLE IF EXISTS tool_call;
DROP TABLE IF EXISTS call_payload;
DROP TABLE IF EXISTS turn;
DROP TABLE IF EXISTS message;
DROP TABLE IF EXISTS conversation;

CREATE TABLE conversation
(
    id                     TEXT PRIMARY KEY,
    -- legacy-friendly columns
    summary                TEXT,
    last_activity          TIMESTAMP,
    usage_input_tokens     INT                DEFAULT 0,
    usage_output_tokens    INT                DEFAULT 0,
    usage_embedding_tokens INT                DEFAULT 0,

    -- v2 columns
    created_at             TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at             TIMESTAMP,
    created_by_user_id     TEXT,
    tenant_id              TEXT,
    agent_id               TEXT,
    agent_config_id        TEXT,
    default_model_provider TEXT,
    default_model          TEXT,
    default_model_params   TEXT,
    title                  TEXT,
    conversation_parent_id       TEXT,
    conversation_parent_turn_id  TEXT,
    metadata               TEXT,
    visibility             TEXT      NOT NULL DEFAULT 'private',
    archived               INTEGER   NOT NULL DEFAULT 0,
    deleted_at             TIMESTAMP,
    last_message_at        TIMESTAMP,
    message_count          INTEGER   NOT NULL DEFAULT 0,
    turn_count             INTEGER   NOT NULL DEFAULT 0,
    retention_ttl_days     INTEGER,
    expires_at             TIMESTAMP,
    status                 VARCHAR(255),

    -- scheduling annotations
    scheduled              INTEGER   CHECK (scheduled IN (0,1)),
    schedule_id            TEXT,
    schedule_run_id        TEXT,
    schedule_kind          TEXT,
    schedule_timezone      TEXT,
    schedule_cron_expr     TEXT,

    -- client attribution
    created_ip             TEXT,
    last_ip                TEXT
);

-- Optional usage breakdown table (kept for compatibility)

CREATE TABLE turn
(
    id                      TEXT PRIMARY KEY,
    conversation_id         TEXT      NOT NULL REFERENCES conversation (id) ON DELETE CASCADE,
    created_at              TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    status                  TEXT      NOT NULL CHECK (status IN
                                                      ('pending', 'running', 'waiting_for_user', 'succeeded', 'failed',
                                                       'canceled')),
    error_message TEXT,
    started_by_message_id   TEXT,
    retry_of                TEXT,
    agent_id_used           TEXT,
    agent_config_used_id    TEXT,
    model_override_provider TEXT,
    model_override          TEXT,
    model_params_override   TEXT
);

CREATE INDEX idx_turn_conversation ON turn (conversation_id);

CREATE TABLE message
(
    id                 TEXT PRIMARY KEY,
    archived           INTEGER   CHECK (archived IN (0, 1)),
    conversation_id    TEXT      NOT NULL REFERENCES conversation (id) ON DELETE CASCADE,
    turn_id            TEXT      REFERENCES turn (id) ON DELETE SET NULL,
    sequence           INTEGER,
    created_at         TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at         TIMESTAMP,
    created_by_user_id TEXT,
    status             TEXT CHECK (status IS NULL OR status IN ('', 'pending','accepted','rejected','cancel','open','summary','summarized', 'completed')),
    mode               TEXT,
    role               TEXT      NOT NULL CHECK (role IN ('system', 'user', 'assistant', 'tool', 'chain')),
    type               TEXT      NOT NULL DEFAULT 'text' CHECK (type IN ('text', 'tool_op',  'control')),
    content            TEXT,
    summary            TEXT,
    context_summary    TEXT,
    tags               TEXT,
    interim            INTEGER   NOT NULL DEFAULT 0 CHECK (interim IN (0, 1)),
    elicitation_id     TEXT,
    parent_message_id  TEXT,
    superseded_by      TEXT,
    linked_conversation_id  TEXT,
    attachment_payload_id  TEXT REFERENCES call_payload (id) ON DELETE SET NULL,
    elicitation_payload_id TEXT REFERENCES call_payload (id) ON DELETE SET NULL,
    -- legacy column to remain compatible with older readers
    tool_name          TEXT,
    embedding_index    BLOB
);


CREATE UNIQUE INDEX idx_message_turn_seq ON message (turn_id, sequence);
CREATE INDEX idx_msg_conv_created ON message (conversation_id, created_at DESC);
-- IP audit indexes
CREATE INDEX idx_conv_created_ip ON conversation (created_ip);
CREATE INDEX idx_conv_last_ip    ON conversation (last_ip);

-- Removed app_user table; consolidated into singular 'user'

CREATE TABLE call_payload
(
    id                       TEXT PRIMARY KEY,
    tenant_id                TEXT,
    kind                     TEXT      NOT NULL CHECK (kind IN ('model_request', 'model_response', 'provider_request',
                                                                'provider_response', 'model_stream', 'tool_request',
                                                                'tool_response', 'elicitation_request', 'elicitation_response')),
    subtype                  TEXT,
    mime_type                TEXT      NOT NULL,
    size_bytes               INTEGER   NOT NULL,
    digest                   TEXT,
    storage                  TEXT      NOT NULL CHECK (storage IN ('inline', 'object')),
    inline_body              BLOB,
    uri                      TEXT,
    compression              TEXT      NOT NULL DEFAULT 'none' CHECK (compression IN ('none', 'gzip', 'zstd')),
    encryption_kms_key_id    TEXT,
    redaction_policy_version TEXT,
    redacted                 INTEGER   NOT NULL DEFAULT 0 CHECK (redacted IN (0, 1)),
    created_at               TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    schema_ref               TEXT,
    CHECK ((storage = 'inline' AND inline_body IS NOT NULL) OR
           (storage = 'object' AND inline_body IS NULL))
);
CREATE INDEX idx_payload_tenant_kind ON call_payload (tenant_id, kind, created_at);
CREATE INDEX idx_payload_digest ON call_payload (digest);

CREATE TABLE model_call
(
    message_id                   TEXT PRIMARY KEY REFERENCES message (id) ON DELETE CASCADE,
    turn_id                      TEXT    REFERENCES turn (id) ON DELETE SET NULL,
    provider                     TEXT    NOT NULL,
    model                        TEXT    NOT NULL,
    model_kind                   TEXT    NOT NULL CHECK (model_kind IN('chat', 'completion', 'vision', 'reranker', 'embedding','other')),
    status                       TEXT CHECK (status IN ('thinking', 'streaming','running', 'completed', 'failed', 'canceled')),
    
    error_code                   TEXT,
    error_message                TEXT,
    finish_reason                TEXT,
    prompt_tokens                INTEGER,
    prompt_cached_tokens         INTEGER,
    completion_tokens            INTEGER,
    total_tokens                 INTEGER,
    prompt_audio_tokens          INTEGER,
    completion_reasoning_tokens  INTEGER,
    completion_audio_tokens      INTEGER,
    completion_accepted_prediction_tokens   INTEGER,
    completion_rejected_prediction_tokens   INTEGER,
    started_at                   TIMESTAMP,
    completed_at                 TIMESTAMP,
    latency_ms                   INTEGER,
    
    cost                         REAL,
    
    redaction_policy_version     TEXT,
    redacted                     INTEGER NOT NULL DEFAULT 0 CHECK (redacted IN (0, 1)),
    trace_id                     TEXT,
    span_id                      TEXT,
    request_payload_id           TEXT    REFERENCES call_payload (id) ON DELETE SET NULL,
    response_payload_id          TEXT    REFERENCES call_payload (id) ON DELETE SET NULL,
    provider_request_payload_id  TEXT    REFERENCES call_payload (id) ON DELETE SET NULL,
    provider_response_payload_id TEXT    REFERENCES call_payload (id) ON DELETE SET NULL,
    stream_payload_id            TEXT    REFERENCES call_payload (id) ON DELETE SET NULL
);

CREATE INDEX idx_model_call_model ON model_call (model);
CREATE INDEX idx_model_call_started_at ON model_call (started_at);

CREATE TABLE tool_call
(
    message_id          TEXT PRIMARY KEY REFERENCES message (id) ON DELETE CASCADE,
    turn_id             TEXT    REFERENCES turn (id) ON DELETE SET NULL,
    op_id               TEXT    NOT NULL,
    attempt             INTEGER NOT NULL DEFAULT 1,
    tool_name           TEXT    NOT NULL,
    tool_kind           TEXT    NOT NULL CHECK (tool_kind IN ('general', 'resource')),
    status              TEXT    NOT NULL CHECK (status IN
                                                ('queued', 'running', 'completed', 'failed', 'skipped', 'canceled')),
    request_ref         TEXT,
    request_hash        TEXT,
    error_code          TEXT,
    error_message       TEXT,
    retriable           INTEGER CHECK (retriable IN (0, 1)),
    started_at          TIMESTAMP,
    completed_at        TIMESTAMP,
    latency_ms          INTEGER,
    cost                REAL,
    trace_id            TEXT,
    span_id             TEXT,
    request_payload_id  TEXT    REFERENCES call_payload (id) ON DELETE SET NULL,
    response_payload_id TEXT    REFERENCES call_payload (id) ON DELETE SET NULL,
    response_overflow   INTEGER CHECK (response_overflow IN (0,1))
);
CREATE UNIQUE INDEX idx_tool_op_attempt ON tool_call (turn_id, op_id, attempt);
CREATE INDEX idx_tool_call_status ON tool_call (status);
CREATE INDEX idx_tool_call_name ON tool_call (tool_name);
CREATE INDEX idx_tool_call_op ON tool_call (turn_id, op_id);



-- Main schedule definition
CREATE TABLE IF NOT EXISTS schedule (
                                        id                    TEXT PRIMARY KEY,
                                        name                  TEXT      NOT NULL UNIQUE,
                                        description           TEXT,

    -- Target agent / model
                                        agent_ref             TEXT      NOT NULL,        -- agent name or id
                                        model_override        TEXT,                      -- optional model ref override

    -- Enable/disable + time window
                                        enabled               INTEGER   NOT NULL DEFAULT 1 CHECK (enabled IN (0,1)),
    start_at              TIMESTAMP NULL,
    end_at                TIMESTAMP NULL,

    -- Frequency
    schedule_type         TEXT      NOT NULL DEFAULT 'cron' CHECK (schedule_type IN ('adhoc','cron','interval')),
    cron_expr             TEXT,                      -- when schedule_type = 'cron'
    interval_seconds      INTEGER,                   -- when schedule_type = 'interval'
    timezone              TEXT      NOT NULL DEFAULT 'UTC',

    -- Task payload (predefined user task)
    task_prompt_uri       TEXT,                      -- URI to load task content
    task_prompt           TEXT,                      -- inline content (optional)

    -- Optional orchestration workflow (reserved)

    -- Bookkeeping
    next_run_at           TIMESTAMP,
    last_run_at           TIMESTAMP,
    last_status           TEXT,                      -- succeeded/failed/skipped
    last_error            TEXT,
    created_at            TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at            TIMESTAMP
    );

CREATE INDEX IF NOT EXISTS idx_schedule_enabled_next ON schedule(enabled, next_run_at);


-- Per-run audit trail
CREATE TABLE IF NOT EXISTS schedule_run (
                                            id                     TEXT PRIMARY KEY,
                                            schedule_id            TEXT      NOT NULL REFERENCES schedule(id) ON DELETE CASCADE,
    created_at             TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at             TIMESTAMP,
    status                 TEXT      NOT NULL DEFAULT 'pending' CHECK (status IN ('pending','prechecking','skipped','running','succeeded','failed')),
    error_message          TEXT,

    -- Precondition outcome
    precondition_ran_at    TIMESTAMP,
    precondition_passed    INTEGER   CHECK (precondition_passed IN (0,1)),
    precondition_result    TEXT,

    -- Conversation spawned by this run
    conversation_id        TEXT      REFERENCES conversation(id) ON DELETE SET NULL,
    conversation_kind      TEXT      NOT NULL DEFAULT 'scheduled' CHECK (conversation_kind IN ('scheduled','precondition')),
    started_at             TIMESTAMP,
    completed_at           TIMESTAMP
    );

CREATE INDEX IF NOT EXISTS idx_run_schedule_status ON schedule_run(schedule_id, status);


CREATE TABLE IF NOT EXISTS users (
  id                 TEXT PRIMARY KEY,
  username           TEXT NOT NULL UNIQUE,
  display_name       TEXT,
  email              TEXT,
  provider           TEXT NOT NULL DEFAULT 'local',
  subject            TEXT,
  hash_ip            TEXT,
  timezone           TEXT NOT NULL DEFAULT 'UTC',
  default_agent_ref  TEXT,
  default_model_ref  TEXT,
  default_embedder_ref TEXT,
  settings           TEXT,
  disabled           INTEGER NOT NULL DEFAULT 0,
  created_at         DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at         DATETIME
);

-- Unique subject per provider (NULL subject allowed for local)
CREATE UNIQUE INDEX IF NOT EXISTS ux_users_provider_subject ON users(provider, subject);
CREATE INDEX IF NOT EXISTS ix_users_hash_ip ON users(hash_ip);

-- OAuth tokens per user (server-side, encrypted). Stores serialized scy/auth.Token as enc_token.
CREATE TABLE IF NOT EXISTS user_oauth_token (
  user_id     TEXT      NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  provider    TEXT      NOT NULL,
  enc_token   TEXT      NOT NULL,
  created_at  DATETIME  NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at  DATETIME,
  PRIMARY KEY (user_id, provider)
);
