PRAGMA foreign_keys = ON;

CREATE TABLE IF NOT EXISTS conversation (
    id TEXT PRIMARY KEY,
    summary TEXT,
    last_activity DATETIME,
    usage_input_tokens INTEGER DEFAULT 0,
    usage_output_tokens INTEGER DEFAULT 0,
    usage_embedding_tokens INTEGER DEFAULT 0,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME,
    created_by_user_id TEXT,
    agent_id TEXT,
    default_model_provider TEXT,
    default_model TEXT,
    default_model_params TEXT,
    title TEXT,
    conversation_parent_id TEXT,
    conversation_parent_turn_id TEXT,
    metadata TEXT,
    visibility TEXT NOT NULL DEFAULT 'private',
    shareable INTEGER NOT NULL DEFAULT 0 CHECK (shareable IN (0,1)),
    status TEXT,
    scheduled INTEGER,
    schedule_id TEXT,
    schedule_run_id TEXT,
    schedule_kind TEXT,
    schedule_timezone TEXT,
    schedule_cron_expr TEXT,
    external_task_ref TEXT
);

CREATE TABLE IF NOT EXISTS turn (
    id TEXT PRIMARY KEY,
    conversation_id TEXT NOT NULL,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    queue_seq INTEGER,
    status TEXT NOT NULL,
    error_message TEXT,
    started_by_message_id TEXT,
    retry_of TEXT,
    agent_id_used TEXT,
    agent_config_used_id TEXT,
    model_override_provider TEXT,
    model_override TEXT,
    model_params_override TEXT,
    run_id TEXT,
    FOREIGN KEY (conversation_id) REFERENCES conversation(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_turn_conversation ON turn(conversation_id);
CREATE INDEX IF NOT EXISTS idx_turn_conv_status_created ON turn(conversation_id, status, created_at);
CREATE INDEX IF NOT EXISTS idx_turn_conv_queue_seq ON turn(conversation_id, queue_seq);

CREATE TABLE IF NOT EXISTS turn_queue (
    id TEXT PRIMARY KEY,
    conversation_id TEXT NOT NULL,
    turn_id TEXT NOT NULL,
    message_id TEXT NOT NULL,
    queue_seq INTEGER NOT NULL,
    status TEXT NOT NULL DEFAULT 'queued',
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME,
    FOREIGN KEY (conversation_id) REFERENCES conversation(id) ON DELETE CASCADE,
    FOREIGN KEY (turn_id) REFERENCES turn(id) ON DELETE CASCADE,
    FOREIGN KEY (message_id) REFERENCES message(id) ON DELETE CASCADE
);

CREATE UNIQUE INDEX IF NOT EXISTS ux_turn_queue_turn_id ON turn_queue(turn_id);
CREATE UNIQUE INDEX IF NOT EXISTS ux_turn_queue_message_id ON turn_queue(message_id);
CREATE INDEX IF NOT EXISTS idx_turn_queue_conv_status_seq ON turn_queue(conversation_id, status, queue_seq, created_at);

CREATE TABLE IF NOT EXISTS call_payload (
    id TEXT PRIMARY KEY,
    tenant_id TEXT,
    kind TEXT NOT NULL,
    subtype TEXT,
    mime_type TEXT NOT NULL,
    size_bytes INTEGER NOT NULL,
    digest TEXT,
    storage TEXT NOT NULL,
    inline_body BLOB,
    uri TEXT,
    compression TEXT NOT NULL DEFAULT 'none',
    encryption_kms_key_id TEXT,
    redaction_policy_version TEXT,
    redacted INTEGER NOT NULL DEFAULT 0,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    schema_ref TEXT
);

CREATE INDEX IF NOT EXISTS idx_payload_tenant_kind ON call_payload(tenant_id, kind, created_at);
CREATE INDEX IF NOT EXISTS idx_payload_digest ON call_payload(digest);

CREATE TABLE IF NOT EXISTS message (
    id TEXT PRIMARY KEY,
    conversation_id TEXT NOT NULL,
    turn_id TEXT,
    archived INTEGER,
    sequence INTEGER,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME,
    created_by_user_id TEXT,
    status TEXT,
    mode TEXT,
    role TEXT NOT NULL,
    type TEXT NOT NULL DEFAULT 'text',
    content TEXT,
    raw_content TEXT,
    summary TEXT,
    context_summary TEXT,
    tags TEXT,
    interim INTEGER NOT NULL DEFAULT 0,
    elicitation_id TEXT,
    parent_message_id TEXT,
    superseded_by TEXT,
    linked_conversation_id TEXT,
    attachment_payload_id TEXT,
    elicitation_payload_id TEXT,
    tool_name TEXT,
    embedding_index BLOB,
    preamble TEXT,
    iteration INTEGER,
    phase TEXT DEFAULT 'final',
    FOREIGN KEY (conversation_id) REFERENCES conversation(id) ON DELETE CASCADE,
    FOREIGN KEY (turn_id) REFERENCES turn(id) ON DELETE SET NULL,
    FOREIGN KEY (attachment_payload_id) REFERENCES call_payload(id) ON DELETE SET NULL,
    FOREIGN KEY (elicitation_payload_id) REFERENCES call_payload(id) ON DELETE SET NULL
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_message_turn_seq ON message(turn_id, sequence);
CREATE INDEX IF NOT EXISTS idx_msg_conv_created ON message(conversation_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_message_iteration ON message(turn_id, iteration, created_at);

CREATE TABLE IF NOT EXISTS users (
    id TEXT PRIMARY KEY,
    username TEXT NOT NULL UNIQUE,
    display_name TEXT,
    email TEXT,
    provider TEXT NOT NULL DEFAULT 'local',
    subject TEXT,
    hash_ip TEXT,
    timezone TEXT NOT NULL DEFAULT 'UTC',
    default_agent_ref TEXT,
    default_model_ref TEXT,
    default_embedder_ref TEXT,
    settings TEXT,
    disabled INTEGER NOT NULL DEFAULT 0,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME
);

CREATE UNIQUE INDEX IF NOT EXISTS ux_users_provider_subject ON users(provider, subject);
CREATE INDEX IF NOT EXISTS ix_users_hash_ip ON users(hash_ip);

CREATE TABLE IF NOT EXISTS model_call (
    message_id TEXT PRIMARY KEY,
    turn_id TEXT,
    provider TEXT NOT NULL,
    model TEXT NOT NULL,
    model_kind TEXT NOT NULL,
    error_code TEXT,
    error_message TEXT,
    finish_reason TEXT,
    prompt_tokens INTEGER,
    prompt_cached_tokens INTEGER,
    completion_tokens INTEGER,
    total_tokens INTEGER,
    prompt_audio_tokens INTEGER,
    completion_reasoning_tokens INTEGER,
    completion_audio_tokens INTEGER,
    completion_accepted_prediction_tokens INTEGER,
    completion_rejected_prediction_tokens INTEGER,
    status TEXT NOT NULL,
    started_at DATETIME,
    completed_at DATETIME,
    latency_ms INTEGER,
    cost REAL,
    trace_id TEXT,
    span_id TEXT,
    request_payload_id TEXT,
    response_payload_id TEXT,
    provider_request_payload_id TEXT,
    provider_response_payload_id TEXT,
    stream_payload_id TEXT,
    run_id TEXT,
    iteration INTEGER,
    FOREIGN KEY (message_id) REFERENCES message(id) ON DELETE CASCADE,
    FOREIGN KEY (turn_id) REFERENCES turn(id) ON DELETE SET NULL,
    FOREIGN KEY (request_payload_id) REFERENCES call_payload(id) ON DELETE SET NULL,
    FOREIGN KEY (response_payload_id) REFERENCES call_payload(id) ON DELETE SET NULL,
    FOREIGN KEY (provider_request_payload_id) REFERENCES call_payload(id) ON DELETE SET NULL,
    FOREIGN KEY (provider_response_payload_id) REFERENCES call_payload(id) ON DELETE SET NULL,
    FOREIGN KEY (stream_payload_id) REFERENCES call_payload(id) ON DELETE SET NULL
);

CREATE INDEX IF NOT EXISTS idx_model_call_model ON model_call(model);
CREATE INDEX IF NOT EXISTS idx_model_call_started_at ON model_call(started_at);
CREATE INDEX IF NOT EXISTS idx_model_call_run ON model_call(run_id, iteration);

CREATE TABLE IF NOT EXISTS tool_call (
    message_id TEXT PRIMARY KEY,
    turn_id TEXT,
    op_id TEXT NOT NULL,
    attempt INTEGER NOT NULL DEFAULT 1,
    tool_name TEXT NOT NULL,
    tool_kind TEXT NOT NULL,
    status TEXT NOT NULL,
    request_hash TEXT,
    error_code TEXT,
    error_message TEXT,
    retriable INTEGER,
    started_at DATETIME,
    completed_at DATETIME,
    latency_ms INTEGER,
    cost REAL,
    trace_id TEXT,
    span_id TEXT,
    request_payload_id TEXT,
    response_payload_id TEXT,
    run_id TEXT,
    iteration INTEGER,
    FOREIGN KEY (message_id) REFERENCES message(id) ON DELETE CASCADE,
    FOREIGN KEY (turn_id) REFERENCES turn(id) ON DELETE SET NULL,
    FOREIGN KEY (request_payload_id) REFERENCES call_payload(id) ON DELETE SET NULL,
    FOREIGN KEY (response_payload_id) REFERENCES call_payload(id) ON DELETE SET NULL
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_tool_op_attempt ON tool_call(turn_id, op_id, attempt);
CREATE INDEX IF NOT EXISTS idx_tool_call_status ON tool_call(status);
CREATE INDEX IF NOT EXISTS idx_tool_call_name ON tool_call(tool_name);
CREATE INDEX IF NOT EXISTS idx_tool_call_op ON tool_call(turn_id, op_id);
CREATE INDEX IF NOT EXISTS idx_tool_call_run ON tool_call(run_id, iteration);

CREATE TABLE IF NOT EXISTS tool_approval_queue (
    id TEXT PRIMARY KEY,
    user_id TEXT NOT NULL,
    conversation_id TEXT,
    turn_id TEXT,
    message_id TEXT,
    tool_name TEXT NOT NULL,
    title TEXT,
    arguments BLOB NOT NULL,
    metadata BLOB,
    status TEXT NOT NULL DEFAULT 'pending',
    decision TEXT,
    approved_by_user_id TEXT,
    approved_at DATETIME,
    executed_at DATETIME,
    error_message TEXT,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME,
    FOREIGN KEY (conversation_id) REFERENCES conversation(id) ON DELETE CASCADE,
    FOREIGN KEY (turn_id) REFERENCES turn(id) ON DELETE SET NULL,
    FOREIGN KEY (message_id) REFERENCES message(id) ON DELETE SET NULL
);

CREATE INDEX IF NOT EXISTS idx_taq_user_status_created ON tool_approval_queue(user_id, status, created_at);
CREATE INDEX IF NOT EXISTS idx_taq_conversation_status ON tool_approval_queue(conversation_id, status, created_at);
CREATE INDEX IF NOT EXISTS idx_taq_turn ON tool_approval_queue(turn_id, created_at);

CREATE TABLE IF NOT EXISTS schedule (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL UNIQUE,
    description TEXT,
    created_by_user_id TEXT,
    visibility TEXT NOT NULL DEFAULT 'private',
    agent_ref TEXT NOT NULL,
    model_override TEXT,
    user_cred_url TEXT,
    enabled INTEGER NOT NULL DEFAULT 1,
    start_at DATETIME,
    end_at DATETIME,
    schedule_type TEXT NOT NULL DEFAULT 'cron',
    cron_expr TEXT,
    interval_seconds INTEGER,
    timezone TEXT NOT NULL DEFAULT 'UTC',
    timeout_seconds INTEGER NOT NULL DEFAULT 0,
    task_prompt_uri TEXT,
    task_prompt TEXT,
    next_run_at DATETIME,
    last_run_at DATETIME,
    last_status TEXT,
    last_error TEXT,
    lease_owner TEXT,
    lease_until DATETIME,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME
);

CREATE INDEX IF NOT EXISTS idx_schedule_enabled_next ON schedule(enabled, next_run_at);
CREATE INDEX IF NOT EXISTS idx_schedule_enabled_next_lease ON schedule(enabled, next_run_at, lease_until);

CREATE TABLE IF NOT EXISTS run (
    id TEXT PRIMARY KEY,
    turn_id TEXT,
    schedule_id TEXT,
    conversation_id TEXT,
    conversation_kind TEXT NOT NULL DEFAULT 'interactive',
    attempt INTEGER NOT NULL DEFAULT 1,
    resumed_from_run_id TEXT,
    status TEXT NOT NULL DEFAULT 'pending',
    error_code TEXT,
    error_message TEXT,
    iteration INTEGER NOT NULL DEFAULT 0,
    max_iterations INTEGER,
    checkpoint_response_id TEXT,
    checkpoint_message_id TEXT,
    checkpoint_data TEXT,
    agent_id TEXT,
    model_provider TEXT,
    model TEXT,
    worker_id TEXT,
    worker_pid INTEGER,
    worker_host TEXT,
    lease_owner TEXT,
    lease_until DATETIME,
    last_heartbeat_at DATETIME,
    security_context TEXT,
    effective_user_id TEXT,
    auth_authority TEXT,
    auth_audience TEXT,
    user_cred_url TEXT,
    heartbeat_interval_sec INTEGER DEFAULT 5,
    scheduled_for DATETIME,
    precondition_ran_at DATETIME,
    precondition_passed INTEGER,
    precondition_result TEXT,
    usage_prompt_tokens INTEGER DEFAULT 0,
    usage_completion_tokens INTEGER DEFAULT 0,
    usage_total_tokens INTEGER DEFAULT 0,
    usage_cost REAL DEFAULT 0,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME,
    started_at DATETIME,
    completed_at DATETIME,
    FOREIGN KEY (conversation_id) REFERENCES conversation(id) ON DELETE CASCADE,
    FOREIGN KEY (turn_id) REFERENCES turn(id) ON DELETE SET NULL,
    FOREIGN KEY (schedule_id) REFERENCES schedule(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_run_turn ON run(turn_id, attempt);
CREATE INDEX IF NOT EXISTS idx_run_worker ON run(worker_id, status);
CREATE INDEX IF NOT EXISTS idx_run_heartbeat ON run(status, last_heartbeat_at);
CREATE INDEX IF NOT EXISTS idx_run_conversation ON run(conversation_id, status);
CREATE INDEX IF NOT EXISTS idx_run_schedule_status ON run(schedule_id, status);
CREATE UNIQUE INDEX IF NOT EXISTS ux_run_schedule_slot ON run(schedule_id, scheduled_for);
CREATE INDEX IF NOT EXISTS idx_run_pid ON run(worker_pid, worker_host, status);

CREATE TABLE IF NOT EXISTS user_oauth_token (
    user_id TEXT NOT NULL,
    provider TEXT NOT NULL,
    enc_token TEXT NOT NULL,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME,
    version INTEGER NOT NULL DEFAULT 0,
    lease_owner TEXT,
    lease_until DATETIME,
    refresh_status TEXT NOT NULL DEFAULT 'idle',
    PRIMARY KEY (user_id, provider),
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS generated_file (
    id TEXT PRIMARY KEY,
    conversation_id TEXT NOT NULL,
    turn_id TEXT,
    message_id TEXT,
    provider TEXT NOT NULL,
    mode TEXT NOT NULL,
    copy_mode TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'ready',
    payload_id TEXT,
    container_id TEXT,
    provider_file_id TEXT,
    filename TEXT,
    mime_type TEXT,
    size_bytes INTEGER,
    checksum TEXT,
    error_message TEXT,
    dedup_key TEXT,
    expires_at DATETIME,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (conversation_id) REFERENCES conversation(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_gf_conversation ON generated_file(conversation_id);
CREATE INDEX IF NOT EXISTS idx_gf_dedup ON generated_file(dedup_key);

CREATE TABLE IF NOT EXISTS workspace_resources (
    kind TEXT NOT NULL,
    name TEXT NOT NULL,
    data BLOB NOT NULL,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (kind, name)
);

CREATE TABLE IF NOT EXISTS session (
    id TEXT PRIMARY KEY,
    user_id TEXT NOT NULL,
    provider TEXT NOT NULL DEFAULT 'local',
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME,
    expires_at DATETIME NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_session_user_id ON session(user_id);
CREATE INDEX IF NOT EXISTS idx_session_expires_at ON session(expires_at);
