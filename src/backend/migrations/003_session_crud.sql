-- 003_session_crud.sql
-- session-crud-service tables.
-- Stores interview sessions with their parameters and generated programs.

CREATE TABLE IF NOT EXISTS session_crud.sessions (
    session_id          UUID PRIMARY KEY,
    user_id             UUID NOT NULL,
    start_time          TIMESTAMP WITH TIME ZONE NOT NULL,
    end_time            TIMESTAMP WITH TIME ZONE,
    params              JSONB NOT NULL,           -- topics, mode, level, source
    interview_program   JSONB,                    -- generated question plan
    program_status      VARCHAR(32),              -- 'ok', 'failed', 'fallback'
    program_meta        JSONB,                    -- coverage, validation details
    program_version     VARCHAR(64),
    created_at          TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at          TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_sessions_user_id
    ON session_crud.sessions(user_id, start_time DESC);
CREATE INDEX IF NOT EXISTS idx_sessions_start_time
    ON session_crud.sessions(start_time DESC);
