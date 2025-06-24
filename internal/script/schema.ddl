DROP TABLE IF EXISTS conversation;
DROP TABLE IF EXISTS message;
DROP TABLE IF EXISTS tool_call;

CREATE TABLE conversation
(
    id            UUID PRIMARY KEY,
    summary       TEXT,
    agent_name    TEXT,
    created_at    TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    last_activity TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    usage_input_tokens INT DEFAULT 0,
    usage_output_tokens INT DEFAULT 0,
    usage_embedding_tokens INT DEFAULT 0
);


-- Token usage breakdown per model
CREATE TABLE conversation_model_usage
(
    conversation_id UUID REFERENCES conversation (id) ON DELETE CASCADE,
    model_name      TEXT NOT NULL,
    input_tokens    INT DEFAULT 0,
    output_tokens   INT DEFAULT 0,
    embedding_tokens INT DEFAULT 0,
    PRIMARY KEY (conversation_id, model_name)
);


CREATE TABLE message
(
    id              UUID PRIMARY KEY,
    conversation_id UUID REFERENCES conversation (id) ON DELETE CASCADE,
    role            TEXT NOT NULL,
    content         TEXT NOT NULL,
    tool_name       TEXT,
    created_at      TIMESTAMP  DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX idx_msg_conv_created ON message (conversation_id, created_at DESC);


CREATE TABLE tool_call
(
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    conversation_id UUID REFERENCES conversation (id) ON DELETE SET NULL,
    tool_name       TEXT NOT NULL,
    arguments       TEXT,
    result          TEXT,
    succeeded       BOOLEAN,    
    error_msg       TEXT,
    started_at      TIMESTAMP  DEFAULT CURRENT_TIMESTAMP,
    finished_at     TIMESTAMP
);


