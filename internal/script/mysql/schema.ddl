-- MySQL 8 schema (no JSON; types close to original SQLite)
-- Engine/charset
SET NAMES utf8mb4;

SET FOREIGN_KEY_CHECKS = 0;

DROP TABLE IF EXISTS model_call;
DROP TABLE IF EXISTS tool_call;
DROP TABLE IF EXISTS call_payload;
DROP TABLE IF EXISTS `message`;
DROP TABLE IF EXISTS turn;
DROP TABLE IF EXISTS conversation;

SET FOREIGN_KEY_CHECKS = 1;

-- =========================
-- conversation
-- =========================
CREATE TABLE conversation
(
    id                     VARCHAR(255) PRIMARY KEY,
    -- legacy-friendly columns
    summary                TEXT,
    last_activity          TIMESTAMP    NULL     DEFAULT NULL,
    usage_input_tokens     BIGINT                DEFAULT 0,
    usage_output_tokens    BIGINT                DEFAULT 0,
    usage_embedding_tokens BIGINT                DEFAULT 0,

    -- v2 columns
    created_at             TIMESTAMP    NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at             TIMESTAMP    NULL     DEFAULT NULL,
    created_by_user_id     VARCHAR(255),
    agent_id               VARCHAR(255),
    default_model_provider TEXT,
    default_model          TEXT,
    default_model_params   TEXT,
    title                  TEXT,
    conversation_parent_id       VARCHAR(255),
    conversation_parent_turn_id  VARCHAR(255),
    metadata               TEXT,
    visibility             VARCHAR(255) NOT NULL DEFAULT 'private',
    status                 VARCHAR(255),

    -- scheduling annotations
    scheduled              TINYINT      NULL CHECK (scheduled IN (0,1)),
    schedule_id            VARCHAR(255) NULL,
    schedule_run_id        VARCHAR(255) NULL,
    schedule_kind          VARCHAR(32)  NULL,
    schedule_timezone      VARCHAR(64)  NULL,
    schedule_cron_expr     VARCHAR(255) NULL,
    -- external task reference for A2A exposure
    external_task_ref      TEXT
) ENGINE = InnoDB
  DEFAULT CHARSET = utf8mb4
  COLLATE = utf8mb4_0900_ai_ci;

-- Optional usage breakdown table (kept for compatibility)
CREATE TABLE turn
(
    id                      VARCHAR(255) PRIMARY KEY,
    conversation_id         VARCHAR(255) NOT NULL,
    created_at              TIMESTAMP    NOT NULL DEFAULT CURRENT_TIMESTAMP,
    status                  VARCHAR(255) NOT NULL CHECK (status IN
                                                         ('pending', 'running', 'waiting_for_user', 'succeeded',
                                                          'failed', 'canceled')),
    error_message           TEXT,
    started_by_message_id   VARCHAR(255),
    retry_of                VARCHAR(255),
    agent_id_used           VARCHAR(255),
    agent_config_used_id    VARCHAR(255),
    model_override_provider TEXT,
    model_override          TEXT,
    model_params_override   TEXT,

    CONSTRAINT fk_turn_conversation
        FOREIGN KEY (conversation_id) REFERENCES conversation (id) ON DELETE CASCADE
);

CREATE INDEX idx_turn_conversation ON turn (conversation_id);



CREATE TABLE call_payload
(
    id                       VARCHAR(255) PRIMARY KEY,
    tenant_id                VARCHAR(255),
    kind                     VARCHAR(255) NOT NULL CHECK (kind IN
                                                          ('model_request', 'model_response', 'provider_request',
                                                           'provider_response', 'model_stream', 'tool_request',
                                                           'tool_response', 'elicitation_request',
                                                           'elicitation_response')),
    subtype                  TEXT,
    mime_type                TEXT         NOT NULL,
    size_bytes               BIGINT       NOT NULL,
    digest                   VARCHAR(255),
    storage                  VARCHAR(255) NOT NULL CHECK (storage IN ('inline', 'object')),
    inline_body              LONGBLOB,
    uri                      TEXT,
    compression              VARCHAR(255) NOT NULL DEFAULT 'none' CHECK (compression IN ('none', 'gzip', 'zstd')),
    encryption_kms_key_id    TEXT,
    redaction_policy_version TEXT,
    redacted                 BIGINT       NOT NULL DEFAULT 0 CHECK (redacted IN (0, 1)),
    created_at               TIMESTAMP    NOT NULL DEFAULT CURRENT_TIMESTAMP,
    schema_ref               TEXT,
    
    CHECK (
        (storage = 'inline' AND inline_body IS NOT NULL) OR
        (storage = 'object' AND inline_body IS NULL)
        )
);

CREATE INDEX idx_payload_tenant_kind ON call_payload (tenant_id, kind, created_at);
CREATE INDEX idx_payload_digest ON call_payload (digest);


CREATE TABLE `message`
(
    id                     VARCHAR(255) PRIMARY KEY,
    conversation_id        VARCHAR(255) NOT NULL,
    turn_id                VARCHAR(255),
    archived               TINYINT      NULL,
    sequence               BIGINT,
    created_at             TIMESTAMP    NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at             TIMESTAMP,
    created_by_user_id     VARCHAR(255),
    status                 VARCHAR(255) CHECK (status IS NULL OR status IN
                                                                 ('', 'pending', 'accepted', 'rejected', 'cancel',
                                                                  'open', 'summary', 'summarized','completed','error')),
    mode                   VARCHAR(255),
    role                   VARCHAR(255) NOT NULL CHECK (role IN ('system', 'user', 'assistant', 'tool', 'chain')),
    `type`                 VARCHAR(255) NOT NULL DEFAULT 'text' CHECK (`type` IN ('text', 'tool_op', 'control')),
    content                MEDIUMTEXT,
    summary                TEXT,
    context_summary        TEXT,
    tags                   TEXT,
    interim                BIGINT       NOT NULL DEFAULT 0 CHECK (interim IN (0, 1)),
    elicitation_id         VARCHAR(255),
    parent_message_id      VARCHAR(255),
    superseded_by          VARCHAR(255),
    linked_conversation_id VARCHAR(255),
    attachment_payload_id  VARCHAR(255),
    elicitation_payload_id VARCHAR(255),
    -- legacy column to remain compatible with older readers
    tool_name              TEXT,
    embedding_index        LONGBLOB,

    CONSTRAINT fk_message_conversation
        FOREIGN KEY (conversation_id) REFERENCES conversation (id) ON DELETE CASCADE,
    CONSTRAINT fk_message_turn
        FOREIGN KEY (turn_id) REFERENCES turn (id) ON DELETE SET NULL,
    CONSTRAINT fk_message_attachment_payload
        FOREIGN KEY (attachment_payload_id) REFERENCES call_payload (id) ON DELETE SET NULL,
    CONSTRAINT fk_message_elicitation_payload
        FOREIGN KEY (elicitation_payload_id) REFERENCES call_payload (id) ON DELETE SET NULL
);

CREATE UNIQUE INDEX idx_message_turn_seq ON `message` (turn_id, sequence);
CREATE INDEX idx_msg_conv_created ON `message` (conversation_id, created_at DESC);

-- Users table for identity and schedule UX state
CREATE TABLE users (
    id                                   VARCHAR(255) PRIMARY KEY,
    username                             VARCHAR(255) NOT NULL UNIQUE,
    display_name                         VARCHAR(255),
    email                                VARCHAR(255),
    provider                             VARCHAR(255) NOT NULL DEFAULT 'local',
    subject                              VARCHAR(255),
    hash_ip                              VARCHAR(255),
    timezone                             VARCHAR(64)  NOT NULL DEFAULT 'UTC',
    default_agent_ref                    VARCHAR(255),
    default_model_ref                    VARCHAR(255),
    default_embedder_ref                 VARCHAR(255),
    settings                             TEXT,
    disabled                             BIGINT       NOT NULL DEFAULT 0,
    created_at                           TIMESTAMP    NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at                           TIMESTAMP    NULL DEFAULT NULL
) ENGINE = InnoDB DEFAULT CHARSET = utf8mb4 COLLATE = utf8mb4_0900_ai_ci;

-- Unique subject per provider (NULL subject allowed for local)
CREATE UNIQUE INDEX ux_users_provider_subject ON users(provider, subject);
CREATE INDEX ix_users_hash_ip ON users(hash_ip);

CREATE TABLE model_call
(
    message_id                            VARCHAR(255) PRIMARY KEY,
    turn_id                               VARCHAR(255),
    provider                              TEXT         NOT NULL,
    model                                 VARCHAR(255) NOT NULL,
    model_kind                            VARCHAR(255) NOT NULL CHECK (model_kind IN
                                                                       ('chat', 'completion', 'vision', 'reranker',
                                                                        'embedding', 'other')),
    
    
    error_code                            TEXT,
    error_message                         TEXT,
    finish_reason                         TEXT,
    prompt_tokens                         BIGINT,
    prompt_cached_tokens                  BIGINT,
    completion_tokens                     BIGINT,
    total_tokens                          BIGINT,
    prompt_audio_tokens                   BIGINT,
    completion_reasoning_tokens           BIGINT,
    completion_audio_tokens               BIGINT,
    completion_accepted_prediction_tokens BIGINT,
    completion_rejected_prediction_tokens BIGINT,
    status                                VARCHAR(255) NOT NULL CHECK (status IN ('thinking', 'streaming','running', 'completed', 'failed', 'canceled')),
    started_at                            TIMESTAMP    NULL     DEFAULT NULL,
    completed_at                          TIMESTAMP    NULL     DEFAULT NULL,
    latency_ms                            BIGINT,
    cost                                  DOUBLE,
    
    trace_id                              TEXT,
    span_id                               TEXT,
    
    request_payload_id                    VARCHAR(255),
    response_payload_id                   VARCHAR(255),
    provider_request_payload_id           VARCHAR(255),
    provider_response_payload_id          VARCHAR(255),
    stream_payload_id                     VARCHAR(255),

    CONSTRAINT fk_model_calls_message
        FOREIGN KEY (message_id) REFERENCES `message` (id) ON DELETE CASCADE,
    CONSTRAINT fk_model_call_turn
        FOREIGN KEY (turn_id) REFERENCES turn (id) ON DELETE SET NULL,
    CONSTRAINT fk_model_call_req_payload
        FOREIGN KEY (request_payload_id) REFERENCES call_payload (id) ON DELETE SET NULL,
    CONSTRAINT fk_model_call_res_payload
        FOREIGN KEY (response_payload_id) REFERENCES call_payload (id) ON DELETE SET NULL,
    CONSTRAINT fk_model_call_provider_req_payload
        FOREIGN KEY (provider_request_payload_id) REFERENCES call_payload (id) ON DELETE SET NULL,
    CONSTRAINT fk_model_call_provider_res_payload
        FOREIGN KEY (provider_response_payload_id) REFERENCES call_payload (id) ON DELETE SET NULL,
    CONSTRAINT fk_model_call_stream_payload
        FOREIGN KEY (stream_payload_id) REFERENCES call_payload (id) ON DELETE SET NULL
);

CREATE INDEX idx_model_call_model ON model_call (model);
CREATE INDEX idx_model_call_started_at ON model_call (started_at);

CREATE TABLE tool_call
(
    message_id          VARCHAR(255) PRIMARY KEY,
    turn_id             VARCHAR(255),
    op_id               TEXT NOT NULL,
    attempt             BIGINT       NOT NULL DEFAULT 1,
    tool_name           VARCHAR(255) NOT NULL,
    tool_kind           VARCHAR(255) NOT NULL CHECK (tool_kind IN ('general', 'resource')),
    status              VARCHAR(255) NOT NULL CHECK (status IN ('queued', 'running', 'completed', 'failed', 'skipped',
                                                                'canceled')),
    -- request_ref removed
    request_hash        TEXT,
    error_code          TEXT,
    error_message       TEXT,
    retriable           BIGINT CHECK (retriable IN (0, 1)),
    started_at          TIMESTAMP    NULL     DEFAULT NULL,
    completed_at        TIMESTAMP    NULL     DEFAULT NULL,
    latency_ms          BIGINT,
    cost                DOUBLE,
    trace_id            TEXT,
    span_id             TEXT,
    request_payload_id  VARCHAR(255),
    response_payload_id VARCHAR(255),
    

    CONSTRAINT fk_tool_call_message
        FOREIGN KEY (message_id) REFERENCES `message` (id) ON DELETE CASCADE,
    CONSTRAINT fk_tool_call_turn
        FOREIGN KEY (turn_id) REFERENCES turn (id) ON DELETE SET NULL,
    CONSTRAINT fk_tool_call_req_payload
        FOREIGN KEY (request_payload_id) REFERENCES call_payload (id) ON DELETE SET NULL,
    CONSTRAINT fk_tool_call_res_payload
        FOREIGN KEY (response_payload_id) REFERENCES call_payload (id) ON DELETE SET NULL
);

CREATE UNIQUE INDEX idx_tool_op_attempt ON tool_call (turn_id, op_id, attempt);
CREATE INDEX idx_tool_call_status ON tool_call (status);
CREATE INDEX idx_tool_call_name ON tool_call (tool_name);
CREATE INDEX idx_tool_call_op ON tool_call (turn_id, op_id(191));
CREATE UNIQUE INDEX idx_tool_op_attempt ON tool_call (turn_id, op_id(191), attempt);

CREATE TABLE IF NOT EXISTS schedule (
                                        id                    VARCHAR(255) PRIMARY KEY,
                                        name                  VARCHAR(255) NOT NULL UNIQUE,
                                        description           TEXT,

    -- Target agent / model
                                        agent_ref             VARCHAR(255) NOT NULL,
                                        model_override        VARCHAR(255),

    -- Enable/disable + time window
                                        enabled               TINYINT      NOT NULL DEFAULT 1 CHECK (enabled IN (0,1)),
                                        start_at              TIMESTAMP    NULL DEFAULT NULL,
                                        end_at                TIMESTAMP    NULL DEFAULT NULL,

    -- Frequency
                                        schedule_type         VARCHAR(32)  NOT NULL DEFAULT 'cron' CHECK (schedule_type IN ('adhoc','cron','interval')),
                                        cron_expr             VARCHAR(255),
                                        interval_seconds      BIGINT,
                                        timezone              VARCHAR(64)  NOT NULL DEFAULT 'UTC',

    -- Task payload (predefined user task)
                                        task_prompt_uri       TEXT,
                                        task_prompt           MEDIUMTEXT,

    -- Optional orchestration workflow (reserved)

    -- Bookkeeping
                                        next_run_at           TIMESTAMP    NULL DEFAULT NULL,
                                        last_run_at           TIMESTAMP    NULL DEFAULT NULL,
                                        last_status           VARCHAR(32),
                                        last_error            TEXT,
                                        created_at            TIMESTAMP    NOT NULL DEFAULT CURRENT_TIMESTAMP,
                                        updated_at            TIMESTAMP    NULL DEFAULT NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE INDEX idx_schedule_enabled_next ON schedule(enabled, next_run_at);

-- Per-run audit trail
CREATE TABLE IF NOT EXISTS schedule_run (
                                            id                     VARCHAR(255) PRIMARY KEY,
                                            schedule_id            VARCHAR(255) NOT NULL,
                                            created_at             TIMESTAMP    NOT NULL DEFAULT CURRENT_TIMESTAMP,
                                            updated_at             TIMESTAMP    NULL DEFAULT NULL,
                                            status                 VARCHAR(32)  NOT NULL DEFAULT 'pending' CHECK (status IN ('pending','prechecking','skipped','running','succeeded','failed')),
                                            error_message          TEXT,

                                            precondition_ran_at    TIMESTAMP    NULL DEFAULT NULL,
                                            precondition_passed    TINYINT      NULL CHECK (precondition_passed IN (0,1)),
                                            precondition_result    MEDIUMTEXT,

                                            conversation_id        VARCHAR(255) NULL,
                                            conversation_kind      VARCHAR(32)  NOT NULL DEFAULT 'scheduled' CHECK (conversation_kind IN ('scheduled','precondition')),
                                            started_at             TIMESTAMP    NULL DEFAULT NULL,
                                            completed_at           TIMESTAMP    NULL DEFAULT NULL,

                                            CONSTRAINT fk_run_schedule FOREIGN KEY (schedule_id) REFERENCES schedule(id) ON DELETE CASCADE,
                                            CONSTRAINT fk_run_conversation FOREIGN KEY (conversation_id) REFERENCES conversation(id) ON DELETE SET NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE INDEX idx_run_schedule_status ON schedule_run(schedule_id, status);

-- OAuth tokens per user (server-side, encrypted). Stores serialized scy/auth.Token as enc_token.
CREATE TABLE IF NOT EXISTS user_oauth_token (
  user_id     VARCHAR(255) NOT NULL,
  provider    VARCHAR(128) NOT NULL,
  enc_token   TEXT         NOT NULL,
  created_at  TIMESTAMP    NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at  TIMESTAMP    NULL DEFAULT NULL,
  PRIMARY KEY (user_id, provider),
  CONSTRAINT fk_uot_user FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;
