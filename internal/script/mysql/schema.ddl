-- MySQL 8 schema (no JSON; types close to original SQLite)
-- Engine/charset
SET NAMES utf8mb4;
SET FOREIGN_KEY_CHECKS = 0;

DROP TABLE IF EXISTS model_calls;
DROP TABLE IF EXISTS tool_calls;
DROP TABLE IF EXISTS call_payloads;
DROP TABLE IF EXISTS `message`;
DROP TABLE IF EXISTS turns;
DROP TABLE IF EXISTS conversation;

SET FOREIGN_KEY_CHECKS = 1;

-- =========================
-- conversation
-- =========================
CREATE TABLE conversation (
                              id                         VARCHAR(255) PRIMARY KEY,
    -- legacy-friendly columns
                              summary                    TEXT,
                              agent_name                 TEXT,
                              last_activity              TIMESTAMP NULL DEFAULT NULL,
                              usage_input_tokens         BIGINT DEFAULT 0,
                              usage_output_tokens        BIGINT DEFAULT 0,
                              usage_embedding_tokens     BIGINT DEFAULT 0,

    -- v2 columns
                              created_at                 TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
                              updated_at                 TIMESTAMP NULL DEFAULT NULL,
                              created_by_user_id         VARCHAR(255),
                              tenant_id                  VARCHAR(255),
                              agent_id                   VARCHAR(255),
                              agent_config_id            VARCHAR(255),
                              default_model_provider     TEXT,
                              default_model              TEXT,
                              default_model_params       TEXT,
                              title                      TEXT,
                              metadata                   TEXT,
                              visibility                 VARCHAR(255) NOT NULL DEFAULT 'private',
                              archived                   BIGINT NOT NULL DEFAULT 0,
                              deleted_at                 TIMESTAMP NULL DEFAULT NULL,
                              last_message_at            TIMESTAMP NULL DEFAULT NULL,
                              last_turn_id               VARCHAR(255),
                              message_count              BIGINT NOT NULL DEFAULT 0,
                              turn_count                 BIGINT NOT NULL DEFAULT 0,
                              retention_ttl_days         BIGINT,
                              expires_at                 TIMESTAMP NULL DEFAULT NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

-- Optional usage breakdown table (kept for compatibility)
CREATE TABLE turns (
                       id                         VARCHAR(255) PRIMARY KEY,
                       conversation_id            VARCHAR(255) NOT NULL,
                       created_at                 TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
                       status                     VARCHAR(255) NOT NULL CHECK (status IN ('pending','running','waiting_for_user','succeeded','failed','canceled')),
                       started_by_message_id      VARCHAR(255),
                       retry_of                   VARCHAR(255),
                       agent_id_used              VARCHAR(255),
                       agent_config_used_id       VARCHAR(255),
                       model_override_provider    TEXT,
                       model_override             TEXT,
                       model_params_override      TEXT,

                       CONSTRAINT fk_turns_conversation
                           FOREIGN KEY (conversation_id) REFERENCES conversation(id) ON DELETE CASCADE
);

CREATE INDEX idx_turns_conversation ON turns(conversation_id);

CREATE TABLE `message` (
                           id                  VARCHAR(255) PRIMARY KEY,
                           conversation_id     VARCHAR(255) NOT NULL,
                           turn_id             VARCHAR(255),
                           sequence            BIGINT,
                           created_at          TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
                           status              VARCHAR(255),
                           role                VARCHAR(255) NOT NULL CHECK (role IN ('system','user','assistant','tool','control')),
                           `type`              VARCHAR(255) NOT NULL DEFAULT 'text' CHECK (`type` IN ('text','tool_op','control')),
                           content             MEDIUMTEXT NOT NULL,
                           context_summary     TEXT,
                           tags                TEXT,
                           interim             BIGINT NOT NULL DEFAULT 0 CHECK (interim IN (0,1)),
                           elicitation_id      VARCHAR(255),
                           parent_message_id   VARCHAR(255),
                           superseded_by       VARCHAR(255),
    -- legacy column to remain compatible with older readers
                           tool_name           TEXT,

                           CONSTRAINT fk_message_conversation
                               FOREIGN KEY (conversation_id) REFERENCES conversation(id) ON DELETE CASCADE,
                           CONSTRAINT fk_message_turn
                               FOREIGN KEY (turn_id)        REFERENCES turns(id)        ON DELETE SET NULL
);

CREATE UNIQUE INDEX idx_message_turn_seq ON `message`(turn_id, sequence);
CREATE INDEX idx_msg_conv_created ON `message` (conversation_id, created_at DESC);

CREATE TABLE call_payloads (
                               id                         VARCHAR(255) PRIMARY KEY,
                               tenant_id                  VARCHAR(255),
                               kind                       VARCHAR(255) NOT NULL CHECK (kind IN ('model_request','model_response','tool_request','tool_response', 'elicitation_request')),
                               subtype                    TEXT,
                               mime_type                  TEXT NOT NULL,
                               size_bytes                 BIGINT NOT NULL,
                               digest                     VARCHAR(255),
                               storage                    VARCHAR(255) NOT NULL CHECK (storage IN ('inline','object')),
                               inline_body                LONGBLOB,
                               uri                        TEXT,
                               compression                VARCHAR(255) NOT NULL DEFAULT 'none' CHECK (compression IN ('none','gzip','zstd')),
                               encryption_kms_key_id      TEXT,
                               redaction_policy_version   TEXT,
                               redacted                   BIGINT NOT NULL DEFAULT 0 CHECK (redacted IN (0,1)),
                               created_at                 TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
                               schema_ref                 TEXT,
                               preview                    TEXT,
                               tags                       TEXT,
                               CHECK (
                                   (storage='inline' AND inline_body IS NOT NULL AND uri IS NULL) OR
                                   (storage='object' AND uri IS NOT NULL AND inline_body IS NULL)
                                   )
);

CREATE INDEX idx_payloads_tenant_kind ON call_payloads(tenant_id, kind, created_at);
CREATE INDEX idx_payloads_digest ON call_payloads(digest);

CREATE TABLE model_calls (
                             message_id                 VARCHAR(255) PRIMARY KEY,
                             turn_id                    VARCHAR(255),
                             provider                   TEXT NOT NULL,
                             model                      VARCHAR(255) NOT NULL,
                             model_kind                 VARCHAR(255) NOT NULL CHECK (model_kind IN ('chat','completion','vision','reranker','embedding','other')),
                             prompt_snapshot            TEXT,
                             prompt_ref                 TEXT,
                             prompt_hash                TEXT,
                             response_snapshot          TEXT,
                             response_ref               TEXT,
                             finish_reason              TEXT,
                             prompt_tokens              BIGINT,
                             completion_tokens          BIGINT,
                             total_tokens               BIGINT,
                             started_at                 TIMESTAMP NULL DEFAULT NULL,
                             completed_at               TIMESTAMP NULL DEFAULT NULL,
                             latency_ms                 BIGINT,
                             cache_hit                  BIGINT NOT NULL DEFAULT 0 CHECK (cache_hit IN (0,1)),
                             cache_key                  TEXT,
                             cost                       DOUBLE,
                             safety_blocked             BIGINT CHECK (safety_blocked IN (0,1)),
                             safety_reasons             TEXT,
                             redaction_policy_version   TEXT,
                             redacted                   BIGINT NOT NULL DEFAULT 0 CHECK (redacted IN (0,1)),
                             trace_id                   TEXT,
                             span_id                    TEXT,
                             request_payload_id         VARCHAR(255),
                             response_payload_id        VARCHAR(255),

                             CONSTRAINT fk_model_calls_message
                                 FOREIGN KEY (message_id)          REFERENCES `message`(id)      ON DELETE CASCADE,
                             CONSTRAINT fk_model_calls_turn
                                 FOREIGN KEY (turn_id)             REFERENCES turns(id)          ON DELETE SET NULL,
                             CONSTRAINT fk_model_calls_req_payload
                                 FOREIGN KEY (request_payload_id)  REFERENCES call_payloads(id)  ON DELETE SET NULL,
                             CONSTRAINT fk_model_calls_res_payload
                                 FOREIGN KEY (response_payload_id) REFERENCES call_payloads(id)  ON DELETE SET NULL
);

CREATE INDEX idx_model_calls_model ON model_calls(model);
CREATE INDEX idx_model_calls_started_at ON model_calls(started_at);

CREATE TABLE tool_calls (
                            message_id                 VARCHAR(255) PRIMARY KEY,
                            turn_id                    VARCHAR(255),
                            op_id                      VARCHAR(255) NOT NULL,
                            attempt                    BIGINT NOT NULL DEFAULT 1,
                            tool_name                  VARCHAR(255) NOT NULL,
                            tool_kind                  VARCHAR(255) NOT NULL CHECK (tool_kind IN ('general','resource')),
                            capability_tags            TEXT,
                            resource_uris              TEXT,
                            status                     VARCHAR(255) NOT NULL CHECK (status IN ('queued','running','completed','failed','skipped','canceled')),
                            request_snapshot           TEXT,
                            request_ref                TEXT,
                            request_hash               TEXT,
                            response_snapshot          TEXT,
                            response_ref               TEXT,
                            error_code                 TEXT,
                            error_message              TEXT,
                            retriable                  BIGINT CHECK (retriable IN (0,1)),
                            started_at                 TIMESTAMP NULL DEFAULT NULL,
                            completed_at               TIMESTAMP NULL DEFAULT NULL,
                            latency_ms                 BIGINT,
                            cost                       DOUBLE,
                            trace_id                   TEXT,
                            span_id                    TEXT,
                            request_payload_id         VARCHAR(255),
                            response_payload_id        VARCHAR(255),

                            CONSTRAINT fk_tool_calls_message
                                FOREIGN KEY (message_id)          REFERENCES `message`(id)      ON DELETE CASCADE,
                            CONSTRAINT fk_tool_calls_turn
                                FOREIGN KEY (turn_id)             REFERENCES turns(id)          ON DELETE SET NULL,
                            CONSTRAINT fk_tool_calls_req_payload
                                FOREIGN KEY (request_payload_id)  REFERENCES call_payloads(id)  ON DELETE SET NULL,
                            CONSTRAINT fk_tool_calls_res_payload
                                FOREIGN KEY (response_payload_id) REFERENCES call_payloads(id)  ON DELETE SET NULL
);

CREATE UNIQUE INDEX idx_tool_ops_attempt ON tool_calls(turn_id, op_id, attempt);
CREATE INDEX idx_tool_calls_status ON tool_calls(status);
CREATE INDEX idx_tool_calls_name ON tool_calls(tool_name);
CREATE INDEX idx_tool_calls_op ON tool_calls(turn_id, op_id);
