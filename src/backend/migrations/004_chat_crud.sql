-- 004_chat_crud.sql
-- chat-crud-service tables.
-- Stores individual messages and full chat dumps per session.

-- Individual messages (user and model turns)
CREATE TABLE IF NOT EXISTS chat_crud.messages (
    id          BIGSERIAL PRIMARY KEY,
    session_id  UUID NOT NULL,
    type        VARCHAR(20) NOT NULL,   -- 'user', 'model', 'system'
    content     TEXT NOT NULL,
    metadata    JSONB,                  -- question_id, decision, score, etc.
    created_at  TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_messages_session_id
    ON chat_crud.messages(session_id, created_at ASC);

-- Full chat dump per session (aggregated JSON for history retrieval)
CREATE TABLE IF NOT EXISTS chat_crud.chat_dumps (
    id          BIGSERIAL PRIMARY KEY,
    session_id  UUID NOT NULL UNIQUE,
    chat        JSONB NOT NULL,         -- full ordered dialogue array
    created_at  TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at  TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_chat_dumps_session_id
    ON chat_crud.chat_dumps(session_id);
